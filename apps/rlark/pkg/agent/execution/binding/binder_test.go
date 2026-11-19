package binding

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

type binderTestHandler struct {
	info      environment.HandlerInfo
	intents   []environment.WriteIntent
	intentErr error
	applyErr  error
}

type binderTestLowerer func(context.Context, agentruntime.UnitLoweringRequest) (agentruntime.DesiredObject, error)

func (f binderTestLowerer) Lower(ctx context.Context, request agentruntime.UnitLoweringRequest) (agentruntime.DesiredObject, error) {
	return f(ctx, request)
}

var noopBinderTestLowerer binderTestLowerer = func(_ context.Context, request agentruntime.UnitLoweringRequest) (agentruntime.DesiredObject, error) {
	return request.Object, nil
}

func (h *binderTestHandler) Info() environment.HandlerInfo { return h.info }

func (h *binderTestHandler) Intents(context.Context, environment.HandlerRequest) ([]environment.WriteIntent, error) {
	return append([]environment.WriteIntent(nil), h.intents...), h.intentErr
}

func (h *binderTestHandler) Apply(_ context.Context, request environment.HandlerRequest) error {
	if h.applyErr != nil {
		return h.applyErr
	}
	plan := request.Plan
	var manifest map[string]any
	if err := json.Unmarshal(plan.Objects[0].Manifest.Object, &manifest); err != nil {
		return err
	}
	manifest["bound"] = request.Binding.RequirementName
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	plan.Objects[0].Manifest.Object = encoded
	return nil
}

func binderFixture(t *testing.T) (BindingPlanBinder, BindRequest) {
	t.Helper()
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	handler := &binderTestHandler{
		info: environment.HandlerInfo{
			ID: "network", Key: environment.HandlerKey{RuntimeKind: "Kubernetes", BindingKind: environment.RequirementNetwork, ProfileType: "CrossDomain"}, Algorithm: "network/v1",
		},
		intents: []environment.WriteIntent{{Slot: "object/workload/bound", Value: []byte("network")}},
	}
	registry, err := environment.NewRegistry([]environment.HandlerRegistration{{Handler: handler}})
	if err != nil {
		t.Fatal(err)
	}
	request := BindRequest{
		Runtime: agentruntime.Info{Kind: "Kubernetes"},
		Plan: executionplan.Plan{
			CompiledHash: "compiled",
			Objects:      []agentruntime.DesiredObject{{LogicalName: "workload", Role: "worker", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: []byte(`{"metadata":{"name":"task"}}`)}}},
			Targets:      []executionplan.MutationTarget{{Name: "workload-pod-template", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Role: "worker", Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}},
			Units:        []executionplan.WorkloadUnit{{Name: "workload-pod-template", ObjectName: "workload", Kind: agentruntime.UnitPodTemplate, Role: "worker", Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}},
		},
		Environment: environment.ResolvedEnvironment{Bindings: []environment.ResolvedBinding{{
			Kind: environment.RequirementNetwork, RequirementName: "cross-domain", Profile: environment.ProfileReference{Type: "CrossDomain"},
			Consumers: []environment.ResolvedConsumer{{TargetName: "workload-pod-template", ObjectName: "workload", Role: "worker"}},
		}}},
	}
	binder := BindingPlanBinder{Handlers: registry, Workloads: WorkloadAssemblerFunc(func(context.Context, BindRequest, *executionplan.Plan) error { return nil }), Lowerer: noopBinderTestLowerer, Validator: PlanValidatorFunc(func(_ context.Context, _ BindRequest, plan *executionplan.Plan) error {
		if len(plan.Objects) != 1 || !json.Valid(plan.Objects[0].Manifest.Object) {
			return fault.New(fault.InvalidArgument, "invalid final plan")
		}
		return nil
	})}
	return binder, request
}

func TestBindingPlanBinderMutatesOnlyDeepCopy(t *testing.T) {
	binder, request := binderFixture(t)
	original := request.Plan.DeepCopy()
	executable, err := binder.Bind(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if executable.Digests.CompiledHash != "compiled" || len(executable.Objects) != 1 || !jsonEqual(executable.Objects[0].Manifest.Object, []byte(`{"metadata":{"name":"task"},"bound":"cross-domain"}`)) {
		t.Fatalf("executable=%#v", executable)
	}
	if len(executable.Objects[0].Units) != 1 || executable.Objects[0].Units[0].Kind != agentruntime.UnitPodTemplate {
		t.Fatalf("runtime units=%#v", executable.Objects[0].Units)
	}
	if !reflect.DeepEqual(request.Plan, original) {
		t.Fatal("Binder mutated its input Plan")
	}
	executable.Objects[0].Manifest.Object[0] = '['
	if !reflect.DeepEqual(request.Plan, original) {
		t.Fatal("executionplan.ExecutablePlan retained a mutable Plan alias")
	}
}

func TestBindingPlanBinderRejectsInvalidTargetsAndConsumersBeforeApply(t *testing.T) {
	tests := map[string]func(*BindRequest){
		"missing target object": func(request *BindRequest) { request.Plan.Targets[0].ObjectName = "missing" },
		"unknown consumer":      func(request *BindRequest) { request.Environment.Bindings[0].Consumers[0].TargetName = "missing" },
		"consumer mismatch": func(request *BindRequest) {
			request.Environment.Bindings[0].Consumers[0].Role = "head"
		},
		"duplicate binding": func(request *BindRequest) {
			request.Environment.Bindings = append(request.Environment.Bindings, request.Environment.Bindings[0])
		},
		"duplicate consumer": func(request *BindRequest) {
			request.Environment.Bindings[0].Consumers = append(request.Environment.Bindings[0].Consumers, request.Environment.Bindings[0].Consumers[0])
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			binder, request := binderFixture(t)
			mutate(&request)
			result, err := binder.Bind(context.Background(), request)
			if !fault.Is(err, fault.InvalidArgument) || !reflect.DeepEqual(result, executionplan.ExecutablePlan{}) {
				t.Fatalf("result=%#v error=%v", result, err)
			}
		})
	}
}

func TestBindingPlanBinderReturnsNoPartialPlanOnHandlerOrValidationFailure(t *testing.T) {
	t.Run("handler", func(t *testing.T) {
		binder, request := binderFixture(t)
		request.Environment.Bindings[0].Profile.Type = "missing"
		result, err := binder.Bind(context.Background(), request)
		if !fault.Is(err, fault.FailedPrecondition) || !reflect.DeepEqual(result, executionplan.ExecutablePlan{}) {
			t.Fatalf("result=%#v error=%v", result, err)
		}
	})

	t.Run("validator", func(t *testing.T) {
		binder, request := binderFixture(t)
		binder.Validator = PlanValidatorFunc(func(context.Context, BindRequest, *executionplan.Plan) error {
			return fault.New(fault.InvalidArgument, "invalid final plan")
		})
		result, err := binder.Bind(context.Background(), request)
		if !fault.Is(err, fault.InvalidArgument) || !reflect.DeepEqual(result, executionplan.ExecutablePlan{}) {
			t.Fatalf("result=%#v error=%v", result, err)
		}
		if string(request.Plan.Objects[0].Manifest.Object) != `{"metadata":{"name":"task"}}` {
			t.Fatal("validation failure mutated input Plan")
		}
	})
}

func TestBindingPlanBinderRunsRuntimeLoweringAfterEnvironmentHandlers(t *testing.T) {
	binder, request := binderFixture(t)
	binder.Lowerer = binderTestLowerer(func(_ context.Context, request agentruntime.UnitLoweringRequest) (agentruntime.DesiredObject, error) {
		var manifest map[string]any
		if err := json.Unmarshal(request.Object.Manifest.Object, &manifest); err != nil {
			return agentruntime.DesiredObject{}, err
		}
		if manifest["bound"] != "cross-domain" {
			return agentruntime.DesiredObject{}, errors.New("lowerer did not receive Environment Handler output")
		}
		manifest["lowered"] = true
		encoded, err := json.Marshal(manifest)
		if err != nil {
			return agentruntime.DesiredObject{}, err
		}
		request.Object.Manifest.Object = encoded
		return request.Object, nil
	})
	result, err := binder.Bind(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(result.Objects[0].Manifest.Object, []byte(`{"metadata":{"name":"task"},"lowered":true,"bound":"cross-domain"}`)) {
		t.Fatalf("manifest=%s", result.Objects[0].Manifest.Object)
	}
}

func jsonEqual(left, right []byte) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

func TestBindingPlanBinderRequiresDependenciesAndHonorsCancellation(t *testing.T) {
	binder, request := binderFixture(t)
	missing := BindingPlanBinder{}
	if _, err := missing.Bind(context.Background(), request); !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("missing dependencies error=%v", err)
	}
	binder.Lowerer = nil
	if _, err := binder.Bind(context.Background(), request); !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("missing lowerer error=%v", err)
	}
	binder.Validator = nil
	if _, err := binder.Bind(context.Background(), request); !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("missing validator error=%v", err)
	}

	binder, request = binderFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := binder.Bind(ctx, request)
	if !fault.Is(err, fault.DeadlineExceeded) || !reflect.DeepEqual(result, executionplan.ExecutablePlan{}) {
		t.Fatalf("result=%#v cancel error=%v", result, err)
	}
}

func TestBindingPlanBinderSupportsEmptyEnvironmentWithExplicitValidator(t *testing.T) {
	registry, err := environment.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	binder := BindingPlanBinder{
		Handlers:  registry,
		Lowerer:   noopBinderTestLowerer,
		Validator: PlanValidatorFunc(func(context.Context, BindRequest, *executionplan.Plan) error { return nil }),
		Workloads: WorkloadAssemblerFunc(func(context.Context, BindRequest, *executionplan.Plan) error { return nil }),
	}
	result, err := binder.Bind(context.Background(), BindRequest{Plan: executionplan.Plan{CompiledHash: "compiled"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Digests.CompiledHash != "compiled" {
		t.Fatalf("result=%#v", result)
	}
}

func TestBindingPlanBinderPreservesHandlerErrors(t *testing.T) {
	binder, request := binderFixture(t)
	sentinel := errors.New("sentinel")
	failing := &binderTestHandler{info: environment.HandlerInfo{ID: "network", Key: environment.HandlerKey{RuntimeKind: "Kubernetes", BindingKind: environment.RequirementNetwork, ProfileType: "CrossDomain"}, Algorithm: "network/v1"}, intentErr: fault.Wrap(fault.Unavailable, "collect intents", sentinel)}
	registry, err := environment.NewRegistry([]environment.HandlerRegistration{{Handler: failing}})
	if err != nil {
		t.Fatal(err)
	}
	binder.Handlers = registry
	_, err = binder.Bind(context.Background(), request)
	if !fault.Is(err, fault.Unavailable) || !errors.Is(err, sentinel) {
		t.Fatalf("error=%v", err)
	}
}
