package server

import (
	"context"
	"slices"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAgentGrantsDeliverySecretAndStatusPermissions(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	server := &Server{kubeClient: client}
	require.NoError(t, server.registerAgent(ctx, "cluster-a"))

	namespace := apis.RLarkAgentNamespacePrefix + "cluster-a"
	role, err := client.RbacV1().Roles(namespace).Get(ctx, apis.RLarkAgentServiceAccountName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, ruleAllows(role.Rules, "", "secrets", "get", "list", "watch", "update", "patch"))
	assert.True(t, ruleAllows(role.Rules, "", "configmaps", "get", "list", "watch", "create", "update", "patch", "delete"))

	require.NoError(t, server.registerAgent(ctx, "cluster-a"))
	role, err = client.RbacV1().Roles(namespace).Get(ctx, apis.RLarkAgentServiceAccountName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.True(t, ruleAllows(role.Rules, "", "secrets", "patch"))
}

func ruleAllows(rules []rbacv1.PolicyRule, group, resource string, verbs ...string) bool {
	for _, rule := range rules {
		if !slices.Contains(rule.APIGroups, group) || !slices.Contains(rule.Resources, resource) {
			continue
		}
		for _, verb := range verbs {
			if !slices.Contains(rule.Verbs, verb) {
				return false
			}
		}
		return true
	}
	return false
}
