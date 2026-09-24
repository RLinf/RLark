package replication

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

type restrictedPolicy struct{}

func (restrictedPolicy) Validate(_ context.Context, _ *corev1.Secret, _ *meta.RESTMapping, _ distribution.NamespaceIdentity, object *unstructured.Unstructured, apply distribution.ApplyPolicy) error {
	if apply.AdoptExisting || apply.ConflictPolicy == "Force" {
		return fmt.Errorf("adoption and force apply are disabled")
	}
	if object.GetKind() == "Secret" {
		secretType, _, _ := unstructured.NestedString(object.Object, "type")
		if secretType == string(corev1.SecretTypeServiceAccountToken) {
			return fmt.Errorf("service account token secrets are denied")
		}
	}
	return nil
}
