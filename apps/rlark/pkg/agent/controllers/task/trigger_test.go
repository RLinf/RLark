package task

import (
	"errors"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
)

func TestTriggerBridgeRedeliversHintsArrivingDuringProcessing(t *testing.T) {
	bridge := newTriggerBridge()
	key := trigger.Key{Namespace: "cluster", Name: "task"}
	first := trigger.Trigger{Key: key, Kind: trigger.RuntimeState, ObjectUID: "uid", Generation: 1, RuntimeID: "kubernetes"}
	if err := bridge.emit(first); err != nil {
		t.Fatal(err)
	}
	<-bridge.events
	if _, ok := bridge.accumulator.Take(key); !ok {
		t.Fatal("first trigger was not available")
	}
	if err := bridge.emit(trigger.Trigger{Key: key, Kind: trigger.Periodic, ObjectUID: "uid", Generation: 1, RuntimeID: "kubernetes"}); err != nil {
		t.Fatal(err)
	}
	if bridge.finish(key) {
		t.Fatal("finish unexpectedly requested a direct requeue")
	}
	event := <-bridge.events
	if event.Object.GetNamespace() != key.Namespace || event.Object.GetName() != key.Name {
		t.Fatalf("event=%#v", event)
	}
	set, ok := bridge.accumulator.Take(key)
	if !ok || !set.Has(trigger.Periodic) || !set.FullReconcileRequired {
		t.Fatalf("second trigger=%#v ok=%v", set, ok)
	}
}

func TestTriggerBridgeFinishRequestsRequeueOnChannelPressure(t *testing.T) {
	bridge := &triggerBridge{
		accumulator: trigger.NewAccumulator(trigger.Config{}),
		events:      make(chan event.GenericEvent, 1),
	}
	key := trigger.Key{Namespace: "cluster", Name: "task"}
	first := trigger.Trigger{Key: key, Kind: trigger.RuntimeState, ObjectUID: "uid", Generation: 1}
	if err := bridge.emit(first); err != nil {
		t.Fatal(err)
	}
	<-bridge.events
	if _, ok := bridge.accumulator.Take(key); !ok {
		t.Fatal("first trigger was not available")
	}
	if err := bridge.emit(trigger.Trigger{Key: key, Kind: trigger.Periodic, ObjectUID: "uid", Generation: 1}); err != nil {
		t.Fatal(err)
	}
	bridge.events <- eventForTest("occupied")
	if !bridge.finish(key) {
		t.Fatal("finish did not request a direct requeue under channel pressure")
	}
	set, ok := bridge.accumulator.Take(key)
	if !ok || !set.Has(trigger.Periodic) || !set.FullReconcileRequired {
		t.Fatalf("retained trigger=%#v ok=%v", set, ok)
	}
}

func eventForTest(name string) event.GenericEvent {
	return event.GenericEvent{Object: &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Namespace: "cluster", Name: name}}}
}

func TestTriggerBridgeChannelSaturationRetainsHintForRetry(t *testing.T) {
	bridge := newTriggerBridge()
	for range cap(bridge.events) {
		bridge.events <- eventForTest("occupied")
	}
	value := trigger.Trigger{Key: trigger.Key{Namespace: "cluster", Name: "task"}, Kind: trigger.DesiredState, ObjectUID: "uid", Generation: 1}
	if err := bridge.emit(value); !errors.Is(err, ErrTriggerBridgeSaturated) {
		t.Fatalf("emit error=%v", err)
	}
	<-bridge.events
	if err := bridge.emit(value); err != nil {
		t.Fatalf("retried emit=%v", err)
	}
	set, ok := bridge.accumulator.Take(value.Key)
	if !ok || !set.Has(trigger.DesiredState) || !set.FullReconcileRequired {
		t.Fatalf("retained trigger=%#v ok=%v", set, ok)
	}
}

func TestTriggerBridgeAccumulatorSaturationDeliversFallbackKey(t *testing.T) {
	bridge := &triggerBridge{
		accumulator: trigger.NewAccumulator(trigger.Config{MaxKeys: 1}),
		events:      make(chan event.GenericEvent, 2),
	}
	retained := trigger.Trigger{Key: trigger.Key{Namespace: "cluster", Name: "retained"}, Kind: trigger.RuntimeState}
	if err := bridge.emit(retained); err != nil {
		t.Fatal(err)
	}
	<-bridge.events

	fallback := trigger.Trigger{Key: trigger.Key{Namespace: "cluster", Name: "fallback"}, Kind: trigger.Periodic, ObjectUID: "uid", Generation: 3}
	if err := bridge.emit(fallback); err != nil {
		t.Fatalf("accumulator saturation was reported as pressure: %v", err)
	}
	got := <-bridge.events
	if got.Object.GetNamespace() != fallback.Key.Namespace || got.Object.GetName() != fallback.Key.Name {
		t.Fatalf("fallback event=%s/%s", got.Object.GetNamespace(), got.Object.GetName())
	}
	if _, ok := bridge.accumulator.Take(fallback.Key); ok {
		t.Fatal("fallback key was unexpectedly retained in the saturated accumulator")
	}
	merged := bridge.accumulator.Merge(trigger.Trigger{Key: fallback.Key, Kind: trigger.Periodic})
	if merged.Status != trigger.MergeSaturated || merged.Fallback == nil || merged.Fallback.Key != fallback.Key || !merged.Fallback.FullReconcileRequired {
		t.Fatalf("fallback contract=%#v", merged)
	}
}

func TestTriggerBridgeFallbackReportsOnlyChannelPressure(t *testing.T) {
	bridge := &triggerBridge{
		accumulator: trigger.NewAccumulator(trigger.Config{MaxKeys: 1}),
		events:      make(chan event.GenericEvent, 1),
	}
	retained := trigger.Trigger{Key: trigger.Key{Namespace: "cluster", Name: "retained"}, Kind: trigger.RuntimeState}
	if err := bridge.emit(retained); err != nil {
		t.Fatal(err)
	}
	fallback := trigger.Trigger{Key: trigger.Key{Namespace: "cluster", Name: "fallback"}, Kind: trigger.Periodic}
	if err := bridge.emit(fallback); !errors.Is(err, ErrTriggerBridgeSaturated) {
		t.Fatalf("fallback channel pressure error=%v", err)
	}
}
