package addon

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/rollout"
	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

const (
	// AddonGenerationAnnotation records the management Addon's generation (its spec revision) a local
	// resource was rendered from. Its resourceVersion is not used: every status write changes it, and an
	// update of a Deployment's annotations moves the Deployment's generation, so the two kept each other
	// moving and the Addon never read as ready.
	AddonGenerationAnnotation = "rlark.io/addon-generation"
	// NotReadyMessageAnnotation on an Addon workload names, in the operator's words, what it means when the
	// workload is not ready ("site cache unavailable"); the Addon's status message starts with it.
	NotReadyMessageAnnotation = "rlark.io/not-ready-message"
)

var errAddonCleanupPending = errors.New("addon resource deletion pending")

// rollDaemonSet makes an Addon's DaemonSet one whose Pods RLark replaces (package rollout): a node that is off
// does not hold the other nodes back, and one node goes first. The policy is the template's rlark.io/rollout
// annotation, with the maxUnavailable of the template's own rolling update where the annotation has none. A
// template that says OnDelete without the annotation keeps its Pods until someone replaces them
// (csi-driver-rclone keeps a node's mounts that way).
func rollDaemonSet(obj *unstructured.Unstructured) error {
	if obj.GroupVersionKind() != appsv1.SchemeGroupVersion.WithKind("DaemonSet") {
		return nil
	}
	annotation, annotated := obj.GetAnnotations()[rlarkv1alpha1.RolloutAnnotation]
	strategy, _, _ := unstructured.NestedString(obj.Object, "spec", "updateStrategy", "type")
	if strategy == string(appsv1.OnDeleteDaemonSetStrategyType) && !annotated {
		return nil
	}
	var policy rlarkv1alpha1.RolloutPolicy
	if annotated {
		var err error
		if policy, err = rlarkv1alpha1.ParseRolloutPolicy(annotation); err != nil {
			return err
		}
	}
	if written, found, _ := unstructured.NestedFieldNoCopy(obj.Object, "spec", "updateStrategy", "rollingUpdate", "maxUnavailable"); found && policy.MaxUnavailable == nil {
		bound := intstr.Parse(fmt.Sprint(written))
		policy.MaxUnavailable = &bound
	}
	obj.SetAnnotations(mergeMaps(obj.GetAnnotations(), map[string]string{rlarkv1alpha1.RolloutAnnotation: policy.Annotation()}))
	return unstructured.SetNestedMap(obj.Object, map[string]interface{}{"type": string(appsv1.OnDeleteDaemonSetStrategyType)}, "spec", "updateStrategy")
}

// rolledDaemonSetWaitingOn names what a DaemonSet RLark rolls still waits for among the nodes that are Ready: a
// Pod of the old template (unless it waits for its node to be idle, which is as asked) or one that is not ready.
// A node that is off is not waited for.
func (r *pullReconciler) rolledDaemonSetWaitingOn(ctx context.Context, ds *appsv1.DaemonSet) (string, error) {
	policy, err := rlarkv1alpha1.ParseRolloutPolicy(ds.Annotations[rlarkv1alpha1.RolloutAnnotation])
	if err != nil {
		return "", err
	}
	pods := &corev1.PodList{}
	if err := r.c.LocalKubeClient.List(ctx, pods, client.InNamespace(ds.Namespace), client.MatchingLabels(ds.Spec.Selector.MatchLabels)); err != nil {
		return "", err
	}
	var waiting []string
	for _, pod := range pods.Items {
		node := &corev1.Node{}
		if err := r.c.LocalKubeClient.Get(ctx, client.ObjectKey{Name: pod.Spec.NodeName}, node); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return "", err
		}
		if !slices.ContainsFunc(node.Status.Conditions, func(c corev1.NodeCondition) bool {
			return c.Type == corev1.NodeReady && c.Status == corev1.ConditionTrue
		}) {
			continue
		}
		switch outdated := pod.Labels["pod-template-generation"] != ds.Annotations[appsv1.DeprecatedTemplateGeneration]; {
		case !podReady(&pod):
			waiting = append(waiting, fmt.Sprintf("%s (%s)", pod.Spec.NodeName, podWaitReason(&pod, node)))
		case outdated && !(policy.WhenIdle && !rollout.Idle(node)):
			waiting = append(waiting, fmt.Sprintf("%s (to be updated)", pod.Spec.NodeName))
		}
	}
	sort.Strings(waiting)
	return strings.Join(waiting, ", "), nil
}

var errPackageMissing = errors.New("PackageMissing")

// renderedPrefix names the Secret that keeps an Addon's rendering in the cluster.
const renderedPrefix = "addon-rendered-"

// rendered returns the Addon's manifests as they were rendered for its current revision (its generation: its
// version and values). They are rendered once per revision, from the Package of the Addon's name and version
// that the control plane published to this cluster's management namespace, and kept in a Secret of the cluster;
// every later reconciliation applies what is kept. So an Addon's workloads change when the Addon does, and not
// when the agent is replaced by one that would render the same Addon differently or that knows other versions.
// A revision whose Package is not there is errPackageMissing, and nothing is rendered in its place.
func (r *pullReconciler) rendered(ctx context.Context, mgmtAddon *rlarkv1alpha1.Addon) ([][]byte, error) {
	generation := strconv.FormatInt(mgmtAddon.Generation, 10)
	kept := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: renderedPrefix + mgmtAddon.Name, Namespace: apis.Namespace}}
	err := r.c.LocalKubeClient.Get(ctx, client.ObjectKeyFromObject(kept), kept)
	if client.IgnoreNotFound(err) != nil {
		return nil, fmt.Errorf("get the kept rendering: %w", err)
	}
	exists := err == nil
	if exists && kept.Labels[addons.LabelAddonUID] == string(mgmtAddon.UID) && kept.Annotations[AddonGenerationAnnotation] == generation {
		names := slices.Sorted(maps.Keys(kept.Data))
		manifests := make([][]byte, 0, len(names))
		for _, name := range names {
			manifests = append(manifests, kept.Data[name])
		}
		return manifests, nil
	}

	published := &corev1.ConfigMap{}
	name := addons.PackageName(mgmtAddon.Spec.AddonName, mgmtAddon.Spec.Version)
	if err := r.c.ManagementClient.Get(ctx, client.ObjectKey{Namespace: mgmtAddon.Namespace, Name: name}, published); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, fmt.Errorf("%w: version %q of addon %q has not been published to this cluster (%s); install or update the Addon through the Gateway, which publishes it", errPackageMissing, mgmtAddon.Spec.Version, mgmtAddon.Spec.AddonName, name)
		}
		return nil, fmt.Errorf("get package %s: %w", name, err)
	}
	item, err := addons.FromPackage(published.Data)
	if err != nil {
		return nil, fmt.Errorf("package %s: %w", name, err)
	}
	rendered, err := item.Render(mgmtAddon.Spec.Values, apis.Namespace, mgmtAddon.Name, string(mgmtAddon.UID))
	if err != nil {
		return nil, err
	}
	manifests := make([][]byte, 0, len(rendered))
	kept.Labels = map[string]string{addons.LabelAddonName: mgmtAddon.Name, addons.LabelAddonUID: string(mgmtAddon.UID)}
	kept.Annotations = map[string]string{AddonGenerationAnnotation: generation}
	kept.Data = map[string][]byte{}
	for i, manifest := range rendered {
		manifests = append(manifests, manifest.Raw)
		kept.Data[fmt.Sprintf("%03d.yaml", i)] = manifest.Raw
	}
	if exists {
		err = r.c.LocalKubeClient.Update(ctx, kept)
	} else {
		err = r.c.LocalKubeClient.Create(ctx, kept)
	}
	if err != nil {
		return nil, fmt.Errorf("keep the rendering: %w", err)
	}
	return manifests, nil
}

// injectImagePullSecrets gives the Pods of an Addon workload the image registry
// credentials distributed to this cluster, matched by image name exactly as for
// a Task's Pods, so an Addon needs no pull-secret parameter of its own.
func (r *pullReconciler) injectImagePullSecrets(ctx context.Context, obj *unstructured.Unstructured) error {
	raw, found, err := unstructured.NestedMap(obj.Object, "spec", "template")
	if err != nil || !found {
		return err
	}
	template := &corev1.PodTemplateSpec{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, template); err != nil {
		return fmt.Errorf("%s/%s pod template: %w", obj.GetKind(), obj.GetName(), err)
	}
	before := len(template.Spec.ImagePullSecrets)
	if err := raykubernetes.EnsureImagePullSecrets(ctx, r.c.LocalKubeClient, template, obj.GetNamespace()); err != nil {
		return err
	}
	if len(template.Spec.ImagePullSecrets) == before {
		return nil
	}
	secrets := make([]interface{}, 0, len(template.Spec.ImagePullSecrets))
	for _, secret := range template.Spec.ImagePullSecrets {
		secrets = append(secrets, map[string]interface{}{"name": secret.Name})
	}
	return unstructured.SetNestedSlice(obj.Object, secrets, "spec", "template", "spec", "imagePullSecrets")
}

// desiredFieldsMatch ignores server-defaulted fields absent from the rendered
// object, while detecting changes to fields the Addon explicitly specifies.
func desiredFieldsMatch(desired, existing *unstructured.Unstructured) bool {
	// Kubernetes consumes Secret.stringData on write and returns only base64
	// data on read. Compare its canonical representation to avoid update loops.
	if desired.GetKind() == "Secret" {
		desired = desired.DeepCopy()
		if raw, exists := desired.Object["stringData"]; exists {
			data, found, err := unstructured.NestedMap(desired.Object, "data")
			if err != nil {
				return false
			}
			if !found {
				data = map[string]interface{}{}
			}
			if raw != nil {
				values, ok := raw.(map[string]interface{})
				if !ok {
					return false
				}
				for k, v := range values {
					value, ok := v.(string)
					if !ok {
						return false
					}
					data[k] = base64.StdEncoding.EncodeToString([]byte(value))
				}
			}
			delete(desired.Object, "stringData")
			if len(data) > 0 {
				desired.Object["data"] = data
			}
		}
	}
	for k, v := range desired.Object {
		switch k {
		case "apiVersion", "kind", "metadata", "status":
			continue
		}
		if !desiredValueMatches(v, existing.Object[k]) {
			return false
		}
	}
	return true
}

func desiredValueMatches(want, got interface{}) bool {
	switch w := want.(type) {
	case map[string]interface{}:
		g, ok := got.(map[string]interface{})
		if !ok {
			return false
		}
		for k, value := range w {
			actual, exists := g[k]
			if !exists || !desiredValueMatches(value, actual) {
				return false
			}
		}
		return true
	case []interface{}:
		g, ok := got.([]interface{})
		if !ok || len(w) != len(g) {
			return false
		}
		for i := range w {
			if !desiredValueMatches(w[i], g[i]) {
				return false
			}
		}
		return true
	default:
		return reflect.DeepEqual(want, got)
	}
}

// replacePodsOnUnselectedNodes deletes a Pod of an Addon Deployment that runs on a node its node selector no
// longer matches, so the Deployment starts it again on a node that does. The scheduler reads the selector only
// when it places a Pod; without this, moving a label (the node a site's image cache runs on, say) would leave
// the Pod where it was. A DaemonSet's controller does this by itself.
func (r *pullReconciler) replacePodsOnUnselectedNodes(ctx context.Context, mgmtAddon *rlarkv1alpha1.Addon) error {
	deployments := &appsv1.DeploymentList{}
	if err := r.c.LocalKubeClient.List(ctx, deployments, client.MatchingLabels{addons.LabelAddonName: mgmtAddon.Name}); err != nil {
		return err
	}
	for _, d := range deployments.Items {
		selector := d.Spec.Template.Spec.NodeSelector
		if len(selector) == 0 {
			continue
		}
		pods := &corev1.PodList{}
		if err := r.c.LocalKubeClient.List(ctx, pods, client.InNamespace(d.Namespace), client.MatchingLabels(d.Spec.Selector.MatchLabels)); err != nil {
			return err
		}
		for i := range pods.Items {
			pod := &pods.Items[i]
			if pod.Spec.NodeName == "" || pod.DeletionTimestamp != nil {
				continue
			}
			node := &corev1.Node{}
			if err := r.c.LocalKubeClient.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, node); client.IgnoreNotFound(err) != nil {
				return err
			}
			matches := true
			for k, v := range selector {
				if node.Labels[k] != v {
					matches = false
				}
			}
			if matches {
				continue
			}
			log.FromContext(ctx).Info("replacing a Pod on a node its selector no longer matches", "pod", pod.Name, "node", pod.Spec.NodeName)
			if err := r.c.LocalKubeClient.Delete(ctx, pod, client.Preconditions{UID: &pod.UID}); client.IgnoreNotFound(err) != nil {
				return err
			}
		}
	}
	return nil
}

// podState says why a workload's Pod is not ready: the init container still running and the last line it
// logged (a prewarm reports its progress there), a container waiting on an image pull or a crash, or the
// scheduler's reason. Empty when nothing more specific is known.
func (r *pullReconciler) podState(ctx context.Context, namespace string, selector map[string]string) string {
	pods := &corev1.PodList{}
	if err := r.c.LocalKubeClient.List(ctx, pods, client.InNamespace(namespace), client.MatchingLabels(selector)); err != nil || len(pods.Items) == 0 {
		return ""
	}
	pod := pods.Items[0]
	for _, s := range pod.Status.InitContainerStatuses {
		if s.State.Running != nil {
			return fmt.Sprintf("init container %s running since %s: %s", s.Name, s.State.Running.StartedAt.Format("15:04:05"), r.lastLogLine(ctx, &pod, s.Name))
		}
		if s.State.Waiting != nil {
			return fmt.Sprintf("init container %s %s: %s", s.Name, s.State.Waiting.Reason, s.State.Waiting.Message)
		}
	}
	for _, s := range pod.Status.ContainerStatuses {
		if s.State.Waiting != nil {
			return fmt.Sprintf("container %s %s: %s", s.Name, s.State.Waiting.Reason, s.State.Waiting.Message)
		}
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
			return "not scheduled: " + c.Message
		}
	}
	return ""
}

func (r *pullReconciler) lastLogLine(ctx context.Context, pod *corev1.Pod, container string) string {
	if r.c.LocalKubeConfig == nil {
		return ""
	}
	clientset, err := kubernetes.NewForConfig(r.c.LocalKubeConfig)
	if err != nil {
		return ""
	}
	tail := int64(1)
	raw, err := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{Container: container, TailLines: &tail}).DoRaw(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// daemonSetWaitingOn names the nodes whose Pod of ds is not ready, each with
// why: a node that is down shows as such, so a page tells "waiting for a node
// that is off" from "an image that does not pull".
func (r *pullReconciler) daemonSetWaitingOn(ctx context.Context, ds *appsv1.DaemonSet) (string, error) {
	pods := &corev1.PodList{}
	if err := r.c.LocalKubeClient.List(ctx, pods, client.InNamespace(ds.Namespace), client.MatchingLabels(ds.Spec.Selector.MatchLabels)); err != nil {
		return "", err
	}
	var waiting []string
	for _, pod := range pods.Items {
		if podReady(&pod) {
			continue
		}
		node := &corev1.Node{}
		if err := r.c.LocalKubeClient.Get(ctx, client.ObjectKey{Name: pod.Spec.NodeName}, node); err != nil {
			waiting = append(waiting, fmt.Sprintf("%s (node not found)", pod.Spec.NodeName))
			continue
		}
		waiting = append(waiting, fmt.Sprintf("%s (%s)", pod.Spec.NodeName, podWaitReason(&pod, node)))
	}
	if len(waiting) == 0 {
		return "", nil
	}
	sort.Strings(waiting)
	return "; waiting on " + strings.Join(waiting, ", "), nil
}

func podReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

func podWaitReason(pod *corev1.Pod, node *corev1.Node) string {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status != corev1.ConditionTrue {
			return "node NotReady"
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.State.Waiting != nil {
			return status.State.Waiting.Reason
		}
	}
	return string(pod.Status.Phase)
}
