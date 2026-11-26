package controller

import (
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

func TestHeartbeatLeaseFromTreatsAMissingLeaseAsDeregistration(t *testing.T) {
	observed := HeartbeatLeaseFrom(nil)
	if observed.Exists || observed.RenewTime != nil || observed.Duration != 0 {
		t.Fatalf("a nil Lease must project onto the zero value, got %#v", observed)
	}
}

func TestHeartbeatLeaseFromToleratesAnUnsetDuration(t *testing.T) {
	renewTime := metav1.NewMicroTime(time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC))
	lease := &coordinationv1.Lease{Spec: coordinationv1.LeaseSpec{RenewTime: &renewTime}}

	observed := HeartbeatLeaseFrom(lease)
	if !observed.Exists {
		t.Fatalf("expected the Lease to be reported as existing, got %#v", observed)
	}
	if observed.RenewTime == nil || !observed.RenewTime.Equal(renewTime.Time) {
		t.Fatalf("expected renewal %s, got %#v", renewTime.Time, observed.RenewTime)
	}
	// An unset leaseDurationSeconds is an optional field on a built-in type,
	// not invalid operator input: the timeout falls back to the default rather
	// than failing the pass.
	if observed.Duration != 0 {
		t.Fatalf("expected no duration, got %s", observed.Duration)
	}
}

func TestHeartbeatLeaseFromConvertsSecondsToADuration(t *testing.T) {
	duration := int32(30)
	observed := HeartbeatLeaseFrom(&coordinationv1.Lease{
		Spec: coordinationv1.LeaseSpec{LeaseDurationSeconds: &duration},
	})
	if observed.Duration != 30*time.Second {
		t.Fatalf("expected 30s, got %s", observed.Duration)
	}
}

func TestObserveLeaseHeartbeatCarriesTheRecordedRenewalForward(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	recorded := metav1.NewTime(now.Add(-5 * time.Second))

	// The Lease is gone but the status already recorded a renewal: the agent
	// must not be declared live off a stale record, yet the record itself has
	// to survive so the next pass can still tell when it expires.
	observation := ObserveLeaseHeartbeat(nil, &recorded, now, sharedreconcile.HeartbeatTimeouts{})
	if observation.State != sharedreconcile.HeartbeatMissing {
		t.Fatalf("expected a missing heartbeat, got %q", observation.State)
	}
	if observation.LastRenewal == nil || !observation.LastRenewal.Equal(recorded.Time) {
		t.Fatalf("expected the recorded renewal to survive, got %#v", observation.LastRenewal)
	}
}

func TestObserveLeaseHeartbeatHonoursAnOperatorOverride(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	renewTime := metav1.NewMicroTime(now.Add(-45 * time.Second))
	duration := int32(30)
	lease := &coordinationv1.Lease{Spec: coordinationv1.LeaseSpec{
		RenewTime:            &renewTime,
		LeaseDurationSeconds: &duration,
	}}

	// The Lease says the renewal expired 15s ago; the operator override says
	// the window is two minutes, so the agent is still live.
	observation := ObserveLeaseHeartbeat(lease, nil, now, sharedreconcile.HeartbeatTimeouts{Override: 2 * time.Minute})
	if observation.State != sharedreconcile.HeartbeatActive {
		t.Fatalf("expected the override to win, got %q", observation.State)
	}
	if observation.Timeout != 2*time.Minute {
		t.Fatalf("expected a 2m timeout, got %s", observation.Timeout)
	}
}

func TestHeartbeatTimeProjectsOnlyARecordedRenewal(t *testing.T) {
	if got := HeartbeatTime(sharedreconcile.HeartbeatObservation{}); got != nil {
		t.Fatalf("expected no timestamp without a renewal, got %#v", got)
	}

	renewal := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	got := HeartbeatTime(sharedreconcile.HeartbeatObservation{LastRenewal: &renewal})
	if got == nil || !got.Time.Equal(renewal) {
		t.Fatalf("expected %s, got %#v", renewal, got)
	}
}

func TestNextHeartbeatCheckIsAlwaysPositive(t *testing.T) {
	now := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	renewal := now.Add(-5 * time.Second)

	if got := NextHeartbeatCheck(now, sharedreconcile.HeartbeatObservation{
		LastRenewal: &renewal,
		Timeout:     30 * time.Second,
	}); got != 25*time.Second {
		t.Fatalf("expected the remaining window of 25s, got %s", got)
	}

	// An already expired heartbeat, and one that was never recorded, both have
	// to be re-examined: expiry emits no event, so waiting for one would leave
	// the status frozen.
	expired := now.Add(-time.Hour)
	if got := NextHeartbeatCheck(now, sharedreconcile.HeartbeatObservation{
		LastRenewal: &expired,
		Timeout:     30 * time.Second,
	}); got != 30*time.Second {
		t.Fatalf("expected the full timeout for an expired heartbeat, got %s", got)
	}
	if got := NextHeartbeatCheck(now, sharedreconcile.HeartbeatObservation{Timeout: time.Minute}); got != time.Minute {
		t.Fatalf("expected the full timeout with no recorded heartbeat, got %s", got)
	}
}
