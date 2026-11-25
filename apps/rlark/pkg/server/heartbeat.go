package server

import (
	"context"
	stderrors "errors"
	"fmt"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

const (
	// heartbeatLeaseManagedByLabel marks heartbeat Leases as owned by the
	// RLark Server so that a stale or foreign object is never silently adopted.
	heartbeatLeaseManagedByLabel = "app.kubernetes.io/managed-by"
	heartbeatLeaseManagedByValue = "rlark-server"
	// heartbeatLeaseWriteAttempts bounds create/update retries on optimistic
	// concurrency conflicts so that a concurrent Server replica can converge.
	heartbeatLeaseWriteAttempts = 4
)

// errHeartbeatLeaseOwnership reports a Lease that cannot be proven to be an
// RLark heartbeat record. It is permanent for the affected connection: the
// renewer reports it instead of overwriting an object it does not own.
var errHeartbeatLeaseOwnership = stderrors.New("heartbeat Lease ownership conflict")

// heartbeatLeaseRenewer maintains a heartbeat Lease as an idempotent liveness
// record rather than a leader-election lock. The Server is the only writer:
// renewals are last-writer-wins, so a reconnecting Agent is observed online as
// soon as its first renewal succeeds instead of waiting for a previous lease
// holder's full lease duration to expire.
type heartbeatLeaseRenewer struct {
	client        kubernetes.Interface
	namespace     string
	name          string
	identity      string
	leaseDuration time.Duration
	retryPeriod   time.Duration
	renewDeadline time.Duration
	now           func() time.Time
}

// Run renews the Lease immediately and then once per retry period until the
// connection context is canceled. It intentionally does not delete or release
// the Lease on cancellation: deletion can race with a newer connection and make
// a healthy Agent appear offline, so expiry is left to the Node controller.
func (r *heartbeatLeaseRenewer) Run(ctx context.Context) {
	logger := log.FromContext(ctx).WithValues("namespace", r.namespace, "lease", r.name)
	if err := r.renewWithDeadline(ctx); err != nil && ctx.Err() == nil {
		logger.Error(err, "renew heartbeat Lease")
	}

	ticker := time.NewTicker(r.retryPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := r.renewWithDeadline(ctx); err != nil && ctx.Err() == nil {
				logger.Error(err, "renew heartbeat Lease")
			}
		}
	}
}

func (r *heartbeatLeaseRenewer) renewWithDeadline(ctx context.Context) error {
	if r.renewDeadline <= 0 {
		return r.renew(ctx)
	}
	renewCtx, cancel := context.WithTimeout(ctx, r.renewDeadline)
	defer cancel()
	return r.renew(renewCtx)
}

// renew writes the heartbeat, retrying after a fresh read when another Server
// replica changed the Lease concurrently. A stale object is never replayed.
func (r *heartbeatLeaseRenewer) renew(ctx context.Context) error {
	var lastErr error
	for attempt := 0; attempt < heartbeatLeaseWriteAttempts; attempt++ {
		err := r.renewOnce(ctx)
		if err == nil {
			return nil
		}
		if !apierrors.IsConflict(err) && !apierrors.IsAlreadyExists(err) {
			return err
		}
		lastErr = err
	}
	return fmt.Errorf("renew heartbeat Lease %s/%s: %w", r.namespace, r.name, lastErr)
}

func (r *heartbeatLeaseRenewer) renewOnce(ctx context.Context) error {
	leases := r.client.CoordinationV1().Leases(r.namespace)
	now := metav1.NewMicroTime(r.now().UTC())

	existing, err := leases.Get(ctx, r.name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      r.name,
				Namespace: r.namespace,
			},
		}
		setHeartbeatLeaseSpec(lease, r.identity, r.leaseDuration, now)
		lease.Labels = heartbeatLeaseLabels(nil)
		if _, err := leases.Create(ctx, lease, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("create heartbeat Lease: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("get heartbeat Lease: %w", err)
	}
	if err := validateHeartbeatLeaseOwnership(existing, r.name); err != nil {
		return err
	}

	updated := existing.DeepCopy()
	setHeartbeatLeaseSpec(updated, r.identity, r.leaseDuration, nextHeartbeatRenewTime(existing.Spec.RenewTime, now))
	updated.Labels = heartbeatLeaseLabels(updated.Labels)
	if _, err := leases.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("update heartbeat Lease: %w", err)
	}
	return nil
}

// validateHeartbeatLeaseOwnership proves that an existing Lease may be managed
// as an RLark heartbeat record. Name and namespace are deterministic and owned
// by the Server; the managed-by label is the explicit proof for objects written
// by this version. Leases written by the previous LeaderElection-based
// implementation predate the label and always carry a duration, so they are
// adopted as the unambiguous legacy form. Anything else is reported as a
// conflict instead of being taken over.
func validateHeartbeatLeaseOwnership(lease *coordinationv1.Lease, name string) error {
	managedBy, labeled := lease.Labels[heartbeatLeaseManagedByLabel]
	if labeled {
		if managedBy != heartbeatLeaseManagedByValue {
			return fmt.Errorf("%w: Lease %s/%s is managed by %q", errHeartbeatLeaseOwnership, lease.Namespace, name, managedBy)
		}
		return nil
	}
	if lease.Spec.LeaseDurationSeconds == nil {
		return fmt.Errorf(
			"%w: Lease %s/%s is neither labeled %q=%q nor a legacy heartbeat record",
			errHeartbeatLeaseOwnership, lease.Namespace, name, heartbeatLeaseManagedByLabel, heartbeatLeaseManagedByValue,
		)
	}
	return nil
}

func heartbeatLeaseLabels(labels map[string]string) map[string]string {
	merged := make(map[string]string, len(labels)+1)
	for key, value := range labels {
		merged[key] = value
	}
	merged[heartbeatLeaseManagedByLabel] = heartbeatLeaseManagedByValue
	return merged
}

func setHeartbeatLeaseSpec(lease *coordinationv1.Lease, identity string, duration time.Duration, renewTime metav1.MicroTime) {
	previousHolder := ""
	if lease.Spec.HolderIdentity != nil {
		previousHolder = *lease.Spec.HolderIdentity
	}
	holder := identity
	lease.Spec.HolderIdentity = &holder
	lease.Spec.LeaseDurationSeconds = ptr.To(int32(duration / time.Second))
	lease.Spec.RenewTime = &renewTime
	if previousHolder != identity || lease.Spec.AcquireTime == nil {
		acquireTime := renewTime
		lease.Spec.AcquireTime = &acquireTime
	}
}

// nextHeartbeatRenewTime keeps RenewTime monotonic so that a Server replica
// with a slightly lagging clock cannot move the heartbeat backwards.
func nextHeartbeatRenewTime(previous *metav1.MicroTime, now metav1.MicroTime) metav1.MicroTime {
	if previous == nil || now.After(previous.Time) {
		return now
	}
	return metav1.NewMicroTime(previous.Add(time.Microsecond))
}
