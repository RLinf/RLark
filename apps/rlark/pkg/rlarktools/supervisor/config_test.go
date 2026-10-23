package supervisor

import (
	"testing"

	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
)

func TestConfigValidate(t *testing.T) {
	valid := Config{APIVersion: APIVersion, Processes: []ProcessConfig{{Name: "ray", Role: executionprocess.RolePrimary, Argv: []string{"/bin/true"}, RestartPolicy: executionprocess.RestartNever, Required: true}, {Name: "ssh", Role: executionprocess.RoleAuxiliary, Argv: []string{"/bin/true"}, RestartPolicy: executionprocess.RestartOnFailure, Required: true}}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.Processes = append(invalid.Processes, invalid.Processes[0])
	if err := invalid.Validate(); err == nil {
		t.Fatal("duplicate process was accepted")
	}
}
