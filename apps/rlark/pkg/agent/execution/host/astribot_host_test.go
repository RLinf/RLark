package host

import (
	"context"
	"reflect"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestKubernetesStatusWriterReplacesAndClearsPerNodeInstances(t *testing.T) {
	task := hostTask()
	task.Status.Instances = []rlarkv1alpha1.TaskInstanceStatus{{Node: "gone", Name: "task-gone", Phase: rlarkv1alpha1.TaskInstanceRunning}}
	scheme := runtime.NewScheme()
	_ = rlarkv1alpha1.AddToScheme(scheme)
	kube := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	writer := KubernetesStatusWriter{Client: kube}
	observed := []rlarkv1alpha1.TaskInstanceStatus{{Node: "a", Name: "task-a", Phase: rlarkv1alpha1.TaskInstanceRunning, Image: "station:v1"}}
	intent := IntentForTask(task, rlarkv1alpha1.TaskPhaseRunning, "", []string{"a"}, StatusPriorityWorkload)
	intent.Instances = observed
	if err := writer.WriteStatus(context.Background(), task, intent); err != nil {
		t.Fatal(err)
	}
	var current rlarkv1alpha1.Task
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(current.Status.Instances, observed) {
		t.Fatalf("instances=%#v, want %#v", current.Status.Instances, observed)
	}
	// Any other phase intent, such as stopping the Task, clears them.
	if err := writer.WriteStatus(context.Background(), &current, IntentForTask(&current, rlarkv1alpha1.TaskPhaseStopped, "", nil, StatusPriorityLifecycle)); err != nil {
		t.Fatal(err)
	}
	if err := kube.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
		t.Fatal(err)
	}
	if current.Status.Instances != nil {
		t.Fatalf("stopped Task kept instances: %#v", current.Status.Instances)
	}
}
