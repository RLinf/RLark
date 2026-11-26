// Package kubernetes provides the Kubernetes implementation of the Upgrader interface.
package kubernetes

import (
	"context"
	"fmt"
	"reflect"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/health"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Upgrader upgrades RLark components on Kubernetes.
type Upgrader struct {
	clientset  *kubernetes.Clientset
	kubeconfig string
	summary    *types.UpgradeSummary
}

// New creates a new Kubernetes Upgrader.
// The kubeconfig path comes from the deploy config. When it is empty or the
// file does not exist (e.g. rlark-upgrade running inside a Pod, where a host
// path is meaningless), fall back to the in-cluster config so the Pod's
// ServiceAccount is used.
func New(cfg *types.DeployConfig) (*Upgrader, error) {
	kubeconfig := cfg.Kubernetes.Kubeconfig
	if kubeconfig == "" {
		kubeconfig = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		// The kubeconfig file is unavailable (e.g. a host path inside a Pod).
		// Fall back to the in-cluster config.
		restConfig, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("build kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create clientset: %w", err)
	}

	return &Upgrader{
		clientset:  clientset,
		kubeconfig: kubeconfig,
	}, nil
}

// PreCheck performs pre-upgrade checks.
func (u *Upgrader) PreCheck(ctx context.Context, cfg *types.DeployConfig) error {
	return u.preCheck(ctx, cfg, false)
}

// PreCheckForRollback performs pre-upgrade checks for a rollback.
// It skips the component health check because the component may be unhealthy
// (that is why the rollback is being performed).
func (u *Upgrader) PreCheckForRollback(ctx context.Context, cfg *types.DeployConfig) error {
	return u.preCheck(ctx, cfg, true)
}

func (u *Upgrader) preCheck(ctx context.Context, cfg *types.DeployConfig, skipHealth bool) error {
	// Check namespace exists (also verifies cluster connectivity)
	if err := u.checkNamespace(ctx); err != nil {
		return fmt.Errorf("namespace: %w", err)
	}

	// Check all components exist and are healthy
	for _, c := range component.ComponentsForPlane(cfg) {
		if err := u.checkComponentExists(ctx, &c); err != nil {
			return err
		}
		if !skipHealth {
			if err := u.checkComponentHealthy(ctx, &c); err != nil {
				return fmt.Errorf("component %s not healthy: %w", c.Name, err)
			}
		}
	}

	// Check node readiness
	if err := u.checkNodeReadiness(ctx); err != nil {
		return fmt.Errorf("node readiness: %w", err)
	}

	return nil
}

// checkNamespace verifies the target namespace exists.
func (u *Upgrader) checkNamespace(ctx context.Context) error {
	_, err := u.clientset.CoreV1().Namespaces().Get(ctx, constants.Namespace, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("namespace %s not found: %w", constants.Namespace, err)
	}
	return nil
}

// checkComponentHealthy verifies a component is healthy.
func (u *Upgrader) checkComponentHealthy(ctx context.Context, c *types.Component) error {
	// Check if pods are running
	pods, err := u.clientset.CoreV1().Pods(constants.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", c.Name),
	})
	if err != nil {
		return err
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found")
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			return fmt.Errorf("pod %s is %s", pod.Name, pod.Status.Phase)
		}
	}

	return nil
}

// checkNodeReadiness verifies enough nodes are ready.
func (u *Upgrader) checkNodeReadiness(ctx context.Context) error {
	nodes, err := u.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	readyCount := 0
	for _, node := range nodes.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
				readyCount++
				break
			}
		}
	}

	if readyCount == 0 {
		return fmt.Errorf("no ready nodes found")
	}

	log.GetLogger().Info("node readiness", "ready", readyCount, "total", len(nodes.Items))
	return nil
}

// PreUpgrade performs pre-upgrade preparation.
func (u *Upgrader) PreUpgrade(ctx context.Context, cfg *types.DeployConfig) error {
	return nil
}

// Upgrade upgrades the components by generating resource manifests and updating existing resources.
func (u *Upgrader) Upgrade(ctx context.Context, cfg *types.DeployConfig) error {
	logger := log.GetLogger()

	// Inject runtime dependencies into context
	ctx = runtime.WithClientset(ctx, u.clientset)
	ctx = runtime.WithKubeconfig(ctx, u.kubeconfig)

	components := component.ComponentsForPlane(cfg)
	for _, c := range components {
		logger.Info("upgrading component", "name", c.Name)

		if err := u.upgradeComponent(ctx, cfg, &c); err != nil {
			return fmt.Errorf("upgrade component %s: %w", c.Name, err)
		}

		// Wait for healthy using context-injected clientset
		if err := health.WaitForHealthy(ctx, cfg, c); err != nil {
			return fmt.Errorf("health check failed for %s: %w", c.Name, err)
		}

		// Post-deploy hook (e.g. KCP CRD install/update)
		if c.PostDeployFn != nil {
			if err := c.PostDeployFn(ctx, cfg); err != nil {
				return fmt.Errorf("post-deploy for %s: %w", c.Name, err)
			}
		}

		logger.Info("component upgraded", "name", c.Name)
	}

	logger.Info("all components upgraded")
	return nil
}

// PostUpgrade performs post-upgrade tasks.
func (u *Upgrader) PostUpgrade(ctx context.Context, cfg *types.DeployConfig) error {
	u.summary = u.buildSummary(ctx, cfg)
	return nil
}

// Summary returns the upgrade summary.
func (u *Upgrader) Summary() *types.UpgradeSummary {
	return u.summary
}

// checkComponentExists checks if a component workload exists.
func (u *Upgrader) checkComponentExists(ctx context.Context, c *types.Component) error {
	var err error
	switch c.WorkloadKind {
	case "DaemonSet":
		_, err = u.clientset.AppsV1().DaemonSets(constants.Namespace).Get(ctx, c.Name, metav1.GetOptions{})
	case "StatefulSet":
		_, err = u.clientset.AppsV1().StatefulSets(constants.Namespace).Get(ctx, c.Name, metav1.GetOptions{})
	default:
		_, err = u.clientset.AppsV1().Deployments(constants.Namespace).Get(ctx, c.Name, metav1.GetOptions{})
	}
	if err != nil {
		return fmt.Errorf("component %s not found, please run 'install' first: %w", c.Name, err)
	}
	return nil
}

// upgradeComponent updates a component's RBAC, workload, service, and PVCs to match the generated manifests.
func (u *Upgrader) upgradeComponent(ctx context.Context, cfg *types.DeployConfig, c *types.Component) error {
	// Update RBAC if needed
	if err := u.upgradeRBAC(ctx, cfg, c); err != nil {
		return fmt.Errorf("upgrade RBAC: %w", err)
	}

	// Update PVCs if needed (e.g., storage expansion)
	if err := u.upgradePVCs(ctx, cfg, c); err != nil {
		return fmt.Errorf("upgrade PVCs: %w", err)
	}

	// Update Service if needed
	if err := u.upgradeService(ctx, cfg, c); err != nil {
		return fmt.Errorf("upgrade service: %w", err)
	}

	// Update Workload
	switch c.WorkloadKind {
	case "DaemonSet":
		return u.upgradeDaemonSet(ctx, cfg, c)
	case "StatefulSet":
		return u.upgradeStatefulSet(ctx, cfg, c)
	default:
		return u.upgradeDeployment(ctx, cfg, c)
	}
}

// upgradeRBAC updates RBAC resources if needed.
func (u *Upgrader) upgradeRBAC(ctx context.Context, cfg *types.DeployConfig, c *types.Component) error {
	sa, cr, crb := component.RBAC(cfg, c)
	if sa == nil {
		return nil
	}

	// Update ServiceAccount
	if err := u.upgradeServiceAccount(ctx, sa); err != nil {
		return err
	}

	// Update ClusterRole
	if err := u.upgradeClusterRole(ctx, cr); err != nil {
		return err
	}

	// Update ClusterRoleBinding
	if err := u.upgradeClusterRoleBinding(ctx, crb); err != nil {
		return err
	}

	return nil
}

// upgradeServiceAccount updates a ServiceAccount if needed.
func (u *Upgrader) upgradeServiceAccount(ctx context.Context, desired *corev1.ServiceAccount) error {
	logger := log.GetLogger()
	existing, err := u.clientset.CoreV1().ServiceAccounts(constants.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = u.clientset.CoreV1().ServiceAccounts(constants.Namespace).Create(ctx, desired, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("create serviceaccount %s: %w", desired.Name, err)
			}
			logger.Info("serviceaccount created", "name", desired.Name)
			return nil
		}
		return fmt.Errorf("get serviceaccount %s: %w", desired.Name, err)
	}

	// Update if labels or annotations changed
	if !reflect.DeepEqual(existing.Labels, desired.Labels) || !reflect.DeepEqual(existing.Annotations, desired.Annotations) {
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations
		_, err = u.clientset.CoreV1().ServiceAccounts(constants.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update serviceaccount %s: %w", desired.Name, err)
		}
		logger.Info("serviceaccount updated", "name", desired.Name)
	}

	return nil
}

// upgradeClusterRole updates a ClusterRole if needed.
func (u *Upgrader) upgradeClusterRole(ctx context.Context, desired *rbacv1.ClusterRole) error {
	logger := log.GetLogger()
	existing, err := u.clientset.RbacV1().ClusterRoles().Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = u.clientset.RbacV1().ClusterRoles().Create(ctx, desired, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("create clusterrole %s: %w", desired.Name, err)
			}
			logger.Info("clusterrole created", "name", desired.Name)
			return nil
		}
		return fmt.Errorf("get clusterrole %s: %w", desired.Name, err)
	}

	// Update if rules changed
	if !reflect.DeepEqual(existing.Rules, desired.Rules) {
		existing.Rules = desired.Rules
		_, err = u.clientset.RbacV1().ClusterRoles().Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update clusterrole %s: %w", desired.Name, err)
		}
		logger.Info("clusterrole updated", "name", desired.Name)
	}

	return nil
}

// upgradeClusterRoleBinding updates a ClusterRoleBinding if needed.
func (u *Upgrader) upgradeClusterRoleBinding(ctx context.Context, desired *rbacv1.ClusterRoleBinding) error {
	logger := log.GetLogger()
	existing, err := u.clientset.RbacV1().ClusterRoleBindings().Get(ctx, desired.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = u.clientset.RbacV1().ClusterRoleBindings().Create(ctx, desired, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("create clusterrolebinding %s: %w", desired.Name, err)
			}
			logger.Info("clusterrolebinding created", "name", desired.Name)
			return nil
		}
		return fmt.Errorf("get clusterrolebinding %s: %w", desired.Name, err)
	}

	// Update if subjects or roleRef changed
	if !reflect.DeepEqual(existing.Subjects, desired.Subjects) || !reflect.DeepEqual(existing.RoleRef, desired.RoleRef) {
		existing.Subjects = desired.Subjects
		existing.RoleRef = desired.RoleRef
		_, err = u.clientset.RbacV1().ClusterRoleBindings().Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update clusterrolebinding %s: %w", desired.Name, err)
		}
		logger.Info("clusterrolebinding updated", "name", desired.Name)
	}

	return nil
}

// upgradePVCs updates PVCs if needed (e.g., storage expansion).
func (u *Upgrader) upgradePVCs(ctx context.Context, cfg *types.DeployConfig, c *types.Component) error {
	logger := log.GetLogger()
	if c.VolumeClaimFn == nil {
		return nil
	}

	desiredClaims := c.VolumeClaimFn(cfg)
	for _, desired := range desiredClaims {
		existing, err := u.clientset.CoreV1().PersistentVolumeClaims(constants.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				_, err = u.clientset.CoreV1().PersistentVolumeClaims(constants.Namespace).Create(ctx, &desired, metav1.CreateOptions{})
				if err != nil {
					return fmt.Errorf("create PVC %s: %w", desired.Name, err)
				}
				logger.Info("PVC created", "name", desired.Name)
				continue
			}
			return fmt.Errorf("get PVC %s: %w", desired.Name, err)
		}

		desiredSize := desired.Spec.Resources.Requests[corev1.ResourceStorage]
		existingSize := existing.Spec.Resources.Requests[corev1.ResourceStorage]

		if desiredSize.Cmp(existingSize) > 0 {
			logger.Info("expanding PVC", "name", desired.Name, "from", existingSize.String(), "to", desiredSize.String())
			existing.Spec.Resources.Requests[corev1.ResourceStorage] = desiredSize
			_, err = u.clientset.CoreV1().PersistentVolumeClaims(constants.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("expand PVC %s: %w", desired.Name, err)
			}
		}
	}

	return nil
}

// upgradeService updates a Service to match the generated manifest.
// Skips update if no changes are detected.
func (u *Upgrader) upgradeService(ctx context.Context, cfg *types.DeployConfig, c *types.Component) error {
	logger := log.GetLogger()
	desired := component.Service(cfg, c)
	if desired == nil {
		return nil
	}

	existing, err := u.clientset.CoreV1().Services(constants.Namespace).Get(ctx, c.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			_, err = u.clientset.CoreV1().Services(constants.Namespace).Create(ctx, desired, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("create service %s: %w", c.Name, err)
			}
			logger.Info("service created", "name", c.Name)
			return nil
		}
		return fmt.Errorf("get service %s: %w", c.Name, err)
	}

	// Preserve ClusterIP and other immutable fields
	desired.Spec.ClusterIP = existing.Spec.ClusterIP
	desired.Spec.ClusterIPs = existing.Spec.ClusterIPs
	desired.Spec.IPFamilies = existing.Spec.IPFamilies
	desired.Spec.IPFamilyPolicy = existing.Spec.IPFamilyPolicy
	desired.Spec.HealthCheckNodePort = existing.Spec.HealthCheckNodePort

	if serviceEqual(existing, desired) {
		logger.V(1).Info("service unchanged, skipping update", "name", c.Name)
		return nil
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations

	_, err = u.clientset.CoreV1().Services(constants.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update service %s: %w", c.Name, err)
	}

	logger.Info("service updated", "name", c.Name)
	return nil
}

// serviceEqual checks if two services are equal (ignoring immutable fields).
func serviceEqual(a, b *corev1.Service) bool {
	return reflect.DeepEqual(a.Spec.Ports, b.Spec.Ports) &&
		reflect.DeepEqual(a.Spec.Selector, b.Spec.Selector) &&
		reflect.DeepEqual(a.Labels, b.Labels) &&
		reflect.DeepEqual(a.Annotations, b.Annotations)
}

// upgradeDeployment updates a Deployment to match the generated manifest.
// Skips update if no changes are detected.
func (u *Upgrader) upgradeDeployment(ctx context.Context, cfg *types.DeployConfig, c *types.Component) error {
	logger := log.GetLogger()
	desired := component.Deployment(cfg, c)
	existing, err := u.clientset.AppsV1().Deployments(constants.Namespace).Get(ctx, c.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get deployment %s: %w", c.Name, err)
	}

	if deploymentEqual(existing, desired) {
		logger.V(1).Info("deployment unchanged, skipping update", "name", c.Name)
		return nil
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations

	_, err = u.clientset.AppsV1().Deployments(constants.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update deployment %s: %w", c.Name, err)
	}

	logger.Info("deployment updated", "name", c.Name)
	return nil
}

// upgradeDaemonSet updates a DaemonSet to match the generated manifest.
// Skips update if no changes are detected.
func (u *Upgrader) upgradeDaemonSet(ctx context.Context, cfg *types.DeployConfig, c *types.Component) error {
	logger := log.GetLogger()
	desired := component.DaemonSet(cfg, c)
	existing, err := u.clientset.AppsV1().DaemonSets(constants.Namespace).Get(ctx, c.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get daemonset %s: %w", c.Name, err)
	}

	if daemonSetEqual(existing, desired) {
		logger.V(1).Info("daemonset unchanged, skipping update", "name", c.Name)
		return nil
	}

	existing.Spec = desired.Spec
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations

	_, err = u.clientset.AppsV1().DaemonSets(constants.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update daemonset %s: %w", c.Name, err)
	}

	logger.Info("daemonset updated", "name", c.Name)
	return nil
}

// upgradeStatefulSet updates a StatefulSet to match the generated manifest.
// Skips update if no changes are detected.
func (u *Upgrader) upgradeStatefulSet(ctx context.Context, cfg *types.DeployConfig, c *types.Component) error {
	logger := log.GetLogger()
	desired := component.StatefulSet(cfg, c)
	existing, err := u.clientset.AppsV1().StatefulSets(constants.Namespace).Get(ctx, c.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get statefulset %s: %w", c.Name, err)
	}

	if statefulSetEqual(existing, desired) {
		logger.V(1).Info("statefulset unchanged, skipping update", "name", c.Name)
		return nil
	}

	existing.Spec.Replicas = desired.Spec.Replicas
	existing.Spec.Template = desired.Spec.Template
	existing.Labels = desired.Labels
	existing.Annotations = desired.Annotations

	_, err = u.clientset.AppsV1().StatefulSets(constants.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("update statefulset %s: %w", c.Name, err)
	}

	logger.Info("statefulset updated", "name", c.Name)
	return nil
}

// deploymentEqual checks if two deployments are equal.
func deploymentEqual(a, b *appsv1.Deployment) bool {
	return reflect.DeepEqual(a.Spec, b.Spec) &&
		reflect.DeepEqual(a.Labels, b.Labels) &&
		reflect.DeepEqual(a.Annotations, b.Annotations)
}

// daemonSetEqual checks if two daemonsets are equal.
func daemonSetEqual(a, b *appsv1.DaemonSet) bool {
	return reflect.DeepEqual(a.Spec, b.Spec) &&
		reflect.DeepEqual(a.Labels, b.Labels) &&
		reflect.DeepEqual(a.Annotations, b.Annotations)
}

// statefulSetEqual checks if two statefulsets are equal.
func statefulSetEqual(a, b *appsv1.StatefulSet) bool {
	return reflect.DeepEqual(a.Spec.Replicas, b.Spec.Replicas) &&
		reflect.DeepEqual(a.Spec.Template, b.Spec.Template) &&
		reflect.DeepEqual(a.Labels, b.Labels) &&
		reflect.DeepEqual(a.Annotations, b.Annotations)
}

// buildSummary creates the upgrade summary.
func (u *Upgrader) buildSummary(ctx context.Context, cfg *types.DeployConfig) *types.UpgradeSummary {
	summary := &types.UpgradeSummary{
		Plane:     string(cfg.Plane),
		Mode:      cfg.EnvMode(),
		Namespace: constants.Namespace,
	}

	for _, c := range component.ComponentsForPlane(cfg) {
		summary.Components = append(summary.Components, types.ComponentStatus{
			Name:    c.Name,
			Healthy: true,
			Port:    c.Port,
		})
	}

	return summary
}
