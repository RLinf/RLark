package kubernetesobjects

import (
	"fmt"
	"reflect"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// Ownership classifies whether an existing compatibility workload can be managed.
type Ownership int

const (
	OwnershipConflict Ownership = iota
	OwnershipLegacy
	OwnershipCurrent
)

// WorkloadAnnotations returns the canonical compatibility identity annotations.
func WorkloadAnnotations(task *rlarkv1alpha1.Task) map[string]string {
	annotations := map[string]string{
		ManagementTaskNameAnnotation:       task.Name,
		ManagementTaskNamespaceAnnotation:  task.Namespace,
		ManagementTaskUIDAnnotation:        string(task.UID),
		ManagementTaskGenerationAnnotation: strconv.FormatInt(task.Generation, 10),
	}
	return annotations
}

// OwnershipForTask classifies current, legacy, or conflicting workload ownership.
func OwnershipForTask(object client.Object, task *rlarkv1alpha1.Task) Ownership {
	annotations := object.GetAnnotations()
	if uid := annotations[ManagementTaskUIDAnnotation]; uid != "" {
		if task.UID == "" || uid != string(task.UID) {
			return OwnershipConflict
		}
	} else {
		if name := annotations[ManagementTaskNameAnnotation]; name != "" && name != task.Name {
			return OwnershipConflict
		}
		if namespace := annotations[ManagementTaskNamespaceAnnotation]; namespace != "" && namespace != task.Namespace {
			return OwnershipConflict
		}
		return OwnershipLegacy
	}
	if name := annotations[ManagementTaskNameAnnotation]; name != "" && name != task.Name {
		return OwnershipConflict
	}
	if namespace := annotations[ManagementTaskNamespaceAnnotation]; namespace != "" && namespace != task.Namespace {
		return OwnershipConflict
	}
	return OwnershipCurrent
}

// MergeWorkloadAnnotations adds current Task identity without removing unrelated annotations.
func MergeWorkloadAnnotations(object client.Object, task *rlarkv1alpha1.Task) {
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	for key, value := range WorkloadAnnotations(task) {
		annotations[key] = value
	}
	object.SetAnnotations(annotations)
}

// AdoptWorkloadAnnotations records the generation at which a legacy workload was adopted.
func AdoptWorkloadAnnotations(object client.Object, task *rlarkv1alpha1.Task) {
	MergeWorkloadAnnotations(object, task)
	annotations := object.GetAnnotations()
	delete(annotations, ManagementTaskGenerationAnnotation)
	annotations[ManagementTaskAdoptedGenerationAnnotation] = fmt.Sprintf("%d", task.Generation)
	object.SetAnnotations(annotations)
}

// WorkloadSelectorChanged reports whether immutable workload selectors differ.
func WorkloadSelectorChanged(existing, desired client.Object) bool {
	return !reflect.DeepEqual(workloadLabelSelector(existing), workloadLabelSelector(desired))
}

// WorkloadSelectorIdentityOnlyMigration reports a legacy-to-current identity-only change.
func WorkloadSelectorIdentityOnlyMigration(existing, desired client.Object) bool {
	existingSelector, desiredSelector := workloadLabelSelector(existing), workloadLabelSelector(desired)
	if existingSelector == nil || desiredSelector == nil {
		return false
	}
	uid := desiredSelector.MatchLabels[ManagementTaskUIDLabel]
	if uid == "" || len(desiredSelector.MatchLabels) != 1 || len(desiredSelector.MatchExpressions) != 0 || existingSelector.MatchLabels[ManagementTaskUIDLabel] != "" {
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

// PrepareWorkloadTemplateUpdate preserves immutable compatibility fields before update.
func PrepareWorkloadTemplateUpdate(existing, desired client.Object) error {
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

func workloadLabelSelector(object client.Object) *metav1.LabelSelector {
	switch value := object.(type) {
	case *appsv1.Deployment:
		return value.Spec.Selector
	case *appsv1.DaemonSet:
		return value.Spec.Selector
	case *appsv1.StatefulSet:
		return value.Spec.Selector
	default:
		return nil
	}
}

func workloadTemplate(object client.Object) corev1.PodTemplateSpec {
	switch value := object.(type) {
	case *appsv1.Deployment:
		return value.Spec.Template
	case *appsv1.DaemonSet:
		return value.Spec.Template
	case *appsv1.StatefulSet:
		return value.Spec.Template
	default:
		return corev1.PodTemplateSpec{}
	}
}

func setWorkloadTemplate(object client.Object, template corev1.PodTemplateSpec) {
	switch value := object.(type) {
	case *appsv1.Deployment:
		value.Spec.Template = template
	case *appsv1.DaemonSet:
		value.Spec.Template = template
	case *appsv1.StatefulSet:
		value.Spec.Template = template
	}
}
