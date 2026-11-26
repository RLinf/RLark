package controller

import (
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

// This file is the only place a coordination Lease is projected onto the shared
// heartbeat primitives. The Node and Cluster controllers read Leases from
// different namespaces under different naming schemes, but they must not draw
// different conclusions from the same observation, so the projection and the
// status/requeue plumbing around it live here rather than once per controller.

// HeartbeatLeaseFrom projects a Lease onto the client-free input the shared
// classifier takes. A nil Lease means the object was not found, which is an
// explicit de-registration signal rather than an error.
func HeartbeatLeaseFrom(lease *coordinationv1.Lease) sharedreconcile.HeartbeatLease {
	if lease == nil {
		return sharedreconcile.HeartbeatLease{}
	}

	observed := sharedreconcile.HeartbeatLease{Exists: true}
	if lease.Spec.RenewTime != nil {
		renewTime := lease.Spec.RenewTime.Time
		observed.RenewTime = &renewTime
	}
	// An unset leaseDurationSeconds is an optional field on a built-in type,
	// not invalid operator input: the timeout simply falls back to the
	// configured or shared default.
	if lease.Spec.LeaseDurationSeconds != nil {
		observed.Duration = time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second
	}
	return observed
}

// ObserveLeaseHeartbeat classifies lease against now, carrying forward the
// renewal already recorded in the object's status so that a Lease whose
// renewTime moves backwards does not flap the object offline.
func ObserveLeaseHeartbeat(
	lease *coordinationv1.Lease,
	recorded *metav1.Time,
	now time.Time,
	timeouts sharedreconcile.HeartbeatTimeouts,
) sharedreconcile.HeartbeatObservation {
	return sharedreconcile.ObserveHeartbeat(HeartbeatLeaseFrom(lease), instantOrNil(recorded), now, timeouts)
}

// HeartbeatTime projects the newest renewal back onto a status timestamp.
func HeartbeatTime(observation sharedreconcile.HeartbeatObservation) *metav1.Time {
	if observation.LastRenewal == nil {
		return nil
	}
	value := metav1.NewTime(*observation.LastRenewal)
	return &value
}

// NextHeartbeatCheck returns how long to wait before the liveness verdict is
// worth recomputing.
//
// Expiry emits no event, so the deadline can only be noticed by looking again.
// The returned interval is always positive, so an agent that has already
// expired - or that never registered - is still re-examined instead of waiting
// for an event that will never arrive. The shell jitters the interval, so
// objects sharing a lease duration do not re-check in lockstep.
func NextHeartbeatCheck(now time.Time, observation sharedreconcile.HeartbeatObservation) time.Duration {
	if observation.LastRenewal == nil {
		return observation.Timeout
	}
	remaining := observation.LastRenewal.Add(observation.Timeout).Sub(now)
	if remaining <= 0 {
		return observation.Timeout
	}
	return remaining
}

func instantOrNil(value *metav1.Time) *time.Time {
	if value == nil {
		return nil
	}
	instant := value.Time
	return &instant
}
