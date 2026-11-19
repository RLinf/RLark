package release

import (
	"github.com/rlinf/rlark/apps/rlark/astribot/identity"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// identityDefinitions are what each of RLark's own clients of the management plane may do there, as this release's
// rlarkadm states it (identity.RLark): the ClusterRole of its rules and its binding. The release writes them with the
// CRDs, so a component that comes with new permissions has them before it starts (11-17: a Gateway that makes
// clusters, without its role for clusterprovisions). The ServiceAccount and its token are rlarkadm identity's, which
// a release does not touch; Station's are in its management RBAC.
func identityDefinitions() ([]*unstructured.Unstructured, error) {
	var out []*unstructured.Unstructured
	add := func(object runtime.Object) error {
		data, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if err != nil {
			return err
		}
		out = append(out, &unstructured.Unstructured{Object: data})
		return nil
	}
	for _, c := range identity.RLark() {
		role := c.ClusterRole
		if c.Rules != nil {
			role = c.Name
			if err := add(&rbacv1.ClusterRole{
				TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRole"},
				ObjectMeta: metav1.ObjectMeta{Name: role}, Rules: c.Rules,
			}); err != nil {
				return nil, err
			}
		}
		if err := add(&rbacv1.ClusterRoleBinding{
			TypeMeta:   metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding"},
			ObjectMeta: metav1.ObjectMeta{Name: c.Name},
			RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role},
			Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: c.ServiceAccount, Namespace: c.Namespace}},
		}); err != nil {
			return nil, err
		}
	}
	return out, nil
}
