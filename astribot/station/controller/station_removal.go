package controller

import (
	"context"
	"fmt"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// A retired Station is the record of a node that was removed, or of the earlier
// node of a name a machine joined again under. It stays until an administrator
// asks for it to go: the Gateway writes who asked on the Station
// (StationDeleteRequestedAnnotation), and this reconciler, which reaches the
// member cluster, deletes it or says on it why it does not
// (StationDeleteRefusedAnnotation).
const (
	StationDeleteRequestedAnnotation = "station.rlark.io/delete-requested-by"
	StationDeleteRefusedAnnotation   = "station.rlark.io/delete-refused"
)

// StationRemovalReconciler deletes retired Stations an administrator asked to
// have deleted. Only a retired Station goes, and only one nothing stands on
// any more: another Station stands for its node's name, or its cluster has no
// Node of that name. The observation stored under the name goes with it when
// it is that Station's node's; an audit event stays. Sequences are not given
// out again: a display name is never another machine's.
type StationRemovalReconciler struct {
	Client        client.Client
	Reader        client.Reader
	Registrations ClusterResolver
	IdentityOps   IdentityOperationCoordinator
	Targets       RebindTargetReader
	Now           func() time.Time
}

func (r *StationRemovalReconciler) SetupWithManager(mgr manager.Manager) error {
	asked := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetAnnotations()[StationDeleteRequestedAnnotation] != ""
	})
	return ctrl.NewControllerManagedBy(mgr).Named("station-removal").For(&stationv1alpha1.Station{}, builder.WithPredicates(asked)).Complete(r)
}

func (r *StationRemovalReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	release, err := r.IdentityOps.Acquire(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, fmt.Errorf("acquire Station identity operation lock: %w", err)
	}
	defer release()
	station := &stationv1alpha1.Station{}
	if err := r.Reader.Get(ctx, req.NamespacedName, station); apierrors.IsNotFound(err) {
		return reconcile.Result{}, nil
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("read Station: %w", err)
	}
	actor := station.Annotations[StationDeleteRequestedAnnotation]
	if actor == "" {
		return reconcile.Result{}, nil
	}
	registration, err := r.Registrations.ResolveNamespace(ctx, req.Namespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	refusal, err := r.refusal(ctx, registration, station)
	if err != nil {
		return reconcile.Result{}, err
	}
	if refusal != "" {
		refused := station.DeepCopy()
		delete(refused.Annotations, StationDeleteRequestedAnnotation)
		refused.Annotations[StationDeleteRefusedAnnotation] = refusal
		if err := r.Client.Patch(ctx, refused, client.MergeFromWithOptions(station, client.MergeFromWithOptimisticLock{})); err != nil {
			return reconcile.Result{}, fmt.Errorf("say why Station %s is not deleted: %w", station.Name, err)
		}
		return reconcile.Result{}, nil
	}

	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	// The audit event is there before anything goes; a retry writes the same one.
	event := &stationv1alpha1.StationAuditEvent{
		ObjectMeta: metav1.ObjectMeta{Namespace: station.Namespace, Name: AuditEventName(string(station.UID) + "/delete")},
		Spec: stationv1alpha1.StationAuditEventSpec{
			Operation: stationv1alpha1.StationReference{Name: station.Name, UID: string(station.UID)}, OperationID: string(station.UID),
			ClusterID: registration.ClusterID, Action: "Delete", Outcome: "Applied", ResultReason: "RetiredStationDeleted",
			Station: stationv1alpha1.StationReference{Name: station.Name, UID: string(station.UID)},
			Actor:   normalizedRequester(actor), OccurredAt: metav1.NewTime(now().UTC()),
			PreviousNodeUID: station.Spec.Binding.NodeUID, PreviousBindingVersion: station.Spec.Binding.Version,
			Reason: fmt.Sprintf("retired Station %s of node %s deleted", station.Spec.DisplayName, station.Spec.Binding.NodeName),
		},
	}
	if err := r.Client.Create(ctx, event); err != nil && !apierrors.IsAlreadyExists(err) {
		return reconcile.Result{}, fmt.Errorf("record the deletion of Station %s: %w", station.Name, err)
	}
	// What is stored under the name is the Station's node's only if it says that node's UID: a machine that joined
	// again under the name has its own there.
	observation := &stationv1alpha1.StationObservation{}
	err = r.Reader.Get(ctx, types.NamespacedName{Namespace: station.Namespace, Name: station.Spec.Binding.NodeName}, observation)
	if err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("read the observation of %s: %w", station.Spec.Binding.NodeName, err)
	}
	if err == nil && observation.Annotations["station.rlark.io/node-uid"] == station.Spec.Binding.NodeUID {
		if err := r.Client.Delete(ctx, observation, client.Preconditions{UID: &observation.UID}); err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
			return reconcile.Result{}, fmt.Errorf("delete the observation of the Station's node: %w", err)
		}
	}
	if err := r.Client.Delete(ctx, station, client.Preconditions{UID: &station.UID}); err != nil && !apierrors.IsNotFound(err) {
		return reconcile.Result{}, fmt.Errorf("delete Station %s: %w", station.Name, err)
	}
	return reconcile.Result{}, nil
}

// refusal says why a Station is not deleted, empty when it is.
func (r *StationRemovalReconciler) refusal(ctx context.Context, registration ClusterRegistration, station *stationv1alpha1.Station) (string, error) {
	if !station.Spec.Retired {
		return "only a retired Station is deleted: remove its node first", nil
	}
	stations := &stationv1alpha1.StationList{}
	if err := r.Reader.List(ctx, stations, client.InNamespace(station.Namespace)); err != nil {
		return "", fmt.Errorf("list Stations: %w", err)
	}
	for i := range stations.Items {
		other := &stations.Items[i]
		if other.UID != station.UID && !other.Spec.Retired && other.Spec.Binding.NodeName == station.Spec.Binding.NodeName {
			return "", nil
		}
	}
	uid, _, exists, err := r.Targets.VerifyNode(ctx, registration.ClusterID, registration.Namespace, station.Spec.Binding.NodeName)
	if err != nil {
		return "", fmt.Errorf("read Node %s of %s: %w", station.Spec.Binding.NodeName, registration.ClusterID, err)
	}
	if exists {
		return fmt.Sprintf("cluster %s has a Node %s (UID %s) and no other Station stands for it yet: it is deleted once that node has a Station of its own, or is removed", registration.ClusterID, station.Spec.Binding.NodeName, uid), nil
	}
	return "", nil
}
