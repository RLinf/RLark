package cluster

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestDrainTasksStopsOnlyTasksInAgentNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "cluster-a", Draining: true},
	}
	targetTask := drainTestTask("target", "cluster-a")
	targetTask.OwnerReferences = []metav1.OwnerReference{{APIVersion: rlarkv1alpha1.GroupVersion.String(), Kind: "Job", Name: "job-a"}}
	otherTask := drainTestTask("other", "cluster-b")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, targetTask, otherTask).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	complete, _, err := r.drainTasks(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Fatal("first drain pass must wait for stopped task observation")
	}

	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(targetTask), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "true" || got.Spec.Suspended {
		t.Fatalf("cluster drain must only mark target task for the Job controller: %#v", got)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(otherTask), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" || got.Spec.Suspended {
		t.Fatalf("task outside cluster was changed: %#v", got)
	}

	complete, _, err = r.drainTasks(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Fatal("drain must wait for the agent to observe the stopped task")
	}

	if err := c.Get(context.Background(), client.ObjectKeyFromObject(targetTask), &got); err != nil {
		t.Fatal(err)
	}
	got.Status.Phase = rlarkv1alpha1.TaskPhaseStopped
	got.Status.ObservedGeneration = got.Generation
	if err := c.Update(context.Background(), &got); err != nil {
		t.Fatal(err)
	}

	complete, _, err = r.drainTasks(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("drain should complete after the agent observes every task stopped")
	}
}

func TestDrainTasksSkipsTerminalTasksAndSuspendsStandaloneTasks(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "cluster-a", Draining: true},
	}
	terminal := drainTestTask("terminal", "cluster-a")
	terminal.Status.Phase = rlarkv1alpha1.TaskPhaseSucceeded
	standalone := drainTestTask("standalone", "cluster-a")
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, terminal, standalone).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	complete, pending, err := r.drainTasks(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if complete || len(pending) != 1 || pending[0] != "cluster-a/standalone" {
		t.Fatalf("unexpected drain result: complete=%t pending=%v", complete, pending)
	}

	var gotTerminal, gotStandalone rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(terminal), &gotTerminal); err != nil {
		t.Fatal(err)
	}
	if gotTerminal.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" || gotTerminal.Spec.Suspended {
		t.Fatalf("terminal task was changed: %#v", gotTerminal)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(standalone), &gotStandalone); err != nil {
		t.Fatal(err)
	}
	if gotStandalone.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "true" || !gotStandalone.Spec.Suspended {
		t.Fatalf("standalone task was not suspended for drain: %#v", gotStandalone)
	}

	gotStandalone.Status.Phase = rlarkv1alpha1.TaskPhaseStopped
	gotStandalone.Status.ObservedGeneration = gotStandalone.Generation
	if err := c.Update(context.Background(), &gotStandalone); err != nil {
		t.Fatal(err)
	}
	complete, pending, err = r.drainTasks(context.Background(), cluster)
	if err != nil {
		t.Fatal(err)
	}
	if !complete || len(pending) != 0 {
		t.Fatalf("drain result after standalone stop: complete=%t pending=%v", complete, pending)
	}
}

func TestClearDrainTaskMarkersOnlyClearsTasksInAgentNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "cluster-a"},
	}
	targetTask := drainTestTask("target", "cluster-a")
	targetTask.Annotations = map[string]string{rlarkv1alpha1.ClusterStoppedAnnotation: "true", "keep": "value"}
	otherTask := drainTestTask("other", "cluster-b")
	otherTask.Annotations = map[string]string{rlarkv1alpha1.ClusterStoppedAnnotation: "true"}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, targetTask, otherTask).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if err := r.clearDrainTaskMarkers(context.Background(), cluster); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(targetTask), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" || got.Annotations["keep"] != "value" {
		t.Fatalf("target Task drain marker was not cleared safely: %#v", got.Annotations)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(otherTask), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "true" {
		t.Fatalf("Task outside cluster was changed: %#v", got.Annotations)
	}
}

func TestClearDrainTaskMarkersResumesOnlyDrainSuspendedStandaloneTasks(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "cluster-a", Draining: true},
	}
	drainSuspended := drainTestTask("drain-suspended", "cluster-a")
	userSuspended := drainTestTask("user-suspended", "cluster-a")
	userSuspended.Spec.Suspended = true
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, drainSuspended, userSuspended).Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	if _, _, err := r.drainTasks(context.Background(), cluster); err != nil {
		t.Fatal(err)
	}

	var got rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(drainSuspended), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended || got.Annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] != "true" {
		t.Fatalf("drain must record that it suspended the standalone Task: %#v", got)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(userSuspended), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] != "" {
		t.Fatalf("drain must not claim a suspension it did not write: %#v", got.Annotations)
	}

	cluster.Spec.Draining = false
	if err := r.clearDrainTaskMarkers(context.Background(), cluster); err != nil {
		t.Fatal(err)
	}

	if err := c.Get(context.Background(), client.ObjectKeyFromObject(drainSuspended), &got); err != nil {
		t.Fatal(err)
	}
	if got.Spec.Suspended ||
		got.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" ||
		got.Annotations[rlarkv1alpha1.ClusterDrainSuspendedAnnotation] != "" {
		t.Fatalf("cancelling the drain must resume the Task it suspended: %#v", got)
	}
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(userSuspended), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Spec.Suspended || got.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" {
		t.Fatalf("cancelling the drain must keep a user suspension: %#v", got)
	}
}

func TestDrainTasksRejectsEmptyAgentNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	cluster := &rlarkv1alpha1.Cluster{Spec: rlarkv1alpha1.ClusterSpec{Draining: true}}
	task := drainTestTask("task-in-default-namespace", "")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, task).Build()
	reconciler := &Reconciler{Client: fakeClient, Scheme: scheme}

	if _, _, err := reconciler.drainTasks(context.Background(), cluster); err == nil {
		t.Fatal("expected drain to reject an empty agent namespace")
	}
	if err := reconciler.clearDrainTaskMarkers(context.Background(), cluster); err == nil {
		t.Fatal("expected marker cleanup to reject an empty agent namespace")
	}

	var unchanged rlarkv1alpha1.Task
	if err := fakeClient.Get(context.Background(), client.ObjectKeyFromObject(task), &unchanged); err != nil {
		t.Fatalf("get Task: %v", err)
	}
	if unchanged.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "" {
		t.Fatalf("task was marked for drain despite empty agent namespace: %#v", unchanged)
	}
}

func drainTestTask(name, namespace string) *rlarkv1alpha1.Task {
	return &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
}
