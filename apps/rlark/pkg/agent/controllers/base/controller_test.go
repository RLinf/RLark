package base

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func controllerRef(kind, name string) metav1.OwnerReference {
	t := true
	return metav1.OwnerReference{Kind: kind, Name: name, Controller: &t, APIVersion: "apps/v1"}
}

func makePod(name, ns string, owners ...metav1.OwnerReference) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			OwnerReferences: owners,
			Annotations:     map[string]string{managementTaskNameAnnotation: "some-task"},
		},
	}
}

func TestPodOwnerRequests(t *testing.T) {
	ctx := context.Background()

	// StatefulSet pod: owned directly by the StatefulSet.
	stsPod := makePod("actor-0", "rlark-system", controllerRef("StatefulSet", "robot-policy-training-actor"))
	got := podOwnerRequests(ctx, fake.NewClientBuilder().Build(), stsPod, "StatefulSet")
	assert.Equal(t, []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: "robot-policy-training-actor", Namespace: "rlark-system"}},
	}, got)

	// DaemonSet pod: owned directly by the DaemonSet.
	dsPod := makePod("ds-pod", "rlark-system", controllerRef("DaemonSet", "my-daemonset"))
	got = podOwnerRequests(ctx, fake.NewClientBuilder().Build(), dsPod, "DaemonSet")
	assert.Equal(t, []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: "my-daemonset", Namespace: "rlark-system"}},
	}, got)

	// Deployment pod: owned by a ReplicaSet that is owned by the Deployment.
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "app-abc",
			Namespace:       "rlark-system",
			OwnerReferences: []metav1.OwnerReference{controllerRef("Deployment", "my-app")},
		},
	}
	depPod := makePod("app-xyz", "rlark-system", controllerRef("ReplicaSet", "app-abc"))
	got = podOwnerRequests(ctx, fake.NewClientBuilder().WithObjects(rs).Build(), depPod, "Deployment")
	assert.Equal(t, []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: "my-app", Namespace: "rlark-system"}},
	}, got)

	// A StatefulSet push controller must not enqueue for a Deployment-owned pod.
	got = podOwnerRequests(ctx, fake.NewClientBuilder().WithObjects(rs).Build(), depPod, "StatefulSet")
	assert.Empty(t, got)

	// Pod without a controller owner yields nothing.
	noOwnerPod := makePod("standalone", "rlark-system")
	assert.Empty(t, podOwnerRequests(ctx, fake.NewClientBuilder().Build(), noOwnerPod, "StatefulSet"))

	// Deployment pod whose ReplicaSet has already been deleted yields nothing.
	orphanPod := makePod("app-gone", "rlark-system", controllerRef("ReplicaSet", "missing-rs"))
	assert.Empty(t, podOwnerRequests(ctx, fake.NewClientBuilder().Build(), orphanPod, "Deployment"))
}

func TestPodOwningKind(t *testing.T) {
	assert.Equal(t, "Deployment", podOwningKind(&appsv1.Deployment{}))
	assert.Equal(t, "StatefulSet", podOwningKind(&appsv1.StatefulSet{}))
	assert.Equal(t, "DaemonSet", podOwningKind(&appsv1.DaemonSet{}))
	assert.Equal(t, "", podOwningKind(&corev1.Pod{}))
	assert.Equal(t, "", podOwningKind(&corev1.Node{}))
}
