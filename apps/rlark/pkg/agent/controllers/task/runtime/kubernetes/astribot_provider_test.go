package kubernetes

import (
	"context"
	"reflect"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestProviderListInstancesProjectsDaemonSetPodNodeReadinessAndImages(t *testing.T) {
	scheme := testScheme(t)
	daemonSet := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "runtime", UID: "daemonset-uid", Annotations: managementAnnotations("task-uid")}, Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}}}}
	pod := func(name, node string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "runtime", UID: types.UID(name), Labels: map[string]string{"app": "task", "pod-template-generation": "4"}, OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: daemonSet.Name, UID: daemonSet.UID, Controller: ptr(true)}}},
			Spec:       corev1.PodSpec{NodeName: node, Containers: []corev1.Container{{Name: "main", Image: "station:v2"}, {Name: "sshd", Image: "tools:v1"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}, {Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
		}
	}
	// robot-a reports that it is idle, robot-b was last reported in use.
	ready := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "robot-a", Annotations: map[string]string{"rlark.io/in-use": ""}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
	off := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "robot-b", Annotations: map[string]string{"rlark.io/in-use": "fleet-station,s1u-robot"}}, Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionUnknown}}}}
	// robot-c's Node no longer exists.
	provider := testProvider(scheme, daemonSet, ready, off, pod("task-a", "robot-a"), pod("task-b", "robot-b"), pod("task-c", "robot-c"))
	snapshot, err := provider.ListInstances(context.Background(), agentruntime.InstanceQuery{Task: taskIdentity(), Workloads: []agentruntime.ObjectRef{objectRefFor(daemonSet, "apps/v1, Kind=DaemonSet")}})
	if err != nil {
		t.Fatal(err)
	}
	conditions := func(name, node string, nodeReady bool, inUse string) []agentruntime.InstanceCondition {
		result := []agentruntime.InstanceCondition{
			{Source: "kubernetes", Scope: "image", Subject: "main", Message: "station:v2"},
			{Source: "kubernetes", Scope: "image", Subject: "sshd", Message: "tools:v1"},
			{Source: "kubernetes", Scope: "instance", Subject: name, Reason: "Ready"},
			{Source: "kubernetes", Scope: "node", Subject: node},
		}
		if inUse != "" {
			result = append(result, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "node", Subject: node, Reason: "InUse", Message: inUse})
		}
		if !nodeReady {
			result = append(result, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "node", Subject: node, Reason: "NotReady"})
		}
		return append(result, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "revision", Subject: name, Message: "4"})
	}
	if len(snapshot.Instances) != 3 {
		t.Fatalf("instances=%#v", snapshot.Instances)
	}
	for i, want := range [][]agentruntime.InstanceCondition{conditions("task-a", "robot-a", true, ""), conditions("task-b", "robot-b", false, "fleet-station,s1u-robot"), conditions("task-c", "robot-c", false, "")} {
		if !reflect.DeepEqual(snapshot.Instances[i].Conditions, want) {
			t.Fatalf("instance %d conditions=%#v, want %#v", i, snapshot.Instances[i].Conditions, want)
		}
	}
}
