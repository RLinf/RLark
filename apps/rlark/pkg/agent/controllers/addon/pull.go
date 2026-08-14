package addon

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

// Constants used by the package.
const (
	AddonRequeueInterval = 10 * time.Second

	// AddonResourceVersionAnnotation annotates local resources linking back to the management Addon CR.
	AddonResourceVersionAnnotation = "rlark.io/addon-resource-version"
)

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

	// Look up addon in catalog
	addon, ok := addons.Registry.Get(mgmtAddon.Spec.AddonName)
	if !ok {
		r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("addon %q not found in catalog", mgmtAddon.Spec.AddonName))
		return reconcile.Result{}, nil
	}

	// Determine target phase
	targetPhase := rlarkv1alpha1.AddonPhaseInstalling
	if mgmtAddon.Status.Phase == rlarkv1alpha1.AddonPhaseReady {
		targetPhase = rlarkv1alpha1.AddonPhaseUpgrading
	}

	// Render manifests
	manifests, err := addon.Render(mgmtAddon.Spec.Values, apis.Namespace, mgmtAddon.Name, string(mgmtAddon.UID))
	if err != nil {
		r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("render error: %v", err))
		return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
	}

	// Apply each manifest
	for i, m := range manifests {
		objs, err := parseManifest(m.Raw)
		if err != nil {
			r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("parse manifest %d: %v", i, err))
			return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
		}
		for _, obj := range objs {
			injectLabels(obj, mgmtAddon.Name, string(mgmtAddon.UID))
			obj.SetAnnotations(mergeMaps(obj.GetAnnotations(), map[string]string{
				AddonResourceVersionAnnotation: mgmtAddon.ResourceVersion,
			}))
			if err := r.applyObject(ctx, obj, &mgmtAddon); err != nil {
				r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("apply %s/%s: %v", obj.GetKind(), obj.GetName(), err))
				return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
			}
		}
	}

	// Check readiness
	ready, msg, err := r.checkReady(ctx, &mgmtAddon)
	if err != nil {
		r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseFailed, fmt.Sprintf("readiness check error: %v", err))
		return reconcile.Result{RequeueAfter: AddonRequeueInterval}, nil
	}

	if ready {
		r.updateStatus(ctx, &mgmtAddon, rlarkv1alpha1.AddonPhaseReady, "")
		return reconcile.Result{}, nil
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

	// Exists — update if resource version annotation changed
	existingAnnotations := existing.GetAnnotations()
	if existingAnnotations != nil && existingAnnotations[AddonResourceVersionAnnotation] == mgmtAddon.ResourceVersion {
		return nil
	}

	logger.Info("updating resource")
	mergedLabels := mergeMaps(existing.GetLabels(), obj.GetLabels())
	mergedAnnotations := mergeMaps(existing.GetAnnotations(), obj.GetAnnotations())
	mergedAnnotations[AddonResourceVersionAnnotation] = mgmtAddon.ResourceVersion

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
		return nil
	}

	labelSelector := client.MatchingLabels{
		addons.LabelAddonName: mgmtAddon.Name,
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

	for _, list := range resources {
		if err := r.c.LocalKubeClient.List(ctx, list, labelSelector); err != nil {
			if client.IgnoreNotFound(err) != nil {
				continue
			}
		}
		items := extractItems(list)
		if items == nil {
			continue
		}
		for _, item := range items {
			if err := r.c.LocalKubeClient.Delete(ctx, item); err != nil {
				if client.IgnoreNotFound(err) != nil {
					continue
				}
			}
		}
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
		if d.Status.ReadyReplicas < *d.Spec.Replicas {
			return false, fmt.Sprintf("Deployment %s: %d/%d ready", d.Name, d.Status.ReadyReplicas, *d.Spec.Replicas), nil
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
		if ds.Status.DesiredNumberScheduled == 0 {
			return false, fmt.Sprintf("DaemonSet %s: no nodes scheduled", ds.Name), nil
		}
		if ds.Status.NumberReady < ds.Status.DesiredNumberScheduled {
			return false, fmt.Sprintf("DaemonSet %s: %d/%d ready", ds.Name, ds.Status.NumberReady, ds.Status.DesiredNumberScheduled), nil
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
		if ss.Status.ReadyReplicas < *ss.Spec.Replicas {
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
	if mgmtAddon.Status.Phase == phase && mgmtAddon.Status.Message == message {
		return
	}
	mgmtAddon.Status.Phase = phase
	mgmtAddon.Status.Message = message
	if phase == rlarkv1alpha1.AddonPhaseReady {
		mgmtAddon.Status.Version = mgmtAddon.Spec.Version
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
