package kubernetes

import (
	"context"
	"crypto/rand"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/health"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Installer installs RLark components.
type Installer struct {
	summary    *types.InstallSummary
	kubeconfig string
}

// Install installs the components.
func (d *Installer) Install(cfg *types.DeployConfig, certBundle *cert.Bundle) error {
	logger := log.GetLogger()
	kubeconfig := cfg.Kubernetes.Kubeconfig
	if kubeconfig == "" {
		kubeconfig = clientcmd.NewDefaultClientConfigLoadingRules().GetDefaultFilename()
	}
	d.kubeconfig = kubeconfig
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("build kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create clientset: %w", err)
	}
	// Inject runtime dependencies into context
	ctx := runtime.WithClientset(context.Background(), clientset)
	ctx = runtime.WithKubeconfig(ctx, kubeconfig)

	if err := ensureNamespace(ctx, clientset); err != nil {
		return err
	}
	if cfg.UsesKubernetesManagementAPI() {
		apiExtensions, err := apiextensionsclient.NewForConfig(restConfig)
		if err != nil {
			return fmt.Errorf("create apiextensions clientset: %w", err)
		}
		installer := &component.DirectCRDInstaller{Client: apiExtensions}
		if err := installer.Install(ctx); err != nil {
			return err
		}
		if err := ensureUIAuthSecretInKubernetes(ctx, clientset, constants.Namespace); err != nil {
			return err
		}
	}

	if certBundle != nil {
		if err := createCertSecret(ctx, clientset, cfg, certBundle); err != nil {
			return err
		}
	}

	if cfg.DB != nil {
		if err := createDBConfigMap(ctx, clientset, cfg); err != nil {
			return err
		}
		if err := createPostgresInitConfigMap(ctx, clientset); err != nil {
			return err
		}
	}

	// Deploy components in dependency order
	for _, c := range component.ComponentsForPlane(cfg) {
		if err := ensureRBAC(ctx, clientset, cfg, &c); err != nil {
			return err
		}

		if svc := component.Service(cfg, &c); svc != nil {
			if err := createService(ctx, clientset, svc); err != nil {
				return err
			}
			logger.Info("service created", "name", svc.Name)
		}

		if err := createWorkload(ctx, clientset, &c, cfg); err != nil {
			return err
		}

		logger.Info("component deployed", "name", c.Name, "port", c.Port)

		// Health check using context-injected clientset
		if err := health.WaitForHealthy(ctx, cfg, c); err != nil {
			return err
		}

		// Post-deploy hook
		if c.PostDeployFn != nil {
			if err := c.PostDeployFn(ctx, cfg); err != nil {
				return err
			}
		}
	}

	logger.Info("plane deployed", "plane", cfg.Plane, "namespace", constants.Namespace)

	d.summary = d.buildSummary(ctx, clientset, cfg)
	return nil
}

// Summary is an exported method.
func (d *Installer) Summary() *types.InstallSummary {
	return d.summary
}

func (d *Installer) buildSummary(ctx context.Context, clientset *kubernetes.Clientset, cfg *types.DeployConfig) *types.InstallSummary {
	summary := &types.InstallSummary{
		Plane:     string(cfg.Plane),
		Mode:      cfg.EnvMode(),
		Namespace: constants.Namespace,
	}

	for _, c := range component.ComponentsForPlane(cfg) {
		addr := ""
		if c.NeedsService && c.Port > 0 {
			addr = fmt.Sprintf("http://%s.%s.svc:%d", c.Name, constants.Namespace, c.Port)
			if c.Name == constants.ComponentServer || c.Name == constants.ComponentKCP {
				addr = fmt.Sprintf("https://%s.%s.svc:%d", c.Name, constants.Namespace, c.Port)
			}
		}
		healthy := false
		if c.HealthCheckFn != nil {
			healthy = c.HealthCheckFn(ctx, cfg) == nil
		}
		summary.Components = append(summary.Components, types.ComponentStatus{
			Name:    c.Name,
			Healthy: healthy,
			Port:    c.Port,
			Address: addr,
		})
	}

	if cfg.Plane == types.PlaneData {
		summary.ControlPlaneAddress = cfg.ControlPlaneAddress
	}

	if cfg.Plane == types.PlaneControl {
		var adminPW, userPW string
		var err error
		if cfg.UsesKubernetesManagementAPI() {
			adminPW, userPW, err = readUIAuthFromKubernetes(ctx, clientset, constants.Namespace)
		} else {
			adminPW, userPW, err = component.ReadUIAuthFromKCP(ctx, clientset, d.kubeconfig)
		}
		if err == nil {
			summary.AdminPassword = adminPW
			summary.UserPassword = userPW
		}
	}

	return summary
}

func ensureUIAuthSecretInKubernetes(ctx context.Context, clientset kubernetes.Interface, namespace string) error {
	if secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, common.UIAuthSecretName, metav1.GetOptions{}); err == nil {
		if len(secret.Data[common.UIAuthAdminPasswordKey]) > 0 &&
			len(secret.Data[common.UIAuthUserPasswordKey]) > 0 &&
			len(secret.Data[common.UIAuthJWTSigningKey]) >= 32 {
			return nil
		}
		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}
		if len(secret.Data[common.UIAuthAdminPasswordKey]) == 0 {
			password, err := generatePassword(16)
			if err != nil {
				return fmt.Errorf("generate admin password: %w", err)
			}
			secret.Data[common.UIAuthAdminPasswordKey] = []byte(password)
		}
		if len(secret.Data[common.UIAuthUserPasswordKey]) == 0 {
			password, err := generatePassword(16)
			if err != nil {
				return fmt.Errorf("generate user password: %w", err)
			}
			secret.Data[common.UIAuthUserPasswordKey] = []byte(password)
		}
		if len(secret.Data[common.UIAuthJWTSigningKey]) < 32 {
			signingKey, err := generateSecret(32)
			if err != nil {
				return fmt.Errorf("generate JWT signing key: %w", err)
			}
			secret.Data[common.UIAuthJWTSigningKey] = signingKey
		}
		if _, err := clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update ui auth secret: %w", err)
		}
		return nil
	} else if !errors.IsNotFound(err) {
		return fmt.Errorf("get ui auth secret: %w", err)
	}
	adminPassword, err := generatePassword(16)
	if err != nil {
		return fmt.Errorf("generate admin password: %w", err)
	}
	userPassword, err := generatePassword(16)
	if err != nil {
		return fmt.Errorf("generate user password: %w", err)
	}
	signingKey, err := generateSecret(32)
	if err != nil {
		return fmt.Errorf("generate JWT signing key: %w", err)
	}
	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: common.UIAuthSecretName, Namespace: namespace},
		Type:       corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			common.UIAuthAdminPasswordKey: []byte(adminPassword),
			common.UIAuthUserPasswordKey:  []byte(userPassword),
			common.UIAuthJWTSigningKey:    signingKey,
		},
	}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create ui auth secret: %w", err)
	}
	return nil
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*()"

func generatePassword(length int) (string, error) {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	out := make([]byte, length)
	for i := range b {
		out[i] = letters[int(b[i])%len(letters)]
	}

	return string(out), nil
}

func generateSecret(length int) ([]byte, error) {
	secret := make([]byte, length)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	return secret, nil
}

func readUIAuthFromKubernetes(ctx context.Context, clientset kubernetes.Interface, namespace string) (string, string, error) {
	secret, err := clientset.CoreV1().Secrets(namespace).Get(ctx, common.UIAuthSecretName, metav1.GetOptions{})
	if err != nil {
		return "", "", fmt.Errorf("get ui auth secret: %w", err)
	}
	adminPassword, ok := secret.Data[common.UIAuthAdminPasswordKey]
	if !ok {
		return "", "", fmt.Errorf("admin-password not found in secret")
	}
	userPassword, ok := secret.Data[common.UIAuthUserPasswordKey]
	if !ok {
		return "", "", fmt.Errorf("user-password not found in secret")
	}
	return string(adminPassword), string(userPassword), nil
}

func ensureNamespace(ctx context.Context, clientset *kubernetes.Clientset) error {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, constants.Namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("get namespace %s: %w", constants.Namespace, err)
	}
	_, err = clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: constants.Namespace},
	}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %s: %w", constants.Namespace, err)
	}
	return nil
}

func ensureRBAC(ctx context.Context, clientset *kubernetes.Clientset, cfg *types.DeployConfig, c *types.Component) error {
	logger := log.FromContext(ctx)
	sa, cr, crb := component.RBAC(cfg, c)
	if sa == nil {
		return nil
	}

	if _, err := clientset.CoreV1().ServiceAccounts(constants.Namespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create serviceaccount %s: %w", sa.Name, err)
		}
		existing, getErr := clientset.CoreV1().ServiceAccounts(constants.Namespace).Get(ctx, sa.Name, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("get serviceaccount %s: %w", sa.Name, getErr)
		}
		sa.ResourceVersion = existing.ResourceVersion
		if _, err := clientset.CoreV1().ServiceAccounts(constants.Namespace).Update(ctx, sa, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update serviceaccount %s: %w", sa.Name, err)
		}
	}

	if _, err := clientset.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := clientset.RbacV1().ClusterRoles().Get(ctx, cr.Name, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("get clusterrole %s: %w", cr.Name, getErr)
			}
			cr.ResourceVersion = existing.ResourceVersion
			if _, err := clientset.RbacV1().ClusterRoles().Update(ctx, cr, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("update clusterrole %s: %w", cr.Name, err)
			}
		} else {
			return fmt.Errorf("create clusterrole %s: %w", cr.Name, err)
		}
	}

	if _, err := clientset.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := clientset.RbacV1().ClusterRoleBindings().Get(ctx, crb.Name, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("get clusterrolebinding %s: %w", crb.Name, getErr)
			}
			crb.ResourceVersion = existing.ResourceVersion
			if _, err := clientset.RbacV1().ClusterRoleBindings().Update(ctx, crb, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("update clusterrolebinding %s: %w", crb.Name, err)
			}
		} else {
			return fmt.Errorf("create clusterrolebinding %s: %w", crb.Name, err)
		}
	}

	logger.Info("ensured RBAC", "component", c.Name, "serviceAccount", sa.Name)
	return nil
}

func createDBConfigMap(ctx context.Context, clientset *kubernetes.Clientset, cfg *types.DeployConfig) error {
	yamlData, err := component.DBConfigYAML(cfg.DB)
	if err != nil {
		return err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "rlark-db-config"},
		Data:       map[string]string{"db.yaml": string(yamlData)},
	}
	return createOrUpdateConfigMap(ctx, clientset, cm)
}

func createPostgresInitConfigMap(ctx context.Context, clientset *kubernetes.Clientset) error {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "rlark-postgres-init"},
		Data:       map[string]string{"init-db.sql": constants.InitDBSQL},
	}
	return createOrUpdateConfigMap(ctx, clientset, cm)
}

func createCertSecret(ctx context.Context, clientset *kubernetes.Clientset, cfg *types.DeployConfig, bundle *cert.Bundle) error {
	name := "rlark-tls"
	if cfg.Plane == types.PlaneData {
		name = "rlark-agent-cert"
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data: map[string][]byte{
			"ca.crt":  bundle.CACertPEM,
			"tls.crt": bundle.CertPEM,
			"tls.key": bundle.KeyPEM,
		},
	}
	_, err := clientset.CoreV1().Secrets(constants.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := clientset.CoreV1().Secrets(constants.Namespace).Get(ctx, name, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("get cert secret %s: %w", name, getErr)
			}
			secret.ResourceVersion = existing.ResourceVersion
			_, err = clientset.CoreV1().Secrets(constants.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("update cert secret %s: %w", name, err)
			}
			return nil
		}
		return fmt.Errorf("create cert secret %s: %w", name, err)
	}
	return nil
}

func createDeployment(ctx context.Context, clientset *kubernetes.Clientset, dep *appsv1.Deployment) error {
	_, err := clientset.AppsV1().Deployments(constants.Namespace).Create(ctx, dep, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := clientset.AppsV1().Deployments(constants.Namespace).Get(ctx, dep.Name, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("get deployment %s: %w", dep.Name, getErr)
			}
			dep.ResourceVersion = existing.ResourceVersion
			_, err = clientset.AppsV1().Deployments(constants.Namespace).Update(ctx, dep, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("update deployment %s: %w", dep.Name, err)
			}
			return nil
		}
		return fmt.Errorf("create deployment %s: %w", dep.Name, err)
	}
	return nil
}

func createWorkload(ctx context.Context, clientset *kubernetes.Clientset, c *types.Component, cfg *types.DeployConfig) error {
	switch c.WorkloadKind {
	case "DaemonSet":
		return createDaemonSet(ctx, clientset, component.DaemonSet(cfg, c))
	case "StatefulSet":
		return createStatefulSet(ctx, clientset, component.StatefulSet(cfg, c))
	default:
		if c.VolumeClaimFn != nil {
			if err := createPVCs(ctx, clientset, c.VolumeClaimFn(cfg)); err != nil {
				return err
			}
		}
		return createDeployment(ctx, clientset, component.Deployment(cfg, c))
	}
}

func createStatefulSet(ctx context.Context, clientset *kubernetes.Clientset, sts *appsv1.StatefulSet) error {
	_, err := clientset.AppsV1().StatefulSets(constants.Namespace).Create(ctx, sts, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := clientset.AppsV1().StatefulSets(constants.Namespace).Get(ctx, sts.Name, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("get statefulset %s: %w", sts.Name, getErr)
			}
			sts.ResourceVersion = existing.ResourceVersion
			_, err = clientset.AppsV1().StatefulSets(constants.Namespace).Update(ctx, sts, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("update statefulset %s: %w", sts.Name, err)
			}
			return nil
		}
		return fmt.Errorf("create statefulset %s: %w", sts.Name, err)
	}
	return nil
}

func createDaemonSet(ctx context.Context, clientset *kubernetes.Clientset, ds *appsv1.DaemonSet) error {
	_, err := clientset.AppsV1().DaemonSets(constants.Namespace).Create(ctx, ds, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := clientset.AppsV1().DaemonSets(constants.Namespace).Get(ctx, ds.Name, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("get daemonset %s: %w", ds.Name, getErr)
			}
			ds.ResourceVersion = existing.ResourceVersion
			_, err = clientset.AppsV1().DaemonSets(constants.Namespace).Update(ctx, ds, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("update daemonset %s: %w", ds.Name, err)
			}
			return nil
		}
		return fmt.Errorf("create daemonset %s: %w", ds.Name, err)
	}
	return nil
}

func createService(ctx context.Context, clientset *kubernetes.Clientset, svc *corev1.Service) error {
	_, err := clientset.CoreV1().Services(constants.Namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := clientset.CoreV1().Services(constants.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("get service %s: %w", svc.Name, getErr)
			}
			svc.ResourceVersion = existing.ResourceVersion
			svc.Spec.ClusterIP = existing.Spec.ClusterIP
			svc.Spec.ClusterIPs = existing.Spec.ClusterIPs
			svc.Spec.IPFamilies = existing.Spec.IPFamilies
			svc.Spec.IPFamilyPolicy = existing.Spec.IPFamilyPolicy
			svc.Spec.HealthCheckNodePort = existing.Spec.HealthCheckNodePort
			_, err = clientset.CoreV1().Services(constants.Namespace).Update(ctx, svc, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("update service %s: %w", svc.Name, err)
			}
			return nil
		}
		return fmt.Errorf("create service %s: %w", svc.Name, err)
	}
	return nil
}

func createOrUpdateConfigMap(ctx context.Context, clientset *kubernetes.Clientset, cm *corev1.ConfigMap) error {
	_, err := clientset.CoreV1().ConfigMaps(constants.Namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			existing, getErr := clientset.CoreV1().ConfigMaps(constants.Namespace).Get(ctx, cm.Name, metav1.GetOptions{})
			if getErr != nil {
				return fmt.Errorf("get configmap %s: %w", cm.Name, getErr)
			}
			cm.ResourceVersion = existing.ResourceVersion
			_, err = clientset.CoreV1().ConfigMaps(constants.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("update configmap %s: %w", cm.Name, err)
			}
			return nil
		}
		return fmt.Errorf("create configmap %s: %w", cm.Name, err)
	}
	return nil
}

func createPVCs(ctx context.Context, clientset *kubernetes.Clientset, claims []corev1.PersistentVolumeClaim) error {
	for _, claim := range claims {
		_, err := clientset.CoreV1().PersistentVolumeClaims(constants.Namespace).Create(ctx, &claim, metav1.CreateOptions{})
		if err != nil {
			if errors.IsAlreadyExists(err) {
				continue
			}
			return fmt.Errorf("create pvc %s: %w", claim.Name, err)
		}
	}
	return nil
}
