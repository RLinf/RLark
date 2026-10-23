package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/observability"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
)

const (
	// TaskFinalizer is owned exclusively by Host cleanup reconciliation.
	TaskFinalizer = common.TaskAgentCleanupFinalizer
	// TaskClaimantAnnotation records the stable Agent scope owning a Task.
	TaskClaimantAnnotation = "rlark.io/management-task-claimant"
	// TaskClaimedWorkloadTypeAnnotation freezes the Workload implementation selected at claim time.
	TaskClaimedWorkloadTypeAnnotation = "execution.rlark.io/claimed-workload-type"
	// TaskClaimedWorkloadVersionAnnotation freezes the Workload contract selected at claim time.
	TaskClaimedWorkloadVersionAnnotation = "execution.rlark.io/claimed-workload-version"
	// TaskClaimedCarrierTypeAnnotation freezes the Runtime carrier selected at claim time.
	TaskClaimedCarrierTypeAnnotation = "execution.rlark.io/claimed-carrier-type"
	// TaskClaimedCarrierVersionAnnotation freezes the carrier contract selected at claim time.
	TaskClaimedCarrierVersionAnnotation = "execution.rlark.io/claimed-carrier-version"
	legacyTaskClaimedDomainAnnotation   = "rlark.io/management-task-claimed-domain"
)

// TaskDecoderFunc adapts a function to TaskDecoder.
type TaskDecoderFunc func(*rlarkv1alpha1.Task) (coordination.TaskSnapshot, error)

// Decode implements TaskDecoder and rejects an unexpected external Task type.
func (f TaskDecoderFunc) Decode(value any) (coordination.TaskSnapshot, error) {
	task, ok := value.(*rlarkv1alpha1.Task)
	if !ok {
		return coordination.TaskSnapshot{}, fmt.Errorf("unsupported Task value %T", value)
	}
	return f(task)
}

// Eligibility classifies whether an external Task belongs to this Bundle and
// whether its current representation can be reconciled.
type Eligibility interface {
	Matches(*rlarkv1alpha1.Task) bool
	ValidationMessage(*rlarkv1alpha1.Task) string
}

// StatusWriter is the single Task Status mutation boundary used by pull and
// targeted observation reconcilers.
type StatusWriter interface {
	WriteStatus(context.Context, *rlarkv1alpha1.Task, StatusIntent) error
}

// KubernetesStatusWriter patches only the compatibility status fields owned by
// the Task Host and fences stale Task incarnations and generations.
type KubernetesStatusWriter struct {
	Client client.Client
}

// WriteStatus applies one fenced StatusIntent.
func (w KubernetesStatusWriter) WriteStatus(ctx context.Context, task *rlarkv1alpha1.Task, intent StatusIntent) error {
	if w.Client == nil {
		return errors.New("management client is required")
	}
	if intent.TaskUID != "" && intent.TaskUID != string(task.UID) {
		return nil
	}
	if intent.Generation != 0 && intent.Generation != task.Generation {
		return nil
	}
	desired := task.DeepCopy()
	if intent.Phase != "" {
		desired.Status.Phase = rlarkv1alpha1.TaskPhase(intent.Phase)
	}
	if intent.MessageSet {
		desired.Status.Message = intent.Message
	}
	if intent.ObservedNodesSet {
		desired.Status.ObservedNodes = slices.Clone(intent.ObservedNodes)
	}
	if desired.Status.Phase == rlarkv1alpha1.TaskPhaseStopped {
		desired.Status.PullProgress = nil
		desired.Status.Events = nil
	}
	// ObservedGeneration is the data-plane controller's reconciliation contract:
	// it records the Spec generation that this observed Phase / Message /
	// ObservedNodes snapshot corresponds to. It is advanced on every accepted
	// write, including writes whose observed values happen to be unchanged,
	// otherwise a Spec-only change would leave the reported status silently stale.
	desired.Status.ObservedGeneration = task.Generation
	for _, mutation := range intent.Conditions {
		if mutation.Remove {
			apimeta.RemoveStatusCondition(&desired.Status.Conditions, mutation.Condition.Type)
			continue
		}
		condition := mutation.Condition
		if condition.ObservedGeneration == 0 {
			condition.ObservedGeneration = task.Generation
		}
		apimeta.SetStatusCondition(&desired.Status.Conditions, condition)
	}
	if reflect.DeepEqual(task.Status, desired.Status) {
		return nil
	}
	if err := w.Client.Status().Patch(ctx, desired, client.MergeFrom(task)); err != nil {
		return fmt.Errorf("patch Task status: %w", err)
	}
	task.Status = desired.Status
	return nil
}

// Host owns Task claim, finalizer, lifecycle dispatch, and Status. Runtime
// objects are mutated only through Executor.
type Host struct {
	Tasks       client.Client
	Bundle      Bundle
	Eligibility Eligibility
	Status      StatusWriter
	Claimant    string
	locks       keyedLocks
}

// Validate rejects incomplete production assembly before reconciliation starts.
func (h *Host) Validate() error {
	if h.Tasks == nil {
		return errors.New("management Task client is required")
	}
	if err := h.Bundle.Validate(); err != nil {
		return err
	}
	if h.Eligibility == nil {
		return errors.New("task eligibility is required")
	}
	if h.Status == nil {
		return errors.New("task status writer is required")
	}
	if h.Claimant == "" {
		return errors.New("task claimant identity is required")
	}
	parts := strings.Split(h.Claimant, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return errors.New("task claimant identity must be namespace/runtime-kind")
	}
	return nil
}

// ShouldReconcile reports whether a conservative trigger source should wake
// this Host for the current Task snapshot. It is a scheduling hint only: Host
// reconciliation re-reads the Task and remains the authority for claim,
// eligibility, validation, and cleanup decisions.
func (h *Host) ShouldReconcile(task *rlarkv1alpha1.Task) bool {
	if task == nil || h.Eligibility == nil || h.Claimant == "" {
		return false
	}
	claimed := slices.Contains(task.Finalizers, TaskFinalizer)
	if !claimed {
		// A claimant annotation without the Host finalizer is not an effective
		// claim and must not override current desired-state eligibility.
		return h.Eligibility.Matches(task)
	}
	claimant := task.Annotations[TaskClaimantAnnotation]
	if claimant != "" {
		// A valid claim remains schedulable by its owning Host even after the
		// mutable selector drifts, and while deletion is in progress.
		return h.isClaimant(task)
	}
	// A claimant-less legacy finalizer can be adopted only when deletion
	// requires cleanup or this Host can safely realize the current Task.
	return task.DeletionTimestamp != nil ||
		(h.Eligibility.Matches(task) && h.Eligibility.ValidationMessage(task) == "")
}

// Reconcile reads current Task state and drives the Runtime-neutral lifecycle.
func (h *Host) Reconcile(ctx context.Context, key types.NamespacedName) (result Result, finalErr error) {
	ctx, scope := h.startObservation(ctx, key)
	defer func() { scope.End(hostObservationResult(result, finalErr)) }()
	// 1. Serialize one management Task by namespace/name, then read its current
	// API state. A missing Task is dropped because identity-based cleanup cannot
	// be proven from the reconcile key alone.
	unlock := h.locks.lock(key.String())
	defer unlock()

	var task rlarkv1alpha1.Task
	if err := h.Tasks.Get(ctx, key, &task); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return Result{Outcome: OutcomeDropped, Reason: "TaskNotFound"}, nil
		}
		return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryTransient}}, err
	}
	scope.Event(observability.Event{Name: "task.read", Attributes: map[string]string{"result": "found"}})
	observability.RecordDetailed(scope, "task.snapshot", observability.PayloadTaskSnapshot, taskSnapshotProjection(&task), nil)

	// 2. Classify ownership and eligibility before making external changes.
	// The finalizer establishes an existing claim, the claimant identifies its
	// owning Host.
	claimed := slices.Contains(task.Finalizers, TaskFinalizer)
	claimantMissing := claimed && task.Annotations[TaskClaimantAnnotation] == ""
	isClaimant := claimed && !claimantMissing && h.isClaimant(&task)
	selectorChanged := claimed && !claimantMissing && isClaimant && !h.Eligibility.Matches(&task)
	matches := h.Eligibility.Matches(&task)
	validation := h.Eligibility.ValidationMessage(&task)
	canRealize := matches && validation == ""

	// Adopt an unambiguous legacy finalizer only for deletion or when this Host
	// can realize the Task. Other claimant-less Tasks remain untouched.
	if claimantMissing && (task.DeletionTimestamp != nil || canRealize) {
		var selector coordination.WorkloadSelector
		var carrier coordination.CarrierSelector
		if snapshot, err := h.decodeRegisteredTask(&task); err == nil {
			selector = snapshot.WorkloadSelector
			carrier = snapshot.CarrierSelector
		}
		if err := h.persistClaim(ctx, &task, selector, carrier, false); err != nil {
			return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryConflict}}, err
		}
		scope.Event(observability.Event{Name: "task.claim", Attributes: map[string]string{"action": "adopt_legacy", "result": "persisted"}})
		claimed, isClaimant = true, true
	}
	// A claimed Runtime selector is immutable. Wait without cleaning
	// the old Runtime or partially applying other Spec changes from this generation.
	if selectorChanged && task.DeletionTimestamp == nil {
		reason := "RuntimeSelectorChanged"
		message := "Task Runtime selector changed after claim; restore the original Runtime or delete and recreate the Task"
		intent := IntentForTask(&task, task.Status.Phase, message, nil, StatusPriorityInvalid)
		if err := h.Status.WriteStatus(ctx, &task, intent); err != nil {
			return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryConflict}}, err
		}
		return Result{Outcome: OutcomeWaitingForChange, Retry: RetryDirective{Class: RetryOnChange}, Reason: reason, Status: &intent}, nil
	}

	// 3. Deletion bypasses normal validation and compilation. Cleanup uses the
	// persisted Runtime and Task UID, then releases Host metadata only
	// after the Runtime reports that owned objects are absent.
	if task.DeletionTimestamp != nil {
		if !isClaimant {
			return Result{Outcome: OutcomeWaitingForChange, Retry: RetryDirective{Class: RetryOnChange}, Reason: "TaskNotClaimedByHost"}, nil
		}
		result, err := h.cleanup(ctx, &task, true)
		if err == nil {
			result.Reason = "TaskDeletionCleanup"
		}
		return result, err
	}
	if !matches || validation != "" {
		// Failure to validate a claimed Task is not authorization to destroy its
		// running Runtime. Preserve claim and workload until Spec or implementation
		// support is restored, or until deletion explicitly requests cleanup.
		reason := "TaskNotEligible"
		if claimed {
			reason = "ClaimedTaskInvalid"
		}
		return Result{Outcome: OutcomeWaitingForChange, Retry: RetryDirective{Class: RetryOnChange}, Reason: reason}, nil
	}

	// 4. Prove that the exact Workload type/version and Runtime kind are
	// registered before claim or execution. Decode/registration failure is not
	// evidence that an existing workload should be destroyed: an Agent upgrade,
	// configuration error, or temporarily unavailable implementation may cause it.
	// Preserve any existing claim and Runtime and wait for a relevant change.
	snapshot, err := h.decodeRegisteredTask(&task)
	if err != nil {
		reason := "TaskDecodeFailed"
		if errors.Is(err, ErrImplementationNotFound) {
			reason = "ImplementationUnavailable"
		}
		if claimed {
			reason = "Claimed" + reason
		}
		return Result{Outcome: OutcomeWaitingForChange, Retry: RetryDirective{Class: RetryOnChange}, Reason: reason}, nil
	}

	// Persist the claim before any Runtime effect. Re-read immediately so the
	// remaining flow uses authoritative metadata and survives a concurrent
	// update, deletion, or process restart after the claim write.
	if !claimed {
		if err := h.persistClaim(ctx, &task, snapshot.WorkloadSelector, snapshot.CarrierSelector, true); err != nil {
			return retryResult(err), err
		}
		scope.Event(observability.Event{Name: "task.claim", Attributes: map[string]string{"action": "claim", "result": "persisted"}})
		if err := h.Tasks.Get(ctx, key, &task); err != nil {
			if client.IgnoreNotFound(err) == nil {
				return Result{Outcome: OutcomeDropped, Reason: "TaskDeletedDuringClaim"}, nil
			}
			return retryResult(err), fmt.Errorf("read claimed Task: %w", err)
		}
	} else if !h.isClaimant(&task) {
		return Result{Outcome: OutcomeWaitingForChange, Retry: RetryDirective{Class: RetryOnChange}, Reason: "ClaimedByOtherHost"}, nil
	}
	if claimed {
		claimedSelector, err := validatedClaimedWorkloadSelector(&task)
		if err != nil {
			return Result{Outcome: OutcomeBlocked, Retry: RetryDirective{Class: RetryOnChange}, Reason: "ClaimedWorkloadSelectorIncomplete"}, err
		}
		if claimedSelector.Type == "" {
			if err := h.persistClaim(ctx, &task, snapshot.WorkloadSelector, snapshot.CarrierSelector, false); err != nil {
				return retryResult(err), err
			}
			claimedSelector = snapshot.WorkloadSelector
		}
		if claimedSelector != snapshot.WorkloadSelector {
			return Result{Outcome: OutcomeWaitingForChange, Retry: RetryDirective{Class: RetryOnChange}, Reason: "WorkloadSelectorChanged"}, nil
		}
		claimedCarrier, err := validatedClaimedCarrierSelector(&task)
		if err != nil {
			return Result{Outcome: OutcomeBlocked, Retry: RetryDirective{Class: RetryOnChange}, Reason: "ClaimedCarrierSelectorIncomplete"}, err
		}
		if claimedCarrier.Type == "" {
			if err := h.persistClaim(ctx, &task, snapshot.WorkloadSelector, snapshot.CarrierSelector, false); err != nil {
				return retryResult(err), err
			}
			claimedCarrier = snapshot.CarrierSelector
		}
		if claimedCarrier != snapshot.CarrierSelector {
			return Result{Outcome: OutcomeWaitingForChange, Retry: RetryDirective{Class: RetryOnChange}, Reason: "CarrierSelectorChanged"}, nil
		}
	}
	if task.Spec.Suspended {
		cleaned, err := h.cleanup(ctx, &task, false)
		if err != nil || cleaned.Outcome != OutcomeSucceeded {
			return cleaned, err
		}
		intent := IntentForTask(&task, rlarkv1alpha1.TaskPhaseStopped, "", nil, StatusPriorityLifecycle)
		if err := h.Status.WriteStatus(ctx, &task, intent); err != nil {
			return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryConflict}}, err
		}
		return Result{Outcome: OutcomeWaitingForChange, Retry: RetryDirective{Class: RetryOnChange}, Reason: "TaskSuspended", Status: &intent}, nil
	}

	// 5. Run Runtime-specific lifecycle admission. Persist its Status before any
	// ordered follow-up effect, then either proceed, perform recoverable cleanup,
	// wait for input, or block on a stable contract violation.
	decision, err := h.Bundle.Lifecycle.BeforeReconcile(ctx, LifecycleInput{External: &task})
	if err != nil {
		return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryTransient}}, err
	}
	scope.Event(observability.Event{Name: "lifecycle.before_reconcile", Attributes: map[string]string{"action": string(decision.Action), "reason": decision.Reason}})
	if decision.Status != nil {
		if err := h.Status.WriteStatus(ctx, &task, *decision.Status); err != nil {
			return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryConflict}}, err
		}
		scope.Event(observability.Event{Name: "task.status", Attributes: map[string]string{"source": "lifecycle_before", "result": "persisted"}})
	}
	if decision.AfterStatus != nil {
		if err := decision.AfterStatus(ctx); err != nil {
			return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryTransient}, Status: decision.Status}, err
		}
	}
	switch decision.Action {
	case "", LifecycleProceed:
	case LifecycleCleanup:
		if decision.Reason == "" {
			decision.Reason = "LifecycleCleanup"
		}
		cleaned, err := h.cleanup(ctx, &task, false)
		if err != nil || cleaned.Outcome != OutcomeSucceeded {
			if err == nil {
				cleaned.Reason = decision.Reason
			}
			return cleaned, err
		}
		if decision.AfterCleanupStatus != nil {
			if err := h.Status.WriteStatus(ctx, &task, *decision.AfterCleanupStatus); err != nil {
				return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryConflict}}, err
			}
			scope.Event(observability.Event{Name: "task.status", Attributes: map[string]string{"source": "lifecycle_cleanup", "result": "persisted"}})
		}
		if decision.AfterCleanup == LifecycleWait {
			return Result{Outcome: OutcomeWaitingForChange, Retry: RetryDirective{Class: RetryOnChange}, Reason: decision.Reason, Status: decision.AfterCleanupStatus}, nil
		}
	case LifecycleWait:
		if decision.Reason == "" {
			decision.Reason = "LifecycleWait"
		}
		return Result{Outcome: OutcomeWaitingForChange, Retry: decision.Retry, Reason: decision.Reason, Status: decision.Status}, nil
	case LifecycleBlock:
		if decision.Reason == "" {
			decision.Reason = "LifecycleBlocked"
		}
		return Result{Outcome: OutcomeBlocked, Retry: decision.Retry, Reason: decision.Reason, Status: decision.Status}, nil
	default:
		return Result{Outcome: OutcomeBlocked}, fmt.Errorf("unknown lifecycle action %q", decision.Action)
	}

	// 6. Decode again after claim and lifecycle processing so the Executor sees
	// the current Task plus compatibility mutations such as image pull Secrets.
	// The Coordinator/Provider path then observes, diffs, and applies Runtime
	// objects idempotently for the frozen Runtime selector.
	snapshot, err = h.decodeRegisteredTask(&task)
	if err != nil {
		return Result{Outcome: OutcomeBlocked, Retry: RetryDirective{Class: RetryOnChange}}, err
	}
	snapshot.RuntimeSelector = h.Bundle.RuntimeSelector
	executionResult, err := h.Bundle.Executor.Reconcile(ctx, snapshot)
	if err != nil {
		return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryTransient}}, err
	}
	scope.Event(observability.Event{Name: "executor.reconcile", Attributes: map[string]string{"result": "succeeded", "applied": fmt.Sprintf("%d", len(executionResult.Applied)), "deleted": fmt.Sprintf("%d", len(executionResult.Deleted))}})
	if h.Bundle.AfterExecution != nil {
		func() {
			defer func() { _ = recover() }()
			h.Bundle.AfterExecution(snapshot, executionResult)
		}()
	}

	// 7. Project the Executor observation and lifecycle post-processing into
	// fenced Status intents. Merge them by priority and write Status once so
	// independent producers cannot overwrite each other's owned result.
	var intent *StatusIntent
	if h.Bundle.StatusFromReconcile != nil {
		intent = h.Bundle.StatusFromReconcile(executionResult, snapshot)
	}
	after, err := h.Bundle.Lifecycle.AfterReconcile(ctx, LifecycleInput{External: &task, Snapshot: snapshot})
	if err != nil {
		return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryTransient}}, err
	}
	scope.Event(observability.Event{Name: "lifecycle.after_reconcile", Attributes: map[string]string{"status": boolString(after != nil)}})
	intents := make([]StatusIntent, 0, 2)
	if intent != nil {
		intents = append(intents, *intent)
	}
	if after != nil {
		intents = append(intents, *after)
	}
	merged := MergeStatusIntents(string(task.UID), task.Generation, intents...)
	if merged != nil {
		if err := h.Status.WriteStatus(ctx, &task, *merged); err != nil {
			return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryConflict}}, err
		}
		scope.Event(observability.Event{Name: "task.status", Attributes: map[string]string{"source": "merged", "result": "persisted"}})
	}
	return Result{Outcome: OutcomeSucceeded, Retry: RetryDirective{Class: RetryNone}, Reason: "Reconciled", Status: merged}, nil
}

func (h *Host) startObservation(ctx context.Context, key types.NamespacedName) (context.Context, observability.Scope) {
	observer := h.Bundle.Observer
	if observer == nil {
		observer = observability.Noop()
	}
	return observer.Start(ctx, observability.Reconcile{Name: "host.reconcile", TaskKey: key.String(), RuntimeID: string(h.Bundle.RuntimeSelector.ID)})
}

func hostObservationResult(result Result, err error) observability.Result {
	observed := observability.Result{Outcome: string(result.Outcome), Reason: result.Reason}
	if observed.Outcome == "" {
		observed.Outcome = "Failed"
	}
	if err != nil {
		observed.FaultCode = string(fault.Internal)
		var domain *fault.Error
		if errors.As(err, &domain) {
			observed.FaultCode = string(domain.Code)
		} else if apierrors.IsConflict(err) {
			observed.FaultCode = string(fault.Conflict)
		}
	}
	return observed
}

type taskSnapshotDiagnostic struct {
	Namespace  string `json:"namespace"`
	Name       string `json:"name"`
	UID        string `json:"uid"`
	Generation int64  `json:"generation"`
	Deleting   bool   `json:"deleting"`
	Claimed    bool   `json:"claimed"`
}

func taskSnapshotProjection(task *rlarkv1alpha1.Task) taskSnapshotDiagnostic {
	return taskSnapshotDiagnostic{
		Namespace: task.Namespace, Name: task.Name, UID: string(task.UID), Generation: task.Generation,
		Deleting: task.DeletionTimestamp != nil,
		Claimed:  slices.Contains(task.Finalizers, TaskFinalizer),
	}
}

func boolString(value bool) string {
	if value {
		return "present"
	}
	return "absent"
}

func (h *Host) decodeRegisteredTask(task *rlarkv1alpha1.Task) (coordination.TaskSnapshot, error) {
	snapshot, err := h.Bundle.Decoder.Decode(task)
	if err != nil {
		return coordination.TaskSnapshot{}, err
	}
	key := ImplementationKey{
		WorkloadType:    workload.TypeID(snapshot.WorkloadSelector.Type),
		WorkloadVersion: snapshot.WorkloadSelector.Version,
		RuntimeKind:     h.Bundle.RuntimeSelector.Kind,
	}
	if _, err := h.Bundle.Implementations.Select(key); err != nil {
		return coordination.TaskSnapshot{}, err
	}
	return snapshot, nil
}

type keyedLocks struct {
	mu      sync.Mutex
	entries map[string]*keyedLock
}

type keyedLock struct {
	mu   sync.Mutex
	refs int
}

func (l *keyedLocks) lock(key string) func() {
	l.mu.Lock()
	if l.entries == nil {
		l.entries = make(map[string]*keyedLock)
	}
	entry := l.entries[key]
	if entry == nil {
		entry = &keyedLock{}
		l.entries[key] = entry
	}
	entry.refs++
	l.mu.Unlock()

	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		l.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(l.entries, key)
		}
		l.mu.Unlock()
	}
}

func (h *Host) cleanup(ctx context.Context, task *rlarkv1alpha1.Task, release bool) (Result, error) {
	identity := agentruntime.TaskIdentity{
		Logical: agentruntime.LogicalTaskID{
			Namespace: task.Namespace,
			Name:      task.Name,
		},
		ObjectUID:  string(task.UID),
		Generation: task.Generation,
	}
	selector, err := validatedClaimedWorkloadSelector(task)
	if err != nil {
		return Result{Outcome: OutcomeBlocked, Retry: RetryDirective{Class: RetryOnChange}, Reason: "ClaimedWorkloadSelectorIncomplete"}, err
	}
	if selector.Type == "" && selector.Version == "" && task.DeletionTimestamp != nil {
		// Claims written before the frozen Workload selector was introduced may
		// enter deletion before this Host can normalize their Runtime root. Decode
		// only the implementation selector from the still-finalized Task so cleanup
		// can use the Adapter's identity inventory; never compile desired resources
		// or persist current Spec-derived identity during deletion.
		snapshot, err := h.decodeRegisteredTask(task)
		if err != nil {
			return Result{Outcome: OutcomeBlocked, Retry: RetryDirective{Class: RetryOnChange}, Reason: "LegacyCleanupSelectorUnavailable"}, fmt.Errorf("recover deleting legacy Task Workload selector: %w", err)
		}
		selector = snapshot.WorkloadSelector
	}
	carrier, err := validatedClaimedCarrierSelector(task)
	if err != nil {
		return Result{Outcome: OutcomeBlocked, Retry: RetryDirective{Class: RetryOnChange}, Reason: "ClaimedCarrierSelectorIncomplete"}, err
	}
	if carrier.Type == "" && carrier.Version == "" {
		snapshot, decodeErr := h.decodeRegisteredTask(task)
		if decodeErr != nil {
			return Result{Outcome: OutcomeBlocked, Retry: RetryDirective{Class: RetryOnChange}, Reason: "CleanupCarrierSelectorUnavailable"}, fmt.Errorf("recover deleting Task carrier selector: %w", decodeErr)
		}
		carrier = snapshot.CarrierSelector
	}
	result, err := h.Bundle.Executor.CleanupTask(ctx, identity, selector, carrier)
	if err != nil {
		return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: RetryTransient}}, err
	}
	if !result.Complete {
		return Result{Outcome: OutcomeCleanupInProgress, Retry: RetryDirective{Class: RetryInProgress}, Reason: "WaitingForRuntimeCleanup"}, nil
	}
	if release {
		patch, err := releaseClaimPatch(task)
		if err != nil {
			return Result{Outcome: OutcomeBlocked}, fmt.Errorf("build Task claim release: %w", err)
		}
		if err := h.Tasks.Patch(ctx, task, patch); err != nil {
			err = h.classifyMetadataPatchError(ctx, task, err)
			return retryResult(err), fmt.Errorf("release Task claim: %w", err)
		}
	}
	return Result{Outcome: OutcomeSucceeded, Retry: RetryDirective{Class: RetryNone}, Reason: "CleanupComplete"}, nil
}

func retryResult(err error) Result {
	class := RetryTransient
	if apierrors.IsConflict(err) {
		class = RetryConflict
	}
	return Result{Outcome: OutcomeRetrying, Retry: RetryDirective{Class: class}}
}

func (h *Host) persistClaim(ctx context.Context, task *rlarkv1alpha1.Task, workloadSelector coordination.WorkloadSelector, carrierSelector coordination.CarrierSelector, addFinalizer bool) error {
	patch, err := persistClaimPatch(task, h.Claimant, workloadSelector, carrierSelector, addFinalizer)
	if err != nil {
		return fmt.Errorf("build Task claim patch: %w", err)
	}
	if err := h.Tasks.Patch(ctx, task, patch); err != nil {
		err = h.classifyMetadataPatchError(ctx, task, err)
		return fmt.Errorf("persist Task claim: %w", err)
	}
	return nil
}

func (h *Host) classifyMetadataPatchError(ctx context.Context, task *rlarkv1alpha1.Task, patchErr error) error {
	if apierrors.IsConflict(patchErr) || !apierrors.IsInvalid(patchErr) {
		return patchErr
	}
	var current rlarkv1alpha1.Task
	if err := h.Tasks.Get(ctx, client.ObjectKeyFromObject(task), &current); err != nil {
		return patchErr
	}
	if current.ResourceVersion == task.ResourceVersion {
		return patchErr
	}
	return apierrors.NewConflict(
		rlarkv1alpha1.GroupVersion.WithResource("tasks").GroupResource(),
		task.Name,
		patchErr,
	)
}

type jsonPatchOperation struct {
	Operation string `json:"op"`
	Path      string `json:"path"`
	Value     any    `json:"value,omitempty"`
}

func persistClaimPatch(task *rlarkv1alpha1.Task, claimant string, workloadSelector coordination.WorkloadSelector, carrierSelector coordination.CarrierSelector, addFinalizer bool) (client.Patch, error) {
	operations := []jsonPatchOperation{{Operation: "test", Path: "/metadata/resourceVersion", Value: task.ResourceVersion}}
	if task.Annotations == nil {
		operations = append(operations, jsonPatchOperation{Operation: "add", Path: "/metadata/annotations", Value: map[string]string{}})
	}
	operations = append(operations, jsonPatchOperation{
		Operation: "add",
		Path:      "/metadata/annotations/" + escapeJSONPointer(TaskClaimantAnnotation),
		Value:     claimant,
	})
	claimedWorkloadType := task.Annotations[TaskClaimedWorkloadTypeAnnotation]
	claimedWorkloadVersion := task.Annotations[TaskClaimedWorkloadVersionAnnotation]
	if claimedWorkloadType == "" && claimedWorkloadVersion == "" && workloadSelector.Type != "" && workloadSelector.Version != "" {
		operations = append(operations,
			jsonPatchOperation{Operation: "add", Path: "/metadata/annotations/" + escapeJSONPointer(TaskClaimedWorkloadTypeAnnotation), Value: workloadSelector.Type},
			jsonPatchOperation{Operation: "add", Path: "/metadata/annotations/" + escapeJSONPointer(TaskClaimedWorkloadVersionAnnotation), Value: workloadSelector.Version},
		)
	}
	claimedCarrierType := task.Annotations[TaskClaimedCarrierTypeAnnotation]
	claimedCarrierVersion := task.Annotations[TaskClaimedCarrierVersionAnnotation]
	if claimedCarrierType == "" && claimedCarrierVersion == "" && carrierSelector.Type != "" && carrierSelector.Version != "" {
		operations = append(operations,
			jsonPatchOperation{Operation: "add", Path: "/metadata/annotations/" + escapeJSONPointer(TaskClaimedCarrierTypeAnnotation), Value: carrierSelector.Type},
			jsonPatchOperation{Operation: "add", Path: "/metadata/annotations/" + escapeJSONPointer(TaskClaimedCarrierVersionAnnotation), Value: carrierSelector.Version},
		)
	}
	if addFinalizer && !slices.Contains(task.Finalizers, TaskFinalizer) {
		if task.Finalizers == nil {
			operations = append(operations, jsonPatchOperation{Operation: "add", Path: "/metadata/finalizers", Value: []string{TaskFinalizer}})
		} else {
			operations = append(operations, jsonPatchOperation{Operation: "add", Path: "/metadata/finalizers/-", Value: TaskFinalizer})
		}
	}
	return rawJSONPatch(operations)
}

func releaseClaimPatch(task *rlarkv1alpha1.Task) (client.Patch, error) {
	operations := []jsonPatchOperation{{Operation: "test", Path: "/metadata/resourceVersion", Value: task.ResourceVersion}}
	for index, finalizer := range task.Finalizers {
		if finalizer == TaskFinalizer {
			path := fmt.Sprintf("/metadata/finalizers/%d", index)
			operations = append(operations,
				jsonPatchOperation{Operation: "test", Path: path, Value: TaskFinalizer},
				jsonPatchOperation{Operation: "remove", Path: path},
			)
			break
		}
	}
	if _, found := task.Annotations[TaskClaimantAnnotation]; found {
		operations = append(operations, jsonPatchOperation{Operation: "remove", Path: "/metadata/annotations/" + escapeJSONPointer(TaskClaimantAnnotation)})
	}
	if _, found := task.Annotations[legacyTaskClaimedDomainAnnotation]; found {
		operations = append(operations, jsonPatchOperation{Operation: "remove", Path: "/metadata/annotations/" + escapeJSONPointer(legacyTaskClaimedDomainAnnotation)})
	}
	if _, found := task.Annotations[TaskClaimedWorkloadTypeAnnotation]; found {
		operations = append(operations, jsonPatchOperation{Operation: "remove", Path: "/metadata/annotations/" + escapeJSONPointer(TaskClaimedWorkloadTypeAnnotation)})
	}
	if _, found := task.Annotations[TaskClaimedWorkloadVersionAnnotation]; found {
		operations = append(operations, jsonPatchOperation{Operation: "remove", Path: "/metadata/annotations/" + escapeJSONPointer(TaskClaimedWorkloadVersionAnnotation)})
	}
	if _, found := task.Annotations[TaskClaimedCarrierTypeAnnotation]; found {
		operations = append(operations, jsonPatchOperation{Operation: "remove", Path: "/metadata/annotations/" + escapeJSONPointer(TaskClaimedCarrierTypeAnnotation)})
	}
	if _, found := task.Annotations[TaskClaimedCarrierVersionAnnotation]; found {
		operations = append(operations, jsonPatchOperation{Operation: "remove", Path: "/metadata/annotations/" + escapeJSONPointer(TaskClaimedCarrierVersionAnnotation)})
	}
	return rawJSONPatch(operations)
}

func rawJSONPatch(operations []jsonPatchOperation) (client.Patch, error) {
	data, err := json.Marshal(operations)
	if err != nil {
		return nil, err
	}
	return client.RawPatch(types.JSONPatchType, data), nil
}

func escapeJSONPointer(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}

func (h *Host) isClaimant(task *rlarkv1alpha1.Task) bool {
	claimant := task.Annotations[TaskClaimantAnnotation]
	if claimant == h.Claimant {
		return true
	}
	parts := strings.Split(claimant, "/")
	want := strings.Split(h.Claimant, "/")
	return len(parts) == 3 && len(want) == 2 && parts[0] == want[0] && parts[1] == want[1]
}

func claimedWorkloadSelector(task *rlarkv1alpha1.Task) coordination.WorkloadSelector {
	return coordination.WorkloadSelector{
		Type:    task.Annotations[TaskClaimedWorkloadTypeAnnotation],
		Version: task.Annotations[TaskClaimedWorkloadVersionAnnotation],
	}
}

func validatedClaimedWorkloadSelector(task *rlarkv1alpha1.Task) (coordination.WorkloadSelector, error) {
	selector := claimedWorkloadSelector(task)
	if (selector.Type == "") != (selector.Version == "") {
		return coordination.WorkloadSelector{}, fmt.Errorf("claimed Workload selector is incomplete")
	}
	return selector, nil
}

func claimedCarrierSelector(task *rlarkv1alpha1.Task) coordination.CarrierSelector {
	return coordination.CarrierSelector{Type: task.Annotations[TaskClaimedCarrierTypeAnnotation], Version: task.Annotations[TaskClaimedCarrierVersionAnnotation]}
}

func validatedClaimedCarrierSelector(task *rlarkv1alpha1.Task) (coordination.CarrierSelector, error) {
	selector := claimedCarrierSelector(task)
	if (selector.Type == "") != (selector.Version == "") {
		return coordination.CarrierSelector{}, fmt.Errorf("claimed carrier selector is incomplete")
	}
	return selector, nil
}

// IntentForTask returns a fenced compatibility status intent.
func IntentForTask(task *rlarkv1alpha1.Task, phase rlarkv1alpha1.TaskPhase, message string, nodes []string, priority StatusPriority) StatusIntent {
	return StatusIntent{TaskUID: string(task.UID), Generation: task.Generation, Phase: string(phase), MessageSet: true, Message: message, ObservedNodesSet: true, ObservedNodes: slices.Clone(nodes), Priority: priority}
}

// ConditionIntent returns a fenced condition mutation intent.
func ConditionIntent(task *rlarkv1alpha1.Task, condition metav1.Condition, remove bool, priority StatusPriority) StatusIntent {
	return StatusIntent{TaskUID: string(task.UID), Generation: task.Generation, Conditions: []ConditionMutation{{Condition: condition, Remove: remove}}, Priority: priority}
}
