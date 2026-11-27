package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/gin-gonic/gin"
	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The heartbeat Lease is what makes a cluster read Online. It is kept for an agent whose tunnel says it is
// ready, and not for one that is only connected.
func TestHeartbeatOnlyForAReadyAgent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	namespace := apis.RLarkAgentNamespacePrefix + "cluster-a"
	connect := func(t *testing.T, ready string) *fake.Clientset {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		kubeClient := fake.NewSimpleClientset()
		server := NewServer(DefaultConfig())
		server.kubeClient = kubeClient
		server.rlarkClient = clientset.NewSimpleClientset()
		ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
		ginContext.Request = httptest.NewRequest(http.MethodGet, "/api/connect", nil).WithContext(ctx)
		ginContext.Set(ContextKeyCertMeta, map[string]string{apis.MetaAgentID: "cluster-a", apis.MetaRemoteDialerClientID: "cluster-a"})
		if ready != "" {
			ginContext.Request.Header.Set(apis.RemoteDialerReadyHeader, ready)
		}
		server.handleProxyConnect(ginContext)
		return kubeClient
	}

	ready := connect(t, "true")
	require.Eventually(t, func() bool {
		_, err := ready.CoordinationV1().Leases(namespace).Get(context.Background(), "heartbeat", metav1.GetOptions{})
		return err == nil
	}, 5*time.Second, 20*time.Millisecond, "a ready agent has no heartbeat")

	for _, said := range []string{"false", ""} {
		notReady := connect(t, said)
		// The agent is registered all the same: its first watches need the namespace and permissions.
		_, err := notReady.CoreV1().Namespaces().Get(context.Background(), namespace, metav1.GetOptions{})
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond)
		leases, err := notReady.CoordinationV1().Leases(namespace).List(context.Background(), metav1.ListOptions{})
		require.NoError(t, err)
		assert.Empty(t, leases.Items, "an agent that said %q has a heartbeat", said)
	}
}

// The agent reads its own Cluster (spec.nodeAgents) and no other.
func TestRegisterAgentGrantsItsOwnCluster(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	server := &Server{kubeClient: client, rlarkClient: clientset.NewSimpleClientset()}
	require.NoError(t, server.registerAgent(ctx, "cluster-a"))

	namespace := apis.RLarkAgentNamespacePrefix + "cluster-a"
	readerName := apis.RLarkAgentServiceAccountName + "-cluster-cluster-a"
	reader, err := client.RbacV1().ClusterRoles().Get(ctx, readerName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []rbacv1.PolicyRule{{APIGroups: []string{"rlinf.io"}, Resources: []string{"clusters"}, ResourceNames: []string{namespace}, Verbs: []string{"get", "list", "watch"}}}, reader.Rules)
	binding, err := client.RbacV1().ClusterRoleBindings().Get(ctx, readerName, metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []rbacv1.Subject{{Kind: "ServiceAccount", Name: apis.RLarkAgentServiceAccountName, Namespace: namespace}}, binding.Subjects)
	assert.Equal(t, readerName, binding.RoleRef.Name)
}
