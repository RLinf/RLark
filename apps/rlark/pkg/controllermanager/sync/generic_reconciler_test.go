package sync

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
)

func TestReconcileAddsSyncFinalizerAfterPersistence(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	r := &genericReconciler[*rlarkv1alpha1.Job]{
		client:  c,
		handler: &genericSyncHandler{},
		newObj:  func() *rlarkv1alpha1.Job { return &rlarkv1alpha1.Job{} },
		syncFn:  func(context.Context, *rlarkv1alpha1.Job) error { return nil },
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
		t.Fatal(err)
	}
	if len(job.Finalizers) != 1 || job.Finalizers[0] != SyncFinalizer {
		t.Fatalf("sync finalizer not added: %v", job.Finalizers)
	}
}

func TestReconcileDoesNotAddSyncFinalizerWhenPersistenceFails(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	r := &genericReconciler[*rlarkv1alpha1.Job]{
		client:  c,
		handler: &genericSyncHandler{},
		newObj:  func() *rlarkv1alpha1.Job { return &rlarkv1alpha1.Job{} },
		syncFn:  func(context.Context, *rlarkv1alpha1.Job) error { return errors.New("database unavailable") },
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err == nil {
		t.Fatal("expected persistence error")
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
		t.Fatal(err)
	}
	if len(job.Finalizers) != 0 {
		t.Fatalf("sync finalizer added before persistence succeeded: %v", job.Finalizers)
	}
}

func TestReconcileSkipsPersistenceAndFinalizerForSkippedObject(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job", Annotations: map[string]string{"skip-sync": ""}}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	called := false
	r := &genericReconciler[*rlarkv1alpha1.Job]{
		client:  c,
		handler: &genericSyncHandler{},
		newObj:  func() *rlarkv1alpha1.Job { return &rlarkv1alpha1.Job{} },
		syncFn: func(context.Context, *rlarkv1alpha1.Job) error {
			called = true
			return nil
		},
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("skipped object was persisted")
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
		t.Fatal(err)
	}
	if len(job.Finalizers) != 0 {
		t.Fatalf("sync finalizer added to skipped object: %v", job.Finalizers)
	}
}

func TestReconcileRemovesOnlySyncFinalizerFromSkippedObject(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:        "job",
		Annotations: map[string]string{"skip-sync": "true"},
		Finalizers:  []string{"example.com/cleanup", SyncFinalizer},
	}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	r := &genericReconciler[*rlarkv1alpha1.Job]{
		client:  c,
		handler: &genericSyncHandler{},
		newObj:  func() *rlarkv1alpha1.Job { return &rlarkv1alpha1.Job{} },
		syncFn: func(context.Context, *rlarkv1alpha1.Job) error {
			return errors.New("must not persist")
		},
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
		t.Fatal(err)
	}
	if len(job.Finalizers) != 1 || job.Finalizers[0] != "example.com/cleanup" {
		t.Fatalf("unexpected finalizers after skip: %v", job.Finalizers)
	}
}

func TestRemoveFinalizerDoesNotMutateInputSlice(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:       "job",
		Finalizers: []string{SyncFinalizer, "example.com/cleanup"},
	}}
	original := job.Finalizers
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	r := &genericReconciler[*rlarkv1alpha1.Job]{client: c}

	if err := r.removeFinalizer(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if original[0] != SyncFinalizer || original[1] != "example.com/cleanup" {
		t.Fatalf("input finalizer backing array was mutated: %v", original)
	}
}

// updateFailingClient makes every metadata write fail, which is how a finalizer
// write failure is reproduced without a real API server.
type updateFailingClient struct {
	client.Client
	err error
}

func (c *updateFailingClient) Update(context.Context, client.Object, ...client.UpdateOption) error {
	return c.err
}

func newSyncScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

// A failed finalizer write must surface as an error so the pass is retried
// through the workqueue's rate limiter. Returning a classified success instead
// would leave the finalizer unreleased with nothing scheduled to retry it.
func TestReconcileReturnsErrorWhenFinalizerWriteFails(t *testing.T) {
	scheme := newSyncScheme(t)
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job"}}
	base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	writeErr := apierrors.NewConflict(
		schema.GroupResource{Group: rlarkv1alpha1.GroupVersion.Group, Resource: "jobs"}, job.Name, errors.New("stale"))
	r := &genericReconciler[*rlarkv1alpha1.Job]{
		client:  &updateFailingClient{Client: base, err: writeErr},
		handler: &genericSyncHandler{},
		newObj:  func() *rlarkv1alpha1.Job { return &rlarkv1alpha1.Job{} },
		syncFn:  func(context.Context, *rlarkv1alpha1.Job) error { return nil },
	}

	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err == nil {
		t.Fatal("expected the finalizer write failure to be returned")
	}
}

// The reason is a metric label, so a conflict (another writer won a race) stays
// distinct from a write that failed for any other cause.
func TestFinalizerFailureClassification(t *testing.T) {
	conflict := apierrors.NewConflict(schema.GroupResource{Resource: "jobs"}, "job", errors.New("stale"))
	if got := finalizerFailure(conflict).Reason; got != controller.ReasonStatusWriteConflict {
		t.Fatalf("conflict reason = %q, want %q", got, controller.ReasonStatusWriteConflict)
	}
	if got := finalizerFailure(errors.New("boom")).Reason; got != reasonFinalizerWrite {
		t.Fatalf("generic failure reason = %q, want %q", got, reasonFinalizerWrite)
	}
}

// A finalizer may only decide on its own presence and its own completion. Once
// the snapshot is persisted SyncFinalizer has nothing left to protect, so it is
// released even while unrelated finalizers still hold the object.
func TestReconcileReleasesSyncFinalizerWhileOtherFinalizersRemain(t *testing.T) {
	scheme := newSyncScheme(t)
	now := metav1.Now()
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:              "job",
		DeletionTimestamp: &now,
		Finalizers:        []string{"example.com/cleanup", SyncFinalizer},
	}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
	r := &genericReconciler[*rlarkv1alpha1.Job]{
		client:  c,
		handler: &genericSyncHandler{},
		newObj:  func() *rlarkv1alpha1.Job { return &rlarkv1alpha1.Job{} },
		syncFn:  func(context.Context, *rlarkv1alpha1.Job) error { return nil },
		shell:   controller.NewShell(nil, nil),
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0 once the finalizer is released", result.RequeueAfter)
	}
	if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(job.Finalizers, SyncFinalizer) {
		t.Fatalf("sync finalizer still held: %v", job.Finalizers)
	}
	if !slices.Contains(job.Finalizers, "example.com/cleanup") {
		t.Fatalf("foreign finalizer must not be touched: %v", job.Finalizers)
	}
}

// Deletion must converge whatever order the coexisting finalizers are removed
// in. A pass that waits for a position in the list makes the outcome depend on
// array order and deadlocks as soon as two waiters coexist.
func TestReconcileReleasesSyncFinalizerInAnyRemovalOrder(t *testing.T) {
	others := []string{"jobs.rlinf.io/task-cleanup", "rlark.io/agent-cleanup"}
	orders := [][]string{
		{SyncFinalizer, others[0], others[1]},
		{others[0], SyncFinalizer, others[1]},
		{others[0], others[1], SyncFinalizer},
	}
	for _, finalizers := range orders {
		t.Run(strings.Join(finalizers, ","), func(t *testing.T) {
			scheme := newSyncScheme(t)
			now := metav1.Now()
			job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
				Name:              "job",
				DeletionTimestamp: &now,
				Finalizers:        slices.Clone(finalizers),
			}}
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(job).Build()
			r := &genericReconciler[*rlarkv1alpha1.Job]{
				client:  c,
				handler: &genericSyncHandler{},
				newObj:  func() *rlarkv1alpha1.Job { return &rlarkv1alpha1.Job{} },
				syncFn:  func(context.Context, *rlarkv1alpha1.Job) error { return nil },
				shell:   controller.NewShell(nil, nil),
			}

			// Reconciling twice proves the release is idempotent: the second
			// pass sees an object whose finalizer is already gone.
			for range 2 {
				if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Name: job.Name}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := c.Get(context.Background(), client.ObjectKey{Name: job.Name}, job); err != nil {
				t.Fatal(err)
			}
			if slices.Contains(job.Finalizers, SyncFinalizer) {
				t.Fatalf("sync finalizer still held: %v", job.Finalizers)
			}
			if len(job.Finalizers) != len(others) {
				t.Fatalf("unexpected finalizers left: %v", job.Finalizers)
			}
		})
	}
}
