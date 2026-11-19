package controller

import (
	"context"
	"errors"
	"fmt"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrNodeDeclaredInSiteNetwork stops the removal of a node the SiteNetwork
// declares with its key: that declaration is the administrator's to change.
var ErrNodeDeclaredInSiteNetwork = errors.New("the SiteNetwork declares this node with its key")

// RemovedNodeAnnotation is, on a NodeAdmission, the UID of the node of its
// name that was removed last, or the removal's own UID when the name had no
// Node in its cluster any more. With no key on the admission, its machine is
// not that node any more.
const RemovedNodeAnnotation = "station.rlark.io/removed-node-uid"

// AdmissionNodeRemoved says whether the node an admission made was removed
// and no machine has redeemed it since.
func AdmissionNodeRemoved(admission *stationv1alpha1.NodeAdmission) bool {
	return admission.Spec.PublicKey == "" && admission.Annotations[RemovedNodeAnnotation] != ""
}

// StationTunnelRelease takes a removed node out of the SiteNetwork's tunnel.
// Its Station is retired, which rejects its enrollment and gives its address
// back, and its admission forgets the machine's key: then nothing puts the
// machine among the peers. A machine admitted again under the name redeems a
// new code with its key; one that joins otherwise gets a Station of its own.
type StationTunnelRelease struct {
	Reader client.Reader
	Writer client.Client
	// Network is the SiteNetwork.
	Network types.NamespacedName
}

func (s StationTunnelRelease) Check(ctx context.Context, op *stationv1alpha1.NodeOperation) error {
	network := &stationv1alpha1.SiteNetwork{}
	if err := s.Reader.Get(ctx, s.Network, network); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("read SiteNetwork: %w", err)
	}
	for _, node := range network.Spec.Nodes {
		if node.ClusterID == op.Spec.ClusterID && node.NodeName == op.Spec.NodeName && node.PublicKey != "" {
			return fmt.Errorf("%w: SiteNetwork %s declares %s/%s with its key; take it out there first", ErrNodeDeclaredInSiteNetwork, s.Network.Name, node.ClusterID, node.NodeName)
		}
	}
	return nil
}

// Remove is done once the node's enrollment, if it has one, is no longer
// answered with a place in the tunnel.
func (s StationTunnelRelease) Remove(ctx context.Context, op *stationv1alpha1.NodeOperation) (bool, error) {
	if err := s.Check(ctx, op); err != nil {
		return false, err
	}
	stations := &stationv1alpha1.StationList{}
	if err := s.Reader.List(ctx, stations, client.InNamespace(op.Namespace)); err != nil {
		return false, fmt.Errorf("list Stations: %w", err)
	}
	for i := range stations.Items {
		station := &stations.Items[i]
		// A name with no Node in its cluster: no Station of it is of a live node.
		if station.Spec.Binding.NodeName != op.Spec.NodeName || op.Spec.NodeUID != "" && station.Spec.Binding.NodeUID != op.Spec.NodeUID || station.Spec.Retired {
			continue
		}
		retired := station.DeepCopy()
		retired.Spec.Retired = true
		if err := s.Writer.Patch(ctx, retired, client.MergeFromWithOptions(station, client.MergeFromWithOptimisticLock{})); err != nil {
			return false, fmt.Errorf("retire Station %s: %w", station.Name, err)
		}
		// Its node reports no more: nothing else would take what it showed last (Bound, a failed probe) off it.
		said := retired.DeepCopy()
		said.Status.Phase = stationv1alpha1.StationPhaseRetired
		if err := s.Writer.Status().Patch(ctx, said, client.MergeFrom(retired)); err != nil {
			return false, fmt.Errorf("say that Station %s is retired: %w", station.Name, err)
		}
	}
	removed := op.Spec.NodeUID
	if removed == "" {
		removed = string(op.UID)
	}
	admission := &stationv1alpha1.NodeAdmission{}
	err := s.Reader.Get(ctx, types.NamespacedName{Namespace: op.Namespace, Name: op.Spec.NodeName}, admission)
	switch {
	case err != nil && !apierrors.IsNotFound(err):
		return false, fmt.Errorf("read NodeAdmission %s: %w", op.Spec.NodeName, err)
	case err == nil && op.Spec.NodeUID == "" && admission.Spec.Serial == "" && admission.Status.Phase != stationv1alpha1.NodeAdmissionJoined:
		// A join that never made a node: its record goes, with the key and
		// the address it holds. A discovered machine's (Serial) stays its Machine's.
		if err := s.Writer.Delete(ctx, admission, client.Preconditions{UID: &admission.UID}); err != nil && !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("clear the unfinished NodeAdmission %s: %w", admission.Name, err)
		}
	case err == nil && (admission.Spec.PublicKey != "" || admission.Annotations[RemovedNodeAnnotation] != removed):
		forgotten := admission.DeepCopy()
		forgotten.Spec.PublicKey, forgotten.Spec.RedeemedAt = "", nil
		if forgotten.Annotations == nil {
			forgotten.Annotations = map[string]string{}
		}
		forgotten.Annotations[RemovedNodeAnnotation] = removed
		if err := s.Writer.Patch(ctx, forgotten, client.MergeFromWithOptions(admission, client.MergeFromWithOptimisticLock{})); err != nil {
			return false, fmt.Errorf("take the machine's key off NodeAdmission %s: %w", admission.Name, err)
		}
	}
	enrollment := &stationv1alpha1.NetworkEnrollment{}
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: op.Namespace, Name: op.Spec.NodeName}, enrollment); apierrors.IsNotFound(err) {
		return true, nil
	} else if err != nil {
		return false, fmt.Errorf("read NetworkEnrollment %s: %w", op.Spec.NodeName, err)
	}
	return op.Spec.NodeUID != "" && enrollment.Spec.NodeUID != op.Spec.NodeUID || enrollment.Status.Phase != stationv1alpha1.NetworkEnrollmentEnrolled, nil
}

// NodePeerCleaners are the places a node is a peer in, each released in turn.
type NodePeerCleaners []NodeWireGuardPeerCleaner

func (c NodePeerCleaners) Check(ctx context.Context, op *stationv1alpha1.NodeOperation) error {
	for _, cleaner := range c {
		if err := cleaner.Check(ctx, op); err != nil {
			return err
		}
	}
	return nil
}

func (c NodePeerCleaners) Remove(ctx context.Context, op *stationv1alpha1.NodeOperation) (bool, error) {
	for _, cleaner := range c {
		if done, err := cleaner.Remove(ctx, op); !done || err != nil {
			return false, err
		}
	}
	return true, nil
}
