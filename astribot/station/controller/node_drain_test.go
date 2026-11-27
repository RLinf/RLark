package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func TestDrainWorkerIdentityAndControlPlane(t *testing.T) {
	for _, tc := range []struct {
		name string
		node *corev1.Node
		want error
	}{
		{"replaced", &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "new"}}, ErrNodeIdentityChanged},
		{"control plane", &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "old", Labels: map[string]string{"node-role.kubernetes.io/control-plane": ""}}}, ErrNodeDrainBlocked},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(tc.node)
			if _, err := DrainWorker(context.Background(), client, "n", types.UID("old")); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
			if len(client.Actions()) != 1 {
				t.Fatalf("unexpected writes: %#v", client.Actions())
			}
		})
	}
}

func TestDrainWorkerRespectsEvictionAndRecovers(t *testing.T) {
	controller := true
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "node-uid"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "default", UID: "pod-uid", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", UID: "owner-uid", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "n"}}
	client := fake.NewSimpleClientset(node, pod)
	blocked := true
	client.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "eviction" {
			return false, nil, nil
		}
		if blocked {
			return true, nil, apierrors.NewTooManyRequests("PDB", 1)
		}
		return true, nil, nil
	})
	if _, err := DrainWorker(context.Background(), client, "n", "node-uid"); !apierrors.IsTooManyRequests(err) {
		t.Fatalf("PDB error: %v", err)
	}
	got, _ := client.CoreV1().Nodes().Get(context.Background(), "n", metav1.GetOptions{})
	if !got.Spec.Unschedulable {
		t.Fatal("node was not cordoned")
	}
	blocked = false
	if done, err := DrainWorker(context.Background(), client, "n", "node-uid"); err != nil || done {
		t.Fatalf("eviction pass: done=%v err=%v", done, err)
	}
	if err := client.CoreV1().Pods("default").Delete(context.Background(), "worker", metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if done, err := DrainWorker(context.Background(), client, "n", "node-uid"); err != nil || !done {
		t.Fatalf("completed pass: done=%v err=%v", done, err)
	}
}

func TestDrainWorkerEvictsAPodWithLocalData(t *testing.T) {
	controller := true
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "uid"}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "data", Namespace: "default", UID: "pod", OwnerReferences: []metav1.OwnerReference{{UID: "owner", Controller: &controller}}}, Spec: corev1.PodSpec{NodeName: "n", Volumes: []corev1.Volume{{Name: "scratch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}}}
	client := fake.NewSimpleClientset(node, pod)
	if _, err := DrainWorker(context.Background(), client, "n", "uid"); err != nil {
		t.Fatalf("got %v", err)
	}
	evicted := false
	for _, action := range client.Actions() {
		evicted = evicted || action.GetSubresource() == "eviction"
	}
	if !evicted {
		t.Fatal("the pod with local data was not evicted")
	}
}

func TestDrainWorkerRejectsUnmanagedPod(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "n", UID: "uid"}}, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "standalone", Namespace: "default"}, Spec: corev1.PodSpec{NodeName: "n"}})
	if _, err := DrainWorker(context.Background(), client, "n", "uid"); !errors.Is(err, ErrNodeDrainBlocked) {
		t.Fatalf("got %v", err)
	}
}
