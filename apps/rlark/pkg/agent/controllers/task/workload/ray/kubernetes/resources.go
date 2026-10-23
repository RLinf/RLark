package kubernetes

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
)

const (
	// ManagementTaskAdoptedGenerationAnnotation records the generation at legacy adoption.
	ManagementTaskAdoptedGenerationAnnotation = kubernetesobjects.ManagementTaskAdoptedGenerationAnnotation
)

type Ownership = kubernetesobjects.Ownership

const (
	OwnershipConflict = kubernetesobjects.OwnershipConflict
	OwnershipLegacy   = kubernetesobjects.OwnershipLegacy
	OwnershipCurrent  = kubernetesobjects.OwnershipCurrent
)

// TODO(execution): These helpers intentionally retain local Kubernetes client
// access while the legacy Task behavior is migrated. After equivalence and
// lifecycle tests are stable, convert reads to approved facts and return
// declarative objects for the Runtime Provider instead of performing writes.

// EnsureImagePullSecrets adds matching registry credentials to a Pod template.
func EnsureImagePullSecrets(ctx context.Context, kubeClient client.Client, template *corev1.PodTemplateSpec, namespace string) error {
	if template == nil {
		return fmt.Errorf("pod template is required")
	}
	secretList := &corev1.SecretList{}
	if err := kubeClient.List(ctx, secretList, client.InNamespace(namespace), client.MatchingLabels{common.ImageRegistryCredentialLabel: "true"}); err != nil {
		return fmt.Errorf("list image registry secrets: %w", err)
	}
	if len(secretList.Items) == 0 {
		return nil
	}
	imageRefs := make([]string, 0, len(template.Spec.Containers)+len(template.Spec.InitContainers))
	for _, container := range template.Spec.Containers {
		if container.Image != "" {
			imageRefs = append(imageRefs, container.Image)
		}
	}
	for _, container := range template.Spec.InitContainers {
		if container.Image != "" {
			imageRefs = append(imageRefs, container.Image)
		}
	}
	if len(imageRefs) == 0 {
		return nil
	}
	type credential struct{ registry, name string }
	credentials := make([]credential, 0, len(secretList.Items))
	for _, secret := range secretList.Items {
		if secret.Type != corev1.SecretTypeDockerConfigJson || len(secret.Data[corev1.DockerConfigJsonKey]) == 0 {
			continue
		}
		registry := common.NormalizeRegistry(secret.Annotations[common.ImageRegistryAnnotationRegistry])
		if registry != "" {
			credentials = append(credentials, credential{registry: registry, name: secret.Name})
		}
	}
	matched := map[string]struct{}{}
	for _, image := range imageRefs {
		image = common.NormalizeRegistry(image)
		for _, credential := range credentials {
			if strings.HasPrefix(image, credential.registry+"/") || image == credential.registry {
				matched[credential.name] = struct{}{}
			}
		}
	}
	existing := make(map[string]struct{}, len(template.Spec.ImagePullSecrets))
	for _, secret := range template.Spec.ImagePullSecrets {
		existing[secret.Name] = struct{}{}
	}
	names := make([]string, 0, len(matched))
	for name := range matched {
		if _, found := existing[name]; !found {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		template.Spec.ImagePullSecrets = append(template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
	}
	return nil
}

// OwnershipForTask classifies current, legacy, or conflicting workload ownership.
func OwnershipForTask(object client.Object, task *rlarkv1alpha1.Task) Ownership {
	return kubernetesobjects.OwnershipForTask(object, task)
}

// MergeWorkloadAnnotations adds current Task identity without removing unrelated annotations.
func MergeWorkloadAnnotations(object client.Object, task *rlarkv1alpha1.Task) {
	kubernetesobjects.MergeWorkloadAnnotations(object, task)
}

// AdoptWorkloadAnnotations records the generation at which a legacy workload was adopted.
func AdoptWorkloadAnnotations(object client.Object, task *rlarkv1alpha1.Task) {
	kubernetesobjects.AdoptWorkloadAnnotations(object, task)
}

// WorkloadSelectorChanged reports whether immutable workload selectors differ.
func WorkloadSelectorChanged(existing, desired client.Object) bool {
	return kubernetesobjects.WorkloadSelectorChanged(existing, desired)
}

// WorkloadSelectorIdentityOnlyMigration reports a legacy-to-current identity-only change.
func WorkloadSelectorIdentityOnlyMigration(existing, desired client.Object) bool {
	return kubernetesobjects.WorkloadSelectorIdentityOnlyMigration(existing, desired)
}

// PrepareWorkloadTemplateUpdate preserves immutable and compatibility fields before update.
func PrepareWorkloadTemplateUpdate(existing, desired client.Object) error {
	return kubernetesobjects.PrepareWorkloadTemplateUpdate(existing, desired)
}
