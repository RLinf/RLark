package controller

import (
	"context"
	"testing"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestKubernetesAuditSinkCreatesAndDeduplicatesImmutableEvent(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	sink := NewKubernetesAuditSink(c, c)
	base := RebindAuditEvent{
		Namespace: "agent-ns", OperationUID: "operation-uid", OperationName: "rebind-1",
		ClusterID: "cluster-a", StationUID: "station-uid", StationName: "stn-a",
		Outcome: "Applied", ResultReason: "RebindApplied", Actor: "alice",
		OldNodeUID: "node-a-uid", NewNodeUID: "node-b-uid", OldVersion: 2, NewVersion: 3,
		Reason: "node replacement", OccurredAt: time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC),
	}
	if err := sink.Record(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	base.OccurredAt = base.OccurredAt.Add(time.Minute)
	if err := sink.Record(context.Background(), base); err != nil {
		t.Fatalf("idempotent retry failed: %v", err)
	}
	var events stationv1alpha1.StationAuditEventList
	if err := c.List(context.Background(), &events); err != nil {
		t.Fatal(err)
	}
	if len(events.Items) != 1 || events.Items[0].Name != AuditEventName("operation-uid") || events.Items[0].Spec.PreviousNodeUID != "node-a-uid" || events.Items[0].Spec.TargetNodeUID != "node-b-uid" {
		t.Fatalf("unexpected audit records: %#v", events.Items)
	}
}

func TestKubernetesAuditSinkRejectsOperationIDCollision(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	sink := NewKubernetesAuditSink(c, c)
	event := RebindAuditEvent{Namespace: "agent-ns", OperationUID: "operation-uid", OperationName: "rebind-1", ClusterID: "cluster-a", StationUID: "station-uid", StationName: "stn-a", Outcome: "Applied", ResultReason: "RebindApplied", Actor: "alice", NewNodeUID: "node-b-uid", NewVersion: 3, Reason: "reason", OccurredAt: time.Now().UTC()}
	if err := sink.Record(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	event.NewNodeUID = "different-node-uid"
	if err := sink.Record(context.Background(), event); err == nil {
		t.Fatal("accepted conflicting event under the same operation UID")
	}
}
