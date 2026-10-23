package observability

import (
	"context"
	"sync"
)

// RecordingObserver stores immutable observations in memory for focused tests.
type RecordingObserver struct {
	mu      sync.Mutex
	records []*recordState
}

// Record is one completed or active reconciliation observation.
type Record struct {
	Root   Reconcile
	Events []Event
	Result *Result
}

type recordState struct {
	mu     sync.Mutex
	record Record
}

// Start implements Observer.
func (o *RecordingObserver) Start(ctx context.Context, root Reconcile) (context.Context, Scope) {
	record := &recordState{record: Record{Root: root}}
	o.mu.Lock()
	o.records = append(o.records, record)
	o.mu.Unlock()
	return ctx, recordingScope{record: record}
}

// Records returns a deep copy of current records.
func (o *RecordingObserver) Records() []Record {
	o.mu.Lock()
	records := append([]*recordState(nil), o.records...)
	o.mu.Unlock()
	result := make([]Record, 0, len(records))
	for _, record := range records {
		record.mu.Lock()
		copyRecord := Record{Root: record.record.Root, Events: append([]Event(nil), record.record.Events...)}
		for i := range copyRecord.Events {
			copyRecord.Events[i].Attributes = cloneStrings(copyRecord.Events[i].Attributes)
			copyRecord.Events[i].Payload = append([]byte(nil), copyRecord.Events[i].Payload...)
		}
		if record.record.Result != nil {
			value := *record.record.Result
			copyRecord.Result = &value
		}
		record.mu.Unlock()
		result = append(result, copyRecord)
	}
	return result
}

type recordingScope struct{ record *recordState }

func (s recordingScope) Event(event Event) {
	s.record.mu.Lock()
	s.record.record.Events = append(s.record.record.Events, event)
	s.record.mu.Unlock()
}

func (s recordingScope) End(result Result) {
	s.record.mu.Lock()
	s.record.record.Result = &result
	s.record.mu.Unlock()
}
