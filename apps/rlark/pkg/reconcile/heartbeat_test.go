package reconcile

import (
	"testing"
	"time"
)

func TestHeartbeatTimeoutsResolvePrefersOverrideThenLeaseThenDefault(t *testing.T) {
	lease := HeartbeatLease{Exists: true, Duration: 30 * time.Second}

	if got := (HeartbeatTimeouts{Override: 5 * time.Second, Default: time.Hour}).Resolve(lease); got != 5*time.Second {
		t.Fatalf("override should win, got %s", got)
	}
	if got := (HeartbeatTimeouts{Default: time.Hour}).Resolve(lease); got != 30*time.Second {
		t.Fatalf("lease duration should win over the default, got %s", got)
	}
	if got := (HeartbeatTimeouts{Default: time.Hour}).Resolve(HeartbeatLease{Exists: true}); got != time.Hour {
		t.Fatalf("default should apply when the Lease carries no duration, got %s", got)
	}
	if got := (HeartbeatTimeouts{}).Resolve(HeartbeatLease{}); got != DefaultHeartbeatTimeout {
		t.Fatalf("package default should be the last resort, got %s", got)
	}
}

func TestObserveHeartbeatTreatsAbsentLeaseAsMissing(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	previous := now.Add(-10 * time.Second)

	observation := ObserveHeartbeat(HeartbeatLease{}, &previous, now, HeartbeatTimeouts{Default: time.Minute})

	if observation.State != HeartbeatMissing {
		t.Fatalf("expected Missing, got %s", observation.State)
	}
	if !observation.Expired() {
		t.Fatal("a missing heartbeat must count as expired")
	}
	// A recorded heartbeat must not mask the absence of the Lease, but it must
	// also not be erased: operators still need to see when the agent was last
	// seen.
	if observation.LastRenewal == nil || !observation.LastRenewal.Equal(previous) {
		t.Fatalf("previous renewal should be passed through, got %v", observation.LastRenewal)
	}
}

func TestObserveHeartbeatTreatsNeverRenewedLeaseAsMissing(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	observation := ObserveHeartbeat(HeartbeatLease{Exists: true, Duration: time.Minute}, nil, now, HeartbeatTimeouts{})

	if observation.State != HeartbeatMissing {
		t.Fatalf("expected Missing, got %s", observation.State)
	}
	if observation.Timeout != time.Minute {
		t.Fatalf("timeout should still resolve from the Lease, got %s", observation.Timeout)
	}
}

func TestObserveHeartbeatClassifiesActiveAndExpired(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	timeouts := HeartbeatTimeouts{Default: time.Minute}

	fresh := now.Add(-30 * time.Second)
	if got := ObserveHeartbeat(HeartbeatLease{Exists: true, RenewTime: &fresh}, nil, now, timeouts); got.State != HeartbeatActive {
		t.Fatalf("expected Active, got %s", got.State)
	}

	// Exactly at the deadline the heartbeat is still honoured; expiry requires
	// the deadline to be in the past, so a renewal racing the check is not
	// reported as a flap.
	edge := now.Add(-time.Minute)
	if got := ObserveHeartbeat(HeartbeatLease{Exists: true, RenewTime: &edge}, nil, now, timeouts); got.State != HeartbeatActive {
		t.Fatalf("expected the deadline itself to be Active, got %s", got.State)
	}

	stale := now.Add(-2 * time.Minute)
	if got := ObserveHeartbeat(HeartbeatLease{Exists: true, RenewTime: &stale}, nil, now, timeouts); got.State != HeartbeatExpired {
		t.Fatalf("expected Expired, got %s", got.State)
	}
}

func TestObserveHeartbeatAdvancesMonotonicallyWhenTheLeaseGoesBackwards(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	previous := now.Add(-10 * time.Second)
	rewound := now.Add(-5 * time.Minute)

	observation := ObserveHeartbeat(
		HeartbeatLease{Exists: true, RenewTime: &rewound},
		&previous,
		now,
		HeartbeatTimeouts{Default: time.Minute},
	)

	// A Lease rewritten with an older RenewTime (a restarted or re-created
	// writer) must not retroactively mark a live agent offline.
	if observation.State != HeartbeatActive {
		t.Fatalf("expected Active from the monotonic value, got %s", observation.State)
	}
	if observation.LastRenewal == nil || !observation.LastRenewal.Equal(previous) {
		t.Fatalf("expected the newest of the two renewals, got %v", observation.LastRenewal)
	}

	// The monotonic value is itself subject to the timeout, so a dead agent
	// still expires rather than being pinned online forever.
	later := now.Add(10 * time.Minute)
	expired := ObserveHeartbeat(
		HeartbeatLease{Exists: true, RenewTime: &rewound},
		&previous,
		later,
		HeartbeatTimeouts{Default: time.Minute},
	)
	if expired.State != HeartbeatExpired {
		t.Fatalf("expected the monotonic value to expire, got %s", expired.State)
	}
}

func TestObserveHeartbeatFlagsClockSkewWithoutRejectingTheRenewal(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	future := now.Add(30 * time.Second)

	observation := ObserveHeartbeat(
		HeartbeatLease{Exists: true, RenewTime: &future},
		nil,
		now,
		HeartbeatTimeouts{Default: time.Minute},
	)

	if !observation.ClockSkew {
		t.Fatal("a renewal in the future must be reported as clock skew")
	}
	// Skew is an observation, not a rejection: dropping the renewal would mark
	// a healthy agent offline because of a clock we do not control.
	if observation.State != HeartbeatActive {
		t.Fatalf("expected Active despite skew, got %s", observation.State)
	}
	if observation.Expired() {
		t.Fatal("an active heartbeat must not report as expired")
	}
}
