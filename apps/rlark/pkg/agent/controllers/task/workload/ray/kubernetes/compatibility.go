package kubernetes

import (
	"fmt"
	"os"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const (
	// ManagementTaskNameAnnotation records the logical management Task name.
	ManagementTaskNameAnnotation = kubernetesobjects.ManagementTaskNameAnnotation
	// ManagementTaskNamespaceAnnotation records the logical management Task namespace.
	ManagementTaskNamespaceAnnotation = kubernetesobjects.ManagementTaskNamespaceAnnotation
	// ManagementTaskGenerationAnnotation records the compiled management Task generation.
	ManagementTaskGenerationAnnotation = kubernetesobjects.ManagementTaskGenerationAnnotation
	// ManagementTaskUIDAnnotation records the management Task object incarnation.
	ManagementTaskUIDAnnotation = kubernetesobjects.ManagementTaskUIDAnnotation
	// ManagementTaskUIDLabel selects objects belonging to a Task incarnation.
	ManagementTaskUIDLabel = kubernetesobjects.ManagementTaskUIDLabel
	// UnsafeTaskPrivilegesEnv enables legacy privileged Task mutations when true.
	UnsafeTaskPrivilegesEnv = "RLARK_ENABLE_UNSAFE_TASK_PRIVILEGES"
)

// TODO(execution): This package deliberately keeps the remaining legacy
// Kubernetes Task mutations together while behavior is frozen. Move Ray
// semantics into explicit Adapter-owned configuration, credential and storage
// concerns into Environment bindings, and unsafe privileges into plan policy.

// WorkloadAnnotations returns compatibility identity annotations for a Task workload.
func WorkloadAnnotations(task *rlarkv1alpha1.Task) map[string]string {
	return kubernetesobjects.WorkloadAnnotations(task)
}

// ApplyWorkloadIdentity adds Task and tenant identity to a Pod template.
func ApplyWorkloadIdentity(template *corev1.PodTemplateSpec, task *rlarkv1alpha1.Task) {
	if template.Labels == nil {
		template.Labels = map[string]string{}
	}
	template.Labels[ManagementTaskUIDLabel] = string(task.UID)
	template.Labels[tenant.IDLabel] = tenant.IDOf(task)
	if template.Annotations == nil {
		template.Annotations = map[string]string{}
	}
	template.Annotations[ManagementTaskUIDAnnotation] = string(task.UID)
}

// WorkloadSelector returns the immutable selector for Pods of a Task incarnation.
func WorkloadSelector(task *rlarkv1alpha1.Task) *metav1.LabelSelector {
	return &metav1.LabelSelector{MatchLabels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}
}

// ApplyTemplateMutations applies the frozen legacy mutation sequence to a Pod template.
func ApplyTemplateMutations(template *corev1.PodTemplateSpec, task *rlarkv1alpha1.Task) {
	applyTaskIdentityAnnotations(template, task)
	ensureLabels(template, task.Name)
	applyNodeSelector(&template.Spec, task.Spec.Placement.NodeSelector)
	applyUnschedulableNodeExclusion(&template.Spec)
	applyAntiAffinity(template)
	if os.Getenv(UnsafeTaskPrivilegesEnv) != "true" {
		return
	}
	role := ""
	if task.Annotations != nil {
		role = task.Annotations[rlarkv1alpha1.RayRoleAnnotation]
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

// BuildDeployment compiles a Kubernetes Task spec into a legacy-compatible Deployment.
func BuildDeployment(task *rlarkv1alpha1.Task, namespace string, spec *rlarkv1alpha1.TaskWorkloadSpec, replicas *int32) *appsv1.Deployment {
	result := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        task.Name,
			Namespace:   namespace,
			Annotations: WorkloadAnnotations(task),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: replicas,
			Selector: WorkloadSelector(task),
			Template: *spec.Template.DeepCopy(),
		},
	}
	tenant.Inherit(task, result)
	return result
}

// BuildStatefulSet compiles a Kubernetes Task spec into a legacy-compatible StatefulSet.
func BuildStatefulSet(task *rlarkv1alpha1.Task, namespace string, spec *rlarkv1alpha1.TaskWorkloadSpec, replicas *int32) *appsv1.StatefulSet {
	result := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        task.Name,
			Namespace:   namespace,
			Annotations: WorkloadAnnotations(task),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            replicas,
			Selector:            WorkloadSelector(task),
			Template:            *spec.Template.DeepCopy(),
			PodManagementPolicy: appsv1.ParallelPodManagement,
		},
	}
	tenant.Inherit(task, result)
	return result
}

// BuildRayHeadService returns the Task-incarnation-owned Service used to discover a Ray head.
func BuildRayHeadService(namespace string, task *rlarkv1alpha1.Task) *corev1.Service {
	annotations := WorkloadAnnotations(task)
	delete(annotations, ManagementTaskGenerationAnnotation)
	delete(annotations, ManagementTaskAdoptedGenerationAnnotation)
	result := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: RayHeadServiceName(task.Name), Namespace: namespace, Labels: map[string]string{"app": task.Name, "prometheus.io/scrape": "true", "prometheus.io/port": "8080"}, Annotations: annotations}, Spec: corev1.ServiceSpec{ClusterIP: "None", PublishNotReadyAddresses: true, Selector: map[string]string{"app": task.Name}, Ports: []corev1.ServicePort{{Name: "ray", Port: 6379, Protocol: corev1.ProtocolTCP}, {Name: "dashboard", Port: 8265, Protocol: corev1.ProtocolTCP}, {Name: "metrics", Port: 8080, Protocol: corev1.ProtocolTCP}}}}
	tenant.Inherit(task, result)
	return result
}

// RayHeadServiceName returns the deterministic legacy Ray head Service name.
func RayHeadServiceName(taskName string) string {
	return fmt.Sprintf("%s-ray-head", taskName)
}

func ensureLabels(template *corev1.PodTemplateSpec, name string) {
	if template == nil || len(template.Labels) == 0 {
		template.Labels = map[string]string{"app": name}
	}
}

func applyTaskIdentityAnnotations(template *corev1.PodTemplateSpec, task *rlarkv1alpha1.Task) {
	if template.Annotations == nil {
		template.Annotations = map[string]string{}
	}
	template.Annotations[ManagementTaskNameAnnotation] = task.Name
	template.Annotations[ManagementTaskNamespaceAnnotation] = task.Namespace
}

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
	template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = append(template.Spec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution, corev1.PodAffinityTerm{TopologyKey: "kubernetes.io/hostname", LabelSelector: &metav1.LabelSelector{MatchLabels: template.Labels}})
}

func applyUnschedulableNodeExclusion(podSpec *corev1.PodSpec) {
	if podSpec.Affinity == nil {
		podSpec.Affinity = &corev1.Affinity{}
	}
	if podSpec.Affinity.NodeAffinity == nil {
		podSpec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	selector := podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	created := false
	if selector == nil {
		selector = &corev1.NodeSelector{}
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = selector
		created = true
	}
	requirement := corev1.NodeSelectorRequirement{
		Key:      "rlark.io/unschedulable",
		Operator: corev1.NodeSelectorOpNotIn,
		Values:   []string{"true"},
	}
	if len(selector.NodeSelectorTerms) == 0 {
		if created {
			selector.NodeSelectorTerms = []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{requirement}}}
		}
		return
	}
	// Required node affinity terms are ORed. Adding an independent term would
	// bypass any existing constraint, so merge the requirement into every term.
	for i := range selector.NodeSelectorTerms {
		term := &selector.NodeSelectorTerms[i]
		if hasNodeSelectorRequirement(*term, requirement) {
			continue
		}
		term.MatchExpressions = append(term.MatchExpressions, requirement)
	}
}

func hasNodeSelectorRequirement(term corev1.NodeSelectorTerm, requirement corev1.NodeSelectorRequirement) bool {
	for _, existing := range term.MatchExpressions {
		if existing.Key != requirement.Key || existing.Operator != requirement.Operator || len(existing.Values) != len(requirement.Values) {
			continue
		}
		matched := true
		for i := range existing.Values {
			if existing.Values[i] != requirement.Values[i] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func applyNodeSelector(podSpec *corev1.PodSpec, selector map[string]string) {
	if podSpec.NodeSelector == nil {
		podSpec.NodeSelector = map[string]string{}
	}
	podSpec.NodeSelector["rlark.io/enabled"] = "true"
	for key, value := range selector {
		podSpec.NodeSelector[key] = value
	}
	// A multi-value placement selector can only be expressed through
	// nodeAffinity: a comma separated value is not a valid Kubernetes label
	// value, so leaving it in NodeSelector makes the API server reject the
	// whole Pod template. Normalize every such key currently on the template,
	// because the compatibility decode may already have merged the Task
	// placement into it before this mutation runs.
	keys := make([]string, 0, len(podSpec.NodeSelector))
	for key, value := range podSpec.NodeSelector {
		if strings.Contains(value, ",") {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return
	}
	sort.Strings(keys)
	requirements := make([]corev1.NodeSelectorRequirement, 0, len(keys))
	for _, key := range keys {
		values := strings.Split(podSpec.NodeSelector[key], ",")
		for i := range values {
			values[i] = strings.TrimSpace(values[i])
		}
		requirements = append(requirements, corev1.NodeSelectorRequirement{Key: key, Operator: corev1.NodeSelectorOpIn, Values: values})
		delete(podSpec.NodeSelector, key)
	}
	if podSpec.Affinity == nil {
		podSpec.Affinity = &corev1.Affinity{}
	}
	if podSpec.Affinity.NodeAffinity == nil {
		podSpec.Affinity.NodeAffinity = &corev1.NodeAffinity{}
	}
	if podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &corev1.NodeSelector{}
	}
	required := podSpec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution
	required.NodeSelectorTerms = append(required.NodeSelectorTerms, corev1.NodeSelectorTerm{MatchExpressions: requirements})
}
