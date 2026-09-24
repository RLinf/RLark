package base

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func controllerRef(kind, name string, uid ...types.UID) metav1.OwnerReference {
	t := true
	ref := metav1.OwnerReference{Kind: kind, Name: name, Controller: &t, APIVersion: "apps/v1"}
	if len(uid) > 0 {
		ref.UID = uid[0]
	}
	return ref
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
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "robot-policy-training-actor", Namespace: "rlark-system", UID: "sts-uid"}}
	stsPod := makePod("actor-0", "rlark-system", controllerRef("StatefulSet", "robot-policy-training-actor", sts.UID))
	got := podOwnerRequests(ctx, fake.NewClientBuilder().WithObjects(sts).Build(), stsPod, "StatefulSet")
	assert.Equal(t, []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: "robot-policy-training-actor", Namespace: "rlark-system"}},
	}, got)

	// DaemonSet pod: owned directly by the DaemonSet.
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "my-daemonset", Namespace: "rlark-system", UID: "ds-uid"}}
	dsPod := makePod("ds-pod", "rlark-system", controllerRef("DaemonSet", "my-daemonset", ds.UID))
	got = podOwnerRequests(ctx, fake.NewClientBuilder().WithObjects(ds).Build(), dsPod, "DaemonSet")
	assert.Equal(t, []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: "my-daemonset", Namespace: "rlark-system"}},
	}, got)

	// Deployment pod: owned by a ReplicaSet that is owned by the Deployment.
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-abc", UID: "rs-uid",
			Namespace:       "rlark-system",
			OwnerReferences: []metav1.OwnerReference{controllerRef("Deployment", "my-app", "dep-uid")},
		},
	}
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "my-app", Namespace: "rlark-system", UID: "dep-uid"}}
	depPod := makePod("app-xyz", "rlark-system", controllerRef("ReplicaSet", "app-abc", rs.UID))
	got = podOwnerRequests(ctx, fake.NewClientBuilder().WithObjects(rs, dep).Build(), depPod, "Deployment")
	assert.Equal(t, []reconcile.Request{
		{NamespacedName: types.NamespacedName{Name: "my-app", Namespace: "rlark-system"}},
	}, got)

	for _, tc := range []struct {
		name string
		pod  *corev1.Pod
		kind string
		objs []client.Object
	}{
		{"direct API version", makePod("actor-0", "rlark-system", metav1.OwnerReference{APIVersion: "extensions/v1beta1", Kind: "StatefulSet", Name: sts.Name, UID: sts.UID, Controller: ptr.To(true)}), "StatefulSet", []client.Object{sts}},
		{"direct UID", makePod("actor-0", "rlark-system", controllerRef("StatefulSet", sts.Name, "wrong")), "StatefulSet", []client.Object{sts}},
		{"ReplicaSet API version", makePod("app-xyz", "rlark-system", metav1.OwnerReference{APIVersion: "extensions/v1beta1", Kind: "ReplicaSet", Name: rs.Name, UID: rs.UID, Controller: ptr.To(true)}), "Deployment", []client.Object{rs, dep}},
		{"ReplicaSet UID", makePod("app-xyz", "rlark-system", controllerRef("ReplicaSet", rs.Name, "wrong")), "Deployment", []client.Object{rs, dep}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Empty(t, podOwnerRequests(ctx, fake.NewClientBuilder().WithObjects(tc.objs...).Build(), tc.pod, tc.kind))
		})
	}
	badRS := rs.DeepCopy()
	badRS.OwnerReferences[0].APIVersion = "extensions/v1beta1"
	assert.Empty(t, podOwnerRequests(ctx, fake.NewClientBuilder().WithObjects(badRS, dep).Build(), depPod, "Deployment"))
	badRS = rs.DeepCopy()
	badRS.OwnerReferences[0].UID = "wrong"
	assert.Empty(t, podOwnerRequests(ctx, fake.NewClientBuilder().WithObjects(badRS, dep).Build(), depPod, "Deployment"))

	// A StatefulSet push controller must not enqueue for a Deployment-owned pod.
	got = podOwnerRequests(ctx, fake.NewClientBuilder().WithObjects(rs, dep).Build(), depPod, "StatefulSet")
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
