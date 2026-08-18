package task

import (
	"context"
	"fmt"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
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
	ManagementTaskNameAnnotation            = "rlark.io/management-task-name"
	ManagementTaskNamespaceAnnotation       = "rlark.io/management-task-namespace"
	ManagementTaskResourceVersionAnnotation = "rlark.io/management-task-resource-version"
	ManagementTaskUIDAnnotation             = "rlark.io/management-task-uid"
	ManagementTaskDomainAnnotation          = "rlark.io/management-task-domain"
	ManagementTaskFinalizer                 = "rlark.io/agent-cleanup"
	DefaultPVCSize                          = "10Gi"
	PVCTaskLabel                            = "rlark.io/task"
	PVCOwnerAnnotation                      = "rlark.io/pvc-owner"
	PVCOwnerTaskAnnotation                  = "rlark.io/pvc-owner-task"
)

// pullReconciler watches management Tasks and creates workloads on local cluster.
type pullReconciler struct {
	c *Controller
}

// Reconcile reconciles the resource.
func (r *pullReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("task", req.NamespacedName)

	var mgmtTask rlarkv1alpha1.Task
	if err := r.c.ManagementClient.Get(ctx, req.NamespacedName, &mgmtTask); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("management Task deleted, cleaning up local workload")
			if cleanupErr := r.cleanupWorkload(ctx, req.Name, req.Namespace); cleanupErr != nil {
				logger.Error(cleanupErr, "failed to clean up workload")
			}
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Handle deletion: clean up local workload and remove finalizer
	if mgmtTask.DeletionTimestamp != nil {
		logger.Info("management Task being deleted, cleaning up local workload")
		workloadNs := getWorkloadNamespace(&mgmtTask)
		if err := r.cleanupWorkload(ctx, mgmtTask.Name, workloadNs); err != nil {
			logger.Error(err, "failed to clean up workload")
			return reconcile.Result{}, err
		}
		mgmtTask.Finalizers = slices.DeleteFunc(mgmtTask.Finalizers, func(s string) bool {
			return s == ManagementTaskFinalizer
		})
		if err := r.c.ManagementClient.Update(ctx, &mgmtTask); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer if not present
	if !slices.Contains(mgmtTask.Finalizers, ManagementTaskFinalizer) {
		mgmtTask.Finalizers = append(mgmtTask.Finalizers, ManagementTaskFinalizer)
		if err := r.c.ManagementClient.Update(ctx, &mgmtTask); err != nil {
			logger.Error(err, "failed to add finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil
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

	workloadSpec := mgmtTask.Spec.Kubernetes.Workload
	applyTemplateMutations(&workloadSpec.Template, &mgmtTask, r.c.Image)

	workloadNamespace := getWorkloadNamespace(&mgmtTask)
	if err := r.ensureImagePullSecrets(ctx, &workloadSpec.Template, workloadNamespace); err != nil {
		logger.Error(err, "failed to ensure image pull secrets")
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

// createOrUpdateWorkload is a generic helper that handles the create-or-update pattern for all workload types.
// existingObj is a pre-allocated empty object for Get, newObj is the fully-built object for Create,
// applyUpdate is a callback that applies spec changes to the existing object when the management Task's
// ResourceVersion has changed (indicating the Task spec was updated).
func (r *pullReconciler) createOrUpdateWorkload(
	ctx context.Context,
	mgmtTask *rlarkv1alpha1.Task,
	workloadKind string,
	existingObj client.Object,
	newObj client.Object,
	applyUpdate func(existingObj client.Object),
) (reconcile.Result, error) {
	logger := log.FromContext(ctx)
	key := types.NamespacedName{Name: newObj.GetName(), Namespace: newObj.GetNamespace()}

	err := r.c.LocalKubeClient.Get(ctx, key, existingObj)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return reconcile.Result{}, err
	}

	if err != nil {
		logger.Info(fmt.Sprintf("creating %s", workloadKind))
		if err := r.c.LocalKubeClient.Create(ctx, newObj); err != nil {
			logger.Error(err, fmt.Sprintf("failed to create %s", workloadKind))
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	annotations := existingObj.GetAnnotations()
	if annotations == nil || annotations[ManagementTaskResourceVersionAnnotation] != mgmtTask.ResourceVersion {
		logger.Info(fmt.Sprintf("%s spec changed (Task ResourceVersion mismatch), updating", workloadKind))
		applyUpdate(existingObj)
		if err := r.c.LocalKubeClient.Update(ctx, existingObj); err != nil {
			logger.Error(err, fmt.Sprintf("failed to update %s", workloadKind))
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

func (r *pullReconciler) createOrUpdateDeployment(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) (reconcile.Result, error) {
	return r.createOrUpdateWorkload(ctx, mgmtTask, "Deployment",
		&appsv1.Deployment{},
		buildDeployment(mgmtTask, spec),
		func(obj client.Object) {
			deploy := obj.(*appsv1.Deployment)
			if deploy.Annotations == nil {
				deploy.Annotations = make(map[string]string)
			}
			deploy.Annotations[ManagementTaskResourceVersionAnnotation] = mgmtTask.ResourceVersion
			deploy.Spec.Replicas = spec.Replicas
			deploy.Spec.Template = spec.Template
		})
}

func (r *pullReconciler) createOrUpdateDaemonSet(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) (reconcile.Result, error) {
	return r.createOrUpdateWorkload(ctx, mgmtTask, "DaemonSet",
		&appsv1.DaemonSet{},
		buildDaemonSet(mgmtTask, spec),
		func(obj client.Object) {
			ds := obj.(*appsv1.DaemonSet)
			if ds.Annotations == nil {
				ds.Annotations = make(map[string]string)
			}
			ds.Annotations[ManagementTaskResourceVersionAnnotation] = mgmtTask.ResourceVersion
			ds.Spec.Template = spec.Template
		})
}

func (r *pullReconciler) createOrUpdateStatefulSet(ctx context.Context, mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) (reconcile.Result, error) {
	return r.createOrUpdateWorkload(ctx, mgmtTask, "StatefulSet",
		&appsv1.StatefulSet{},
		buildStatefulSet(mgmtTask, spec),
		func(obj client.Object) {
			sts := obj.(*appsv1.StatefulSet)
			if sts.Annotations == nil {
				sts.Annotations = make(map[string]string)
			}
			sts.Annotations[ManagementTaskResourceVersionAnnotation] = mgmtTask.ResourceVersion
			sts.Spec.Replicas = spec.Replicas
			sts.Spec.Template = spec.Template
		})
}

func (r *pullReconciler) cleanupWorkload(ctx context.Context, name string, namespace string) error {
	if r.c.LocalKubeClient == nil {
		return nil
	}
	workloadKey := types.NamespacedName{Name: name, Namespace: namespace}

	for _, obj := range []client.Object{&appsv1.Deployment{}, &appsv1.DaemonSet{}, &appsv1.StatefulSet{}} {
		if err := r.c.LocalKubeClient.Get(ctx, workloadKey, obj); err == nil {
			if err := r.c.LocalKubeClient.Delete(ctx, obj); err != nil {
				return err
			}
		}
	}

	svcKey := types.NamespacedName{Name: rayHeadServiceName(name), Namespace: namespace}
	var svc corev1.Service
	if err := r.c.LocalKubeClient.Get(ctx, svcKey, &svc); err == nil {
		if err := r.c.LocalKubeClient.Delete(ctx, &svc); err != nil {
			return err
		}
	}

	if err := r.cleanupPVCs(ctx, name, namespace); err != nil {
		return err
	}

	return nil
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
		existingCM.Data = cm.Data
		if owner != nil {
			existingCM.OwnerReferences = cm.OwnerReferences
		}
		if err := r.c.LocalKubeClient.Update(ctx, &existingCM); err != nil {
			return fmt.Errorf("update ray ConfigMap %s: %w", cm.Name, err)
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
		if workloadSpec.PvcStorageMap != nil {
			storageClassName = workloadSpec.PvcStorageMap[claimName]
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
						corev1.ResourceStorage: resource.MustParse(DefaultPVCSize),
					},
				},
			},
		}

		if storageClassName != "" {
			pvc.Spec.StorageClassName = &storageClassName
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
				logger.Info("Updating PVC storage class", "pvc", claimName, "storageClass", storageClassName)
				existing.Spec.StorageClassName = &storageClassName
				if err := r.c.LocalKubeClient.Update(ctx, existing); err != nil {
					return fmt.Errorf("update PVC %s: %w", claimName, err)
				}
			}
		}
	}

	return nil
}

func (r *pullReconciler) cleanupPVCs(ctx context.Context, taskName string, namespace string) error {
	if r.c.LocalKubeClient == nil {
		return nil
	}

	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := r.c.LocalKubeClient.List(ctx, pvcList, client.InNamespace(namespace), client.MatchingLabels{
		PVCTaskLabel: taskName,
	}); err != nil {
		if errors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("list PVCs for task %s: %w", taskName, err)
	}

	if len(pvcList.Items) == 0 {
		var allPVCs corev1.PersistentVolumeClaimList
		if err := r.c.LocalKubeClient.List(ctx, &allPVCs, client.InNamespace(namespace)); err != nil {
			if errors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("list all PVCs in namespace %s: %w", namespace, err)
		}
		for _, pvc := range allPVCs.Items {
			if pvc.Annotations != nil && pvc.Annotations[PVCOwnerTaskAnnotation] == taskName {
				pvcList.Items = append(pvcList.Items, pvc)
			}
		}
	}

	for i := range pvcList.Items {
		logger := log.FromContext(ctx).WithValues("pvc", pvcList.Items[i].Name)
		logger.Info("Deleting PVC owned by task", "pvc", pvcList.Items[i].Name, "task", taskName)
		if err := r.c.LocalKubeClient.Delete(ctx, &pvcList.Items[i]); err != nil {
			if !errors.IsNotFound(err) {
				return fmt.Errorf("delete PVC %s: %w", pvcList.Items[i].Name, err)
			}
		}
	}

	return nil
}

func pvcNameForVolume(taskName, volumeName string) string {
	return fmt.Sprintf("pvc-%s-%s", taskName, volumeName)
}

func getWorkloadNamespace(mgmtTask *rlarkv1alpha1.Task) string {
	_ = mgmtTask
	return "rlark-system"
}

// --- workload builder functions ---

// ensureLabels ensures the pod template has labels, adding a default if none are set.
func ensureLabels(template *corev1.PodTemplateSpec, name string) {
	if template == nil || len(template.Labels) == 0 {
		template.Labels = map[string]string{
			"app": name,
		}
	}
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
	applySSHServer(template, mgmtTask, image)
	ensureLabels(template, mgmtTask.Name)
	applyNodeSelector(&template.Spec, mgmtTask.Spec.NodeSelector)
	applyAntiAffinity(template)

	// todo 后续 rlinf 使用新方式访问真机设备后去掉
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

// ensureImagePullSecrets syncs image registry secrets from the management cluster
// to the local workload namespace and injects matching ImagePullSecrets into the template.
func (r *pullReconciler) ensureImagePullSecrets(ctx context.Context, template *corev1.PodTemplateSpec, workloadNamespace string) error {
	logger := log.FromContext(ctx)

	// List all image registry secrets from the management cluster
	secretList := &corev1.SecretList{}
	if err := r.c.ManagementClient.List(ctx, secretList, &client.ListOptions{
		LabelSelector: labels.Set{common.ImageRegistrySecretLabel: "true"}.AsSelector(),
		Namespace:     common.SecretNamespace,
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

	// Build a map of registry prefix -> secret name
	registryToSecret := make(map[string]string, len(secretList.Items))
	for _, secret := range secretList.Items {
		registry := secret.Annotations[common.ImageRegistryAnnotationRegistry]
		if registry == "" {
			continue
		}
		registryToSecret[registry] = secret.Name
	}

	// Find matching registries for our images
	matchedSecrets := make(map[string]bool)
	for _, image := range imageRefs {
		for registry, secretName := range registryToSecret {
			if strings.HasPrefix(image, registry+"/") || image == registry {
				matchedSecrets[secretName] = true
				break
			}
		}
	}

	if len(matchedSecrets) == 0 {
		return nil
	}

	// Sync each matched secret to the local workload namespace
	for secretName := range matchedSecrets {
		if err := r.syncImagePullSecret(ctx, secretName, workloadNamespace); err != nil {
			logger.Error(err, "failed to sync image pull secret", "secret", secretName, "namespace", workloadNamespace)
			continue
		}
	}

	// Add matched secrets to template.Spec.ImagePullSecrets (avoid duplicates)
	existing := make(map[string]bool, len(template.Spec.ImagePullSecrets))
	for _, ips := range template.Spec.ImagePullSecrets {
		existing[ips.Name] = true
	}
	for secretName := range matchedSecrets {
		if !existing[secretName] {
			template.Spec.ImagePullSecrets = append(template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: secretName})
		}
	}

	return nil
}

// syncImagePullSecret copies a dockerconfigjson secret from the management cluster
// to the local workload namespace.
func (r *pullReconciler) syncImagePullSecret(ctx context.Context, secretName, destNamespace string) error {
	logger := log.FromContext(ctx)

	var srcSecret corev1.Secret
	if err := r.c.ManagementClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: common.SecretNamespace}, &srcSecret); err != nil {
		return fmt.Errorf("get source secret: %w", err)
	}

	var destSecret corev1.Secret
	err := r.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: destNamespace}, &destSecret)
	if err == nil {
		destSecret.Data = srcSecret.Data
		destSecret.Type = srcSecret.Type
		destSecret.Labels = srcSecret.Labels
		destSecret.Annotations = srcSecret.Annotations
		return r.c.LocalKubeClient.Update(ctx, &destSecret)
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("get local secret: %w", err)
	}

	newSecret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        secretName,
			Namespace:   destNamespace,
			Labels:      srcSecret.Labels,
			Annotations: srcSecret.Annotations,
		},
		Type: srcSecret.Type,
		Data: srcSecret.Data,
	}
	logger.Info("syncing image pull secret to local namespace", "secret", secretName, "namespace", destNamespace)
	return r.c.LocalKubeClient.Create(ctx, &newSecret)
}

func buildDeployment(mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mgmtTask.Name,
			Namespace: getWorkloadNamespace(mgmtTask),
			Annotations: map[string]string{
				ManagementTaskNameAnnotation:            mgmtTask.Name,
				ManagementTaskNamespaceAnnotation:       mgmtTask.Namespace,
				ManagementTaskUIDAnnotation:             string(mgmtTask.UID),
				ManagementTaskResourceVersionAnnotation: mgmtTask.ResourceVersion,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: spec.Template.Labels,
			},
			Template: spec.Template,
		},
	}
}

func buildDaemonSet(mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mgmtTask.Name,
			Namespace: getWorkloadNamespace(mgmtTask),
			Annotations: map[string]string{
				ManagementTaskNameAnnotation:            mgmtTask.Name,
				ManagementTaskNamespaceAnnotation:       mgmtTask.Namespace,
				ManagementTaskUIDAnnotation:             string(mgmtTask.UID),
				ManagementTaskResourceVersionAnnotation: mgmtTask.ResourceVersion,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: spec.Template.Labels,
			},
			Template: spec.Template,
		},
	}
}

func buildStatefulSet(mgmtTask *rlarkv1alpha1.Task, spec *rlarkv1alpha1.KubernetesWorkloadSpec) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mgmtTask.Name,
			Namespace: getWorkloadNamespace(mgmtTask),
			Annotations: map[string]string{
				ManagementTaskNameAnnotation:            mgmtTask.Name,
				ManagementTaskNamespaceAnnotation:       mgmtTask.Namespace,
				ManagementTaskUIDAnnotation:             string(mgmtTask.UID),
				ManagementTaskResourceVersionAnnotation: mgmtTask.ResourceVersion,
			},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: spec.Template.Labels,
			},
			Template: spec.Template,
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
