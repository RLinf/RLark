package component

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rlinf/rlark/api/config"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/constants"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// PostDeployKCP initializes KCP after deployment.
// This is only called when KCP is used as the management API (not UsesKubernetesManagementAPI).
// It retrieves runtime dependencies from context.
func PostDeployKCP(ctx context.Context, cfg *types.DeployConfig) error {
	clientset, ok := runtime.GetClientset(ctx)
	if !ok {
		return fmt.Errorf("clientset not found in context")
	}

	kubeconfig, ok := runtime.GetKubeconfig(ctx)
	if !ok {
		return fmt.Errorf("kubeconfig not found in context")
	}

	// Extract KCP admin kubeconfig and save to ConfigMap
	if err := extractAndSaveKCPKubeconfig(ctx, clientset, kubeconfig); err != nil {
		return err
	}

	// Install CRDs via exec
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

	installer := &ExecCRDInstaller{
		PodName:    podName,
		Kubeconfig: kubeconfig,
	}
	if err := installer.Install(ctx); err != nil {
		return fmt.Errorf("install CRDs to kcp: %w", err)
	}

	// Create UI auth secret
	return createUIAuthSecretInKCP(ctx, clientset, kubeconfig)
}

// extractAndSaveKCPKubeconfig extracts the admin kubeconfig from KCP pod and saves it to a ConfigMap.
func extractAndSaveKCPKubeconfig(ctx context.Context, clientset kubernetes.Interface, kubeconfig string) error {
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

	// Wait for admin.kubeconfig to be available
	deadline := time.Now().Add(60 * time.Second)
	var kubeconfigData string
	for {
		cmd := kubectlCommand(kubeconfig, "-n", constants.Namespace, "exec", podName, "--", "cat", constants.KCPDataDir+"/admin.kubeconfig")
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
	return nil
}

// readCRDFiles reads all embedded CRD files.
func readCRDFiles() (map[string][]byte, error) {
	entries, err := config.CRDFiles.ReadDir("crd/bases")
	if err != nil {
		return nil, fmt.Errorf("read embedded CRD files: %w", err)
	}

	files := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, err := config.CRDFiles.ReadFile("crd/bases/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read embedded CRD %s: %w", entry.Name(), err)
		}
		files[entry.Name()] = content
	}
	return files, nil
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

// createUIAuthSecretInKCP creates the rlark-ui-auth secret in KCP
func createUIAuthSecretInKCP(ctx context.Context, clientset kubernetes.Interface, kubeconfig string) error {
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
	checkCmd := kubectlCommand(kubeconfig, "-n", constants.Namespace, "exec", podName, "--",
		"kubectl", "--kubeconfig", kc, "get", "secret", common.UIAuthSecretName, "-n", "default")
	if err := checkCmd.Run(); err == nil {
		adminPassword, userPassword, signingKey, err := readUIAuthSecretFromKCP(ctx, clientset, kubeconfig)
		if err != nil {
			return fmt.Errorf("existing ui auth secret in kcp is invalid: %w", err)
		}
		if len(signingKey) < 32 {
			signingKey, err = generateSecret(32)
			if err != nil {
				return fmt.Errorf("generate JWT signing key: %w", err)
			}
			manifest, err := uiAuthSecretManifest(adminPassword, userPassword, signingKey)
			if err != nil {
				return fmt.Errorf("marshal ui auth secret: %w", err)
			}
			applyCmd := kubectlCommand(kubeconfig, "-n", constants.Namespace, "exec", "-i", podName, "--",
				"kubectl", "--kubeconfig", kc, "apply", "--validate=false", "-f", "-")
			applyCmd.Stdin = bytes.NewReader(manifest)
			if err := applyCmd.Run(); err != nil {
				return fmt.Errorf("update ui auth secret in kcp: %w", err)
			}
		}
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
	signingKey, err := generateSecret(32)
	if err != nil {
		return fmt.Errorf("generate JWT signing key: %w", err)
	}

	manifest, err := uiAuthSecretManifest(adminPassword, userPassword, signingKey)
	if err != nil {
		return fmt.Errorf("marshal ui auth secret: %w", err)
	}

	applyCmd := kubectlCommand(kubeconfig, "-n", constants.Namespace, "exec", "-i", podName, "--",
		"kubectl", "--kubeconfig", kc, "apply", "--validate=false", "-f", "-")
	applyCmd.Stdin = bytes.NewReader(manifest)
	var errBuf bytes.Buffer
	applyCmd.Stderr = &errBuf
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("apply ui auth secret in kcp: %w: %s", err, errBuf.String())
	}

	logger.Info("ui auth secret created in KCP", "name", common.UIAuthSecretName)
	return nil
}

func uiAuthSecretManifest(adminPassword, userPassword string, signingKey []byte) ([]byte, error) {
	return yaml.Marshal(&corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.UIAuthSecretName,
			Namespace: "default",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			common.UIAuthAdminPasswordKey: []byte(adminPassword),
			common.UIAuthUserPasswordKey:  []byte(userPassword),
			common.UIAuthJWTSigningKey:    signingKey,
		},
	})
}

// ReadUIAuthFromKCP reads the ui auth secret from KCP via kubectl exec.
func ReadUIAuthFromKCP(ctx context.Context, clientset kubernetes.Interface, kubeconfig string) (adminPW, userPW string, err error) {
	adminPW, userPW, _, err = readUIAuthSecretFromKCP(ctx, clientset, kubeconfig)
	return adminPW, userPW, err
}

func readUIAuthSecretFromKCP(ctx context.Context, clientset kubernetes.Interface, kubeconfig string) (adminPW, userPW string, signingKey []byte, err error) {
	pods, err := clientset.CoreV1().Pods(constants.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + constants.ComponentKCP,
	})
	if err != nil {
		return "", "", nil, fmt.Errorf("list kcp pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return "", "", nil, fmt.Errorf("no kcp pods found")
	}
	podName := pods.Items[0].Name
	kc := constants.KCPDataDir + "/admin.kubeconfig"

	cmd := kubectlCommand(kubeconfig, "-n", constants.Namespace, "exec", podName, "--",
		"kubectl", "--kubeconfig", kc, "get", "secret", common.UIAuthSecretName, "-n", "default",
		"-o", "json")
	var jsonOut bytes.Buffer
	cmd.Stdout = &jsonOut
	if err := cmd.Run(); err != nil {
		return "", "", nil, fmt.Errorf("read ui auth secret from kcp: %w", err)
	}

	return parseAuthSecretJSONWithSigningKey(jsonOut.Bytes())
}

func parseAuthSecretJSONWithSigningKey(data []byte) (adminPW, userPW string, signingKey []byte, err error) {
	var s struct {
		Data map[string]string `json:"data"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return "", "", nil, err
	}
	adminRaw, ok := s.Data["admin-password"]
	if !ok {
		return "", "", nil, fmt.Errorf("admin-password not found in secret")
	}
	userRaw, ok := s.Data["user-password"]
	if !ok {
		return "", "", nil, fmt.Errorf("user-password not found in secret")
	}
	adminDec, err := base64.StdEncoding.DecodeString(adminRaw)
	if err != nil {
		return "", "", nil, fmt.Errorf("decode admin-password: %w", err)
	}
	userDec, err := base64.StdEncoding.DecodeString(userRaw)
	if err != nil {
		return "", "", nil, fmt.Errorf("decode user-password: %w", err)
	}
	if signingRaw, ok := s.Data[common.UIAuthJWTSigningKey]; ok {
		signingKey, err = base64.StdEncoding.DecodeString(signingRaw)
		if err != nil {
			return "", "", nil, fmt.Errorf("decode %s: %w", common.UIAuthJWTSigningKey, err)
		}
	}
	return string(adminDec), string(userDec), signingKey, nil
}
