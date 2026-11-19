package addon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/rollout"
	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

// Constants used by the package.
const (
	AddonRequeueInterval = 10 * time.Second
)

// Reason values for the Addon Ready condition. They are stable, CamelCase
// identifiers consumed by clients.
const (
	addonReasonReady      = "AddonReady"
	addonReasonFailed     = "AddonFailed"
	addonReasonInstalling = "AddonInstalling"
	addonReasonUpgrading  = "AddonUpgrading"
	addonReasonUnknown    = "AddonPhaseUnknown"
)

// addonReadiness maps an Addon phase onto the Ready condition. Ready is the
// structured companion of Phase: clients read the boolean conclusion and its
// machine-readable Reason without having to know the phase vocabulary.
func addonReadiness(phase rlarkv1alpha1.AddonPhase) (metav1.ConditionStatus, string) {
	switch phase {
	case rlarkv1alpha1.AddonPhaseReady:
		return metav1.ConditionTrue, addonReasonReady
	case rlarkv1alpha1.AddonPhaseFailed:
		return metav1.ConditionFalse, addonReasonFailed
	case rlarkv1alpha1.AddonPhaseInstalling:
		return metav1.ConditionFalse, addonReasonInstalling
	case rlarkv1alpha1.AddonPhaseUpgrading:
		return metav1.ConditionFalse, addonReasonUpgrading
	default:
		return metav1.ConditionUnknown, addonReasonUnknown
	}
}

type pullReconciler struct {
	c *Controller
}

// Reconcile reconciles the resource.
func (r *pullReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("addon", req.NamespacedName)

	var mgmtAddon rlarkv1alpha1.Addon
	if err := r.c.ManagementClient.Get(ctx, req.NamespacedName, &mgmtAddon); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Handle deletion: clean up local resources and remove finalizer
	if mgmtAddon.DeletionTimestamp != nil {
		logger.Info("management Addon being deleted, cleaning up local resources")
		if err := r.cleanupAddon(ctx, &mgmtAddon); err != nil {
			if errors.Is(err, errAddonCleanupPending) {
				return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
			}
			logger.Error(err, "failed to clean up addon resources")
			return reconcile.Result{}, err
		}
		mgmtAddon.Finalizers = slices.DeleteFunc(mgmtAddon.Finalizers, func(s string) bool {
			return s == AddonFinalizer
		})
		if err := r.c.ManagementClient.Update(ctx, &mgmtAddon); err != nil {
			logger.Error(err, "failed to remove finalizer")
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer if not present
	if !slices.Contains(mgmtAddon.Finalizers, AddonFinalizer) {
		mgmtAddon.Finalizers = append(mgmtAddon.Finalizers, AddonFinalizer)
		if err := r.c.ManagementClient.Update(ctx, &mgmtAddon); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
	}

	// Only handle Kubernetes agent type
	if r.c.AgentType != string(rlarkv1alpha1.AgentTypeKubernetes) {
		return reconcile.Result{}, nil
	}

	// Determine target phase
	targetPhase := rlarkv1alpha1.AddonPhaseInstalling
	if mgmtAddon.Status.Phase == rlarkv1alpha1.AddonPhaseReady {
		targetPhase = rlarkv1alpha1.AddonPhaseUpgrading
	}

	manifests, err := r.rendered(ctx, &mgmtAddon)
	if errors.Is(err, errPackageMissing) {
		// Nothing of the Addon is touched. Where nothing new was asked of it (the agent was replaced, the
		// Addon is as it was), its workloads are still what its status says; a new revision that cannot be
		// rendered has failed.
		phase := mgmtAddon.Status.Phase
		if phase == "" || mgmtAddon.Status.ObservedGeneration != mgmtAddon.Generation {
			phase = rlarkv1alpha1.AddonPhaseFailed
		}
		r.updateStatus(ctx, &mgmtAddon, phase, err.Error())
		return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
	}
	if err != nil {
		r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("render error: %v", err))
		return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
	}

	// Apply each manifest
	var storageViews []storageref.View
	for i, m := range manifests {
		objs, err := parseManifest(m)
		if err != nil {
			r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("parse manifest %d: %v", i, err))
			return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
		}
		for _, obj := range objs {
			if err := r.resolveStorage(ctx, obj, apis.Namespace, &storageViews); err != nil {
				r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("storage: %v", err))
				return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
			}
			if err := rollDaemonSet(obj); err != nil {
				r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("%s/%s: %v", obj.GetKind(), obj.GetName(), err))
				return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
			}
			injectLabels(obj, mgmtAddon.Name, string(mgmtAddon.UID))
			obj.SetAnnotations(mergeMaps(obj.GetAnnotations(), map[string]string{
				AddonGenerationAnnotation: strconv.FormatInt(mgmtAddon.Generation, 10),
			}))
			if err := r.applyObject(ctx, obj, &mgmtAddon); err != nil {
				r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("apply %s/%s: %v", obj.GetKind(), obj.GetName(), err))
				return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
			}
		}
	}

	if err := r.replacePodsOnUnselectedNodes(ctx, &mgmtAddon); err != nil {
		r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("placement: %v", err))
		return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
	}

	// Check readiness
	ready, msg, err := r.checkReady(ctx, &mgmtAddon)
	if err != nil {
		r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("readiness check error: %v", err))
		return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
	}

	if ready {
		r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseReady, "")
		return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
	}

	// Not ready yet — check if any pods are failed
	failed := r.checkFailedPods(ctx, &mgmtAddon)
	if failed != "" {
		r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, failed)
		return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
	}

	// Still installing or upgrading
	r.updateStatus(ctx, &mgmtAddon, targetPhase, msg)
	return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
}

func (r *pullReconciler) applyObject(ctx context.Context, obj *unstructured.Unstructured, mgmtAddon *rlarkv1alpha1.Addon) error {
	logger := log.FromContext(ctx).WithValues("kind", obj.GetKind(), "name", obj.GetName())

	gvk := obj.GroupVersionKind()
	// For namespaced resources, set namespace to the addon's namespace (cluster ID)
	if obj.GetNamespace() == "" && isNamespaced(gvk) {
		obj.SetNamespace(apis.Namespace)
	}

	if err := r.injectImagePullSecrets(ctx, obj); err != nil {
		return err
	}

	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(gvk)

	err := r.c.LocalKubeClient.Get(ctx, key, existing)
	if err != nil && client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("get %s/%s: %w", gvk.Kind, key, err)
	}

	if err != nil {
		// Not found — create
		logger.Info("creating resource")
		if err := r.c.LocalKubeClient.Create(ctx, obj); err != nil {
			return fmt.Errorf("create %s/%s: %w", gvk.Kind, key, err)
		}
		return nil
	}

	// Never adopt a same-named object belonging to a different Addon instance.
	labels := existing.GetLabels()
	if labels[addons.LabelAddonName] != mgmtAddon.Name || labels[addons.LabelAddonUID] != string(mgmtAddon.UID) || mgmtAddon.UID == "" {
		return fmt.Errorf("ownership conflict for %s/%s", gvk.Kind, key)
	}
	// The annotation identifies the rendered revision, but does not prove that
	// the live fields still match it. Check desired fields on each reconciliation.
	existingAnnotations := existing.GetAnnotations()
	sameGeneration := existingAnnotations != nil && existingAnnotations[AddonGenerationAnnotation] == strconv.FormatInt(mgmtAddon.Generation, 10)
	ready, _, _ := unstructured.NestedInt64(existing.Object, "status", "readyReplicas")
	pods, _, _ := unstructured.NestedInt64(existing.Object, "status", "replicas")
	if gvk.Kind == "DaemonSet" {
		ready, _, _ = unstructured.NestedInt64(existing.Object, "status", "numberReady")
		pods, _, _ = unstructured.NestedInt64(existing.Object, "status", "desiredNumberScheduled")
	}
	if sameGeneration && ready == pods {
		// The credentials given by image name are not a reason to replace the ready Pods of an Addon that has not
		// changed (a credential that comes to match, or is deleted): they are taken with the Addon's next change,
		// or at once while a Pod is not ready, which may be waiting for the very credential.
		if pulls, found, _ := unstructured.NestedFieldCopy(existing.Object, "spec", "template", "spec", "imagePullSecrets"); found {
			_ = unstructured.SetNestedField(obj.Object, pulls, "spec", "template", "spec", "imagePullSecrets")
		} else {
			unstructured.RemoveNestedField(obj.Object, "spec", "template", "spec", "imagePullSecrets")
		}
	}
	if sameGeneration && desiredFieldsMatch(obj, existing) {
		return nil
	}

	logger.Info("updating resource")
	mergedLabels := mergeMaps(existing.GetLabels(), obj.GetLabels())
	mergedAnnotations := mergeMaps(existing.GetAnnotations(), obj.GetAnnotations())
	mergedAnnotations[AddonGenerationAnnotation] = strconv.FormatInt(mgmtAddon.Generation, 10)

	existing.SetLabels(mergedLabels)
	existing.SetAnnotations(mergedAnnotations)
	// Sync all top-level fields (spec, data, binaryData, type, rules, subjects, roleRef, etc.)
	// Skip fields managed separately or by the server.
	for k, v := range obj.Object {
		switch k {
		case "apiVersion", "kind", "metadata", "status":
			continue
		default:
			existing.Object[k] = v
		}
	}

	if err := r.c.LocalKubeClient.Update(ctx, existing); err != nil {
		return fmt.Errorf("update %s/%s: %w", gvk.Kind, key, err)
	}
	return nil
}

func (r *pullReconciler) cleanupAddon(ctx context.Context, mgmtAddon *rlarkv1alpha1.Addon) error {
	if r.c.LocalKubeClient == nil {
		return errors.New("local Kubernetes client unavailable during addon cleanup")
	}
	if mgmtAddon.UID == "" {
		return errors.New("cannot clean up addon without UID")
	}

	labelSelector := client.MatchingLabels{
		addons.LabelAddonName: mgmtAddon.Name,
		addons.LabelAddonUID:  string(mgmtAddon.UID),
	}

	// Delete all resources with the addon label
	resources := []client.ObjectList{
		&appsv1.DeploymentList{},
		&appsv1.DaemonSetList{},
		&appsv1.StatefulSetList{},
		&corev1.ConfigMapList{},
		&corev1.ServiceList{},
		&corev1.SecretList{},
		&rbacv1.ClusterRoleBindingList{},
		&rbacv1.ClusterRoleList{},
		&rbacv1.RoleBindingList{},
		&rbacv1.RoleList{},
		&corev1.ServiceAccountList{},
		&storagev1.CSIDriverList{},
		&storagev1.StorageClassList{},
	}

	pending := false
	for _, list := range resources {
		if err := r.c.LocalKubeClient.List(ctx, list, labelSelector); err != nil {
			return fmt.Errorf("list addon resources %T: %w", list, err)
		}
		items := extractItems(list)
		if items == nil {
			continue
		}
		for _, item := range items {
			uid := item.GetUID()
			version := item.GetResourceVersion()
			if uid == "" || version == "" {
				return fmt.Errorf("addon resource %s/%s has no UID or resourceVersion", item.GetNamespace(), item.GetName())
			}
			if err := r.c.LocalKubeClient.Delete(ctx, item, client.Preconditions{UID: &uid, ResourceVersion: &version}); err != nil {
				if client.IgnoreNotFound(err) == nil {
					continue
				}
				return fmt.Errorf("delete addon resource %s/%s: %w", item.GetNamespace(), item.GetName(), err)
			}
			pending = true
		}
	}
	if pending {
		return errAddonCleanupPending
	}
	return nil
}

func (r *pullReconciler) checkReady(ctx context.Context, mgmtAddon *rlarkv1alpha1.Addon) (bool, string, error) {
	logger := log.FromContext(ctx)
	labels := client.MatchingLabels{
		addons.LabelAddonName: mgmtAddon.Name,
	}

	var foundWorkloads bool

	// Check Deployments
	depList := &appsv1.DeploymentList{}
	if err := r.c.LocalKubeClient.List(ctx, depList, labels); err != nil {
		return false, "", err
	}
	logger.Info("checkReady: deployments found", "count", len(depList.Items))
	for _, d := range depList.Items {
		foundWorkloads = true
		// Pods of an older template (a rollout stuck on an image pull, say) do not count.
		if d.Status.ObservedGeneration < d.Generation || d.Status.UpdatedReplicas < *d.Spec.Replicas || d.Status.Replicas > d.Status.UpdatedReplicas || d.Status.ReadyReplicas < *d.Spec.Replicas {
			msg := fmt.Sprintf("Deployment %s: %d/%d updated, %d ready", d.Name, d.Status.UpdatedReplicas, *d.Spec.Replicas, d.Status.ReadyReplicas)
			if why := r.podState(ctx, d.Namespace, d.Spec.Selector.MatchLabels); why != "" {
				msg += "; " + why
			}
			if label := d.Annotations[NotReadyMessageAnnotation]; label != "" {
				msg = label + ": " + msg
			}
			return false, msg, nil
		}
	}

	// Check DaemonSets
	dsList := &appsv1.DaemonSetList{}
	if err := r.c.LocalKubeClient.List(ctx, dsList, labels); err != nil {
		return false, "", err
	}
	logger.Info("checkReady: daemonsets found", "count", len(dsList.Items), "labels", labels)
	for _, ds := range dsList.Items {
		foundWorkloads = true
		if ds.Status.ObservedGeneration < ds.Generation {
			return false, fmt.Sprintf("DaemonSet %s: not yet observed", ds.Name), nil
		}
		// A DaemonSet meant for one class of nodes (a robot's, say) is fine on a
		// cluster without such nodes; one meant for every node is not.
		pod := ds.Spec.Template.Spec
		if ds.Status.DesiredNumberScheduled == 0 && len(pod.NodeSelector) == 0 && (pod.Affinity == nil || pod.Affinity.NodeAffinity == nil) {
			return false, fmt.Sprintf("DaemonSet %s: no nodes scheduled", ds.Name), nil
		}
		if rollout.Rolled(&ds) {
			waiting, err := r.rolledDaemonSetWaitingOn(ctx, &ds)
			if err != nil {
				return false, "", err
			}
			if waiting != "" {
				return false, fmt.Sprintf("DaemonSet %s: waiting on %s", ds.Name, waiting), nil
			}
			continue
		}
		// Under OnDelete the Pods take a new template only when someone replaces them (csi-driver-rclone keeps
		// a node's mounts that way), so not-yet-updated Pods are what was asked for, not an unfinished rollout.
		rolling := ds.Spec.UpdateStrategy.Type != appsv1.OnDeleteDaemonSetStrategyType
		if (rolling && ds.Status.UpdatedNumberScheduled < ds.Status.DesiredNumberScheduled) || ds.Status.NumberReady < ds.Status.DesiredNumberScheduled {
			waiting, err := r.daemonSetWaitingOn(ctx, &ds)
			if err != nil {
				return false, "", err
			}
			return false, fmt.Sprintf("DaemonSet %s: %d/%d ready%s", ds.Name, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled, waiting), nil
		}
	}

	// Check StatefulSets
	ssList := &appsv1.StatefulSetList{}
	if err := r.c.LocalKubeClient.List(ctx, ssList, labels); err != nil {
		return false, "", err
	}
	logger.Info("checkReady: statefulsets found", "count", len(ssList.Items))
	for _, ss := range ssList.Items {
		foundWorkloads = true
		if ss.Status.ObservedGeneration < ss.Generation || ss.Status.UpdatedReplicas < *ss.Spec.Replicas || ss.Status.ReadyReplicas < *ss.Spec.Replicas {
			return false, fmt.Sprintf("StatefulSet %s: %d/%d ready", ss.Name, ss.Status.ReadyReplicas, *ss.Spec.Replicas), nil
		}
	}

	if !foundWorkloads {
		return false, "waiting for workload resources to be created", nil
	}

	return true, "", nil
}

func (r *pullReconciler) checkFailedPods(ctx context.Context, mgmtAddon *rlarkv1alpha1.Addon) string {
	podList := &corev1.PodList{}
	if err := r.c.LocalKubeClient.List(ctx, podList,
		client.MatchingLabels{addons.LabelAddonName: mgmtAddon.Name},
	); err != nil {
		return ""
	}

	for _, p := range podList.Items {
		if p.Status.Phase == corev1.PodFailed {
			var msg string
			if len(p.Status.ContainerStatuses) > 0 {
				cs := p.Status.ContainerStatuses[0]
				if cs.State.Terminated != nil {
					msg = cs.State.Terminated.Message
				}
			}
			if msg == "" {
				msg = fmt.Sprintf("Pod %s failed (reason: %s)", p.Name, p.Status.Reason)
			}
			return msg
		}
	}
	return ""
}

func (r *pullReconciler) updateStatus(ctx context.Context, mgmtAddon *rlarkv1alpha1.Addon, phase rlarkv1alpha1.AddonPhase, message string) {
	changed := false

	if mgmtAddon.Status.Phase != phase {
		mgmtAddon.Status.Phase = phase
		changed = true
	}
	if mgmtAddon.Status.Message != message {
		mgmtAddon.Status.Message = message
		changed = true
	}
	if phase == rlarkv1alpha1.AddonPhaseReady && mgmtAddon.Status.Version != mgmtAddon.Spec.Version {
		mgmtAddon.Status.Version = mgmtAddon.Spec.Version
		changed = true
	}

	// ObservedGeneration is part of the controller contract: consumers compare it
	// against metadata.generation to detect stale phase / conditions. It must be
	// advanced even when phase and message happen to be unchanged, otherwise a
	// Spec-only change would leave the status silently stale.
	if mgmtAddon.Status.ObservedGeneration != mgmtAddon.Generation {
		mgmtAddon.Status.ObservedGeneration = mgmtAddon.Generation
		changed = true
	}

	readyStatus, reason := addonReadiness(phase)
	if apimeta.SetStatusCondition(&mgmtAddon.Status.Conditions, metav1.Condition{
		Type:    rlarkv1alpha1.ConditionTypeReady,
		Status:  readyStatus,
		Reason:  reason,
		Message: message,
	}) {
		changed = true
	}

	if !changed {
		return
	}
	if err := r.c.ManagementClient.Status().Update(ctx, mgmtAddon); err != nil {
		log.FromContext(ctx).Error(err, "failed to update addon status", "phase", phase)
	}
}

// --- helpers ---

func parseManifest(data []byte) ([]*unstructured.Unstructured, error) {
	var result []*unstructured.Unstructured

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	for {
		obj := &unstructured.Unstructured{}
		if err := decoder.Decode(obj); err != nil {
			if err.Error() == "EOF" {
				break
			}
			if strings.Contains(err.Error(), "doesn't contain any data") {
				break
			}
			return nil, err
		}
		if len(obj.Object) == 0 {
			continue
		}
		result = append(result, obj)
	}
	return result, nil
}

func injectLabels(obj *unstructured.Unstructured, addonName, addonUID string) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[addons.LabelAddonName] = addonName
	labels[addons.LabelAddonUID] = addonUID
	obj.SetLabels(labels)
}

func mergeMaps(a, b map[string]string) map[string]string {
	result := make(map[string]string)
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		result[k] = v
	}
	return result
}

var namespacedKinds = map[string]bool{
	"Deployment":            true,
	"DaemonSet":             true,
	"StatefulSet":           true,
	"Pod":                   true,
	"Service":               true,
	"ConfigMap":             true,
	"Secret":                true,
	"ServiceAccount":        true,
	"PersistentVolumeClaim": true,
	"Role":                  true,
	"RoleBinding":           true,
}

func isNamespaced(gvk schema.GroupVersionKind) bool {
	return namespacedKinds[gvk.Kind]
}

func extractItems(list client.ObjectList) []client.Object {
	switch l := list.(type) {
	case *appsv1.DeploymentList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *appsv1.DaemonSetList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *appsv1.StatefulSetList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *corev1.ConfigMapList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *corev1.ServiceList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *corev1.SecretList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *rbacv1.ClusterRoleBindingList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *rbacv1.ClusterRoleList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *corev1.ServiceAccountList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *rbacv1.RoleList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *rbacv1.RoleBindingList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *storagev1.CSIDriverList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	case *storagev1.StorageClassList:
		result := make([]client.Object, 0, len(l.Items))
		for i := range l.Items {
			result = append(result, &l.Items[i])
		}
		return result
	}
	return nil
}
