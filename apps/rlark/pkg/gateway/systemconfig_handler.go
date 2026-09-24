package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/logquery"
	rlarkadmtypes "github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
)

// systemConfig is the unified request/response shape for the system config
// API. Each category is a sub-struct; missing categories in a PUT body are
// left unchanged.
type systemConfig struct {
	SSH        *sshConfig                  `json:"ssh,omitempty"`
	Log        *logquery.Config            `json:"log,omitempty"`
	Deployment *rlarkadmtypes.DeployConfig `json:"deployment,omitempty"`
}

type sshConfig struct {
	JumpHost string `json:"jumpHost,omitempty"`
	JumpPort string `json:"jumpPort,omitempty"`
}

func validateDeploymentConfig(cfg *rlarkadmtypes.DeployConfig) error {
	if cfg == nil {
		return nil
	}
	cfg.ControlPlaneAddress = strings.TrimSpace(cfg.ControlPlaneAddress)
	cfg.SSHAddress = strings.TrimSpace(cfg.SSHAddress)
	controlPlaneAddress := cfg.ControlPlaneAddress
	if cfg.DB != nil || cfg.Docker != nil || cfg.Raw != nil || cfg.Cert != nil {
		return fmt.Errorf("deployment defaults only support data-plane agent fields")
	}
	if cfg.Kubernetes == nil {
		return fmt.Errorf("deployment defaults must use the kubernetes environment")
	}
	if cfg.Kubernetes.ManagementAPI != "" ||
		cfg.Kubernetes.GatewayImage != "" ||
		cfg.Kubernetes.ControllerManagerImage != "" ||
		cfg.Kubernetes.ServerImage != "" ||
		cfg.Kubernetes.KCPImage != "" ||
		cfg.Kubernetes.EtcdImage != "" ||
		cfg.Kubernetes.PostgresqlImage != "" ||
		cfg.Kubernetes.UIImage != "" ||
		cfg.Kubernetes.Replicas != 0 ||
		cfg.Kubernetes.Storage != nil ||
		cfg.Kubernetes.KCP != nil ||
		cfg.Kubernetes.Etcd != nil ||
		cfg.Kubernetes.Postgresql != nil {
		return fmt.Errorf("deployment defaults only support kubernetes agent fields")
	}
	cfg.APIVersion = "rlark.io/v1alpha1"
	cfg.Kind = "DeployConfig"
	cfg.Plane = rlarkadmtypes.PlaneData
	if cfg.ControlPlaneAddress == "" {
		cfg.ControlPlaneAddress = "https://system-config-default.invalid"
	}
	cfg.Cert = &rlarkadmtypes.CertConfig{CACert: "configured", AgentCert: "configured", AgentKey: "configured"}
	if err := cfg.Validate(); err != nil {
		cfg.ControlPlaneAddress = controlPlaneAddress
		cfg.Cert = nil
		return err
	}
	cfg.ControlPlaneAddress = controlPlaneAddress
	cfg.Cert = nil
	return nil
}

func validateSSHConfig(cfg *sshConfig) error {
	if cfg == nil {
		return nil
	}
	host := strings.TrimSpace(cfg.JumpHost)
	port := strings.TrimSpace(cfg.JumpPort)
	if host == "" {
		if port != "" {
			return fmt.Errorf("jumpHost is required when jumpPort is set")
		}
		return nil
	}
	if strings.ContainsAny(host, " \t\r\n/@") {
		return fmt.Errorf("jumpHost must be a hostname or IP address without spaces, scheme, user, or path")
	}
	if strings.Contains(host, ":") {
		if parsed, err := url.Parse("ssh://" + host); err != nil || parsed.Hostname() != host {
			return fmt.Errorf("jumpHost must not include a port; use jumpPort instead")
		}
	}
	if port != "" {
		value, err := strconv.Atoi(port)
		if err != nil || value < 1 || value > 65535 {
			return fmt.Errorf("jumpPort must be an integer between 1 and 65535")
		}
	}
	cfg.JumpHost = host
	cfg.JumpPort = port
	return nil
}

func preserveMaskedLogFields(incoming, existing *logquery.Config) {
	if incoming == nil || existing == nil || incoming.Backend != existing.Backend {
		return
	}
	for _, key := range []string{"accessKeyId", "accessKeySecret"} {
		if incoming.Config[key] == "****" {
			if value, ok := existing.Config[key]; ok {
				incoming.Config[key] = value
			}
		}
	}
}

func (g *Gateway) getSystemConfigSecret(ctx context.Context) (*corev1.Secret, error) {
	secret, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(ctx, common.SystemConfigSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func (g *Gateway) ensureSystemConfigSecret(ctx context.Context) (*corev1.Secret, error) {
	secret, err := g.getSystemConfigSecret(ctx)
	if err == nil {
		return secret, nil
	}
	if !errors.IsNotFound(err) {
		return nil, err
	}
	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.SystemConfigSecretName,
			Namespace: g.managementNamespace(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}
	created, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create system config secret: %w", err)
	}
	return created, nil
}

// readSystemConfig decodes the Secret into a systemConfig. It tolerates the
// legacy flat sshJumpHost/sshJumpPort keys.
func readSystemConfig(secret *corev1.Secret) *systemConfig {
	cfg := &systemConfig{}
	if secret == nil || secret.Data == nil {
		return cfg
	}

	// New-style JSON categories.
	if raw, ok := secret.Data[common.SystemConfigKeySSH]; ok {
		var s sshConfig
		if err := json.Unmarshal(raw, &s); err == nil {
			cfg.SSH = &s
		}
	}
	if raw, ok := secret.Data[common.SystemConfigKeyLog]; ok {
		var l logquery.Config
		if err := json.Unmarshal(raw, &l); err == nil {
			cfg.Log = &l
		}
	}
	if raw, ok := secret.Data[common.SystemConfigKeyDeployment]; ok {
		var deployment rlarkadmtypes.DeployConfig
		if err := json.Unmarshal(raw, &deployment); err == nil {
			cfg.Deployment = &deployment
		}
	}

	// Legacy flat keys take lower precedence and only fill gaps.
	if cfg.SSH == nil {
		host := string(secret.Data[common.SystemConfigKeySSHJumpHost])
		port := string(secret.Data[common.SystemConfigKeySSHJumpPort])
		if host != "" || port != "" {
			cfg.SSH = &sshConfig{JumpHost: host, JumpPort: port}
		}
	}

	return cfg
}

// writeSystemConfigToSecret encodes the non-nil categories of cfg into the
// Secret's data. It does not touch categories that are nil in cfg.
func writeSystemConfigToSecret(secret *corev1.Secret, cfg *systemConfig) error {
	if secret.Data == nil {
		secret.Data = map[string][]byte{}
	}

	if cfg.SSH != nil {
		raw, err := json.Marshal(cfg.SSH)
		if err != nil {
			return fmt.Errorf("marshal ssh config: %w", err)
		}
		secret.Data[common.SystemConfigKeySSH] = raw
		// Clean up legacy keys so we don't read them again.
		delete(secret.Data, common.SystemConfigKeySSHJumpHost)
		delete(secret.Data, common.SystemConfigKeySSHJumpPort)
	}
	if cfg.Log != nil {
		raw, err := json.Marshal(cfg.Log)
		if err != nil {
			return fmt.Errorf("marshal log config: %w", err)
		}
		secret.Data[common.SystemConfigKeyLog] = raw
	}
	if cfg.Deployment != nil {
		raw, err := json.Marshal(cfg.Deployment)
		if err != nil {
			return fmt.Errorf("marshal deployment config: %w", err)
		}
		secret.Data[common.SystemConfigKeyDeployment] = raw
	}
	return nil
}

// maskSystemConfig returns a copy of cfg with sensitive fields redacted, so
// it is safe to return to the UI.
func maskSystemConfig(cfg *systemConfig) *systemConfig {
	if cfg == nil {
		return nil
	}
	out := *cfg
	if cfg.Log != nil {
		masked := *cfg.Log
		masked.Config = logquery.MaskSensitiveFields(masked.Backend, masked.Config)
		out.Log = &masked
	}
	return &out
}

func (g *Gateway) handleGetSystemConfig(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	secret, err := g.getSystemConfigSecret(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusOK, systemConfig{})
			return
		}
		logger.Error(err, "failed to get system config secret")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get system config: %v", err)})
		return
	}

	c.JSON(http.StatusOK, maskSystemConfig(readSystemConfig(secret)))
}

const systemConfigMaxRetries = 5

func (g *Gateway) handleUpdateSystemConfig(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	var req systemConfig
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if err := validateSSHConfig(req.SSH); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid ssh config: %v", err)})
		return
	}
	if err := validateDeploymentConfig(req.Deployment); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid deployment config: %v", err)})
		return
	}

	for attempt := 0; attempt < systemConfigMaxRetries; attempt++ {
		secret, err := g.ensureSystemConfigSecret(ctx)
		if err != nil {
			logger.Error(err, "failed to ensure system config secret")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to access system config: %v", err)})
			return
		}
		if req.Log != nil {
			preserveMaskedLogFields(req.Log, readSystemConfig(secret).Log)
			if err := logquery.ValidateConfig(req.Log); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid log config: %v", err)})
				return
			}
		}

		if err := writeSystemConfigToSecret(secret, &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if _, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			if errors.IsConflict(err) {
				logger.Info("conflict updating system config secret, retrying", "attempt", attempt+1)
				continue
			}
			logger.Error(err, "failed to update system config secret")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update system config: %v", err)})
			return
		}

		// Return the merged, masked view so the UI sees the effective config.
		c.JSON(http.StatusOK, maskSystemConfig(readSystemConfig(secret)))
		return
	}

	c.JSON(http.StatusConflict, gin.H{"error": "failed to update system config secret after retries: too many conflicts"})
}
