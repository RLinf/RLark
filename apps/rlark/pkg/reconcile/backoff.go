package reconcile

import (
	"math/rand/v2"
	"time"
)

// Default requeue intervals per RetryClass. They exist so that a controller
// expresses *why* it wants to come back (the class) instead of hard-coding
// *when* (a literal duration). Before this package every controller carried its
// own constant, which made the platform's retry behaviour impossible to reason
// about or tune in one place.
const (
	// DefaultConflictBackoff is short because a conflict means another writer
	// won a race that we can immediately re-attempt against fresh state.
	DefaultConflictBackoff = 250 * time.Millisecond
	// DefaultInProgressBackoff paces a self-driven multi-step operation, such
	// as finalizer cleanup that has stopped children but not yet deleted them.
	DefaultInProgressBackoff = time.Second
	// DefaultTransientBackoff paces retries of an external effect that failed
	// for a reason we expect to disappear on its own.
	DefaultTransientBackoff = 5 * time.Second
	// DefaultJitterFraction spreads requeues of objects whose deadlines were
	// created together, so a controller restart does not rebuild a thundering
	// herd of simultaneous wakeups.
	DefaultJitterFraction = 0.1
)

// Backoff turns a RetryDirective into the concrete interval handed back to
// controller-runtime. It is a value type so that each controller can hold its
// own tuned copy without sharing mutable state.
type Backoff struct {
	Conflict   time.Duration
	InProgress time.Duration
	Transient  time.Duration

	// JitterFraction is the fraction of the base interval that may be added to
	// it. Jitter is strictly additive: a requeue may fire later than the base
	// interval but never earlier. Deadline-driven callers (such as a lease
	// expiry or an archive TTL) depend on that, because waking up early only
	// burns a reconcile pass without being able to make progress.
	JitterFraction float64

	// Rand returns a value in [0, 1). It exists so tests can make jitter
	// deterministic; a nil Rand uses the process-wide generator.
	Rand func() float64
}

// DefaultBackoff returns the platform defaults.
func DefaultBackoff() Backoff {
	return Backoff{
		Conflict:       DefaultConflictBackoff,
		InProgress:     DefaultInProgressBackoff,
		Transient:      DefaultTransientBackoff,
		JitterFraction: DefaultJitterFraction,
	}
}

// Interval resolves one directive.
//
// An explicit After always wins, because a caller that computed a deadline from
// observed state (lease expiry, TTL remaining) knows something this package
// does not. Otherwise the class selects the interval, and RetryNone and
// RetryOnChange resolve to zero: those two classes mean "do not poll, wait for
// a watch event", which is the behaviour a correct level-triggered controller
// should fall back to.
func (b Backoff) Interval(directive RetryDirective) time.Duration {
	base := directive.After
	if base <= 0 {
		switch directive.Class {
		case RetryConflict:
			base = b.orDefault(b.Conflict, DefaultConflictBackoff)
		case RetryInProgress:
			base = b.orDefault(b.InProgress, DefaultInProgressBackoff)
		case RetryTransient:
			base = b.orDefault(b.Transient, DefaultTransientBackoff)
		case RetryNone, RetryOnChange:
			return 0
		default:
			return 0
		}
	}
	return base + b.jitter(base)
}

func (b Backoff) orDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func (b Backoff) jitter(base time.Duration) time.Duration {
	if b.JitterFraction <= 0 || base <= 0 {
		return 0
	}
	source := b.Rand
	if source == nil {
		source = rand.Float64
	}
	fraction := b.JitterFraction
	if fraction > 1 {
		fraction = 1
	}
	return time.Duration(float64(base) * fraction * source())
}
