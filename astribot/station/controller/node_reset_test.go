package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestAReportingNodeIsResetFromInsideItsCluster(t *testing.T) {
	ctx := context.Background()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "nb260706069", UID: "node-uid"},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
	agent := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-system", Name: "rlark-agent-node"},
		Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			ImagePullSecrets: []corev1.LocalObjectReference{{Name: "ir-8be41d25ef634987"}},
			Containers:       []corev1.Container{{Name: "agent", Image: "registry.rlark.internal:5443/rlark/agent@sha256:b3a7", Args: []string{"--mode=node"}}}}}}}
	other := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-system", Name: "csi"},
		Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "csi", Image: "csi"}}}}}}
	member := k8sfake.NewSimpleClientset(node, agent, other)
	reset := InClusterNodeReset{Members: nodeMemberClients{"cluster-a": member}}
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{UID: "01234567-89ab-cdef-0123-456789abcdef"},
		Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "nb260706069", NodeUID: "node-uid"}}

	if done, err := reset.Reset(ctx, op); done || err != nil {
		t.Fatalf("first pass: %v %v", done, err)
	}
	pod, err := member.CoreV1().Pods("rlark-system").Get(ctx, nodeResetPodName(op), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	container := pod.Spec.Containers[0]
	if pod.Spec.NodeName != "nb260706069" || !pod.Spec.HostPID || !pod.Spec.HostNetwork || container.Image != "registry.rlark.internal:5443/rlark/agent@sha256:b3a7" ||
		len(pod.Spec.ImagePullSecrets) != 1 || !*container.SecurityContext.Privileged || container.Command[0] != "nsenter" || !strings.Contains(strings.Join(container.Command, " "), "systemd-run") ||
		!strings.Contains(container.Command[len(container.Command)-1], "kubeadm reset --force") {
		t.Fatalf("reset pod: %+v", pod.Spec)
	}
	// The pod has run, but the kubelet still reports: not done.
	pod.Status.Phase = corev1.PodSucceeded
	_, _ = member.CoreV1().Pods("rlark-system").UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	if done, err := reset.Reset(ctx, op); done || err != nil {
		t.Fatalf("kubelet still reporting: %v %v", done, err)
	}
	node.Status.Conditions[0].Status = corev1.ConditionUnknown
	_, _ = member.CoreV1().Nodes().UpdateStatus(ctx, node, metav1.UpdateOptions{})
	if done, err := reset.Reset(ctx, op); !done || err != nil {
		t.Fatalf("reset and stopped: %v %v", done, err)
	}

	// A reset that could not start says so.
	pod.Status.Phase, pod.Status.Message = corev1.PodFailed, "systemd-run: unit exists"
	_, _ = member.CoreV1().Pods("rlark-system").UpdateStatus(ctx, pod, metav1.UpdateOptions{})
	if _, err := reset.Reset(ctx, op); err == nil || !strings.Contains(err.Error(), "unit exists") {
		t.Fatalf("a failed reset pod: %v", err)
	}
}

func TestANodeThatNoLongerReportsIsLeftForItsNextJoin(t *testing.T) {
	ctx := context.Background()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "node-uid"}}
	member := k8sfake.NewSimpleClientset(node)
	op := &stationv1alpha1.NodeOperation{ObjectMeta: metav1.ObjectMeta{UID: "01234567-89ab-cdef-0123-456789abcdef"},
		Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "n", NodeUID: "node-uid"}}
	if done, err := (InClusterNodeReset{Members: nodeMemberClients{"cluster-a": member}}).Reset(ctx, op); !done || err != nil {
		t.Fatalf("%v %v", done, err)
	}
	if pods, _ := member.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); len(pods.Items) != 0 {
		t.Fatal("a reset pod was made for a node no kubelet runs")
	}
	op.Spec.NodeUID = "another"
	if _, err := (InClusterNodeReset{Members: nodeMemberClients{"cluster-a": member}}).Reset(ctx, op); !errors.Is(err, ErrNodeIdentityChanged) {
		t.Fatalf("another node of the name: %v", err)
	}
}
