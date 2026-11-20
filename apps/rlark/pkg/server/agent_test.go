package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"

	"github.com/gin-gonic/gin"
	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterAgentGrantsDeliverySecretAndStatusPermissions(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	rlarkClient := clientset.NewSimpleClientset()
	server := &Server{kubeClient: client, rlarkClient: rlarkClient}
	require.NoError(t, server.registerAgent(ctx, "cluster-a"))

	clusterID := apis.RLarkAgentNamespacePrefix + "cluster-a"
	cluster, err := rlarkClient.RlinfV1alpha1().Clusters().Get(ctx, clusterID, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, clusterID, cluster.Name)
	assert.Equal(t, rlarkv1alpha1.ClusterSpec{AgentNamespace: clusterID}, cluster.Spec)

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

func TestAgentConnectionMigratesLegacyCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	kubeClient := fake.NewSimpleClientset()
	rlarkClient := clientset.NewSimpleClientset()
	server := NewServer(Config{})
	server.kubeClient = kubeClient
	server.rlarkClient = rlarkClient

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/connect", nil)
	ginContext, _ := gin.CreateTestContext(response)
	ginContext.Request = request
	ginContext.Set(ContextKeyCertMeta, map[string]string{
		apis.MetaAgentID:              "legacy-cluster",
		apis.MetaRemoteDialerClientID: "legacy-cluster",
	})
	ginContext.Request = ginContext.Request.WithContext(ctx)
	ginContext.Request.Header.Set(apis.RemoteDialerRoleHeader, "unsupported-role")
	server.handleProxyConnect(ginContext)

	clusterID := apis.RLarkAgentNamespacePrefix + "legacy-cluster"
	cluster, err := rlarkClient.RlinfV1alpha1().Clusters().Get(ctx, clusterID, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, clusterID, cluster.Name)
	assert.Equal(t, rlarkv1alpha1.ClusterSpec{AgentNamespace: clusterID}, cluster.Spec)
	_, err = kubeClient.CoreV1().Namespaces().Get(ctx, apis.RLarkAgentNamespacePrefix+"legacy-cluster", metav1.GetOptions{})
	require.NoError(t, err)
}

func TestRegisterAgentReusesLegacyCluster(t *testing.T) {
	ctx := context.Background()
	legacyNamespace := apis.RLarkAgentNamespacePrefix + "dev-guangdong-d"
	kubeClient := fake.NewSimpleClientset()
	rlarkClient := clientset.NewSimpleClientset(
		&rlarkv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "dev-guangdong-d"},
			Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: legacyNamespace},
		},
		&rlarkv1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker-1",
				Namespace: legacyNamespace,
				Labels: map[string]string{
					rlarkv1alpha1.LabelClusterID: legacyNamespace,
				},
			},
		})
	server := &Server{kubeClient: kubeClient, rlarkClient: rlarkClient}

	require.NoError(t, server.registerAgent(ctx, "dev-guangdong-d"))
	cluster, err := rlarkClient.RlinfV1alpha1().Clusters().Get(ctx, "dev-guangdong-d", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, legacyNamespace, cluster.Spec.AgentNamespace)
	_, err = rlarkClient.RlinfV1alpha1().Clusters().Get(ctx, legacyNamespace, metav1.GetOptions{})
	require.Error(t, err)
	node, err := rlarkClient.RlinfV1alpha1().Nodes(legacyNamespace).Get(ctx, "worker-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "dev-guangdong-d", node.Labels[rlarkv1alpha1.LabelClusterID])
}

func TestRegisterAgentMigratesLegacyNodesForExistingCluster(t *testing.T) {
	ctx := context.Background()
	legacyNamespace := apis.RLarkAgentNamespacePrefix + "dev-guangdong-d"
	kubeClient := fake.NewSimpleClientset()
	rlarkClient := clientset.NewSimpleClientset(
		&rlarkv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: legacyNamespace},
			Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: legacyNamespace},
		},
		&rlarkv1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker-1",
				Namespace: legacyNamespace,
				Labels:    map[string]string{rlarkv1alpha1.LabelClusterID: legacyNamespace},
			},
		},
		&rlarkv1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "worker-2",
				Namespace: legacyNamespace,
				Labels:    map[string]string{rlarkv1alpha1.LabelClusterID: "stale-cluster"},
			},
		},
	)
	server := &Server{kubeClient: kubeClient, rlarkClient: rlarkClient}

	require.NoError(t, server.registerAgent(ctx, "dev-guangdong-d"))
	node, err := rlarkClient.RlinfV1alpha1().Nodes(legacyNamespace).Get(ctx, "worker-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, legacyNamespace, node.Labels[rlarkv1alpha1.LabelClusterID])
	node, err = rlarkClient.RlinfV1alpha1().Nodes(legacyNamespace).Get(ctx, "worker-2", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, legacyNamespace, node.Labels[rlarkv1alpha1.LabelClusterID])
}

func TestEnsureClusterCompensatesAfterConcurrentCreate(t *testing.T) {
	ctx := context.Background()
	agentID := "cluster-a"
	agentNamespace := apis.RLarkAgentNamespacePrefix + agentID
	rlarkClient := clientset.NewSimpleClientset(&rlarkv1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "worker-1",
			Namespace: agentNamespace,
			Labels:    map[string]string{rlarkv1alpha1.LabelClusterID: "stale-cluster"},
		},
	})
	rlarkClient.PrependReactor("create", "clusters", func(action clientgotesting.Action) (bool, k8sruntime.Object, error) {
		cluster := &rlarkv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: agentNamespace},
			Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: agentNamespace},
		}
		return true, cluster, apierrors.NewAlreadyExists(rlarkv1alpha1.Resource("clusters"), agentNamespace)
	})
	rlarkClient.PrependReactor("get", "clusters", func(action clientgotesting.Action) (bool, k8sruntime.Object, error) {
		return true, &rlarkv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: agentNamespace},
			Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: agentNamespace},
		}, nil
	})
	server := &Server{rlarkClient: rlarkClient}

	require.NoError(t, server.ensureCluster(ctx, agentID))
	node, err := rlarkClient.RlinfV1alpha1().Nodes(agentNamespace).Get(ctx, "worker-1", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, agentNamespace, node.Labels[rlarkv1alpha1.LabelClusterID])
}

func TestEnsureClusterRejectsDuplicateAgentNamespace(t *testing.T) {
	ctx := context.Background()
	agentID := "dev-guangdong-d"
	agentNamespace := apis.RLarkAgentNamespacePrefix + agentID
	rlarkClient := clientset.NewSimpleClientset(
		&rlarkv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "dev-guangdong-d"},
			Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: agentNamespace},
		},
		&rlarkv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: agentNamespace},
			Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: agentNamespace},
		},
	)
	server := &Server{rlarkClient: rlarkClient}

	err := server.ensureCluster(ctx, agentID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "conflicting Clusters")
}

func TestEnsureClusterPreservesExistingSpec(t *testing.T) {
	ctx := context.Background()
	clusterID := apis.RLarkAgentNamespacePrefix + "cluster-a"
	rlarkClient := clientset.NewSimpleClientset(&rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterID},
		Spec:       rlarkv1alpha1.ClusterSpec{AgentNamespace: "custom-agent-namespace", Cordoned: true},
	})
	server := &Server{rlarkClient: rlarkClient}

	require.NoError(t, server.ensureCluster(ctx, "cluster-a"))
	cluster, err := rlarkClient.RlinfV1alpha1().Clusters().Get(ctx, clusterID, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, rlarkv1alpha1.ClusterSpec{AgentNamespace: "custom-agent-namespace", Cordoned: true}, cluster.Spec)
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
