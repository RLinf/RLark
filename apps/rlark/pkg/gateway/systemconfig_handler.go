package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/logquery"
)

// systemConfig is the unified request/response shape for the system config
// API. Each category is a sub-struct; missing categories in a PUT body are
// left unchanged.
type systemConfig struct {
	SSH *sshConfig       `json:"ssh,omitempty"`
	Log *logquery.Config `json:"log,omitempty"`
}

type sshConfig struct {
	JumpHost string `json:"jumpHost,omitempty"`
	JumpPort string `json:"jumpPort,omitempty"`
}

func (g *Gateway) getSystemConfigSecret(ctx context.Context) (*corev1.Secret, error) {
	secret, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Get(ctx, common.SystemConfigSecretName, metav1.GetOptions{})
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
			Namespace: common.SecretNamespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{},
	}
	created, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Create(ctx, secret, metav1.CreateOptions{})
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

	// Validate the log backend config if provided, so we fail fast on bad
	// input instead of persisting something the querier can't construct.
	if req.Log != nil {
		if err := logquery.ValidateConfig(req.Log); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid log config: %v", err)})
			return
		}
	}

	for attempt := 0; attempt < systemConfigMaxRetries; attempt++ {
		secret, err := g.ensureSystemConfigSecret(ctx)
		if err != nil {
			logger.Error(err, "failed to ensure system config secret")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to access system config: %v", err)})
			return
		}

		if err := writeSystemConfigToSecret(secret, &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if _, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
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
