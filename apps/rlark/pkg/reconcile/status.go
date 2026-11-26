package reconcile

import (
	"context"
	"slices"

	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StatusPriority defines deterministic precedence when one reconciliation pass
// produces several status intents.
//
// Priority exists because the inputs to a pass are independent: an environment
// probe, a workload observation, a validation verdict, a lifecycle decision and
// a cleanup step may all have an opinion about Status. Without an explicit
// order the written Status would depend on evaluation order, which makes the
// same observed state produce different stored state across releases.
type StatusPriority int

const (
	// StatusPriorityEnvironment reports state observed outside the object,
	// such as reachability of a dependency.
	StatusPriorityEnvironment StatusPriority = iota + 10
	// StatusPriorityControlPlane reports a roll-up computed by the control
	// plane from state it does not own, such as a heartbeat Lease or the
	// aggregated phase of child objects.
	//
	// It outranks a bare environment probe because a roll-up already accounts
	// for one, and it is outranked by a workload observation because the data
	// plane is the authority on what the workload is actually doing.
	StatusPriorityControlPlane
	// StatusPriorityWorkload reports state observed from the managed workload.
	StatusPriorityWorkload
	// StatusPriorityInvalid reports that the desired state cannot be accepted,
	// which outranks any observation of the workload it describes.
	StatusPriorityInvalid
	// StatusPriorityLifecycle reports a lifecycle decision taken by the
	// reconciler itself.
	StatusPriorityLifecycle
	// StatusPriorityCleanup reports identity-scoped teardown, which outranks
	// every other intent because the object is going away.
	StatusPriorityCleanup
)

// ConditionMutation describes a typed condition upsert or removal.
//
// Removal is expressed as data rather than as an absent condition so that a
// low-priority intent cannot silently resurrect a condition that a
// higher-priority intent has retired.
type ConditionMutation struct {
	Condition metav1.Condition
	Remove    bool
}

// ApplyConditionMutations applies typed condition upserts and removals to a
// condition list and reports whether the list changed.
//
// It exists so that every writer applies conditions the same way: an upsert
// always goes through apimeta.SetStatusCondition (which owns
// LastTransitionTime semantics) and a retirement always goes through
// apimeta.RemoveStatusCondition. Hand-rolled slice manipulation is what lets
// two writers disagree about whether a condition was retired or merely set to
// False.
//
// generation backfills ObservedGeneration on upserts that did not set it, and
// must be the metadata.generation the mutations were computed against. Pass 0
// for observation-driven conditions, which carry no generation contract and
// must not claim one.
func ApplyConditionMutations(conditions *[]metav1.Condition, generation int64, mutations []ConditionMutation) bool {
	changed := false
	for _, mutation := range mutations {
		if mutation.Remove {
			if apimeta.RemoveStatusCondition(conditions, mutation.Condition.Type) {
				changed = true
			}
			continue
		}
		condition := mutation.Condition
		if condition.ObservedGeneration == 0 {
			condition.ObservedGeneration = generation
		}
		if apimeta.SetStatusCondition(conditions, condition) {
			changed = true
		}
	}
	return changed
}

// StatusIntent is a reconciler's declared intent for the status of one object
// incarnation.
//
// ObjectUID and Generation are identity fences, not metadata. An intent may be
// produced by an asynchronous observation that completed after the object was
// deleted and recreated, or after its Spec moved on. Carrying the identity it
// was computed against lets the merge step and the writer drop it instead of
// publishing an observation of state that no longer exists.
//
// Observed is a type parameter because the observed payload is the only
// resource-specific part of a status intent: the data plane reports observed
// nodes for a Task, while control-plane resources report their own shapes.
// Making it a parameter keeps one merge implementation for every resource
// instead of one struct per resource, and keeps the payload typed rather than
// an untyped escape hatch.
//
// The *Set booleans distinguish "no opinion" from "an opinion whose value is
// the zero value". Without them an intent could not clear a message or report
// an empty observation.
type StatusIntent[Observed any] struct {
	// ObjectUID fences the intent to one object instance. An empty value
	// matches any instance.
	ObjectUID string
	// Generation fences the intent to one Spec revision. A zero value matches
	// any generation.
	Generation int64
	// Phase is the coarse lifecycle phase to publish. An empty value means no
	// opinion.
	Phase string
	// MessageSet reports whether Message carries an opinion.
	MessageSet bool
	// Message is the human-readable detail to publish.
	Message string
	// ObservedSet reports whether Observed carries an opinion.
	ObservedSet bool
	// Observed is the resource-specific observation to publish.
	Observed Observed
	// Conditions are typed condition upserts and removals to apply.
	Conditions []ConditionMutation
	// Priority orders this intent against the other intents of the same pass.
	Priority StatusPriority
	// ReasonCode is a stable, low-cardinality reason for the published state.
	// It must satisfy ValidateReason.
	ReasonCode string
}

// StatusWriter publishes a merged status intent for one object.
//
// Implementations must be idempotent: applying the same intent to the same
// observed object twice must perform at most one write. They must also honor
// the identity fences carried by the intent and silently drop an intent that
// does not match the object they were handed, because a stale observation is
// not an error to retry.
type StatusWriter[T client.Object, Observed any] interface {
	WriteStatus(ctx context.Context, obj T, intent StatusIntent[Observed]) error
}

// MergeStatusIntents returns the highest-priority intent that still applies to
// the given object incarnation, or nil when no intent applies.
//
// Intents fenced to a different UID or generation are discarded rather than
// merged, so an observation that completed against a previous incarnation can
// never contribute to the published status. Equal-priority intents are merged
// in input order, with later opinions replacing earlier ones and conditions
// appended so the writer can apply them as typed upserts and removals.
//
// Observed is copied as an opaque value: this function cannot deep-copy an
// arbitrary payload. Callers must pass a payload they already own, exactly as
// they must for any value stored behind a pointer.
func MergeStatusIntents[Observed any](objectUID string, generation int64, intents ...StatusIntent[Observed]) *StatusIntent[Observed] {
	var merged *StatusIntent[Observed]
	for _, intent := range intents {
		if intent.ObjectUID != "" && intent.ObjectUID != objectUID {
			continue
		}
		if intent.Generation != 0 && intent.Generation != generation {
			continue
		}
		if merged == nil || intent.Priority > merged.Priority {
			winner := intent
			winner.Conditions = slices.Clone(intent.Conditions)
			merged = &winner
			continue
		}
		if intent.Priority < merged.Priority {
			continue
		}
		if intent.Phase != "" {
			merged.Phase = intent.Phase
		}
		if intent.MessageSet {
			merged.MessageSet = true
			merged.Message = intent.Message
		}
		if intent.ObservedSet {
			merged.ObservedSet = true
			merged.Observed = intent.Observed
		}
		merged.Conditions = append(merged.Conditions, intent.Conditions...)
		if intent.ReasonCode != "" {
			merged.ReasonCode = intent.ReasonCode
		}
	}
	if merged != nil {
		if merged.ObjectUID == "" {
			merged.ObjectUID = objectUID
		}
		if merged.Generation == 0 {
			merged.Generation = generation
		}
	}
	return merged
}
