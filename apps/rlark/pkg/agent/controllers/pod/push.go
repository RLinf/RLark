package pod

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
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
	for _, uid := range r.c.pendingDeleteUIDs(req.NamespacedName) {
		if _, err := r.deleteManagementPod(ctx, logger, uid); err != nil {
			return reconcile.Result{}, err
		}
		r.c.acknowledgeDeleteUID(req.NamespacedName, uid)
	}

	var k8sPod corev1.Pod
	if err := r.c.LocalKubeClient.Get(ctx, req.NamespacedName, &k8sPod); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to get local K8s Pod")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Only reconcile pods managed by rlark (have management-task annotation)
	annotations := k8sPod.Annotations
	if annotations == nil {
		return reconcile.Result{}, nil
	}
	taskName := annotations["rlark.io/management-task-name"]
	taskNamespace := annotations["rlark.io/management-task-namespace"]
	taskUID := annotations["rlark.io/management-task-uid"]
	if taskName == "" {
		return reconcile.Result{}, nil
	}
	if taskNamespace == "" {
		logger.Info("ignoring Pod with incomplete management Task identity")
		return reconcile.Result{}, nil
	}
	if taskUID == "" {
		resolvedUID, err := r.c.resolveLegacyTaskUID(ctx, &k8sPod, taskName, taskNamespace)
		if err != nil {
			return reconcile.Result{}, err
		}
		if resolvedUID == "" {
			return reconcile.Result{}, nil
		}
		taskUID = resolvedUID
	}

	desiredPod, err := r.buildRLarkPodFromK8sPod(ctx, &k8sPod, taskName, taskNamespace, taskUID)
	if err != nil {
		return reconcile.Result{}, err
	}
	return r.updateManagementPod(ctx, logger, desiredPod)
}

func (c *Controller) resolveLegacyTaskUID(ctx context.Context, pod *corev1.Pod, taskName, taskNamespace string) (string, error) {
	var task rlarkv1alpha1.Task
	if err := c.ManagementClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: taskNamespace}, &task); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return "", nil
		}
		return "", err
	}
	owner := metav1.GetControllerOf(pod)
	if owner == nil || owner.APIVersion != appsv1.SchemeGroupVersion.String() {
		return "", nil
	}
	var workload client.Object
	switch owner.Kind {
	case "StatefulSet":
		workload = &appsv1.StatefulSet{}
	case "DaemonSet":
		workload = &appsv1.DaemonSet{}
	case "ReplicaSet":
		var rs appsv1.ReplicaSet
		if err := c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: owner.Name, Namespace: pod.Namespace}, &rs); err != nil || rs.UID != owner.UID {
			return "", nil
		}
		owner = metav1.GetControllerOf(&rs)
		if owner == nil || owner.APIVersion != appsv1.SchemeGroupVersion.String() || owner.Kind != "Deployment" {
			return "", nil
		}
		workload = &appsv1.Deployment{}
	default:
		return "", nil
	}
	if err := c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: owner.Name, Namespace: pod.Namespace}, workload); err != nil || workload.GetUID() != owner.UID {
		return "", nil
	}
	a := workload.GetAnnotations()
	if a["rlark.io/management-task-name"] != taskName || a["rlark.io/management-task-namespace"] != taskNamespace {
		return "", nil
	}
	if uid := a["rlark.io/management-task-uid"]; uid != "" && uid != string(task.UID) {
		return "", nil
	}
	return string(task.UID), nil
}

func validateManagementPodIdentity(existing, desired *rlarkv1alpha1.Pod) error {
	// A same-name object with no RLark identity cannot be proven to be ours.
	if existing.Labels[rlarkv1alpha1.PodLabelLocalPodUID] == "" &&
		existing.Labels[rlarkv1alpha1.PodLabelTaskUID] == "" && metav1.GetControllerOf(existing) == nil &&
		existing.Spec.PodName == "" && existing.Spec.TaskName == "" && len(existing.Annotations) == 0 {
		return apierrors.NewConflict(rlarkv1alpha1.Resource("pods"), existing.Name, fmt.Errorf("existing Pod has no authoritative RLark identity"))
	}
	checks := []struct{ name, got, want string }{
		{"object name", existing.Name, desired.Name},
		{"local Pod UID", existing.Labels[rlarkv1alpha1.PodLabelLocalPodUID], desired.Labels[rlarkv1alpha1.PodLabelLocalPodUID]},
		{"local Pod name", existing.Labels[rlarkv1alpha1.PodLabelLocalPodName], desired.Labels[rlarkv1alpha1.PodLabelLocalPodName]},
		{"local Pod namespace", existing.Labels[rlarkv1alpha1.PodLabelLocalPodNamespace], desired.Labels[rlarkv1alpha1.PodLabelLocalPodNamespace]},
		{"Task name", existing.Labels[rlarkv1alpha1.PodLabelTaskName], desired.Labels[rlarkv1alpha1.PodLabelTaskName]},
		{"Task UID", existing.Labels[rlarkv1alpha1.PodLabelTaskUID], desired.Labels[rlarkv1alpha1.PodLabelTaskUID]},
		{"agent scope", existing.Labels[rlarkv1alpha1.PodLabelAgentScope], desired.Labels[rlarkv1alpha1.PodLabelAgentScope]},
		{"spec local Pod name", existing.Spec.PodName, desired.Spec.PodName},
		{"spec local Pod namespace", existing.Spec.PodNamespace, desired.Spec.PodNamespace},
		{"spec Task name", existing.Spec.TaskName, desired.Spec.TaskName},
		{"spec Task namespace", existing.Spec.TaskNamespace, desired.Spec.TaskNamespace},
	}
	for _, check := range checks {
		if check.got != "" && check.want != "" && check.got != check.want {
			return apierrors.NewConflict(rlarkv1alpha1.Resource("pods"), existing.Name, fmt.Errorf("%s is %q, not %q", check.name, check.got, check.want))
		}
	}
	existingOwner, desiredOwner := metav1.GetControllerOf(existing), metav1.GetControllerOf(desired)
	if existingOwner != nil && (desiredOwner == nil || existingOwner.APIVersion != desiredOwner.APIVersion || existingOwner.UID != desiredOwner.UID || existingOwner.Kind != desiredOwner.Kind || existingOwner.Name != desiredOwner.Name) {
		return apierrors.NewConflict(rlarkv1alpha1.Resource("pods"), existing.Name, fmt.Errorf("controller owner does not match reporting Task"))
	}
	return nil
}

func (r *pushPodReconciler) buildRLarkPodFromK8sPod(ctx context.Context, k8sPod *corev1.Pod, taskName, taskNamespace, taskUID string) (*rlarkv1alpha1.Pod, error) {
	phase := convertK8sPodPhase(k8sPod.Status.Phase)
	message := k8sPod.Status.Message
	// A K8s Pod can be phase=Running while its main container (the container
	// named "main") is stuck in CrashLoopBackOff. Surface that as Failed on
	// the management Pod CR so operators see the failure immediately instead
	// of a misleading Running/Pending, and the UI tooltip can show the
	// waiting message. Only override when the pod has not already Succeeded.
	if clMsg, crashed := mainContainerCrashLoopMessage(k8sPod); crashed && phase != rlarkv1alpha1.PodPhaseSucceeded {
		phase = rlarkv1alpha1.PodPhaseFailed
		message = clMsg
	}

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
		Message: message,
	}

	// Labels enable lookup by k8s pod name/namespace (e.g. for deletion when only the
	// pod name is available, not the UID).
	labels := map[string]string{
		rlarkv1alpha1.PodLabelLocalPodName:      k8sPod.Name,
		rlarkv1alpha1.PodLabelLocalPodNamespace: k8sPod.Namespace,
		rlarkv1alpha1.PodLabelLocalPodUID:       string(k8sPod.UID),
		rlarkv1alpha1.PodLabelTaskName:          taskName,
		rlarkv1alpha1.PodLabelTaskUID:           taskUID,
		rlarkv1alpha1.PodLabelAgentScope:        r.c.ManagementNamespace,
	}
	mgmtPod := &rlarkv1alpha1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      string(k8sPod.UID),
			Namespace: r.c.ManagementNamespace,
			Labels:    labels,
		},
		Spec:   podSpec,
		Status: podStatus,
	}

	var task rlarkv1alpha1.Task
	if err := r.c.ManagementClient.Get(ctx, types.NamespacedName{Name: taskName, Namespace: taskNamespace}, &task); err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("verify management Task identity: %w", err)
		}
		return nil, fmt.Errorf("verify management Task identity: Task %s/%s not found", taskNamespace, taskName)
	}
	if string(task.UID) != taskUID {
		return nil, fmt.Errorf("verify management Task identity: Task %s/%s UID is %q, not %q", taskNamespace, taskName, task.UID, taskUID)
	}
	if podSpec.Domain != "" && task.Spec.Domain != podSpec.Domain {
		return nil, fmt.Errorf("verify management Task identity: Task %s/%s domain is %q, not %q", taskNamespace, taskName, task.Spec.Domain, podSpec.Domain)
	}

	// Cross-namespace owner references are illegal. Identity is still verified
	// above for cross-namespace Tasks, but ownership is only set when legal.
	if taskNamespace == r.c.ManagementNamespace {
		mgmtPod.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(),
			Kind:       "Task",
			Name:       task.Name,
			UID:        task.UID,
			Controller: ptr.To(true),
		}}
	}
	return mgmtPod, nil
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
	if err := validateManagementPodIdentity(&mgmtPod, desiredPod); err != nil {
		return reconcile.Result{}, err
	}

	labels := convergeMetadata(mgmtPod.Labels, desiredPod.Labels, podManagedLabelKeys)
	annotations := convergeMetadata(mgmtPod.Annotations, desiredPod.Annotations, nil)
	ownerReferences := convergeControllerOwner(mgmtPod.OwnerReferences, desiredPod.OwnerReferences)
	if !reflect.DeepEqual(mgmtPod.Labels, labels) || !reflect.DeepEqual(mgmtPod.Annotations, annotations) ||
		!reflect.DeepEqual(mgmtPod.OwnerReferences, ownerReferences) || !reflect.DeepEqual(mgmtPod.Spec, desiredPod.Spec) {
		original := mgmtPod.DeepCopy()
		mgmtPod.Labels = labels
		mgmtPod.Annotations = annotations
		mgmtPod.OwnerReferences = ownerReferences
		mgmtPod.Spec = desiredPod.Spec
		if err := r.c.ManagementClient.Patch(ctx, &mgmtPod, client.MergeFrom(original)); err != nil {
			logger.Error(err, "failed to update management Pod")
			return reconcile.Result{}, err
		}
	}

	if !reflect.DeepEqual(mgmtPod.Status, desiredPod.Status) {
		original := mgmtPod.DeepCopy()
		mgmtPod.Status = desiredPod.Status
		if err := r.c.ManagementClient.Status().Patch(ctx, &mgmtPod, client.MergeFrom(original)); err != nil {
			logger.Error(err, "failed to update management Pod status")
			return reconcile.Result{}, err
		}
	}

	logger.V(1).Info("management Pod reported successfully")
	return reconcile.Result{}, nil
}

func (r *pushPodReconciler) deleteManagementPod(ctx context.Context, logger logr.Logger, uid types.UID) (reconcile.Result, error) {
	var candidate rlarkv1alpha1.Pod
	key := types.NamespacedName{Name: string(uid), Namespace: r.c.ManagementNamespace}
	if err := r.c.ManagementClient.Get(ctx, key, &candidate); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if candidate.Labels[rlarkv1alpha1.PodLabelLocalPodUID] != string(uid) ||
		candidate.Labels[rlarkv1alpha1.PodLabelAgentScope] != r.c.ManagementNamespace {
		return reconcile.Result{}, nil
	}
	if err := r.c.ManagementClient.Delete(ctx, &candidate); err != nil && client.IgnoreNotFound(err) != nil {
		logger.Error(err, "failed to delete management Pod", "managementPod", candidate.Name)
		return reconcile.Result{}, err
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
		return rlarkv1alpha1.PodPhaseUnknown
	}
}

var podManagedLabelKeys = []string{
	rlarkv1alpha1.PodLabelTaskName,
	rlarkv1alpha1.PodLabelTaskUID,
	rlarkv1alpha1.PodLabelLocalPodName,
	rlarkv1alpha1.PodLabelLocalPodNamespace,
	rlarkv1alpha1.PodLabelLocalPodUID,
	rlarkv1alpha1.PodLabelAgentScope,
}

func convergeMetadata(existing, managed map[string]string, managedKeys []string) map[string]string {
	if existing == nil && managed == nil {
		return nil
	}
	merged := make(map[string]string, len(existing)+len(managed))
	for key, value := range existing {
		merged[key] = value
	}
	for _, key := range managedKeys {
		delete(merged, key)
	}
	for key, value := range managed {
		if value == "" {
			delete(merged, key)
		} else {
			merged[key] = value
		}
	}
	return merged
}

func convergeControllerOwner(existing, desired []metav1.OwnerReference) []metav1.OwnerReference {
	owners := make([]metav1.OwnerReference, 0, len(existing)+len(desired))
	for _, owner := range existing {
		if owner.Controller == nil || !*owner.Controller {
			owners = append(owners, owner)
		}
	}
	owners = append(owners, desired...)
	if len(owners) == 0 {
		return nil
	}
	return owners
}

// mainContainerCrashLoopMessage inspects the workload's main container (the
// container named "main") of a K8s Pod. When it is in the CrashLoopBackOff
// waiting state, it returns the waiting message (falling back to the reason)
// and true so the management Pod CR can be surfaced as Failed. This augments
// the raw pod phase: a Pod can be phase=Running while its main container is
// stuck in CrashLoopBackOff.
func mainContainerCrashLoopMessage(k8sPod *corev1.Pod) (string, bool) {
	for _, cs := range k8sPod.Status.ContainerStatuses {
		if cs.Name != "main" {
			continue
		}
		if waiting := cs.State.Waiting; waiting != nil && waiting.Reason == "CrashLoopBackOff" {
			msg := waiting.Message
			if msg == "" {
				msg = waiting.Reason
			}
			return msg, true
		}
	}
	return "", false
}
