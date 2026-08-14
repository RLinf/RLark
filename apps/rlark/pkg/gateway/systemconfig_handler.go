package gateway

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

type systemConfigResponse struct {
	SSHJumpHost string `json:"sshJumpHost"`
	SSHJumpPort string `json:"sshJumpPort"`
}

type updateSystemConfigRequest struct {
	SSHJumpHost string `json:"sshJumpHost"`
	SSHJumpPort string `json:"sshJumpPort"`
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

func (g *Gateway) handleGetSystemConfig(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	secret, err := g.getSystemConfigSecret(ctx)
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusOK, systemConfigResponse{})
			return
		}
		logger.Error(err, "failed to get system config secret")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get system config: %v", err)})
		return
	}

	resp := systemConfigResponse{
		SSHJumpHost: string(secret.Data[common.SystemConfigKeySSHJumpHost]),
		SSHJumpPort: string(secret.Data[common.SystemConfigKeySSHJumpPort]),
	}
	c.JSON(http.StatusOK, resp)
}

const systemConfigMaxRetries = 5

func (g *Gateway) handleUpdateSystemConfig(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	var req updateSystemConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	for attempt := 0; attempt < systemConfigMaxRetries; attempt++ {
		secret, err := g.ensureSystemConfigSecret(ctx)
		if err != nil {
			logger.Error(err, "failed to ensure system config secret")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to access system config: %v", err)})
			return
		}

		if secret.Data == nil {
			secret.Data = map[string][]byte{}
		}
		secret.Data[common.SystemConfigKeySSHJumpHost] = []byte(req.SSHJumpHost)
		secret.Data[common.SystemConfigKeySSHJumpPort] = []byte(req.SSHJumpPort)

		if _, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			if errors.IsConflict(err) {
				logger.Info("conflict updating system config secret, retrying", "attempt", attempt+1)
				continue
			}
			logger.Error(err, "failed to update system config secret")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update system config: %v", err)})
			return
		}

		c.JSON(http.StatusOK, systemConfigResponse(req))
		return
	}

	c.JSON(http.StatusConflict, gin.H{"error": "failed to update system config secret after retries: too many conflicts"})
}
