package binding

import (
	"context"
	"errors"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

type desiredValidatorFunc func(agentruntime.DesiredObject) error

func (f desiredValidatorFunc) ValidateDesired(object agentruntime.DesiredObject) error {
	return f(object)
}

func TestDesiredPlanValidatorValidatesEveryObject(t *testing.T) {
	var names []string
	validator := DesiredPlanValidator{RuntimeKind: "Kubernetes", Objects: desiredValidatorFunc(func(object agentruntime.DesiredObject) error {
		names = append(names, object.LogicalName)
		return nil
	})}
	ownership := agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}
	plan := &executionplan.Plan{Objects: []agentruntime.DesiredObject{{LogicalName: "a", Ownership: ownership}, {LogicalName: "b", Ownership: ownership}}}
	if err := validator.Validate(context.Background(), BindRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}}, plan); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("validated=%v", names)
	}
}

func TestDesiredPlanValidatorPreservesErrorsAndRejectsIncompleteAssembly(t *testing.T) {
	sentinel := errors.New("invalid desired object")
	validator := DesiredPlanValidator{RuntimeKind: "Kubernetes", Objects: desiredValidatorFunc(func(agentruntime.DesiredObject) error { return sentinel })}
	request := BindRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}}
	object := agentruntime.DesiredObject{LogicalName: "object", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}}
	if err := validator.Validate(context.Background(), request, &executionplan.Plan{Objects: []agentruntime.DesiredObject{object}}); !errors.Is(err, sentinel) {
		t.Fatalf("error=%v", err)
	}
	if err := (DesiredPlanValidator{}).Validate(context.Background(), request, &executionplan.Plan{}); !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("missing validator error=%v", err)
	}
	if err := validator.Validate(context.Background(), BindRequest{Runtime: agentruntime.Info{Kind: "Other"}}, &executionplan.Plan{}); !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("wrong Runtime error=%v", err)
	}
	if err := validator.Validate(context.Background(), request, nil); !fault.Is(err, fault.Internal) {
		t.Fatalf("nil Plan error=%v", err)
	}
}

func TestDesiredPlanValidatorRejectsInvalidObjectGraphBeforeRuntimeValidation(t *testing.T) {
	called := false
	validator := DesiredPlanValidator{RuntimeKind: "Kubernetes", Objects: desiredValidatorFunc(func(agentruntime.DesiredObject) error {
		called = true
		return nil
	})}
	object := agentruntime.DesiredObject{LogicalName: "workload", DependsOn: []string{"missing"}, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}}
	err := validator.Validate(context.Background(), BindRequest{Runtime: agentruntime.Info{Kind: "Kubernetes"}}, &executionplan.Plan{Objects: []agentruntime.DesiredObject{object}})
	if !fault.Is(err, fault.InvalidArgument) || called {
		t.Fatalf("error=%v runtime validator called=%t", err, called)
	}
}
