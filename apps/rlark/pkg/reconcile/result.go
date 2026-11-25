// Package reconcile holds the resource-neutral reconciliation primitives shared
// by the control plane and the data plane.
//
// A reconciler in RLark answers two questions on every pass: what happened
// (Outcome), and when should this object be visited again (RetryDirective).
// Keeping both as explicit data rather than as an (error, requeue) pair is what
// lets the surrounding shell classify a pass without inspecting error text, and
// lets tests assert the classification directly.
//
// This package deliberately knows nothing about any particular API type. It is
// imported by both the control-plane controllers and the data-plane execution
// Host, so it must not depend on either.
package reconcile

import "time"

// Outcome is the mutually exclusive classification of one reconciliation pass.
//
// Exactly one Outcome describes a pass. Callers must switch on it rather than
// infer intent from whether an error was returned, because several outcomes are
// expected steady states that carry no error at all.
type Outcome string

const (
	// OutcomeSucceeded means desired state was reached for the observed
	// generation and nothing further is pending.
	OutcomeSucceeded Outcome = "Succeeded"
	// OutcomeWaitingForChange means progress depends on an external state
	// change that will arrive as a watch event, so no timed retry is needed.
	OutcomeWaitingForChange Outcome = "WaitingForChange"
	// OutcomeRetrying means the pass failed in a way that is expected to
	// resolve on its own and must be retried.
	OutcomeRetrying Outcome = "Retrying"
	// OutcomeCleanupInProgress means identity-scoped teardown is still running
	// and the object must be revisited until it completes.
	OutcomeCleanupInProgress Outcome = "CleanupInProgress"
	// OutcomeBlocked means a stable condition prevents progress and retrying
	// unchanged input cannot help.
	OutcomeBlocked Outcome = "Blocked"
	// OutcomeDropped means the object is not owned by this reconciler, or is
	// no longer relevant, and was intentionally not acted upon.
	OutcomeDropped Outcome = "Dropped"
)

// RetryClass classifies retry behavior without inspecting error text.
//
// The class, not the error string, is what the shell turns into a requeue
// decision. This keeps backoff policy in one place and keeps error messages
// free to change without altering scheduling behavior.
type RetryClass string

const (
	// RetryNone requests no further visit beyond the normal resync.
	RetryNone RetryClass = "None"
	// RetryOnChange requests no timed visit; a watch event will drive the
	// next pass.
	RetryOnChange RetryClass = "OnChange"
	// RetryConflict marks a lost optimistic-concurrency race, which should be
	// retried promptly against freshly read state.
	RetryConflict RetryClass = "Conflict"
	// RetryTransient marks a temporary external failure that should be retried
	// with backoff.
	RetryTransient RetryClass = "Transient"
	// RetryInProgress marks a long-running operation that should be polled
	// until it finishes.
	RetryInProgress RetryClass = "InProgress"
)

// RetryDirective describes when a result should be retried. After is a hint
// that the shell may clamp or jitter; a zero After means the shell picks the
// interval for the class.
type RetryDirective struct {
	Class RetryClass
	After time.Duration
}

// Result is the classified result of one reconciliation pass.
//
// Observed is the resource-specific observation payload carried by the status
// intent; see StatusIntent for why it is a type parameter.
type Result[Observed any] struct {
	// Outcome is the mutually exclusive classification of the pass.
	Outcome Outcome
	// Retry tells the shell whether and when to reconcile again.
	Retry RetryDirective
	// Reason is a stable, low-cardinality explanation for the result. It is
	// data for the shell, not a log message: it must satisfy ValidateReason so
	// it can be used as a metric label and an Event reason.
	Reason string
	// Status is the merged status intent applied during this pass, when one
	// exists.
	Status *StatusIntent[Observed]
}
