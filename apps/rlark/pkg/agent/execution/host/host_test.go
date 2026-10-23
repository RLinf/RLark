package host

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/observability"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

func TestHostClaimsAndReconcilesWithoutRuntimeSpecificClient(t *testing.T) {
	task := hostTask()
	host, executor, kube := newTestHost(t, task)
	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeSucceeded || executor.reconciles != 1 {
		t.Fatalf("result=%#v reconciles=%d", result, executor.reconciles)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if current.Annotations[TaskClaimantAnnotation] != "cluster/Fake" || !contains(current.Finalizers, TaskFinalizer) {
		t.Fatalf("claim annotations=%v finalizers=%v", current.Annotations, current.Finalizers)
	}
}

func TestHostRecordsLifecycleWithoutChangingResult(t *testing.T) {
	task := hostTask()
	host, executor, _ := newTestHost(t, task)
	recorder := &observability.RecordingObserver{}
	host.Bundle.Observer = observability.Safe(recorder, observability.StaticConfigSource{Config: observability.Config{Payloads: map[observability.Payload]bool{observability.PayloadTaskSnapshot: true}}}, nil, nil)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded || executor.reconciles != 1 {
		t.Fatalf("result=%#v error=%v reconciles=%d", result, err, executor.reconciles)
	}
	records := recorder.Records()
	if len(records) != 1 || records[0].Result == nil || records[0].Result.Outcome != string(OutcomeSucceeded) || records[0].Result.Reason != "Reconciled" {
		t.Fatalf("records=%#v", records)
	}
	want := map[string]bool{"task.read": false, "task.snapshot": false, "task.claim": false, "lifecycle.before_reconcile": false, "executor.reconcile": false, "lifecycle.after_reconcile": false}
	for _, event := range records[0].Events {
		if _, found := want[event.Name]; found {
			want[event.Name] = true
		}
		if event.Name == "task.snapshot" && len(event.Payload) == 0 {
			t.Fatal("task snapshot payload was not recorded")
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("event %q missing from %#v", name, records[0].Events)
		}
	}
}

func TestHostObserverPanicDoesNotChangeResultOrEffects(t *testing.T) {
	task := hostTask()
	host, executor, _ := newTestHost(t, task)
	host.Bundle.Observer = observability.Safe(panickingHostObserver{}, nil, nil, nil)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded || executor.reconciles != 1 {
		t.Fatalf("result=%#v error=%v reconciles=%d", result, err, executor.reconciles)
	}
}

func TestHostDecodesLifecycleMutationAfterInitialClaim(t *testing.T) {
	task := hostTask()
	host, executor, _ := newTestHost(t, task)
	host.Bundle.Decoder = TaskDecoderFunc(func(task *rlarkv1alpha1.Task) (coordination.TaskSnapshot, error) {
		return coordination.TaskSnapshot{
			Identity:         agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: task.Namespace, Name: task.Name}, ObjectUID: string(task.UID), Generation: task.Generation},
			WorkloadSelector: coordination.WorkloadSelector{Type: "FakeWorkload", Version: "v1"},
			CarrierSelector:  coordination.CarrierSelector{Type: "FakeCarrier", Version: "v1"},
			Configuration: coordination.WorkloadConfiguration{Carrier: coordination.VersionedConfiguration{
				Type:       "test.example/lifecycle",
				Version:    "v1",
				Parameters: []byte(task.Annotations["test.example/lifecycle-value"]),
			}},
		}, nil
	})
	host.Bundle.Lifecycle = mutatingLifecycle{mutate: func(task *rlarkv1alpha1.Task) {
		if task.Annotations == nil {
			task.Annotations = make(map[string]string)
		}
		task.Annotations["test.example/lifecycle-value"] = "current"
	}}

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if got := string(executor.snapshots[0].Configuration.Carrier.Parameters); got != "current" {
		t.Fatalf("Executor received lifecycle value %q, want current", got)
	}
}

func TestHostAdoptsLegacyFinalizerAndConfirmsCleanup(t *testing.T) {
	task := hostTask()
	task.Finalizers = []string{TaskFinalizer}
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, kube := newTestHost(t, task)
	executor.cleanupComplete = false
	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeCleanupInProgress {
		t.Fatalf("result=%#v", result)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if !contains(current.Finalizers, TaskFinalizer) || current.Annotations[TaskClaimantAnnotation] != "cluster/Fake" {
		t.Fatalf("legacy claim=%v finalizers=%v", current.Annotations, current.Finalizers)
	}
	executor.cleanupComplete = true
	if _, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task)); err != nil {
		t.Fatal(err)
	}
	current = rlarkv1alpha1.Task{}
	err = kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current)
	if client.IgnoreNotFound(err) != nil {
		t.Fatal(err)
	}
	if err == nil && contains(current.Finalizers, TaskFinalizer) {
		t.Fatalf("cleanup-complete finalizer=%v", current.Finalizers)
	}
}

func TestHostLeavesUnrealizableLegacyFinalizerWithoutClaimantUntouched(t *testing.T) {
	task := hostTask()
	task.Finalizers = []string{TaskFinalizer}
	task.Annotations = map[string]string{"other.example/key": "keep"}
	task.Spec.Runtime.Kind = "Other"
	host, executor, kube := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange || result.Retry.Class != RetryOnChange {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.reconciles != 0 || executor.cleanups != 0 {
		t.Fatalf("unrealizable legacy Task reached Executor: %#v", executor)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if !contains(current.Finalizers, TaskFinalizer) || current.Annotations[TaskClaimantAnnotation] != "" || current.Annotations["other.example/key"] != "keep" {
		t.Fatalf("unrealizable legacy Task metadata changed: %#v", current.ObjectMeta)
	}
	if !reflect.DeepEqual(current.Status, task.Status) {
		t.Fatalf("unrealizable legacy Task status changed: %#v", current.Status)
	}
}

func TestHostAdoptsRealizableLegacyFinalizer(t *testing.T) {
	task := hostTask()
	task.Finalizers = []string{TaskFinalizer}
	host, executor, kube := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.reconciles != 1 || executor.cleanups != 0 {
		t.Fatalf("realizable legacy Task executor=%#v", executor)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if current.Annotations[TaskClaimantAnnotation] != "cluster/Fake" || current.Annotations[TaskClaimedWorkloadTypeAnnotation] != "FakeWorkload" || current.Annotations[TaskClaimedWorkloadVersionAnnotation] != "v1" || !contains(current.Finalizers, TaskFinalizer) {
		t.Fatalf("realizable legacy Task was not adopted: %#v", current.ObjectMeta)
	}
}

func TestHostBackfillsMissingClaimedWorkloadSelectorBeforeReconcile(t *testing.T) {
	task := claimedHostTask()
	host, executor, kube := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded || executor.reconciles != 1 || executor.cleanups != 0 {
		t.Fatalf("result=%#v error=%v executor=%#v", result, err, executor)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if got := claimedWorkloadSelector(&current); got.Type != "FakeWorkload" || got.Version != "v1" {
		t.Fatalf("backfilled selector=%#v", got)
	}
	if current.Annotations[TaskClaimantAnnotation] != "cluster/Fake" || !contains(current.Finalizers, TaskFinalizer) {
		t.Fatalf("backfill changed claim scope: %#v", current.ObjectMeta)
	}
}

func TestHostNonDeletingTaskDoesNotCompletePartialClaimedWorkloadSelector(t *testing.T) {
	task := claimedHostTask()
	task.Annotations[TaskClaimedWorkloadTypeAnnotation] = "FakeWorkload"
	host, executor, kube := newTestHost(t, task)
	statusWrites := 0
	host.Status = statusWriterFunc(func(context.Context, *rlarkv1alpha1.Task, StatusIntent) error {
		statusWrites++
		return nil
	})

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err == nil || result.Outcome != OutcomeBlocked || result.Retry.Class != RetryOnChange || result.Reason != "ClaimedWorkloadSelectorIncomplete" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.reconciles != 0 || executor.cleanups != 0 || statusWrites != 0 {
		t.Fatalf("partial selector caused effects: executor=%#v statusWrites=%d", executor, statusWrites)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if current.Annotations[TaskClaimedWorkloadTypeAnnotation] != "FakeWorkload" || current.Annotations[TaskClaimedWorkloadVersionAnnotation] != "" {
		t.Fatalf("partial selector was completed: %v", current.Annotations)
	}
}

func TestHostFreezesClaimedRuntimeSelector(t *testing.T) {
	task := hostTask()
	task.Finalizers = []string{TaskFinalizer}
	task.Annotations = map[string]string{TaskClaimantAnnotation: "cluster/Fake"}
	task.Spec.Runtime.Kind = "Other"
	host, executor, _ := newTestHost(t, task)
	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeWaitingForChange || executor.reconciles != 0 || executor.cleanups != 0 {
		t.Fatalf("result=%#v executor=%#v", result, executor)
	}
}

func TestKubernetesStatusWriterFencesStaleIntent(t *testing.T) {
	task := hostTask()
	scheme := runtime.NewScheme()
	_ = rlarkv1alpha1.AddToScheme(scheme)
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	writer := KubernetesStatusWriter{Client: kube}
	intent := IntentForTask(task, rlarkv1alpha1.TaskPhaseRunning, "stale", nil, StatusPriorityWorkload)
	intent.TaskUID = "old"
	if err := writer.WriteStatus(context.Background(), task, intent); err != nil {
		t.Fatal(err)
	}
	var current rlarkv1alpha1.Task
	_ = kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current)
	if current.Status.Phase != "" {
		t.Fatalf("stale intent wrote status: %#v", current.Status)
	}
	if current.Status.ObservedGeneration != 0 {
		t.Fatalf("stale intent advanced observedGeneration to %d, want 0", current.Status.ObservedGeneration)
	}
}

func TestKubernetesStatusWriterAdvancesObservedGenerationOnSpecOnlyChange(t *testing.T) {
	task := hostTask()
	// The observed values already match what the writer will compute, so the
	// only thing left to report is that generation 2 has now been reconciled.
	task.Status.Phase = rlarkv1alpha1.TaskPhaseRunning
	task.Status.Message = "ready"
	task.Status.ObservedGeneration = 1
	scheme := runtime.NewScheme()
	_ = rlarkv1alpha1.AddToScheme(scheme)
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	writer := KubernetesStatusWriter{Client: kube}
	intent := IntentForTask(task, rlarkv1alpha1.TaskPhaseRunning, "ready", nil, StatusPriorityWorkload)
	if err := writer.WriteStatus(context.Background(), task, intent); err != nil {
		t.Fatal(err)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if current.Status.ObservedGeneration != task.Generation {
		t.Fatalf("observedGeneration = %d, want %d", current.Status.ObservedGeneration, task.Generation)
	}
	if current.Status.Phase != rlarkv1alpha1.TaskPhaseRunning || current.Status.Message != "ready" {
		t.Fatalf("observed values changed: %#v", current.Status)
	}
}

func TestKubernetesStatusWriterObservedGenerationIsIdempotent(t *testing.T) {
	task := hostTask()
	task.Status.Phase = rlarkv1alpha1.TaskPhaseRunning
	task.Status.Message = "ready"
	task.Status.ObservedGeneration = task.Generation
	scheme := runtime.NewScheme()
	_ = rlarkv1alpha1.AddToScheme(scheme)
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	before := &rlarkv1alpha1.Task{}
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), before); err != nil {
		t.Fatal(err)
	}
	writer := KubernetesStatusWriter{Client: kube}
	intent := IntentForTask(task, rlarkv1alpha1.TaskPhaseRunning, "ready", nil, StatusPriorityWorkload)
	if err := writer.WriteStatus(context.Background(), task, intent); err != nil {
		t.Fatal(err)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if current.ResourceVersion != before.ResourceVersion {
		t.Fatalf("no-op write patched status: resourceVersion %s -> %s", before.ResourceVersion, current.ResourceVersion)
	}
}

func TestKubernetesStatusWriterConditionOnlyPreservesFields(t *testing.T) {
	task := hostTask()
	task.Status.Phase = rlarkv1alpha1.TaskPhaseRunning
	task.Status.Message = "ready"
	task.Status.ObservedNodes = []string{"node"}
	scheme := runtime.NewScheme()
	_ = rlarkv1alpha1.AddToScheme(scheme)
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	writer := KubernetesStatusWriter{Client: kube}
	intent := ConditionIntent(task, metav1.Condition{Type: "Progressing", Status: metav1.ConditionTrue, Reason: "Test"}, false, StatusPriorityLifecycle)
	if err := writer.WriteStatus(context.Background(), task, intent); err != nil {
		t.Fatal(err)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if current.Status.Phase != rlarkv1alpha1.TaskPhaseRunning || current.Status.Message != "ready" || len(current.Status.ObservedNodes) != 1 {
		t.Fatalf("condition-only intent changed fields: %#v", current.Status)
	}
}

func TestHostDoesNotClaimInvalidTask(t *testing.T) {
	task := hostTask()
	host, executor, kube := newTestHost(t, task)
	host.Eligibility = fixedEligibility{matches: true, validation: "invalid"}
	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeWaitingForChange || executor.reconciles != 0 || executor.cleanups != 0 {
		t.Fatalf("result=%#v executor=%#v", result, executor)
	}
	var current rlarkv1alpha1.Task
	_ = kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current)
	if contains(current.Finalizers, TaskFinalizer) || current.Annotations[TaskClaimantAnnotation] != "" {
		t.Fatalf("invalid Task was claimed: %#v", current.ObjectMeta)
	}
}

func TestHostDoesNotClaimUnregisteredImplementation(t *testing.T) {
	task := hostTask()
	host, executor, kube := newTestHost(t, task)
	host.Bundle.Implementations = &implementationRegistry{adapters: map[ImplementationKey]workload.Adapter{}}

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange || result.Retry.Class != RetryOnChange {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.reconciles != 0 || executor.cleanups != 0 {
		t.Fatalf("unregistered implementation reached executor: %#v", executor)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if contains(current.Finalizers, TaskFinalizer) || current.Annotations[TaskClaimantAnnotation] != "" {
		t.Fatalf("unregistered implementation was claimed: %#v", current.ObjectMeta)
	}
}

func TestHostDoesNotClaimUndecodableTask(t *testing.T) {
	task := hostTask()
	host, executor, kube := newTestHost(t, task)
	host.Bundle.Decoder = TaskDecoderFunc(func(*rlarkv1alpha1.Task) (coordination.TaskSnapshot, error) {
		return coordination.TaskSnapshot{}, errors.New("unsupported current Task shape")
	})

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange || result.Retry.Class != RetryOnChange || result.Reason != "TaskDecodeFailed" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.reconciles != 0 || executor.cleanups != 0 {
		t.Fatalf("undecodable Task reached executor: %#v", executor)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if contains(current.Finalizers, TaskFinalizer) || current.Annotations[TaskClaimantAnnotation] != "" {
		t.Fatalf("undecodable Task was claimed: %#v", current.ObjectMeta)
	}
}

func TestHostPreservesClaimedTaskWithUnregisteredImplementation(t *testing.T) {
	task := claimedHostTask()
	host, executor, kube := newTestHost(t, task)
	host.Bundle.Implementations = &implementationRegistry{adapters: map[ImplementationKey]workload.Adapter{}}

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange || result.Retry.Class != RetryOnChange || result.Reason != "ClaimedImplementationUnavailable" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.cleanups != 0 || executor.reconciles != 0 {
		t.Fatalf("executor=%#v", executor)
	}
	assertHostClaimPreserved(t, kube, task)
}

func TestHostPreservesClaimedTaskWhenCurrentSpecCannotDecode(t *testing.T) {
	task := claimedHostTask()
	host, executor, kube := newTestHost(t, task)
	host.Bundle.Decoder = TaskDecoderFunc(func(*rlarkv1alpha1.Task) (coordination.TaskSnapshot, error) {
		return coordination.TaskSnapshot{}, errors.New("unsupported current Task shape")
	})

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange || result.Retry.Class != RetryOnChange || result.Reason != "ClaimedTaskDecodeFailed" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.cleanups != 0 || executor.reconciles != 0 {
		t.Fatalf("executor=%#v", executor)
	}
	assertHostClaimPreserved(t, kube, task)
}

func TestHostPreservesClaimedInvalidTask(t *testing.T) {
	task := claimedHostTask()
	host, executor, kube := newTestHost(t, task)
	host.Eligibility = fixedEligibility{matches: true, validation: "invalid"}
	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeWaitingForChange || result.Retry.Class != RetryOnChange || result.Reason != "ClaimedTaskInvalid" || executor.cleanups != 0 || executor.reconciles != 0 {
		t.Fatalf("result=%#v executor=%#v", result, executor)
	}
	assertHostClaimPreserved(t, kube, task)

	host.Eligibility = testEligibility{}
	result, err = host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded || executor.reconciles != 1 || executor.cleanups != 0 {
		t.Fatalf("recovered result=%#v error=%v executor=%#v", result, err, executor)
	}
}

func TestHostClaimedImplementationRecoveryResumesReconcile(t *testing.T) {
	task := claimedHostTask()
	host, executor, _ := newTestHost(t, task)
	registered := host.Bundle.Implementations
	host.Bundle.Implementations = &implementationRegistry{adapters: map[ImplementationKey]workload.Adapter{}}

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange {
		t.Fatalf("unavailable result=%#v error=%v", result, err)
	}
	host.Bundle.Implementations = registered
	result, err = host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("recovered result=%#v error=%v", result, err)
	}
	if executor.cleanups != 0 || executor.reconciles != 1 {
		t.Fatalf("executor=%#v", executor)
	}
}

func TestHostDeletionWithUndecodableSpecUsesClaimedIdentity(t *testing.T) {
	task := claimedHostTask()
	task.Annotations[TaskClaimedWorkloadTypeAnnotation] = "FakeWorkload"
	task.Annotations[TaskClaimedWorkloadVersionAnnotation] = "v1"
	task.Annotations[TaskClaimedCarrierTypeAnnotation] = "FakeCarrier"
	task.Annotations[TaskClaimedCarrierVersionAnnotation] = "v1"
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, _ := newTestHost(t, task)
	host.Bundle.Decoder = TaskDecoderFunc(func(*rlarkv1alpha1.Task) (coordination.TaskSnapshot, error) {
		return coordination.TaskSnapshot{}, errors.New("unsupported current Task shape")
	})

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded || result.Reason != "TaskDeletionCleanup" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.cleanups != 1 || executor.reconciles != 0 || len(executor.cleanupIdentities) != 1 {
		t.Fatalf("executor=%#v", executor)
	}
	if executor.cleanupIdentities[0].ObjectUID != string(task.UID) {
		t.Fatalf("cleanup identity=%#v", executor.cleanupIdentities[0])
	}
}

func TestHostDeletionRecoversCompletelyMissingLegacyWorkloadSelector(t *testing.T) {
	task := claimedHostTask()
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, _ := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded || result.Reason != "TaskDeletionCleanup" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.cleanups != 1 || executor.reconciles != 0 || len(executor.cleanupSelectors) != 1 {
		t.Fatalf("executor=%#v", executor)
	}
	if selector := executor.cleanupSelectors[0]; selector.Type != "FakeWorkload" || selector.Version != "v1" {
		t.Fatalf("cleanup selector=%#v", selector)
	}
}

func TestHostDeletionDoesNotReplacePartialClaimedWorkloadSelector(t *testing.T) {
	task := claimedHostTask()
	task.Annotations[TaskClaimedWorkloadTypeAnnotation] = "FakeWorkload"
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, _ := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err == nil || result.Outcome != OutcomeBlocked || result.Retry.Class != RetryOnChange || result.Reason != "ClaimedWorkloadSelectorIncomplete" || executor.cleanups != 0 {
		t.Fatalf("result=%#v error=%v executor=%#v", result, err, executor)
	}
}

func TestHostDeletionBlocksWhenLegacySelectorCannotBeDecoded(t *testing.T) {
	task := claimedHostTask()
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, kube := newTestHost(t, task)
	host.Bundle.Decoder = TaskDecoderFunc(func(*rlarkv1alpha1.Task) (coordination.TaskSnapshot, error) {
		return coordination.TaskSnapshot{}, errors.New("unsupported legacy Task shape")
	})

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err == nil || result.Outcome != OutcomeBlocked || result.Retry.Class != RetryOnChange || result.Reason != "LegacyCleanupSelectorUnavailable" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.cleanups != 0 || executor.reconciles != 0 {
		t.Fatalf("undecodable Task reached executor: %#v", executor)
	}
	assertHostClaimPreserved(t, kube, task)
}

func TestHostDeletionWithInvalidSpecUsesClaimedIdentity(t *testing.T) {
	task := claimedHostTask()
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, _ := newTestHost(t, task)
	host.Eligibility = fixedEligibility{matches: true, validation: "invalid"}

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded || result.Reason != "TaskDeletionCleanup" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.cleanups != 1 || executor.reconciles != 0 || len(executor.cleanupIdentities) != 1 {
		t.Fatalf("executor=%#v", executor)
	}
	if executor.cleanupIdentities[0].ObjectUID != string(task.UID) {
		t.Fatalf("cleanup identity=%#v", executor.cleanupIdentities[0])
	}
}

func TestHostLeavesOtherClaimUntouched(t *testing.T) {
	task := claimedHostTask()
	task.Annotations[TaskClaimantAnnotation] = "other/Fake"
	host, executor, _ := newTestHost(t, task)
	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeWaitingForChange || executor.cleanups != 0 || executor.reconciles != 0 {
		t.Fatalf("result=%#v executor=%#v", result, executor)
	}
}

func TestHostRejectsClaimedWorkloadSelectorChange(t *testing.T) {
	task := claimedHostTask()
	task.Annotations[TaskClaimedWorkloadTypeAnnotation] = "OldWorkload"
	task.Annotations[TaskClaimedWorkloadVersionAnnotation] = "v1"
	host, executor, kube := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange || result.Retry.Class != RetryOnChange || result.Reason != "WorkloadSelectorChanged" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.reconciles != 0 || executor.cleanups != 0 {
		t.Fatalf("executor=%#v", executor)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if got := claimedWorkloadSelector(&current); got.Type != "OldWorkload" || got.Version != "v1" {
		t.Fatalf("claimed selector=%#v", got)
	}
	if current.Annotations[TaskClaimantAnnotation] != "cluster/Fake" || !contains(current.Finalizers, TaskFinalizer) {
		t.Fatalf("migration changed claim scope: %#v", current.ObjectMeta)
	}
}

func TestHostRejectsClaimedCarrierSelectorChange(t *testing.T) {
	task := claimedHostTask()
	task.Annotations[TaskClaimedCarrierTypeAnnotation] = "OldCarrier"
	host, executor, kube := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange || result.Retry.Class != RetryOnChange || result.Reason != "CarrierSelectorChanged" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.reconciles != 0 || executor.cleanups != 0 {
		t.Fatalf("executor=%#v", executor)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if got := claimedCarrierSelector(&current); got.Type != "OldCarrier" || got.Version != "v1" {
		t.Fatalf("claimed carrier=%#v", got)
	}
}

func TestHostSuspensionCleansRuntimeAndRetainsClaim(t *testing.T) {
	task := claimedHostTask()
	task.Spec.Suspended = true
	host, executor, kube := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange || result.Reason != "TaskSuspended" {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.cleanups != 1 || executor.reconciles != 0 {
		t.Fatalf("executor=%#v", executor)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if current.Status.Phase != rlarkv1alpha1.TaskPhaseStopped {
		t.Fatalf("phase=%q", current.Status.Phase)
	}
	assertHostClaimPreserved(t, kube, task)

	current.Spec.Suspended = false
	if err := kube.Update(context.Background(), &current); err != nil {
		t.Fatal(err)
	}
	result, err = host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("resume result=%#v error=%v", result, err)
	}
	if executor.reconciles != 1 {
		t.Fatalf("reconciles=%d", executor.reconciles)
	}
}

func TestHostWorkloadMigrationDeletionUsesClaimedSelector(t *testing.T) {
	task := claimedHostTask()
	task.Annotations[TaskClaimedWorkloadTypeAnnotation] = "OldWorkload"
	task.Annotations[TaskClaimedWorkloadVersionAnnotation] = "v1"
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, _ := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded || result.Reason != "TaskDeletionCleanup" || executor.cleanups != 1 || executor.reconciles != 0 {
		t.Fatalf("result=%#v error=%v executor=%#v", result, err, executor)
	}
	if got := executor.cleanupSelectors[0]; got.Type != "OldWorkload" || got.Version != "v1" {
		t.Fatalf("cleanup selector=%#v", got)
	}
}

func TestHostLifecycleDecisions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		decision   LifecycleDecision
		want       Outcome
		cleanups   int
		reconciles int
	}{
		{name: "wait", decision: LifecycleDecision{Action: LifecycleWait, Retry: RetryDirective{Class: RetryOnChange}}, want: OutcomeWaitingForChange},
		{name: "block", decision: LifecycleDecision{Action: LifecycleBlock, Retry: RetryDirective{Class: RetryOnChange}}, want: OutcomeBlocked},
		{name: "cleanup proceed", decision: LifecycleDecision{Action: LifecycleCleanup}, want: OutcomeSucceeded, cleanups: 1, reconciles: 1},
		{name: "cleanup wait", decision: LifecycleDecision{Action: LifecycleCleanup, AfterCleanup: LifecycleWait}, want: OutcomeWaitingForChange, cleanups: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			task := claimedHostTask()
			host, executor, _ := newTestHost(t, task)
			host.Bundle.Lifecycle = fixedLifecycle{decision: tc.decision}
			result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
			if err != nil {
				t.Fatal(err)
			}
			if result.Outcome != tc.want || executor.cleanups != tc.cleanups || executor.reconciles != tc.reconciles {
				t.Fatalf("result=%#v executor=%#v", result, executor)
			}
		})
	}
}

func TestHostRunsLifecycleEffectOnlyAfterStatusWrite(t *testing.T) {
	task := claimedHostTask()
	host, executor, _ := newTestHost(t, task)
	events := []string{}
	host.Status = statusWriterFunc(func(context.Context, *rlarkv1alpha1.Task, StatusIntent) error {
		events = append(events, "status")
		return nil
	})
	intent := IntentForTask(task, rlarkv1alpha1.TaskPhasePending, "migration", nil, StatusPriorityLifecycle)
	host.Bundle.Lifecycle = fixedLifecycle{decision: LifecycleDecision{
		Action: LifecycleWait,
		Status: &intent,
		AfterStatus: func(context.Context) error {
			events = append(events, "effect")
			return nil
		},
		Retry: RetryDirective{Class: RetryInProgress},
	}}

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeWaitingForChange {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if !slices.Equal(events, []string{"status", "effect"}) {
		t.Fatalf("events=%v", events)
	}
	if executor.reconciles != 0 {
		t.Fatalf("reconciles=%d", executor.reconciles)
	}
}

func TestHostSkipsLifecycleEffectWhenStatusWriteFails(t *testing.T) {
	task := claimedHostTask()
	host, _, _ := newTestHost(t, task)
	effectCalls := 0
	host.Status = statusWriterFunc(func(context.Context, *rlarkv1alpha1.Task, StatusIntent) error {
		return apierrors.NewConflict(schema.GroupResource{Group: rlarkv1alpha1.GroupVersion.Group, Resource: "tasks"}, task.Name, errors.New("simulated conflict"))
	})
	intent := IntentForTask(task, rlarkv1alpha1.TaskPhasePending, "migration", nil, StatusPriorityLifecycle)
	host.Bundle.Lifecycle = fixedLifecycle{decision: LifecycleDecision{
		Action: LifecycleWait,
		Status: &intent,
		AfterStatus: func(context.Context) error {
			effectCalls++
			return nil
		},
	}}

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err == nil || result.Outcome != OutcomeRetrying || result.Retry.Class != RetryConflict {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if effectCalls != 0 {
		t.Fatalf("effect calls=%d", effectCalls)
	}
}

func TestHostCleanupFailureRetainsClaim(t *testing.T) {
	task := claimedHostTask()
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, kube := newTestHost(t, task)
	executor.cleanupErr = errors.New("temporary cleanup failure")
	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err == nil || result.Outcome != OutcomeRetrying || result.Retry.Class != RetryTransient {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	var current rlarkv1alpha1.Task
	if getErr := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); getErr != nil {
		t.Fatal(getErr)
	}
	if !contains(current.Finalizers, TaskFinalizer) {
		t.Fatalf("cleanup failure released claim: %#v", current.ObjectMeta)
	}
}

func TestHostCleanupRecoversAfterTransientFailure(t *testing.T) {
	task := claimedHostTask()
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, kube := newTestHost(t, task)
	executor.cleanupErr = errors.New("temporary cleanup failure")

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err == nil || result.Outcome != OutcomeRetrying || result.Retry.Class != RetryTransient {
		t.Fatalf("first result=%#v error=%v", result, err)
	}
	executor.cleanupErr = nil
	result, err = host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("recovery result=%#v error=%v", result, err)
	}
	if executor.cleanups != 2 {
		t.Fatalf("cleanup calls=%d, want 2", executor.cleanups)
	}
	var current rlarkv1alpha1.Task
	err = kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current)
	if client.IgnoreNotFound(err) != nil {
		t.Fatal(err)
	}
	if err == nil && (contains(current.Finalizers, TaskFinalizer) || current.Annotations[TaskClaimantAnnotation] != "") {
		t.Fatalf("recovered cleanup retained claim: %#v", current.ObjectMeta)
	}
}

func TestHostCleanupReleaseConflictRetainsClaimAndRetriesCleanup(t *testing.T) {
	task := claimedHostTask()
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, kube := newTestHost(t, task)
	conflicts := &conflictOncePatchClient{Client: kube, conflict: true}
	host.Tasks = conflicts

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err == nil || !apierrors.IsConflict(err) || result.Outcome != OutcomeRetrying || result.Retry.Class != RetryConflict {
		t.Fatalf("conflict result=%#v error=%v", result, err)
	}
	var current rlarkv1alpha1.Task
	if getErr := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); getErr != nil {
		t.Fatal(getErr)
	}
	if !contains(current.Finalizers, TaskFinalizer) || current.Annotations[TaskClaimantAnnotation] == "" {
		t.Fatalf("release conflict changed claim: %#v", current.ObjectMeta)
	}

	result, err = host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("retry result=%#v error=%v", result, err)
	}
	if executor.cleanups != 2 {
		t.Fatalf("cleanup calls=%d, want cleanup to be safely repeated", executor.cleanups)
	}
}

func TestHostClaimPatchUsesOptimisticLock(t *testing.T) {
	task := hostTask()
	task.Finalizers = []string{"other.example/finalizer"}
	host, executor, kube := newTestHost(t, task)
	inspector := &optimisticLockPatchClient{Client: kube}
	host.Tasks = inspector

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if !inspector.sawResourceVersion {
		t.Fatal("claim patch omitted metadata.resourceVersion optimistic lock")
	}
	if executor.reconciles != 1 {
		t.Fatalf("reconciles=%d, want 1", executor.reconciles)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if !contains(current.Finalizers, "other.example/finalizer") || !contains(current.Finalizers, TaskFinalizer) {
		t.Fatalf("claim replaced finalizers: %v", current.Finalizers)
	}
	if inspector.replacedFinalizerTable {
		t.Fatalf("claim patch replaced metadata.finalizers: %s", inspector.patchData)
	}
}

func TestHostClaimReleasePatchUsesOptimisticLock(t *testing.T) {
	task := claimedHostTask()
	task.Finalizers = []string{"before.example/finalizer", TaskFinalizer, "after.example/finalizer"}
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, _, kube := newTestHost(t, task)
	inspector := &optimisticLockPatchClient{Client: kube}
	host.Tasks = inspector

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if !inspector.sawResourceVersion {
		t.Fatal("claim release patch omitted metadata.resourceVersion optimistic lock")
	}
	var current rlarkv1alpha1.Task
	getErr := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current)
	if client.IgnoreNotFound(getErr) != nil {
		t.Fatal(getErr)
	}
	if getErr == nil && (!contains(current.Finalizers, "before.example/finalizer") || !contains(current.Finalizers, "after.example/finalizer") || contains(current.Finalizers, TaskFinalizer)) {
		t.Fatalf("release changed unowned finalizers: %v", current.Finalizers)
	}
	if inspector.replacedFinalizerTable {
		t.Fatalf("release patch replaced metadata.finalizers: %s", inspector.patchData)
	}
}

func TestClaimPatchesMutateOnlyOwnedFinalizerEntry(t *testing.T) {
	task := hostTask()
	task.ResourceVersion = "17"
	task.Finalizers = []string{"one.example/finalizer", "two.example/finalizer"}
	task.Annotations = map[string]string{"other.example/key": "keep"}

	claim, err := persistClaimPatch(task, "cluster/Fake", coordination.WorkloadSelector{Type: "FakeWorkload", Version: "v1"}, coordination.CarrierSelector{Type: "FakeCarrier", Version: "v1"}, true)
	if err != nil {
		t.Fatal(err)
	}
	claimData, err := claim.Data(task)
	if err != nil {
		t.Fatal(err)
	}
	claimPayload := string(claimData)
	if !strings.Contains(claimPayload, `"op":"add","path":"/metadata/finalizers/-","value":"`+TaskFinalizer+`"`) {
		t.Fatalf("claim does not append owned finalizer: %s", claimPayload)
	}
	if strings.Contains(claimPayload, `"path":"/metadata/finalizers","value"`) || strings.Contains(claimPayload, "one.example/finalizer") || strings.Contains(claimPayload, "two.example/finalizer") {
		t.Fatalf("claim replaces or copies the finalizer table: %s", claimPayload)
	}

	claimed := task.DeepCopy()
	claimed.Finalizers = []string{"one.example/finalizer", TaskFinalizer, "two.example/finalizer"}
	claimed.Annotations[TaskClaimantAnnotation] = "cluster/Fake"
	release, err := releaseClaimPatch(claimed)
	if err != nil {
		t.Fatal(err)
	}
	releaseData, err := release.Data(claimed)
	if err != nil {
		t.Fatal(err)
	}
	releasePayload := string(releaseData)
	if !strings.Contains(releasePayload, `"op":"test","path":"/metadata/finalizers/1","value":"`+TaskFinalizer+`"`) || !strings.Contains(releasePayload, `"op":"remove","path":"/metadata/finalizers/1"`) {
		t.Fatalf("release does not test and remove only the owned entry: %s", releasePayload)
	}
	if strings.Contains(releasePayload, `"path":"/metadata/finalizers","value"`) || strings.Contains(releasePayload, "one.example/finalizer") || strings.Contains(releasePayload, "two.example/finalizer") {
		t.Fatalf("release replaces or copies the finalizer table: %s", releasePayload)
	}
}

func TestReleaseClaimRemovesLegacyClaimedDomainAnnotation(t *testing.T) {
	task := claimedHostTask()
	task.Annotations[legacyTaskClaimedDomainAnnotation] = "legacy-domain"

	patch, err := releaseClaimPatch(task)
	if err != nil {
		t.Fatal(err)
	}
	data, err := patch.Data(task)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), escapeJSONPointer(legacyTaskClaimedDomainAnnotation)) {
		t.Fatalf("release patch did not remove legacy claimed-domain annotation: %s", data)
	}
}

func TestHostClaimConflictDoesNotOverwriteConcurrentMetadata(t *testing.T) {
	task := hostTask()
	host, executor, kube := newTestHost(t, task)
	conflicts := &conflictOncePatchClient{Client: kube, conflict: true}
	host.Tasks = conflicts

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err == nil || !apierrors.IsConflict(err) || result.Outcome != OutcomeRetrying || result.Retry.Class != RetryConflict {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.reconciles != 0 || executor.cleanups != 0 {
		t.Fatalf("claim conflict reached executor: %#v", executor)
	}
	var current rlarkv1alpha1.Task
	if getErr := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); getErr != nil {
		t.Fatal(getErr)
	}
	if contains(current.Finalizers, TaskFinalizer) || current.Annotations[TaskClaimantAnnotation] != "" {
		t.Fatalf("claim conflict persisted stale ownership: %#v", current.ObjectMeta)
	}
}

func TestHostNotFoundDoesNotRunNameOnlyCleanup(t *testing.T) {
	task := hostTask()
	host, executor, kube := newTestHost(t, task)
	if err := kube.Delete(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeDropped {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.cleanups != 0 || executor.reconciles != 0 {
		t.Fatalf("not-found Task caused executor calls: %#v", executor)
	}
}

func TestHostRuntimeSelectorRecoveryResumesReconcile(t *testing.T) {
	task := claimedHostTask()
	task.Spec.Runtime.Kind = "Other"
	host, executor, kube := newTestHost(t, task)
	key := client.ObjectKeyFromObject(task)

	result, err := host.Reconcile(context.Background(), key)
	if err != nil || result.Outcome != OutcomeWaitingForChange {
		t.Fatalf("changed selector result=%#v error=%v", result, err)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), key, &current); err != nil {
		t.Fatal(err)
	}
	current.Spec.Runtime.Kind = "Fake"
	if err := kube.Update(context.Background(), &current); err != nil {
		t.Fatal(err)
	}

	result, err = host.Reconcile(context.Background(), key)
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("restored selector result=%#v error=%v", result, err)
	}
	if executor.reconciles != 1 || executor.cleanups != 0 {
		t.Fatalf("executor=%#v", executor)
	}
}

func TestHostDeletionAfterSelectorChangeUsesClaimedIdentity(t *testing.T) {
	task := claimedHostTask()
	task.Spec.Runtime.Kind = "Other"
	now := metav1.Now()
	task.DeletionTimestamp = &now
	host, executor, _ := newTestHost(t, task)

	result, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%#v error=%v", result, err)
	}
	if executor.cleanups != 1 || len(executor.cleanupIdentities) != 1 {
		t.Fatalf("executor=%#v", executor)
	}
	identity := executor.cleanupIdentities[0]
	if identity.ObjectUID != string(task.UID) || identity.Logical.Namespace != task.Namespace || identity.Logical.Name != task.Name {
		t.Fatalf("cleanup identity=%#v", identity)
	}
}

func TestHostValidationRejectsMalformedClaimant(t *testing.T) {
	task := hostTask()
	host, _, _ := newTestHost(t, task)
	for _, claimant := range []string{"", "runtime", "/Fake", "cluster/", "cluster/Fake/extra"} {
		host.Claimant = claimant
		if err := host.Validate(); err == nil {
			t.Fatalf("claimant %q was accepted", claimant)
		}
	}
}

func TestHostSerializesSameTaskKey(t *testing.T) {
	task := claimedHostTask()
	host, _, _ := newTestHost(t, task)
	lifecycle := &blockingLifecycle{entered: make(chan struct{}, 2), release: make(chan struct{})}
	host.Bundle.Lifecycle = lifecycle
	key := client.ObjectKeyFromObject(task)
	errors := make(chan error, 2)
	go func() { _, err := host.Reconcile(context.Background(), key); errors <- err }()
	<-lifecycle.entered
	go func() { _, err := host.Reconcile(context.Background(), key); errors <- err }()
	select {
	case <-lifecycle.entered:
		t.Fatal("same Task key reconciled concurrently")
	case <-time.After(30 * time.Millisecond):
	}
	lifecycle.release <- struct{}{}
	<-lifecycle.entered
	lifecycle.release <- struct{}{}
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
}

func TestHostAllowsDifferentTaskKeys(t *testing.T) {
	first := claimedHostTask()
	second := claimedHostTask()
	second.Name = "other"
	second.UID = "other-uid"
	host, _, kube := newTestHost(t, first)
	if err := kube.Create(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	lifecycle := &blockingLifecycle{entered: make(chan struct{}, 2), release: make(chan struct{}, 2)}
	host.Bundle.Lifecycle = lifecycle
	errors := make(chan error, 2)
	go func() {
		_, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(first))
		errors <- err
	}()
	go func() {
		_, err := host.Reconcile(context.Background(), client.ObjectKeyFromObject(second))
		errors <- err
	}()
	for range 2 {
		select {
		case <-lifecycle.entered:
		case <-time.After(time.Second):
			t.Fatal("different Task keys blocked each other")
		}
	}
	lifecycle.release <- struct{}{}
	lifecycle.release <- struct{}{}
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
}

func newTestHost(t *testing.T, task *rlarkv1alpha1.Task) (*Host, *testExecutor, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	executor := &testExecutor{cleanupComplete: true}
	registry := &implementationRegistry{adapters: map[ImplementationKey]workload.Adapter{
		{WorkloadType: "FakeWorkload", WorkloadVersion: "v1", RuntimeKind: "Fake"}: fakeAdapter{},
		{WorkloadType: "OldWorkload", WorkloadVersion: "v1", RuntimeKind: "Fake"}:  fakeAdapter{id: "OldWorkload"},
	}}
	bundle := Bundle{RuntimeSelector: coordination.RuntimeSelector{ID: "fake", Kind: "Fake"}, Decoder: TaskDecoderFunc(func(task *rlarkv1alpha1.Task) (coordination.TaskSnapshot, error) {
		return coordination.TaskSnapshot{
			Identity: agentruntime.TaskIdentity{
				Logical: agentruntime.LogicalTaskID{
					Namespace: task.Namespace,
					Name:      task.Name,
				},
				ObjectUID:  string(task.UID),
				Generation: task.Generation,
			},
			WorkloadSelector: coordination.WorkloadSelector{Type: "FakeWorkload", Version: "v1"},
			CarrierSelector:  coordination.CarrierSelector{Type: "FakeCarrier", Version: "v1"},
		}, nil
	}), Executor: executor, Implementations: registry, Lifecycle: testLifecycle{}}
	return &Host{Tasks: kube, Bundle: bundle, Eligibility: testEligibility{}, Status: KubernetesStatusWriter{Client: kube}, Claimant: "cluster/Fake"}, executor, kube
}

func hostTask() *rlarkv1alpha1.Task {
	return &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "cluster", UID: "uid", Generation: 2}, Spec: rlarkv1alpha1.TaskSpec{Runtime: rlarkv1alpha1.TaskRuntimeSpec{Kind: "Fake"}}}
}

func claimedHostTask() *rlarkv1alpha1.Task {
	task := hostTask()
	task.Finalizers = []string{TaskFinalizer}
	task.Annotations = map[string]string{
		TaskClaimantAnnotation:              "cluster/Fake",
		TaskClaimedCarrierTypeAnnotation:    "FakeCarrier",
		TaskClaimedCarrierVersionAnnotation: "v1",
	}
	return task
}

func assertHostClaimPreserved(t *testing.T, kube client.Client, task *rlarkv1alpha1.Task) {
	t.Helper()
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if !contains(current.Finalizers, TaskFinalizer) || current.Annotations[TaskClaimantAnnotation] != "cluster/Fake" {
		t.Fatalf("claim was not preserved: %#v", current.ObjectMeta)
	}
}

type testEligibility struct{}

func (testEligibility) Matches(task *rlarkv1alpha1.Task) bool {
	return task.Spec.Runtime.Kind == "Fake"
}
func (testEligibility) ValidationMessage(*rlarkv1alpha1.Task) string { return "" }

func TestHostShouldReconcileUsesEffectiveClaimAndEligibility(t *testing.T) {
	host := &Host{Claimant: "cluster/Fake", Eligibility: testEligibility{}}
	tests := []struct {
		name   string
		mutate func(*rlarkv1alpha1.Task)
		want   bool
	}{
		{name: "unclaimed eligible", want: true},
		{name: "unclaimed ineligible", mutate: func(task *rlarkv1alpha1.Task) { task.Spec.Runtime.Kind = "Other" }, want: false},
		{name: "annotation without finalizer", mutate: func(task *rlarkv1alpha1.Task) {
			task.Spec.Runtime.Kind = "Other"
			task.Annotations = map[string]string{TaskClaimantAnnotation: host.Claimant}
		}, want: false},
		{name: "own claim despite selector drift", mutate: func(task *rlarkv1alpha1.Task) {
			task.Finalizers = []string{TaskFinalizer}
			task.Spec.Runtime.Kind = "Other"
			task.Annotations = map[string]string{TaskClaimantAnnotation: host.Claimant}
		}, want: true},
		{name: "foreign claim", mutate: func(task *rlarkv1alpha1.Task) {
			task.Finalizers = []string{TaskFinalizer}
			task.Annotations = map[string]string{TaskClaimantAnnotation: "other/Fake"}
		}, want: false},
		{name: "legacy three-part own claim", mutate: func(task *rlarkv1alpha1.Task) {
			task.Finalizers = []string{TaskFinalizer}
			task.Annotations = map[string]string{TaskClaimantAnnotation: "cluster/Fake/legacy-domain"}
		}, want: true},
		{name: "legacy finalizer eligible", mutate: func(task *rlarkv1alpha1.Task) {
			task.Finalizers = []string{TaskFinalizer}
			task.Annotations = map[string]string{}
		}, want: true},
		{name: "legacy finalizer deletion", mutate: func(task *rlarkv1alpha1.Task) {
			task.Finalizers = []string{TaskFinalizer}
			task.Spec.Runtime.Kind = "Other"
			task.DeletionTimestamp = &metav1.Time{Time: time.Unix(100, 0)}
		}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			task := hostTask()
			if test.mutate != nil {
				test.mutate(task)
			}
			if got := host.ShouldReconcile(task); got != test.want {
				t.Fatalf("ShouldReconcile()=%v, want %v for metadata=%#v", got, test.want, task.ObjectMeta)
			}
		})
	}
}

type panickingHostObserver struct{}

func (panickingHostObserver) Start(ctx context.Context, _ observability.Reconcile) (context.Context, observability.Scope) {
	return ctx, panickingHostScope{}
}

type panickingHostScope struct{}

func (panickingHostScope) Event(observability.Event) { panic("observer event") }
func (panickingHostScope) End(observability.Result)  { panic("observer end") }

type fixedEligibility struct {
	matches    bool
	validation string
}

func (e fixedEligibility) Matches(*rlarkv1alpha1.Task) bool             { return e.matches }
func (e fixedEligibility) ValidationMessage(*rlarkv1alpha1.Task) string { return e.validation }

type statusWriterFunc func(context.Context, *rlarkv1alpha1.Task, StatusIntent) error

func (f statusWriterFunc) WriteStatus(ctx context.Context, task *rlarkv1alpha1.Task, intent StatusIntent) error {
	return f(ctx, task, intent)
}

type testLifecycle struct{}

func (testLifecycle) BeforeReconcile(context.Context, LifecycleInput) (LifecycleDecision, error) {
	return LifecycleDecision{Action: LifecycleProceed}, nil
}

type fixedLifecycle struct{ decision LifecycleDecision }

type mutatingLifecycle struct {
	mutate func(*rlarkv1alpha1.Task)
}

func (l mutatingLifecycle) BeforeReconcile(_ context.Context, input LifecycleInput) (LifecycleDecision, error) {
	task, ok := input.External.(*rlarkv1alpha1.Task)
	if !ok {
		return LifecycleDecision{}, errors.New("expected Task lifecycle input")
	}
	l.mutate(task)
	return LifecycleDecision{Action: LifecycleProceed}, nil
}

func (mutatingLifecycle) AfterReconcile(context.Context, LifecycleInput) (*StatusIntent, error) {
	return nil, nil
}

func (l fixedLifecycle) BeforeReconcile(context.Context, LifecycleInput) (LifecycleDecision, error) {
	return l.decision, nil
}

type blockingLifecycle struct {
	entered chan struct{}
	release chan struct{}
}

func (l *blockingLifecycle) BeforeReconcile(context.Context, LifecycleInput) (LifecycleDecision, error) {
	l.entered <- struct{}{}
	<-l.release
	return LifecycleDecision{Action: LifecycleProceed}, nil
}
func (*blockingLifecycle) AfterReconcile(context.Context, LifecycleInput) (*StatusIntent, error) {
	return nil, nil
}
func (fixedLifecycle) AfterReconcile(context.Context, LifecycleInput) (*StatusIntent, error) {
	return nil, nil
}
func (testLifecycle) AfterReconcile(context.Context, LifecycleInput) (*StatusIntent, error) {
	return nil, nil
}

type testExecutor struct {
	mu                   sync.Mutex
	reconciles, cleanups int
	cleanupComplete      bool
	cleanupErr           error
	cleanupIdentities    []agentruntime.TaskIdentity
	cleanupSelectors     []coordination.WorkloadSelector
	snapshots            []coordination.TaskSnapshot
	onCleanup            func()
}

type fakeAdapter struct{ id workload.TypeID }

func (a fakeAdapter) Info(context.Context) (workload.TypeInfo, error) {
	id := a.id
	if id == "" {
		id = "FakeWorkload"
	}
	return workload.TypeInfo{ID: id, Version: "v1", SupportedRuntimeKinds: []agentruntime.Kind{"Fake"}}, nil
}

func (fakeAdapter) Compile(context.Context, workload.CompileRequest) (executionplan.Plan, error) {
	return executionplan.Plan{}, nil
}

func (fakeAdapter) Observe(context.Context, workload.ObserveRequest) (workload.Observation, error) {
	return workload.Observation{}, nil
}

func (e *testExecutor) Reconcile(_ context.Context, snapshot coordination.TaskSnapshot) (coordinator.ReconcileResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reconciles++
	e.snapshots = append(e.snapshots, snapshot)
	return coordinator.ReconcileResult{}, nil
}
func (e *testExecutor) CleanupTask(_ context.Context, identity agentruntime.TaskIdentity, selector coordination.WorkloadSelector, _ coordination.CarrierSelector) (coordinator.CleanupResult, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.cleanups++
	e.cleanupIdentities = append(e.cleanupIdentities, identity)
	e.cleanupSelectors = append(e.cleanupSelectors, selector)
	if e.onCleanup != nil {
		e.onCleanup()
	}
	return coordinator.CleanupResult{Complete: e.cleanupComplete}, e.cleanupErr
}
func (*testExecutor) ObserveRoot(context.Context, coordination.TaskSnapshot, agentruntime.ObjectRef) (coordinator.ObserveResult, error) {
	return coordinator.ObserveResult{}, nil
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

type conflictOncePatchClient struct {
	client.Client
	conflict bool
}

type optimisticLockPatchClient struct {
	client.Client
	sawResourceVersion     bool
	replacedFinalizerTable bool
	patchData              string
}

func (c *optimisticLockPatchClient) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.PatchOption) error {
	data, err := patch.Data(object)
	if err != nil {
		return err
	}
	c.patchData = string(data)
	c.sawResourceVersion = strings.Contains(c.patchData, `"path":"/metadata/resourceVersion"`)
	c.replacedFinalizerTable = strings.Contains(c.patchData, `"path":"/metadata/finalizers","value":[`) && strings.Contains(c.patchData, "other.example/finalizer")
	return c.Client.Patch(ctx, object, patch, options...)
}

func (c *conflictOncePatchClient) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.PatchOption) error {
	if c.conflict {
		c.conflict = false
		return apierrors.NewConflict(schema.GroupResource{Group: rlarkv1alpha1.GroupVersion.Group, Resource: "tasks"}, object.GetName(), errors.New("simulated conflict"))
	}
	return c.Client.Patch(ctx, object, patch, options...)
}
