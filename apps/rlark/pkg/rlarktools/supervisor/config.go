// Package supervisor runs a bounded set of trusted workload processes.
//
// A required auxiliary declares a service that must remain running for the
// primary's lifetime. Any exit is a service failure, including exit code zero.
// RestartOnFailure permits bounded recovery from non-zero exits; after a stable
// run, the consecutive-failure budget resets so occasional failures do not
// consume a lifetime restart allowance.
//
// On Linux, child-subreaper setup is best effort. The production supervisor is
// container PID 1 and therefore already adopts orphaned descendants; a denied
// PR_SET_CHILD_SUBREAPER call does not prevent workload startup.
package supervisor

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
)

const APIVersion = "rlark.io/supervisor/v1alpha1"

type Config struct {
	APIVersion string          `json:"apiVersion"`
	Processes  []ProcessConfig `json:"processes"`
}

type ProcessConfig struct {
	Name          string                         `json:"name"`
	Role          executionprocess.Role          `json:"role"`
	Argv          []string                       `json:"argv"`
	RestartPolicy executionprocess.RestartPolicy `json:"restartPolicy"`
	Required      bool                           `json:"required"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read supervisor config: %w", err)
	}
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return Config{}, fmt.Errorf("decode supervisor config: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	if c.APIVersion != APIVersion {
		return fmt.Errorf("unsupported supervisor apiVersion %q", c.APIVersion)
	}
	primary := 0
	seen := map[string]struct{}{}
	for _, process := range c.Processes {
		if strings.TrimSpace(process.Name) == "" || len(process.Argv) == 0 || !filepath.IsAbs(process.Argv[0]) {
			return fmt.Errorf("process requires a name and absolute executable")
		}
		if _, found := seen[process.Name]; found {
			return fmt.Errorf("duplicate process %q", process.Name)
		}
		seen[process.Name] = struct{}{}
		switch process.Role {
		case executionprocess.RolePrimary:
			primary++
			if process.RestartPolicy != executionprocess.RestartNever || !process.Required {
				return fmt.Errorf("primary process %q must be required and never restarted", process.Name)
			}
		case executionprocess.RoleAuxiliary:
			if process.RestartPolicy != executionprocess.RestartNever && process.RestartPolicy != executionprocess.RestartOnFailure {
				return fmt.Errorf("auxiliary process %q has unsupported restart policy", process.Name)
			}
		default:
			return fmt.Errorf("process %q has unknown role", process.Name)
		}
	}
	if primary != 1 {
		return fmt.Errorf("supervisor requires exactly one primary process")
	}
	return nil
}
