package pod

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

type countingClient struct {
	client.Client
	patches       int
	statusPatches int
}

type failingGetClient struct {
	client.Client
	err error
}

type failingNamedGetClient struct {
	client.Client
	name string
	err  error
}

type failingDeleteClient struct {
	client.Client
	failName string
}

func (c *failingDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if obj.GetName() == c.failName {
		return errors.New("management delete failed")
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func (c *failingGetClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return c.err
}

func (c *failingNamedGetClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if key.Name == c.name {
		return c.err
	}
	return c.Client.Get(ctx, key, obj, opts...)
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return scheme
}

func (c *countingClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	c.patches++
	return c.Client.Patch(ctx, obj, patch, opts...)
}

func TestBuildManagementPodIdentityOwnerAndUnknown(t *testing.T) {
	scheme := testScheme(t)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "agent-a", UID: "task-uid"}, Spec: rlarkv1alpha1.TaskSpec{Domain: "domain-a"}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	r := &pushPodReconciler{c: NewPodController(base.Controller{ManagementClient: management, ManagementNamespace: "agent-a"})}
	local := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "work", UID: "pod-uid", Annotations: map[string]string{"rlark.io/management-task-domain": "domain-a"}},
		Status:     corev1.PodStatus{Phase: corev1.PodUnknown},
	}

	got, err := r.buildRLarkPodFromK8sPod(context.Background(), local, "task", "agent-a", "task-uid")
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		rlarkv1alpha1.PodLabelTaskName:          "task",
		rlarkv1alpha1.PodLabelTaskUID:           "task-uid",
		rlarkv1alpha1.PodLabelLocalPodName:      "pod",
		rlarkv1alpha1.PodLabelLocalPodNamespace: "work",
		rlarkv1alpha1.PodLabelLocalPodUID:       "pod-uid",
		rlarkv1alpha1.PodLabelAgentScope:        "agent-a",
	} {
		if got.Labels[key] != want {
			t.Errorf("label %s = %q, want %q", key, got.Labels[key], want)
		}
	}
	if _, ok := got.Labels[rlarkv1alpha1.PodLabelDomain]; ok {
		t.Errorf("domain must not be copied into a label: %v", got.Labels)
	}
	if got.Spec.Domain != "domain-a" {
		t.Errorf("spec domain = %q, want domain-a", got.Spec.Domain)
	}
	if got.Status.Phase != rlarkv1alpha1.PodPhaseUnknown {
		t.Errorf("phase = %q, want Unknown", got.Status.Phase)
	}
	if len(got.OwnerReferences) != 1 || got.OwnerReferences[0].UID != task.UID {
		t.Fatalf("owner references = %+v, want verified Task", got.OwnerReferences)
	}

	if _, err = r.buildRLarkPodFromK8sPod(context.Background(), local, "task", "agent-a", "stale-uid"); err == nil {
		t.Fatal("stale Task UID was accepted")
	}
}

func TestLegacyManagementPodBackfillPreservesMetadata(t *testing.T) {
	scheme := testScheme(t)
	legacy := &rlarkv1alpha1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "pod-uid", Namespace: "agent-a",
		Labels: map[string]string{"other.io/label": "keep"}, Annotations: map[string]string{"other.io/annotation": "keep"},
	}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(legacy).Build()
	r := &pushPodReconciler{c: NewPodController(base.Controller{ManagementClient: management, ManagementNamespace: "agent-a"})}
	desired := legacy.DeepCopy()
	desired.Labels = map[string]string{rlarkv1alpha1.PodLabelLocalPodUID: "pod-uid", rlarkv1alpha1.PodLabelAgentScope: "agent-a"}
	desired.Annotations = nil

	if _, err := r.updateManagementPod(context.Background(), logr.Discard(), desired); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Pod
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(legacy), &got); err != nil {
		t.Fatal(err)
	}
	if got.Labels["other.io/label"] != "keep" || got.Annotations["other.io/annotation"] != "keep" ||
		got.Labels[rlarkv1alpha1.PodLabelLocalPodUID] != "pod-uid" {
		t.Fatalf("metadata was not preserved/backfilled: labels=%v annotations=%v", got.Labels, got.Annotations)
	}
}

func TestUpdateManagementPodRejectsContradictoryPartialIdentity(t *testing.T) {
	scheme := testScheme(t)
	for _, tc := range []struct {
		name   string
		mutate func(*rlarkv1alpha1.Pod)
	}{
		{"local UID", func(p *rlarkv1alpha1.Pod) { p.Labels[rlarkv1alpha1.PodLabelLocalPodUID] = "other" }},
		{"local name", func(p *rlarkv1alpha1.Pod) { p.Spec.PodName = "other" }},
		{"Task name", func(p *rlarkv1alpha1.Pod) { p.Labels[rlarkv1alpha1.PodLabelTaskName] = "other" }},
		{"Task namespace", func(p *rlarkv1alpha1.Pod) { p.Spec.TaskNamespace = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			existing := &rlarkv1alpha1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-uid", Namespace: "agent-a", Labels: map[string]string{}}}
			tc.mutate(existing)
			desired := &rlarkv1alpha1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-uid", Namespace: "agent-a", Labels: map[string]string{
				rlarkv1alpha1.PodLabelLocalPodUID: "pod-uid", rlarkv1alpha1.PodLabelLocalPodName: "pod", rlarkv1alpha1.PodLabelTaskName: "task",
			}}, Spec: rlarkv1alpha1.PodSpec{PodName: "pod", TaskNamespace: "agent-a"}}
			management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(existing).Build()
			r := &pushPodReconciler{c: NewPodController(base.Controller{ManagementClient: management})}
			if _, err := r.updateManagementPod(context.Background(), logr.Discard(), desired); !apierrors.IsConflict(err) {
				t.Fatalf("error = %v, want conflict", err)
			}
		})
	}
}

func TestUpdateManagementPodRejectsCompletelyUnannotatedSameName(t *testing.T) {
	scheme := testScheme(t)
	existing := &rlarkv1alpha1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod-uid", Namespace: "agent-a"}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(existing).Build()
	r := &pushPodReconciler{c: NewPodController(base.Controller{ManagementClient: management})}
	desired := managementPod("pod-uid", "agent-a", "work", "pod", "pod-uid")
	desired.Labels[rlarkv1alpha1.PodLabelTaskName] = "task"
	desired.Labels[rlarkv1alpha1.PodLabelTaskUID] = "task-uid"
	desired.Spec.TaskName, desired.Spec.TaskNamespace = "task", "agent-a"

	if _, err := r.updateManagementPod(context.Background(), logr.Discard(), desired); !apierrors.IsConflict(err) {
		t.Fatalf("error = %v, want conflict", err)
	}
	var got rlarkv1alpha1.Pod
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Labels) != 0 || got.Spec != (rlarkv1alpha1.PodSpec{}) || got.Status != (rlarkv1alpha1.PodStatus{}) {
		t.Fatalf("unannotated same-name Pod was modified: labels=%v spec=%+v status=%+v", got.Labels, got.Spec, got.Status)
	}
}

func TestUpdateManagementPodRejectsControllerOwnerMismatch(t *testing.T) {
	scheme := testScheme(t)
	controller := true
	existing := &rlarkv1alpha1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "pod-uid", Namespace: "agent-a",
		Labels:          map[string]string{rlarkv1alpha1.PodLabelDomain: "old", "other.io/label": "keep"},
		OwnerReferences: []metav1.OwnerReference{{Kind: "Task", Name: "old", UID: "old", Controller: &controller}},
	}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(existing).Build()
	r := &pushPodReconciler{c: NewPodController(base.Controller{ManagementClient: management})}
	desired := existing.DeepCopy()
	desired.Labels = map[string]string{rlarkv1alpha1.PodLabelAgentScope: "agent-a"}
	desired.OwnerReferences = nil
	if _, err := r.updateManagementPod(context.Background(), logr.Discard(), desired); !apierrors.IsConflict(err) {
		t.Fatalf("error = %v, want conflict", err)
	}
	var got rlarkv1alpha1.Pod
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil {
		t.Fatal(err)
	}
	if got.Labels[rlarkv1alpha1.PodLabelDomain] != "old" || got.Labels["other.io/label"] != "keep" || len(got.OwnerReferences) != 1 {
		t.Fatalf("conflicting Pod was modified: labels=%v owners=%v", got.Labels, got.OwnerReferences)
	}
}

func TestDelayedDeleteDoesNotDeleteReplacementUID(t *testing.T) {
	scheme := testScheme(t)
	current := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "work", UID: "uid-b"}}
	old := managementPod("uid-a", "agent-a", "work", "pod", "uid-a")
	newPod := managementPod("uid-b", "agent-a", "work", "pod", "uid-b")
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(old, newPod).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(current).Build()
	r := &pushPodReconciler{c: NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: local, ManagementNamespace: "agent-a"})}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "pod", Namespace: "work"}}); err != nil {
		t.Fatal(err)
	}
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(newPod), &rlarkv1alpha1.Pod{}); err != nil {
		t.Fatalf("replacement management Pod deleted: %v", err)
	}
}

func TestUIDDeleteEventDeletesOnlyMatchingScopedPod(t *testing.T) {
	scheme := testScheme(t)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "agent-a", UID: "task-uid"}}
	old := managementPod("uid-a", "agent-a", "work", "pod", "uid-a")
	replacement := managementPod("uid-b", "agent-a", "work", "pod", "uid-b")
	unrelated := managementPod("uid-c", "agent-a", "work", "other", "uid-c")
	localReplacement := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "work", UID: "uid-b", Annotations: map[string]string{
		"rlark.io/management-task-name": "task", "rlark.io/management-task-namespace": "agent-a", "rlark.io/management-task-uid": "task-uid",
	}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(old, replacement, unrelated, task).Build()
	c := NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(localReplacement).Build(), ManagementNamespace: "agent-a"})
	c.deleteUIDs[types.NamespacedName{Name: "pod", Namespace: "work"}] = []types.UID{"uid-a"}
	r := &pushPodReconciler{c: c}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "pod", Namespace: "work"}}); err != nil {
		t.Fatal(err)
	}
	assertNotFound(t, management, old)
	assertExists(t, management, replacement)
	assertExists(t, management, unrelated)
}

func TestDeleteQueueAcknowledgesOnlySuccessfulItems(t *testing.T) {
	scheme := testScheme(t)
	first := managementPod("uid-a", "agent-a", "work", "pod", "uid-a")
	failed := managementPod("uid-b", "agent-a", "work", "pod", "uid-b")
	rest := managementPod("uid-c", "agent-a", "work", "pod", "uid-c")
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(first, failed, rest).Build()
	management := &failingDeleteClient{Client: baseClient, failName: "uid-b"}
	c := NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: fake.NewClientBuilder().WithScheme(scheme).Build(), ManagementNamespace: "agent-a"})
	key := types.NamespacedName{Name: "pod", Namespace: "work"}
	c.deleteUIDs[key] = []types.UID{"uid-a", "uid-b", "uid-c"}
	if _, err := (&pushPodReconciler{c: c}).Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err == nil {
		t.Fatal("expected delete failure")
	}
	assertNotFound(t, baseClient, first)
	assertExists(t, baseClient, failed)
	assertExists(t, baseClient, rest)
	got := c.pendingDeleteUIDs(key)
	if len(got) != 2 || got[0] != "uid-b" || got[1] != "uid-c" {
		t.Fatalf("pending UIDs = %v, want [uid-b uid-c]", got)
	}
}

func TestOrphanSweepRecoveryScopeLegacyAndUIDRace(t *testing.T) {
	scheme := testScheme(t)
	localCurrent := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "reused", Namespace: "work", UID: "uid-b"}}
	orphan := managementPod("orphan", "agent-a", "work", "gone", "uid-orphan")
	stale := managementPod("uid-a", "agent-a", "work", "reused", "uid-a")
	current := managementPod("uid-b", "agent-a", "work", "reused", "uid-b")
	otherScope := managementPod("other", "agent-b", "work", "gone", "uid-other")
	legacy := managementPod("legacy", "", "work", "gone", "")
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(orphan, stale, current, otherScope, legacy).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(localCurrent).Build()
	sweeper := NewOrphanSweeper(NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: local, ManagementNamespace: "agent-a"}), time.Minute, 1)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sweeper.now = func() time.Time { return now }

	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertExists(t, management, orphan)
	assertExists(t, management, stale)
	now = now.Add(16 * time.Minute)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertNotFound(t, management, orphan)
	assertNotFound(t, management, stale)
	assertExists(t, management, current)
	assertExists(t, management, otherScope)
	assertExists(t, management, legacy)
}

func TestOrphanSweepAbortsOnLocalAPIError(t *testing.T) {
	scheme := testScheme(t)
	first := managementPod("first", "agent-a", "work", "first", "uid-first")
	second := managementPod("second", "agent-a", "work", "second", "uid-second")
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(first, second).Build()
	localErr := errors.New("local API unavailable")
	local := &failingGetClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), err: localErr}
	sweeper := NewOrphanSweeper(NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: local, ManagementNamespace: "agent-a"}), time.Minute, 1)

	if err := sweeper.Sweep(context.Background()); !errors.Is(err, localErr) {
		t.Fatalf("Sweep error = %v, want %v", err, localErr)
	}
	assertExists(t, management, first)
	assertExists(t, management, second)
}

func TestOrphanSweepContinuesAfterObjectError(t *testing.T) {
	scheme := testScheme(t)
	first := managementPod("first", "agent-a", "work", "first", "uid-first")
	second := managementPod("second", "agent-a", "work", "second", "uid-second")
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(first, second).Build()
	localErr := errors.New("later local API unavailable")
	localBase := fake.NewClientBuilder().WithScheme(scheme).Build()
	local := &failingNamedGetClient{Client: localBase, name: "second", err: localErr}
	sweeper := NewOrphanSweeper(NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: local, ManagementNamespace: "agent-a"}), time.Minute, 10)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sweeper.now = func() time.Time { return now }

	if err := sweeper.Sweep(context.Background()); !errors.Is(err, localErr) {
		t.Fatalf("Sweep error = %v, want %v", err, localErr)
	}
	var got rlarkv1alpha1.Pod
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(first), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[staleSinceAnnotation] != now.Format(time.RFC3339Nano) || got.Status.Phase != rlarkv1alpha1.PodPhaseUnknown {
		t.Fatalf("failure for another Pod blocked cleanup: annotations=%v status=%+v", got.Annotations, got.Status)
	}
}

func TestOrphanSweepRechecksBeforeDeleting(t *testing.T) {
	scheme := testScheme(t)
	pod := managementPod("pod-uid", "agent-a", "work", "pod", "pod-uid")
	pod.Annotations = map[string]string{staleSinceAnnotation: time.Now().Add(-time.Hour).Format(time.RFC3339Nano)}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "work", UID: "pod-uid"}}).Build()
	sweeper := NewOrphanSweeper(NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: local, ManagementNamespace: "agent-a"}), time.Minute, 10)
	sweeper.now = func() time.Time { return time.Now() }

	if err := sweeper.markOrDeleteStale(context.Background(), pod); err != nil {
		t.Fatal(err)
	}
	assertExists(t, management, pod)
}

func TestOrphanSweepDeletesMissingOrReplacedTask(t *testing.T) {
	scheme := testScheme(t)
	local := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "work", UID: "pod-uid"}}
	missing := managementPod("missing", "agent-a", "work", "pod", "pod-uid")
	missing.Spec.TaskName, missing.Spec.TaskNamespace = "gone", "agent-a"
	missing.Labels[rlarkv1alpha1.PodLabelTaskName], missing.Labels[rlarkv1alpha1.PodLabelTaskUID] = "gone", "old-task"
	replaced := managementPod("replaced", "agent-a", "work", "pod", "pod-uid")
	replaced.Spec.TaskName, replaced.Spec.TaskNamespace = "task", "agent-a"
	replaced.Labels[rlarkv1alpha1.PodLabelTaskName], replaced.Labels[rlarkv1alpha1.PodLabelTaskUID] = "task", "old-task"
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "agent-a", UID: "new-task"}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(missing, replaced, task).Build()
	sweeper := NewOrphanSweeper(NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(local).Build(), ManagementNamespace: "agent-a"}), time.Minute, 10)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sweeper.now = func() time.Time { return now }
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertExists(t, management, missing)
	assertExists(t, management, replaced)
	now = now.Add(16 * time.Minute)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	assertNotFound(t, management, missing)
	assertNotFound(t, management, replaced)
}

func TestOrphanSweepResetsMalformedStaleSince(t *testing.T) {
	scheme := testScheme(t)
	pod := managementPod("uid", "agent-a", "work", "gone", "uid")
	pod.Annotations = map[string]string{staleSinceAnnotation: "invalid"}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(pod).Build()
	sweeper := NewOrphanSweeper(NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: fake.NewClientBuilder().WithScheme(scheme).Build(), ManagementNamespace: "agent-a"}), time.Minute, 10)
	now := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	sweeper.now = func() time.Time { return now }
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Pod
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(pod), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[staleSinceAnnotation] != now.Format(time.RFC3339Nano) {
		t.Fatalf("stale-since = %q, want %q", got.Annotations[staleSinceAnnotation], now.Format(time.RFC3339Nano))
	}
}

func TestOrphanSweepRecoveryUpdatesStatusAndClearsStale(t *testing.T) {
	scheme := testScheme(t)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "agent-a", UID: "task-uid"}}
	local := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "work", UID: "pod-uid", Annotations: map[string]string{
		"rlark.io/management-task-name": "task", "rlark.io/management-task-namespace": "agent-a", "rlark.io/management-task-uid": "task-uid",
	}}, Spec: corev1.PodSpec{NodeName: "node-a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.2"}}
	pod := managementPod("pod-uid", "agent-a", "work", "pod", "pod-uid")
	pod.Labels[rlarkv1alpha1.PodLabelTaskName] = "task"
	pod.Labels[rlarkv1alpha1.PodLabelTaskUID] = "task-uid"
	pod.Spec.TaskName, pod.Spec.TaskNamespace = "task", "agent-a"
	pod.Annotations = map[string]string{staleSinceAnnotation: time.Now().Add(-time.Minute).Format(time.RFC3339Nano)}
	pod.Status = rlarkv1alpha1.PodStatus{Phase: rlarkv1alpha1.PodPhaseUnknown, Message: "Stale: local Pod is missing or its identity changed"}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(pod, task).Build()
	sweeper := NewOrphanSweeper(NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(local).Build(), ManagementNamespace: "agent-a"}), time.Minute, 10)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Pod
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(pod), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[staleSinceAnnotation] != "" || got.Status.Phase != rlarkv1alpha1.PodPhaseRunning || got.Status.Node != "node-a" || got.Status.IP != "10.0.0.2" || got.Status.Message != "" {
		t.Fatalf("recovered Pod = annotations=%v status=%+v", got.Annotations, got.Status)
	}
}

func TestOrphanSweepRecoversLegacyStatefulSetPodWithoutTaskUID(t *testing.T) {
	scheme := testScheme(t)
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "agent-a", UID: "task-uid"}}
	controller := true
	statefulSet := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "workload", Namespace: "work", UID: "sts-uid", Annotations: map[string]string{
		"rlark.io/management-task-name": "task", "rlark.io/management-task-namespace": "agent-a", "rlark.io/management-task-uid": "task-uid",
	}}}
	local := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "work", UID: "pod-uid", Annotations: map[string]string{
		"rlark.io/management-task-name": "task", "rlark.io/management-task-namespace": "agent-a",
	}, OwnerReferences: []metav1.OwnerReference{{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "StatefulSet", Name: statefulSet.Name, UID: statefulSet.UID, Controller: &controller}}},
		Spec: corev1.PodSpec{NodeName: "node-a"}, Status: corev1.PodStatus{Phase: corev1.PodRunning, PodIP: "10.0.0.2"}}
	pod := managementPod("pod-uid", "agent-a", "work", "pod", "pod-uid")
	pod.Labels[rlarkv1alpha1.PodLabelTaskName] = "task"
	pod.Labels[rlarkv1alpha1.PodLabelTaskUID] = "task-uid"
	pod.Spec.TaskName, pod.Spec.TaskNamespace = "task", "agent-a"
	pod.Annotations = map[string]string{staleSinceAnnotation: time.Now().Add(-time.Minute).Format(time.RFC3339Nano)}
	pod.Status = rlarkv1alpha1.PodStatus{Phase: rlarkv1alpha1.PodPhaseUnknown, Message: "stale"}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(pod, task).Build()
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(local, statefulSet).Build()
	sweeper := NewOrphanSweeper(NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: localClient, ManagementNamespace: "agent-a"}), time.Minute, 10)
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Pod
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(pod), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[staleSinceAnnotation] != "" || got.Status.Phase != rlarkv1alpha1.PodPhaseRunning || got.Status.Node != "node-a" || got.Status.IP != "10.0.0.2" || got.Labels[rlarkv1alpha1.PodLabelTaskUID] != "task-uid" {
		t.Fatalf("legacy Pod was not recovered: labels=%v annotations=%v status=%+v", got.Labels, got.Annotations, got.Status)
	}
}

func TestOrphanSweepConservativelyBackfillsLegacyIdentity(t *testing.T) {
	scheme := testScheme(t)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "agent-a", UID: "task-uid"}, Spec: rlarkv1alpha1.TaskSpec{Domain: "domain-a"}}
	local := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "work", UID: "pod-uid", Annotations: map[string]string{
		"rlark.io/management-task-name": "task", "rlark.io/management-task-namespace": "agent-a", "rlark.io/management-task-uid": "task-uid",
	}}}
	legacy := managementPod("pod-uid", "", "work", "pod", "")
	legacy.Spec.TaskName, legacy.Spec.TaskNamespace, legacy.Spec.Domain = "task", "agent-a", "domain-a"
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&rlarkv1alpha1.Pod{}).WithObjects(legacy, task).Build()
	sweeper := NewOrphanSweeper(NewPodController(base.Controller{ManagementClient: management, LocalKubeClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(local).Build(), ManagementNamespace: "agent-a"}), time.Minute, 10)
	sweeper.now = func() time.Time { return time.Now() }
	if err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Pod
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(legacy), &got); err != nil {
		t.Fatal(err)
	}
	if got.Labels[rlarkv1alpha1.PodLabelAgentScope] != "agent-a" || got.Labels[rlarkv1alpha1.PodLabelTaskUID] != "task-uid" || got.Labels[rlarkv1alpha1.PodLabelLocalPodUID] != "pod-uid" {
		t.Fatalf("legacy identity not backfilled: %v", got.Labels)
	}
}

func managementPod(name, scope, namespace, localName, uid string) *rlarkv1alpha1.Pod {
	labels := map[string]string{
		rlarkv1alpha1.PodLabelLocalPodName: localName, rlarkv1alpha1.PodLabelLocalPodNamespace: namespace,
	}
	if scope != "" {
		labels[rlarkv1alpha1.PodLabelAgentScope] = scope
	}
	if uid != "" {
		labels[rlarkv1alpha1.PodLabelLocalPodUID] = uid
	}
	return &rlarkv1alpha1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "agent-a", Labels: labels}, Spec: rlarkv1alpha1.PodSpec{PodName: localName, PodNamespace: namespace}}
}

func assertExists(t *testing.T, c client.Client, pod *rlarkv1alpha1.Pod) {
	t.Helper()
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(pod), &rlarkv1alpha1.Pod{}); err != nil {
		t.Fatalf("expected %s to exist: %v", pod.Name, err)
	}
}

func assertNotFound(t *testing.T, c client.Client, pod *rlarkv1alpha1.Pod) {
	t.Helper()
	err := c.Get(context.Background(), client.ObjectKeyFromObject(pod), &rlarkv1alpha1.Pod{})
	if !apierrors.IsNotFound(err) {
		t.Fatalf("expected %s to be deleted, got %v", pod.Name, err)
	}
}

func (c *countingClient) Status() client.StatusWriter {
	return &countingStatusWriter{SubResourceWriter: c.Client.Status(), parent: c}
}

type countingStatusWriter struct {
	client.SubResourceWriter
	parent *countingClient
}

func (w *countingStatusWriter) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
	w.parent.statusPatches++
	return w.SubResourceWriter.Patch(ctx, obj, patch, opts...)
}

func TestUpdateManagementPodSkipsUnchangedWrites(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	pod := &rlarkv1alpha1.Pod{}
	pod.Name = "uid"
	pod.Namespace = "cluster"
	pod.Labels = map[string]string{"task": "task", rlarkv1alpha1.PodLabelAgentScope: "cluster"}
	pod.Spec.TaskName = "task"
	pod.Status.Phase = rlarkv1alpha1.PodPhaseRunning

	wrapped := &countingClient{Client: fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&rlarkv1alpha1.Pod{}).
		WithObjects(pod).
		Build()}
	r := &pushPodReconciler{c: NewPodController(base.Controller{ManagementClient: wrapped})}
	desiredUnchanged := pod.DeepCopy()

	if _, err := r.updateManagementPod(context.Background(), logr.Discard(), desiredUnchanged); err != nil {
		t.Fatal(err)
	}
	if wrapped.patches != 0 || wrapped.statusPatches != 0 {
		t.Fatalf("unchanged Pod caused patches: spec=%d status=%d", wrapped.patches, wrapped.statusPatches)
	}

	desired := pod.DeepCopy()
	desired.Status.IP = "10.0.0.1"
	if _, err := r.updateManagementPod(context.Background(), logr.Discard(), desired); err != nil {
		t.Fatal(err)
	}
	if wrapped.patches != 0 || wrapped.statusPatches != 1 {
		t.Fatalf("status-only change caused patches: spec=%d status=%d, want 0/1", wrapped.patches, wrapped.statusPatches)
	}
}
