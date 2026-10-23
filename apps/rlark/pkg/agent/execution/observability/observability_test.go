package observability

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestSafeObserverRecordsConfiguredSanitizedBoundedPayload(t *testing.T) {
	recorder := &RecordingObserver{}
	clock := fixedClock{now: time.Unix(100, 0)}
	observer := Safe(recorder, StaticConfigSource{Config: Config{Payloads: map[Payload]bool{PayloadManifests: true}, MaxEventBytes: 128}}, clock, nil)
	_, scope := observer.Start(context.Background(), Reconcile{Name: "task.reconcile", TaskKey: "management/task"})
	RecordDetailed(scope, "manifest", PayloadManifests, map[string]any{"kind": "Secret", "data": map[string]string{"token": "value"}, "metadata": map[string]string{"name": "secret"}, "padding": "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz"}, nil)
	scope.End(Result{Outcome: "Succeeded"})
	records := recorder.Records()
	if len(records) != 1 || len(records[0].Events) != 1 || records[0].Result == nil {
		t.Fatalf("records=%#v", records)
	}
	event := records[0].Events[0]
	if !event.At.Equal(clock.now) || len(event.Payload) > 128 {
		t.Fatalf("event=%#v", event)
	}
	if !event.Truncated {
		t.Fatal("expected bounded payload truncation")
	}
	if string(event.Payload) == "" || contains(string(event.Payload), "value") {
		t.Fatalf("secret leaked in %q", event.Payload)
	}
}

func TestSafeObserverKeepsLastValidDynamicConfig(t *testing.T) {
	recorder := &RecordingObserver{}
	source := &sequenceSource{values: []configResult{{config: Config{Payloads: map[Payload]bool{PayloadPlan: true}, MaxEventBytes: 256}}, {err: errors.New("invalid")}, {err: errors.New("invalid again")}}}
	clock := &mutableClock{now: time.Unix(1, 0)}
	observer := Safe(recorder, source, clock, nil)
	_, first := observer.Start(context.Background(), Reconcile{Name: "first"})
	RecordDetailed(first, "plan", PayloadPlan, map[string]string{"value": "first"}, nil)
	first.End(Result{})
	_, second := observer.Start(context.Background(), Reconcile{Name: "second"})
	RecordDetailed(second, "plan", PayloadPlan, map[string]string{"value": "second"}, nil)
	second.End(Result{})
	clock.now = clock.now.Add(time.Second)
	_, third := observer.Start(context.Background(), Reconcile{Name: "third"})
	third.End(Result{})
	records := recorder.Records()
	if len(records) != 3 || len(records[1].Events) != 2 || records[1].Events[0].Name != "ConfigRejected" || records[1].Events[1].Name != "plan" || len(records[2].Events) != 0 {
		t.Fatalf("records=%#v", records)
	}
}

func TestSafeObserverIsolatesPanics(t *testing.T) {
	observer := Safe(panicObserver{}, StaticConfigSource{}, fixedClock{}, nil)
	_, scope := observer.Start(context.Background(), Reconcile{})
	scope.Event(Event{Name: "event"})
	scope.End(Result{})
}

func TestProductionSanitizerRedactsSensitiveValues(t *testing.T) {
	value, err := ProductionSanitizer().Sanitize(PayloadManifests, map[string]any{
		"password": "password-value",
		"spec":     map[string]any{"containers": []any{map[string]any{"env": []any{map[string]any{"name": "NORMAL", "value": "normal-value"}}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(value)
	if contains(string(data), "password-value") || contains(string(data), "normal-value") {
		t.Fatalf("sensitive value leaked: %s", data)
	}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type mutableClock struct{ now time.Time }

func (c *mutableClock) Now() time.Time { return c.now }

type configResult struct {
	config Config
	err    error
}

type sequenceSource struct {
	values []configResult
	next   int
}

func (s *sequenceSource) Snapshot() (Config, error) {
	value := s.values[s.next]
	s.next++
	return value.config, value.err
}

type panicObserver struct{}

func (panicObserver) Start(context.Context, Reconcile) (context.Context, Scope) {
	return context.Background(), panicScope{}
}

type panicScope struct{}

func (panicScope) Event(Event) { panic("event") }
func (panicScope) End(Result)  { panic("end") }

func contains(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
