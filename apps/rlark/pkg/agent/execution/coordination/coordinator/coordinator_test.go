package coordinator

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/observability"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

func TestCoordinatorEvaluateComputesStableDryRun(t *testing.T) {
	task := coordinatorTask()
	desiredConfig := desired("config", "v1", "ConfigMap", "config", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"},"data":{"v":"new"}}`)
	desiredWorkload := desired("workload", "apps/v1", "Deployment", "task", []string{"config"}, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"},"spec":{"replicas":1}}`)
	currentConfig := object("v1", "ConfigMap", "config", "config-uid", `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"},"data":{"v":"old"}}`)
	currentWorkload := object("apps/v1", "Deployment", "task", "workload-uid", string(desiredWorkload.Manifest.Object))
	stale := object("v1", "Service", "stale", "stale-uid", `{"apiVersion":"v1","kind":"Service","metadata":{"namespace":"runtime","name":"stale"}}`)
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Revision: "objects", Objects: []agentruntime.Object{currentConfig, currentWorkload, stale}}, instances: agentruntime.InstanceSnapshot{Revision: "instances"}}
	adapter := &fakeAdapter{plan: executionplan.Plan{AdapterID: "Deployment", AdapterVersion: "v1", CompiledHash: "compiled", Objects: []agentruntime.DesiredObject{desiredWorkload, desiredConfig}, Discovery: executionplan.InstanceDiscovery{Roots: []string{"workload"}}}, observation: workload.Observation{Phase: "Running"}}
	coordinator := testCoordinator(provider, adapter)
	first, err := coordinator.Evaluate(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	second, err := coordinator.Evaluate(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if provider.applies != 0 || provider.deletes != 0 {
		t.Fatal("Evaluate mutated the Runtime")
	}
	if !reflect.DeepEqual(first.Operations, second.Operations) || first.Plan.Digests.ExecutableHash != second.Plan.Digests.ExecutableHash {
		t.Fatal("same facts produced an unstable decision")
	}
	if len(first.Operations) != 1 || first.Operations[0].Kind != OperationUpdate || first.Operations[0].LogicalName != "config" {
		t.Fatalf("operations=%#v", first.Operations)
	}
	if first.Operations[0].IdempotencyKey == "" {
		t.Fatal("missing idempotency key")
	}
	if len(first.Orphans) != 1 || first.Orphans[0].UID != "stale-uid" {
		t.Fatalf("orphans=%#v", first.Orphans)
	}
	if got := []string{first.Plan.Objects[0].LogicalName, first.Plan.Objects[1].LogicalName}; !reflect.DeepEqual(got, []string{"config", "workload"}) {
		t.Fatalf("dependency order=%v", got)
	}
	if len(provider.lastQuery.Workloads) != 1 || provider.lastQuery.Workloads[0].UID != "workload-uid" {
		t.Fatalf("instance roots=%#v", provider.lastQuery.Workloads)
	}
	if first.Observation.Phase != "Running" {
		t.Fatalf("observation=%#v", first.Observation)
	}
}

func TestCoordinatorEvaluateCreatesInDependencyOrder(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
	adapter := &fakeAdapter{plan: executionplan.Plan{CompiledHash: "compiled", Objects: []agentruntime.DesiredObject{desired("workload", "apps/v1", "Deployment", "task", []string{"config"}, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`), desired("config", "v1", "ConfigMap", "config", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"}}`)}}}
	result, err := testCoordinator(provider, adapter).Evaluate(context.Background(), coordinatorTask())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 2 || result.Operations[0].LogicalName != "config" || result.Operations[1].LogicalName != "workload" {
		t.Fatalf("operations=%#v", result.Operations)
	}
}

func TestCoordinatorEvaluateReplaceDeletesBeforeRecreate(t *testing.T) {
	desiredService := desired("service", "v1", "Service", "task-head", nil, `{"apiVersion":"v1","kind":"Service","metadata":{"namespace":"runtime","name":"task-head"},"spec":{"selector":{"role":"new"}}}`)
	desiredService.UpdateMode = agentruntime.UpdateReplace
	currentService := object("v1", "Service", "task-head", "service-uid", `{"apiVersion":"v1","kind":"Service","metadata":{"namespace":"runtime","name":"task-head"},"spec":{"selector":{"role":"old"}}}`)
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{currentService}}}
	adapter := &fakeAdapter{plan: executionplan.Plan{CompiledHash: "compiled", Objects: []agentruntime.DesiredObject{desiredService}}}
	coordinator := testCoordinator(provider, adapter)

	result, err := coordinator.Evaluate(context.Background(), coordinatorTask())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || result.Operations[0].Kind != OperationDelete || result.Operations[0].Current == nil || result.Operations[0].Current.UID != "service-uid" || result.Operations[0].Desired != nil {
		t.Fatalf("replace operations=%#v", result.Operations)
	}

	provider.managed.Objects = nil
	result, err = coordinator.Evaluate(context.Background(), coordinatorTask())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 1 || result.Operations[0].Kind != OperationCreate || result.Operations[0].Desired == nil || result.Operations[0].Current != nil {
		t.Fatalf("recreate operations=%#v", result.Operations)
	}
}

func TestCoordinatorRejectsIncarnationAdoptionWithoutBothCapabilities(t *testing.T) {
	adopt := agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid", IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}
	inventory := &workload.Inventory{Candidates: []agentruntime.ObjectCandidate{{Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Namespace: "runtime", Name: "task", Ownership: adopt, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup}}}
	tests := []struct {
		name                string
		adapterCapabilities []agentruntime.Capability
		runtimeCapabilities []agentruntime.Capability
	}{
		{name: "adapter missing", runtimeCapabilities: []agentruntime.Capability{agentruntime.CapabilityIncarnationAdoption}},
		{name: "runtime missing", adapterCapabilities: []agentruntime.Capability{agentruntime.CapabilityIncarnationAdoption}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes", Capabilities: test.runtimeCapabilities}}
			adapter := &fakeAdapter{inventory: inventory, capabilities: test.adapterCapabilities}
			_, err := testCoordinator(provider, adapter).Evaluate(context.Background(), coordinatorTask())
			if !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("error=%v, want capability failure", err)
			}
		})
	}
}

func TestCoordinatorRejectsReconcileProfileWithoutBothCapabilities(t *testing.T) {
	object := desired("workload", "apps/v1", "Deployment", "task", nil, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	tests := []struct {
		name                string
		adapterCapabilities []agentruntime.Capability
		runtimeCapabilities []agentruntime.Capability
	}{
		{name: "adapter missing", adapterCapabilities: []agentruntime.Capability{}, runtimeCapabilities: []agentruntime.Capability{agentruntime.CapabilityReconcileProfiles}},
		{name: "runtime missing", adapterCapabilities: []agentruntime.Capability{agentruntime.CapabilityReconcileProfiles}, runtimeCapabilities: []agentruntime.Capability{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes", Capabilities: test.runtimeCapabilities}}
			adapter := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{object}}, capabilities: test.adapterCapabilities}
			_, err := testCoordinator(provider, adapter).Evaluate(context.Background(), coordinatorTask())
			if !fault.Is(err, fault.FailedPrecondition) {
				t.Fatalf("error=%v, want capability failure", err)
			}
		})
	}
}

func TestCoordinatorUsesRuntimeObjectComparator(t *testing.T) {
	desiredWorkload := desired("workload", "apps/v1", "Deployment", "task", nil, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{object("apps/v1", "Deployment", "task", "uid", `{"different":true}`)}}}
	coordinator := testCoordinator(provider, &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{desiredWorkload}}})
	called := false
	coordinator.Runtimes[0].Comparator = ObjectComparatorFunc(func(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
		called = true
		return true, nil
	})
	result, err := coordinator.Evaluate(context.Background(), coordinatorTask())
	if err != nil {
		t.Fatal(err)
	}
	if !called || len(result.Operations) != 0 {
		t.Fatalf("called=%v operations=%#v", called, result.Operations)
	}
}

func TestCoordinatorReconcileExecutesDependencyOrderAndRefreshes(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
	adapter := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{
		desired("workload", "apps/v1", "Deployment", "task", []string{"config"}, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`),
		desired("config", "v1", "ConfigMap", "config", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"}}`),
	}}}
	result, err := testCoordinator(provider, adapter).Reconcile(context.Background(), coordinatorTask())
	if err != nil {
		t.Fatal(err)
	}
	if provider.applies != 2 || provider.deletes != 0 || provider.listManaged != 2 {
		t.Fatalf("result=%#v provider=%#v", result, provider)
	}
	if got := []string{provider.applyRequests[0].Desired.LogicalName, provider.applyRequests[1].Desired.LogicalName}; !reflect.DeepEqual(got, []string{"config", "workload"}) {
		t.Fatalf("apply order=%v", got)
	}
	if provider.applyRequests[0].IdempotencyKey == "" || provider.applyRequests[1].IdempotencyKey == "" {
		t.Fatal("missing stable idempotency key")
	}
}

func TestCoordinatorRecordsBoundedEvaluationAndEffects(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
	adapter := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{
		desired("workload", "apps/v1", "Deployment", "task", nil, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`),
	}}}
	recorder := &observability.RecordingObserver{}
	observer := observability.Safe(recorder, observability.StaticConfigSource{Config: observability.Config{Payloads: map[observability.Payload]bool{observability.PayloadPlan: true, observability.PayloadOperations: true}}}, nil, nil)
	coordinator := testCoordinator(provider, adapter)
	coordinator.Observer = observer

	result, err := coordinator.Reconcile(context.Background(), coordinatorTask())
	if err != nil || len(result.Applied) != 1 {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	records := recorder.Records()
	if len(records) != 3 { // reconcile plus initial and fresh evaluation
		t.Fatalf("records=%#v", records)
	}
	var applyObserved, planObserved, operationsObserved bool
	for _, record := range records {
		if record.Result == nil || record.Result.Outcome != "Succeeded" {
			t.Fatalf("record=%#v", record)
		}
		for _, event := range record.Events {
			switch event.Name {
			case "provider.apply":
				applyObserved = true
			case "adapter.compile.plan":
				planObserved = len(event.Payload) != 0
			case "coordinator.diff.operations":
				operationsObserved = len(event.Payload) != 0
			}
		}
	}
	if !applyObserved || !planObserved || !operationsObserved {
		t.Fatalf("apply=%v plan=%v operations=%v records=%#v", applyObserved, planObserved, operationsObserved, records)
	}
}

func TestCoordinatorRecordsTypedPartialFailureWithoutChangingEffects(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, applyErrAt: 2, applyErr: fault.New(fault.Unavailable, "offline")}
	adapter := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{
		desired("workload", "apps/v1", "Deployment", "task", []string{"config"}, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`),
		desired("config", "v1", "ConfigMap", "config", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"}}`),
	}}}
	recorder := &observability.RecordingObserver{}
	coordinator := testCoordinator(provider, adapter)
	coordinator.Observer = observability.Safe(recorder, nil, nil, nil)

	result, err := coordinator.Reconcile(context.Background(), coordinatorTask())
	if !fault.Is(err, fault.Unavailable) || len(result.Applied) != 1 || provider.applies != 2 {
		t.Fatalf("result=%#v error=%v provider=%#v", result, err, provider)
	}
	records := recorder.Records()
	if len(records) != 2 || records[0].Result == nil || records[0].Result.FaultCode != string(fault.Unavailable) {
		t.Fatalf("records=%#v", records)
	}
}

func TestCoordinatorExecutableHashIgnoresAdapterEmissionOrder(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
	config := desired("config", "v1", "ConfigMap", "config", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"}}`)
	workloadObject := desired("workload", "apps/v1", "Deployment", "task", []string{"config"}, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	first, err := testCoordinator(provider, &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{workloadObject, config}}}).Evaluate(context.Background(), coordinatorTask())
	if err != nil {
		t.Fatal(err)
	}
	second, err := testCoordinator(provider, &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{config, workloadObject}}}).Evaluate(context.Background(), coordinatorTask())
	if err != nil {
		t.Fatal(err)
	}
	if first.Plan.Digests.ExecutableHash != second.Plan.Digests.ExecutableHash {
		t.Fatalf("hashes differ: %s != %s", first.Plan.Digests.ExecutableHash, second.Plan.Digests.ExecutableHash)
	}
}

func TestCoordinatorEvaluateRejectsInvalidDependencyGraph(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
	adapter := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{desired("a", "v1", "ConfigMap", "a", []string{"b"}, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"a"}}`), desired("b", "v1", "ConfigMap", "b", []string{"a"}, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"b"}}`)}}}
	_, err := testCoordinator(provider, adapter).Evaluate(context.Background(), coordinatorTask())
	if !fault.Is(err, fault.InvalidArgument) || provider.listManaged != 0 {
		t.Fatalf("error=%v listManaged=%d", err, provider.listManaged)
	}
}

func TestCoordinatorEvaluateStopsOnSnapshotFailure(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, listErr: errors.New("offline")}
	adapter := &fakeAdapter{plan: executionplan.Plan{}}
	_, err := testCoordinator(provider, adapter).Evaluate(context.Background(), coordinatorTask())
	if err == nil || provider.listInstances != 0 || provider.applies != 0 || provider.deletes != 0 {
		t.Fatalf("error=%v provider=%#v", err, provider)
	}
}

func TestCoordinatorEvaluateDeletingUsesReverseLifecycleOrder(t *testing.T) {
	task := coordinatorTask()
	task.Deleting = true
	config := desired("config", "v1", "ConfigMap", "config", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"}}`)
	workloadObject := desired("workload", "apps/v1", "Deployment", "task", []string{"config"}, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	retained := desired("retained", "v1", "PersistentVolumeClaim", "data", nil, `{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"namespace":"runtime","name":"data"}}`)
	retained.Lifecycle = agentruntime.LifecycleRetain
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{object("v1", "ConfigMap", "config", "config-uid", string(config.Manifest.Object)), object("apps/v1", "Deployment", "task", "workload-uid", string(workloadObject.Manifest.Object)), object("v1", "PersistentVolumeClaim", "data", "pvc-uid", string(retained.Manifest.Object))}}}
	adapter := &fakeAdapter{plan: executionplan.Plan{CompiledHash: "compiled", Objects: []agentruntime.DesiredObject{workloadObject, config, retained}}}
	result, err := testCoordinator(provider, adapter).Evaluate(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Operations) != 2 || result.Operations[0].LogicalName != "workload" || result.Operations[1].LogicalName != "config" {
		t.Fatalf("cleanup operations=%#v", result.Operations)
	}
	for _, operation := range result.Operations {
		if operation.Kind != OperationDelete {
			t.Fatalf("deleting Task proposed %s", operation.Kind)
		}
	}
	if provider.listInstances != 1 || len(provider.lastQuery.Workloads) != 0 {
		t.Fatalf("deleting Task queried workload instances: %#v", provider.lastQuery)
	}
	if result.CleanupComplete {
		t.Fatal("cleanup reported complete while deletable objects remain")
	}
}

func TestCoordinatorDeletingReportsCompleteWithOnlyRetainedResources(t *testing.T) {
	task := coordinatorTask()
	task.Deleting = true
	retained := desired("retained", "v1", "ConfigMap", "shared", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"shared"}}`)
	retained.Lifecycle = agentruntime.LifecycleRetain
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{object("v1", "ConfigMap", "shared", "shared-uid", string(retained.Manifest.Object))}}}
	result, err := testCoordinator(provider, &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{retained}}}).Evaluate(context.Background(), task)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CleanupComplete || len(result.Operations) != 0 {
		t.Fatalf("result=%#v", result)
	}
}

func TestCoordinatorObserveRootSkipsCompileAndManagedScan(t *testing.T) {
	task := coordinatorTask()
	root := object("apps/v1", "Deployment", "task", "workload-uid", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task","uid":"workload-uid"}}`)
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, getObject: root, instances: agentruntime.InstanceSnapshot{Revision: "pods"}}
	adapter := &fakeAdapter{observation: workload.Observation{Phase: "Running"}}
	result, err := testCoordinator(provider, adapter).ObserveRoot(context.Background(), task, root.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.compiles != 0 || adapter.observes != 1 || provider.gets != 1 || provider.listInstances != 1 || provider.listManaged != 0 || provider.applies != 0 || provider.deletes != 0 {
		t.Fatalf("result=%#v provider=%#v adapter=%#v", result, provider, adapter)
	}
}

func TestCoordinatorCleanupDoesNotCompileCurrentSpec(t *testing.T) {
	workloadObject := object("apps/v1", "Deployment", "task", "workload-uid", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	retained := object("v1", "PersistentVolumeClaim", "data", "pvc-uid", `{"apiVersion":"v1","kind":"PersistentVolumeClaim","metadata":{"namespace":"runtime","name":"data"}}`)
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{workloadObject, retained}}}
	adapter := &fakeAdapter{}
	coordinator := testCoordinator(provider, adapter)
	result, err := coordinator.CleanupTask(context.Background(), coordinatorTask().Identity, coordinatorTask().RuntimeSelector, coordinatorTask().WorkloadSelector, coordinatorTask().CarrierSelector)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.compiles != 0 || provider.deletes != 1 || len(result.Deleted) != 1 || result.Deleted[0].UID != "workload-uid" {
		t.Fatalf("result=%#v provider=%#v adapter=%#v", result, provider, adapter)
	}
}

func TestCoordinatorCleanupUsesInventoryLifecycleAndReverseOrder(t *testing.T) {
	secret := object("v1", "Secret", "task-ssh", "secret-uid", `{"apiVersion":"v1","kind":"Secret","metadata":{"namespace":"runtime","name":"task-ssh"}}`)
	workloadObject := object("apps/v1", "Deployment", "task", "workload-uid", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	retained := object("v1", "ConfigMap", "shared", "config-uid", `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"shared"}}`)
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{secret, workloadObject, retained}}}
	inventory := workload.Inventory{Candidates: []agentruntime.ObjectCandidate{
		{Kind: secret.Manifest.Type, Namespace: "runtime", Name: "task-ssh", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
		{Kind: workloadObject.Manifest.Type, Namespace: "runtime", Name: "task", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup},
		{Kind: retained.Manifest.Type, Namespace: "runtime", Name: "shared", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentitySharedResource, OwnerKey: "shared"}, Lifecycle: agentruntime.LifecycleRetain},
	}}

	result, err := testCoordinator(provider, &fakeAdapter{inventory: &inventory}).CleanupTask(context.Background(), coordinatorTask().Identity, coordinatorTask().RuntimeSelector, coordinatorTask().WorkloadSelector, coordinatorTask().CarrierSelector)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].UID != "workload-uid" || provider.deleteRequests[0].Ref.UID != "workload-uid" {
		t.Fatalf("first cleanup=%#v requests=%#v", result, provider.deleteRequests)
	}
	provider.managed.Objects = []agentruntime.Object{secret, retained}
	result, err = testCoordinator(provider, &fakeAdapter{inventory: &inventory}).CleanupTask(context.Background(), coordinatorTask().Identity, coordinatorTask().RuntimeSelector, coordinatorTask().WorkloadSelector, coordinatorTask().CarrierSelector)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Deleted) != 1 || result.Deleted[0].UID != "secret-uid" || provider.deleteRequests[1].Ref.UID != "secret-uid" {
		t.Fatalf("second cleanup=%#v requests=%#v", result, provider.deleteRequests)
	}
}

func TestCoordinatorCleanupRejectsDeleteLifecycleOnLabelCandidate(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
	inventory := workload.Inventory{Candidates: []agentruntime.ObjectCandidate{{Kind: agentruntime.ObjectKind{Version: "v1", Kind: "Secret"}, Namespace: "runtime", Labels: map[string]string{"task": "task"}, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup}}}
	_, err := testCoordinator(provider, &fakeAdapter{inventory: &inventory}).CleanupTask(context.Background(), coordinatorTask().Identity, coordinatorTask().RuntimeSelector, coordinatorTask().WorkloadSelector, coordinatorTask().CarrierSelector)
	if !fault.Is(err, fault.InvalidArgument) || provider.deletes != 0 {
		t.Fatalf("error=%v deletes=%d", err, provider.deletes)
	}
}

func TestCoordinatorReconcileStopsAfterPartialApplyFailureAndRecoversFromObservedState(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, applyErrAt: 2, applyErr: fault.New(fault.Unavailable, "runtime unavailable")}
	adapter := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{
		desired("workload", "apps/v1", "Deployment", "task", []string{"config"}, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`),
		desired("config", "v1", "ConfigMap", "config", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"}}`),
	}}}
	coordinator := testCoordinator(provider, adapter)

	result, err := coordinator.Reconcile(context.Background(), coordinatorTask())
	if !fault.Is(err, fault.Unavailable) {
		t.Fatalf("Reconcile() error = %v, want unavailable", err)
	}
	if provider.applies != 2 || len(result.Applied) != 1 || provider.listManaged != 1 {
		t.Fatalf("partial result=%#v provider=%#v", result, provider)
	}
	firstKey := provider.applyRequests[0].IdempotencyKey
	provider.applyErrAt, provider.applyErr = 0, nil
	provider.managed.Objects = []agentruntime.Object{object("v1", "ConfigMap", "config", "config-uid", string(adapter.plan.Objects[1].Manifest.Object))}

	result, err = coordinator.Reconcile(context.Background(), coordinatorTask())
	if err != nil {
		t.Fatal(err)
	}
	if provider.applies != 3 || len(result.Applied) != 1 || provider.applyRequests[2].Desired.LogicalName != "workload" {
		t.Fatalf("recovery replayed completed effects: result=%#v requests=%#v", result, provider.applyRequests)
	}
	if firstKey == "" || provider.applyRequests[0].IdempotencyKey != firstKey {
		t.Fatal("idempotency key changed during failed attempt")
	}
}

func TestCoordinatorReconcileReturnsPartialResultWhenFreshObservationFails(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, listErrAt: 2, listErr: fault.New(fault.Unavailable, "refresh failed")}
	adapter := &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{desired("workload", "apps/v1", "Deployment", "task", nil, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)}}}

	result, err := testCoordinator(provider, adapter).Reconcile(context.Background(), coordinatorTask())
	if !fault.Is(err, fault.Unavailable) {
		t.Fatalf("Reconcile() error = %v, want unavailable", err)
	}
	if provider.applies != 1 || len(result.Applied) != 1 || provider.listManaged != 2 {
		t.Fatalf("successful external effect was not retained in partial result: %#v provider=%#v", result, provider)
	}
}

func TestCoordinatorReconcileWithoutOperationsDoesNotRefresh(t *testing.T) {
	desiredWorkload := desired("workload", "apps/v1", "Deployment", "task", nil, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{object("apps/v1", "Deployment", "task", "uid", string(desiredWorkload.Manifest.Object))}}}
	result, err := testCoordinator(provider, &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{desiredWorkload}}}).Reconcile(context.Background(), coordinatorTask())
	if err != nil {
		t.Fatal(err)
	}
	if provider.listManaged != 1 || provider.applies != 0 || provider.deletes != 0 || len(result.Applied) != 0 {
		t.Fatalf("no-op reconcile performed extra work: result=%#v provider=%#v", result, provider)
	}
}

func TestCoordinatorCleanupWaitsForConfirmedAbsence(t *testing.T) {
	workloadObject := object("apps/v1", "Deployment", "task", "workload-uid", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managedSequence: []agentruntime.ObjectSnapshot{
		{Objects: []agentruntime.Object{workloadObject}},
		{Objects: []agentruntime.Object{workloadObject}},
		{Objects: []agentruntime.Object{workloadObject}},
		{},
	}}
	coordinator := testCoordinator(provider, &fakeAdapter{})

	first, err := coordinator.CleanupTask(context.Background(), coordinatorTask().Identity, coordinatorTask().RuntimeSelector, coordinatorTask().WorkloadSelector, coordinatorTask().CarrierSelector)
	if err != nil {
		t.Fatal(err)
	}
	if first.Complete || len(first.Deleted) != 1 {
		t.Fatalf("cleanup completed before absence was observed: %#v", first)
	}
	second, err := coordinator.CleanupTask(context.Background(), coordinatorTask().Identity, coordinatorTask().RuntimeSelector, coordinatorTask().WorkloadSelector, coordinatorTask().CarrierSelector)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Complete || len(second.Deleted) != 1 || provider.deletes != 2 {
		t.Fatalf("cleanup did not converge after absence: %#v provider=%#v", second, provider)
	}
	if provider.deleteRequests[0].IdempotencyKey == "" || provider.deleteRequests[0].IdempotencyKey != provider.deleteRequests[1].IdempotencyKey {
		t.Fatalf("cleanup retry key is not stable: %#v", provider.deleteRequests)
	}
}

func TestCoordinatorCleanupRecoversAfterReconstructionFromRuntimeState(t *testing.T) {
	workloadObject := object("apps/v1", "Deployment", "task", "workload-uid", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{workloadObject}}}

	firstCoordinator := testCoordinator(provider, &fakeAdapter{})
	first, err := firstCoordinator.CleanupTask(context.Background(), coordinatorTask().Identity, coordinatorTask().RuntimeSelector, coordinatorTask().WorkloadSelector, coordinatorTask().CarrierSelector)
	if err != nil {
		t.Fatal(err)
	}
	if first.Complete || len(first.Deleted) != 1 {
		t.Fatalf("first cleanup=%#v", first)
	}

	// Reconstruct the Coordinator without carrying any result from the first
	// attempt. The Runtime's observed state is the only recovery checkpoint.
	provider.managed = agentruntime.ObjectSnapshot{}
	restartedCoordinator := testCoordinator(provider, &fakeAdapter{})
	second, err := restartedCoordinator.CleanupTask(context.Background(), coordinatorTask().Identity, coordinatorTask().RuntimeSelector, coordinatorTask().WorkloadSelector, coordinatorTask().CarrierSelector)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Complete || len(second.Deleted) != 0 || provider.deletes != 1 {
		t.Fatalf("cleanup after reconstruction=%#v provider=%#v", second, provider)
	}
}

func TestCoordinatorCleanupFailureNeverReportsComplete(t *testing.T) {
	workloadObject := object("apps/v1", "Deployment", "task", "workload-uid", `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	tests := []struct {
		name     string
		provider *fakeProvider
	}{
		{name: "delete", provider: &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{workloadObject}}, deleteErrAt: 1, deleteErr: fault.New(fault.Unavailable, "delete failed")}},
		{name: "confirmation", provider: &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{workloadObject}}, listErrAt: 2, listErr: fault.New(fault.Unavailable, "confirmation failed")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coordinator := testCoordinator(test.provider, &fakeAdapter{})
			result, err := coordinator.CleanupTask(context.Background(), coordinatorTask().Identity, coordinatorTask().RuntimeSelector, coordinatorTask().WorkloadSelector, coordinatorTask().CarrierSelector)
			if !fault.Is(err, fault.Unavailable) || result.Complete {
				t.Fatalf("CleanupTask() = %#v, %v", result, err)
			}
		})
	}
}

func TestCoordinatorReconcileStopsAfterPartialDeleteFailure(t *testing.T) {
	task := coordinatorTask()
	task.Deleting = true
	config := desired("config", "v1", "ConfigMap", "config", nil, `{"apiVersion":"v1","kind":"ConfigMap","metadata":{"namespace":"runtime","name":"config"}}`)
	workloadObject := desired("workload", "apps/v1", "Deployment", "task", []string{"config"}, `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"namespace":"runtime","name":"task"}}`)
	provider := &fakeProvider{
		info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"},
		managed: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{
			object("v1", "ConfigMap", "config", "config-uid", string(config.Manifest.Object)),
			object("apps/v1", "Deployment", "task", "workload-uid", string(workloadObject.Manifest.Object)),
		}},
		deleteErrAt: 2,
		deleteErr:   fault.New(fault.Unavailable, "delete failed"),
	}
	result, err := testCoordinator(provider, &fakeAdapter{plan: executionplan.Plan{Objects: []agentruntime.DesiredObject{workloadObject, config}}}).Reconcile(context.Background(), task)
	if !fault.Is(err, fault.Unavailable) {
		t.Fatalf("Reconcile() error = %v, want unavailable", err)
	}
	if provider.deletes != 2 || len(result.Deleted) != 1 || result.Deleted[0].UID != "workload-uid" {
		t.Fatalf("partial delete result=%#v requests=%#v", result, provider.deleteRequests)
	}
	if provider.deleteRequests[0].Ref.UID != "workload-uid" || provider.deleteRequests[1].Ref.UID != "config-uid" {
		t.Fatalf("delete order=%#v", provider.deleteRequests)
	}
}

func TestCoordinatorPropagatesCancellationWithoutRuntimeWrites(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, listErr: ctx.Err()}
	_, err := testCoordinator(provider, &fakeAdapter{plan: executionplan.Plan{}}).Reconcile(ctx, coordinatorTask())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Reconcile() error = %v, want context canceled", err)
	}
	if provider.applies != 0 || provider.deletes != 0 || provider.listInstances != 0 {
		t.Fatalf("canceled reconcile continued to external effects: %#v", provider)
	}
}

func TestCoordinatorRejectsIncompleteDependenciesBeforeExternalReads(t *testing.T) {
	tests := []struct {
		name        string
		coordinator *Coordinator
		wantCode    fault.Code
	}{
		{name: "missing runtime", coordinator: &Coordinator{}, wantCode: fault.NotFound},
		{name: "nil provider", coordinator: &Coordinator{Runtimes: []RuntimeRegistration{{Info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}}}, wantCode: fault.FailedPrecondition},
		{name: "missing workload", coordinator: &Coordinator{Runtimes: []RuntimeRegistration{{Info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, Provider: &fakeProvider{}}}, Resolver: passResolver{}, Binder: passBinder{}}, wantCode: fault.NotFound},
		{name: "nil adapter", coordinator: &Coordinator{Runtimes: []RuntimeRegistration{{Info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, Provider: &fakeProvider{}}}, Workloads: []WorkloadRegistration{{Type: "Deployment", Version: "v1", RuntimeKind: "Kubernetes"}}, Resolver: passResolver{}, Binder: passBinder{}}, wantCode: fault.FailedPrecondition},
		{name: "missing resolver", coordinator: &Coordinator{Runtimes: []RuntimeRegistration{{Info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}, Provider: &fakeProvider{}}}, Workloads: []WorkloadRegistration{{Type: "Deployment", Version: "v1", RuntimeKind: "Kubernetes", Adapter: &fakeAdapter{}}}, Binder: passBinder{}}, wantCode: fault.FailedPrecondition},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.coordinator.Evaluate(context.Background(), coordinatorTask())
			if !fault.Is(err, test.wantCode) {
				t.Fatalf("Evaluate() error = %v, want %s", err, test.wantCode)
			}
		})
	}
}

func TestCoordinatorSharedRegistryMissingImplementationIsNotFound(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
	coordinator := testCoordinator(provider, &fakeAdapter{})
	coordinator.Implementations = missingImplementationRegistry{}

	_, err := coordinator.Evaluate(context.Background(), coordinatorTask())
	if !fault.Is(err, fault.NotFound) {
		t.Fatalf("Evaluate() error = %v, want NotFound", err)
	}
}

func TestCoordinatorPassesMutationTargetsToResolverBeforeProviderReads(t *testing.T) {
	provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
	kind := agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	target := executionplan.MutationTarget{Name: "workload-pod-template", ObjectName: "workload", Kind: executionplan.TargetPodTemplate, Locator: agentruntime.ObjectLocator{Type: kind, Path: "spec.template"}}
	adapter := &fakeAdapter{plan: executionplan.Plan{Targets: []executionplan.MutationTarget{target}}}
	resolver := &captureResolver{err: fault.New(fault.FailedPrecondition, "profile is unavailable")}
	coordinator := testCoordinator(provider, adapter)
	coordinator.Resolver = resolver

	_, err := coordinator.Evaluate(context.Background(), coordinatorTask())
	if !fault.Is(err, fault.FailedPrecondition) {
		t.Fatalf("Evaluate() error = %v, want FailedPrecondition", err)
	}
	if len(resolver.request.Targets) != 1 || resolver.request.Targets[0] != target {
		t.Fatalf("resolver targets=%#v, want %#v", resolver.request.Targets, []executionplan.MutationTarget{target})
	}
	if provider.listManaged != 0 || provider.listInstances != 0 || provider.applies != 0 || provider.deletes != 0 {
		t.Fatalf("provider was used after environment resolution failed: %#v", provider)
	}
}

func testCoordinator(provider *fakeProvider, adapter *fakeAdapter) *Coordinator {
	if provider.info.Capabilities == nil {
		provider.info.Capabilities = []agentruntime.Capability{agentruntime.CapabilityReconcileProfiles}
	}
	if adapter.capabilities == nil {
		adapter.capabilities = []agentruntime.Capability{agentruntime.CapabilityReconcileProfiles}
	}
	return &Coordinator{Runtimes: []RuntimeRegistration{{Info: provider.info, Provider: provider}}, Workloads: []WorkloadRegistration{{Type: "Deployment", Version: "v1", RuntimeKind: provider.info.Kind, Adapter: adapter}}, Resolver: passResolver{}, Binder: passBinder{}}
}

func TestCoordinatorRejectsBinderOwnedFinalDigests(t *testing.T) {
	for name, digests := range map[string]executionplan.Digests{
		"executable": {ExecutableHash: "sha256:unexpected"},
		"binding":    {BindingRevision: "sha256:unexpected"},
	} {
		t.Run(name, func(t *testing.T) {
			provider := &fakeProvider{info: agentruntime.Info{ID: "runtime", Kind: "Kubernetes"}}
			coordinator := testCoordinator(provider, &fakeAdapter{plan: executionplan.Plan{}})
			coordinator.Binder = digestBinder{digests: digests}
			if _, err := coordinator.Evaluate(context.Background(), coordinatorTask()); !fault.Is(err, fault.Internal) {
				t.Fatalf("Evaluate() error=%v, want Internal", err)
			}
			if provider.listManaged != 0 || provider.listInstances != 0 {
				t.Fatalf("provider read after invalid Binder digest: %#v", provider)
			}
		})
	}
}

type digestBinder struct{ digests executionplan.Digests }

func (b digestBinder) Bind(context.Context, binding.BindRequest) (executionplan.ExecutablePlan, error) {
	return executionplan.ExecutablePlan{Digests: b.digests}, nil
}

func coordinatorTask() coordination.TaskSnapshot {
	return coordination.TaskSnapshot{Identity: agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: "management", Name: "task"}, ObjectUID: "task-uid", Generation: 3}, RuntimeSelector: coordination.RuntimeSelector{ID: "runtime", Kind: "Kubernetes"}, WorkloadSelector: coordination.WorkloadSelector{Type: "Deployment", Version: "v1"}}
}
func desired(logical, apiVersion, kind, name string, dependencies []string, manifest string) agentruntime.DesiredObject {
	group, version := splitVersion(apiVersion)
	objectKind := agentruntime.ObjectKind{Group: group, Version: version, Kind: kind}
	return agentruntime.DesiredObject{LogicalName: logical, Kind: objectKind, Manifest: agentruntime.TypedManifest{Type: objectKind, Object: []byte(manifest)}, DependsOn: dependencies, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace, ReconcileProfile: agentruntime.ObjectKind{Group: "test.execution.rlark.io", Version: "v1", Kind: "Projection"}}
}
func object(apiVersion, kind, name, uid, manifest string) agentruntime.Object {
	group, version := splitVersion(apiVersion)
	objectKind := agentruntime.ObjectKind{Group: group, Version: version, Kind: kind}
	return agentruntime.Object{Ref: agentruntime.ObjectRef{RuntimeID: "runtime", ID: kind, Namespace: "runtime", Name: name, UID: uid, LogicalTask: coordinatorTask().Identity.Logical}, Manifest: agentruntime.TypedManifest{Type: objectKind, Object: []byte(manifest)}}
}
func splitVersion(value string) (string, string) {
	for i := range value {
		if value[i] == '/' {
			return value[:i], value[i+1:]
		}
	}
	return "", value
}

type passResolver struct{}

type captureResolver struct {
	request environment.ResolveRequest
	err     error
}

type missingImplementationRegistry struct{}

func (missingImplementationRegistry) SelectImplementation(workload.TypeID, string, agentruntime.Kind) (workload.Adapter, error) {
	return nil, fmt.Errorf("exact implementation missing: %w", workload.ErrAdapterNotFound)
}

func (passResolver) Resolve(_ context.Context, request environment.ResolveRequest) (environment.ResolvedEnvironment, error) {
	return environment.ResolvedEnvironment{Task: request.Task}, nil
}

func (r *captureResolver) Resolve(_ context.Context, request environment.ResolveRequest) (environment.ResolvedEnvironment, error) {
	r.request = request
	return environment.ResolvedEnvironment{}, r.err
}

type passBinder struct{}

func (passBinder) Bind(_ context.Context, request binding.BindRequest) (executionplan.ExecutablePlan, error) {
	return executionplan.ExecutablePlan{Digests: executionplan.Digests{CompiledHash: request.Plan.CompiledHash}, Objects: request.Plan.Objects, Discovery: request.Plan.Discovery, Update: request.Plan.Update}, nil
}

type fakeAdapter struct {
	plan         executionplan.Plan
	observation  workload.Observation
	inventory    *workload.Inventory
	capabilities []agentruntime.Capability
	compiles     int
	observes     int
}

func (a *fakeAdapter) Inventory(context.Context, workload.InventoryRequest) (workload.Inventory, error) {
	if a.inventory != nil {
		return *a.inventory, nil
	}
	return workload.Inventory{Candidates: []agentruntime.ObjectCandidate{{Kind: agentruntime.ObjectKind{Group: "apps", Version: "v1", Kind: "Deployment"}, Namespace: "runtime", Name: "task", Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: "task-uid"}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup}}}, nil
}

func (a *fakeAdapter) Info(context.Context) (workload.TypeInfo, error) {
	return workload.TypeInfo{Capabilities: a.capabilities}, nil
}
func (a *fakeAdapter) Compile(context.Context, workload.CompileRequest) (executionplan.Plan, error) {
	a.compiles++
	return a.plan, nil
}
func (a *fakeAdapter) Observe(context.Context, workload.ObserveRequest) (workload.Observation, error) {
	a.observes++
	return a.observation, nil
}

type fakeProvider struct {
	info                                         agentruntime.Info
	managed                                      agentruntime.ObjectSnapshot
	instances                                    agentruntime.InstanceSnapshot
	listErr                                      error
	lastQuery                                    agentruntime.InstanceQuery
	listManaged, listInstances, applies, deletes int
	applyRequests                                []agentruntime.ApplyRequest
	deleteRequests                               []agentruntime.DeleteRequest
	getObject                                    agentruntime.Object
	gets                                         int
	managedSequence                              []agentruntime.ObjectSnapshot
	applyErrAt, deleteErrAt, listErrAt           int
	applyErr, deleteErr                          error
}

func (p *fakeProvider) Info(context.Context) (agentruntime.Info, error) { return p.info, nil }
func (p *fakeProvider) Get(context.Context, agentruntime.ObjectRef) (agentruntime.Object, error) {
	p.gets++
	return p.getObject, nil
}

func (p *fakeProvider) ListManaged(context.Context, agentruntime.TaskIdentity) (agentruntime.ObjectSnapshot, error) {
	p.listManaged++
	if p.listErrAt == p.listManaged {
		return agentruntime.ObjectSnapshot{}, p.listErr
	}
	if len(p.managedSequence) >= p.listManaged {
		return p.managedSequence[p.listManaged-1], nil
	}
	if p.listErrAt != 0 {
		return p.managed, nil
	}
	return p.managed, p.listErr
}
func (p *fakeProvider) ListCandidates(_ context.Context, _ agentruntime.ManagedObjectQuery) (agentruntime.ObjectSnapshot, error) {
	return p.ListManaged(context.Background(), agentruntime.TaskIdentity{})
}
func (p *fakeProvider) Apply(_ context.Context, request agentruntime.ApplyRequest) (agentruntime.ApplyResult, error) {
	p.applies++
	p.applyRequests = append(p.applyRequests, request)
	if p.applyErrAt == p.applies {
		return agentruntime.ApplyResult{}, p.applyErr
	}
	return agentruntime.ApplyResult{}, nil
}
func (p *fakeProvider) Delete(_ context.Context, request agentruntime.DeleteRequest) error {
	p.deletes++
	p.deleteRequests = append(p.deleteRequests, request)
	if p.deleteErrAt == p.deletes {
		return p.deleteErr
	}
	return nil
}
func (p *fakeProvider) ListInstances(_ context.Context, query agentruntime.InstanceQuery) (agentruntime.InstanceSnapshot, error) {
	p.listInstances++
	p.lastQuery = query
	return p.instances, nil
}
