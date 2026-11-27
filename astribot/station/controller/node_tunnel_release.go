package controller

import (
	"context"
	"errors"
	"fmt"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
// Its NetworkEnrollment is retired, which rejects it, and its admission
// forgets the machine's key: then nothing puts the machine among the peers.
// A machine admitted again under the name redeems a new code with its key
// and takes the record over.
type StationTunnelRelease struct {
	Reader  client.Reader
	Writer  client.Client
	Members NodeMemberClientSource
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
// answered with a place in the tunnel and its network DaemonSet pod is gone.
func (s StationTunnelRelease) Remove(ctx context.Context, op *stationv1alpha1.NodeOperation) (bool, error) {
	if err := s.Check(ctx, op); err != nil {
		return false, err
	}
	key := types.NamespacedName{Namespace: op.Namespace, Name: op.Spec.NodeName}
	enrollment := &stationv1alpha1.NetworkEnrollment{}
	err := s.Reader.Get(ctx, key, enrollment)
	if err != nil && !apierrors.IsNotFound(err) {
		return false, fmt.Errorf("read NetworkEnrollment %s: %w", op.Spec.NodeName, err)
	}
	// A name with no Node in its cluster: the record of it is of no live node.
	recorded := err == nil && (op.Spec.NodeUID == "" || enrollment.Spec.NodeUID == op.Spec.NodeUID)
	if recorded && !enrollment.Spec.Retired {
		retired := enrollment.DeepCopy()
		retired.Spec.Retired = true
		if err := s.Writer.Patch(ctx, retired, client.MergeFromWithOptions(enrollment, client.MergeFromWithOptimisticLock{})); err != nil {
			return false, fmt.Errorf("retire NetworkEnrollment %s: %w", enrollment.Name, err)
		}
	}
	removed := op.Spec.NodeUID
	if removed == "" {
		removed = string(op.UID)
	}
	admission := &stationv1alpha1.NodeAdmission{}
	err = s.Reader.Get(ctx, key, admission)
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
	if op.Spec.NodeUID != "" {
		if stopped, err := s.stopNetworkPod(ctx, op); !stopped || err != nil {
			return false, err
		}
	}
	return !recorded || enrollment.Status.Phase != stationv1alpha1.NetworkEnrollmentEnrolled, nil
}

const (
	stationSystemNamespace = "station-system"
	stationNetworkLabel    = "station.rlark.io/network"
)

// stopNetworkPod takes the node out of the network DaemonSet (the label that
// selects it) and is done when no pod of it is left on the node. A node
// whose tunnel is a host service (station.rlark.io/network=host) has none.
func (s StationTunnelRelease) stopNetworkPod(ctx context.Context, op *stationv1alpha1.NodeOperation) (bool, error) {
	member, err := s.Members.Client(ctx, op.Spec.ClusterID)
	if err != nil {
		return false, fmt.Errorf("connect member cluster: %w", err)
	}
	node, err := member.CoreV1().Nodes().Get(ctx, op.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("read member node for peer cleanup: %w", err)
	}
	if string(node.UID) != op.Spec.NodeUID || node.DeletionTimestamp != nil {
		return false, ErrNodeIdentityChanged
	}
	if node.Labels[stationNetworkLabel] == "enabled" {
		delete(node.Labels, stationNetworkLabel)
		if _, err := member.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{}); err != nil {
			return false, fmt.Errorf("take the node out of the network DaemonSet: %w", err)
		}
	}
	pods, err := member.CoreV1().Pods(stationSystemNamespace).List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + op.Spec.NodeName, LabelSelector: "app=station-agent-network"})
	if err != nil {
		return false, fmt.Errorf("list the node's network DaemonSet pods: %w", err)
	}
	// No kubelet stops the pod on a node that no longer reports.
	if !nodeReady(node) {
		now := int64(0)
		for i := range pods.Items {
			pod := &pods.Items[i]
			if err := member.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &now, Preconditions: &metav1.Preconditions{UID: &pod.UID}}); err != nil && !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("delete network DaemonSet pod %s: %w", pod.Name, err)
			}
		}
	}
	return len(pods.Items) == 0, nil
}
