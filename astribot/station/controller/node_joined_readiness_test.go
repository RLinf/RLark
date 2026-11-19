package controller

import (
	"context"
	"errors"
	"testing"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestStationJoinedReadinessChecksBinding(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	station := &stationv1alpha1.Station{ObjectMeta: metav1.ObjectMeta{Name: "station", Namespace: "ns", UID: "station-uid"}, Spec: stationv1alpha1.StationSpec{Binding: stationv1alpha1.StationBinding{NodeName: "worker", NodeUID: "uid"}}, Status: stationv1alpha1.StationStatus{Phase: stationv1alpha1.StationPhaseBound}}
	reader := StationJoinedReadiness{Reader: fake.NewClientBuilder().WithScheme(scheme).WithObjects(station).Build(), Nodes: testNodeReadiness{ready: true}}
	ready, err := reader.Ready(context.Background(), "cluster", "ns", "worker", "uid")
	if err != nil || !ready {
		t.Fatalf("matched: %v %v", ready, err)
	}
	if _, err := reader.Ready(context.Background(), "cluster", "ns", "worker", "other"); !errors.Is(err, ErrStationRebindRequired) {
		t.Fatalf("replacement: %v", err)
	}
	ready, err = reader.Ready(context.Background(), "cluster", "other", "worker", "uid")
	if err != nil || ready {
		t.Fatalf("other namespace: %v %v", ready, err)
	}
	duplicate := station.DeepCopy()
	duplicate.Name = "other"
	duplicate.UID = "other"
	reader.Reader = fake.NewClientBuilder().WithScheme(scheme).WithObjects(station, duplicate).Build()
	if _, err := reader.Ready(context.Background(), "cluster", "ns", "worker", "uid"); err == nil {
		t.Fatal("accepted ambiguous binding")
	}
}
