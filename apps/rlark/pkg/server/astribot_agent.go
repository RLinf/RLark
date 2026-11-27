package server

import (
	"context"
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/leaderelection"
)

// grantOwnClusterReader lets the agent read its own Cluster and no other: the cluster Agent keeps its cluster's
// node Agent DaemonSet as spec.nodeAgents declares it. Cluster is cluster-scoped, so the grant names it (in its
// canonical form the Cluster is named as the agent's namespace).
func (s *Server) grantOwnClusterReader(ctx context.Context, agentID, namespace, saName string) error {
	readerName := fmt.Sprintf("%s-cluster-%s", saName, agentID)
	reader := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: readerName},
		Rules: []rbacv1.PolicyRule{
			{APIGroups: []string{"rlinf.io"}, Resources: []string{"clusters"}, ResourceNames: []string{namespace}, Verbs: []string{"get", "list", "watch"}},
		},
	}
	if _, err := s.kubeClient.RbacV1().ClusterRoles().Update(ctx, reader, metav1.UpdateOptions{}); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("update ClusterRole %s: %w", readerName, err)
		}
		if _, err := s.kubeClient.RbacV1().ClusterRoles().Create(ctx, reader, metav1.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create ClusterRole %s: %w", readerName, err)
		}
	}
	readerBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: readerName},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: saName, Namespace: namespace}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: readerName, APIGroup: "rbac.authorization.k8s.io"},
	}
	if _, err := s.kubeClient.RbacV1().ClusterRoleBindings().Update(ctx, readerBinding, metav1.UpdateOptions{}); err != nil {
		if !errors.IsNotFound(err) {
			return fmt.Errorf("update ClusterRoleBinding %s: %w", readerName, err)
		}
		if _, err := s.kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, readerBinding, metav1.CreateOptions{}); err != nil && !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create ClusterRoleBinding %s: %w", readerName, err)
		}
	}
	return nil
}

// keepLeading runs le until ctx ends. Run returns when a renewal misses RenewDeadline (the management API
// stalled): the lease is taken again, where the official code gave it up for good (an agent's node stayed Offline
// until the agent reconnected; the server stopped broadcasting).
func keepLeading(ctx context.Context, le *leaderelection.LeaderElector) {
	for ctx.Err() == nil {
		le.Run(ctx)
	}
}
