package controller

import (
	"context"
	"errors"
	"fmt"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NodeMemberClientSource interface {
	Client(context.Context, string) (kubernetes.Interface, error)
}

// JoinedNodeReadiness verifies RLark's observation belongs to the specified
// member Node instance, rather than accepting a same-named stale record.
type JoinedNodeReadiness interface {
	Ready(context.Context, string, string, string, string) (bool, error)
}

// NodeWireGuardPeerCleaner takes a removed node out of the tunnel: Check
// before drain, Remove until it is done.
type NodeWireGuardPeerCleaner interface {
	Check(context.Context, *stationv1alpha1.NodeOperation) error
	Remove(context.Context, *stationv1alpha1.NodeOperation) (bool, error)
}

type NodeLifecycleExecutor struct {
	Members     NodeMemberClientSource
	ResetDriver NodeResetDriver
	// Management holds the management plane's Node of a node removed.
	Management client.Client
	// Power powers a robot off before it is removed.
	Power interface {
		PowerOff(context.Context, *stationv1alpha1.NodeOperation) (bool, error)
	}
	PeerCleaner     NodeWireGuardPeerCleaner
	JoinedReadiness JoinedNodeReadiness
	// Installer runs the stages of a Join, which redeems a NodeAdmission.
	Installer NodeOperationExecutor
	// RemovalReady checks that reset and peer cleanup are available before drain.
	RemovalReady func(context.Context, *stationv1alpha1.NodeOperation) error
}

var ErrNodeStageUnavailable = errors.New("node operation stage is not implemented")

// Execute only completes stages whose effects are verified by current state.
// The stage list of each action (nodeOperationStages) says which stages run.
func (e *NodeLifecycleExecutor) Execute(ctx context.Context, op *stationv1alpha1.NodeOperation, stage string) (bool, string, error) {
	if op.Spec.Action == "Join" && stage != "VerifyNode" && stage != "VerifyRLark" {
		if e.Installer == nil {
			return false, "", ErrNodeStageUnavailable
		}
		return e.Installer.Execute(ctx, op, stage)
	}
	// A removal of a name its cluster has no Node of (Preflight found none):
	// nothing in the cluster is powered off, drained or reset. A machine that
	// joins again under the name is reset by its installer.
	if op.Spec.Action == "Remove" && op.Spec.NodeUID == "" && (stage == "PowerOff" || stage == "Drain" || stage == "Reset") {
		return true, "", nil
	}
	switch stage {
	case "PowerOff":
		done, err := e.Power.PowerOff(ctx, op)
		return done, op.Spec.NodeUID, err
	case "Reset":
		done, err := e.ResetDriver.Reset(ctx, op)
		return done, op.Spec.NodeUID, err
	case "RemovePeer":
		done, err := e.PeerCleaner.Remove(ctx, op)
		return done, op.Spec.NodeUID, err
	}
	member, err := e.Members.Client(ctx, op.Spec.ClusterID)
	if err != nil {
		return false, "", fmt.Errorf("connect member cluster: %w", err)
	}
	node, err := member.CoreV1().Nodes().Get(ctx, op.Spec.NodeName, metav1.GetOptions{})
	switch stage {
	case "Preflight":
		// Nothing logs in to the machine: a reporting node is reset from inside its cluster.
		if apierrors.IsNotFound(err) && op.Spec.NodeUID == "" {
			if err := e.RemovalReady(ctx, op); err != nil {
				return false, "", fmt.Errorf("verify removal prerequisites: %w", err)
			}
			return true, "", nil
		}
		if err != nil {
			return false, "", fmt.Errorf("read node before removal: %w", err)
		}
		if op.Spec.NodeUID == "" {
			op.Spec.NodeUID = string(node.UID)
		}
		if err := e.RemovalReady(ctx, op); err != nil {
			return false, "", fmt.Errorf("verify removal prerequisites: %w", err)
		}
		if string(node.UID) != op.Spec.NodeUID || node.DeletionTimestamp != nil {
			return false, "", ErrNodeIdentityChanged
		}
		for _, role := range []string{"node-role.kubernetes.io/control-plane", "node-role.kubernetes.io/master"} {
			if _, found := node.Labels[role]; found {
				return false, "", fmt.Errorf("%w: control-plane node", ErrNodeDrainBlocked)
			}
		}
		return true, op.Spec.NodeUID, nil
	case "Drain":
		if err := e.RemovalReady(ctx, op); err != nil {
			return false, "", fmt.Errorf("verify removal prerequisites: %w", err)
		}
		if err != nil {
			return false, "", fmt.Errorf("read node before drain: %w", err)
		}
		// No kubelet confirms an eviction on a node that no longer reports.
		drain := DrainWorker
		if !nodeReady(node) {
			drain = ForceDrainWorker
		}
		done, err := drain(ctx, member, op.Spec.NodeName, types.UID(op.Spec.NodeUID))
		return done, op.Spec.NodeUID, err
	case "VerifyNode":
		if apierrors.IsNotFound(err) {
			return false, op.Status.NodeUID, nil
		}
		if err != nil {
			return false, "", fmt.Errorf("read joined node: %w", err)
		}
		if string(node.UID) != op.Status.NodeUID || node.DeletionTimestamp != nil {
			return false, "", ErrNodeIdentityChanged
		}
		return nodeReady(node), op.Status.NodeUID, nil
	case "VerifyRLark":
		if apierrors.IsNotFound(err) {
			return false, "", ErrNodeIdentityChanged
		}
		if err != nil {
			return false, "", fmt.Errorf("read joined node: %w", err)
		}
		if string(node.UID) != op.Status.NodeUID || node.DeletionTimestamp != nil {
			return false, "", ErrNodeIdentityChanged
		}
		ready, err := e.JoinedReadiness.Ready(ctx, op.Spec.ClusterID, op.Namespace, op.Spec.NodeName, op.Status.NodeUID)
		if err != nil {
			return false, "", fmt.Errorf("verify RLark node: %w", err)
		}
		return ready, op.Status.NodeUID, nil
	case "DeleteNode", "VerifyRemoval":
		if apierrors.IsNotFound(err) {
			// The management plane's Node of it goes after it: nothing
			// else removes it, and the node list shows those.
			listed := &unstructured.Unstructured{}
			listed.SetGroupVersionKind(rlarkNodeGVK)
			listed.SetNamespace(op.Namespace)
			listed.SetName(op.Spec.NodeName)
			if err := e.Management.Get(ctx, client.ObjectKeyFromObject(listed), listed); apierrors.IsNotFound(err) {
				// The node's record goes last: nothing of the node is left to report under it.
				if stage == "VerifyRemoval" {
					enrollment := &stationv1alpha1.NetworkEnrollment{ObjectMeta: metav1.ObjectMeta{Namespace: op.Namespace, Name: op.Spec.NodeName}}
					if err := e.Management.Delete(ctx, enrollment); err != nil && !apierrors.IsNotFound(err) {
						return false, "", fmt.Errorf("delete the node's NetworkEnrollment: %w", err)
					}
				}
				return true, op.Spec.NodeUID, nil
			} else if err != nil {
				return false, "", fmt.Errorf("read the management plane's Node: %w", err)
			}
			if stage == "DeleteNode" && listed.GetDeletionTimestamp() == nil {
				if err := e.Management.Delete(ctx, listed); err != nil && !apierrors.IsNotFound(err) {
					return false, "", fmt.Errorf("delete the management plane's Node: %w", err)
				}
			}
			return false, op.Spec.NodeUID, nil
		}
		if err != nil {
			return false, "", fmt.Errorf("read node before removal: %w", err)
		}
		if string(node.UID) != op.Spec.NodeUID {
			return false, "", ErrNodeIdentityChanged
		}
		if stage == "VerifyRemoval" || node.DeletionTimestamp != nil {
			return false, op.Spec.NodeUID, nil
		}
		uid := types.UID(op.Spec.NodeUID)
		if err := member.CoreV1().Nodes().Delete(ctx, op.Spec.NodeName, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
			return false, "", fmt.Errorf("delete node: %w", err)
		}
		return false, op.Spec.NodeUID, nil
	default:
		return false, "", fmt.Errorf("%w: %s", ErrNodeStageUnavailable, stage)
	}
}
