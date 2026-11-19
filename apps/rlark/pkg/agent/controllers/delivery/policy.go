package delivery

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

type restrictedPolicy struct{}

func (restrictedPolicy) Validate(_ context.Context, _ *corev1.Secret, mapping *meta.RESTMapping, _ distribution.NamespaceIdentity, object *unstructured.Unstructured, apply distribution.ApplyPolicy) error {
	if apply.AdoptExisting || apply.ConflictPolicy == "Force" {
		return fmt.Errorf("adoption and force apply are disabled")
	}
	groupResource := mapping.Resource.GroupResource().String()
	switch groupResource {
	case "nodes", "serviceaccounts/token", "certificatesigningrequests.certificates.k8s.io", "apiservices.apiregistration.k8s.io",
		"mutatingwebhookconfigurations.admissionregistration.k8s.io", "validatingwebhookconfigurations.admissionregistration.k8s.io",
		"clusterroles.rbac.authorization.k8s.io", "clusterrolebindings.rbac.authorization.k8s.io", "customresourcedefinitions.apiextensions.k8s.io":
		return fmt.Errorf("resource %s is denied", groupResource)
	}
	if object.GetKind() == "Secret" {
		secretType, _, _ := unstructured.NestedString(object.Object, "type")
		if secretType == string(corev1.SecretTypeServiceAccountToken) {
			return fmt.Errorf("service account token secrets are denied")
		}
	}
	return nil
}
