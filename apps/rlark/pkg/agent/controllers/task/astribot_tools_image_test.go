package task

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// The cluster Agent's --image is the image of the tools every Task Pod starts with. Giving the Agent another
// replaces no running Pod: a workload that is otherwise as its Task says keeps the tools image it has. A Task
// that changes gets the Agent's current image with the Pods the change replaces anyway.
func TestAnotherToolsImageDoesNotReplaceRunningPods(t *testing.T) {
	ctx := context.Background()
	scheme := executionScheme(t)
	task := perNodeExecutionTask()
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}
	existing := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "daemonset-uid", ResourceVersion: "1", Generation: 1, Annotations: map[string]string{
			ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace,
			ManagementTaskUIDAnnotation: string(task.UID), ManagementTaskGenerationAnnotation: fmt.Sprintf("%d", task.Generation),
		}},
		Spec: appsv1.DaemonSetSpec{Selector: selector, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: selector.MatchLabels}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "station:v1"}}}}},
	}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(existing).WithObjects(existing).
		WithIndex(&corev1.Event{}, "involvedObject.uid", func(object client.Object) []string {
			return []string{string(object.(*corev1.Event).InvolvedObject.UID)}
		}).Build()
	key := client.ObjectKeyFromObject(task)
	// reconcile is one pass of a cluster Agent started with --image=image; it returns the DaemonSet and its tools image.
	reconcileWith := func(image string) (*appsv1.DaemonSet, string) {
		t.Helper()
		execution, err := NewKubernetesExecutionCoordinator(local, image)
		if err != nil {
			t.Fatal(err)
		}
		controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
		if _, err := (&pullReconciler{c: controller}).Reconcile(ctx, reconcile.Request{NamespacedName: key}); err != nil {
			t.Fatal(err)
		}
		daemonSet := &appsv1.DaemonSet{}
		if err := local.Get(ctx, client.ObjectKeyFromObject(existing), daemonSet); err != nil {
			t.Fatal(err)
		}
		for _, container := range daemonSet.Spec.Template.Spec.InitContainers {
			if container.Name == kubernetesobjects.ToolsInitContainerName {
				return daemonSet, container.Image
			}
		}
		t.Fatalf("no tools container in %#v", daemonSet.Spec.Template.Spec.InitContainers)
		return nil, ""
	}

	first, image := reconcileWith("rlark:1")
	if image != "rlark:1" {
		t.Fatalf("a new workload's tools image is %s", image)
	}
	second, image := reconcileWith("rlark:2")
	// What the Agent records of its own plan on the workload may change; what the Pods are made from does not.
	if image != "rlark:1" || !reflect.DeepEqual(second.Spec, first.Spec) {
		t.Fatalf("another --image changed a running workload: tools image %s, spec %#v", image, second.Spec)
	}
	if again, _ := reconcileWith("rlark:2"); again.ResourceVersion != second.ResourceVersion {
		t.Fatalf("the workload is written on every pass: resourceVersion %s -> %s", second.ResourceVersion, again.ResourceVersion)
	}

	// The Task changes: its Pods are replaced, and take the tools image of now.
	var current rlarkv1alpha1.Task
	if err := management.Get(ctx, key, &current); err != nil {
		t.Fatal(err)
	}
	current.Generation++
	current.Spec.Workload.Template.Spec.Containers[0].Image = "station:v2"
	if err := management.Update(ctx, &current); err != nil {
		t.Fatal(err)
	}
	third, image := reconcileWith("rlark:2")
	if main := third.Spec.Template.Spec.Containers[0].Image; main != "station:v2" || image != "rlark:2" {
		t.Fatalf("after the Task changed: main %s, tools %s", main, image)
	}
}
