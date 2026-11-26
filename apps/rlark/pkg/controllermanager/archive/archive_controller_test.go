package archive

import (
	"context"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
)

func TestIsTerminalPhase(t *testing.T) {
	r := &JobResource{}
	tests := []struct {
		phase string
		want  bool
	}{
		{"Pending", false},
		{"Running", false},
		{"Stopped", true},
		{"Succeeded", false}, // 仅 Stopped 需要归档，其他状态先忽略
		{"Failed", false},
		{"", false},
	}
	for _, tt := range tests {
		job := &rlarkv1alpha1.Job{Status: rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhase(tt.phase)}}
		if got := r.IsTerminal(job); got != tt.want {
			t.Errorf("IsTerminal(%q) = %v, want %v", tt.phase, got, tt.want)
		}
	}
}

func TestArchiveTTLRequeue(t *testing.T) {
	res := &JobResource{}

	// 刚进入终态，未到 TTL，应延迟
	now := metav1.Now()
	job := &rlarkv1alpha1.Job{
		Status: rlarkv1alpha1.JobStatus{
			Phase:   rlarkv1alpha1.JobPhaseStopped,
			EndTime: &now,
		},
	}

	ttl := DefaultArchiveTTL
	endTime := res.GetEndTime(job)
	if endTime == nil {
		t.Fatal("expected non-nil endTime")
		return
	}
	remaining := ttl - time.Since(*endTime)
	if remaining <= 0 {
		t.Errorf("expected positive remaining for just-ended job, got %v", remaining)
	}

	// 已超过 TTL，应触发归档
	old := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	job.Status.EndTime = &old
	endTime = res.GetEndTime(job)
	remaining = ttl - time.Since(*endTime)
	if remaining > 0 {
		t.Errorf("expected negative remaining for old job, got %v", remaining)
	}
}

func TestDefaultArchiveTTL(t *testing.T) {
	if DefaultArchiveTTL <= 0 {
		t.Errorf("DefaultArchiveTTL should be positive, got %v", DefaultArchiveTTL)
	}
}

func newArchiveReconciler(t *testing.T, ttl time.Duration, objs ...client.Object) *Reconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	// The production shell is used on purpose: the TTL assertions below are
	// about the interval the shell actually hands to controller-runtime,
	// including jitter.
	return NewReconciler(c, scheme, &JobResource{}, 1, ttl, controller.NewShell(nil, nil))
}

func stoppedJob(name string, endTime time.Time) *rlarkv1alpha1.Job {
	end := metav1.NewTime(endTime)
	return &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Status: rlarkv1alpha1.JobStatus{
			Phase:   rlarkv1alpha1.JobPhaseStopped,
			EndTime: &end,
		},
	}
}

func requestFor(obj client.Object) ctrl.Request {
	return ctrl.Request{NamespacedName: client.ObjectKeyFromObject(obj)}
}

func jobExists(t *testing.T, r *Reconciler, obj client.Object) bool {
	t.Helper()
	err := r.Get(context.Background(), client.ObjectKeyFromObject(obj), &rlarkv1alpha1.Job{})
	if err == nil {
		return true
	}
	if apierrors.IsNotFound(err) {
		return false
	}
	t.Fatal(err)
	return false
}

// A TTL that has not elapsed produces no event of its own, so the pass has to
// requeue itself. Jitter may delay that wakeup but must never pull it earlier
// than the deadline: waking up early burns a pass that cannot make progress.
func TestReconcileWaitsForArchiveTTLWithoutDeleting(t *testing.T) {
	ttl := time.Hour
	endTime := time.Now()
	job := stoppedJob("waiting", endTime)
	r := newArchiveReconciler(t, ttl, job)

	result, err := r.Reconcile(context.Background(), requestFor(job))
	if err != nil {
		t.Fatal(err)
	}
	// Recomputed after the pass, so it is a lower bound on what the pass saw.
	remaining := ttl - time.Since(endTime)
	if result.RequeueAfter < remaining {
		t.Fatalf("RequeueAfter = %v, want >= %v (jitter must not fire early)", result.RequeueAfter, remaining)
	}
	if !jobExists(t, r, job) {
		t.Fatal("job was archived before its TTL elapsed")
	}
}

func TestReconcileDeletesAfterArchiveTTL(t *testing.T) {
	job := stoppedJob("elapsed", time.Now().Add(-10*time.Minute))
	r := newArchiveReconciler(t, 5*time.Minute, job)

	result, err := r.Reconcile(context.Background(), requestFor(job))
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0 for a completed archive", result.RequeueAfter)
	}
	if jobExists(t, r, job) {
		t.Fatal("job still present after its TTL elapsed")
	}
}

// Re-running the pass after the object is already gone must settle, not retry:
// the desired state is reached, so there is nothing left to drive.
func TestReconcileDropsMissingObject(t *testing.T) {
	r := newArchiveReconciler(t, 5*time.Minute)

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKey{Name: "gone", Namespace: "default"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter != 0 {
		t.Fatalf("RequeueAfter = %v, want 0 for a vanished object", result.RequeueAfter)
	}
}

func TestReconcileLeavesNonTerminalAndDeletingObjectsAlone(t *testing.T) {
	running := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "running", Namespace: "default"},
		Status:     rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseRunning},
	}
	deleting := stoppedJob("deleting", time.Now().Add(-10*time.Minute))
	deleting.Finalizers = []string{"rlark.io/test"}
	deleting.DeletionTimestamp = ptrTime(metav1.Now())

	for _, job := range []*rlarkv1alpha1.Job{running, deleting} {
		r := newArchiveReconciler(t, 5*time.Minute, job)
		result, err := r.Reconcile(context.Background(), requestFor(job))
		if err != nil {
			t.Fatalf("%s: %v", job.Name, err)
		}
		// Both cases wait for a watch event rather than polling.
		if result.RequeueAfter != 0 {
			t.Fatalf("%s: RequeueAfter = %v, want 0", job.Name, result.RequeueAfter)
		}
		if !jobExists(t, r, job) {
			t.Fatalf("%s: job was archived", job.Name)
		}
	}
}

func ptrTime(t metav1.Time) *metav1.Time { return &t }

// A terminal object whose status never recorded an end time still has to be
// archived. The TTL origin is the first pass that saw it, and that instant has
// to survive on the object: if every pass restarted the clock, the object
// would be requeued forever and never deleted.
func TestReconcileArchivesTerminalObjectMissingEndTime(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "no-end-time", Namespace: "default"},
		Status:     rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseStopped},
	}
	r := newArchiveReconciler(t, time.Hour, job)

	result, err := r.Reconcile(context.Background(), requestFor(job))
	if err != nil {
		t.Fatal(err)
	}
	if result.RequeueAfter <= 0 {
		t.Fatalf("RequeueAfter = %v, want a positive wait for a fresh object", result.RequeueAfter)
	}
	if !jobExists(t, r, job) {
		t.Fatal("job was archived on the pass that first observed it")
	}

	var stored rlarkv1alpha1.Job
	if err := r.Get(context.Background(), client.ObjectKeyFromObject(job), &stored); err != nil {
		t.Fatal(err)
	}
	recorded := stored.Annotations[ArchiveObservedSinceAnnotation]
	if recorded == "" {
		t.Fatal("archive origin was not recorded on the object")
	}

	// Backdate the recorded origin past the TTL. The next pass must read it
	// back and delete, rather than restarting the clock.
	before := stored.DeepCopy()
	stored.Annotations[ArchiveObservedSinceAnnotation] =
		time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	if err := r.Patch(context.Background(), &stored, client.MergeFrom(before)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Reconcile(context.Background(), requestFor(job)); err != nil {
		t.Fatal(err)
	}
	if jobExists(t, r, job) {
		t.Fatal("job survived a TTL measured from the first observation")
	}
}

// A corrupted origin must not decide when the object is deleted. The pass
// rewrites it and measures from that moment instead.
func TestReconcileRewritesUnparsableArchiveOrigin(t *testing.T) {
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "bad-origin",
			Namespace:   "default",
			Annotations: map[string]string{ArchiveObservedSinceAnnotation: "not-a-timestamp"},
		},
		Status: rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhaseStopped},
	}
	r := newArchiveReconciler(t, time.Hour, job)

	if _, err := r.Reconcile(context.Background(), requestFor(job)); err != nil {
		t.Fatal(err)
	}
	if !jobExists(t, r, job) {
		t.Fatal("job was archived off an unparsable origin")
	}

	var stored rlarkv1alpha1.Job
	if err := r.Get(context.Background(), client.ObjectKeyFromObject(job), &stored); err != nil {
		t.Fatal(err)
	}
	recorded := stored.Annotations[ArchiveObservedSinceAnnotation]
	if _, err := time.Parse(time.RFC3339, recorded); err != nil {
		t.Fatalf("archive origin %q was not rewritten into a usable timestamp", recorded)
	}
}
