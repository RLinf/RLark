package task

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// stubReader serves a fixed sequence of List pages and records the options it
// was called with, so pagination behaviour can be asserted without a
// management API.
type stubReader struct {
	pages []rlarkv1alpha1.TaskList
	calls []client.ListOptions
	err   error
}

func (s *stubReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return fmt.Errorf("unexpected Get")
}

func (s *stubReader) List(_ context.Context, list client.ObjectList, opts ...client.ListOption) error {
	options := client.ListOptions{}
	options.ApplyOptions(opts)
	s.calls = append(s.calls, options)
	if s.err != nil {
		return s.err
	}
	index := len(s.calls) - 1
	if index >= len(s.pages) {
		return fmt.Errorf("unexpected List call #%d", len(s.calls))
	}
	target, ok := list.(*rlarkv1alpha1.TaskList)
	if !ok {
		return fmt.Errorf("unexpected list type %T", list)
	}
	s.pages[index].DeepCopyInto(target)
	return nil
}

// recordingTimer records the waits Start requests and cancels the run once the
// expected number of rounds has happened, so Start terminates deterministically.
type recordingTimer struct {
	waits  []time.Duration
	rounds int
	cancel context.CancelFunc
}

func (t *recordingTimer) Wait(ctx context.Context, duration time.Duration) error {
	t.waits = append(t.waits, duration)
	if len(t.waits) >= t.rounds {
		t.cancel()
	}
	return ctx.Err()
}

func mkResyncTask(name string, phase rlarkv1alpha1.TaskPhase) rlarkv1alpha1.Task {
	return rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "rlark-system"},
		Status:     rlarkv1alpha1.TaskStatus{Phase: phase},
	}
}

func drain(events chan event.GenericEvent) []string {
	keys := make([]string, 0, len(events))
	for {
		select {
		case e := <-events:
			keys = append(keys, e.Object.GetNamespace()+"/"+e.Object.GetName())
		default:
			return keys
		}
	}
}

func TestDefaultResyncOptionsAreBounded(t *testing.T) {
	options := DefaultResyncOptions()
	if options.Period <= 0 {
		t.Errorf("default period must be positive, got %s", options.Period)
	}
	if options.PageSize <= 0 {
		t.Errorf("default page size must be positive, got %d", options.PageSize)
	}
}

func TestNewResyncNormalizesPageSize(t *testing.T) {
	events := make(chan event.GenericEvent, 1)
	for _, pageSize := range []int64{0, -5} {
		r := newResync(&stubReader{}, events, ResyncOptions{Period: time.Minute, PageSize: pageSize})
		if r.pageSize != defaultResyncPageSize {
			t.Errorf("page size %d: expected normalization to %d, got %d",
				pageSize, defaultResyncPageSize, r.pageSize)
		}
	}
}

// The scan must only run on the leader: a standby replica running it would
// duplicate management API load without changing any outcome.
func TestResyncNeedsLeaderElection(t *testing.T) {
	if !newResync(&stubReader{}, make(chan event.GenericEvent, 1), DefaultResyncOptions()).NeedLeaderElection() {
		t.Errorf("Task freshness resync must require leader election")
	}
}

func TestResyncStartRejectsNegativePeriod(t *testing.T) {
	r := newResync(&stubReader{}, make(chan event.GenericEvent, 1), ResyncOptions{Period: -time.Second})
	if err := r.Start(context.Background()); err == nil {
		t.Errorf("expected a negative period to be rejected")
	}
}

func TestResyncStartRequiresReaderAndChannel(t *testing.T) {
	if err := (&resync{events: make(chan event.GenericEvent, 1), period: time.Minute}).Start(context.Background()); err == nil {
		t.Errorf("expected a missing reader to be rejected")
	}
	if err := (&resync{reader: &stubReader{}, period: time.Minute}).Start(context.Background()); err == nil {
		t.Errorf("expected a missing event channel to be rejected")
	}
}

// A zero period disables the scan; the Runnable must then idle until shutdown
// instead of exiting, which the manager would treat as a failed Runnable.
func TestResyncStartWithZeroPeriodIdlesUntilShutdown(t *testing.T) {
	reader := &stubReader{}
	r := newResync(reader, make(chan event.GenericEvent, 1), ResyncOptions{Period: 0})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.Start(ctx) }()

	select {
	case err := <-done:
		t.Fatalf("Start returned before shutdown: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected a clean shutdown, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("Start did not return after shutdown")
	}
	if len(reader.calls) != 0 {
		t.Errorf("expected no List call when the scan is disabled, got %d", len(reader.calls))
	}
}

func TestResyncScanEnqueuesOnlyPendingTasks(t *testing.T) {
	reader := &stubReader{pages: []rlarkv1alpha1.TaskList{{
		Items: []rlarkv1alpha1.Task{
			mkResyncTask("pending-1", rlarkv1alpha1.TaskPhasePending),
			mkResyncTask("running-1", rlarkv1alpha1.TaskPhaseRunning),
			mkResyncTask("pending-2", rlarkv1alpha1.TaskPhasePending),
		},
	}}}
	events := make(chan event.GenericEvent, 8)
	r := newResync(reader, events, DefaultResyncOptions())

	if err := r.scan(context.Background()); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	got := drain(events)
	want := []string{"rlark-system/pending-1", "rlark-system/pending-2"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("wakeup %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestResyncScanPaginates(t *testing.T) {
	reader := &stubReader{pages: []rlarkv1alpha1.TaskList{
		{
			ListMeta: metav1.ListMeta{Continue: "page-2"},
			Items:    []rlarkv1alpha1.Task{mkResyncTask("pending-1", rlarkv1alpha1.TaskPhasePending)},
		},
		{
			Items: []rlarkv1alpha1.Task{mkResyncTask("pending-2", rlarkv1alpha1.TaskPhasePending)},
		},
	}}
	events := make(chan event.GenericEvent, 8)
	r := newResync(reader, events, ResyncOptions{Period: time.Minute, PageSize: 1})

	if err := r.scan(context.Background()); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	if len(reader.calls) != 2 {
		t.Fatalf("expected 2 List calls, got %d", len(reader.calls))
	}
	if reader.calls[0].Continue != "" {
		t.Errorf("first page must not carry a continue token, got %q", reader.calls[0].Continue)
	}
	if reader.calls[1].Continue != "page-2" {
		t.Errorf("second page must carry the server continue token, got %q", reader.calls[1].Continue)
	}
	if reader.calls[0].Limit != 1 {
		t.Errorf("expected the configured page size as the List limit, got %d", reader.calls[0].Limit)
	}
	if len(drain(events)) != 2 {
		t.Errorf("expected both pages to be enqueued")
	}
}

// A scan must walk every page of the population. Stopping early would leave
// the tail of a large Task population permanently unvisited, because each
// round restarts pagination from the beginning.
func TestResyncScanCoversEveryPage(t *testing.T) {
	pages := make([]rlarkv1alpha1.TaskList, 0, 5)
	for i := 0; i < 5; i++ {
		page := rlarkv1alpha1.TaskList{
			Items: []rlarkv1alpha1.Task{mkResyncTask(fmt.Sprintf("pending-%d", i), rlarkv1alpha1.TaskPhasePending)},
		}
		if i < 4 {
			page.ListMeta = metav1.ListMeta{Continue: fmt.Sprintf("page-%d", i+1)}
		}
		pages = append(pages, page)
	}
	reader := &stubReader{pages: pages}
	events := make(chan event.GenericEvent, 8)
	r := newResync(reader, events, ResyncOptions{Period: time.Minute, PageSize: 1})

	if err := r.scan(context.Background()); err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(reader.calls) != len(pages) {
		t.Errorf("expected the scan to read all %d pages, got %d List calls", len(pages), len(reader.calls))
	}
	if got := len(drain(events)); got != len(pages) {
		t.Errorf("expected every page to be enqueued, got %d wakeups", got)
	}
}

// A saturated channel must slow the scan down rather than drop a key: the
// controller drains the channel into a deduplicating work queue, so waiting
// costs nothing but losing a key would leave that Task stale.
func TestResyncScanWaitsForSaturatedChannel(t *testing.T) {
	reader := &stubReader{pages: []rlarkv1alpha1.TaskList{{
		Items: []rlarkv1alpha1.Task{
			mkResyncTask("pending-1", rlarkv1alpha1.TaskPhasePending),
			mkResyncTask("pending-2", rlarkv1alpha1.TaskPhasePending),
		},
	}}}
	events := make(chan event.GenericEvent, 1)
	r := newResync(reader, events, DefaultResyncOptions())

	done := make(chan error, 1)
	go func() { done <- r.scan(context.Background()) }()

	select {
	case err := <-done:
		t.Fatalf("scan returned while the channel was full: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	first := <-events
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("scan failed after the channel drained: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("scan did not resume after the channel drained")
	}

	second := <-events
	if first.Object.GetName() != "pending-1" || second.Object.GetName() != "pending-2" {
		t.Errorf("expected both Tasks to be delivered in order, got %q and %q",
			first.Object.GetName(), second.Object.GetName())
	}
}

// Shutdown must not block on a full channel: the scan has to give up waiting
// and let the Runnable exit.
func TestResyncScanStopsWaitingOnShutdown(t *testing.T) {
	reader := &stubReader{pages: []rlarkv1alpha1.TaskList{{
		Items: []rlarkv1alpha1.Task{
			mkResyncTask("pending-1", rlarkv1alpha1.TaskPhasePending),
			mkResyncTask("pending-2", rlarkv1alpha1.TaskPhasePending),
		},
	}}}
	r := newResync(reader, make(chan event.GenericEvent, 1), DefaultResyncOptions())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.scan(ctx) }()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected the scan to report cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("scan kept waiting after shutdown")
	}
}

func TestResyncScanPropagatesListErrors(t *testing.T) {
	reader := &stubReader{err: errors.New("management API down")}
	r := newResync(reader, make(chan event.GenericEvent, 1), DefaultResyncOptions())

	if err := r.scan(context.Background()); err == nil {
		t.Errorf("expected the List failure to be reported to the caller")
	}
}

// A failing scan is an external, retryable condition. Returning it from the
// Runnable would terminate the controller-manager, so Start must keep looping.
func TestResyncStartKeepsRunningAfterScanFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	timer := &recordingTimer{rounds: 2, cancel: cancel}
	r := newResync(&stubReader{err: errors.New("management API down")},
		make(chan event.GenericEvent, 1), ResyncOptions{Period: time.Minute})
	r.timer = timer
	r.jitter = func() float64 { return 0.5 }

	if err := r.Start(ctx); err != nil {
		t.Fatalf("expected Start to shut down cleanly, got %v", err)
	}
	if len(timer.waits) != 2 {
		t.Fatalf("expected 2 scan rounds before shutdown, got %d", len(timer.waits))
	}
	if timer.waits[0] != time.Minute {
		t.Errorf("expected a mid-range jitter of exactly the period, got %s", timer.waits[0])
	}
}

func TestJitteredResyncPeriodStaysWithinBand(t *testing.T) {
	period := 10 * time.Second
	for _, unit := range []float64{-1, 0, 0.5, 1, 2} {
		got := jitteredResyncPeriod(period, unit)
		if got < 8*time.Second || got > 12*time.Second {
			t.Errorf("unit %v: expected a jittered period in [8s,12s], got %s", unit, got)
		}
	}
	if got := jitteredResyncPeriod(period, 0.5); got != period {
		t.Errorf("expected the mid-range unit to yield the configured period, got %s", got)
	}
}

// Acceptance for the removed fixed-interval requeue: with every Node watch
// event dropped, one resync round must still bring the aggregated status to
// the current Node state, and the reconciler must not ask for a requeue.
func TestResyncRecoversWhenEveryWatchEventIsLost(t *testing.T) {
	scheme := newTestScheme(t)
	task := mkPullTask([]string{"node-a"}, "img:a")
	node := mkPullNode("node-a", "rlark-system", []rlarkv1alpha1.PullProgress{
		{Image: "img:a", Status: "Pulling", Message: "50%"},
	})
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(task, node).
		WithStatusSubresource(task).
		WithIndex(&rlarkv1alpha1.Task{}, taskObservedNodeField, indexTaskByObservedNodes).
		Build()
	r := &Reconciler{Client: c, Scheme: scheme}

	// No watch event ever reaches the controller; only the periodic scan does.
	events := make(chan event.GenericEvent, 8)
	scanner := newResync(&stubReader{pages: []rlarkv1alpha1.TaskList{{Items: []rlarkv1alpha1.Task{*task}}}},
		events, DefaultResyncOptions())
	if err := scanner.scan(context.Background()); err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	wakeups := drain(events)
	if len(wakeups) != 1 {
		t.Fatalf("expected the stale Task to be re-enqueued once, got %v", wakeups)
	}

	result, err := r.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: client.ObjectKeyFromObject(task),
	})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if !result.IsZero() {
		t.Errorf("expected no self-requeue, got %+v", result)
	}

	var reconciled rlarkv1alpha1.Task
	if err := c.Get(context.Background(), client.ObjectKeyFromObject(task), &reconciled); err != nil {
		t.Fatalf("failed to read back the Task: %v", err)
	}
	if len(reconciled.Status.PullProgress) != 1 || reconciled.Status.PullProgress[0].Message != "50%" {
		t.Errorf("expected the aggregated pull progress to converge, got %v", reconciled.Status.PullProgress)
	}
}
