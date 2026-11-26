package task

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const (
	// defaultResyncPeriod is the fallback scan interval. Watch events carry the
	// fast path; this period only bounds how long a lost event can keep an
	// aggregated Task status stale.
	defaultResyncPeriod = 2 * time.Minute

	// defaultResyncPageSize bounds one List page so a large Task population
	// cannot turn a scan into a single oversized management API request.
	defaultResyncPageSize = int64(200)

	// resyncChannelCapacity buffers the cross-goroutine wakeup channel so a
	// scan rarely has to wait on the controller. The channel only carries keys;
	// when it is full the scan blocks until the controller drains it, which
	// paces the scan to what the reconciler can actually absorb.
	resyncChannelCapacity = 1024
)

// ResyncOptions configures the periodic Task freshness scan.
type ResyncOptions struct {
	// Period is the interval between scans, jittered to 0.8x-1.2x so replicas
	// and restarts do not align into a thundering herd. Zero disables the scan
	// (watch events only); negative values are rejected.
	Period time.Duration

	// PageSize bounds one List page. Zero or negative uses the default.
	PageSize int64
}

// DefaultResyncOptions returns the default Task freshness scan settings.
func DefaultResyncOptions() ResyncOptions {
	return ResyncOptions{
		Period:   defaultResyncPeriod,
		PageSize: defaultResyncPageSize,
	}
}

type resyncTimer interface {
	Wait(context.Context, time.Duration) error
}

type realResyncTimer struct{}

func (realResyncTimer) Wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// resync periodically re-enqueues Pending Tasks so a lost Node watch event
// cannot keep Task.status.pullProgress or Task.status.events stale forever.
//
// It carries no memory of previous rounds: every scan recomputes the candidate
// set from current management state, and the reconciler re-derives the
// aggregated value from current Node state.
type resync struct {
	reader   client.Reader
	events   chan event.GenericEvent
	period   time.Duration
	pageSize int64
	timer    resyncTimer
	jitter   func() float64
}

// newResync constructs the freshness scanner for one Task reconciler.
//
// reader must be an uncached API reader: the controller-runtime cache rejects
// a List that carries a continue token, so paginated scans cannot use the
// cached client.
func newResync(reader client.Reader, events chan event.GenericEvent, options ResyncOptions) *resync {
	pageSize := options.PageSize
	if pageSize <= 0 {
		pageSize = defaultResyncPageSize
	}
	return &resync{
		reader:   reader,
		events:   events,
		period:   options.Period,
		pageSize: pageSize,
		timer:    realResyncTimer{},
		jitter:   rand.Float64,
	}
}

// NeedLeaderElection keeps the scan aligned with the active Task controller, so
// standby replicas never duplicate the scan load.
func (*resync) NeedLeaderElection() bool { return true }

// Start runs one scan immediately, then waits a jittered period between scans.
func (r *resync) Start(ctx context.Context) error {
	if r.reader == nil || r.events == nil {
		return fmt.Errorf("task freshness resync requires an API reader and an event channel")
	}
	if r.period < 0 {
		return fmt.Errorf("task freshness resync period must not be negative")
	}
	if r.period == 0 {
		<-ctx.Done()
		return nil
	}
	for {
		if err := r.scan(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// Management API failures are external and retryable. Returning the
			// error from a Manager Runnable would terminate the whole process.
			log.FromContext(ctx).Error(err, "Task freshness resync scan failed; retrying after interval")
		}
		if err := r.timer.Wait(ctx, jitteredResyncPeriod(r.period, r.jitter())); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

// scan walks the whole Task population once. Every round starts a fresh
// pagination from the beginning, so no Task can be permanently skipped; the
// page size is the only bound, and it only keeps a single request from growing
// unbounded.
func (r *resync) scan(ctx context.Context) error {
	continuation := ""
	for {
		var tasks rlarkv1alpha1.TaskList
		if err := r.reader.List(ctx, &tasks, client.Limit(r.pageSize), client.Continue(continuation)); err != nil {
			return fmt.Errorf("list Tasks for freshness resync: %w", err)
		}
		for i := range tasks.Items {
			task := &tasks.Items[i]
			if task.Status.Phase != rlarkv1alpha1.TaskPhasePending {
				continue
			}
			if err := r.enqueue(ctx, task); err != nil {
				return err
			}
		}
		continuation = tasks.Continue
		if continuation == "" || len(tasks.Items) == 0 {
			return nil
		}
	}
}

// enqueue delivers a key-only wakeup, waiting for channel space instead of
// dropping the key. The controller drains the channel into a deduplicating
// unbounded work queue, so waiting here only slows the scan down to the rate
// the reconciler sustains and never loses a Task.
func (r *resync) enqueue(ctx context.Context, task *rlarkv1alpha1.Task) error {
	object := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Namespace: task.Namespace,
		Name:      task.Name,
	}}
	select {
	case r.events <- event.GenericEvent{Object: object}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// jitteredResyncPeriod spreads scans over 0.8x-1.2x the configured period.
func jitteredResyncPeriod(period time.Duration, unit float64) time.Duration {
	if unit < 0 {
		unit = 0
	} else if unit > 1 {
		unit = 1
	}
	return time.Duration(float64(period) * (0.8 + 0.4*unit))
}
