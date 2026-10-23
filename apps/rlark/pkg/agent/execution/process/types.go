// Package process defines the bounded, non-sensitive process declarations that
// trusted Workload Adapters and Environment handlers contribute for one
// execution unit. Runtime-specific assemblers lower these declarations into a
// supervisor configuration.
package process

import agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"

// Role identifies the lifecycle role of a managed process.
type Role string

const (
	RolePrimary   Role = "Primary"
	RoleAuxiliary Role = "Auxiliary"
)

// RestartPolicy defines whether the supervisor may restart a process.
type RestartPolicy string

const (
	RestartNever     RestartPolicy = "Never"
	RestartOnFailure RestartPolicy = "OnFailure"
)

// Declaration describes one trusted process without embedding secret material.
// Target is an exact mutation-target name and Locator fences the object schema.
type Declaration struct {
	Name          string
	Target        string
	Locator       agentruntime.ObjectLocator
	ContainerName string
	Role          Role
	Executable    string
	Args          []string
	RestartPolicy RestartPolicy
	Required      bool
}
