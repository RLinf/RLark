package server

import (
	"context"
	"fmt"
	"time"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

func (s *Server) registerAgent(ctx context.Context, agentID string) error {
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
		if err != nil {
			return fmt.Errorf("create ClusterRole %s: %w", clusterRoleName, err)
		}
	} else {
		if _, err := s.kubeClient.RbacV1().ClusterRoles().Update(ctx, clusterRole, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update ClusterRole %s: %w", clusterRoleName, err)
		}
	}

	clusterRbName := clusterRoleName + "-binding"
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
		if err != nil {
			return fmt.Errorf("create ClusterRoleBinding %s: %w", clusterRbName, err)
		}
	} else {
		if _, err := s.kubeClient.RbacV1().ClusterRoleBindings().Update(ctx, clusterRb, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update ClusterRoleBinding %s: %w", clusterRbName, err)
		}
	}

	return nil
}

func (s *Server) startAgentBroadcaster(ctx context.Context, agentID, role, connID string) error {
	// 向集群广播该 Agent 的存在信息。
	// 这主要用于：标记 Agent 的活跃状态。
	// 暂时只对主 Agent（role 为空）进行广播，其他角色的 Agent 不进行广播。

	if role != "" {
		return nil
	}

	namespace := apis.RLarkAgentNamespacePrefix + agentID
	id := "heartbeat"
	rl, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		namespace, id,
		s.kubeClient.CoreV1(),
		s.kubeClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: connID,
		},
	)
	if err != nil {
		return fmt.Errorf("create lock: %w", err)
	}
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: time.Second * 30,
		RenewDeadline: time.Second * 10,
		RetryPeriod:   time.Second * 5,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				<-ctx.Done()
			},
			OnStoppedLeading: func() {},
			OnNewLeader:      func(identity string) {},
		},
	})
	if err != nil {
		return fmt.Errorf("create leader elector: %w", err)
	}

	go le.Run(ctx)
	return nil
}
