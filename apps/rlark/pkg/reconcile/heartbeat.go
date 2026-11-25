package reconcile

import "time"

// DefaultHeartbeatTimeout is the validity window applied to a renewal when
// neither the caller nor the Lease itself states one. It matches the Lease
// default used by Kubernetes components and is deliberately generous: a
// controller that under-estimates the window reports a live agent as offline.
const DefaultHeartbeatTimeout = time.Minute

// HeartbeatState is the liveness conclusion drawn from one heartbeat
// observation. It is a closed set and is safe to use as a metric label.
type HeartbeatState string

const (
	// HeartbeatMissing means no renewal has ever been observed: either the
	// Lease does not exist, or it exists but was never renewed. The two cases
	// are indistinguishable for liveness, so they are not separate states;
	// callers that must tell them apart should look at HeartbeatLease.Exists.
	HeartbeatMissing HeartbeatState = "Missing"
	// HeartbeatActive means the most recent renewal is still inside its
	// validity window.
	HeartbeatActive HeartbeatState = "Active"
	// HeartbeatExpired means a renewal was observed but has aged past its
	// validity window.
	HeartbeatExpired HeartbeatState = "Expired"
)

// HeartbeatLease is the subset of a coordination Lease that liveness
// classification depends on.
//
// It exists so that classification is a pure function of plain values: the
// Node and Cluster controllers read Leases from different namespaces under
// different naming schemes, but must not draw different conclusions from the
// same observation.
type HeartbeatLease struct {
	// Exists reports whether the Lease object was found at all. A zero
	// HeartbeatLease therefore describes an agent that has never registered.
	Exists bool
	// RenewTime is the Lease's spec.renewTime, or nil when the Lease has never
	// been renewed.
	RenewTime *time.Time
	// Duration is the Lease's spec.leaseDurationSeconds. Zero means the Lease
	// did not state one.
	Duration time.Duration
}

// HeartbeatTimeouts resolves how long a single renewal stays valid.
//
// Override exists because an operator-configured timeout must beat whatever
// the agent wrote into its own Lease; an agent that renews too slowly cannot
// be allowed to extend its own grace period indefinitely.
type HeartbeatTimeouts struct {
	// Override, when positive, is used regardless of what the Lease states.
	Override time.Duration
	// Default, when positive, is used if neither Override nor the Lease state
	// a duration. A zero Default falls back to DefaultHeartbeatTimeout.
	Default time.Duration
	// Grace, when positive, extends the resolved window. It exists for
	// deployments whose agents renew over a slow or lossy link, where a
	// renewal that arrives late is still proof of life; it is additive so the
	// configured allowance stays legible next to whatever duration the Lease
	// itself states.
	Grace time.Duration
}

// Resolve returns the validity window to apply to lease.
func (t HeartbeatTimeouts) Resolve(lease HeartbeatLease) time.Duration {
	return t.base(lease) + t.Grace
}

func (t HeartbeatTimeouts) base(lease HeartbeatLease) time.Duration {
	if t.Override > 0 {
		return t.Override
	}
	if lease.Duration > 0 {
		return lease.Duration
	}
	if t.Default > 0 {
		return t.Default
	}
	return DefaultHeartbeatTimeout
}

// HeartbeatObservation is the result of classifying one heartbeat read.
type HeartbeatObservation struct {
	// State is the liveness conclusion.
	State HeartbeatState
	// Timeout is the validity window that was applied. Callers use it as the
	// requeue deadline, because nothing can change the conclusion before the
	// current renewal can expire.
	Timeout time.Duration
	// LastRenewal is the newest renewal known so far, or nil if none has ever
	// been observed. It never moves backwards; see ObserveHeartbeat.
	LastRenewal *time.Time
	// ClockSkew reports that the observed renewal is dated after now, which
	// means the agent's clock runs ahead of the control plane's. The renewal
	// is still honoured — refusing it would take a reachable agent offline for
	// a reason it cannot detect — but the fact is surfaced so it can be logged
	// instead of silently widening the grace period.
	ClockSkew bool
}

// Expired reports whether the observation means the agent is not live.
func (o HeartbeatObservation) Expired() bool {
	return o.State != HeartbeatActive
}

// ObserveHeartbeat classifies lease against now, carrying forward the newest
// renewal already recorded in the object's status.
//
// previous is the last renewal the controller persisted (nil if none). It
// matters for two boundary cases that a naive implementation gets wrong:
//
//   - A Lease whose renewTime moves backwards. That happens when an agent is
//     recreated, when two agent replicas briefly both renew, or when an
//     agent's clock steps back. Classifying on the regressed value would flap
//     the object to Offline even though renewals are still arriving, so
//     ObserveHeartbeat classifies on the newest renewal it has ever seen.
//     This cannot keep a dead agent alive: once that newest renewal itself
//     ages past the timeout, the state becomes Expired.
//   - A Lease dated in the future. The renewal is accepted and ClockSkew is
//     set, rather than being treated as missing.
//
// A missing or never-renewed Lease yields HeartbeatMissing. Lease absence is
// an explicit de-registration signal and must not be masked by a renewal the
// controller recorded earlier; previous is still carried through unchanged, so
// the recorded timestamp survives for display and for the next comparison.
func ObserveHeartbeat(lease HeartbeatLease, previous *time.Time, now time.Time, timeouts HeartbeatTimeouts) HeartbeatObservation {
	observation := HeartbeatObservation{
		Timeout:     timeouts.Resolve(lease),
		LastRenewal: previous,
	}
	if !lease.Exists || lease.RenewTime == nil {
		observation.State = HeartbeatMissing
		return observation
	}

	renewal := *lease.RenewTime
	observation.ClockSkew = renewal.After(now)
	if previous != nil && previous.After(renewal) {
		renewal = *previous
	}
	newest := renewal
	observation.LastRenewal = &newest
	observation.State = stateFor(newest, now, observation.Timeout)
	return observation
}

func stateFor(renewal, now time.Time, timeout time.Duration) HeartbeatState {
	if renewal.Add(timeout).Before(now) {
		return HeartbeatExpired
	}
	return HeartbeatActive
}
