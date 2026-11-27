package kubernetes

import (
	"context"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// instanceNode is what a per-node Task's instance reports of its node.
type instanceNode struct {
	// notReady: the node is down, or no longer exists. It tells an instance that is down from one whose node is: a
	// per-node Task does not wait for nodes that are switched off.
	notReady bool
	// inUse is the node's in-use annotation ("not reported" when the node has none): an instance that updates only
	// when idle waits for it without holding the Task back.
	inUse string
}

// instanceNodes reads the nodes of a DaemonSet's Pods, adding each node it finds to the objects the snapshot's
// revision is derived from. Other carriers' Pods are rescheduled off nodes that are down, so their nodes are not
// read.
func (p *Provider) instanceNodes(ctx context.Context, object client.Object, pods []corev1.Pod, objectsForRevision *[]client.Object) (map[string]instanceNode, error) {
	nodes := map[string]instanceNode{}
	if _, perNode := object.(*appsv1.DaemonSet); !perNode {
		return nodes, nil
	}
	for i := range pods {
		name := pods[i].Spec.NodeName
		if _, seen := nodes[name]; seen || name == "" {
			continue
		}
		var node corev1.Node
		if err := p.client.Get(ctx, types.NamespacedName{Name: name}, &node); err != nil {
			if !apierrors.IsNotFound(err) {
				return nil, fault.Wrap(fault.Internal, "get instance node", err)
			}
			nodes[name] = instanceNode{notReady: true}
			continue
		}
		*objectsForRevision = append(*objectsForRevision, &node)
		state := instanceNode{notReady: true}
		if value, reported := node.Annotations[rlarkv1alpha1.NodeInUseAnnotation]; !reported {
			state.inUse = "not reported"
		} else {
			state.inUse = value
		}
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				state.notReady = false
			}
		}
		nodes[name] = state
	}
	return nodes, nil
}

// perNodeConditions are what a per-node Task reports for each instance besides the official conditions: its node's
// state, the template generation it was made from (the DaemonSet controller's pod-template-generation label, which
// tells an updated instance from an old one), its readiness and its containers' images. Like the official node
// condition they travel as conditions, so the runtime-neutral Instance and its transport need no Kubernetes fields.
func perNodeConditions(pod *corev1.Pod, node instanceNode) []agentruntime.InstanceCondition {
	var conditions []agentruntime.InstanceCondition
	if node.notReady {
		conditions = append(conditions, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "node", Subject: pod.Spec.NodeName, Reason: "NotReady"})
	}
	if node.inUse != "" {
		conditions = append(conditions, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "node", Subject: pod.Spec.NodeName, Reason: "InUse", Message: node.inUse})
	}
	if generation, found := pod.Labels["pod-template-generation"]; found {
		conditions = append(conditions, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "revision", Subject: pod.Name, Message: generation})
	}
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			conditions = append(conditions, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "instance", Subject: pod.Name, Reason: "Ready"})
		}
	}
	for _, container := range pod.Spec.Containers {
		conditions = append(conditions, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "image", Subject: container.Name, Message: container.Image})
	}
	return conditions
}

// keepPlatformAdditions is the official normalization of the Pod defaults the API server fills in
// (preserveObservedPodDefaults), with what the platform adds kept as the workload has it. What the platform adds is
// not a reason to replace running Pods: the image of its containers (the cluster Agent's --image), and, while the
// Task is the one the workload was written for (canonicalGeneration) and all its Pods are ready, the image registry
// credentials the Agent gives by image name (a credential that comes to match, or is deleted). With those as the
// workload has them, a template that is otherwise as desired is left as it is. Pods that are replaced anyway,
// because the Task changed, take the platform's current ones, as every new Pod does; so does a workload with a Pod
// that is not ready, which may be waiting for the very credential.
func keepPlatformAdditions(current, desired client.Object, canonicalGeneration bool) {
	currentTemplate, desiredTemplate := workloadTemplatePointer(current), workloadTemplatePointer(desired)
	asRunning := desiredTemplate.DeepCopy()
	keepPlatformImages(currentTemplate, asRunning)
	allReady := false
	switch existing := current.(type) {
	case *appsv1.Deployment:
		allReady = existing.Status.ReadyReplicas == existing.Status.Replicas
	case *appsv1.DaemonSet:
		allReady = existing.Status.NumberReady == existing.Status.DesiredNumberScheduled
	case *appsv1.StatefulSet:
		allReady = existing.Status.ReadyReplicas == existing.Status.Replicas
	}
	if canonicalGeneration && allReady {
		asRunning.Spec.ImagePullSecrets = currentTemplate.Spec.ImagePullSecrets
	}
	preserveObservedPodDefaults(currentTemplate, asRunning)
	if reflect.DeepEqual(currentTemplate, asRunning) {
		*desiredTemplate = *asRunning
	} else {
		preserveObservedPodDefaults(currentTemplate, desiredTemplate)
	}
}

// keepPlatformImages gives desired's platform containers the images they have in current.
func keepPlatformImages(current, desired *corev1.PodTemplateSpec) {
	images := map[string]string{}
	for _, container := range append(append([]corev1.Container(nil), current.Spec.InitContainers...), current.Spec.Containers...) {
		if kubernetesobjects.PlatformContainer(container.Name) {
			images[container.Name] = container.Image
		}
	}
	for _, containers := range [][]corev1.Container{desired.Spec.InitContainers, desired.Spec.Containers} {
		for i := range containers {
			if image, runs := images[containers[i].Name]; runs {
				containers[i].Image = image
			}
		}
	}
}

// takeRollout gives the DaemonSet the Task's way of rolling it (its carrier's rollout policy) and reports whether
// that changed: a change to it is applied like a template change.
func takeRollout(existing, want *appsv1.DaemonSet) bool {
	existing.Annotations[rlarkv1alpha1.RolloutAnnotation] = want.Annotations[rlarkv1alpha1.RolloutAnnotation]
	changed := !reflect.DeepEqual(existing.Spec.UpdateStrategy, want.Spec.UpdateStrategy)
	existing.Spec.UpdateStrategy = want.Spec.UpdateStrategy
	return changed
}
