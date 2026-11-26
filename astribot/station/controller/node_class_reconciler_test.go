package controller

import (
	"context"
	"reflect"
	"testing"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestNodeClassReconcilerKeepsNodesAtTheirClass(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	class := &stationv1alpha1.NodeClass{ObjectMeta: metav1.ObjectMeta{Name: "robot-s1u"}, Spec: stationv1alpha1.NodeClassSpec{
		Labels: map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/node-category": "robot"}, Taints: []string{"rlark.io/node-category=robot:NoSchedule"},
		Naming: &stationv1alpha1.NodeClassNaming{NodeName: "s1u-%02d-x86"}}}
	admission := func(name string, labels map[string]string) *stationv1alpha1.NodeAdmission {
		return &stationv1alpha1.NodeAdmission{
			ObjectMeta: metav1.ObjectMeta{Namespace: "agents-robot", Name: name},
			Spec:       stationv1alpha1.NodeAdmissionSpec{ClusterID: "rlark-robot-s1u-01", Labels: labels, NodeClass: "robot-s1u"}}
	}
	node := func(name string, labels, annotations map[string]string) *unstructured.Unstructured {
		n := &unstructured.Unstructured{}
		n.SetGroupVersionKind(rlarkNodeGVK)
		n.SetNamespace("rlark-robot-s1u-01")
		n.SetName(name)
		n.SetLabels(labels)
		n.SetAnnotations(annotations)
		return n
	}
	// s1u-03 came in when the class also declared robot-body and before it
	// declared the category; s1u-04 has not joined; s1u-01 was admitted by hand.
	byHand := admission("s1u-01-x86", map[string]string{"rlark.io/robot-id": "s1u-01"})
	byHand.Spec.NodeClass = ""
	// s1u-05 came in from the list of machines with a number and a body of its own.
	numbered := admission("s1u-05-x86", map[string]string{"rlark.io/robot-model": "old", "rlark.io/robot-id": "s1u-05", "rlark.io/node-category": "observer"})
	numbered.Annotations = map[string]string{machineLabelsAnnotation: "rlark.io/node-category,rlark.io/robot-id"}
	// s1u-04's number is a Station someone gave s1u-01 by hand: it takes the first free one.
	byHand.Spec.Labels[stationLabel] = "S04"
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(class,
		admission("s1u-03-x86", map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/robot-body": "shadow"}),
		admission("s1u-04-x86", nil), byHand, numbered,
		node("s1u-03-x86", map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/robot-body": "shadow", "kubernetes.io/hostname": "s1u-03-x86"},
			map[string]string{nodeClassLabelsAnnotation: "rlark.io/robot-body,rlark.io/robot-model"}),
		node("s1u-01-x86", map[string]string{"rlark.io/robot-id": "s1u-01"}, nil)).Build()
	r := &NodeClassReconciler{Client: c, Reader: c}
	ctx := context.Background()
	for range 2 { // the second pass changes nothing
		if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: types.NamespacedName{Name: "robot-s1u"}}); err != nil {
			t.Fatal(err)
		}
	}

	for name, station := range map[string]string{"s1u-03-x86": "S03", "s1u-04-x86": "S01"} {
		var stored stationv1alpha1.NodeAdmission
		if err := c.Get(ctx, client.ObjectKey{Namespace: "agents-robot", Name: name}, &stored); err != nil ||
			!reflect.DeepEqual(stored.Spec.Labels, map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/node-category": "robot", stationLabel: station}) ||
			!reflect.DeepEqual(stored.Spec.Taints, class.Spec.Taints) || stored.Annotations[machineLabelsAnnotation] != stationLabel {
			t.Fatalf("admission %s after the class changed: %+v %v %v", name, stored.Spec, stored.Annotations, err)
		}
	}
	var kept stationv1alpha1.NodeAdmission
	if err := c.Get(ctx, client.ObjectKey{Namespace: "agents-robot", Name: "s1u-05-x86"}, &kept); err != nil ||
		!reflect.DeepEqual(kept.Spec.Labels, map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/robot-id": "s1u-05", "rlark.io/node-category": "observer", stationLabel: "S05"}) {
		t.Fatalf("the labels a machine has of its own stay: %+v %v", kept.Spec.Labels, err)
	}
	got := node("s1u-03-x86", nil, nil)
	if err := c.Get(ctx, client.ObjectKeyFromObject(got), got); err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"rlark.io/robot-model": "s1u", "rlark.io/node-category": "robot", "kubernetes.io/hostname": "s1u-03-x86", stationLabel: "S03"}; !reflect.DeepEqual(got.GetLabels(), want) ||
		got.GetAnnotations()[nodeClassLabelsAnnotation] != "rlark.io/node-category,rlark.io/robot-model,rlark.io/station" {
		t.Fatalf("management Node of s1u-03: %v %v", got.GetLabels(), got.GetAnnotations())
	}
	untouched := node("s1u-01-x86", nil, nil)
	if err := c.Get(ctx, client.ObjectKeyFromObject(untouched), untouched); err != nil || !reflect.DeepEqual(untouched.GetLabels(), map[string]string{"rlark.io/robot-id": "s1u-01"}) {
		t.Fatalf("a node admitted by hand keeps its labels: %v %v", untouched.GetLabels(), err)
	}
}
