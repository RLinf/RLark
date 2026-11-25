package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

// This fixture has a Kubernetes management API only. Its workload backend is an
// independent typed store, never a local Kubernetes workload client. Rebuilding
// Host/Coordinator leaves both durable stores intact and drops execution memory.
type conformanceStore struct {
	mu                       sync.Mutex
	objects                  map[string]conformanceObject
	applies                  []agentruntime.ApplyRequest
	deletes                  []agentruntime.DeleteRequest
	failApplyAt, loseReplyAt int
	delayDelete              bool
	unavailable              bool
	nextUID                  int
	applyEntered             chan struct{}
	applyRelease             chan struct{}
	loseDeleteReplyAt        int
}

type conformanceProvider struct {
	store        *conformanceStore
	capabilities []agentruntime.Capability
}

type conformanceObject struct {
	agentruntime.Object
	Ownership agentruntime.ResourceOwnership
	Lifecycle agentruntime.ResourceLifecycle
}

func (p conformanceProvider) Info(context.Context) (agentruntime.Info, error) {
	return agentruntime.Info{ID: "fixture", Kind: "Fake", Capabilities: p.capabilities}, nil
}
func (p conformanceProvider) Get(ctx context.Context, ref agentruntime.ObjectRef) (agentruntime.Object, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return agentruntime.Object{}, err
	}
	o, ok := p.store.objects[ref.Name]
	if !ok || o.Ref.UID != ref.UID {
		return agentruntime.Object{}, fault.New(fault.NotFound, "instance absent")
	}
	return o.Object, nil
}
func (p conformanceProvider) ListManaged(context.Context, agentruntime.TaskIdentity) (agentruntime.ObjectSnapshot, error) {
	return agentruntime.ObjectSnapshot{}, errors.New("conformance requires Adapter-directed inventory")
}
func (p conformanceProvider) ListCandidates(ctx context.Context, q agentruntime.ManagedObjectQuery) (agentruntime.ObjectSnapshot, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return agentruntime.ObjectSnapshot{}, err
	}
	if p.store.unavailable {
		return agentruntime.ObjectSnapshot{}, errors.New("backend unavailable")
	}
	result := agentruntime.ObjectSnapshot{}
	for _, candidate := range q.Candidates {
		if o, ok := p.store.objects[candidate.Name]; ok {
			if o.Ownership != candidate.Ownership || o.Ref.LogicalTask != q.Task.Logical {
				return agentruntime.ObjectSnapshot{}, fault.New(fault.Conflict, "foreign runtime owner")
			}
			result.Objects = append(result.Objects, o.Object)
		}
	}
	return result, nil
}
func (p conformanceProvider) Apply(ctx context.Context, req agentruntime.ApplyRequest) (agentruntime.ApplyResult, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return agentruntime.ApplyResult{}, err
	}
	p.store.applies = append(p.store.applies, req)
	if p.store.applyEntered != nil {
		close(p.store.applyEntered)
		select {
		case <-ctx.Done():
			return agentruntime.ApplyResult{}, ctx.Err()
		case <-p.store.applyRelease:
		}
	}
	if len(p.store.applies) == p.store.failApplyAt {
		return agentruntime.ApplyResult{}, errors.New("apply failed before write")
	}
	var manifest struct{ Metadata struct{ Name string } }
	if err := json.Unmarshal(req.Desired.Manifest.Object, &manifest); err != nil {
		return agentruntime.ApplyResult{}, err
	}
	current, exists := p.store.objects[manifest.Metadata.Name]
	if exists && current.Ownership != req.Desired.Ownership {
		return agentruntime.ApplyResult{}, fault.New(fault.Conflict, "foreign runtime owner")
	}
	if req.Current != nil && (!exists || req.Current.UID != current.Ref.UID) {
		return agentruntime.ApplyResult{}, fault.New(fault.Conflict, "runtime UID changed")
	}
	if !exists {
		p.store.nextUID++
		current.Ref = agentruntime.ObjectRef{RuntimeID: "fixture", ID: req.Desired.Kind.Kind, Namespace: "backend", Name: manifest.Metadata.Name, UID: fmt.Sprint(p.store.nextUID), LogicalTask: req.Task.Logical}
	}
	current.Manifest, current.Ownership, current.Lifecycle = req.Desired.Manifest, req.Desired.Ownership, req.Desired.Lifecycle
	p.store.objects[manifest.Metadata.Name] = current
	if len(p.store.applies) == p.store.loseReplyAt {
		return agentruntime.ApplyResult{}, errors.New("reply lost after write")
	}
	return agentruntime.ApplyResult{Ref: current.Ref, Object: current.Object}, nil
}
func (p conformanceProvider) Delete(ctx context.Context, req agentruntime.DeleteRequest) error {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	p.store.deletes = append(p.store.deletes, req)
	o, exists := p.store.objects[req.Ref.Name]
	if !exists {
		return nil
	}
	if o.Ref.UID != req.Ref.UID || o.Ownership.OwnerKey != req.Task.ObjectUID {
		return fault.New(fault.Conflict, "delete ownership changed")
	}
	if !p.store.delayDelete {
		delete(p.store.objects, req.Ref.Name)
	}
	if len(p.store.deletes) == p.store.loseDeleteReplyAt {
		return errors.New("delete reply lost after write")
	}
	return nil
}
func (p conformanceProvider) ListInstances(ctx context.Context, q agentruntime.InstanceQuery) (agentruntime.InstanceSnapshot, error) {
	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return agentruntime.InstanceSnapshot{}, err
	}
	result := agentruntime.InstanceSnapshot{}
	for _, root := range q.Workloads {
		if current, ok := p.store.objects[root.Name]; ok && current.Ref.UID == root.UID {
			result.Instances = append(result.Instances, agentruntime.Instance{Task: q.Task, Name: root.Name})
		}
	}
	return result, nil
}

type conformanceAdapter struct {
	badProfile   bool
	badSemantics string
}

func (conformanceAdapter) Observe(_ context.Context, req workload.ObserveRequest) (workload.Observation, error) {
	phase := workload.Phase("Pending")
	if len(req.Instances.Instances) != 0 {
		phase = "Running"
	}
	return workload.Observation{Phase: phase}, nil
}

func (conformanceAdapter) Info(context.Context) (workload.TypeInfo, error) {
	return workload.TypeInfo{ID: "FakeWorkload", Version: "v1", SupportedRuntimeKinds: []agentruntime.Kind{"Fake"}, Capabilities: []agentruntime.Capability{agentruntime.CapabilityWorkloadInventory}}, nil
}

func (a conformanceAdapter) Compile(_ context.Context, req workload.CompileRequest) (executionplan.Plan, error) {
	objects := conformanceObjects(req.Task.Identity)
	if a.badProfile {
		objects[0].ReconcileProfile = agentruntime.ObjectKind{Version: "v99", Kind: "Unknown"}
	}
	plan := executionplan.Plan{Objects: objects, Discovery: executionplan.InstanceDiscovery{Roots: []string{"root"}}}
	switch a.badSemantics {
	case "environment":
		plan.Environment.Environments = []executionplan.EnvironmentRequirement{{Name: "unknown", Type: "unknown", Version: "v99"}}
	case "units":
		plan.Units = []executionplan.WorkloadUnit{{Name: "unknown", ObjectName: "root", Kind: "unknown"}}
	case "processes":
		plan.Processes = []executionprocess.Declaration{{Name: "unknown", Required: true}}
	}
	return plan, nil
}
func (conformanceAdapter) Inventory(_ context.Context, req workload.InventoryRequest) (workload.Inventory, error) {
	result := workload.Inventory{}
	for _, o := range conformanceObjects(req.Task) {
		result.Candidates = append(result.Candidates, agentruntime.ObjectCandidate{Kind: o.Kind, Namespace: "backend", Name: req.Task.Logical.Name + "-" + o.LogicalName, Ownership: o.Ownership, Lifecycle: o.Lifecycle})
	}
	return result, nil
}

func conformanceObjects(task agentruntime.TaskIdentity) []agentruntime.DesiredObject {
	var objects []agentruntime.DesiredObject
	for _, logical := range []string{"config", "root"} {
		kind := agentruntime.ObjectKind{Group: "fixture.test", Version: "v1", Kind: "Object"}
		o := agentruntime.DesiredObject{LogicalName: logical, Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: []byte(fmt.Sprintf(`{"apiVersion":"fixture.test/v1","kind":"Object","metadata":{"namespace":"backend","name":%q},"spec":{"generation":%d}}`, task.Logical.Name+"-"+logical, task.Generation))}, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: task.ObjectUID}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace}
		if logical == "root" {
			o.DependsOn = []string{"config"}
		}
		objects = append(objects, o)
	}
	return objects
}

type conformanceResolver struct{}

func (conformanceResolver) Resolve(_ context.Context, req environment.ResolveRequest) (environment.ResolvedEnvironment, error) {
	if !req.Requirements.Empty() {
		return environment.ResolvedEnvironment{}, fault.New(fault.FailedPrecondition, "unsupported required environment")
	}
	return environment.ResolvedEnvironment{Task: req.Task}, nil
}

type conformanceLowerer struct{}

func (conformanceLowerer) Lower(_ context.Context, req agentruntime.UnitLoweringRequest) (agentruntime.DesiredObject, error) {
	if len(req.Object.Units) != 0 {
		return agentruntime.DesiredObject{}, fault.New(fault.FailedPrecondition, "unsupported units")
	}
	return req.Object, nil
}

type conformanceValidator struct{}

func (conformanceValidator) ValidateDesired(o agentruntime.DesiredObject) error {
	if o.ReconcileProfile != (agentruntime.ObjectKind{}) {
		return fault.New(fault.FailedPrecondition, "unsupported reconcile profile")
	}
	return nil
}

func newConformanceHost(t *testing.T, tasks client.Client, store *conformanceStore) *Host {
	t.Helper()
	adapter := conformanceAdapter{}
	registry, err := NewImplementationRegistry(context.Background(), []WorkloadRegistration{{Key: ImplementationKey{WorkloadType: "FakeWorkload", WorkloadVersion: "v1", RuntimeKind: "Fake"}, Adapter: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	provider := conformanceProvider{store: store, capabilities: []agentruntime.Capability{agentruntime.CapabilityCandidateInventory}}
	info, _ := provider.Info(context.Background())
	handlers, err := environment.NewRegistry(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &coordinator.Coordinator{Runtimes: []coordinator.RuntimeRegistration{{Info: info, Provider: provider}}, Implementations: registry.(coordinator.WorkloadImplementationSelector), Resolver: conformanceResolver{}, Binder: binding.BindingPlanBinder{Handlers: handlers, Validator: binding.DesiredPlanValidator{RuntimeKind: "Fake", Objects: conformanceValidator{}}, Workloads: binding.WorkloadAssemblerFunc(func(_ context.Context, _ binding.BindRequest, plan *executionplan.Plan) error {
		if len(plan.Processes) != 0 {
			return fault.New(fault.FailedPrecondition, "unsupported required process")
		}
		return nil
	}), Lowerer: conformanceLowerer{}}}
	selector := coordination.RuntimeSelector{ID: "fixture", Kind: "Fake"}
	h := &Host{Tasks: tasks, Eligibility: testEligibility{}, Status: KubernetesStatusWriter{Client: tasks}, Claimant: "cluster/Fake", Bundle: Bundle{RuntimeSelector: selector, Implementations: registry, Lifecycle: testLifecycle{}, Executor: CoordinatorExecutor{Coordinator: c, Selector: selector}, Decoder: TaskDecoderFunc(func(task *rlarkv1alpha1.Task) (coordination.TaskSnapshot, error) {
		return coordination.TaskSnapshot{Identity: agentruntime.TaskIdentity{Logical: agentruntime.LogicalTaskID{Namespace: task.Namespace, Name: task.Name}, ObjectUID: string(task.UID), Generation: task.Generation}, WorkloadSelector: coordination.WorkloadSelector{Type: "FakeWorkload", Version: "v1"}, CarrierSelector: coordination.CarrierSelector{Type: "FakeCarrier", Version: "v1"}}, nil
	}), Admit: func(ctx context.Context, input LifecycleInput) error { return c.AdmitTask(ctx, input.Snapshot) }, StatusFromReconcile: func(result coordinator.ReconcileResult, s coordination.TaskSnapshot) *StatusIntent {
		return &StatusIntent{ObjectUID: s.Identity.ObjectUID, Generation: s.Identity.Generation, Phase: string(result.Evaluation.Observation.Phase)}
	}}}
	if err := h.Validate(); err != nil {
		t.Fatal(err)
	}
	return h
}

func conformanceFixture(t *testing.T) (*Host, client.Client, *conformanceStore) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	tasks := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Task{}).WithObjects(hostTask()).Build()
	store := &conformanceStore{objects: make(map[string]conformanceObject)}
	return newConformanceHost(t, tasks, store), tasks, store
}
func conformanceReconcile(t *testing.T, h *Host, outcome Outcome) {
	t.Helper()
	r, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask()))
	if err != nil || r.Outcome != outcome {
		t.Fatalf("outcome=%s want=%s err=%v reason=%s", r.Outcome, outcome, err, r.Reason)
	}
}
func conformanceUpdate(t *testing.T, tasks client.Client, mutate func(*rlarkv1alpha1.Task)) {
	t.Helper()
	var task rlarkv1alpha1.Task
	if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &task); err != nil {
		t.Fatal(err)
	}
	mutate(&task)
	if err := tasks.Update(context.Background(), &task); err != nil {
		t.Fatal(err)
	}
}

func TestConformanceLifecycleDelayedCleanupAndReconstruction(t *testing.T) {
	h, tasks, store := conformanceFixture(t)
	conformanceReconcile(t, h, OutcomeSucceeded)
	assertHostClaimPreserved(t, tasks, hostTask())
	if len(store.applies) != 2 || store.applies[0].Desired.LogicalName != "config" || store.applies[1].Desired.LogicalName != "root" {
		t.Fatal("dependency order changed")
	}
	for range 3 {
		conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeSucceeded)
	}
	if len(store.applies) != 2 {
		t.Fatal("duplicate reconcile/restart wrote unchanged objects")
	}
	store.delayDelete = true
	conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) { task.Spec.Suspended = true })
	conformanceReconcile(t, h, OutcomeCleanupInProgress)
	assertHostClaimPreserved(t, tasks, hostTask())
	if store.deletes[0].Ref.Name != "task-root" {
		t.Fatal("cleanup did not delete dependants first")
	}
	store.delayDelete = false
	conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeCleanupInProgress)
	conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeWaitingForChange)
	assertHostClaimPreserved(t, tasks, hostTask())
	conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) { task.Spec.Suspended = false; task.Generation++ })
	conformanceReconcile(t, h, OutcomeSucceeded)
	if len(store.objects) != 2 || len(store.applies) != 4 {
		t.Fatal("resume did not recreate missing objects")
	}
	store.delayDelete = true
	if err := tasks.Delete(context.Background(), hostTask()); err != nil {
		t.Fatal(err)
	}
	conformanceReconcile(t, h, OutcomeCleanupInProgress)
	assertHostClaimPreserved(t, tasks, hostTask())
	store.unavailable = true
	if _, err := newConformanceHost(t, tasks, store).Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask())); err == nil {
		t.Fatal("unavailable backend treated as empty")
	}
	assertHostClaimPreserved(t, tasks, hostTask())
	store.unavailable, store.delayDelete = false, false
	conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeCleanupInProgress)
	conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeSucceeded)
	var deleted rlarkv1alpha1.Task
	if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &deleted); !apierrors.IsNotFound(err) {
		t.Fatalf("Task not finalized: %v", err)
	}
}

func TestConformancePartialApplyAndLostReplyRecoverFromStore(t *testing.T) {
	for _, lost := range []bool{false, true} {
		t.Run(fmt.Sprint(lost), func(t *testing.T) {
			h, tasks, store := conformanceFixture(t)
			if lost {
				store.loseReplyAt = 2
			} else {
				store.failApplyAt = 2
			}
			if _, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask())); err == nil {
				t.Fatal("fault did not interrupt reconcile")
			}
			assertHostClaimPreserved(t, tasks, hostTask())
			first := store.objects["task-config"].Ref.UID
			conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeSucceeded)
			want := 3
			if lost {
				want = 2
			}
			if len(store.applies) != want || len(store.objects) != 2 || store.objects["task-config"].Ref.UID != first {
				t.Fatal("restart replayed a successful effect")
			}
			if !lost && store.applies[1].IdempotencyKey != store.applies[2].IdempotencyKey {
				t.Fatal("retry operation lost stable idempotency key")
			}
			conformanceReconcile(t, h, OutcomeSucceeded)
			if len(store.applies) != want {
				t.Fatal("steady state wrote objects")
			}
		})
	}
}

func TestConformanceClaimAndStatusConflicts(t *testing.T) {
	h, tasks, store := conformanceFixture(t)
	h.Tasks = &conflictOncePatchClient{Client: tasks, conflict: true}
	if _, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask())); !apierrors.IsConflict(err) {
		t.Fatalf("claim error=%v", err)
	}
	if len(store.applies) != 0 {
		t.Fatal("execution preceded successful claim")
	}
	conflict := true
	h.Status = statusWriterFunc(func(ctx context.Context, task *rlarkv1alpha1.Task, intent StatusIntent) error {
		if conflict {
			conflict = false
			return apierrors.NewConflict(rlarkv1alpha1.GroupVersion.WithResource("tasks").GroupResource(), task.Name, errors.New("status conflict"))
		}
		return (KubernetesStatusWriter{Client: tasks}).WriteStatus(ctx, task, intent)
	})
	if _, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask())); !apierrors.IsConflict(err) {
		t.Fatalf("status error=%v", err)
	}
	conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeSucceeded)
	if len(store.applies) != 2 {
		t.Fatal("status conflict replayed Runtime writes")
	}
}

func TestConformanceForeignClaimsAndUIDReplacementHaveNoEffects(t *testing.T) {
	h, tasks, store := conformanceFixture(t)
	conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) {
		task.Finalizers = []string{TaskFinalizer}
		task.Annotations = map[string]string{TaskClaimantAnnotation: "other/Fake"}
	})
	conformanceReconcile(t, h, OutcomeWaitingForChange)
	if len(store.applies)+len(store.deletes) != 0 {
		t.Fatal("foreign claim reached backend effects")
	}
	conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) { task.Finalizers = nil; task.Annotations = nil })
	conformanceReconcile(t, h, OutcomeSucceeded)
	// Simulate loss of the old management object without deleting the backend.
	conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) { task.Finalizers = nil })
	if err := tasks.Delete(context.Background(), hostTask()); err != nil {
		t.Fatal(err)
	}
	replacement := hostTask()
	replacement.UID = types.UID("replacement")
	if err := tasks.Create(context.Background(), replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := newConformanceHost(t, tasks, store).Reconcile(context.Background(), client.ObjectKeyFromObject(replacement)); !fault.Is(err, fault.Conflict) {
		t.Fatalf("replacement adopted old UID: %v", err)
	}
	if len(store.applies) != 2 || len(store.deletes) != 0 {
		t.Fatal("replacement changed foreign incarnation")
	}
}

func TestConformanceSerializationAndCancellation(t *testing.T) {
	h, tasks, store := conformanceFixture(t)
	other := hostTask()
	other.Name, other.UID = "other", "other-uid"
	if err := tasks.Create(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	blocking := &blockingLifecycle{entered: make(chan struct{}, 3), release: make(chan struct{})}
	t.Cleanup(func() {
		select {
		case <-blocking.release:
		default:
			close(blocking.release)
		}
	})
	h.Bundle.Lifecycle = blocking
	results := make(chan error, 3)
	go func() {
		_, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask()))
		results <- err
	}()
	select {
	case <-blocking.entered:
	case err := <-results:
		t.Fatalf("first reconcile exited before lifecycle: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("first reconcile did not reach lifecycle")
	}
	go func() {
		_, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask()))
		results <- err
	}()
	go func() { _, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(other)); results <- err }()
	select {
	case <-blocking.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("different Tasks were globally serialized")
	}
	select {
	case <-blocking.entered:
		t.Fatal("same Task entered lifecycle concurrently")
	case <-time.After(20 * time.Millisecond):
	}
	close(blocking.release)
	for range 3 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if len(store.applies) != 4 {
		t.Fatal("concurrent duplicate execution wrote extra objects")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Reconcile(ctx, client.ObjectKeyFromObject(hostTask())); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
	if len(store.applies) != 4 {
		t.Fatal("cancelled reconcile wrote objects")
	}
}

func conformanceReplaceAdapter(t *testing.T, h *Host, adapter conformanceAdapter) {
	t.Helper()
	r, err := NewImplementationRegistry(context.Background(), []WorkloadRegistration{{Key: ImplementationKey{WorkloadType: "FakeWorkload", WorkloadVersion: "v1", RuntimeKind: "Fake"}, Adapter: adapter}})
	if err != nil {
		t.Fatal(err)
	}
	h.Bundle.Implementations = r
	h.Bundle.Executor.(CoordinatorExecutor).Coordinator.Implementations = r.(coordinator.WorkloadImplementationSelector)
}

func TestConformanceAdmissionRejectsCapabilitiesAndSemanticsBeforeLifecycle(t *testing.T) {
	for _, missing := range []string{"inventory", "profile", "environment", "units", "processes", "lowerer", "assembler"} {
		t.Run(missing, func(t *testing.T) {
			h, tasks, store := conformanceFixture(t)
			c := h.Bundle.Executor.(CoordinatorExecutor).Coordinator
			switch missing {
			case "inventory":
				c.Runtimes[0].Info.Capabilities = nil
			case "profile":
				conformanceReplaceAdapter(t, h, conformanceAdapter{badProfile: true})
			case "environment", "units", "processes":
				conformanceReplaceAdapter(t, h, conformanceAdapter{badSemantics: missing})
			case "lowerer", "assembler":
				b := c.Binder.(binding.BindingPlanBinder)
				if missing == "lowerer" {
					b.Lowerer = nil
				} else {
					b.Workloads = nil
				}
				c.Binder = b
			}
			lifecycleEffects := 0
			h.Bundle.Lifecycle = mutatingLifecycle{mutate: func(*rlarkv1alpha1.Task) { lifecycleEffects++ }}
			result, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask()))
			if err != nil || result.Outcome != OutcomeBlocked || result.Status == nil || result.Retry.Class != RetryOnChange {
				t.Fatalf("admission result=%s err=%v", result.Outcome, err)
			}
			if lifecycleEffects+len(store.applies)+len(store.deletes) != 0 {
				t.Fatal("rejected contract caused effects")
			}
			var task rlarkv1alpha1.Task
			if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &task); err != nil {
				t.Fatal(err)
			}
			if contains(task.Finalizers, TaskFinalizer) {
				t.Fatal("invalid execution was claimed")
			}
			assertAdmissionCondition(t, &task, metav1.ConditionFalse)
		})
	}
}

func TestConformanceCleanupDoesNotRequireCurrentCompilation(t *testing.T) {
	h, tasks, store := conformanceFixture(t)
	conformanceReconcile(t, h, OutcomeSucceeded)
	conformanceReplaceAdapter(t, h, conformanceAdapter{badProfile: true})
	if err := tasks.Delete(context.Background(), hostTask()); err != nil {
		t.Fatal(err)
	}
	conformanceReconcile(t, h, OutcomeCleanupInProgress)
	assertHostClaimPreserved(t, tasks, hostTask())
	conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeSucceeded)
	if len(store.objects) != 0 || len(store.applies) != 2 {
		t.Fatal("identity-only cleanup compiled invalid current workload")
	}
}

func assertAdmissionCondition(t *testing.T, task *rlarkv1alpha1.Task, status metav1.ConditionStatus) {
	t.Helper()
	c := apimeta.FindStatusCondition(task.Status.Conditions, ConditionExecutionAdmitted)
	reason := "ExecutionAdmissionSucceeded"
	if status == metav1.ConditionFalse {
		reason = "ExecutionAdmissionFailed"
	}
	if c == nil || c.Status != status || c.Reason != reason || c.Message == "" || c.ObservedGeneration != task.Generation {
		t.Fatalf("admission condition=%#v generation=%d", c, task.Generation)
	}
}

func TestConformanceAdmissionVisibilityIdempotenceAndRecovery(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		t.Run(fmt.Sprint(claimed), func(t *testing.T) {
			h, tasks, store := conformanceFixture(t)
			if claimed {
				conformanceReconcile(t, h, OutcomeSucceeded)
			}
			conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) { task.Generation++ })
			var before rlarkv1alpha1.Task
			if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &before); err != nil {
				t.Fatal(err)
			}
			before.Status.Message = "retained observation"
			before.Status.ObservedNodes = []string{"node-a"}
			before.Status.Conditions = append(before.Status.Conditions, metav1.Condition{Type: "OtherOwner", Status: metav1.ConditionTrue, Reason: "Observed", Message: "retained", LastTransitionTime: metav1.Now()})
			if err := tasks.Status().Update(context.Background(), &before); err != nil {
				t.Fatal(err)
			}
			conformanceReplaceAdapter(t, h, conformanceAdapter{badProfile: true})
			applies := len(store.applies)
			conformanceReconcile(t, h, OutcomeBlocked)
			var rejected rlarkv1alpha1.Task
			if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &rejected); err != nil {
				t.Fatal(err)
			}
			assertAdmissionCondition(t, &rejected, metav1.ConditionFalse)
			if rejected.Status.ObservedGeneration != before.Status.ObservedGeneration || rejected.Status.ObservedGeneration == rejected.Generation {
				t.Fatal("current admission rejection refreshed stale workload generation")
			}
			if claimed && rejected.Status.Phase != rlarkv1alpha1.TaskPhaseRunning {
				t.Fatal("rejection lost the previous Running observation")
			}
			if rejected.Status.Phase != before.Status.Phase || rejected.Status.Message != before.Status.Message || !reflect.DeepEqual(rejected.Status.ObservedNodes, before.Status.ObservedNodes) || !reflect.DeepEqual(apimeta.FindStatusCondition(rejected.Status.Conditions, "OtherOwner"), apimeta.FindStatusCondition(before.Status.Conditions, "OtherOwner")) || len(store.applies) != applies || len(store.deletes) != 0 || contains(rejected.Finalizers, TaskFinalizer) != claimed {
				t.Fatal("rejection changed lifecycle or Runtime state")
			}
			conformanceReconcile(t, h, OutcomeBlocked)
			var repeated rlarkv1alpha1.Task
			if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &repeated); err != nil {
				t.Fatal(err)
			}
			if repeated.ResourceVersion != rejected.ResourceVersion {
				t.Fatal("identical rejection patched Status again")
			}
			conformanceReplaceAdapter(t, h, conformanceAdapter{})
			conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) { task.Generation++ })
			conformanceReconcile(t, h, OutcomeSucceeded)
			if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &repeated); err != nil {
				t.Fatal(err)
			}
			assertAdmissionCondition(t, &repeated, metav1.ConditionTrue)
			if repeated.Status.ObservedGeneration != repeated.Generation {
				t.Fatal("recovered workload did not refresh top-level generation")
			}
			if len(store.objects) != 2 {
				t.Fatal("corrected configuration did not execute")
			}
			conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) { task.Spec.Suspended = true })
			conformanceReplaceAdapter(t, h, conformanceAdapter{badProfile: true})
			conformanceReconcile(t, h, OutcomeCleanupInProgress)
			conformanceReconcile(t, h, OutcomeWaitingForChange)
			if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &repeated); err != nil {
				t.Fatal(err)
			}
			if apimeta.FindStatusCondition(repeated.Status.Conditions, ConditionExecutionAdmitted) != nil {
				t.Fatal("suspended Task retained execution admission")
			}
		})
	}
}

func TestConformanceAdmissionStatusFailureCannotReleaseExecution(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		for _, conflict := range []bool{false, true} {
			t.Run(fmt.Sprintf("rejected=%t/conflict=%t", rejected, conflict), func(t *testing.T) {
				h, tasks, store := conformanceFixture(t)
				if rejected {
					conformanceReplaceAdapter(t, h, conformanceAdapter{badProfile: true})
				}
				failure := errors.New("status unavailable")
				class := RetryTransient
				if conflict {
					failure = apierrors.NewConflict(rlarkv1alpha1.GroupVersion.WithResource("tasks").GroupResource(), "task", failure)
					class = RetryConflict
				}
				h.Status = statusWriterFunc(func(context.Context, *rlarkv1alpha1.Task, StatusIntent) error { return failure })
				result, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask()))
				if !errors.Is(err, failure) || result.Outcome != OutcomeRetrying || result.Retry.Class != class || len(store.applies)+len(store.deletes) != 0 {
					t.Fatalf("status failure result=%#v err=%v", result, err)
				}
				var task rlarkv1alpha1.Task
				if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &task); err != nil {
					t.Fatal(err)
				}
				if contains(task.Finalizers, TaskFinalizer) {
					t.Fatal("failed Status write permitted claim")
				}
				h.Status = KubernetesStatusWriter{Client: tasks}
				outcome := OutcomeSucceeded
				if rejected {
					outcome = OutcomeBlocked
				}
				conformanceReconcile(t, h, outcome)
			})
		}
	}
}

func TestConformanceAdmissionTransientErrorsPreserveChains(t *testing.T) {
	for _, failure := range []error{context.Canceled, context.DeadlineExceeded, fault.Wrap(fault.InvalidArgument, "cancelled validation", context.Canceled), fault.New(fault.Unavailable, "backend unavailable")} {
		h, tasks, store := conformanceFixture(t)
		h.Bundle.Admit = func(context.Context, LifecycleInput) error { return failure }
		result, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask()))
		if !errors.Is(err, failure) || result.Outcome != OutcomeRetrying || len(store.applies)+len(store.deletes) != 0 {
			t.Fatalf("transient result=%#v err=%v", result, err)
		}
		var task rlarkv1alpha1.Task
		if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &task); err != nil {
			t.Fatal(err)
		}
		if len(task.Finalizers) != 0 || apimeta.FindStatusCondition(task.Status.Conditions, ConditionExecutionAdmitted) != nil {
			t.Fatal("transient admission failure published a permanent result")
		}
	}
}

func TestConformanceAdmissionDoesNotPublishTransportCause(t *testing.T) {
	h, tasks, _ := conformanceFixture(t)
	h.Bundle.Admit = func(context.Context, LifecycleInput) error {
		return fault.Wrap(fault.InvalidArgument, "remote message secret-token", errors.New("transport secret-token"))
	}
	conformanceReconcile(t, h, OutcomeBlocked)
	var task rlarkv1alpha1.Task
	if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &task); err != nil {
		t.Fatal(err)
	}
	assertAdmissionCondition(t, &task, metav1.ConditionFalse)
	if strings.Contains(apimeta.FindStatusCondition(task.Status.Conditions, ConditionExecutionAdmitted).Message, "secret-token") {
		t.Fatal("transport message/cause leaked into Status")
	}
}

func TestConformanceLostAndReorderedHintsRepairCurrentGeneration(t *testing.T) {
	h, tasks, store := conformanceFixture(t)
	conformanceReconcile(t, h, OutcomeSucceeded)
	conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) { task.Generation = 9 })
	accumulator := trigger.NewAccumulator(trigger.Config{MaxKeys: 1})
	key := trigger.Key{Namespace: "cluster", Name: "task"}
	// The desired-state notification was lost; delayed, duplicate Runtime hints
	// cannot replace current Task state. A repair wakeup uses only the queue key.
	for _, generation := range []int64{8, 2, 8} {
		accumulator.Merge(trigger.Trigger{Key: key, Kind: trigger.RuntimeState, ObjectUID: "old-uid", Generation: generation})
	}
	accumulator.WakeupFailed(key) // bounded wakeup channel was full
	accumulator.Merge(trigger.Trigger{Key: key, Kind: trigger.Periodic})
	batch, ok := accumulator.Take(key)
	if !ok || !batch.FullReconcileRequired {
		t.Fatal("periodic hint did not force full repair")
	}
	conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeSucceeded)
	accumulator.Done(key)
	if len(store.applies) != 4 || store.applies[2].Task.Generation != 9 || store.applies[3].Task.Generation != 9 {
		t.Fatal("hint payload was treated as authoritative generation")
	}
	var task rlarkv1alpha1.Task
	if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &task); err != nil {
		t.Fatal(err)
	}
	writer := KubernetesStatusWriter{Client: tasks}
	for _, stale := range []StatusIntent{{ObjectUID: "old-uid", Generation: 9, Phase: "Failed"}, {ObjectUID: string(task.UID), Generation: 2, Phase: "Failed"}} {
		stale.Conditions = []ConditionMutation{{Condition: metav1.Condition{Type: ConditionExecutionAdmitted, Status: metav1.ConditionFalse, Reason: "ExecutionAdmissionFailed", ObservedGeneration: stale.Generation}}}
		if err := writer.WriteStatus(context.Background(), &task, stale); err != nil {
			t.Fatal(err)
		}
	}
	if task.Status.Phase != rlarkv1alpha1.TaskPhaseRunning || task.Status.ObservedGeneration != 9 {
		t.Fatal("stale intent replaced current observation")
	}
	assertAdmissionCondition(t, &task, metav1.ConditionTrue)
	// Accumulator saturation requires conservative repair even without a batch.
	accumulator.Merge(trigger.Trigger{Key: trigger.Key{Namespace: "cluster", Name: "pressure"}, Kind: trigger.DesiredState})
	fallback := accumulator.Merge(trigger.Trigger{Key: key, Kind: trigger.Periodic})
	if fallback.Status != trigger.MergeSaturated || !fallback.Fallback.FullReconcileRequired {
		t.Fatal("saturation dropped repair")
	}
	conformanceReconcile(t, h, OutcomeSucceeded)
	if len(store.applies) != 4 {
		t.Fatal("fallback repair replayed writes")
	}
}

func TestConformanceInFlightCancellationPreservesClaimAndRecovers(t *testing.T) {
	h, tasks, store := conformanceFixture(t)
	store.applyEntered, store.applyRelease = make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := h.Reconcile(ctx, client.ObjectKeyFromObject(hostTask())); result <- err }()
	select {
	case <-store.applyEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("apply never entered")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("in-flight Apply ignored cancellation")
	}
	assertHostClaimPreserved(t, tasks, hostTask())
	if len(store.objects) != 0 {
		t.Fatal("cancelled Apply wrote backend")
	}
	store.applyEntered = nil
	conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeSucceeded)
	if len(store.objects) != 2 {
		t.Fatal("cancelled execution could not resume")
	}
}

func TestConformanceLostDeleteReplyRetainsFinalizerUntilFreshInventory(t *testing.T) {
	h, tasks, store := conformanceFixture(t)
	conformanceReconcile(t, h, OutcomeSucceeded)
	store.loseDeleteReplyAt = 1
	if err := tasks.Delete(context.Background(), hostTask()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask())); err == nil {
		t.Fatal("lost delete response did not interrupt cleanup")
	}
	assertHostClaimPreserved(t, tasks, hostTask())
	if len(store.objects) != 1 {
		t.Fatal("delete fault did not persist its effect")
	}
	conformanceReconcile(t, newConformanceHost(t, tasks, store), OutcomeSucceeded)
	if len(store.deletes) != 2 || store.deletes[1].Ref.Name != "task-config" {
		t.Fatal("reconstruction replayed a confirmed absent deletion")
	}
}
