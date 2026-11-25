// Package envtest starts a real Kubernetes control plane (etcd + kube-apiserver)
// with the RLark CRDs installed, so controller tests can assert behaviour that a
// fake client cannot faithfully reproduce: metadata.generation bumps, status
// subresource isolation, CRD schema validation and defaulting.
//
// Tests built on this helper are opt-in. They are skipped when `go test -short`
// is used (the mode `make test-go` runs in) and when the envtest binaries are
// not available. Run `make test-envtest` in apps/rlark to download the binaries
// and execute them.
package envtest

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// assetsEnvVar is the variable setup-envtest exports to point at the etcd and
// kube-apiserver binaries.
const assetsEnvVar = "KUBEBUILDER_ASSETS"

// Scheme returns a scheme containing the built-in Kubernetes types plus the
// RLark API group, matching what the controller manager registers.
func Scheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("add client-go scheme: %v", err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("add RLark scheme: %v", err)
	}
	return scheme
}

// Start brings up an API server with the RLark CRDs installed and returns its
// rest config. The environment is stopped when the test finishes. The test is
// skipped instead of failed when the environment cannot be used, so that the
// default unit-test run stays hermetic.
func Start(t *testing.T) *rest.Config {
	t.Helper()

	if testing.Short() {
		t.Skip("envtest requires a real API server; skipped in -short mode")
	}
	if os.Getenv(assetsEnvVar) == "" {
		t.Skipf("%s is not set; run `make setup-envtest` in apps/rlark to download the envtest binaries", assetsEnvVar)
	}

	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{crdDirectory(t)},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := env.Start()
	if err != nil {
		t.Fatalf("start envtest environment: %v", err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Errorf("stop envtest environment: %v", err)
		}
	})
	return cfg
}

// crdDirectory locates api/config/crd/bases by walking up from the test's
// working directory, so the helper does not depend on how deeply nested the
// calling package is.
func crdDirectory(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolve working directory: %v", err)
	}
	for {
		candidate := filepath.Join(dir, "api", "config", "crd", "bases")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("locate api/config/crd/bases: not found in any parent directory")
		}
		dir = parent
	}
}
