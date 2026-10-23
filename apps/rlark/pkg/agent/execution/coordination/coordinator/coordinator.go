package coordinator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/evidence"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/observability"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

// RuntimeRegistration binds advertised Runtime information to its Provider.
type RuntimeRegistration struct {
	Info       agentruntime.Info
	Provider   agentruntime.Provider
	Comparator ObjectComparator
}

// ObjectComparator applies Runtime-specific, effect-free desired-state
// comparison. Nil uses JSON semantic equality.
type ObjectComparator interface {
	Equal(agentruntime.TypedManifest, agentruntime.DesiredObject) (bool, error)
}

// ObjectComparatorFunc adapts a function to ObjectComparator.
type ObjectComparatorFunc func(agentruntime.TypedManifest, agentruntime.DesiredObject) (bool, error)

// Equal compares one observed manifest with its desired declaration.
func (f ObjectComparatorFunc) Equal(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
	return f(current, desired)
}

// WorkloadRegistration binds a versioned Workload Type to its Adapter.
type WorkloadRegistration struct {
	Type        workload.TypeID
	Version     string
	RuntimeKind agentruntime.Kind
	Adapter     workload.Adapter
}

// WorkloadImplementationSelector selects an explicitly registered Workload
// implementation for one exact Runtime kind.
type WorkloadImplementationSelector interface {
	SelectImplementation(workload.TypeID, string, agentruntime.Kind) (workload.Adapter, error)
}

// OperationKind identifies a Runtime mutation calculated by Evaluate.
type OperationKind string

const (
	// OperationCreate creates a desired object that is currently absent.
	OperationCreate OperationKind = "Create"
	// OperationUpdate moves an existing object to its desired state.
	OperationUpdate OperationKind = "Update"
	// OperationDelete removes an owned object from the Runtime.
	OperationDelete OperationKind = "Delete"
)

// Operation is one ordered, idempotent Runtime mutation.
type Operation struct {
	Kind           OperationKind
	LogicalName    string
	Desired        *agentruntime.DesiredObject
	Current        *agentruntime.ObjectRef
	IdempotencyKey string
}

// Result is a complete, non-mutating execution evaluation.
type Result struct {
	Runtime     agentruntime.Info
	Plan        executionplan.ExecutablePlan
	Managed     agentruntime.ObjectSnapshot
	Instances   agentruntime.InstanceSnapshot
	Observation workload.Observation
	Operations  []Operation
	Orphans     []agentruntime.ObjectRef
	Roots       []agentruntime.ObjectRef
	// CleanupComplete is true only when a deleting evaluation can prove that
	// every planned DeleteOnTaskCleanup object is absent from the complete
	// Runtime snapshot. Controllers must use this gate before completing Task
	// deletion, stop, or restart cleanup.
	CleanupComplete bool
}

// ReconcileResult combines the fresh evaluation with mutations executed by Reconcile.
type ReconcileResult struct {
	Evaluation Result
	Applied    []agentruntime.ApplyResult
	Deleted    []agentruntime.ObjectRef
	Revision   agentruntime.ExecutionRevision
}

// CleanupResult reports cleanup mutations and whether all classified objects are absent.
type CleanupResult struct {
	Managed  agentruntime.ObjectSnapshot
	Deleted  []agentruntime.ObjectRef
	Complete bool
}

// ObserveResult contains a targeted root object observation and projected Workload state.
type ObserveResult struct {
	Runtime     agentruntime.Info
	Object      agentruntime.Object
	Instances   agentruntime.InstanceSnapshot
	Observation workload.Observation
}

// Coordinator compiles Workloads and is the sole writer to registered Runtime Providers.
type Coordinator struct {
	Runtimes  []RuntimeRegistration
	Workloads []WorkloadRegistration
	// Implementations is the immutable production registry shared with Host.
	// Workloads remains as a compatibility construction path for focused tests.
	Implementations WorkloadImplementationSelector
	Resolver        environment.Resolver
	Binder          binding.PlanBinder
	// BinderForTask may construct a per-reconcile Binder from the already decoded
	// local Task snapshot. It must perform no external I/O. Production uses this
	// seam to expose short-lived local material without storing it in portable
	// Environment requirements or a shared Binder.
	BinderForTask func(coordination.TaskSnapshot) (binding.PlanBinder, error)
	// Observer records best-effort execution diagnostics. It is never consulted
	// for reconciliation decisions and nil is equivalent to Noop.
	Observer observability.Observer
}

// CleanupTask reconciles already-managed Runtime objects without compiling the
// current Workload Spec. It selects the frozen implementation and asks that
// Adapter for identity-derived inventory, so deletion remains safe when the
// current Spec can no longer be decoded. The injected policy must classify only
// resources whose ownership and delete lifecycle can be proven.
func (c *Coordinator) CleanupTask(ctx context.Context, task agentruntime.TaskIdentity, runtimeSelector coordination.RuntimeSelector, workloadSelector coordination.WorkloadSelector, carrierSelector coordination.CarrierSelector) (CleanupResult, error) {
	ctx, scope := c.startObservation(ctx, "coordinator.cleanup_task", task, runtimeSelector)
	var finalErr error
	defer func() { scope.End(observationResult(finalErr)) }()
	// 1. Select the claimed Runtime and frozen Workload implementation. Cleanup
	// deliberately does not compile because current Task Spec may be undecodable.
	runtimeRegistration, err := c.selectRuntime(runtimeSelector)
	if err != nil {
		finalErr = err
		return CleanupResult{}, err
	}
	if workloadSelector.Type == "" || workloadSelector.Version == "" {
		legacy, ok := runtimeRegistration.Provider.(agentruntime.ClaimedWorkloadProvider)
		if !ok {
			finalErr = fault.New(fault.FailedPrecondition, "legacy claim has no frozen Workload identity and Runtime cannot recover it")
			return CleanupResult{}, finalErr
		}
		workloadType, workloadVersion, err := legacy.ClaimedWorkload(ctx, task)
		if err != nil {
			finalErr = err
			return CleanupResult{}, err
		}
		workloadSelector = coordination.WorkloadSelector{Type: workloadType, Version: workloadVersion}
	}
	adapter, err := c.selectWorkload(workloadSelector, runtimeRegistration.Info.Kind)
	if err != nil {
		finalErr = err
		return CleanupResult{}, err
	}
	inventoryAdapter, ok := adapter.(workload.InventoryAdapter)
	if !ok {
		finalErr = fault.New(fault.FailedPrecondition, "selected Workload does not support identity-based inventory")
		return CleanupResult{}, finalErr
	}
	inventoryProvider, ok := runtimeRegistration.Provider.(agentruntime.InventoryProvider)
	if !ok {
		finalErr = fault.New(fault.FailedPrecondition, "selected Runtime does not support Adapter-directed inventory")
		return CleanupResult{}, finalErr
	}
	inventory, err := inventoryAdapter.Inventory(ctx, workload.InventoryRequest{Task: task, Runtime: runtimeRegistration.Info, Carrier: carrierSelector})
	if err != nil {
		finalErr = err
		return CleanupResult{}, err
	}
	adapterInfo, err := adapter.Info(ctx)
	if err != nil {
		finalErr = err
		return CleanupResult{}, err
	}
	if err := validateIncarnationAdoptionCapabilities(inventory.Candidates, adapterInfo.Capabilities, runtimeRegistration.Info.Capabilities); err != nil {
		finalErr = err
		return CleanupResult{}, err
	}
	// 2. Read one complete Adapter-directed snapshot and let the injected policy
	// choose only objects whose ownership and delete lifecycle are proven.
	managed, err := inventoryProvider.ListCandidates(ctx, agentruntime.ManagedObjectQuery{Task: task, Candidates: inventory.Candidates})
	if err != nil {
		finalErr = err
		return CleanupResult{}, err
	}
	scope.Event(observability.Event{Name: "provider.list_managed", Attributes: map[string]string{"objects": fmt.Sprintf("%d", len(managed.Objects))}})
	deletable, err := cleanupCandidates(inventory.Candidates, managed)
	if err != nil {
		finalErr = err
		return CleanupResult{}, err
	}
	result := CleanupResult{Managed: managed, Complete: len(deletable) == 0}
	deleteKey := lastPresentCleanupCandidate(inventory.Candidates, managed)
	for _, object := range managed.Objects {
		candidate, ok := deletable[objectKey(object.Manifest.Type, object.Ref.Namespace, object.Ref.Name)]
		if !ok || agentruntime.CandidateKey(candidate) != deleteKey {
			continue
		}
		result.Complete = false
		key := cleanupOperationKey(task, runtimeRegistration.Info.ID, object.Ref)
		if err := runtimeRegistration.Provider.Delete(ctx, agentruntime.DeleteRequest{IdempotencyKey: key, Task: task, Ref: object.Ref}); err != nil {
			finalErr = err
			return result, err
		}
		scope.Event(observability.Event{Name: "provider.delete", Attributes: map[string]string{"kind": object.Manifest.Type.Kind, "result": "accepted"}})
		result.Deleted = append(result.Deleted, object.Ref)
	}
	// 3. With no delete candidate in the snapshot, absence is already confirmed.
	if len(result.Deleted) == 0 {
		return result, nil
	}
	// 4. A successful Delete only means the request was accepted. Re-read a
	// complete snapshot and report completion only after every deletable object
	// is actually absent; otherwise Host retains its finalizer and retries.
	fresh, err := inventoryProvider.ListCandidates(ctx, agentruntime.ManagedObjectQuery{Task: task, Candidates: inventory.Candidates})
	if err != nil {
		finalErr = err
		return result, err
	}
	result.Managed = fresh
	freshDeletable, err := cleanupCandidates(inventory.Candidates, fresh)
	if err != nil {
		finalErr = err
		return result, err
	}
	result.Complete = len(freshDeletable) == 0
	return result, nil
}

func cleanupCandidates(candidates []agentruntime.ObjectCandidate, managed agentruntime.ObjectSnapshot) (map[string]agentruntime.ObjectCandidate, error) {
	declared := make(map[string]agentruntime.ObjectCandidate, len(candidates))
	for _, candidate := range candidates {
		if candidate.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup {
			continue
		}
		key := agentruntime.CandidateKey(candidate)
		if key == "" {
			return nil, fault.New(fault.InvalidArgument, "DeleteOnTaskCleanup inventory candidate must use an exact name")
		}
		if _, found := declared[key]; found {
			return nil, fault.New(fault.InvalidArgument, "duplicate DeleteOnTaskCleanup inventory candidate")
		}
		declared[key] = candidate
	}
	result := make(map[string]agentruntime.ObjectCandidate)
	for _, object := range managed.Objects {
		key := objectKey(object.Manifest.Type, object.Ref.Namespace, object.Ref.Name)
		if candidate, found := declared[key]; found {
			result[key] = candidate
		}
	}
	return result, nil
}

func lastPresentCleanupCandidate(candidates []agentruntime.ObjectCandidate, managed agentruntime.ObjectSnapshot) string {
	present := make(map[string]struct{}, len(managed.Objects))
	for _, object := range managed.Objects {
		present[objectKey(object.Manifest.Type, object.Ref.Namespace, object.Ref.Name)] = struct{}{}
	}
	for i := len(candidates) - 1; i >= 0; i-- {
		candidate := candidates[i]
		if candidate.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup {
			continue
		}
		key := agentruntime.CandidateKey(candidate)
		if _, found := present[key]; found {
			return key
		}
	}
	return ""
}

// ObserveRoot performs targeted status observation for one already-validated
// workload root. It intentionally skips Compile, Resolve, Bind, ListManaged,
// and diff so high-frequency workload or Pod status changes remain cheap.
func (c *Coordinator) ObserveRoot(ctx context.Context, task coordination.TaskSnapshot, root agentruntime.ObjectRef) (ObserveResult, error) {
	ctx, scope := c.startObservation(ctx, "coordinator.observe_root", task.Identity, task.RuntimeSelector)
	var finalErr error
	defer func() { scope.End(observationResult(finalErr)) }()
	runtimeRegistration, err := c.selectRuntime(task.RuntimeSelector)
	if err != nil {
		finalErr = err
		return ObserveResult{}, err
	}
	adapter, err := c.selectWorkload(task.WorkloadSelector, runtimeRegistration.Info.Kind)
	if err != nil {
		finalErr = err
		return ObserveResult{}, err
	}
	object, err := runtimeRegistration.Provider.Get(ctx, root)
	if err != nil {
		finalErr = err
		return ObserveResult{}, err
	}
	instances, err := runtimeRegistration.Provider.ListInstances(ctx, agentruntime.InstanceQuery{Task: task.Identity, Workloads: []agentruntime.ObjectRef{object.Ref}, Roots: []string{"workload"}})
	if err != nil {
		finalErr = err
		return ObserveResult{}, err
	}
	observation, err := adapter.Observe(ctx, workload.ObserveRequest{Task: task, Objects: agentruntime.ObjectSnapshot{Objects: []agentruntime.Object{object}}, Instances: instances})
	if err != nil {
		finalErr = err
		return ObserveResult{}, err
	}
	return ObserveResult{Runtime: runtimeRegistration.Info, Object: object, Instances: instances, Observation: observation}, nil
}

func cleanupOperationKey(task agentruntime.TaskIdentity, runtimeID agentruntime.ID, ref agentruntime.ObjectRef) string {
	value := strings.Join([]string{"cleanup", task.Logical.String(), task.ObjectUID, string(runtimeID), ref.ID, ref.Namespace, ref.Name, ref.UID}, "\x00")
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Evaluate computes a complete execution decision without mutating the
// Runtime. Production controllers can shadow this method safely.
func (c *Coordinator) Evaluate(ctx context.Context, task coordination.TaskSnapshot) (Result, error) {
	ctx, scope := c.startObservation(ctx, "coordinator.evaluate", task.Identity, task.RuntimeSelector)
	var finalErr error
	defer func() { scope.End(observationResult(finalErr)) }()
	// 1. Resolve the exact Runtime and Workload implementation before reading or
	// writing external state. Selection failures are stable contract faults.
	runtimeRegistration, err := c.selectRuntime(task.RuntimeSelector)
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	adapter, err := c.selectWorkload(task.WorkloadSelector, runtimeRegistration.Info.Kind)
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	if c.Resolver == nil || c.Binder == nil {
		finalErr = fault.New(fault.FailedPrecondition, "environment resolver and plan binder are required")
		return Result{}, finalErr
	}
	// 2. Compile Runtime-neutral Workload intent, resolve Environment bindings,
	// and bind both into the complete Runtime-specific executable plan.
	plan, err := adapter.Compile(ctx, workload.CompileRequest{Task: task, Runtime: runtimeRegistration.Info})
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	plan.Environment.Environments = append(plan.Environment.Environments, task.Environment.Environments...)
	scope.Event(observability.Event{Name: "adapter.compile", Attributes: map[string]string{"objects": fmt.Sprintf("%d", len(plan.Objects))}})
	observability.RecordDetailed(scope, "adapter.compile.plan", observability.PayloadPlan, plan, nil)
	resolved, err := c.Resolver.Resolve(ctx, environment.ResolveRequest{Task: task.Identity, Runtime: runtimeRegistration.Info, Requirements: plan.Environment, Targets: plan.Targets})
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	binder := c.Binder
	if c.BinderForTask != nil {
		binder, err = c.BinderForTask(task)
		if err != nil {
			finalErr = err
			return Result{}, err
		}
	}
	if binder == nil {
		finalErr = fault.New(fault.FailedPrecondition, "environment plan binder is required")
		return Result{}, finalErr
	}
	executable, err := binder.Bind(ctx, binding.BindRequest{Runtime: runtimeRegistration.Info, Plan: plan, Environment: resolved})
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	if executable.Digests.ExecutableHash != "" || executable.Digests.BindingRevision != "" {
		finalErr = fault.New(fault.Internal, "plan binder must not set Coordinator-owned execution digests")
		return Result{}, finalErr
	}
	if executable.Digests.CompiledHash == "" {
		executable.Digests.CompiledHash = plan.CompiledHash
	}
	// 3. Canonicalize dependency order and derive a stable executable hash. This
	// makes operation ordering and idempotency independent of adapter slice order.
	ordered, err := orderDesired(executable.Objects)
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	executable.Objects = ordered
	// Hash the canonical dependency order rather than adapter emission order.
	// The Coordinator owns this digest because it is used for idempotency keys.
	executable.Digests.ExecutableHash, err = executableHash(executable)
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	bindingRevision, err := environment.BindingRevision(resolved)
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	executable.Digests.BindingRevision = bindingRevision
	// 4. Read a complete managed snapshot and calculate the desired/current diff.
	// Evaluate is read-only: it returns operations but never applies them.
	inventoryAdapter, ok := adapter.(workload.InventoryAdapter)
	if !ok {
		finalErr = fault.New(fault.FailedPrecondition, "selected Workload does not support identity-based inventory")
		return Result{}, finalErr
	}
	inventoryProvider, ok := runtimeRegistration.Provider.(agentruntime.InventoryProvider)
	if !ok {
		finalErr = fault.New(fault.FailedPrecondition, "selected Runtime does not support Adapter-directed inventory")
		return Result{}, finalErr
	}
	inventory, err := inventoryAdapter.Inventory(ctx, workload.InventoryRequest{Task: task.Identity, Runtime: runtimeRegistration.Info, Carrier: task.CarrierSelector})
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	adapterInfo, err := adapter.Info(ctx)
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	if err := validateIncarnationAdoptionCapabilities(inventory.Candidates, adapterInfo.Capabilities, runtimeRegistration.Info.Capabilities); err != nil {
		finalErr = err
		return Result{}, err
	}
	if err := validateReconcileProfileCapabilities(executable.Objects, adapterInfo.Capabilities, runtimeRegistration.Info.Capabilities); err != nil {
		finalErr = err
		return Result{}, err
	}
	managed, err := inventoryProvider.ListCandidates(ctx, agentruntime.ManagedObjectQuery{Task: task.Identity, Candidates: inventory.Candidates})
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	scope.Event(observability.Event{Name: "provider.list_managed", Attributes: map[string]string{"objects": fmt.Sprintf("%d", len(managed.Objects))}})
	operations, roots, orphans, err := diff(task, runtimeRegistration.Info.ID, executable, managed, runtimeRegistration.Comparator)
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	scope.Event(observability.Event{Name: "coordinator.diff", Attributes: map[string]string{"operations": fmt.Sprintf("%d", len(operations)), "orphans": fmt.Sprintf("%d", len(orphans))}})
	observability.RecordDetailed(scope, "coordinator.diff.operations", observability.PayloadOperations, operationSummaries(operations), nil)
	// 5. Observe only current root objects and their instances, then let the
	// Workload Adapter project Runtime state into semantic workload status.
	instances, err := runtimeRegistration.Provider.ListInstances(ctx, agentruntime.InstanceQuery{Task: task.Identity, Workloads: roots, Roots: executable.Discovery.Roots, Options: executable.Discovery.Options})
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	observation, err := adapter.Observe(ctx, workload.ObserveRequest{Task: task, Plan: executable, Objects: managed, Instances: instances})
	if err != nil {
		finalErr = err
		return Result{}, err
	}
	return Result{Runtime: runtimeRegistration.Info, Plan: executable, Managed: managed, Instances: instances, Observation: observation, Operations: operations, Orphans: orphans, Roots: roots, CleanupComplete: cleanupComplete(task, executable, managed)}, nil
}

func validateIncarnationAdoptionCapabilities(candidates []agentruntime.ObjectCandidate, adapterCapabilities, runtimeCapabilities []agentruntime.Capability) error {
	// TODO(execution): Replace feature-specific capability gates with one
	// versioned composition check that consumes every advertised capability,
	// including inventory, owned-field projection, and claimed recovery.
	required := false
	for _, candidate := range candidates {
		if candidate.Ownership.IncarnationPolicy == agentruntime.IncarnationOwnershipAllowAdoption {
			required = true
			break
		}
	}
	if !required {
		return nil
	}
	if !hasCapability(adapterCapabilities, agentruntime.CapabilityIncarnationAdoption) {
		return fault.New(fault.FailedPrecondition, "selected Workload declares incarnation adoption without advertising its capability")
	}
	if !hasCapability(runtimeCapabilities, agentruntime.CapabilityIncarnationAdoption) {
		return fault.New(fault.FailedPrecondition, "selected Runtime does not support Task-incarnation adoption")
	}
	return nil
}

func validateReconcileProfileCapabilities(objects []agentruntime.DesiredObject, adapterCapabilities, runtimeCapabilities []agentruntime.Capability) error {
	required := false
	for _, object := range objects {
		if object.ReconcileProfile != (agentruntime.ObjectKind{}) {
			required = true
			break
		}
	}
	if !required {
		return nil
	}
	if !hasCapability(adapterCapabilities, agentruntime.CapabilityReconcileProfiles) {
		return fault.New(fault.FailedPrecondition, "selected Workload declares reconcile profiles without advertising its capability")
	}
	if !hasCapability(runtimeCapabilities, agentruntime.CapabilityReconcileProfiles) {
		return fault.New(fault.FailedPrecondition, "selected Runtime does not support typed reconcile profiles")
	}
	return nil
}

func hasCapability(capabilities []agentruntime.Capability, required agentruntime.Capability) bool {
	for _, capability := range capabilities {
		if capability == required {
			return true
		}
	}
	return false
}

func cleanupComplete(task coordination.TaskSnapshot, plan executionplan.ExecutablePlan, managed agentruntime.ObjectSnapshot) bool {
	if !task.Deleting {
		return false
	}
	current := make(map[string]struct{}, len(managed.Objects))
	for _, object := range managed.Objects {
		current[objectKey(object.Manifest.Type, object.Ref.Namespace, object.Ref.Name)] = struct{}{}
	}
	for _, desired := range plan.Objects {
		if desired.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup {
			continue
		}
		identity, err := decodeIdentity(desired.Manifest)
		if err != nil {
			return false
		}
		if _, found := current[objectKey(desired.Kind, identity.Metadata.Namespace, identity.Metadata.Name)]; found {
			return false
		}
	}
	return true
}

// Reconcile is the single Runtime writer. It evaluates a complete snapshot,
// executes the ordered operations, then re-evaluates observation from a fresh
// complete snapshot. Callers must not run a legacy writer for the same Task.
func (c *Coordinator) Reconcile(ctx context.Context, task coordination.TaskSnapshot) (ReconcileResult, error) {
	ctx, scope := c.startObservation(ctx, "coordinator.reconcile", task.Identity, task.RuntimeSelector)
	var finalErr error
	defer func() { scope.End(observationResult(finalErr)) }()
	// 1. Evaluate from current Runtime state before performing any write.
	evaluation, err := c.Evaluate(ctx, task)
	if err != nil {
		finalErr = err
		return ReconcileResult{}, err
	}
	runtimeRegistration, err := c.selectRuntime(task.RuntimeSelector)
	if err != nil {
		finalErr = err
		return ReconcileResult{}, err
	}
	// 2. Execute the canonical operation sequence through the selected Provider.
	// Each operation has a deterministic idempotency key. On partial failure,
	// return completed effects and let the next reconcile recover from observation.
	result := ReconcileResult{Evaluation: evaluation}
	result.Revision = executionRevision(task, evaluation)
	provenOwners := make([]agentruntime.ObjectRef, 0, len(evaluation.Managed.Objects))
	for _, object := range evaluation.Managed.Objects {
		provenOwners = append(provenOwners, object.Ref)
	}
	for _, operation := range evaluation.Operations {
		switch operation.Kind {
		case OperationCreate, OperationUpdate:
			if operation.Desired == nil {
				finalErr = fault.New(fault.Internal, "Apply operation has no desired object")
				return ReconcileResult{}, finalErr
			}
			applied, err := runtimeRegistration.Provider.Apply(ctx, agentruntime.ApplyRequest{IdempotencyKey: operation.IdempotencyKey, Task: task.Identity, Desired: *operation.Desired, Current: operation.Current, ProvenOwners: provenOwners, Revision: executionRevision(task, evaluation)})
			if err != nil {
				finalErr = err
				return result, err
			}
			result.Applied = append(result.Applied, applied)
			scope.Event(observability.Event{Name: "provider.apply", Attributes: map[string]string{"action": string(operation.Kind), "kind": operation.Desired.Kind.Kind, "result": "succeeded"}})
		case OperationDelete:
			if operation.Current == nil {
				finalErr = fault.New(fault.Internal, "Delete operation has no current object")
				return ReconcileResult{}, finalErr
			}
			if err := runtimeRegistration.Provider.Delete(ctx, agentruntime.DeleteRequest{IdempotencyKey: operation.IdempotencyKey, Task: task.Identity, Ref: *operation.Current}); err != nil {
				finalErr = err
				return result, err
			}
			result.Deleted = append(result.Deleted, *operation.Current)
			scope.Event(observability.Event{Name: "provider.delete", Attributes: map[string]string{"action": string(operation.Kind), "result": "accepted"}})
		default:
			finalErr = fault.New(fault.Internal, fmt.Sprintf("unknown execution operation %q", operation.Kind))
			return ReconcileResult{}, finalErr
		}
	}
	// 3. Re-evaluate after any write. The fresh complete snapshot, rather than
	// assumed operation results, becomes the authoritative observation. A no-op
	// evaluation performs no second read and remains a zero-write steady state.
	if len(evaluation.Operations) != 0 {
		fresh, err := c.Evaluate(ctx, task)
		if err != nil {
			finalErr = err
			return result, err
		}
		result.Evaluation = fresh
	}
	return result, nil
}

func (c *Coordinator) startObservation(ctx context.Context, name string, task agentruntime.TaskIdentity, selector coordination.RuntimeSelector) (context.Context, observability.Scope) {
	observer := c.Observer
	if observer == nil {
		observer = observability.Noop()
	}
	return observer.Start(ctx, observability.Reconcile{Name: name, TaskKey: task.Logical.String(), TaskUID: task.ObjectUID, Generation: task.Generation, RuntimeID: string(selector.ID)})
}

func observationResult(err error) observability.Result {
	if err == nil {
		return observability.Result{Outcome: "Succeeded"}
	}
	code := fault.Internal
	var domain *fault.Error
	if errors.As(err, &domain) {
		code = domain.Code
	}
	return observability.Result{Outcome: "Failed", FaultCode: string(code)}
}

type operationSummary struct {
	Kind        OperationKind `json:"kind"`
	LogicalName string        `json:"logicalName"`
	ObjectKind  string        `json:"objectKind,omitempty"`
	Fenced      bool          `json:"fenced"`
}

func operationSummaries(operations []Operation) []operationSummary {
	result := make([]operationSummary, 0, len(operations))
	for _, operation := range operations {
		summary := operationSummary{Kind: operation.Kind, LogicalName: operation.LogicalName, Fenced: operation.Current != nil && operation.Current.UID != ""}
		if operation.Desired != nil {
			summary.ObjectKind = operation.Desired.Kind.Kind
		}
		result = append(result, summary)
	}
	return result
}

func executionRevision(task coordination.TaskSnapshot, evaluation Result) agentruntime.ExecutionRevision {
	return agentruntime.ExecutionRevision{
		RuntimeID: evaluation.Runtime.ID, RuntimeKind: evaluation.Runtime.Kind,
		WorkloadType: task.WorkloadSelector.Type, WorkloadVersion: task.WorkloadSelector.Version,
		CompiledDigest:   string(canonicalDigest(evaluation.Plan.Digests.CompiledHash)),
		ExecutableDigest: string(canonicalDigest(evaluation.Plan.Digests.ExecutableHash)),
		BindingRevision:  evaluation.Plan.Digests.BindingRevision,
	}
}

func canonicalDigest(value string) evidence.Digest {
	digest := evidence.Digest(value)
	if digest.Validate() == nil {
		return digest
	}
	return evidence.NewDigest([]byte(value))
}

func (c *Coordinator) selectRuntime(selector coordination.RuntimeSelector) (RuntimeRegistration, error) {
	candidates := make([]coordination.RuntimeCandidate, 0, len(c.Runtimes))
	byID := make(map[agentruntime.ID]RuntimeRegistration, len(c.Runtimes))
	for _, candidate := range c.Runtimes {
		candidates = append(candidates, coordination.RuntimeCandidate{Info: candidate.Info})
		byID[candidate.Info.ID] = candidate
	}
	selected, err := coordination.SelectRuntime(selector, candidates)
	if err != nil {
		return RuntimeRegistration{}, selectionFault("select Runtime", err)
	}
	registration := byID[selected.Info.ID]
	if registration.Provider == nil {
		return RuntimeRegistration{}, fault.New(fault.FailedPrecondition, "selected Runtime has no Provider")
	}
	return registration, nil
}

func (c *Coordinator) selectWorkload(selector coordination.WorkloadSelector, runtimeKind agentruntime.Kind) (workload.Adapter, error) {
	if c.Implementations != nil {
		adapter, err := c.Implementations.SelectImplementation(workload.TypeID(selector.Type), selector.Version, runtimeKind)
		if err != nil {
			return nil, selectionFault("select Workload implementation", err)
		}
		return adapter, nil
	}
	candidates := make([]workload.Candidate[workload.Adapter], 0, len(c.Workloads))
	for _, candidate := range c.Workloads {
		if candidate.RuntimeKind != runtimeKind {
			continue
		}
		candidates = append(candidates, workload.Candidate[workload.Adapter]{Type: candidate.Type, Version: candidate.Version, Value: candidate.Adapter})
	}
	selected, err := workload.Select(selector, candidates)
	if err != nil {
		return nil, selectionFault("select Workload", err)
	}
	if selected == nil {
		return nil, fault.New(fault.FailedPrecondition, "selected Workload has no Adapter")
	}
	return selected, nil
}

func selectionFault(operation string, err error) error {
	if errors.Is(err, coordination.ErrRuntimeNotFound) || errors.Is(err, workload.ErrAdapterNotFound) {
		return fault.Wrap(fault.NotFound, operation, err)
	}
	return fault.Wrap(fault.FailedPrecondition, operation, err)
}

type manifestIdentity struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Metadata   struct {
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"metadata"`
}

func diff(task coordination.TaskSnapshot, runtimeID agentruntime.ID, plan executionplan.ExecutablePlan, managed agentruntime.ObjectSnapshot, comparator ObjectComparator) ([]Operation, []agentruntime.ObjectRef, []agentruntime.ObjectRef, error) {
	// 1. Index the complete current snapshot by concrete Runtime identity and
	// prepare discovery-root lookup independently of provider return order.
	current := make(map[string]agentruntime.Object, len(managed.Objects))
	for _, object := range managed.Objects {
		current[objectKey(object.Manifest.Type, object.Ref.Namespace, object.Ref.Name)] = object
	}
	desiredKeys := make(map[string]struct{}, len(plan.Objects))
	operations := make([]Operation, 0)
	rootsByName := make(map[string]agentruntime.ObjectRef)
	rootNames := make(map[string]struct{}, len(plan.Discovery.Roots))
	for _, root := range plan.Discovery.Roots {
		rootNames[root] = struct{}{}
	}
	iteration := make([]int, len(plan.Objects))
	for i := range plan.Objects {
		iteration[i] = i
	}
	// 2. Creation/update follows dependency order. Deletion traverses the same
	// DAG in reverse so dependants are removed before their dependencies.
	if task.Deleting {
		for left, right := 0, len(iteration)-1; left < right; left, right = left+1, right-1 {
			iteration[left], iteration[right] = iteration[right], iteration[left]
		}
	}
	// 3. Compare each desired object with its same-kind/name current object.
	// Emit lifecycle-approved deletes for Task deletion, otherwise emit create or
	// update operations and capture existing roots for instance observation.
	for _, i := range iteration {
		desired := plan.Objects[i]
		identity, err := decodeIdentity(desired.Manifest)
		if err != nil {
			return nil, nil, nil, fault.Wrap(fault.InvalidArgument, "decode desired object identity", err)
		}
		key := objectKey(desired.Kind, identity.Metadata.Namespace, identity.Metadata.Name)
		desiredKeys[key] = struct{}{}
		if task.Deleting {
			if existing, found := current[key]; found && desired.Lifecycle == agentruntime.LifecycleDeleteOnTaskCleanup {
				ref := existing.Ref
				operation := Operation{Kind: OperationDelete, LogicalName: desired.LogicalName, Current: &ref}
				operation.IdempotencyKey = operationKey(task.Identity, runtimeID, plan.Digests.ExecutableHash, operation)
				operations = append(operations, operation)
			}
			continue
		}
		operation := Operation{LogicalName: desired.LogicalName}
		existing, found := current[key]
		if found {
			if _, root := rootNames[desired.LogicalName]; root {
				rootsByName[desired.LogicalName] = existing.Ref
			}
			equal, err := desiredObjectEqual(existing.Manifest, desired, comparator)
			if err != nil {
				return nil, nil, nil, fault.Wrap(fault.InvalidArgument, "compare desired object", err)
			}
			if equal {
				continue
			}
			if desired.UpdateMode == agentruntime.UpdateReplace {
				operation.Kind, operation.Current = OperationDelete, &existing.Ref
			} else {
				operation.Kind, operation.Desired, operation.Current = OperationUpdate, &desired, &existing.Ref
			}
		} else {
			operation.Kind, operation.Desired = OperationCreate, &desired
		}
		operation.IdempotencyKey = operationKey(task.Identity, runtimeID, plan.Digests.ExecutableHash, operation)
		operations = append(operations, operation)
	}
	if task.Deleting {
		return operations, nil, nil, nil
	}
	// 4. Report managed objects absent from the desired plan as orphans. They are
	// diagnostic here; diff does not silently infer permission to delete them.
	orphans := make([]agentruntime.ObjectRef, 0)
	for i := len(managed.Objects) - 1; i >= 0; i-- {
		object := managed.Objects[i]
		key := objectKey(object.Manifest.Type, object.Ref.Namespace, object.Ref.Name)
		if _, retained := desiredKeys[key]; retained {
			continue
		}
		orphans = append(orphans, object.Ref)
	}
	// 5. Preserve adapter-declared root order and include only roots that exist in
	// the current snapshot. Newly created roots appear after the fresh evaluation.
	roots := make([]agentruntime.ObjectRef, 0, len(plan.Discovery.Roots))
	for _, name := range plan.Discovery.Roots {
		if ref, found := rootsByName[name]; found {
			roots = append(roots, ref)
		}
	}
	return operations, roots, orphans, nil
}

func desiredObjectEqual(current agentruntime.TypedManifest, desired agentruntime.DesiredObject, comparator ObjectComparator) (bool, error) {
	if comparator != nil {
		return comparator.Equal(current, desired)
	}
	return jsonEqual(current.Object, desired.Manifest.Object), nil
}

func orderDesired(objects []agentruntime.DesiredObject) ([]agentruntime.DesiredObject, error) {
	byName := make(map[string]agentruntime.DesiredObject, len(objects))
	indegree := make(map[string]int, len(objects))
	children := make(map[string][]string)
	for _, object := range objects {
		if object.LogicalName == "" {
			return nil, fault.New(fault.InvalidArgument, "desired object logical name is required")
		}
		if _, found := byName[object.LogicalName]; found {
			return nil, fault.New(fault.InvalidArgument, "duplicate desired object logical name")
		}
		byName[object.LogicalName], indegree[object.LogicalName] = object, 0
	}
	for _, object := range objects {
		for _, dependency := range object.DependsOn {
			if _, found := byName[dependency]; !found {
				return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("desired object %q depends on missing object %q", object.LogicalName, dependency))
			}
			indegree[object.LogicalName]++
			children[dependency] = append(children[dependency], object.LogicalName)
		}
	}
	ready := make([]string, 0)
	for name, degree := range indegree {
		if degree == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)
	result := make([]agentruntime.DesiredObject, 0, len(objects))
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		result = append(result, byName[name])
		sort.Strings(children[name])
		for _, child := range children[name] {
			indegree[child]--
			if indegree[child] == 0 {
				ready = append(ready, child)
				sort.Strings(ready)
			}
		}
	}
	if len(result) != len(objects) {
		return nil, fault.New(fault.InvalidArgument, "desired object dependencies contain a cycle")
	}
	return result, nil
}

func decodeIdentity(manifest agentruntime.TypedManifest) (manifestIdentity, error) {
	var identity manifestIdentity
	if !json.Valid(manifest.Object) {
		return identity, errors.New("manifest is not valid JSON")
	}
	if err := json.Unmarshal(manifest.Object, &identity); err != nil {
		return identity, err
	}
	if identity.Metadata.Name == "" {
		return identity, errors.New("manifest metadata.name is required")
	}
	return identity, nil
}
func objectKey(kind agentruntime.ObjectKind, namespace, name string) string {
	return strings.Join([]string{kind.Group, kind.Version, kind.Kind, namespace, name}, "/")
}
func jsonEqual(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil && reflect.DeepEqual(a, b)
}
func executableHash(plan executionplan.ExecutablePlan) (string, error) {
	objects := make([]agentruntime.DesiredObject, len(plan.Objects))
	for i := range plan.Objects {
		objects[i] = plan.Objects[i]
		objects[i].Manifest.Object = append([]byte(nil), plan.Objects[i].Manifest.Object...)
		if objects[i].Kind.Kind == "Secret" {
			var manifest map[string]any
			if err := json.Unmarshal(objects[i].Manifest.Object, &manifest); err != nil {
				return "", err
			}
			if _, found := manifest["data"]; found {
				manifest["data"] = "[REDACTED]"
			}
			if _, found := manifest["stringData"]; found {
				manifest["stringData"] = "[REDACTED]"
			}
			redacted, err := json.Marshal(manifest)
			if err != nil {
				return "", err
			}
			objects[i].Manifest.Object = redacted
		}
	}
	value := struct {
		Objects   []agentruntime.DesiredObject
		Discovery executionplan.InstanceDiscovery
		Update    executionplan.UpdatePolicy
	}{objects, plan.Discovery, plan.Update}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func operationKey(task agentruntime.TaskIdentity, runtimeID agentruntime.ID, hash string, operation Operation) string {
	value := fmt.Sprintf("%s\x00%s\x00%d\x00%s\x00%s\x00%s\x00%s", task.Logical.String(), task.ObjectUID, task.Generation, runtimeID, hash, operation.Kind, operation.LogicalName)
	if operation.Current != nil {
		value += "\x00" + operation.Current.ID + "\x00" + operation.Current.UID
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
