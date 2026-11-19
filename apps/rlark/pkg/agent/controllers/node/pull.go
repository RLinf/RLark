package node

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

type pullNodeReconciler struct {
	c *Controller
}

// Reconcile reconciles the resource.
func (r *pullNodeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", req.NamespacedName)

	var mgmtNode rlarkv1alpha1.Node
	if err := r.c.ManagementClient.Get(ctx, req.NamespacedName, &mgmtNode); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	var localNode corev1.Node
	if err := r.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: mgmtNode.Name}, &localNode); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("local K8s Node not found, skipping label sync")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	changed := false
	if localNode.Labels == nil {
		localNode.Labels = make(map[string]string)
	}

	for key := range localNode.Labels {
		if !isManagementOwnedNodeLabel(key) {
			continue
		}
		if _, exists := mgmtNode.Labels[key]; !exists {
			delete(localNode.Labels, key)
			changed = true
		}
	}
	for key, value := range mgmtNode.Labels {
		if !isManagementOwnedNodeLabel(key) {
			continue
		}
		if localNode.Labels[key] != value {
			localNode.Labels[key] = value
			changed = true
		}
	}

	if localNode.Spec.Unschedulable != mgmtNode.Spec.Unschedulable {
		localNode.Spec.Unschedulable = mgmtNode.Spec.Unschedulable
		changed = true
	}

	if !changed {
		return reconcile.Result{}, nil
	}

	if err := r.c.LocalKubeClient.Update(ctx, &localNode); err != nil {
		logger.Error(err, "failed to update local K8s Node")
		return reconcile.Result{}, err
	}

	logger.Info("synced scheduling labels and schedulability from management Node to local K8s Node")
	return reconcile.Result{}, nil
}
