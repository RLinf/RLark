package server

import (
	"context"
	"fmt"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

func (s *Server) registerAgent(ctx context.Context, agentID string) error {
	if err := s.ensureCluster(ctx, agentID); err != nil {
		return fmt.Errorf("ensure Cluster %s: %w", agentID, err)
	}

	// 检查 agent 是否已经完成注册，如果没有，则进行注册流程。

	// 1. 创建 Kubernetes Namespace
	namespace := apis.RLarkAgentNamespacePrefix + agentID
	_, err := s.kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get namespace %s: %w", namespace, err)
		}
		_, err = s.kubeClient.CoreV1().Namespaces().Create(ctx, &v1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create namespace %s: %w", namespace, err)
		}
	}

	// 2. 创建 ServiceAccount
	saName := apis.RLarkAgentServiceAccountName
	_, err = s.kubeClient.CoreV1().ServiceAccounts(namespace).Get(ctx, saName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get serviceaccount %s/%s: %w", namespace, saName, err)
		}
		_, err = s.kubeClient.CoreV1().ServiceAccounts(namespace).Create(ctx, &v1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: namespace,
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create serviceaccount %s/%s: %w", namespace, saName, err)
		}
	}

	// 3. 创建 RBAC 规则
	// 每个 agent 有自己的 RoleBinding（名称含 agentID），避免多 agent 接入时互相覆盖 subject
	rules := []rbacv1.PolicyRule{
		{APIGroups: []string{"rlinf.io"}, Resources: []string{"nodes", "tasks", "pods", "addons"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
		{APIGroups: []string{"rlinf.io"}, Resources: []string{"nodes/status", "tasks/status", "pods/status", "addons/status"}, Verbs: []string{"get", "update", "patch"}},
		{APIGroups: []string{"rlinf.io"}, Resources: []string{"domainpeers"}, Verbs: []string{"get", "list", "watch"}},
		{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "list", "watch", "update", "patch"}},
		{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "list", "watch", "create", "update", "patch", "delete"}},
		{APIGroups: []string{"coordination.k8s.io"}, Resources: []string{"leases"}, Verbs: []string{"get", "create", "update", "patch"}},
	}
	roleName := saName
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: roleName, Namespace: namespace},
		Rules:      rules,
	}
	_, err = s.kubeClient.RbacV1().Roles(namespace).Get(ctx, roleName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get Role %s/%s: %w", namespace, roleName, err)
		}
		_, err = s.kubeClient.RbacV1().Roles(namespace).Create(ctx, role, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create Role %s/%s: %w", namespace, roleName, err)
		}
	} else {
		if _, err := s.kubeClient.RbacV1().Roles(namespace).Update(ctx, role, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update Role %s/%s: %w", namespace, roleName, err)
		}
	}

	rbName := fmt.Sprintf("%s-cluster-binding-%s", roleName, agentID)
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: rbName, Namespace: namespace},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: saName, Namespace: namespace}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: roleName, APIGroup: "rbac.authorization.k8s.io"},
	}
	_, err = s.kubeClient.RbacV1().RoleBindings(namespace).Get(ctx, rbName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get RoleBinding %s/%s: %w", namespace, rbName, err)
		}
		_, err = s.kubeClient.RbacV1().RoleBindings(namespace).Create(ctx, rb, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create RoleBinding %s/%s: %w", namespace, rbName, err)
		}
	} else {
		if _, err := s.kubeClient.RbacV1().RoleBindings(namespace).Update(ctx, rb, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update RoleBinding %s/%s: %w", namespace, rbName, err)
		}
	}

	// 4. 创建 ClusterRole 和 ClusterRoleBinding，授予 agent 集群级别的只读权限
	clusterRoleName := roleName + "-cluster"
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRoleName},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "list"}},
		},
	}
	_, err = s.kubeClient.RbacV1().ClusterRoles().Get(ctx, clusterRoleName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get ClusterRole %s: %w", clusterRoleName, err)
		}
		_, err = s.kubeClient.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create ClusterRole %s: %w", clusterRoleName, err)
		}
	} else {
		if _, err := s.kubeClient.RbacV1().ClusterRoles().Update(ctx, clusterRole, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update ClusterRole %s: %w", clusterRoleName, err)
		}
	}

	clusterRbName := fmt.Sprintf("%s-binding-%s", clusterRoleName, agentID)
	clusterRb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: clusterRbName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: saName, Namespace: namespace}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: clusterRoleName, APIGroup: "rbac.authorization.k8s.io"},
	}
	_, err = s.kubeClient.RbacV1().ClusterRoleBindings().Get(ctx, clusterRbName, metav1.GetOptions{})
	if err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("get ClusterRoleBinding %s: %w", clusterRbName, err)
		}
		_, err = s.kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, clusterRb, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create ClusterRoleBinding %s: %w", clusterRbName, err)
		}
	} else {
		if _, err := s.kubeClient.RbacV1().ClusterRoleBindings().Update(ctx, clusterRb, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update ClusterRoleBinding %s: %w", clusterRbName, err)
		}
	}

	return nil
}

func (s *Server) ensureCluster(ctx context.Context, agentID string) error {
	clusterID := apis.RLarkAgentNamespacePrefix + agentID
	clusterClient := s.rlarkClient.RlinfV1alpha1().Clusters()
	clusters, err := clusterClient.List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Clusters: %w", err)
	}
	matchingClusters := make([]*rlarkv1alpha1.Cluster, 0, 1)
	for i := range clusters.Items {
		cluster := &clusters.Items[i]
		if cluster.Spec.AgentNamespace == clusterID {
			matchingClusters = append(matchingClusters, cluster)
		}
	}
	switch len(matchingClusters) {
	case 0:
		existingCluster, getErr := clusterClient.Get(ctx, clusterID, metav1.GetOptions{})
		if getErr == nil {
			return s.migrateClusterNodes(ctx, existingCluster)
		}
		if !errors.IsNotFound(getErr) {
			return fmt.Errorf("get Cluster: %w", getErr)
		}
	case 1:
		if matchingClusters[0].Name == clusterID {
			return s.migrateClusterNodes(ctx, matchingClusters[0])
		}
		log.FromContext(ctx).Info("Reusing legacy Cluster for agent namespace", "agentID", agentID, "cluster", matchingClusters[0].Name, "agentNamespace", clusterID)
		return s.migrateClusterNodes(ctx, matchingClusters[0])
	default:
		matchingNames := make([]string, 0, len(matchingClusters))
		for _, cluster := range matchingClusters {
			matchingNames = append(matchingNames, cluster.Name)
		}
		log.FromContext(ctx).Error(nil, "Conflicting Clusters share an agent namespace", "agentID", agentID, "agentNamespace", clusterID, "clusters", matchingNames)
		return fmt.Errorf("conflicting Clusters for agent namespace %q: %v", clusterID, matchingNames)
	}

	cluster, err := clusterClient.Create(ctx, &rlarkv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: clusterID},
		Spec: rlarkv1alpha1.ClusterSpec{
			AgentNamespace: clusterID,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create Cluster: %w", err)
		}
		cluster, err = clusterClient.Get(ctx, clusterID, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get concurrently created Cluster %s: %w", clusterID, err)
		}
		if cluster.Spec.AgentNamespace != clusterID {
			return fmt.Errorf("concurrently created Cluster %s has agent namespace %q, expected %q", clusterID, cluster.Spec.AgentNamespace, clusterID)
		}
	}

	return s.migrateClusterNodes(ctx, cluster)
}

func (s *Server) migrateClusterNodes(ctx context.Context, cluster *rlarkv1alpha1.Cluster) error {
	managementNamespace := cluster.Spec.AgentNamespace
	if managementNamespace == "" {
		return nil
	}
	nodes, err := s.rlarkClient.RlinfV1alpha1().Nodes(managementNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Nodes in management namespace %s: %w", managementNamespace, err)
	}
	for i := range nodes.Items {
		node := &nodes.Items[i]
		labels := make(map[string]string, len(node.Labels))
		for key, value := range node.Labels {
			labels[key] = value
		}
		if labels[rlarkv1alpha1.LabelClusterID] == cluster.Name {
			continue
		}
		labels[rlarkv1alpha1.LabelClusterID] = cluster.Name
		node.Labels = labels
		if _, err := s.rlarkClient.RlinfV1alpha1().Nodes(node.Namespace).Update(ctx, node, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("migrate Node %s/%s: %w", node.Namespace, node.Name, err)
		}
	}
	return nil
}

func (s *Server) startAgentBroadcaster(ctx context.Context, agentID, role, connID string) error {
	// 主 Agent 的 Lease 表示集群连接状态；节点 Agent 的 Lease 表示单个节点连接状态。
	id := "heartbeat"
	if role != "" {
		nodeName, ok := strings.CutPrefix(role, "node-agent:")
		if !ok || nodeName == "" {
			return nil
		}
		id = "node-heartbeat-" + nodeName
	}

	namespace := apis.RLarkAgentNamespacePrefix + agentID
	leConfig := configs.DefaultLeaderElectionConfig()
	leConfig.Key = namespace + "/" + id
	leConfig.Identity = connID
	leConfig.LeaseDuration = 45 * time.Second
	leConfig.RenewDeadline = 30 * time.Second
	leConfig.RetryPeriod = 15 * time.Second
	electionConfig, err := leConfig.Build(s.kubeClient, namespace, leaderelection.LeaderCallbacks{
		OnStartedLeading: func(ctx context.Context) {
			<-ctx.Done()
		},
		OnStoppedLeading: func() {},
		OnNewLeader:      func(identity string) {},
	})
	if err != nil {
		return fmt.Errorf("build leader election config: %w", err)
	}
	le, err := leaderelection.NewLeaderElector(electionConfig)
	if err != nil {
		return fmt.Errorf("create leader elector: %w", err)
	}

	go le.Run(ctx)
	return nil
}
