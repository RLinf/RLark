package gateway

import (
	"context"
	"encoding/json"
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
	// sshKeyAddedAtAnnotationPrefix 是记录每个 user 公钥添加时间的 annotation 前缀。
	// 完整 key = prefix + "." + user，value 是 RFC3339 字符串数组，与
	// Secret.Data[user] 里按行分割的 key 一一对应。
	sshKeyAddedAtAnnotationPrefix = "rlark.io/ssh-key-added-at"
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

	secret, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(ctx, common.SSHUserKeySecretName, metav1.GetOptions{})
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
			Namespace: g.managementNamespace(),
		},
		Data: make(map[string][]byte),
	}

	created, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Create(ctx, secret, metav1.CreateOptions{})
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

// sshKeyAddedAtAnnotationKey 返回记录某 user 公钥添加时间数组的 annotation key。
func sshKeyAddedAtAnnotationKey(user string) string {
	return sshKeyAddedAtAnnotationPrefix + "." + user
}

// readSSHKeyAddedAts 读取某 user 所有公钥的添加时间数组。
// 返回 nil 表示没有记录（老数据）。
func readSSHKeyAddedAts(secret *corev1.Secret, user string) []time.Time {
	if secret.Annotations == nil {
		return nil
	}
	raw, ok := secret.Annotations[sshKeyAddedAtAnnotationKey(user)]
	if !ok || raw == "" {
		return nil
	}
	var timestamps []string
	if err := json.Unmarshal([]byte(raw), &timestamps); err != nil {
		return nil
	}
	result := make([]time.Time, 0, len(timestamps))
	for _, ts := range timestamps {
		if t, err := time.Parse(time.RFC3339, ts); err == nil {
			result = append(result, t)
		}
	}
	return result
}

// writeSSHKeyAddedAts 把某 user 的添加时间数组写回 annotations。
// 长度为 0 时删除该 annotation。
func writeSSHKeyAddedAts(secret *corev1.Secret, user string, ats []time.Time) {
	key := sshKeyAddedAtAnnotationKey(user)
	if len(ats) == 0 {
		delete(secret.Annotations, key)
		return
	}
	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	timestamps := make([]string, len(ats))
	for i, t := range ats {
		timestamps[i] = t.UTC().Format(time.RFC3339)
	}
	data, err := json.Marshal(timestamps)
	if err != nil {
		return
	}
	secret.Annotations[key] = string(data)
}

// sshKeyAddedAt 读取某 user 第 index 个公钥的添加时间。
// 老数据没有 annotation 时回落到 secret.CreationTimestamp。
func sshKeyAddedAt(secret *corev1.Secret, user string, index int) time.Time {
	ats := readSSHKeyAddedAts(secret, user)
	if index >= 0 && index < len(ats) {
		return ats[index]
	}
	return secret.CreationTimestamp.Time
}

func (g *Gateway) handleListSSHUserKeys(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	secret, err := g.getSSHKeySecret(c.Request.Context())
	if err != nil {
		logger.Error(err, "failed to get ssh key secret")
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("failed to get ssh keys: %v", err))
		return
	}

	if secret == nil {
		c.JSON(http.StatusOK, []sshUserKeyItem{})
		return
	}

	userFilter := c.Query("user")
	keysByUser := parseSSHKeysFromSecret(secret)

	// Starts as an empty slice, not a nil one: a nil slice serializes to null and
	// would make the same endpoint answer null here but [] on the missing-secret
	// branch above.
	items := []sshUserKeyItem{}
	for user, keys := range keysByUser {
		if userFilter != "" && user != userFilter {
			continue
		}
		for i, key := range keys {
			items = append(items, sshUserKeyItem{
				Index:     i,
				User:      user,
				PublicKey: key,
				AddedAt:   sshKeyAddedAt(secret, user, i).UTC().Format(time.RFC3339),
			})
		}
	}

	c.JSON(http.StatusOK, items)
}

func findSSHKeyDuplicate(keysByUser map[string][]string, user, publicKey string) string {
	if _, exists := keysByUser[user]; exists {
		return "public key name already exists"
	}
	for _, keys := range keysByUser {
		for _, key := range keys {
			if key == publicKey {
				return "public key already exists"
			}
		}
	}
	return ""
}

func (g *Gateway) handleCreateSSHUserKey(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())

	var req createSSHUserKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "invalid request body")
		return
	}

	if req.PublicKey == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "public_key is required")
		return
	}

	req.User = strings.TrimSpace(req.User)
	if req.User == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "user is required")
		return
	}

	pubKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(req.PublicKey))
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, fmt.Sprintf("invalid public key: %v", err))
		return
	}

	normalizedKey := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(pubKey)))
	ctx := c.Request.Context()

	for attempt := 0; attempt < sshKeyMaxRetries; attempt++ {
		secret, err := g.ensureSSHKeySecret(ctx)
		if err != nil {
			logger.Error(err, "failed to ensure ssh key secret")
			respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("failed to access ssh key store: %v", err))
			return
		}

		if secret.Data == nil {
			secret.Data = make(map[string][]byte)
		}

		keysByUser := parseSSHKeysFromSecret(secret)
		if duplicate := findSSHKeyDuplicate(keysByUser, req.User, normalizedKey); duplicate != "" {
			respondError(c, http.StatusConflict, CodeResourceAlreadyExists, duplicate)
			return
		}

		lines := []string{normalizedKey}
		secret.Data[req.User] = []byte(strings.Join(lines, "\n"))

		// 记录添加时间：按 user 存 RFC3339 数组到 annotation，
		// index 与 Data 里的行号一一对应。老数据没有 annotation 时
		// 列表接口会回落到 secret.CreationTimestamp。
		// 当前 create 逻辑是覆盖该 user 的所有 key，所以这里直接重置为单元素。
		writeSSHKeyAddedAts(secret, req.User, []time.Time{time.Now()})

		_, err = g.rawClient.CoreV1().Secrets(g.managementNamespace()).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			if errors.IsConflict(err) {
				logger.Info("conflict updating ssh key secret, retrying", "attempt", attempt+1)
				continue
			}
			logger.Error(err, "failed to update ssh key secret")
			respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("failed to save ssh key: %v", err))
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true, "user": req.User})
		return
	}

	respondError(c, http.StatusConflict, CodeResourceConflict, "failed to update ssh key secret after retries: too many conflicts")
}

func (g *Gateway) handleDeleteSSHUserKey(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())

	user := c.Query("user")
	indexStr := c.Param("id")
	if user == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "user query parameter is required")
		return
	}

	index, err := strconv.Atoi(indexStr)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "invalid key index")
		return
	}

	ctx := c.Request.Context()

	for attempt := 0; attempt < sshKeyMaxRetries; attempt++ {
		secret, err := g.getSSHKeySecret(ctx)
		if err != nil || secret == nil {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "no ssh keys found")
			return
		}

		raw, ok := secret.Data[user]
		if !ok {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "no keys found for user")
			return
		}

		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if index < 0 || index >= len(lines) {
			respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "key index out of range")
			return
		}

		lines = append(lines[:index], lines[index+1:]...)
		if len(lines) > 0 {
			secret.Data[user] = []byte(strings.Join(lines, "\n"))
		} else {
			delete(secret.Data, user)
		}

		// 同步删除 annotation 里对应 index 的添加时间
		addedAts := readSSHKeyAddedAts(secret, user)
		if index < len(addedAts) {
			addedAts = append(addedAts[:index], addedAts[index+1:]...)
		}
		writeSSHKeyAddedAts(secret, user, addedAts)

		_, err = g.rawClient.CoreV1().Secrets(g.managementNamespace()).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			if errors.IsConflict(err) {
				logger.Info("conflict updating ssh key secret, retrying", "attempt", attempt+1)
				continue
			}
			logger.Error(err, "failed to update ssh key secret")
			respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("failed to delete ssh key: %v", err))
			return
		}

		c.JSON(http.StatusOK, gin.H{"ok": true})
		return
	}

	respondError(c, http.StatusConflict, CodeResourceConflict, "failed to delete ssh key secret after retries: too many conflicts")
}
