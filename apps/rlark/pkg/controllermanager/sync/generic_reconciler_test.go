package sync

import (
	"context"
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
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
