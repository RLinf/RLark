// Package trigger defines best-effort Task reconciliation hints. Triggers are
// never authoritative state; consumers must re-read Task and Runtime state.
package trigger

import (
	"errors"
	"sync"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// Kind identifies the source and safe use of one reconciliation hint.
type Kind string

const (
	// DesiredState reports a management Task change and always requires a full reconcile.
	DesiredState Kind = "DesiredState"
	// RuntimeState reports an observed Runtime root change that may qualify for targeted observation.
	RuntimeState Kind = "RuntimeState"
	// Periodic repairs state after missed events or process restart and always requires a full reconcile.
	Periodic Kind = "Periodic"
)

// Key is the stable queue identity of one logical management Task.
type Key struct {
	Namespace string
	Name      string
}

// Validate reports whether the queue identity is complete.
func (k Key) Validate() error {
	if k.Namespace == "" || k.Name == "" {
		return errors.New("trigger Task namespace and name are required")
	}
	return nil
}

// String returns the namespace-qualified Task key.
func (k Key) String() string { return k.Namespace + "/" + k.Name }

// Trigger is a best-effort hint carrying the Task incarnation observed by its source.
type Trigger struct {
	Key        Key
	Kind       Kind
	RuntimeID  agentruntime.ID
	ObjectUID  string
	Generation int64
	Root       *agentruntime.ObjectRef
}

// Set is one atomically taken batch for a Task key.
type Set struct {
	Key                   Key
	Kinds                 map[Kind]struct{}
	RuntimeID             agentruntime.ID
	ObjectUID             string
	Generation            int64
	Roots                 []agentruntime.ObjectRef
	FullReconcileRequired bool
}

// Has reports whether the batch contains a hint of kind.
func (s Set) Has(kind Kind) bool {
	_, found := s.Kinds[kind]
	return found
}

// MergeStatus reports whether a merged trigger must be enqueued.
type MergeStatus int

const (
	// MergeCoalesced means an existing queue item or active reconciliation will observe the hint.
	MergeCoalesced MergeStatus = iota
	// MergeEnqueue means the caller must enqueue the key.
	MergeEnqueue
	// MergeSaturated means the bounded accumulator could not retain a new key;
	// the caller must enqueue a conservative full reconcile without relying on Take.
	MergeSaturated
)

// MergeResult tells the caller how to bridge a hint into its queue. Fallback is
// populated only for saturation, when the bounded accumulator cannot retain a
// new key and the caller must reconcile conservatively without calling Take.
type MergeResult struct {
	Status   MergeStatus
	Fallback *Set
}

// Config bounds in-memory hint retention. Zero values select conservative defaults.
type Config struct {
	MaxKeys        int
	MaxRootsPerKey int
}

const (
	defaultMaxKeys        = 10_000
	defaultMaxRootsPerKey = 16
)

// Accumulator coalesces hints while controller-runtime queues only Task keys.
// It is process-local and never an authoritative store.
type Accumulator struct {
	mu             sync.Mutex
	maxKeys        int
	maxRootsPerKey int
	entries        map[Key]*entry
}

type entry struct {
	pending    Set
	processing bool
	queued     bool
}

// NewAccumulator constructs a bounded concurrent accumulator.
func NewAccumulator(config Config) *Accumulator {
	if config.MaxKeys <= 0 {
		config.MaxKeys = defaultMaxKeys
	}
	if config.MaxRootsPerKey <= 0 {
		config.MaxRootsPerKey = defaultMaxRootsPerKey
	}
	return &Accumulator{maxKeys: config.MaxKeys, maxRootsPerKey: config.MaxRootsPerKey, entries: make(map[Key]*entry)}
}

// Merge coalesces one hint. MergeEnqueue requires exactly one queue Add by the caller.
func (a *Accumulator) Merge(value Trigger) MergeResult {
	a.mu.Lock()
	defer a.mu.Unlock()

	current := a.entries[value.Key]
	if current == nil {
		if len(a.entries) >= a.maxKeys {
			fallback := emptySet(value.Key)
			mergeSet(&fallback, value, a.maxRootsPerKey)
			fallback.Roots = nil
			fallback.FullReconcileRequired = true
			return MergeResult{Status: MergeSaturated, Fallback: &fallback}
		}
		current = &entry{pending: emptySet(value.Key)}
		a.entries[value.Key] = current
	}
	mergeSet(&current.pending, value, a.maxRootsPerKey)
	if current.processing || current.queued {
		return MergeResult{Status: MergeCoalesced}
	}
	current.queued = true
	return MergeResult{Status: MergeEnqueue}
}

// Take atomically removes the current batch and marks key as processing.
func (a *Accumulator) Take(key Key) (Set, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	current := a.entries[key]
	if current == nil || current.processing {
		return Set{}, false
	}
	current.queued = false
	current.processing = true
	result := cloneSet(current.pending)
	current.pending = emptySet(key)
	return result, true
}

// Done completes processing. true means hints arrived after Take and the caller
// must enqueue key again. Done deletes idle entries so capacity remains bounded.
func (a *Accumulator) Done(key Key) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	current := a.entries[key]
	if current == nil || !current.processing {
		return false
	}
	current.processing = false
	if setEmpty(current.pending) {
		delete(a.entries, key)
		return false
	}
	if current.queued {
		return false
	}
	current.queued = true
	return true
}

// WakeupFailed releases a queued marker after the caller could not deliver the
// required queue wakeup. Pending hints remain intact so a retried Merge can
// request another enqueue. It returns false when the key is absent, processing,
// or no wakeup was outstanding.
func (a *Accumulator) WakeupFailed(key Key) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	current := a.entries[key]
	if current == nil || current.processing || !current.queued {
		return false
	}
	current.queued = false
	return true
}

func emptySet(key Key) Set { return Set{Key: key, Kinds: make(map[Kind]struct{})} }

func setEmpty(value Set) bool {
	return len(value.Kinds) == 0 && value.ObjectUID == "" && value.Generation == 0 && value.RuntimeID == "" && len(value.Roots) == 0 && !value.FullReconcileRequired
}

func mergeSet(set *Set, value Trigger, maxRoots int) {
	set.Kinds[value.Kind] = struct{}{}
	wasFullReconcile := set.FullReconcileRequired
	if value.Kind == DesiredState || value.Kind == Periodic || value.Kind == "" {
		set.FullReconcileRequired = true
	}
	if set.ObjectUID == "" {
		set.ObjectUID = value.ObjectUID
	} else if value.ObjectUID != "" && set.ObjectUID != value.ObjectUID {
		set.FullReconcileRequired = true
	}
	if value.ObjectUID == "" || set.ObjectUID == value.ObjectUID {
		if value.Generation > set.Generation {
			set.Generation = value.Generation
		}
	}
	if set.RuntimeID == "" {
		set.RuntimeID = value.RuntimeID
	} else if value.RuntimeID != "" && set.RuntimeID != value.RuntimeID {
		set.FullReconcileRequired = true
	}
	if set.FullReconcileRequired && !wasFullReconcile {
		set.Roots = nil
	}
	if value.Root == nil || set.FullReconcileRequired {
		return
	}
	for _, existing := range set.Roots {
		if sameRoot(existing, *value.Root) {
			return
		}
	}
	if len(set.Roots) >= maxRoots {
		set.Roots = nil
		set.FullReconcileRequired = true
		return
	}
	set.Roots = append(set.Roots, *value.Root)
}

func sameRoot(left, right agentruntime.ObjectRef) bool {
	return left.RuntimeID == right.RuntimeID && left.ID == right.ID && left.Namespace == right.Namespace && left.Name == right.Name && left.UID == right.UID
}

func cloneSet(value Set) Set {
	result := value
	result.Kinds = make(map[Kind]struct{}, len(value.Kinds))
	for kind := range value.Kinds {
		result.Kinds[kind] = struct{}{}
	}
	result.Roots = append([]agentruntime.ObjectRef(nil), value.Roots...)
	return result
}
