package controller

import (
	"context"
	"errors"
	"testing"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestStationJoinedReadinessIsOfTheNodesOwnRecord(t *testing.T) {
	ctx := context.Background()
	enrollment := &stationv1alpha1.NetworkEnrollment{ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: "ns"}, Spec: stationv1alpha1.NetworkEnrollmentSpec{ClusterID: "cluster", NodeUID: "uid"},
		Status: stationv1alpha1.NetworkEnrollmentStatus{Node: &stationv1alpha1.NetworkEnrollmentNode{State: stationv1alpha1.NodeStateBound}}}
	c := tunnelFixture(t, enrollment)
	reader := StationJoinedReadiness{Reader: c, Nodes: testNodeReadiness{ready: true}}
	if ready, err := reader.Ready(ctx, "cluster", "ns", "worker", "uid"); err != nil || !ready {
		t.Fatalf("matched: %v %v", ready, err)
	}
	// The record is still of the name's earlier Node, or of no node yet: waited for.
	if ready, err := reader.Ready(ctx, "cluster", "ns", "worker", "other"); err != nil || ready {
		t.Fatalf("a record of the earlier Node: %v %v", ready, err)
	}
	if ready, err := reader.Ready(ctx, "cluster", "other", "worker", "uid"); err != nil || ready {
		t.Fatalf("other namespace: %v %v", ready, err)
	}
	enrollment.Status.Node.State = stationv1alpha1.NodeStateConflict
	if err := c.Status().Update(ctx, enrollment); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Ready(ctx, "cluster", "ns", "worker", "other"); !errors.Is(err, ErrNodeIdentityChanged) {
		t.Fatalf("a record in Conflict: %v", err)
	}
}
