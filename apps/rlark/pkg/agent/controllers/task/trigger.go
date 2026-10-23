package task

import (
	"context"
	"errors"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
)

const defaultTriggerChannelCapacity = 1024

// ErrTriggerBridgeSaturated reports that the bounded cross-manager event
// channel cannot currently accept a wakeup. Callers must retry through their
// own controller queue; trigger correctness must not depend on silent drops.
var ErrTriggerBridgeSaturated = errors.New("task trigger bridge is saturated")

type triggerBridge struct {
	accumulator *trigger.Accumulator
	events      chan event.GenericEvent
}

// finish completes the current trigger batch. It returns true when pending
// hints could not be woken through the bounded channel and the current
// controller request must be requeued without reporting a reconcile error.
func (b *triggerBridge) finish(key trigger.Key) bool {
	if !b.accumulator.Done(key) {
		return false
	}
	object := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name}}
	select {
	case b.events <- event.GenericEvent{Object: object}:
		return false
	default:
		b.accumulator.WakeupFailed(key)
		return true
	}
}

func (c *Controller) emitRuntimeTrigger(_ context.Context, task *rlarkv1alpha1.Task, root agentruntime.ObjectRef) error {
	if c.triggers == nil {
		return errors.New("task trigger bridge is not configured")
	}
	return c.triggers.emit(trigger.Trigger{
		Key:        trigger.Key{Namespace: task.Namespace, Name: task.Name},
		Kind:       trigger.RuntimeState,
		RuntimeID:  root.RuntimeID,
		ObjectUID:  string(task.UID),
		Generation: task.Generation,
		Root:       &root,
	})
}

func newTriggerBridge() *triggerBridge {
	return &triggerBridge{
		accumulator: trigger.NewAccumulator(trigger.Config{}),
		events:      make(chan event.GenericEvent, defaultTriggerChannelCapacity),
	}
}

func (b *triggerBridge) emit(value trigger.Trigger) error {
	if err := value.Key.Validate(); err != nil {
		return err
	}
	merged := b.accumulator.Merge(value)
	if merged.Status == trigger.MergeCoalesced {
		return nil
	}
	if merged.Status == trigger.MergeSaturated {
		if merged.Fallback == nil || merged.Fallback.Key != value.Key || !merged.Fallback.FullReconcileRequired {
			return errors.New("saturated task trigger accumulator returned an invalid fallback")
		}
		// The accumulator cannot retain this key, but the pull reconciler uses a
		// conservative full-reconcile set when Take finds no pending entry. The
		// controller-runtime queue carries only the key, so a same-key entry created
		// before consumption could replace that implicit fallback. Such an entry may
		// use fast observation only after the verified evidence gate proves it safe;
		// preserving that conservative gate is part of this key-only bridge contract.
	}
	object := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Namespace: value.Key.Namespace, Name: value.Key.Name}}
	select {
	case b.events <- event.GenericEvent{Object: object}:
		return nil
	default:
		if merged.Status == trigger.MergeEnqueue {
			// The accumulator keeps the hint but no wakeup was delivered. Mark the
			// queued item complete so a retried source event can request a new wakeup.
			b.accumulator.WakeupFailed(value.Key)
		}
		return ErrTriggerBridgeSaturated
	}
}
