package runtime

import (
	"errors"
	"testing"
)

func TestLogicalTaskIDRequiresNamespaceAndName(t *testing.T) {
	tests := []struct {
		name string
		id   LogicalTaskID
		ok   bool
	}{
		{name: "valid", id: LogicalTaskID{Namespace: "cluster", Name: "task"}, ok: true},
		{name: "missing namespace", id: LogicalTaskID{Name: "task"}},
		{name: "missing name", id: LogicalTaskID{Namespace: "cluster"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.id.Validate(); (err == nil) != tt.ok {
				t.Fatalf("Validate() error = %v, want success = %v", err, tt.ok)
			}
		})
	}
}

func TestValidateObjectsAcceptsDAG(t *testing.T) {
	objects := []DesiredObject{
		object("config"),
		object("service", "config"),
		object("workload", "config", "service"),
	}
	if err := ValidateObjects(objects); err != nil {
		t.Fatalf("ValidateObjects() error = %v", err)
	}
}

func TestValidateObjectsRejectsUnknownDependency(t *testing.T) {
	err := ValidateObjects([]DesiredObject{object("workload", "missing")})
	if err == nil {
		t.Fatal("ValidateObjects() returned nil")
	}
}

func TestValidateObjectsRejectsCycle(t *testing.T) {
	err := ValidateObjects([]DesiredObject{object("a", "b"), object("b", "a")})
	if !errors.Is(err, ErrObjectDependencyCycle) {
		t.Fatalf("ValidateObjects() error = %v, want dependency cycle", err)
	}
}

func TestValidateObjectsRejectsInvalidArchitectureContracts(t *testing.T) {
	tests := []struct {
		name    string
		objects []DesiredObject
	}{
		{name: "missing logical name", objects: []DesiredObject{object("")}},
		{name: "duplicate logical name", objects: []DesiredObject{object("same"), object("same")}},
		{name: "missing owner key", objects: []DesiredObject{func() DesiredObject { value := object("workload"); value.Ownership.OwnerKey = ""; return value }()}},
		{name: "unknown ownership scope", objects: []DesiredObject{func() DesiredObject { value := object("workload"); value.Ownership.Scope = "Unknown"; return value }()}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateObjects(test.objects); err == nil {
				t.Fatal("ValidateObjects() error = nil")
			}
		})
	}
}

func TestResourceOwnershipScopeContracts(t *testing.T) {
	tests := []struct {
		name      string
		ownership ResourceOwnership
		ok        bool
	}{
		{name: "task incarnation", ownership: ResourceOwnership{Scope: IdentityTaskIncarnation, OwnerKey: "uid"}, ok: true},
		{name: "logical task", ownership: ResourceOwnership{Scope: IdentityLogicalTask, OwnerKey: "namespace/name"}, ok: true},
		{name: "runtime instance", ownership: ResourceOwnership{Scope: IdentityRuntimeInstance, OwnerKey: "instance"}, ok: true},
		{name: "shared resource", ownership: ResourceOwnership{Scope: IdentitySharedResource, OwnerKey: "shared"}, ok: true},
		{name: "external", ownership: ResourceOwnership{Scope: IdentityExternal, OwnerKey: "external"}, ok: true},
		{name: "missing owner", ownership: ResourceOwnership{Scope: IdentityLogicalTask}},
		{name: "unknown scope", ownership: ResourceOwnership{Scope: "Unknown", OwnerKey: "owner"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.ownership.Validate(); (err == nil) != test.ok {
				t.Fatalf("Validate() error = %v, want success = %v", err, test.ok)
			}
		})
	}
}

func TestOwnershipDoesNotImplyLifecycle(t *testing.T) {
	retain := object("retain")
	retain.Lifecycle = LifecycleRetain
	cleanup := object("cleanup")
	cleanup.Lifecycle = LifecycleDeleteOnTaskCleanup

	if retain.Ownership != cleanup.Ownership {
		t.Fatal("test objects should have identical ownership")
	}
	if retain.Lifecycle == cleanup.Lifecycle {
		t.Fatal("identical ownership must permit different lifecycle policies")
	}
}

func object(name string, dependencies ...string) DesiredObject {
	return DesiredObject{
		LogicalName: name,
		Ownership: ResourceOwnership{
			Scope:    IdentityLogicalTask,
			OwnerKey: "cluster/task",
		},
		Lifecycle: LifecycleDeleteOnTaskCleanup,
		DependsOn: dependencies,
	}
}
