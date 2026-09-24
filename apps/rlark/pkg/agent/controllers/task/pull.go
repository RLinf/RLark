package task

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// Constants used by the package.
const (
	ManagementTaskNameAnnotation              = "rlark.io/management-task-name"
	ManagementTaskNamespaceAnnotation         = "rlark.io/management-task-namespace"
	ManagementTaskGenerationAnnotation        = "rlark.io/management-task-generation"
	ManagementTaskAdoptedGenerationAnnotation = "rlark.io/management-task-adopted-generation"
	ManagementTaskUIDAnnotation               = "rlark.io/management-task-uid"
	ManagementTaskDomainAnnotation            = "rlark.io/management-task-domain"
	ManagementTaskClaimantAnnotation          = "rlark.io/management-task-claimant"
	ManagementTaskClaimedDomainAnnotation     = "rlark.io/management-task-claimed-domain"
	ManagementTaskUIDLabel                    = "rlark.io/management-task-uid"
	ManagementTaskFinalizer                   = "rlark.io/agent-cleanup"
	PVCTaskLabel                              = "rlark.io/task"
	PVCOwnerAnnotation                        = "rlark.io/pvc-owner"
	PVCOwnerTaskAnnotation                    = "rlark.io/pvc-owner-task"
	RestartedAtAnnotation                     = "rlark.io/restarted-at"
	StoppedAnnotation                         = "rlark.io/stopped"
	CleanupRequeueInterval                    = 2 * time.Second
	UnsafeTaskPrivilegesEnv                   = "RLARK_ENABLE_UNSAFE_TASK_PRIVILEGES"
	WorkloadProgressingCondition              = "Progressing"
	WorkloadKindMigrationReason               = "WorkloadKindMigration"
)

// pullReconciler watches management Tasks and creates workloads on local cluster.
type pullReconciler struct {
	c *Controller
}

type workloadOwnership int

const (
	workloadConflict workloadOwnership = iota
	workloadLegacyOwned
	workloadOwned
)

// Reconcile reconciles the resource.
func (r *pullReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("task", req.NamespacedName)

	var mgmtTask rlarkv1alpha1.Task
	if err := r.c.ManagementClient.Get(ctx, req.NamespacedName, &mgmtTask); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// A recreated Task may have the same name. Without the deleted Task UID,
			// name-only cleanup cannot prove ownership and is therefore unsafe.
			logger.Info("management Task deleted; skipping unverified name-only cleanup")
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	claimed := slices.Contains(mgmtTask.Finalizers, ManagementTaskFinalizer)
	claimantMissing := claimed && mgmtTask.Annotations[ManagementTaskClaimantAnnotation] == ""
	isClaimant := claimed && (claimantMissing || r.isTaskClaimant(&mgmtTask))
	if claimantMissing && (mgmtTask.DeletionTimestamp != nil || r.canRealize(&mgmtTask)) {
		if mgmtTask.Annotations == nil {
			mgmtTask.Annotations = map[string]string{}
		}
		mgmtTask.Annotations[ManagementTaskClaimantAnnotation] = r.claimantIdentity()
		mgmtTask.Annotations[ManagementTaskClaimedDomainAnnotation] = mgmtTask.Spec.Domain
		if err := r.c.ManagementClient.Update(ctx, &mgmtTask); err != nil {
			logger.Error(err, "failed to persist Task claim")
			return reconcile.Result{}, err
		}
	}
	if mgmtTask.DeletionTimestamp != nil || (isClaimant && !r.canRealize(&mgmtTask)) {
		if !isClaimant {
			logger.Info("Task cleanup is claimed by another agent, skipping")
			return reconcile.Result{}, nil
		}
		logger.Info("management Task being deleted, cleaning up local workload")
		workloadNs := getWorkloadNamespace(&mgmtTask)
		pending, err := r.cleanupWorkload(ctx, &mgmtTask, workloadNs)
		if err != nil {
			logger.Error(err, "failed to clean up workload")
			return reconcile.Result{}, err
		}
		if pending {
			return reconcile.Result{RequeueAfter: CleanupRequeueInterval}, nil
		}
		mgmtTask.Finalizers = slices.DeleteFunc(mgmtTask.Finalizers, func(s string) bool {
			return s == ManagementTaskFinalizer
		})
		delete(mgmtTask.Annotations, ManagementTaskClaimantAnnotation)
		delete(mgmtTask.Annotations, ManagementTaskClaimedDomainAnnotation)
		if err := r.c.ManagementClient.Update(ctx, &mgmtTask); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Compare mgmtTask.Spec.AgentType with controller AgentType — skip if mismatch
	if mgmtTask.Spec.AgentType != rlarkv1alpha1.AgentType(r.c.AgentType) {
		logger.Info(fmt.Sprintf("Task AgentType %s does not match controller AgentType %s, skipping", mgmtTask.Spec.AgentType, r.c.AgentType))
		return reconcile.Result{}, nil
	}

	if mgmtTask.Spec.Kubernetes == nil || mgmtTask.Spec.Kubernetes.Workload == nil {
		logger.Info("Task has no Kubernetes workload spec, skipping")
		return reconcile.Result{}, nil
	}
	if !validWorkloadKind(mgmtTask.Spec.Kubernetes.Workload.Kind) {
		logger.Info("Task has unsupported Kubernetes workload kind, skipping")
		return reconcile.Result{}, nil
	}

	// Only claim Tasks this controller can actually realize. Existing finalizers
	// without ownership metadata predate claimant annotations and are adopted by
	// the responsible agent before any normal work or cleanup.
	if !claimed {
		if mgmtTask.Annotations == nil {
			mgmtTask.Annotations = map[string]string{}
		}
		mgmtTask.Annotations[ManagementTaskClaimantAnnotation] = r.claimantIdentity()
		mgmtTask.Annotations[ManagementTaskClaimedDomainAnnotation] = mgmtTask.Spec.Domain
		if !claimed {
			mgmtTask.Finalizers = append(mgmtTask.Finalizers, ManagementTaskFinalizer)
		}
		if err := r.c.ManagementClient.Update(ctx, &mgmtTask); err != nil {
			logger.Error(err, "failed to persist Task claim")
			return reconcile.Result{}, err
		}
	} else if !r.isTaskClaimant(&mgmtTask) {
		logger.Info("Task is claimed by another agent, skipping")
		return reconcile.Result{}, nil
	}

	workloadSpec := mgmtTask.Spec.Kubernetes.Workload
	workloadNamespace := getWorkloadNamespace(&mgmtTask)
	restarting, err := r.restartCleanupRequired(ctx, &mgmtTask, workloadNamespace)
	if err != nil {
		return reconcile.Result{}, err
	}
	if mgmtTask.Annotations[StoppedAnnotation] != "true" &&
		(restarting || mgmtTask.Status.Phase == rlarkv1alpha1.TaskPhaseStopped) &&
		mgmtTask.Status.Phase != rlarkv1alpha1.TaskPhasePending {
		return updateMgmtTaskStatus(ctx, logger, r.c.ManagementClient, &mgmtTask, rlarkv1alpha1.TaskPhasePending, "", nil)
	}
	if mgmtTask.Annotations[StoppedAnnotation] == "true" || restarting {
		pending, err := r.cleanupWorkload(ctx, &mgmtTask, workloadNamespace)
		if err != nil {
			return reconcile.Result{}, err
		}
		if pending {
			return reconcile.Result{RequeueAfter: CleanupRequeueInterval}, nil
		}
		if mgmtTask.Annotations[StoppedAnnotation] == "true" {
			return updateMgmtTaskStatus(ctx, logger, r.c.ManagementClient, &mgmtTask, rlarkv1alpha1.TaskPhaseStopped, "", nil)
		}
	}

	applyTemplateMutations(&workloadSpec.Template, &mgmtTask, r.c.Image)
	applyWorkloadIdentity(&workloadSpec.Template, &mgmtTask)

	if mgmtTask.Status.Phase == rlarkv1alpha1.TaskPhaseRunning {
		if mgmtTask.Spec.SSHPublicKey != "" {
			if err := r.appendSSHPublicKeyToRunningPods(ctx, &mgmtTask, workloadNamespace, workloadSpec.Template.Labels); err != nil {
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	if err := r.ensureImagePullSecrets(ctx, &workloadSpec.Template, workloadNamespace); err != nil {
		return reconcile.Result{}, fmt.Errorf("ensure image pull secrets: %w", err)
	}

	if err := r.ensurePVCs(ctx, &mgmtTask, workloadSpec); err != nil {
		logger.Error(err, "failed to ensure PVCs")
		return reconcile.Result{}, err
	}

	if err := r.ensureRayResources(ctx, &mgmtTask, nil); err != nil {
		logger.Error(err, "failed to ensure ray resources")
		return reconcile.Result{}, err
	}

	switch workloadSpec.Kind {
	case rlarkv1alpha1.KubernetesWorkloadDeployment:
		return r.createOrUpdateDeployment(ctx, &mgmtTask, workloadSpec)
	case rlarkv1alpha1.KubernetesWorkloadDaemonSet:
		return r.createOrUpdateDaemonSet(ctx, &mgmtTask, workloadSpec)
	case rlarkv1alpha1.KubernetesWorkloadStatefulSet:
		return r.createOrUpdateStatefulSet(ctx, &mgmtTask, workloadSpec)
	default:
		logger.Info(fmt.Sprintf("unknown workload kind: %s, skipping", workloadSpec.Kind))
		return reconcile.Result{}, nil
	}
}

func (r *pullReconciler) canRealize(task *rlarkv1alpha1.Task) bool {
	return task.Spec.AgentType == rlarkv1alpha1.AgentType(r.c.AgentType) && task.Spec.Domain == r.claimedDomain(task) && task.Spec.Kubernetes != nil &&
		task.Spec.Kubernetes.Workload != nil && validWorkloadKind(task.Spec.Kubernetes.Workload.Kind)
}

// createOrUpdateWorkload is a generic helper that handles the create-or-update pattern for all workload types.
// existingObj is a pre-allocated empty object for Get, newObj is the fully-built object for Create,
// applyUpdate is a callback that applies spec changes to the existing object when the management Task's
// generation has changed (indicating the Task spec was updated).
func (r *pullReconciler) createOrUpdateWorkload(
	ctx context.Context,
	mgmtTask *rlarkv1alpha1.Task,
	workloadKind string,
	existingObj client.Object,
	newObj client.Object,
	applyUpdate func(existingObj, desiredObj client.Object),
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	key := types.NamespacedName{Name: newObj.GetName(), Namespace: newObj.GetNamespace()}

	pending, err := r.deleteOtherWorkloadKinds(ctx, key, workloadKind, mgmtTask)
	if err != nil {
		return reconcile.Result{}, err
	}
	if pending {
		return reconcile.Result{RequeueAfter: CleanupRequeueInterval}, nil
	}

	err = r.c.LocalKubeClient.Get(ctx, key, existingObj)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, err
	}

	if err != nil {
		logger.Info(fmt.Sprintf("creating %s", workloadKind))
		if err := r.c.LocalKubeClient.Create(ctx, newObj); err != nil {
			logger.Error(err, fmt.Sprintf("failed to create %s", workloadKind))
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, r.clearWorkloadKindMigration(ctx, mgmtTask)
	}

	ownership := workloadOwnershipForTask(existingObj, mgmtTask)
	if ownership == workloadConflict {
		return reconcile.Result{}, fmt.Errorf("%s %s is not owned by Task UID %q", workloadKind, key, mgmtTask.UID)
	}
	selectorChanged := workloadSelectorChanged(existingObj, newObj)
	if selectorChanged && !workloadSelectorIdentityOnlyMigration(existingObj, newObj) {
		return reconcile.Result{}, fmt.Errorf("%s %s has an immutable selector that differs from the desired selector", workloadKind, key)
	}
	annotations := existingObj.GetAnnotations()
	missingGenerationBaseline := annotations[ManagementTaskGenerationAnnotation] == "" && annotations[ManagementTaskAdoptedGenerationAnnotation] == ""
	if ownership == workloadLegacyOwned || (selectorChanged && missingGenerationBaseline) {
		adoptWorkloadAnnotations(existingObj, mgmtTask)
		if err := r.c.LocalKubeClient.Update(ctx, existingObj); err != nil {
			return reconcile.Result{}, fmt.Errorf("backfill %s identity metadata: %w", workloadKind, err)
		}
		return reconcile.Result{}, r.clearWorkloadKindMigration(ctx, mgmtTask)
	}

	annotations = existingObj.GetAnnotations()
	if annotations == nil || (annotations[ManagementTaskGenerationAnnotation] != taskGeneration(mgmtTask) && annotations[ManagementTaskAdoptedGenerationAnnotation] != taskGeneration(mgmtTask)) {
		logger.Info(fmt.Sprintf("%s spec changed (Task generation mismatch), updating", workloadKind))
		if err := prepareWorkloadTemplateUpdate(existingObj, newObj); err != nil {
			return reconcile.Result{}, fmt.Errorf("update %s %s: %w", workloadKind, key, err)
		}
		applyUpdate(existingObj, newObj)
		annotations := existingObj.GetAnnotations()
		delete(annotations, ManagementTaskAdoptedGenerationAnnotation)
		existingObj.SetAnnotations(annotations)
		if err := r.c.LocalKubeClient.Update(ctx, existingObj); err != nil {
			logger.Error(err, fmt.Sprintf("failed to update %s", workloadKind))
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, r.clearWorkloadKindMigration(ctx, mgmtTask)
}

func validWorkloadKind(kind rlarkv1alpha1.KubernetesWorkloadKind) bool {
	switch kind {
	case rlarkv1alpha1.KubernetesWorkloadDeployment, rlarkv1alpha1.KubernetesWorkloadDaemonSet, rlarkv1alpha1.KubernetesWorkloadStatefulSet:
		return true
	default:
		return false
	}
}

func (r *pullReconciler) deleteOtherWorkloadKinds(ctx context.Context, key types.NamespacedName, wantKind string, task *rlarkv1alpha1.Task) (bool, error) {
	for kind, obj := range map[string]client.Object{
		"Deployment": &appsv1.Deployment{}, "DaemonSet": &appsv1.DaemonSet{}, "StatefulSet": &appsv1.StatefulSet{},
	} {
		if kind == wantKind {
			continue
		}
		err := r.c.LocalKubeClient.Get(ctx, key, obj)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if workloadOwnershipForTask(obj, task) == workloadConflict {
			return false, fmt.Errorf("cannot migrate Task %s: conflicting %s %s is not owned by Task UID %q", client.ObjectKeyFromObject(task), kind, key, task.UID)
		}
		if err := r.markWorkloadKindMigration(ctx, task, kind, wantKind); err != nil {
			return false, err
		}
		if obj.GetDeletionTimestamp().IsZero() {
			if err := r.c.LocalKubeClient.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !errors.IsNotFound(err) {
				return false, err
			}
		}
		return true, nil
	}
	return false, nil
}

func (r *pullReconciler) markWorkloadKindMigration(ctx context.Context, task *rlarkv1alpha1.Task, fromKind, toKind string) error {
	desired := task.DeepCopy()
	desired.Status.Phase = rlarkv1alpha1.TaskPhasePending
	desired.Status.ObservedNodes = nil
	apimeta.SetStatusCondition(&desired.Status.Conditions, metav1.Condition{
		Type:               WorkloadProgressingCondition,
		Status:             metav1.ConditionTrue,
		Reason:             WorkloadKindMigrationReason,
		Message:            fmt.Sprintf("Migrating workload from %s to %s", fromKind, toKind),
		ObservedGeneration: task.Generation,
	})
	if reflect.DeepEqual(task.Status, desired.Status) {
		return nil
	}
	if r.c.ManagementClient == nil {
		task.Status = desired.Status
		return nil
	}
	if err := r.c.ManagementClient.Status().Patch(ctx, desired, client.MergeFrom(task)); err != nil {
		return fmt.Errorf("mark Task workload kind migration: %w", err)
	}
	task.Status = desired.Status
	return nil
}

func (r *pullReconciler) clearWorkloadKindMigration(ctx context.Context, task *rlarkv1alpha1.Task) error {
	condition := apimeta.FindStatusCondition(task.Status.Conditions, WorkloadProgressingCondition)
	if condition == nil || condition.Reason != WorkloadKindMigrationReason {
		return nil
	}
	desired := task.DeepCopy()
	apimeta.RemoveStatusCondition(&desired.Status.Conditions, WorkloadProgressingCondition)
	if r.c.ManagementClient == nil {
		task.Status = desired.Status
		return nil
	}
	if err := r.c.ManagementClient.Status().Patch(ctx, desired, client.MergeFrom(task)); err != nil {
		return fmt.Errorf("clear Task workload kind migration: %w", err)
	}
	task.Status = desired.Status
	return nil
}

func workloadOwnershipForTask(obj client.Object, task *rlarkv1alpha1.Task) workloadOwnership {
	a := obj.GetAnnotations()
	if uid := a[ManagementTaskUIDAnnotation]; uid != "" {
		if task.UID == "" || uid != string(task.UID) {
			return workloadConflict
		}
	} else {
		if name := a[ManagementTaskNameAnnotation]; name != "" && name != task.Name {
			return workloadConflict
		}
		if namespace := a[ManagementTaskNamespaceAnnotation]; namespace != "" && namespace != task.Namespace {
			return workloadConflict
		}
		// Pre-identity workloads may have no management annotations. Their
		// deterministic name and namespace are the only available identity, so
		// accept them unless stronger metadata contradicts the current Task.
		return workloadLegacyOwned
	}
	if name := a[ManagementTaskNameAnnotation]; name != "" && name != task.Name {
		return workloadConflict
	}
	if namespace := a[ManagementTaskNamespaceAnnotation]; namespace != "" && namespace != task.Namespace {
		return workloadConflict
	}
	return workloadOwned
}

func (r *pullReconciler) claimantIdentity() string {
	return r.c.ManagementNamespace + "/" + r.c.AgentType
}

func (r *pullReconciler) isTaskClaimant(task *rlarkv1alpha1.Task) bool {
	claimant := task.Annotations[ManagementTaskClaimantAnnotation]
	if claimant == r.claimantIdentity() {
		return true
	}
	// The previous implementation appended the claimed domain to the stable
	// namespace/runtime scope. Accept it without consulting leader identity.
	parts := strings.Split(claimant, "/")
	return len(parts) == 3 && parts[0] == r.c.ManagementNamespace && parts[1] == r.c.AgentType
}

func (r *pullReconciler) claimedDomain(task *rlarkv1alpha1.Task) string {
	if domain, ok := task.Annotations[ManagementTaskClaimedDomainAnnotation]; ok {
		return domain
	}
	parts := strings.Split(task.Annotations[ManagementTaskClaimantAnnotation], "/")
	if len(parts) == 3 {
		return parts[2]
	}
	return task.Spec.Domain
}

func workloadSelectorChanged(existing, desired client.Object) bool {
	return !reflect.DeepEqual(workloadLabelSelector(existing), workloadLabelSelector(desired))
}

func workloadLabelSelector(obj client.Object) *metav1.LabelSelector {
	switch o := obj.(type) {
	case *appsv1.Deployment:
		return o.Spec.Selector
	case *appsv1.StatefulSet:
		return o.Spec.Selector
	case *appsv1.DaemonSet:
		return o.Spec.Selector
	default:
		return nil
	}
}

func workloadTemplate(obj client.Object) corev1.PodTemplateSpec {
	switch workload := obj.(type) {
	case *appsv1.Deployment:
		return workload.Spec.Template
	case *appsv1.DaemonSet:
		return workload.Spec.Template
	case *appsv1.StatefulSet:
		return workload.Spec.Template
	default:
		return corev1.PodTemplateSpec{}
	}
}

// workloadSelectorIdentityOnlyMigration identifies selectors that differ only
// because the desired workload adds the Task UID identity selector.
func workloadSelectorIdentityOnlyMigration(existing, desired client.Object) bool {
	existingSelector := workloadLabelSelector(existing)
	desiredSelector := workloadLabelSelector(desired)
	if existingSelector == nil || desiredSelector == nil {
		return false
	}
	uid := desiredSelector.MatchLabels[ManagementTaskUIDLabel]
	if uid == "" || len(desiredSelector.MatchLabels) != 1 || len(desiredSelector.MatchExpressions) != 0 ||
		existingSelector.MatchLabels[ManagementTaskUIDLabel] != "" {
		return false
	}
	for _, expression := range existingSelector.MatchExpressions {
		if expression.Key == ManagementTaskUIDLabel {
			return false
		}
	}
	_, err := metav1.LabelSelectorAsSelector(existingSelector)
	return err == nil
}

func prepareWorkloadTemplateUpdate(existing, desired client.Object) error {
	selector := workloadLabelSelector(existing)
	if selector == nil {
		return nil
	}
	template := workloadTemplate(desired)
	if template.Labels == nil {
		template.Labels = map[string]string{}
	}
	for key, value := range selector.MatchLabels {
		template.Labels[key] = value
	}
	parsed, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return fmt.Errorf("invalid existing selector: %w", err)
	}
	if !parsed.Matches(labels.Set(template.Labels)) {
		return fmt.Errorf("desired pod template labels do not satisfy existing selector")
	}
	setWorkloadTemplate(desired, template)
	return nil
}

func setWorkloadTemplate(obj client.Object, template corev1.PodTemplateSpec) {
	switch workload := obj.(type) {
	case *appsv1.Deployment:
		workload.Spec.Template = template
	case *appsv1.DaemonSet:
		workload.Spec.Template = template
	case *appsv1.StatefulSet:
		workload.Spec.Template = template
	}
}

func (r *pullReconciler) appendSSHPublicKeyToRunningPods(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, namespace string, podLabels map[string]string) error {
	if r.c.LocalKubeConfig == nil {
		return fmt.Errorf("local Kubernetes config is required to update SSH authorized keys")
	}

	var pods corev1.PodList
	if err := r.c.LocalKubeClient.List(ctx, &pods, client.InNamespace(namespace), client.MatchingLabels(podLabels)); err != nil {
		return fmt.Errorf("list running task Pods: %w", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning || !containerRunning(&pod, "main") {
			continue
		}
		if err := appendSSHPublicKeyToPod(ctx, r.c.LocalKubeConfig, pod.Namespace, pod.Name, mgmtTask.Spec.SSHPublicKey); err != nil {
			return fmt.Errorf("append SSH public key to Pod %s: %w", pod.Name, err)
		}
	}
	return nil
}

func containerRunning(pod *corev1.Pod, container string) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == container {
			return status.State.Running != nil
		}
	}
	return false
}

func appendSSHPublicKeyToPod(ctx context.Context, kubeConfig *rest.Config, namespace, podName, publicKey string) error {
	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}

	execReq := kubeClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("exec").
		Param("container", "main").
		Param("command", "sh").
		Param("command", "-c").
		Param("command", "mkdir -p /root/.ssh && cat >> /root/.ssh/authorized_keys").
		Param("stdin", "true").
		Param("stdout", "true").
		Param("stderr", "true").
		Param("tty", "false")

	executor, err := remotecommand.NewSPDYExecutor(kubeConfig, "POST", execReq.URL())
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}

	var stderr bytes.Buffer
	if err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  strings.NewReader(publicKey + "\n"),
		Stdout: io.Discard,
		Stderr: &stderr,
		Tty:    false,
	}); err != nil {
		return fmt.Errorf("exec stream: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

func (r *pullReconciler) restartCleanupRequired(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, namespace string) (bool, error) {
	restartedAt := mgmtTask.Annotations[RestartedAtAnnotation]
	if restartedAt == "" {
		return false, nil
	}
	key := types.NamespacedName{Name: mgmtTask.Name, Namespace: namespace}
	for _, obj := range []client.Object{&appsv1.Deployment{}, &appsv1.DaemonSet{}, &appsv1.StatefulSet{}} {
		err := r.c.LocalKubeClient.Get(ctx, key, obj)
		if err == nil {
			return obj.GetAnnotations()[RestartedAtAnnotation] != restartedAt, nil
		}
		if !errors.IsNotFound(err) {
			return false, err
		}
	}
	var pvcs corev1.PersistentVolumeClaimList
	if err := r.c.LocalKubeClient.List(ctx, &pvcs, client.InNamespace(namespace), client.MatchingLabels{PVCTaskLabel: mgmtTask.Name}); err != nil {
		return false, err
	}
	return len(pvcs.Items) > 0, nil
}

func (r *pullReconciler) createOrUpdateDeployment(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) (reconcile.Result, error) {
	return r.createOrUpdateWorkload(ctx, mgmtTask, "Deployment",
		&appsv1.Deployment{},
		buildDeployment(mgmtTask, spec),
		func(obj, desired client.Object) {
			deploy := obj.(*appsv1.Deployment)
			mergeWorkloadAnnotations(deploy, mgmtTask)
			deploy.Spec.Replicas = spec.Replicas
			deploy.Spec.Template = desired.(*appsv1.Deployment).Spec.Template
		})
}

func (r *pullReconciler) createOrUpdateDaemonSet(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) (reconcile.Result, error) {
	return r.createOrUpdateWorkload(ctx, mgmtTask, "DaemonSet",
		&appsv1.DaemonSet{},
		buildDaemonSet(mgmtTask, spec),
		func(obj, desired client.Object) {
			ds := obj.(*appsv1.DaemonSet)
			mergeWorkloadAnnotations(ds, mgmtTask)
			ds.Spec.Template = desired.(*appsv1.DaemonSet).Spec.Template
		})
}

func (r *pullReconciler) createOrUpdateStatefulSet(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) (reconcile.Result, error) {
	return r.createOrUpdateWorkload(ctx, mgmtTask, "StatefulSet",
		&appsv1.StatefulSet{},
		buildStatefulSet(mgmtTask, spec),
		func(obj, desired client.Object) {
			sts := obj.(*appsv1.StatefulSet)
			mergeWorkloadAnnotations(sts, mgmtTask)
			sts.Spec.Replicas = spec.Replicas
			sts.Spec.Template = desired.(*appsv1.StatefulSet).Spec.Template
		})
}

func (r *pullReconciler) cleanupWorkload(ctx context.Context, task *rlarkv1alpha1.Task, namespace string) (bool, error) {
	if r.c.LocalKubeClient == nil {
		return false, nil
	}
	pending := false
	workloadKey := types.NamespacedName{Name: task.Name, Namespace: namespace}

	for _, obj := range []client.Object{&appsv1.Deployment{}, &appsv1.DaemonSet{}, &appsv1.StatefulSet{}} {
		err := r.c.LocalKubeClient.Get(ctx, workloadKey, obj)
		if err != nil && !errors.IsNotFound(err) {
			return false, err
		}
		if err == nil {
			if workloadOwnershipForTask(obj, task) == workloadConflict {
				continue
			}
			pending = true
			if obj.GetDeletionTimestamp().IsZero() {
				if err := r.c.LocalKubeClient.Delete(ctx, obj, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !errors.IsNotFound(err) {
					return false, err
				}
			}
		}
	}

	svcKey := types.NamespacedName{Name: rayHeadServiceName(task.Name), Namespace: namespace}
	var svc corev1.Service
	if err := r.c.LocalKubeClient.Get(ctx, svcKey, &svc); err == nil && svc.DeletionTimestamp.IsZero() {
		if err := r.c.LocalKubeClient.Delete(ctx, &svc); err != nil && !errors.IsNotFound(err) {
			return false, err
		}
	} else if err != nil && !errors.IsNotFound(err) {
		return false, err
	}

	if pending {
		return true, nil
	}

	pvcsPending, err := r.cleanupPVCs(ctx, task, namespace)
	if err != nil {
		return false, err
	}
	return pvcsPending, nil
}

func (r *pullReconciler) ensureRayResources(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, owner client.Object) error {
	annotations := mgmtTask.Annotations
	if annotations == nil || annotations[rlarkv1alpha1.RayRoleAnnotation] == "" {
		return nil
	}
	role := annotations[rlarkv1alpha1.RayRoleAnnotation]
	namespace := getWorkloadNamespace(mgmtTask)

	cm := buildRayConfigMap(namespace, role)
	if owner != nil {
		if err := ctrl.SetControllerReference(owner, cm, controllers.MgmtScheme); err != nil {
			return fmt.Errorf("set owner reference on ConfigMap %s: %w", cm.Name, err)
		}
	}
	var existingCM corev1.ConfigMap
	err := r.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: cm.Name, Namespace: cm.Namespace}, &existingCM)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("get ray ConfigMap %s: %w", cm.Name, err)
	}
	if err != nil {
		if err := r.c.LocalKubeClient.Create(ctx, cm); err != nil {
			return fmt.Errorf("create ray ConfigMap %s: %w", cm.Name, err)
		}
	} else {
		ownerReferencesChanged := owner != nil && !reflect.DeepEqual(existingCM.OwnerReferences, cm.OwnerReferences)
		if !reflect.DeepEqual(existingCM.Data, cm.Data) || ownerReferencesChanged {
			existingCM.Data = cm.Data
			if owner != nil {
				existingCM.OwnerReferences = cm.OwnerReferences
			}
			if err := r.c.LocalKubeClient.Update(ctx, &existingCM); err != nil {
				return fmt.Errorf("update ray ConfigMap %s: %w", cm.Name, err)
			}
		}
	}

	if role == rlarkv1alpha1.RayRoleHead {
		svc := buildRayHeadService(namespace, mgmtTask.Name)
		if owner != nil {
			if err := ctrl.SetControllerReference(owner, svc, controllers.MgmtScheme); err != nil {
				return fmt.Errorf("set owner reference on Service %s: %w", svc.Name, err)
			}
		}
		var existingSvc corev1.Service
		err := r.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: svc.Name, Namespace: svc.Namespace}, &existingSvc)
		if err != nil && client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("get ray head Service %s: %w", svc.Name, err)
		}
		if err != nil {
			if err := r.c.LocalKubeClient.Create(ctx, svc); err != nil {
				return fmt.Errorf("create ray head Service %s: %w", svc.Name, err)
			}
		}
	}

	return nil
}

func (r *pullReconciler) ensurePVCs(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, workloadSpec *rlarkv1alpha1.KubernetesWorkloadSpec) error {
	namespace := getWorkloadNamespace(mgmtTask)
	logger := log.FromContext(ctx).WithValues("task", mgmtTask.Name)

	for i := range workloadSpec.Template.Spec.Volumes {
		vol := &workloadSpec.Template.Spec.Volumes[i]
		if vol.PersistentVolumeClaim == nil {
			continue
		}

		claimName := vol.PersistentVolumeClaim.ClaimName
		if claimName == "" {
			claimName = pvcNameForVolume(mgmtTask.Name, vol.Name)
			vol.PersistentVolumeClaim.ClaimName = claimName
			logger.Info("PVC volume has no claim name, using generated name", "volume", vol.Name, "pvc", claimName)
		}

		storageClassName := ""
		//nolint:staticcheck // Support for deprecated PVC storage class mapping
		if workloadSpec.PvcStorageMap != nil {
			storageClassName = workloadSpec.PvcStorageMap[claimName]
		}
		pvcSizeGb := int32(10)
		//nolint:staticcheck // Support for deprecated PVC storage class mapping
		if workloadSpec.PvcSizeGbMap != nil && workloadSpec.PvcSizeGbMap[claimName] > 0 {
			pvcSizeGb = workloadSpec.PvcSizeGbMap[claimName]
		}

		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      claimName,
				Namespace: namespace,
				Labels: map[string]string{
					PVCTaskLabel: mgmtTask.Name,
				},
				Annotations: map[string]string{
					PVCOwnerAnnotation:     mgmtTask.Name,
					PVCOwnerTaskAnnotation: mgmtTask.Name,
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(fmt.Sprintf("%dGi", int(pvcSizeGb))),
					},
				},
			},
		}

		if storageClassName != "" {
			pvc.Spec.StorageClassName = &storageClassName
		} else {
			logger.Info("No valid storage class name provided for claim", "claim", claimName)
		}

		existing := &corev1.PersistentVolumeClaim{}
		err := r.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: claimName, Namespace: namespace}, existing)
		if err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("get PVC %s: %w", claimName, err)
		}
		if errors.IsNotFound(err) {
			if storageClassName == "" {
				logger.Info("Creating PVC with default storage class", "pvc", claimName)
			} else {
				logger.Info("Creating PVC", "pvc", claimName, "storageClass", storageClassName)
			}
			if err := r.c.LocalKubeClient.Create(ctx, pvc); err != nil {
				return fmt.Errorf("create PVC %s: %w", claimName, err)
			}
		} else {
			if storageClassName != "" && (existing.Spec.StorageClassName == nil || *existing.Spec.StorageClassName != storageClassName) {
				existingClass := ""
				if existing.Spec.StorageClassName != nil {
					existingClass = *existing.Spec.StorageClassName
				}
				return fmt.Errorf("PVC %s storageClassName is immutable: existing %q, requested %q", claimName, existingClass, storageClassName)
			}
		}
	}

	return nil
}

func (r *pullReconciler) cleanupPVCs(ctx context.Context, task *rlarkv1alpha1.Task, namespace string) (bool, error) {
	if r.c.LocalKubeClient == nil {
		return false, nil
	}

	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.c.LocalKubeClient.List(ctx, pvcList, client.InNamespace(namespace), client.MatchingLabels{PVCTaskLabel: task.Name}); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("list PVCs for task %s: %w", task.Name, err)
	}

	if len(pvcList.Items) == 0 {
		var allPVCs corev1.PersistentVolumeClaimList
		if err := r.c.LocalKubeClient.List(ctx, &allPVCs, client.InNamespace(namespace)); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("list all PVCs in namespace %s: %w", namespace, err)
		}
		for _, pvc := range allPVCs.Items {
			if pvc.Annotations != nil && pvc.Annotations[PVCOwnerTaskAnnotation] == task.Name {
				pvcList.Items = append(pvcList.Items, pvc)
			}
		}
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		logger := log.FromContext(ctx).WithValues("pvc", pvc.Name)
		logger.Info("Deleting PVC owned by task", "pvc", pvc.Name, "task", task.Name)
		if pvc.DeletionTimestamp.IsZero() {
			if err := r.c.LocalKubeClient.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
				return false, fmt.Errorf("delete PVC %s: %w", pvc.Name, err)
			}
		}
	}

	return len(pvcList.Items) > 0, nil
}

func pvcNameForVolume(taskName, volumeName string) string {
	return fmt.Sprintf("pvc-%s-%s", taskName, volumeName)
}

func getWorkloadNamespace(mgmtTask *rlarkv1alpha1.Task) string {
	_ = mgmtTask
	return "rlark-system"
}

// --- workload builder functions ---

func workloadAnnotations(mgmtTask *rlarkv1alpha1.Task) map[string]string {
	annotations := map[string]string{
		ManagementTaskNameAnnotation:       mgmtTask.Name,
		ManagementTaskNamespaceAnnotation:  mgmtTask.Namespace,
		ManagementTaskUIDAnnotation:        string(mgmtTask.UID),
		ManagementTaskGenerationAnnotation: taskGeneration(mgmtTask),
	}
	if mgmtTask.Spec.Domain != "" {
		annotations[ManagementTaskDomainAnnotation] = mgmtTask.Spec.Domain
	}
	if restartedAt := mgmtTask.Annotations[RestartedAtAnnotation]; restartedAt != "" {
		annotations[RestartedAtAnnotation] = restartedAt
	}
	return annotations
}

func mergeWorkloadAnnotations(obj client.Object, mgmtTask *rlarkv1alpha1.Task) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	for key, value := range workloadAnnotations(mgmtTask) {
		annotations[key] = value
	}
	if mgmtTask.Annotations[RestartedAtAnnotation] == "" {
		delete(annotations, RestartedAtAnnotation)
	}
	obj.SetAnnotations(annotations)
}

func adoptWorkloadAnnotations(obj client.Object, mgmtTask *rlarkv1alpha1.Task) {
	mergeWorkloadAnnotations(obj, mgmtTask)
	annotations := obj.GetAnnotations()
	delete(annotations, ManagementTaskGenerationAnnotation)
	annotations[ManagementTaskAdoptedGenerationAnnotation] = taskGeneration(mgmtTask)
	obj.SetAnnotations(annotations)
}

func taskGeneration(mgmtTask *rlarkv1alpha1.Task) string {
	return strconv.FormatInt(mgmtTask.Generation, 10)
}

// ensureLabels ensures the pod template has labels, adding a default if none are set.
func ensureLabels(template *corev1.PodTemplateSpec, name string) {
	if template == nil || len(template.Labels) == 0 {
		template.Labels = map[string]string{
			"app": name,
		}
	}
}

func applyWorkloadIdentity(template *corev1.PodTemplateSpec, mgmtTask *rlarkv1alpha1.Task) {
	if template.Labels == nil {
		template.Labels = map[string]string{}
	}
	template.Labels[ManagementTaskUIDLabel] = string(mgmtTask.UID)
	if template.Annotations == nil {
		template.Annotations = map[string]string{}
	}
	template.Annotations[ManagementTaskUIDAnnotation] = string(mgmtTask.UID)
}

func workloadSelector(mgmtTask *rlarkv1alpha1.Task) *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: map[string]string{ManagementTaskUIDLabel: string(mgmtTask.UID)}}
}

// applyAntiAffinity injects a pod anti-affinity rule so that pods from the same
// workload are scheduled on different nodes when possible.
func applyAntiAffinity(template *corev1.PodTemplateSpec) {
	if template == nil {
		return
	}

	if template.Spec.Affinity == nil {
		template.Spec.Affinity = &corev1.Affinity{}
	}

	if template.Spec.Affinity.PodAntiAffinity == nil {
		template.Spec.Affinity.PodAntiAffinity = &corev1.PodAntiAffinity{}
	}

	template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = append(
		template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
		corev1.PodAffinityTerm{
			TopologyKey: "kubernetes.io/hostname",
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: template.Labels,
			},
		},
	)
}

// applyTemplateMutations applies all common pod template mutations shared by workload builders.
func applyTemplateMutations(template *corev1.PodTemplateSpec, mgmtTask *rlarkv1alpha1.Task, image string) {
	applyDomainAnnotation(template, mgmtTask)
	applyRayInit(template, mgmtTask)
	applyNetworkSidecar(template, mgmtTask, image)
	applyRLarkTools(template, image)
	ensureLabels(template, mgmtTask.Name)
	applyNodeSelector(&template.Spec, mgmtTask.Spec.NodeSelector)
	applyAntiAffinity(template)

	// todo 后续 rlinf 使用新方式访问真机设备后去掉
	if os.Getenv(UnsafeTaskPrivilegesEnv) != "true" {
		return
	}
	role := ""
	if mgmtTask.Annotations != nil {
		role = mgmtTask.Annotations[rlarkv1alpha1.RayRoleAnnotation]
	}
	if role != rlarkv1alpha1.RayRoleHead {
		template.Spec.HostNetwork = true
		template.Spec.DNSPolicy = corev1.DNSClusterFirstWithHostNet
	}
	for i := range template.Spec.Containers {
		if template.Spec.Containers[i].SecurityContext == nil {
			template.Spec.Containers[i].SecurityContext = &corev1.SecurityContext{}
		}
		template.Spec.Containers[i].SecurityContext.Privileged = utils.Ptr(true)
	}
	for i := range template.Spec.InitContainers {
		if template.Spec.InitContainers[i].SecurityContext == nil {
			template.Spec.InitContainers[i].SecurityContext = &corev1.SecurityContext{}
		}
		template.Spec.InitContainers[i].SecurityContext.Privileged = utils.Ptr(true)
	}
}

// ensureImagePullSecrets injects locally delivered credentials matching template images.
func (r *pullReconciler) ensureImagePullSecrets(ctx context.Context, template *corev1.PodTemplateSpec, workloadNamespace string) error {
	secretList := &corev1.SecretList{}
	if err := r.c.LocalKubeClient.List(ctx, secretList, client.InNamespace(workloadNamespace), client.MatchingLabels{
		common.ImageRegistryCredentialLabel: "true",
	}); err != nil {
		return fmt.Errorf("list image registry secrets: %w", err)
	}

	if len(secretList.Items) == 0 {
		return nil
	}

	// Collect all image references from the template
	imageRefs := []string{}
	for _, c := range template.Spec.Containers {
		if c.Image != "" {
			imageRefs = append(imageRefs, c.Image)
		}
	}
	for _, c := range template.Spec.InitContainers {
		if c.Image != "" {
			imageRefs = append(imageRefs, c.Image)
		}
	}

	if len(imageRefs) == 0 {
		return nil
	}

	type registryCredential struct {
		registry string
		name     string
	}
	credentials := make([]registryCredential, 0, len(secretList.Items))
	for _, secret := range secretList.Items {
		if secret.Type != corev1.SecretTypeDockerConfigJson || len(secret.Data[corev1.DockerConfigJsonKey]) == 0 {
			continue
		}
		registry := common.NormalizeRegistry(secret.Annotations[common.ImageRegistryAnnotationRegistry])
		if registry == "" {
			continue
		}
		credentials = append(credentials, registryCredential{registry: registry, name: secret.Name})
	}

	matchedSecrets := make(map[string]struct{})
	for _, image := range imageRefs {
		image = common.NormalizeRegistry(image)
		for _, credential := range credentials {
			if strings.HasPrefix(image, credential.registry+"/") || image == credential.registry {
				matchedSecrets[credential.name] = struct{}{}
			}
		}
	}

	if len(matchedSecrets) == 0 {
		return nil
	}

	existing := make(map[string]struct{}, len(template.Spec.ImagePullSecrets))
	for _, ips := range template.Spec.ImagePullSecrets {
		existing[ips.Name] = struct{}{}
	}
	names := make([]string, 0, len(matchedSecrets))
	for secretName := range matchedSecrets {
		if _, found := existing[secretName]; !found {
			names = append(names, secretName)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		template.Spec.ImagePullSecrets = append(template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
	}
	return nil
}

func buildDeployment(mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        mgmtTask.Name,
			Namespace:   getWorkloadNamespace(mgmtTask),
			Annotations: workloadAnnotations(mgmtTask),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: spec.Replicas,
			Selector: workloadSelector(mgmtTask),
			Template: spec.Template,
		},
	}
}

func buildDaemonSet(mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        mgmtTask.Name,
			Namespace:   getWorkloadNamespace(mgmtTask),
			Annotations: workloadAnnotations(mgmtTask),
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: workloadSelector(mgmtTask),
			Template: spec.Template,
		},
	}
}

func buildStatefulSet(mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        mgmtTask.Name,
			Namespace:   getWorkloadNamespace(mgmtTask),
			Annotations: workloadAnnotations(mgmtTask),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            spec.Replicas,
			Selector:            workloadSelector(mgmtTask),
			Template:            spec.Template,
			PodManagementPolicy: appsv1.ParallelPodManagement,
		},
	}
}

// --- helper functions ---

// applyNodeSelector applies the task's node selector to the pod spec.
// Values containing commas are converted to nodeAffinity with In operator;
// single values use nodeSelector directly.
func applyNodeSelector(podSpec *corev1.PodSpec, selector map[string]string) {
	if len(selector) == 0 {
		return
	}

	var nodeSelector map[string]string
	var affinityTerms []corev1.NodeSelectorTerm

	for k, v := range selector {
		if strings.Contains(v, ",") {
			values := strings.Split(v, ",")
			for i := range values {
				values[i] = strings.TrimSpace(values[i])
			}
			affinityTerms = append(affinityTerms, corev1.NodeSelectorTerm{
				MatchExpressions: []corev1.NodeSelectorRequirement{
					{
						Key:      k,
						Operator: corev1.NodeSelectorOpIn,
						Values:   values,
					},
				},
			})
		} else {
			if nodeSelector == nil {
				nodeSelector = make(map[string]string)
			}
			nodeSelector[k] = v
		}
	}

	podSpec.NodeSelector = nodeSelector
	if len(affinityTerms) > 0 {
		if podSpec.Affinity == nil {
			podSpec.Affinity = &corev1.Affinity{}
		}

		podSpec.Affinity.NodeAffinity = &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: affinityTerms,
			},
		}
	}
}

// applyDomainAnnotation injects the management-task annotations (including domain)
// into the Pod template spec so that pods created by the workload carry these annotations.
func applyDomainAnnotation(template *corev1.PodTemplateSpec, mgmtTask *rlarkv1alpha1.Task) {
	if template.Annotations == nil {
		template.Annotations = make(map[string]string)
	}
	template.Annotations[ManagementTaskNameAnnotation] = mgmtTask.Name
	template.Annotations[ManagementTaskNamespaceAnnotation] = mgmtTask.Namespace
	if mgmtTask.Spec.Domain != "" {
		template.Annotations[ManagementTaskDomainAnnotation] = mgmtTask.Spec.Domain
	}
}
