package controller

import (
	"context"
	"fmt"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StationJoinedReadiness combines the node's record with the management Node
// condition: the record is of this Node UID and its node reports (Bound). A
// same-named RLark Node alone is not identity. A record still of the name's
// earlier Node is waited for; one in Conflict will not pass to this Node.
type StationJoinedReadiness struct {
	Reader client.Reader
	Nodes  NodeReadiness
}

func (r StationJoinedReadiness) Ready(ctx context.Context, clusterID, namespace, nodeName, nodeUID string) (bool, error) {
	enrollment := &stationv1alpha1.NetworkEnrollment{}
	if err := r.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: nodeName}, enrollment); apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("read NetworkEnrollment %s: %w", nodeName, err)
	}
	if nodeState(enrollment) == stationv1alpha1.NodeStateConflict {
		return false, fmt.Errorf("%w: the record of %s is of Node UID %s; delete the record and the node registers again", ErrNodeIdentityChanged, nodeName, enrollment.Spec.NodeUID)
	}
	if enrollment.Spec.Retired || enrollment.Spec.NodeUID != nodeUID || nodeState(enrollment) != stationv1alpha1.NodeStateBound {
		return false, nil
	}
	return r.Nodes.Ready(ctx, clusterID, namespace, nodeName)
}
