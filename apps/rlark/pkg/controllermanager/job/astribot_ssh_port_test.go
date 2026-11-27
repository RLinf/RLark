package job

import (
	"encoding/json"
	"testing"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// The Job's key always replaces whatever the template carried; the port is
// the template's when it names one, so that host-network Tasks of one node
// can each have their own.
func TestSSHEnvironmentKeepsADeclaredPort(t *testing.T) {
	read := func(spec rlarkv1alpha1.TaskSpec) taskapi.SSHEnvironmentParametersV1 {
		t.Helper()
		if len(spec.Environments) != 1 {
			t.Fatalf("environments: %+v", spec.Environments)
		}
		var parameters taskapi.SSHEnvironmentParametersV1
		if err := json.Unmarshal(spec.Environments[0].Parameters.Raw, &parameters); err != nil {
			t.Fatal(err)
		}
		return parameters
	}
	plain := rlarkv1alpha1.TaskSpec{}
	setSSHEnvironment(&plain, "ssh-ed25519 job")
	if got := read(plain); got.Port != 2222 || len(got.AuthorizedKeys) != 1 || got.AuthorizedKeys[0] != "ssh-ed25519 job" {
		t.Fatalf("default: %+v", got)
	}
	declared := rlarkv1alpha1.TaskSpec{Environments: []rlarkv1alpha1.TaskEnvironment{{
		Type: taskapi.EnvironmentTypeSSH, Version: taskapi.EnvironmentVersionSSHV1,
		Parameters: apiextensionsv1.JSON{Raw: []byte(`{"port":2224,"authorizedKeys":["ssh-ed25519 someone-else"]}`)},
	}}}
	setSSHEnvironment(&declared, "ssh-ed25519 job")
	if got := read(declared); got.Port != 2224 || len(got.AuthorizedKeys) != 1 || got.AuthorizedKeys[0] != "ssh-ed25519 job" {
		t.Fatalf("declared port: %+v", got)
	}
}
