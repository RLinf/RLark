package controller

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// RebindTargetReader resolves a target Node from the registered member API using an uncached read.
type RebindTargetReader interface {
	VerifyNode(context.Context, string, string, string) (nodeUID, systemUUID string, exists bool, err error)
}

// RebindAuditSink must durably deduplicate by OperationUID before returning success.
type RebindAuditSink interface {
	Record(context.Context, RebindAuditEvent) error
}

type RebindAuditEvent struct {
	Namespace          string
	OperationUID       string
	OperationName      string
	ClusterID          string
	StationUID         string
	StationName        string
	ObservedStationUID string
	Outcome            string
	ResultReason       string
	Actor              string
	OldNodeUID         string
	NewNodeUID         string
	OldVersion         int64
	NewVersion         int64
	Reason             string
	OccurredAt         time.Time
}

type RebindReconciler struct {
	client        client.Client
	reader        client.Reader
	registrations ClusterResolver
	identityOps   IdentityOperationCoordinator
	targets       RebindTargetReader
	audit         RebindAuditSink
	now           func() time.Time
	retryAfter    time.Duration
}

func NewRebindReconciler(writer client.Client, reader client.Reader, registrations ClusterResolver, identityOps IdentityOperationCoordinator, targets RebindTargetReader, audit RebindAuditSink, now func() time.Time) *RebindReconciler {
	return &RebindReconciler{client: writer, reader: reader, registrations: registrations, identityOps: identityOps, targets: targets, audit: audit, now: now, retryAfter: 5 * time.Second}
}

func (r *RebindReconciler) SetupWithManager(mgr manager.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).For(&stationv1alpha1.StationRebind{}).Complete(r)
}

var _ reconcile.Reconciler = (*RebindReconciler)(nil)

func (r *RebindReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	release, err := r.identityOps.Acquire(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("acquire Station identity operation lock: %w", err)
	}
	defer release()

	registration, err := r.registrations.ResolveNamespace(ctx, req.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	key := req.NamespacedName
	operation := &stationv1alpha1.StationRebind{}
	if err := r.reader.Get(ctx, key, operation); apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("read StationRebind: %w", err)
	}
	if operation.Status.Phase == stationv1alpha1.StationRebindPhaseApplied || operation.Status.Phase == stationv1alpha1.StationRebindPhaseRejected {
		return reconcile.Result{}, nil
	}

	stationKey := types.NamespacedName{Namespace: operation.Namespace, Name: operation.Spec.StationRef.Name}
	station := &stationv1alpha1.Station{}
	if err := r.reader.Get(ctx, stationKey, station); apierrors.IsNotFound(err) {
		return r.reject(ctx, operation, registration.ClusterID, "StationNotFound", "referenced Station does not exist")
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("read referenced Station: %w", err)
	}
	if string(station.UID) != operation.Spec.StationRef.UID {
		return r.reject(ctx, operation, registration.ClusterID, "StationUIDChanged", "referenced Station object UID does not match")
	}
	if station.Spec.Retired {
		return r.reject(ctx, operation, registration.ClusterID, "StationRetired", "retired Stations cannot be rebound")
	}

	verifiedUID, systemUUID, exists, err := r.targets.VerifyNode(ctx, registration.ClusterID, registration.Namespace, operation.Spec.TargetNodeName)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("verify target Node: %w", err)
	}
	if !exists || verifiedUID != operation.Spec.TargetNodeUID {
		return r.reject(ctx, operation, registration.ClusterID, "TargetNodeIdentityMismatch", "target Node is absent or its UID does not match the request")
	}
	if operation.Spec.TargetSystemUUID != "" && operation.Spec.TargetSystemUUID != systemUUID {
		return r.reject(ctx, operation, registration.ClusterID, "TargetEvidenceMismatch", "target system UUID does not match current server-side evidence")
	}
	if station.Spec.Binding.NodeUID == verifiedUID && operation.Status.PreviousBinding == nil {
		return r.reject(ctx, operation, registration.ClusterID, "TargetAlreadyBound", "target Node is already the current Station binding")
	}
	var stations stationv1alpha1.StationList
	if err := r.reader.List(ctx, &stations, client.InNamespace(registration.Namespace)); err != nil {
		return reconcile.Result{}, fmt.Errorf("list Stations before rebind: %w", err)
	}
	for i := range stations.Items {
		other := &stations.Items[i]
		if other.UID != station.UID && other.Spec.Binding.NodeUID == verifiedUID {
			return r.reject(ctx, operation, registration.ClusterID, "TargetNodeAlreadyBound", "target Node UID is already owned by another Station")
		}
	}

	if operation.Status.PreviousBinding == nil {
		if station.Spec.Binding.Version != operation.Spec.ExpectedBindingVersion {
			return r.reject(ctx, operation, registration.ClusterID, "BindingVersionConflict", "Station binding version does not match the request")
		}
		if err := r.savePreviousBinding(ctx, operation, station.Spec.Binding); err != nil {
			return reconcile.Result{}, err
		}
		if err := r.reader.Get(ctx, key, operation); err != nil {
			return reconcile.Result{}, fmt.Errorf("reread Rebind after recording prior binding: %w", err)
		}
	}
	oldBinding := *operation.Status.PreviousBinding
	newVersion := operation.Spec.ExpectedBindingVersion + 1
	if station.Spec.Binding.Version == operation.Spec.ExpectedBindingVersion {
		if err := r.applyBinding(ctx, station, operation, systemUUID); err != nil {
			if errors.Is(err, errRebindStale) {
				latest := &stationv1alpha1.Station{}
				if readErr := r.reader.Get(ctx, stationKey, latest); readErr != nil {
					return reconcile.Result{}, fmt.Errorf("read Station after concurrent rebind: %w", readErr)
				}
				if latest.UID != station.UID || latest.Spec.Retired || latest.Spec.Binding.Version != newVersion || latest.Spec.Binding.NodeName != operation.Spec.TargetNodeName || latest.Spec.Binding.NodeUID != verifiedUID {
					return r.reject(ctx, operation, registration.ClusterID, "BindingVersionConflict", "Station binding version changed before the operation was applied")
				}
				station = latest
			} else {
				return reconcile.Result{}, err
			}
		}
	} else if station.Spec.Binding.Version == newVersion && station.Spec.Binding.NodeName == operation.Spec.TargetNodeName && station.Spec.Binding.NodeUID == verifiedUID {
		// A previous attempt may have committed the Station patch before failing to record audit.
	} else {
		return r.reject(ctx, operation, registration.ClusterID, "BindingVersionConflict", "Station binding version no longer matches the request")
	}

	event := RebindAuditEvent{
		Namespace: operation.Namespace, OperationUID: string(operation.UID), OperationName: operation.Name, ClusterID: registration.ClusterID,
		StationUID: string(station.UID), StationName: station.Name,
		Outcome: "Applied", ResultReason: "RebindApplied", Actor: operation.Spec.RequestedBy,
		OldNodeUID: oldBinding.NodeUID, NewNodeUID: verifiedUID,
		OldVersion: operation.Spec.ExpectedBindingVersion, NewVersion: newVersion,
		Reason:     operation.Spec.Reason,
		OccurredAt: r.now().UTC(),
	}
	if err := r.audit.Record(ctx, event); err != nil {
		if statusErr := r.setOperationCondition(ctx, operation, "AuditRecorded", metav1.ConditionFalse, "AuditStoreUnavailable", "binding applied; durable audit event is pending"); statusErr != nil {
			return reconcile.Result{}, errors.Join(fmt.Errorf("record rebind audit: %w", err), statusErr)
		}
		return reconcile.Result{RequeueAfter: r.retryAfter}, nil
	}
	if err := r.finish(ctx, operation, stationv1alpha1.StationRebindPhaseApplied, "RebindApplied", "binding and durable audit event were recorded"); err != nil {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

var errRebindStale = errors.New("rebind target changed during compare-and-swap")

func (r *RebindReconciler) applyBinding(ctx context.Context, original *stationv1alpha1.Station, operation *stationv1alpha1.StationRebind, systemUUID string) error {
	key := types.NamespacedName{Namespace: original.Namespace, Name: original.Name}
	for attempt := 0; attempt < sequenceCASAttempts; attempt++ {
		current := &stationv1alpha1.Station{}
		if err := r.reader.Get(ctx, key, current); err != nil {
			return fmt.Errorf("reread Station before rebind: %w", err)
		}
		if current.UID != original.UID || current.Spec.Retired || current.Spec.Binding.Version != operation.Spec.ExpectedBindingVersion {
			return errRebindStale
		}
		base := current.DeepCopy()
		current.Spec.Binding.NodeName = operation.Spec.TargetNodeName
		current.Spec.Binding.NodeUID = operation.Spec.TargetNodeUID
		current.Spec.Binding.SystemUUID = systemUUID
		current.Spec.Binding.Version = operation.Spec.ExpectedBindingVersion + 1
		current.Spec.Binding.BoundBy = normalizedRequester(operation.Spec.RequestedBy)
		if err := r.client.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); apierrors.IsConflict(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("patch Station binding: %w", err)
		}
		return nil
	}
	return errors.New("Station binding remained contended after bounded retries")
}

func (r *RebindReconciler) reject(ctx context.Context, operation *stationv1alpha1.StationRebind, clusterID, reason, message string) (reconcile.Result, error) {
	actor := operation.Spec.RequestedBy
	if actor == "" {
		actor = "unknown"
	}
	stationUID := operation.Spec.StationRef.UID
	observedStationUID := ""
	oldNodeUID := ""
	oldVersion := operation.Spec.ExpectedBindingVersion
	station := &stationv1alpha1.Station{}
	if err := r.reader.Get(ctx, types.NamespacedName{Namespace: operation.Namespace, Name: operation.Spec.StationRef.Name}, station); err == nil {
		observedStationUID = string(station.UID)
		oldNodeUID = station.Spec.Binding.NodeUID
		oldVersion = station.Spec.Binding.Version
	} else if !apierrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("read Station for rejection audit: %w", err)
	}
	if err := r.audit.Record(ctx, RebindAuditEvent{
		Namespace: operation.Namespace, OperationUID: string(operation.UID), OperationName: operation.Name, ClusterID: clusterID,
		StationUID: stationUID, StationName: operation.Spec.StationRef.Name,
		ObservedStationUID: observedStationUID,
		Outcome:            "Rejected", ResultReason: reason,
		Actor:      actor,
		OldNodeUID: oldNodeUID,
		NewNodeUID: operation.Spec.TargetNodeUID,
		OldVersion: oldVersion,
		Reason:     operation.Spec.Reason,
		OccurredAt: r.now().UTC(),
	}); err != nil {
		if statusErr := r.setOperationCondition(ctx, operation, "AuditRecorded", metav1.ConditionFalse, "AuditStoreUnavailable", "rejection is pending durable audit"); statusErr != nil {
			return reconcile.Result{}, errors.Join(fmt.Errorf("record rejected rebind audit: %w", err), statusErr)
		}
		return reconcile.Result{RequeueAfter: r.retryAfter}, nil
	}
	return reconcile.Result{}, r.finish(ctx, operation, stationv1alpha1.StationRebindPhaseRejected, reason, message)
}

func (r *RebindReconciler) savePreviousBinding(ctx context.Context, operation *stationv1alpha1.StationRebind, binding stationv1alpha1.StationBinding) error {
	for attempt := 0; attempt < sequenceCASAttempts; attempt++ {
		current := &stationv1alpha1.StationRebind{}
		key := types.NamespacedName{Namespace: operation.Namespace, Name: operation.Name}
		if err := r.reader.Get(ctx, key, current); err != nil {
			return fmt.Errorf("reread Rebind before storing prior binding: %w", err)
		}
		if current.Status.PreviousBinding != nil {
			if !reflect.DeepEqual(*current.Status.PreviousBinding, binding) {
				return errors.New("Rebind prior binding snapshot conflicts with current Station")
			}
			return nil
		}
		base := current.DeepCopy()
		previous := binding
		current.Status.PreviousBinding = &previous
		current.Status.ObservedGeneration = current.Generation
		if err := r.client.Status().Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); apierrors.IsConflict(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("persist prior Station binding: %w", err)
		}
		return nil
	}
	return errors.New("Rebind prior binding snapshot remained contended after bounded retries")
}

func (r *RebindReconciler) finish(ctx context.Context, operation *stationv1alpha1.StationRebind, phase stationv1alpha1.StationRebindPhase, reason, message string) error {
	for attempt := 0; attempt < sequenceCASAttempts; attempt++ {
		current := &stationv1alpha1.StationRebind{}
		key := types.NamespacedName{Namespace: operation.Namespace, Name: operation.Name}
		if err := r.reader.Get(ctx, key, current); err != nil {
			return fmt.Errorf("reread Rebind before terminal status: %w", err)
		}
		if current.Status.Phase == phase {
			return nil
		}
		base := current.DeepCopy()
		current.Status.Phase = phase
		current.Status.ObservedGeneration = current.Generation
		now := metav1.NewTime(r.now().UTC())
		current.Status.CompletedAt = &now
		apimeta.SetStatusCondition(&current.Status.Conditions, metav1.Condition{
			Type: "AuditRecorded", Status: metav1.ConditionTrue, Reason: "Recorded", Message: "an idempotent audit event is durably recorded", LastTransitionTime: now,
		})
		apimeta.SetStatusCondition(&current.Status.Conditions, metav1.Condition{
			Type: "Completed", Status: metav1.ConditionTrue, Reason: reason, Message: message, LastTransitionTime: now, ObservedGeneration: current.Generation,
		})
		if err := r.client.Status().Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); apierrors.IsConflict(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("patch terminal Rebind status: %w", err)
		}
		return nil
	}
	return errors.New("Rebind status remained contended after bounded retries")
}

func (r *RebindReconciler) setOperationCondition(ctx context.Context, operation *stationv1alpha1.StationRebind, conditionType string, status metav1.ConditionStatus, reason, message string) error {
	for attempt := 0; attempt < sequenceCASAttempts; attempt++ {
		current := &stationv1alpha1.StationRebind{}
		key := types.NamespacedName{Namespace: operation.Namespace, Name: operation.Name}
		if err := r.reader.Get(ctx, key, current); err != nil {
			return fmt.Errorf("reread Rebind before condition update: %w", err)
		}
		base := current.DeepCopy()
		now := metav1.NewTime(r.now().UTC())
		apimeta.SetStatusCondition(&current.Status.Conditions, metav1.Condition{
			Type: conditionType, Status: status, Reason: reason, Message: message, LastTransitionTime: now,
		})
		if reflect.DeepEqual(base.Status, current.Status) {
			return nil
		}
		if err := r.client.Status().Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{})); apierrors.IsConflict(err) {
			continue
		} else if err != nil {
			return fmt.Errorf("patch Rebind condition: %w", err)
		}
		return nil
	}
	return errors.New("Rebind condition remained contended after bounded retries")
}

func normalizedRequester(requestedBy string) string {
	if requestedBy == SelfHealRequester {
		return "system:" + requestedBy
	}
	return "admin:" + strings.TrimPrefix(requestedBy, "admin:")
}
