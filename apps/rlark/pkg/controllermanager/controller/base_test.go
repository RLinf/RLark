package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

// stubReconciler drives one scripted state-machine pass. It deliberately does
// not implement ObservedGenerationRecorder, so the tests can distinguish the
// shell's behaviour for resources that carry the field from those that do not.
type stubReconciler struct {
	client.Client
	terminal bool
	pass     func(obj client.Object) (bool, error)
}

func (r *stubReconciler) ReconcileStateMachine(_ context.Context, obj client.Object) (bool, error) {
	if r.pass == nil {
		return false, nil
	}
	return r.pass(obj)
}

func (r *stubReconciler) IsTerminal(client.Object) bool { return r.terminal }

// observedGenerationReconciler adds the optional interface.
type observedGenerationReconciler struct {
	stubReconciler
	calls int
}

func (r *observedGenerationReconciler) SetObservedGeneration(obj client.Object, generation int64) bool {
	r.calls++
	wf := obj.(*rlarkv1alpha1.Workflow)
	if wf.Status.ObservedGeneration == generation {
		return false
	}
	wf.Status.ObservedGeneration = generation
	return true
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func newWorkflow() *rlarkv1alpha1.Workflow {
	return &rlarkv1alpha1.Workflow{
		ObjectMeta: metav1.ObjectMeta{Name: "workflow", Namespace: "default", Generation: 3},
	}
}

func newFakeClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	return fake.NewClientBuilder().
		WithScheme(testScheme(t)).
		WithObjects(objs...).
		WithStatusSubresource(&rlarkv1alpha1.Workflow{}).
		Build()
}

// deterministicShell removes jitter so that interval assertions can be exact.
// Production wiring uses NewShell, which keeps jitter on.
func deterministicShell(recorder record.EventRecorder) Shell {
	backoff := sharedreconcile.DefaultBackoff()
	backoff.Rand = func() float64 { return 0 }
	return Shell{Backoff: backoff, Recorder: recorder}
}

func requestFor(obj client.Object) ctrl.Request {
	return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)}
}

// A pass that stopped children but has not seen them disappear must come back
// on its own: there is no incoming watch event that proves the children are
// gone, so losing this requeue would strand the object forever.
func TestRunStateMachineRequeuesChildCleanupWithoutError(t *testing.T) {
	wf := newWorkflow()
	r := &stubReconciler{
		Client: newFakeClient(t, wf),
		pass: func(client.Object) (bool, error) {
			return false, ErrRequeueAfterChildCleanup
		},
	}

	result, err := deterministicShell(nil).RunStateMachine(
		context.Background(), requestFor(wf), &rlarkv1alpha1.Workflow{}, "workflow", r)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != ChildCleanupRequeue {
		t.Fatalf("RequeueAfter = %v, want %v", result.RequeueAfter, ChildCleanupRequeue)
	}
}

// A successful pass must not schedule a poll. Requeueing after success would
// turn every healthy object into a permanent timer, which is exactly the load
// the watch-driven model exists to avoid.
func TestRunStateMachineDoesNotPollAfterSuccess(t *testing.T) {
	wf := newWorkflow()
	r := &stubReconciler{Client: newFakeClient(t, wf)}

	result, err := deterministicShell(nil).RunStateMachine(
		context.Background(), requestFor(wf), &rlarkv1alpha1.Workflow{}, "workflow", r)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}

// A deleted object is not an error and not something to retry.
func TestRunStateMachineDropsMissingObject(t *testing.T) {
	wf := newWorkflow()
	r := &stubReconciler{
		Client: newFakeClient(t),
		pass: func(client.Object) (bool, error) {
			t.Fatal("state machine ran for an object that no longer exists")
			return false, nil
		},
	}

	result, err := deterministicShell(nil).RunStateMachine(
		context.Background(), requestFor(wf), &rlarkv1alpha1.Workflow{}, "workflow", r)
	if err != nil {
		t.Fatalf("missing object reported as an error: %v", err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}

// ObservedGeneration is the contract consumers read to decide whether a
// condition describes the Spec they submitted. The shell must advance it, and
// must persist it, when a pass completes.
func TestRunStateMachineAdvancesObservedGenerationOnSuccess(t *testing.T) {
	wf := newWorkflow()
	fakeClient := newFakeClient(t, wf)
	r := &observedGenerationReconciler{stubReconciler: stubReconciler{Client: fakeClient}}

	if _, err := deterministicShell(nil).RunStateMachine(
		context.Background(), requestFor(wf), &rlarkv1alpha1.Workflow{}, "workflow", r); err != nil {
		t.Fatal(err)
	}

	if r.calls != 1 {
		t.Fatalf("SetObservedGeneration called %d times, want exactly 1", r.calls)
	}

	stored := &rlarkv1alpha1.Workflow{}
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(wf), stored); err != nil {
		t.Fatal(err)
	}
	if stored.Status.ObservedGeneration != wf.Generation {
		t.Fatalf("stored observedGeneration = %d, want %d", stored.Status.ObservedGeneration, wf.Generation)
	}
}

// A pass that is still waiting for child cleanup has not finished processing
// the current Spec, so claiming it has would let a consumer read a stale
// condition as current.
func TestRunStateMachineKeepsObservedGenerationWhileCleaningUp(t *testing.T) {
	wf := newWorkflow()
	r := &observedGenerationReconciler{stubReconciler: stubReconciler{
		Client: newFakeClient(t, wf),
		pass: func(client.Object) (bool, error) {
			return false, ErrRequeueAfterChildCleanup
		},
	}}

	if _, err := deterministicShell(nil).RunStateMachine(
		context.Background(), requestFor(wf), &rlarkv1alpha1.Workflow{}, "workflow", r); err != nil {
		t.Fatal(err)
	}
	if r.calls != 0 {
		t.Fatalf("SetObservedGeneration called %d times during cleanup, want 0", r.calls)
	}
}

// Same invariant on the failure path.
func TestRunStateMachineKeepsObservedGenerationOnFailure(t *testing.T) {
	wf := newWorkflow()
	failure := errors.New("boom")
	r := &observedGenerationReconciler{stubReconciler: stubReconciler{
		Client: newFakeClient(t, wf),
		pass: func(client.Object) (bool, error) {
			return false, failure
		},
	}}
	recorder := record.NewFakeRecorder(4)

	_, err := deterministicShell(recorder).RunStateMachine(
		context.Background(), requestFor(wf), &rlarkv1alpha1.Workflow{}, "workflow", r)
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want %v", err, failure)
	}
	if r.calls != 0 {
		t.Fatalf("SetObservedGeneration called %d times on failure, want 0", r.calls)
	}

	select {
	case event := <-recorder.Events:
		if !strings.Contains(event, ReasonReconcileFailed) {
			t.Fatalf("event %q does not carry reason %q", event, ReasonReconcileFailed)
		}
	default:
		t.Fatal("no event emitted for a failed reconcile pass")
	}
}

// A terminal object must not be touched and must not be polled.
func TestRunStateMachineSkipsTerminalObject(t *testing.T) {
	wf := newWorkflow()
	r := &stubReconciler{
		Client:   newFakeClient(t, wf),
		terminal: true,
		pass: func(client.Object) (bool, error) {
			t.Fatal("state machine ran for a terminal object")
			return false, nil
		},
	}

	result, err := deterministicShell(nil).RunStateMachine(
		context.Background(), requestFor(wf), &rlarkv1alpha1.Workflow{}, "workflow", r)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}

// Run is the entry point for controllers that own their own pass. It must
// translate the retry class rather than require the caller to pick a duration.
func TestRunTranslatesRetryClassIntoInterval(t *testing.T) {
	shell := deterministicShell(nil)

	result, err := shell.Run(context.Background(), "widget", func(context.Context) (Result, error) {
		return Result{
			Outcome: sharedreconcile.OutcomeRetrying,
			Reason:  "Testing",
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryTransient},
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != sharedreconcile.DefaultTransientBackoff {
		t.Fatalf("RequeueAfter = %v, want %v", result.RequeueAfter, sharedreconcile.DefaultTransientBackoff)
	}
}

// An error must reach controller-runtime unchanged so the workqueue keeps its
// exponential rate limiting; the shell must not convert it into a fixed-rate
// requeue.
func TestRunPropagatesErrorWithoutRequeueAfter(t *testing.T) {
	failure := errors.New("boom")

	result, err := deterministicShell(nil).Run(context.Background(), "widget",
		func(context.Context) (Result, error) {
			return Result{Retry: sharedreconcile.RetryDirective{Class: sharedreconcile.RetryTransient}}, failure
		})
	if !errors.Is(err, failure) {
		t.Fatalf("err = %v, want %v", err, failure)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0", result.RequeueAfter)
	}
}

// Metrics must be optional: a controller built without them behaves the same.
func TestRunWorksWithoutMetrics(t *testing.T) {
	if _, err := (Shell{}).Run(context.Background(), "widget",
		func(context.Context) (Result, error) { return Result{}, nil }); err != nil {
		t.Fatal(err)
	}
}

// Registering twice against the same registry must fail loudly instead of
// panicking inside a constructor.
func TestNewMetricsReportsDuplicateRegistration(t *testing.T) {
	registry := prometheus.NewRegistry()
	if _, err := NewMetrics(registry); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMetrics(registry); err == nil {
		t.Fatal("second registration against the same registry succeeded")
	}
}
