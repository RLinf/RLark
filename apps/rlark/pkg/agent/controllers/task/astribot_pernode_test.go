package task

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

func perNodeExecutionTask() *rlarkv1alpha1.Task {
	task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
	task.Annotations[host.TaskClaimedCarrierTypeAnnotation] = taskapi.CarrierTypeKubernetesDaemonSet
	task.Spec.Placement = rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"rlark.io/node-category": "robot"}, PerNode: true}
	task.Spec.Workload.Carrier.Type = taskapi.CarrierTypeKubernetesDaemonSet
	task.Spec.Workload.Carrier.Parameters.Raw = nil
	task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "station:v1"}}
	return task
}

// perNodePod is the Pod the DaemonSet controller would place on node.
func perNodePod(daemonSet *appsv1.DaemonSet, node string, ready bool, waiting string) *corev1.Pod {
	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: daemonSet.Name + "-" + node, Namespace: daemonSet.Namespace, UID: types.UID("pod-" + node),
			Labels:          map[string]string{ManagementTaskUIDLabel: daemonSet.Spec.Selector.MatchLabels[ManagementTaskUIDLabel], "pod-template-generation": "1"},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: daemonSet.Name, UID: daemonSet.UID, Controller: &controller}},
		},
		Spec:   corev1.PodSpec{NodeName: node, Containers: []corev1.Container{{Name: "main", Image: "station:v1"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	if ready {
		pod.Status.Conditions = []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}
	}
	if waiting != "" {
		pod.Status.Phase = corev1.PodPending
		pod.Status.ContainerStatuses = []corev1.ContainerStatus{{Name: "main", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: waiting}}}}
	}
	return pod
}

// TestPerNodeTaskFollowsItsNodes runs a per-node Task through the Agent: the
// pull reconcile renders a DaemonSet, and each change the scoped Runtime
// observer sees is projected into the Task's phase and per-node instances as
// nodes join, switch off, fail and leave. Only instances on Ready nodes count
// toward the phase, and the Task is never Failed.
func TestPerNodeTaskFollowsItsNodes(t *testing.T) {
	scheme := executionScheme(t)
	task := perNodeExecutionTask()
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}
	existing := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "daemonset-uid", ResourceVersion: "1", Generation: 1, Annotations: map[string]string{
			ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace,
			ManagementTaskUIDAnnotation: string(task.UID), ManagementTaskGenerationAnnotation: fmt.Sprintf("%d", task.Generation),
			appsv1.DeprecatedTemplateGeneration: "1",
		}},
		Spec: appsv1.DaemonSetSpec{Selector: selector, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: selector.MatchLabels}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "station:v1"}}}}},
	}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(existing).WithObjects(existing).
		WithIndex(&corev1.Event{}, "involvedObject.uid", func(object client.Object) []string {
			return []string{string(object.(*corev1.Event).InvolvedObject.UID)}
		}).Build()
	execution, err := newTestKubernetesExecutionCoordinator(local, "rlark:test")
	if err != nil {
		t.Fatal(err)
	}
	controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
	var observer *pushRuntimeReconciler
	for _, observation := range controller.observations {
		if observation.GVK.Kind != "DaemonSet" {
			continue
		}
		observer, err = newPushRuntimeReconciler(controller, observation, KubernetesObservationScope{
			RuntimeID: kubernetesruntime.DefaultID, Namespace: tenant.DefaultNamespace, ManagementNamespace: task.Namespace,
		}, local)
		if err != nil {
			t.Fatal(err)
		}
	}
	if observer == nil {
		t.Fatal("DaemonSet observation is not registered")
	}
	pull := &pullReconciler{c: controller}
	key := client.ObjectKeyFromObject(task)
	if _, err := pull.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
		t.Fatal(err)
	}
	var daemonSet appsv1.DaemonSet
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(existing), &daemonSet); err != nil {
		t.Fatal(err)
	}
	if daemonSet.Spec.Template.Spec.NodeSelector["rlark.io/node-category"] != "robot" || daemonSet.Spec.UpdateStrategy.Type != appsv1.OnDeleteDaemonSetStrategyType || daemonSet.Annotations[rlarkv1alpha1.RolloutAnnotation] != "{}" {
		t.Fatalf("rendered DaemonSet spec=%#v", daemonSet.Spec)
	}

	stale := daemonSet.DeepCopy()
	stale.Annotations[ManagementTaskUIDAnnotation] = "previous-task-uid"
	if _, err := observer.enqueueRoot(context.Background(), stale); err != nil {
		t.Fatal(err)
	}
	select {
	case <-controller.triggers.events:
		t.Fatal("a previous Task incarnation enqueued a current Task")
	default:
	}

	// observe stands in for the kubelets and the DaemonSet controller: it sets
	// the nodes of the class (name -> Ready) and the Pods on them, then lets the
	// push and pull controllers report them.
	observe := func(nodes map[string]bool, pods ...*corev1.Pod) rlarkv1alpha1.TaskStatus {
		t.Helper()
		var podList corev1.PodList
		if err := local.List(context.Background(), &podList); err != nil {
			t.Fatal(err)
		}
		for i := range podList.Items {
			if err := local.Delete(context.Background(), &podList.Items[i]); err != nil {
				t.Fatal(err)
			}
		}
		var nodeList corev1.NodeList
		if err := local.List(context.Background(), &nodeList); err != nil {
			t.Fatal(err)
		}
		for i := range nodeList.Items {
			if err := local.Delete(context.Background(), &nodeList.Items[i]); err != nil {
				t.Fatal(err)
			}
		}
		for name, ready := range nodes {
			status := corev1.ConditionUnknown
			if ready {
				status = corev1.ConditionTrue
			}
			node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: status}}}}
			if err := local.Create(context.Background(), node); err != nil {
				t.Fatal(err)
			}
		}
		for _, pod := range pods {
			if err := local.Create(context.Background(), pod); err != nil {
				t.Fatal(err)
			}
		}
		if err := local.Get(context.Background(), client.ObjectKeyFromObject(existing), &daemonSet); err != nil {
			t.Fatal(err)
		}
		daemonSet.Status = appsv1.DaemonSetStatus{ObservedGeneration: daemonSet.Generation, DesiredNumberScheduled: int32(len(nodes))}
		if err := local.Status().Update(context.Background(), &daemonSet); err != nil {
			t.Fatal(err)
		}
		if _, err := observer.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(existing)}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-controller.triggers.events:
		case <-time.After(time.Second):
			t.Fatal("runtime trigger was not emitted")
		}
		if _, err := pull.Reconcile(context.Background(), reconcile.Request{NamespacedName: key}); err != nil {
			t.Fatal(err)
		}
		var got rlarkv1alpha1.Task
		if err := management.Get(context.Background(), key, &got); err != nil {
			t.Fatal(err)
		}
		return got.Status
	}
	running := func(node string) rlarkv1alpha1.TaskInstanceStatus {
		return rlarkv1alpha1.TaskInstanceStatus{Node: node, Name: "task-" + node, Phase: rlarkv1alpha1.TaskInstanceRunning, Image: "station:v1"}
	}
	pod := func(node string) *corev1.Pod { return perNodePod(&daemonSet, node, true, "") }

	status := observe(map[string]bool{"robot-a": true, "robot-b": true}, pod("robot-a"), pod("robot-b"))
	if status.Phase != rlarkv1alpha1.TaskPhaseRunning || !reflect.DeepEqual(status.ObservedNodes, []string{"robot-a", "robot-b"}) ||
		!reflect.DeepEqual(status.Instances, []rlarkv1alpha1.TaskInstanceStatus{running("robot-a"), running("robot-b")}) {
		t.Fatalf("two nodes: status=%#v", status)
	}

	// A robot is switched off: its node is not Ready and its Pod no longer
	// ready. It is listed, but the Task stays Running.
	status = observe(map[string]bool{"robot-a": true, "robot-b": false}, pod("robot-a"), perNodePod(&daemonSet, "robot-b", false, ""))
	off := rlarkv1alpha1.TaskInstanceStatus{Node: "robot-b", Name: "task-robot-b", Phase: rlarkv1alpha1.TaskInstancePending, Image: "station:v1", Message: "node not ready"}
	if status.Phase != rlarkv1alpha1.TaskPhaseRunning || status.Message != "1 of 2 instances on nodes that are not ready" ||
		!reflect.DeepEqual(status.Instances, []rlarkv1alpha1.TaskInstanceStatus{running("robot-a"), off}) {
		t.Fatalf("node switched off: status=%#v", status)
	}

	// A node joins the class but is not Ready yet: still Running.
	status = observe(map[string]bool{"robot-a": true, "robot-b": true, "robot-c": false}, pod("robot-a"), pod("robot-b"), perNodePod(&daemonSet, "robot-c", false, "ContainerCreating"))
	if status.Phase != rlarkv1alpha1.TaskPhaseRunning || status.Instances[2].Message != "node not ready" {
		t.Fatalf("node joining: status=%#v", status)
	}

	// The joined node is Ready and pulls the image: real work, so Pending.
	status = observe(map[string]bool{"robot-a": true, "robot-b": true, "robot-c": true}, pod("robot-a"), pod("robot-b"), perNodePod(&daemonSet, "robot-c", false, "ContainerCreating"))
	joining := rlarkv1alpha1.TaskInstanceStatus{Node: "robot-c", Name: "task-robot-c", Phase: rlarkv1alpha1.TaskInstancePending, Image: "station:v1", Message: "ContainerCreating"}
	if status.Phase != rlarkv1alpha1.TaskPhasePending || status.Message != "2 of 3 instances on ready nodes updated and available" ||
		!reflect.DeepEqual(status.Instances, []rlarkv1alpha1.TaskInstanceStatus{running("robot-a"), running("robot-b"), joining}) {
		t.Fatalf("node joined: status=%#v", status)
	}

	// Its instance keeps crashing: the instance is Failed, the Task only Pending.
	status = observe(map[string]bool{"robot-a": true, "robot-b": true, "robot-c": true}, pod("robot-a"), pod("robot-b"), perNodePod(&daemonSet, "robot-c", false, "CrashLoopBackOff"))
	failing := rlarkv1alpha1.TaskInstanceStatus{Node: "robot-c", Name: "task-robot-c", Phase: rlarkv1alpha1.TaskInstanceFailed, Image: "station:v1", Message: "pod task-robot-c container main: CrashLoopBackOff"}
	if status.Phase != rlarkv1alpha1.TaskPhasePending || status.Message != failing.Message ||
		!reflect.DeepEqual(status.Instances, []rlarkv1alpha1.TaskInstanceStatus{running("robot-a"), running("robot-b"), failing}) {
		t.Fatalf("node failing: status=%#v", status)
	}

	// The node leaves the class and takes its instance with it.
	status = observe(map[string]bool{"robot-a": true, "robot-b": true}, pod("robot-a"), pod("robot-b"))
	if status.Phase != rlarkv1alpha1.TaskPhaseRunning || status.Message != "" || !reflect.DeepEqual(status.Instances, []rlarkv1alpha1.TaskInstanceStatus{running("robot-a"), running("robot-b")}) {
		t.Fatalf("node left: status=%#v", status)
	}
}

func TestPerNodeInstancesAreSortedByNodeAndKeepTheMainImage(t *testing.T) {
	instances := agentruntime.InstanceSnapshot{Instances: []agentruntime.Instance{
		{Name: "task-z", Phase: "Pending", Conditions: []agentruntime.InstanceCondition{{Scope: "instance", Subject: "task-z", Reason: "FailedScheduling"}, {Scope: "image", Subject: "main", Message: "station:v2"}}},
		{Name: "task-y", Phase: "Running", Conditions: []agentruntime.InstanceCondition{{Scope: "node", Subject: "a"}, {Scope: "image", Subject: "sidecar", Message: "tools:v1"}, {Scope: "image", Subject: "main", Message: "station:v1"}, {Scope: "instance", Subject: "task-y", Reason: "Ready"}}},
	}}
	got := perNodeInstances(instances)
	want := []rlarkv1alpha1.TaskInstanceStatus{
		{Name: "task-z", Phase: rlarkv1alpha1.TaskInstancePending, Image: "station:v2", Message: "FailedScheduling"},
		{Node: "a", Name: "task-y", Phase: rlarkv1alpha1.TaskInstanceRunning, Image: "station:v1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("instances=%#v, want %#v", got, want)
	}
}
