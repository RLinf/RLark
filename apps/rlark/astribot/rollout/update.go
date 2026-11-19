// Package rollout replaces the Pods of the DaemonSets RLark places.
package rollout

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// templateGenerationLabel is the label the DaemonSet controller gives each Pod:
// the template generation it was made from.
const templateGenerationLabel = "pod-template-generation"

// A node's in-use annotation is reported on a change and may be a few seconds
// behind its machine, a Pod becoming available after its minReadySeconds is no
// event, and a missed event must not leave a Pod outdated: while any is, the
// DaemonSet is looked at again this often.
const pendingInterval = 30 * time.Second

// Updater replaces the Pods of every DaemonSet RLark places: node Agents,
// per-node Tasks, the DaemonSets of Addons, the Station components. Such a
// DaemonSet carries rlark.io/rollout and replaces nothing by itself
// (OnDelete); the Updater deletes an outdated Pod and the DaemonSet
// controller starts the new one.
//
// Only nodes that are Ready count. The DaemonSet controller's own rolling
// update counts the Pod of a node that is off as unavailable, deletes it, and
// waits for a deletion no kubelet is there to carry out: one such node held
// every other node's update. Here that Pod is neither deleted nor counted,
// and is replaced when its node is back.
//
// The first nodes to get a new template are the policy's canary; until their
// new Pods are available no other is touched, so a template that does not
// work stops at one node. After that, as many are replaced at once as the
// policy's maxUnavailable leaves among the ready nodes; a new Pod that does
// not become available uses up the bound like any other, which is what stops
// a rollout that breaks later. With whenIdle a Pod is replaced only on a node
// that reports it is not in use.
//
// A Pod is deleted only once its node holds the new template's images. Until
// then a pull Pod stands on the node: the template's images with
// IfNotPresent, the template's pull secrets and network, and a command that
// does not exist, so kubelet pulls them as it will for the new Pod, and starts
// nothing. The old Pod runs meanwhile: a template whose Pods do not pull
// (imagePullPolicy Never), or whose node reaches its registry only through the
// old Pod (a Station's tunnel), is replaced on a node that has what the new
// Pod runs. A pull that fails keeps the old Pod; its pull Pod says why.
type Updater struct {
	Local client.Client
	// Now is the clock a Pod's minReadySeconds are read against.
	Now func() time.Time
}

// Setup reconciles such a DaemonSet when it, one of its Pods, or a node's
// readiness or in-use annotation changes.
func (u *Updater) Setup(localManager ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(localManager).
		Named("daemonset-rollout").
		For(&appsv1.DaemonSet{}, builder.WithPredicates(predicate.NewPredicateFuncs(func(object client.Object) bool {
			return Rolled(object.(*appsv1.DaemonSet))
		}))).
		// Its Pods and its pull Pods (owned, not controlled: the DaemonSet controller would release them).
		Watches(&corev1.Pod{}, handler.EnqueueRequestForOwner(localManager.GetScheme(), localManager.GetRESTMapper(), &appsv1.DaemonSet{})).
		Watches(&corev1.Node{}, handler.EnqueueRequestsFromMapFunc(u.daemonSets), builder.WithPredicates(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				before, after := e.ObjectOld.(*corev1.Node), e.ObjectNew.(*corev1.Node)
				return Idle(before) != Idle(after) || nodeReady(before) != nodeReady(after)
			},
		})).
		Complete(u)
}

// Rolled reports whether RLark replaces daemonSet's Pods.
func Rolled(daemonSet *appsv1.DaemonSet) bool {
	_, annotated := daemonSet.Annotations[rlarkv1alpha1.RolloutAnnotation]
	return annotated && daemonSet.Spec.UpdateStrategy.Type == appsv1.OnDeleteDaemonSetStrategyType
}

// Roll makes daemonSet one whose Pods RLark replaces as policy says.
func Roll(daemonSet *appsv1.DaemonSet, policy rlarkv1alpha1.RolloutPolicy) {
	if daemonSet.Annotations == nil {
		daemonSet.Annotations = map[string]string{}
	}
	daemonSet.Annotations[rlarkv1alpha1.RolloutAnnotation] = policy.Annotation()
	daemonSet.Spec.UpdateStrategy = appsv1.DaemonSetUpdateStrategy{Type: appsv1.OnDeleteDaemonSetStrategyType}
}

// Idle reports whether node's machine says it is not in use. A node whose
// node Agent has not reported yet is not idle.
func Idle(node *corev1.Node) bool {
	value, reported := node.Annotations[rlarkv1alpha1.NodeInUseAnnotation]
	return reported && value == ""
}

func nodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// podAvailable reports whether pod has been ready for minReadySeconds.
func podAvailable(pod *corev1.Pod, minReadySeconds int32, now time.Time) bool {
	if pod.DeletionTimestamp != nil {
		return false
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue && !condition.LastTransitionTime.Add(time.Duration(minReadySeconds)*time.Second).After(now)
		}
	}
	return false
}

func (u *Updater) daemonSets(ctx context.Context, _ client.Object) []reconcile.Request {
	var list appsv1.DaemonSetList
	if err := u.Local.List(ctx, &list); err != nil {
		log.FromContext(ctx).Error(err, "list DaemonSets for a node change")
		return nil
	}
	var requests []reconcile.Request
	for i := range list.Items {
		if Rolled(&list.Items[i]) {
			requests = append(requests, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
		}
	}
	return requests
}

// Reconcile deletes the outdated Pods of one DaemonSet that may be replaced now.
func (u *Updater) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	logger := log.FromContext(ctx).WithValues("daemonset", request.NamespacedName)
	var daemonSet appsv1.DaemonSet
	if err := u.Local.Get(ctx, request.NamespacedName, &daemonSet); err != nil {
		return reconcile.Result{}, client.IgnoreNotFound(err)
	}
	if !Rolled(&daemonSet) || daemonSet.DeletionTimestamp != nil {
		return reconcile.Result{}, nil
	}
	policy, err := rlarkv1alpha1.ParseRolloutPolicy(daemonSet.Annotations[rlarkv1alpha1.RolloutAnnotation])
	if err != nil {
		return reconcile.Result{}, reconcile.TerminalError(err)
	}
	// Until the DaemonSet controller has seen the current template, its Pods'
	// generation labels and its status describe the one before.
	if daemonSet.Status.ObservedGeneration < daemonSet.Generation {
		return reconcile.Result{RequeueAfter: time.Second}, nil
	}
	var pods corev1.PodList
	if err := u.Local.List(ctx, &pods, client.InNamespace(daemonSet.Namespace), client.MatchingLabels(daemonSet.Spec.Selector.MatchLabels)); err != nil {
		return reconcile.Result{}, fmt.Errorf("list the DaemonSet's Pods: %w", err)
	}
	now := time.Now()
	if u.Now != nil {
		now = u.Now()
	}
	generation := daemonSet.Annotations[appsv1.DeprecatedTemplateGeneration]
	nodes := map[string]*corev1.Node{}
	var broken, replaceable []*corev1.Pod
	// Among the Pods on ready nodes: readyNodes of them in all, updated that
	// run the current template and are available, inFlight that are on their
	// way to it (being deleted, or new and not available yet), held that are
	// outdated, do not run, and are on a node in use.
	owned, readyNodes, updated, inFlight, held, outdated := 0, 0, 0, 0, 0, 0
	for i := range pods.Items {
		pod := &pods.Items[i]
		if !metav1.IsControlledBy(pod, &daemonSet) {
			continue
		}
		owned++
		node, found := nodes[pod.Spec.NodeName]
		if !found {
			node = &corev1.Node{}
			if err := u.Local.Get(ctx, types.NamespacedName{Name: pod.Spec.NodeName}, node); err != nil {
				if !apierrors.IsNotFound(err) {
					return reconcile.Result{}, fmt.Errorf("get node %s: %w", pod.Spec.NodeName, err)
				}
				node = nil
			}
			nodes[pod.Spec.NodeName] = node
		}
		old := pod.Labels[templateGenerationLabel] != generation
		if old {
			outdated++
		}
		if node == nil || !nodeReady(node) {
			continue
		}
		readyNodes++
		available := podAvailable(pod, daemonSet.Spec.MinReadySeconds, now)
		switch {
		case !old && available:
			updated++
		case !old || pod.DeletionTimestamp != nil:
			inFlight++
		case available:
			if !policy.WhenIdle || Idle(node) {
				replaceable = append(replaceable, pod)
			}
		default:
			// The old template, and not running anyway.
			if !policy.WhenIdle || Idle(node) {
				broken = append(broken, pod)
			} else {
				held++
			}
		}
	}
	pulls, err := u.pulls(ctx, &daemonSet, outdated > 0)
	if err != nil {
		return reconcile.Result{}, err
	}
	if outdated == 0 {
		return reconcile.Result{}, nil
	}
	// A node whose Pod was deleted and is not started again yet has no Pod to
	// count, but is as much on its way as one being replaced.
	if missing := int(daemonSet.Status.DesiredNumberScheduled) - owned; missing > 0 {
		inFlight += missing
	}
	sort.Slice(replaceable, func(i, j int) bool { return replaceable[i].Spec.NodeName < replaceable[j].Spec.NodeName })
	var replace []*corev1.Pod
	if canary := policy.CanaryNodes(); updated < canary {
		// The canary is a Pod that ran: one that did not run before says
		// nothing of the new template when it does not run after.
		candidates := append(replaceable, broken...)
		replace = candidates[:max(0, min(canary-updated-inFlight, len(candidates)))]
	} else {
		bound, err := policy.Unavailable(readyNodes)
		if err != nil {
			return reconcile.Result{}, reconcile.TerminalError(fmt.Errorf("read maxUnavailable: %w", err))
		}
		// An outdated Pod that does not run anyway costs nothing to replace;
		// the ones that do run go within what the bound leaves.
		replace = append(broken, replaceable[:max(0, min(bound-inFlight-held-len(broken), len(replaceable)))]...)
	}
	for _, pod := range replace {
		pull := pulls[pod.Spec.NodeName]
		if waiting, err := u.pulled(ctx, &daemonSet, nodes[pod.Spec.NodeName], pull); err != nil || waiting != "" {
			if err != nil {
				return reconcile.Result{}, err
			}
			logger.Info("the node does not hold the new template's images yet", "node", pod.Spec.NodeName, "pull", waiting)
			continue
		}
		if err := u.Local.Delete(ctx, pod, client.Preconditions{UID: &pod.UID}); client.IgnoreNotFound(err) != nil {
			return reconcile.Result{}, fmt.Errorf("delete outdated Pod %s: %w", pod.Name, err)
		}
		logger.Info("replacing an outdated Pod", "pod", pod.Name, "node", pod.Spec.NodeName, "templateGeneration", generation, "updated", updated, "readyNodes", readyNodes)
		if pull != nil {
			if err := u.Local.Delete(ctx, pull); client.IgnoreNotFound(err) != nil {
				return reconcile.Result{}, fmt.Errorf("delete pull Pod %s: %w", pull.Name, err)
			}
		}
	}
	return reconcile.Result{RequeueAfter: pendingInterval}, nil
}

// The labels of a pull Pod: the DaemonSet it pulls for and the template generation whose images it pulls.
const (
	PullForLabel      = "rlark.io/pull-for"
	pullTemplateLabel = "rlark.io/pull-template"
	// pullOnly is the pull Pod's command: no image has it, so its containers end at their start.
	pullOnly = "/rlark-pull-only"
)

// pulls are daemonSet's pull Pods for its current template by node. Those for another template, and all of them
// once nothing is outdated (keep false), are deleted.
func (u *Updater) pulls(ctx context.Context, daemonSet *appsv1.DaemonSet, keep bool) (map[string]*corev1.Pod, error) {
	var list corev1.PodList
	if err := u.Local.List(ctx, &list, client.InNamespace(daemonSet.Namespace), client.MatchingLabels{PullForLabel: daemonSet.Name}); err != nil {
		return nil, fmt.Errorf("list the pull Pods: %w", err)
	}
	pulls := map[string]*corev1.Pod{}
	for i := range list.Items {
		pull := &list.Items[i]
		if keep && pull.Labels[pullTemplateLabel] == daemonSet.Annotations[appsv1.DeprecatedTemplateGeneration] {
			pulls[pull.Spec.NodeName] = pull
			continue
		}
		if err := u.Local.Delete(ctx, pull); client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("delete pull Pod %s: %w", pull.Name, err)
		}
	}
	return pulls, nil
}

// templateImages are the images daemonSet's template runs, each once.
func templateImages(daemonSet *appsv1.DaemonSet) []string {
	var images []string
	for _, container := range slices.Concat(daemonSet.Spec.Template.Spec.InitContainers, daemonSet.Spec.Template.Spec.Containers) {
		if !slices.Contains(images, container.Image) {
			images = append(images, container.Image)
		}
	}
	return images
}

// holds reports whether node lists image among the ones it has (kubelet lists the largest few dozen: one it
// does not list may still be there, and its pull Pod says so at once).
func holds(node *corev1.Node, image string) bool {
	_, digest, byDigest := strings.Cut(image, "@")
	for _, held := range node.Status.Images {
		if slices.ContainsFunc(held.Names, func(name string) bool {
			return name == image || byDigest && strings.HasSuffix(name, "@"+digest)
		}) {
			return true
		}
	}
	return false
}

// pulled is "" once node holds every image of daemonSet's template, else what its pull Pod waits on; a node
// without a pull Pod gets one.
func (u *Updater) pulled(ctx context.Context, daemonSet *appsv1.DaemonSet, node *corev1.Node, pull *corev1.Pod) (string, error) {
	images := templateImages(daemonSet)
	if !slices.ContainsFunc(images, func(image string) bool { return !holds(node, image) }) {
		return "", nil
	}
	if pull == nil {
		if err := u.Local.Create(ctx, pullPod(daemonSet, node.Name, images)); client.IgnoreAlreadyExists(err) != nil {
			return "", fmt.Errorf("create the pull Pod on %s: %w", node.Name, err)
		}
		return "pull Pod made", nil
	}
	if len(pull.Status.ContainerStatuses) < len(pull.Spec.Containers) {
		return fmt.Sprintf("pull Pod %s", pull.Status.Phase), nil
	}
	var waiting []string
	for _, status := range pull.Status.ContainerStatuses {
		// A container that ran or ended was made from its image: the image is on the node.
		if status.State.Waiting != nil {
			waiting = append(waiting, fmt.Sprintf("%s %s %s", status.Image, status.State.Waiting.Reason, status.State.Waiting.Message))
		}
	}
	return strings.Join(waiting, "; "), nil
}

// pullPod is a Pod on node that has kubelet pull images as daemonSet's Pods pull them, and runs nothing.
func pullPod(daemonSet *appsv1.DaemonSet, node string, images []string) *corev1.Pod {
	template := daemonSet.Spec.Template.Spec
	containers := make([]corev1.Container, 0, len(images))
	for i, image := range images {
		containers = append(containers, corev1.Container{Name: fmt.Sprintf("image-%d", i), Image: image, ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{pullOnly}})
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: daemonSet.Name + "-pull" + daemonSet.Annotations[appsv1.DeprecatedTemplateGeneration] + "-" + node, Namespace: daemonSet.Namespace,
			Labels:          map[string]string{PullForLabel: daemonSet.Name, pullTemplateLabel: daemonSet.Annotations[appsv1.DeprecatedTemplateGeneration]},
			OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: daemonSet.Name, UID: daemonSet.UID}},
		},
		Spec: corev1.PodSpec{
			NodeName: node, RestartPolicy: corev1.RestartPolicyNever, HostNetwork: template.HostNetwork,
			ImagePullSecrets: template.ImagePullSecrets, Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			AutomountServiceAccountToken: new(false), TerminationGracePeriodSeconds: new(int64(0)), Containers: containers,
		},
	}
}
