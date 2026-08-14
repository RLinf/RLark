package kubernetes

import (
	"context"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Uninstall uninstalls the components.
func (d *Installer) Uninstall(cfg *types.DeployConfig, purge bool) error {
	logger := log.GetLogger()
	kubeconfig := cfg.Kubernetes.Kubeconfig
	if kubeconfig == "" {
		kubeconfig = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("build kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create clientset: %w", err)
	}
	ctx := context.Background()

	comps := component.ComponentsForPlane(cfg)
	for i := len(comps) - 1; i >= 0; i-- {
		c := comps[i]
		if err := deleteWorkload(ctx, clientset, &c); err != nil {
			return err
		}
		if svc := component.Service(cfg, &c); svc != nil {
			if err := deleteService(ctx, clientset, svc.Name); err != nil {
				return err
			}
		}
		if err := deleteRBAC(ctx, clientset, &c); err != nil {
			return err
		}
		logger.Info("component removed", "name", c.Name)
	}

	if cfg.Plane == types.PlaneControl {
		for _, cmName := range []string{"rlark-db-config", "rlark-postgres-init", "rlark-kcp-kubeconfig"} {
			if err := deleteConfigMap(ctx, clientset, cmName); err != nil {
				return err
			}
		}
		if err := deleteSecret(ctx, clientset, "rlark-tls"); err != nil {
			return err
		}
	} else {
		if err := deleteSecret(ctx, clientset, "rlark-agent-cert"); err != nil {
			return err
		}
	}

	if purge {
		if err := clientset.CoreV1().Namespaces().Delete(ctx, constants.Namespace, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return fmt.Errorf("delete namespace %s: %w", constants.Namespace, err)
		}
		logger.Info("namespace deleted", "name", constants.Namespace)
	}

	logger.Info("plane uninstalled", "plane", cfg.Plane, "namespace", constants.Namespace)
	return nil
}

func deleteWorkload(ctx context.Context, clientset *kubernetes.Clientset, c *types.Component) error {
	switch c.WorkloadKind {
	case "DaemonSet":
		return deleteDaemonSet(ctx, clientset, c.Name)
	case "StatefulSet":
		return deleteStatefulSet(ctx, clientset, c.Name)
	default:
		return deleteDeployment(ctx, clientset, c.Name)
	}
}

func deleteStatefulSet(ctx context.Context, clientset *kubernetes.Clientset, name string) error {
	err := clientset.AppsV1().StatefulSets(constants.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete statefulset %s: %w", name, err)
	}
	return nil
}

func deleteDeployment(ctx context.Context, clientset *kubernetes.Clientset, name string) error {
	err := clientset.AppsV1().Deployments(constants.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete deployment %s: %w", name, err)
	}
	return nil
}

func deleteDaemonSet(ctx context.Context, clientset *kubernetes.Clientset, name string) error {
	err := clientset.AppsV1().DaemonSets(constants.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete daemonset %s: %w", name, err)
	}
	return nil
}

func deleteService(ctx context.Context, clientset *kubernetes.Clientset, name string) error {
	err := clientset.CoreV1().Services(constants.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete service %s: %w", name, err)
	}
	return nil
}

func deleteConfigMap(ctx context.Context, clientset *kubernetes.Clientset, name string) error {
	err := clientset.CoreV1().ConfigMaps(constants.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete configmap %s: %w", name, err)
	}
	return nil
}

func deleteSecret(ctx context.Context, clientset *kubernetes.Clientset, name string) error {
	err := clientset.CoreV1().Secrets(constants.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete secret %s: %w", name, err)
	}
	return nil
}

func deleteRBAC(ctx context.Context, clientset *kubernetes.Clientset, c *types.Component) error {
	sa, cr, crb := component.RBAC(c)
	if sa == nil {
		return nil
	}

	if err := clientset.CoreV1().ServiceAccounts(constants.Namespace).Delete(ctx, sa.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete serviceaccount %s: %w", sa.Name, err)
	}
	if err := clientset.RbacV1().ClusterRoleBindings().Delete(ctx, crb.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete clusterrolebinding %s: %w", crb.Name, err)
	}
	if err := clientset.RbacV1().ClusterRoles().Delete(ctx, cr.Name, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("delete clusterrole %s: %w", cr.Name, err)
	}
	return nil
}
