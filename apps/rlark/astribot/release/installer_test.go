package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const newInstaller = "registry.rlark.internal:5443/rlark/staging-artifact@sha256:bbbbbbbbbbbb00000000000000000000000000000000000000000000000000b1"

// fakeBootstrap is Station API's machine: the installer laid down is served once Station API reads it.
type fakeBootstrap struct {
	member kubernetes.Interface
	put    []string
}

func (b *fakeBootstrap) Put(_ context.Context, artifact string) (string, string, error) {
	b.put = append(b.put, artifact)
	return "install-v1.33.13-1111111.bundle.tar", strings.Repeat("ab", 32), nil
}

func (b *fakeBootstrap) Served(ctx context.Context) (string, error) {
	deployment, err := b.member.AppsV1().Deployments(stationNamespace).Get(ctx, stationAPI, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	for _, arg := range deployment.Spec.Template.Spec.Containers[0].Args {
		if arg == apiBundleFlag+bootstrapMount+"install-v1.33.13-1111111.bundle.tar" {
			return strings.Repeat("ab", 32), nil
		}
	}
	return strings.Repeat("cd", 32), nil
}

// The environment's installer goes to Station API's machine in the station-api stage, both Station Deployments read
// it and say which artifact it is, and Station API serves it before the stage is done; the next run has nothing to
// do for it.
func TestApplyLaysDownTheEnvironmentsInstaller(t *testing.T) {
	s := newFakeSite(t)
	office := s.members["office"]
	for name, flag := range map[string]string{stationAPI: apiBundleFlag, stationController: controllerBundleFlag} {
		deployment, err := office.AppsV1().Deployments(stationNamespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		deployment.Spec.Template.Spec.Containers[0].Args = []string{"--bootstrap-ca-file=/var/run/station-bootstrap/ca.crt", flag + bootstrapMount + "install-v1.33.13-6ac8e8c.bundle.tar"}
		if _, err := office.AppsV1().Deployments(stationNamespace).Update(context.Background(), deployment, metav1.UpdateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	_, environment, release := writeFiles(t)
	// The environment's installer is the release's image installer-<environment>, built from the environment's base.
	release.Images["installer-"+environment.Name] = newInstaller
	desired := environment.Desired(release, testClusters)
	var out bytes.Buffer
	run := newRun(t, s, environment, desired, &out)
	host := &fakeBootstrap{member: office}
	run.Site.Installer = host
	current, err := run.Site.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := run.Plan(context.Background(), current)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(func() []string {
		var lines []string
		for _, change := range plan {
			lines = append(lines, change.String())
		}
		return lines
	}(), "\n"), "station-api      installer: (none) -> registry.rlark.internal:5443/rlark/staging-artifact@sha256:bbbbbbbbbbbb") {
		t.Fatalf("plan %v", plan)
	}
	if err := run.Apply(context.Background()); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(host.put) != 1 || host.put[0] != newInstaller {
		t.Fatalf("laid down %v", host.put)
	}
	for name, flag := range map[string]string{stationAPI: apiBundleFlag, stationController: controllerBundleFlag} {
		deployment, _ := office.AppsV1().Deployments(stationNamespace).Get(context.Background(), name, metav1.GetOptions{})
		args := deployment.Spec.Template.Spec.Containers[0].Args
		if deployment.Annotations[installerAnnotation] != newInstaller || args[1] != flag+bootstrapMount+"install-v1.33.13-1111111.bundle.tar" || args[0] != "--bootstrap-ca-file=/var/run/station-bootstrap/ca.crt" {
			t.Errorf("%s: annotations %v, args %v", name, deployment.Annotations, args)
		}
	}
	if current, err = run.Site.Read(context.Background()); err != nil || current.Installer != newInstaller {
		t.Fatalf("read back %q, %v", current.Installer, err)
	}
	if changes := Changes("station-api", current, desired); len(changes) != 0 {
		t.Fatalf("a second run would change %v", changes)
	}
}

// The installer is taken out of the artifact's layer as it lies in the registry, under its own name, readable by
// Station's containers; a file of that name is not written again.
func TestStationBootstrapLaysDownTheArtifactsBundle(t *testing.T) {
	var layer bytes.Buffer
	compressed := gzip.NewWriter(&layer)
	archive := tar.NewWriter(compressed)
	for name, content := range map[string]string{"Dockerfile": "FROM scratch", "install-v1.33.13-2b1e626.bundle.tar": "the bundle"} {
		_ = archive.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(content)), Typeflag: tar.TypeReg})
		_, _ = archive.Write([]byte(content))
	}
	archive.Close()
	compressed.Close()
	layerDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(layer.Bytes()))
	manifest := fmt.Sprintf(`{"schemaVersion":2,"layers":[{"digest":%q}]}`, layerDigest)
	manifestDigest := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(manifest)))
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/v2/rlark/staging-artifact/manifests/" + manifestDigest:
			_, _ = w.Write([]byte(manifest))
		case "/v2/rlark/staging-artifact/blobs/" + layerDigest:
			_, _ = w.Write(layer.Bytes())
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "https://")
	dir := t.TempDir()
	b := &StationBootstrap{Dir: dir, Owner: os.Getuid(), Registry: &RegistryClient{Registry: host, base: server.URL, HTTP: server.Client()}}
	file, sum, err := b.Put(context.Background(), host+"/rlark/staging-artifact@"+manifestDigest)
	if err != nil {
		t.Fatal(err)
	}
	info, statErr := os.Stat(filepath.Join(dir, file))
	if file != "install-v1.33.13-2b1e626.bundle.tar" || sum != fmt.Sprintf("%x", sha256.Sum256([]byte("the bundle"))) || statErr != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("laid down %q %s: %v %v", file, sum, info, statErr)
	}
	before := requests
	if again, againSum, err := b.Put(context.Background(), host+"/rlark/staging-artifact@"+manifestDigest); err != nil || again != file || againSum != sum || requests != before+2 {
		t.Fatalf("again: %q %s %v after %d requests", again, againSum, err, requests-before)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("the directory holds %v", entries)
	}
}
