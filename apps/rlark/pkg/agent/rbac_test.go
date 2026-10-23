package agent

import (
	"io"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestQuickstartAgentTerminalRBAC(t *testing.T) {
	manifest, err := os.Open("../../docs/examples/agent-rbac.yaml")
	require.NoError(t, err)
	defer func() { require.NoError(t, manifest.Close()) }()

	var account *corev1.ServiceAccount
	var role *rbacv1.ClusterRole
	var binding *rbacv1.ClusterRoleBinding
	decoder := yaml.NewYAMLOrJSONDecoder(manifest, 4096)
	for {
		var raw runtime.RawExtension
		err := decoder.Decode(&raw)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(raw.Raw, nil, nil)
		require.NoError(t, err)
		switch obj := obj.(type) {
		case *corev1.ServiceAccount:
			account = obj
		case *rbacv1.ClusterRole:
			role = obj
		case *rbacv1.ClusterRoleBinding:
			binding = obj
		}
	}
	require.NotNil(t, account)
	require.NotNil(t, role)
	require.NotNil(t, binding)
	require.Equal(t, "rlark-agent", account.Name)
	require.Equal(t, "rlark-system", account.Namespace)
	require.Equal(t, rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: role.Name}, binding.RoleRef)
	require.Contains(t, binding.Subjects, rbacv1.Subject{Kind: "ServiceAccount", Name: account.Name, Namespace: account.Namespace})

	// handleTerminal uses POST pods/exec. Ordinary Pod access does not grant
	// access to this subresource, and exec does not need mutation verbs other
	// than create.
	for _, tt := range []struct {
		verb string
		want bool
	}{
		{"create", true},
		{"delete", false},
		{"patch", false},
		{"update", false},
	} {
		t.Run(tt.verb, func(t *testing.T) {
			allowed := false
			for _, rule := range role.Rules {
				if len(rule.ResourceNames) == 0 &&
					(slices.Contains(rule.APIGroups, "") || slices.Contains(rule.APIGroups, "*")) &&
					(slices.Contains(rule.Resources, "pods/exec") || slices.Contains(rule.Resources, "*")) &&
					(slices.Contains(rule.Verbs, tt.verb) || slices.Contains(rule.Verbs, "*")) {
					allowed = true
				}
			}
			require.Equal(t, tt.want, allowed)
		})
	}
}
