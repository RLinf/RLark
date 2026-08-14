package kubernetes

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/rlinf/rlark/api/config"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/component"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/health"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Installer installs RLark components.
type Installer struct {
	summary *types.InstallSummary
}

// Install installs the components.
func (d *Installer) Install(cfg *types.DeployConfig, certBundle *cert.Bundle) error {
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

	if err := ensureNamespace(ctx, clientset); err != nil {
		return err
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

	// todo 可以并发部署
	for _, c := range component.ComponentsForPlane(cfg) {
		c.HealthCheckFn = health.K8sWorkloadHealthCheck(clientset, c)
		if c.Name == constants.ComponentKCP {
			c.PostDeployFn = extractKCPKubeconfigFn(ctx, clientset)
		}

		// 如果组件已存在且健康，跳过部署（CRD apply 在循环结束后统一执行）
		if c.HealthCheckFn != nil && c.HealthCheckFn(cfg) == nil {
			logger.Info("component already healthy, skipping", "name", c.Name)
			continue
		}

		if err := ensureRBAC(ctx, clientset, &c); err != nil {
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

		if err := health.WaitForHealthy(cfg, c); err != nil {
			return err
		}

		if c.PostDeployFn != nil {
			if err := c.PostDeployFn(cfg); err != nil {
				return err
			}
		}
	}

	// 始终 apply CRD，无论 KCP 是否新部署
	if cfg.Plane == types.PlaneControl {
		if err := createUIAuthSecretInKCP(ctx, clientset); err != nil {
			return err
		}
		if err := applyKCP(ctx, clientset); err != nil {
			return err
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
			healthy = c.HealthCheckFn(cfg) == nil
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
		if adminPW, userPW, err := readUIAuthFromKCP(ctx, clientset); err == nil {
			summary.AdminPassword = adminPW
			summary.UserPassword = userPW
		}
	}

	return summary
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

func ensureRBAC(ctx context.Context, clientset *kubernetes.Clientset, c *types.Component) error {
	logger := log.FromContext(ctx)
	sa, cr, crb := component.RBAC(c)
	if sa == nil {
		return nil
	}

	if _, err := clientset.CoreV1().ServiceAccounts(constants.Namespace).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("create serviceaccount %s: %w", sa.Name, err)
		}
		if _, err := clientset.CoreV1().ServiceAccounts(constants.Namespace).Update(ctx, sa, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("update serviceaccount %s: %w", sa.Name, err)
		}
	}

	if _, err := clientset.RbacV1().ClusterRoles().Create(ctx, cr, metav1.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) {
			if _, err := clientset.RbacV1().ClusterRoles().Update(ctx, cr, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("update clusterrole %s: %w", cr.Name, err)
			}
		} else {
			return fmt.Errorf("create clusterrole %s: %w", cr.Name, err)
		}
	}

	if _, err := clientset.RbacV1().ClusterRoleBindings().Create(ctx, crb, metav1.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) {
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

// extractKCPKubeconfigFn returns a PostDeployFn that extracts admin.kubeconfig
// from the KCP pod and creates a ConfigMap for other components to mount.
func extractKCPKubeconfigFn(ctx context.Context, clientset *kubernetes.Clientset) func(cfg *types.DeployConfig) error {
	return func(cfg *types.DeployConfig) error {
		return extractAndApplyKCP(ctx, clientset)
	}
}

func applyKCP(ctx context.Context, clientset *kubernetes.Clientset) error {
	return extractAndApplyKCP(ctx, clientset)
}

func extractAndApplyKCP(ctx context.Context, clientset *kubernetes.Clientset) error {
	logger := log.GetLogger()
	pods, err := clientset.CoreV1().Pods(constants.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + constants.ComponentKCP,
	})
	if err != nil {
		return fmt.Errorf("list kcp pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no kcp pods found")
	}

	podName := pods.Items[0].Name

	// Wait for admin.kubeconfig to be available, then extract via kubectl exec
	deadline := time.Now().Add(60 * time.Second)
	var kubeconfigData string
	for {
		cmd := exec.Command("kubectl", "-n", constants.Namespace, "exec", podName, "--", "cat", constants.KCPDataDir+"/admin.kubeconfig")
		var buf bytes.Buffer
		cmd.Stdout = &buf
		err := cmd.Run()
		if err == nil && buf.Len() > 0 {
			kubeconfigData = buf.String()
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for kcp admin.kubeconfig: %w", err)
		}
		time.Sleep(2 * time.Second)
	}

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "rlark-kcp-kubeconfig"},
		Data:       map[string]string{"admin.kubeconfig": kubeconfigData},
	}
	_, err = clientset.CoreV1().ConfigMaps(constants.Namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			_, err = clientset.CoreV1().ConfigMaps(constants.Namespace).Update(ctx, cm, metav1.UpdateOptions{})
			if err != nil {
				return fmt.Errorf("update kcp kubeconfig configmap: %w", err)
			}
		} else {
			return fmt.Errorf("create kcp kubeconfig configmap: %w", err)
		}
	}

	logger.Info("extracted admin.kubeconfig to ConfigMap")

	if err := installCRDs(podName); err != nil {
		return fmt.Errorf("install CRDs to kcp: %w", err)
	}

	return nil
}

// installCRDs applies embedded CRD manifests into the KCP pod via kubectl exec,
// since the local machine cannot reach the KCP API directly.
func installCRDs(podName string) error {
	logger := log.GetLogger()

	entries, err := config.CRDFiles.ReadDir("crd/bases")
	if err != nil {
		return fmt.Errorf("read embedded CRD files: %w", err)
	}
	if len(entries) == 0 {
		logger.Error(nil, "no CRD files embedded, skipping CRD install")
		return nil
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		crdName := entry.Name()
		content, err := config.CRDFiles.ReadFile("crd/bases/" + crdName)
		if err != nil {
			return fmt.Errorf("read embedded CRD %s: %w", crdName, err)
		}

		kc := constants.KCPDataDir + "/admin.kubeconfig"
		var lastErr error
		for attempt := 0; attempt < 5; attempt++ {
			createCmd := exec.Command("kubectl", "-n", constants.Namespace, "exec", "-i", podName, "--",
				"kubectl", "--kubeconfig", kc, "create", "--validate=false", "-f", "-")
			createCmd.Stdin = strings.NewReader(string(content))
			var createErr bytes.Buffer
			createCmd.Stderr = &createErr
			if err := createCmd.Run(); err == nil {
				lastErr = nil
				break
			}
			replaceCmd := exec.Command("kubectl", "-n", constants.Namespace, "exec", "-i", podName, "--",
				"kubectl", "--kubeconfig", kc, "replace", "--validate=false", "-f", "-")
			replaceCmd.Stdin = strings.NewReader(string(content))
			var replaceErr bytes.Buffer
			replaceCmd.Stderr = &replaceErr
			if err := replaceCmd.Run(); err == nil {
				lastErr = nil
				break
			}
			if strings.Contains(replaceErr.String(), "field is immutable") {
				deleteCmd := exec.Command("kubectl", "-n", constants.Namespace, "exec", podName, "--",
					"kubectl", "--kubeconfig", kc, "delete", "--ignore-not-found", "-f", "-")
				deleteCmd.Stdin = strings.NewReader(string(content))
				var deleteErr bytes.Buffer
				deleteCmd.Stderr = &deleteErr
				if err := deleteCmd.Run(); err == nil {
					recreateCmd := exec.Command("kubectl", "-n", constants.Namespace, "exec", "-i", podName, "--",
						"kubectl", "--kubeconfig", kc, "create", "--validate=false", "-f", "-")
					recreateCmd.Stdin = strings.NewReader(string(content))
					var recreateErr bytes.Buffer
					recreateCmd.Stderr = &recreateErr
					if err := recreateCmd.Run(); err == nil {
						lastErr = nil
						break
					}
					lastErr = fmt.Errorf("apply %s: recreate after delete: %s", crdName, recreateErr.String())
				} else {
					lastErr = fmt.Errorf("apply %s: delete before recreate: %s", crdName, deleteErr.String())
				}
			} else {
				lastErr = fmt.Errorf("apply %s: create: %s | replace: %s", crdName, createErr.String(), replaceErr.String())
			}
			if attempt < 2 {
				time.Sleep(5 * time.Second)
			}
		}
		if lastErr != nil {
			return lastErr
		}
		logger.Info("applied CRD", "file", crdName)
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

// createUIAuthSecretInKCP creates the rlark-ui-auth secret in KCP (not local K8s)
// by running kubectl inside the KCP pod.
func createUIAuthSecretInKCP(ctx context.Context, clientset *kubernetes.Clientset) error {
	logger := log.FromContext(ctx)

	pods, err := clientset.CoreV1().Pods(constants.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + constants.ComponentKCP,
	})
	if err != nil {
		return fmt.Errorf("list kcp pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no kcp pods found")
	}
	podName := pods.Items[0].Name
	kc := constants.KCPDataDir + "/admin.kubeconfig"

	// Check if secret already exists in KCP
	checkCmd := exec.Command("kubectl", "-n", constants.Namespace, "exec", podName, "--",
		"kubectl", "--kubeconfig", kc, "get", "secret", common.UIAuthSecretName, "-n", "default")
	if err := checkCmd.Run(); err == nil {
		logger.Info("ui auth secret already exists in KCP, skipping", "name", common.UIAuthSecretName)
		return nil
	}

	adminPassword, err := generatePassword(16)
	if err != nil {
		return fmt.Errorf("generate admin password: %w", err)
	}
	userPassword, err := generatePassword(16)
	if err != nil {
		return fmt.Errorf("generate user password: %w", err)
	}

	manifest := fmt.Sprintf(`
apiVersion: v1
kind: Secret
metadata:
  name: %s
  namespace: default
type: Opaque
stringData:
  admin-password: %s
  user-password: %s
`, common.UIAuthSecretName, adminPassword, userPassword)

	applyCmd := exec.Command("kubectl", "-n", constants.Namespace, "exec", "-i", podName, "--",
		"kubectl", "--kubeconfig", kc, "apply", "-f", "-")
	applyCmd.Stdin = strings.NewReader(manifest)
	var errBuf bytes.Buffer
	applyCmd.Stderr = &errBuf
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("apply ui auth secret in kcp: %w: %s", err, errBuf.String())
	}

	logger.Info("ui auth secret created in KCP", "name", common.UIAuthSecretName)
	return nil
}

// readUIAuthFromKCP reads the ui auth secret from KCP via kubectl exec.
func readUIAuthFromKCP(ctx context.Context, clientset *kubernetes.Clientset) (adminPW, userPW string, err error) {
	pods, err := clientset.CoreV1().Pods(constants.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + constants.ComponentKCP,
	})
	if err != nil {
		return "", "", fmt.Errorf("list kcp pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return "", "", fmt.Errorf("no kcp pods found")
	}
	podName := pods.Items[0].Name
	kc := constants.KCPDataDir + "/admin.kubeconfig"

	cmd := exec.Command("kubectl", "-n", constants.Namespace, "exec", podName, "--",
		"kubectl", "--kubeconfig", kc, "get", "secret", common.UIAuthSecretName, "-n", "default",
		"-o", "json")
	var jsonOut bytes.Buffer
	cmd.Stdout = &jsonOut
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("read ui auth secret from kcp: %w", err)
	}

	return parseAuthSecretJSON(jsonOut.Bytes())
}

func parseAuthSecretJSON(data []byte) (adminPW, userPW string, err error) {
	var s struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return "", "", err
	}
	adminRaw, ok := s.Data["admin-password"]
	if !ok {
		return "", "", fmt.Errorf("admin-password not found in secret")
	}
	userRaw, ok := s.Data["user-password"]
	if !ok {
		return "", "", fmt.Errorf("user-password not found in secret")
	}
	adminDec, err := base64.StdEncoding.DecodeString(adminRaw)
	if err != nil {
		return "", "", fmt.Errorf("decode admin-password: %w", err)
	}
	userDec, err := base64.StdEncoding.DecodeString(userRaw)
	if err != nil {
		return "", "", fmt.Errorf("decode user-password: %w", err)
	}
	return string(adminDec), string(userDec), nil
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
