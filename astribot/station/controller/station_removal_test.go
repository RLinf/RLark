package controller

import (
	"context"
	"strings"
	"testing"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type memberNodes map[string]string

func (m memberNodes) VerifyNode(_ context.Context, _, _, name string) (string, string, bool, error) {
	uid, exists := m[name]
	return uid, "", exists, nil
}

// A retired Station an administrator asked to have deleted goes once nothing
// stands on it (nb260706069's Station-0004 after the machine joined again, the
// 3090's Station-0002 after it left the office cluster): with the observation
// that is its node's, leaving an audit event. One that is not retired, or whose
// node is in its cluster with no Station of its own, stays and says why.
func TestARetiredStationIsDeletedWhenAskedAndNothingStandsOnIt(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	station := func(name, node, uid string, retired, asked bool) *stationv1alpha1.Station {
		s := &stationv1alpha1.Station{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: name, UID: types.UID("uid-of-" + name)},
			Spec: stationv1alpha1.StationSpec{DisplayName: "Station-" + name, Sequence: 1, Retired: retired, Binding: stationv1alpha1.StationBinding{NodeName: node, NodeUID: uid, Version: 2, BoundBy: "system"}}}
		if asked {
			s.Annotations = map[string]string{StationDeleteRequestedAnnotation: "alice"}
		}
		return s
	}
	observation := func(node, uid string) *stationv1alpha1.StationObservation {
		return &stationv1alpha1.StationObservation{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: node, UID: types.UID("observation-" + node), Annotations: map[string]string{"station.rlark.io/node-uid": uid}}}
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		// nb260706069 joined again: its old Station is retired, a new one stands for the name, whose observation it is.
		station("old", "nb260706069", "old-uid", true, true), station("new", "nb260706069", "new-uid", false, false), observation("nb260706069", "new-uid"),
		// A node that left: nothing of its name is in the cluster; its last observation is still stored.
		station("left", "alexfu", "left-uid", true, true), observation("alexfu", "left-uid"),
		// Retired by mistake while its node is still in the cluster and has no other Station.
		station("held", "wendychen", "held-uid", true, true),
		// Not retired.
		station("bound", "cailiang", "bound-uid", false, true),
	).Build()
	r := &StationRemovalReconciler{Client: c, Reader: c, Registrations: testRegistrations{testNamespace: {ClusterID: "cluster-a", Namespace: testNamespace}},
		IdentityOps: NewSerializedIdentityOperations(), Targets: memberNodes{"nb260706069": "new-uid", "wendychen": "held-uid", "cailiang": "bound-uid"}}
	for _, name := range []string{"old", "left", "held", "bound", "old"} {
		if _, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: client.ObjectKey{Namespace: testNamespace, Name: name}}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	get := func(name string) (*stationv1alpha1.Station, error) {
		s := &stationv1alpha1.Station{}
		return s, c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: name}, s)
	}
	for _, name := range []string{"old", "left"} {
		if _, err := get(name); !apierrors.IsNotFound(err) {
			t.Fatalf("retired Station %s must be deleted: %v", name, err)
		}
	}
	if err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: "nb260706069"}, &stationv1alpha1.StationObservation{}); err != nil {
		t.Fatalf("the observation of the machine that joined again must stay: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKey{Namespace: testNamespace, Name: "alexfu"}, &stationv1alpha1.StationObservation{}); !apierrors.IsNotFound(err) {
		t.Fatalf("the observation of the node that left must go: %v", err)
	}
	for name, why := range map[string]string{"held": "no other Station stands for it", "bound": "only a retired Station"} {
		s, err := get(name)
		if err != nil || s.Annotations[StationDeleteRequestedAnnotation] != "" || !strings.Contains(s.Annotations[StationDeleteRefusedAnnotation], why) {
			t.Fatalf("%s must stay and say why: %v %v", name, s.Annotations, err)
		}
	}
	var events stationv1alpha1.StationAuditEventList
	if err := c.List(ctx, &events, client.InNamespace(testNamespace)); err != nil || len(events.Items) != 2 {
		t.Fatalf("audit events: %d %v", len(events.Items), err)
	}
	for _, event := range events.Items {
		if event.Spec.Action != "Delete" || event.Spec.Actor != "admin:alice" || event.Spec.Outcome != "Applied" {
			t.Fatalf("audit event: %+v", event.Spec)
		}
	}
}
