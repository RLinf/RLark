package gateway

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	gossh "golang.org/x/crypto/ssh"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

const (
	sshKeyMaxRetries = 5
)

type sshUserKeyItem struct {
	Index     int    `json:"index"`
	User      string `json:"user"`
	PublicKey string `json:"public_key"`
	AddedAt   string `json:"added_at"`
	Notes     string `json:"notes,omitempty"`
}

type createSSHUserKeyRequest struct {
	User      string `json:"user" binding:"required"`
	PublicKey string `json:"public_key" binding:"required"`
	Notes     string `json:"notes,omitempty"`
}

func (g *Gateway) getSSHKeySecret(ctx context.Context) (*corev1.Secret, error) {
	if g.rawClient == nil {
		return nil, fmt.Errorf("raw kubernetes client not initialized")
	}

	secret, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Get(ctx, common.SSHUserKeySecretName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	return secret, nil
}

func (g *Gateway) ensureSSHKeySecret(ctx context.Context) (*corev1.Secret, error) {
	secret, err := g.getSSHKeySecret(ctx)
	if err != nil {
		return nil, err
	}

	if secret != nil {
		return secret, nil
	}

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.SSHUserKeySecretName,
			Namespace: common.SecretNamespace,
		},
		Data: make(map[string][]byte),
	}

	created, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		if errors.IsAlreadyExists(err) {
			return g.getSSHKeySecret(ctx)
		}
		return nil, fmt.Errorf("create ssh key secret: %w", err)
	}

	return created, nil
}

func parseSSHKeysFromSecret(secret *corev1.Secret) map[string][]string {
	result := make(map[string][]string)
	for user, raw := range secret.Data {
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		var keys []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				keys = append(keys, line)
			}
		}

		if len(keys) > 0 {
			result[user] = keys
		}
	}

	return result
}

func (g *Gateway) handleListSSHUserKeys(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	secret, err := g.getSSHKeySecret(c.Request.Context())
	if err != nil {
		logger.Error(err, "failed to get ssh key secret")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get ssh keys: %v", err)})
		return
	}

	if secret == nil {
		c.JSON(http.StatusOK, []sshUserKeyItem{})
		return
	}

	userFilter := c.Query("user")
	keysByUser := parseSSHKeysFromSecret(secret)

	var items []sshUserKeyItem
	for user, keys := range keysByUser {
		if userFilter != "" && user != userFilter {
			continue
		}
		for i, key := range keys {
			items = append(items, sshUserKeyItem{
				Index:     i,
				User:      user,
				PublicKey: key,
				AddedAt:   secret.CreationTimestamp.Format(time.RFC3339),
			})
		}
	}

	c.JSON(http.StatusOK, items)
}

func (g *Gateway) handleCreateSSHUserKey(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())

	var req createSSHUserKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.PublicKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "public_key is required"})
		return
	}

	pubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(req.PublicKey))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid public key: %v", err)})
		return
	}

	normalizedKey := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pubKey)))
	ctx := c.Request.Context()

	for attempt := 0; attempt < sshKeyMaxRetries; attempt++ {
		secret, err := g.ensureSSHKeySecret(ctx)
		if err != nil {
			logger.Error(err, "failed to ensure ssh key secret")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to access ssh key store: %v", err)})
			return
		}

		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		existing := strings.TrimSpace(string(secret.Data[req.User]))
		var lines []string
		if existing != "" {
			lines = strings.Split(existing, "\n")
		}

		for _, line := range lines {
			if strings.TrimSpace(line) == normalizedKey {
				c.JSON(http.StatusConflict, gin.H{"error": "public key already exists for this user"})
				return
			}
		}
		lines = append(lines, normalizedKey)
		secret.Data[req.User] = []byte(strings.Join(lines, "\n"))

		_, err = g.rawClient.CoreV1().Secrets(common.SecretNamespace).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			if errors.IsConflict(err) {
				logger.Info("conflict updating ssh key secret, retrying", "attempt", attempt+1)
				continue
			}
			logger.Error(err, "failed to update ssh key secret")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to save ssh key: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true, "user": req.User})
		return
	}

	c.JSON(http.StatusConflict, gin.H{"error": "failed to update ssh key secret after retries: too many conflicts"})
}

func (g *Gateway) handleDeleteSSHUserKey(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())

	user := c.Query("user")
	indexStr := c.Param("id")
	if user == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user query parameter is required"})
		return
	}

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key index"})
		return
	}

	ctx := c.Request.Context()

	for attempt := 0; attempt < sshKeyMaxRetries; attempt++ {
		secret, err := g.getSSHKeySecret(ctx)
		if err != nil || secret == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "no ssh keys found"})
			return
		}

		raw, ok := secret.Data[user]
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "no keys found for user"})
			return
		}

		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if index < 0 || index >= len(lines) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key index out of range"})
			return
		}

		lines = append(lines[:index], lines[index+1:]...)
		if len(lines) > 0 {
			secret.Data[user] = []byte(strings.Join(lines, "\n"))
		} else {
			delete(secret.Data, user)
		}

		_, err = g.rawClient.CoreV1().Secrets(common.SecretNamespace).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			if errors.IsConflict(err) {
				logger.Info("conflict updating ssh key secret, retrying", "attempt", attempt+1)
				continue
			}
			logger.Error(err, "failed to update ssh key secret")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete ssh key: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	c.JSON(http.StatusConflict, gin.H{"error": "failed to delete ssh key secret after retries: too many conflicts"})
}
