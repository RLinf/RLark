package reconcile

import (
	"testing"
	"time"
)

// A zero Backoff must still produce usable intervals, because a controller that
// forgot to configure one would otherwise hot-loop on every retry class.
func TestZeroBackoffFallsBackToDefaults(t *testing.T) {
	var backoff Backoff
	backoff.Rand = func() float64 { return 0 }

	cases := []struct {
		class RetryClass
		want  time.Duration
	}{
		{RetryConflict, DefaultConflictBackoff},
		{RetryInProgress, DefaultInProgressBackoff},
		{RetryTransient, DefaultTransientBackoff},
	}
	for _, tc := range cases {
		if got := backoff.Interval(RetryDirective{Class: tc.class}); got != tc.want {
			t.Errorf("Interval(%s) = %v, want %v", tc.class, got, tc.want)
		}
	}
}

// RetryNone and RetryOnChange mean "do not poll, wait for a watch event". A
// non-zero interval here would silently reintroduce polling everywhere.
func TestIntervalIsZeroWhenNoPollingIsRequested(t *testing.T) {
	backoff := DefaultBackoff()
	for _, class := range []RetryClass{RetryNone, RetryOnChange} {
		if got := backoff.Interval(RetryDirective{Class: class}); got != 0 {
			t.Errorf("Interval(%s) = %v, want 0", class, got)
		}
	}
}

// An explicit After carries information the shared package cannot derive, such
// as the remaining lease or archive TTL, so it must win over the class default.
func TestExplicitAfterOverridesClassDefault(t *testing.T) {
	backoff := DefaultBackoff()
	backoff.Rand = func() float64 { return 0 }

	got := backoff.Interval(RetryDirective{Class: RetryTransient, After: 90 * time.Second})
	if got != 90*time.Second {
		t.Fatalf("Interval = %v, want 90s", got)
	}
}

// Even RetryNone must honour an explicit deadline, otherwise deadline-driven
// controllers would have to bypass the shared helper entirely.
func TestExplicitAfterAppliesToNonPollingClasses(t *testing.T) {
	backoff := DefaultBackoff()
	backoff.Rand = func() float64 { return 0 }

	got := backoff.Interval(RetryDirective{Class: RetryOnChange, After: 30 * time.Second})
	if got != 30*time.Second {
		t.Fatalf("Interval = %v, want 30s", got)
	}
}

// Jitter must only ever delay. Waking up early is never useful for a deadline
// driven requeue, and it is what makes many controllers re-run for nothing.
func TestJitterOnlyAddsDelay(t *testing.T) {
	backoff := DefaultBackoff()
	base := 10 * time.Second

	for _, draw := range []float64{0, 0.5, 1} {
		backoff.Rand = func() float64 { return draw }
		got := backoff.Interval(RetryDirective{Class: RetryTransient, After: base})
		if got < base {
			t.Errorf("draw %v: Interval = %v, want >= %v", draw, got, base)
		}
		if max := base + time.Duration(float64(base)*DefaultJitterFraction); got > max {
			t.Errorf("draw %v: Interval = %v, want <= %v", draw, got, max)
		}
	}
}

// A misconfigured fraction above 1 would let a single pass stall for twice the
// intended interval or longer, so it is clamped instead of trusted.
func TestJitterFractionIsClamped(t *testing.T) {
	backoff := DefaultBackoff()
	backoff.JitterFraction = 5
	backoff.Rand = func() float64 { return 1 }

	base := time.Second
	got := backoff.Interval(RetryDirective{Class: RetryTransient, After: base})
	if got != 2*base {
		t.Fatalf("Interval = %v, want %v", got, 2*base)
	}
}

// Disabling jitter must be possible, so that tests and deterministic flows can
// assert exact intervals.
func TestZeroJitterFractionDisablesJitter(t *testing.T) {
	backoff := DefaultBackoff()
	backoff.JitterFraction = 0

	got := backoff.Interval(RetryDirective{Class: RetryInProgress})
	if got != DefaultInProgressBackoff {
		t.Fatalf("Interval = %v, want %v", got, DefaultInProgressBackoff)
	}
}

// An unknown class must not be guessed at. Returning zero keeps the pass on the
// watch-driven path instead of inventing a polling interval.
func TestUnknownRetryClassDoesNotPoll(t *testing.T) {
	backoff := DefaultBackoff()
	if got := backoff.Interval(RetryDirective{Class: RetryClass("Bogus")}); got != 0 {
		t.Fatalf("Interval = %v, want 0", got)
	}
}
