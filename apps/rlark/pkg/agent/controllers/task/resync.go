package task

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
)

const (
	defaultResyncPageSize = int64(200)
	defaultResyncBudget   = 10_000
)

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

type resyncHost interface {
	ShouldReconcile(*rlarkv1alpha1.Task) bool
}

// TaskResync periodically repairs missed Task and Runtime events by emitting
// conservative full-reconcile triggers from current management Task state.
type TaskResync struct {
	client    client.Client
	list      func(context.Context, *rlarkv1alpha1.TaskList, ...client.ListOption) error
	namespace string
	host      resyncHost
	period    time.Duration
	emit      func(trigger.Trigger) error
	timer     resyncTimer
	jitter    func() float64
	pageSize  int64
	budget    int
}

// NewTaskResync constructs the shared management Task scanner for one Task Controller.
func NewTaskResync(kubeClient client.Client, namespace string, period time.Duration, controller *Controller) *TaskResync {
	var emit func(trigger.Trigger) error
	var taskHost resyncHost
	if controller != nil && controller.triggers != nil {
		emit = controller.triggers.emit
	}
	if controller != nil {
		taskHost = controller.host
	}
	return &TaskResync{
		client: kubeClient, namespace: namespace, host: taskHost, period: period, emit: emit,
		timer: realResyncTimer{}, jitter: rand.Float64, pageSize: defaultResyncPageSize, budget: defaultResyncBudget,
	}
}

// NeedLeaderElection keeps scanning aligned with the active management Task controller.
func (*TaskResync) NeedLeaderElection() bool { return true }

// Start immediately runs one repair scan, then waits a jittered period between scans.
func (r *TaskResync) Start(ctx context.Context) error {
	if r.client == nil || r.emit == nil || r.namespace == "" || r.host == nil {
		return fmt.Errorf("task Runtime resync requires client, namespace, Task Host, and trigger bridge")
	}
	if r.period == 0 {
		<-ctx.Done()
		return nil
	}
	if r.period < 0 {
		return fmt.Errorf("task Runtime resync period must not be negative")
	}
	for {
		if err := r.scan(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			// Management API failures are external and retryable. Returning the
			// error from a Manager Runnable would terminate the entire Agent.
			log.FromContext(ctx).Error(err, "Task Runtime resync scan failed; retrying after interval")
		}
		if err := r.timer.Wait(ctx, jitteredResyncPeriod(r.period, r.jitter())); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

func (r *TaskResync) scan(ctx context.Context) error {
	remaining := r.budget
	continuation := ""
	pressure := 0
	for remaining > 0 {
		limit := r.pageSize
		if int64(remaining) < limit {
			limit = int64(remaining)
		}
		var tasks rlarkv1alpha1.TaskList
		list := r.list
		if list == nil {
			list = func(ctx context.Context, tasks *rlarkv1alpha1.TaskList, options ...client.ListOption) error {
				return r.client.List(ctx, tasks, options...)
			}
		}
		if err := list(ctx, &tasks, client.InNamespace(r.namespace), client.Limit(limit), client.Continue(continuation)); err != nil {
			return fmt.Errorf("list management Tasks for Runtime resync: %w", err)
		}
		for i := range tasks.Items {
			task := &tasks.Items[i]
			if !r.shouldEnqueue(task) {
				continue
			}
			if err := r.emit(trigger.Trigger{
				Key: trigger.Key{Namespace: task.Namespace, Name: task.Name}, Kind: trigger.Periodic,
				ObjectUID: string(task.UID), Generation: task.Generation,
			}); err != nil {
				if errors.Is(err, ErrTriggerBridgeSaturated) {
					// Do not let one saturated key starve later pages. The bridge keeps
					// coalesced hints when possible, and future scans retry wakeups.
					pressure++
					continue
				}
				return fmt.Errorf("emit Task Runtime resync trigger for %s/%s: %w", task.Namespace, task.Name, err)
			}
		}
		remaining -= len(tasks.Items)
		continuation = tasks.Continue
		if continuation == "" || len(tasks.Items) == 0 {
			if pressure > 0 {
				log.FromContext(ctx).Info("Task Runtime resync completed with trigger bridge pressure", "affectedTasks", pressure)
			}
			return nil
		}
	}
	if pressure > 0 {
		log.FromContext(ctx).Info("Task Runtime resync reached budget with trigger bridge pressure", "affectedTasks", pressure)
	}
	return nil
}

func (r *TaskResync) shouldEnqueue(task *rlarkv1alpha1.Task) bool {
	return r.host != nil && r.host.ShouldReconcile(task)
}

func jitteredResyncPeriod(period time.Duration, unit float64) time.Duration {
	if unit < 0 {
		unit = 0
	} else if unit > 1 {
		unit = 1
	}
	return time.Duration(float64(period) * (0.8 + 0.4*unit))
}
