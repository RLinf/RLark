package pod

import (
	"context"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// pushPodReconciler watches local K8s Pods and reports their info to management Pod CRs.
type pushPodReconciler struct {
	c *Controller
}

// Reconcile reconciles the resource.
func (r *pushPodReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("pod", req.NamespacedName)

	var k8sPod corev1.Pod
	if err := r.c.LocalKubeClient.Get(ctx, req.NamespacedName, &k8sPod); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to get local K8s Pod")
			return reconcile.Result{}, err
		}
		// Pod deleted — clean up management Pod CR
		return r.deleteManagementPod(ctx, logger, req.Name, req.Namespace)
	}

	// Only reconcile pods managed by rlark (have management-task annotation)
	annotations := k8sPod.Annotations
	if annotations == nil {
		return reconcile.Result{}, nil
	}
	taskName := annotations["rlark.io/management-task-name"]
	taskNamespace := annotations["rlark.io/management-task-namespace"]
	if taskName == "" {
		return reconcile.Result{}, nil
	}

	desiredPod := r.buildRLarkPodFromK8sPod(&k8sPod, taskName, taskNamespace)
	return r.updateManagementPod(ctx, logger, desiredPod)
}

func (r *pushPodReconciler) buildRLarkPodFromK8sPod(k8sPod *corev1.Pod, taskName, taskNamespace string) *rlarkv1alpha1.Pod {
	phase := convertK8sPodPhase(k8sPod.Status.Phase)

	podSpec := rlarkv1alpha1.PodSpec{
		TaskNamespace: taskNamespace,
		TaskName:      taskName,
		PodNamespace:  k8sPod.Namespace,
		PodName:       k8sPod.Name,
	}
	// Domain is read from pod annotation set by task pull controller
	if domain := k8sPod.Annotations["rlark.io/management-task-domain"]; domain != "" {
		podSpec.Domain = domain
	}

	podStatus := rlarkv1alpha1.PodStatus{
		Phase:   phase,
		Node:    k8sPod.Spec.NodeName,
		IP:      k8sPod.Status.PodIP,
		Message: k8sPod.Status.Message,
	}

	// Labels enable lookup by k8s pod name/namespace (e.g. for deletion when only the
	// pod name is available, not the UID).
	return &rlarkv1alpha1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      string(k8sPod.UID),
			Namespace: r.c.ManagementNamespace,
			Labels: map[string]string{
				rlarkv1alpha1.PodLabelLocalPodName:      k8sPod.Name,
				rlarkv1alpha1.PodLabelLocalPodNamespace: k8sPod.Namespace,
				rlarkv1alpha1.PodLabelTaskName:          taskName,
			},
		},
		Spec:   podSpec,
		Status: podStatus,
	}
}

func (r *pushPodReconciler) updateManagementPod(ctx context.Context, logger logr.Logger, desiredPod *rlarkv1alpha1.Pod) (reconcile.Result, error) {
	var mgmtPod rlarkv1alpha1.Pod
	err := r.c.ManagementClient.Get(ctx, types.NamespacedName{Name: desiredPod.Name, Namespace: desiredPod.Namespace}, &mgmtPod)
	if err != nil && client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed to get management Pod")
		return reconcile.Result{}, err
	}

	if err != nil {
		// Create — status subresource is dropped by API server on Create,
		// so we must call Status().Update afterwards.
		logger.Info("creating Pod on management cluster")
		if err := r.c.ManagementClient.Create(ctx, desiredPod); err != nil {
			logger.Error(err, "failed to create management Pod")
			return reconcile.Result{}, err
		}
		mgmtPod := desiredPod.DeepCopy()
		mgmtPod.Status = desiredPod.Status
		if err := r.c.ManagementClient.Status().Update(ctx, mgmtPod); err != nil {
			logger.Error(err, "failed to set management Pod status after create")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Update labels + spec in one call
	mgmtPod.Labels = desiredPod.Labels
	mgmtPod.Spec = desiredPod.Spec
	if err := r.c.ManagementClient.Update(ctx, &mgmtPod); err != nil {
		logger.Error(err, "failed to update management Pod")
		return reconcile.Result{}, err
	}

	// Update status
	mgmtPod.Status = desiredPod.Status
	if err := r.c.ManagementClient.Status().Update(ctx, &mgmtPod); err != nil {
		logger.Error(err, "failed to update management Pod status")
		return reconcile.Result{}, err
	}

	logger.V(1).Info("management Pod reported successfully")
	return reconcile.Result{}, nil
}

func (r *pushPodReconciler) deleteManagementPod(ctx context.Context, logger logr.Logger, name, namespace string) (reconcile.Result, error) {
	// Find management Pod(s) by the k8s pod name/namespace labels (name alone isn't
	// enough since management Pods are named by k8s UID, not pod name).
	var mgmtPodList rlarkv1alpha1.PodList
	if err := r.c.ManagementClient.List(ctx, &mgmtPodList,
		client.InNamespace(r.c.ManagementNamespace),
		client.MatchingLabels{
			rlarkv1alpha1.PodLabelLocalPodName:      name,
			rlarkv1alpha1.PodLabelLocalPodNamespace: namespace,
		}); err != nil {
		logger.Error(err, "failed to list management Pods for deletion")
		return reconcile.Result{}, err
	}
	for i := range mgmtPodList.Items {
		if err := r.c.ManagementClient.Delete(ctx, &mgmtPodList.Items[i]); err != nil && client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to delete management Pod", "managementPod", mgmtPodList.Items[i].Name)
			return reconcile.Result{}, err
		}
	}
	if len(mgmtPodList.Items) > 0 {
		logger.V(1).Info("management Pod(s) deleted", "count", len(mgmtPodList.Items))
	}
	return reconcile.Result{}, nil
}

func convertK8sPodPhase(phase corev1.PodPhase) rlarkv1alpha1.PodPhase {
	switch phase {
	case corev1.PodPending:
		return rlarkv1alpha1.PodPhasePending
	case corev1.PodRunning:
		return rlarkv1alpha1.PodPhaseRunning
	case corev1.PodSucceeded:
		return rlarkv1alpha1.PodPhaseSucceeded
	case corev1.PodFailed:
		return rlarkv1alpha1.PodPhaseFailed
	default:
		return rlarkv1alpha1.PodPhasePending
	}
}
