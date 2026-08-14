package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

type imageRegistryItem struct {
	Name     string `json:"name"`
	Registry string `json:"registry"`
	Username string `json:"username"`
}

type createImageRegistryRequest struct {
	Name     string `json:"name" binding:"required"`
	Registry string `json:"registry" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type updateImageRegistryRequest struct {
	Registry string `json:"registry" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password,omitempty"`
}

func listImageRegistrySecrets(ctx context.Context, rawClient kubernetes.Interface) ([]corev1.Secret, error) {
	secretList, err := rawClient.CoreV1().Secrets(common.SecretNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{common.ImageRegistrySecretLabel: "true"}.AsSelector().String(),
	})
	if err != nil {
		return nil, err
	}
	return secretList.Items, nil
}

func (g *Gateway) handleListImageRegistries(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	secrets, err := listImageRegistrySecrets(ctx, g.rawClient)
	if err != nil {
		logger.Error(err, "failed to list image registry secrets")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list image registries: %v", err)})
		return
	}

	items := make([]imageRegistryItem, 0, len(secrets))
	for _, secret := range secrets {
		items = append(items, imageRegistryItem{
			Name:     secret.Name,
			Registry: secret.Annotations[common.ImageRegistryAnnotationRegistry],
			Username: secret.Annotations[common.ImageRegistryAnnotationUsername],
		})
	}

	c.JSON(http.StatusOK, items)
}

func (g *Gateway) handleGetImageRegistry(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	name := c.Param("name")
	secret, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "image registry not found"})
			return
		}
		logger.Error(err, "failed to get image registry secret")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get image registry: %v", err)})
		return
	}

	c.JSON(http.StatusOK, imageRegistryItem{
		Name:     secret.Name,
		Registry: secret.Annotations[common.ImageRegistryAnnotationRegistry],
		Username: secret.Annotations[common.ImageRegistryAnnotationUsername],
	})
}

func buildDockerConfigJSON(registry, username, password string) ([]byte, error) {
	dockerConfig := map[string]map[string]map[string]string{
		"auths": {
			registry: {
				"auth": base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
			},
		},
	}
	return json.Marshal(dockerConfig)
}

func (g *Gateway) handleCreateImageRegistry(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	var req createImageRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	configJSON, err := buildDockerConfigJSON(req.Registry, req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("marshal docker config: %v", err)})
		return
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: common.SecretNamespace,
			Labels: map[string]string{
				common.ImageRegistrySecretLabel: "true",
			},
			Annotations: map[string]string{
				common.ImageRegistryAnnotationRegistry: req.Registry,
				common.ImageRegistryAnnotationUsername: req.Username,
			},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{
			corev1.DockerConfigJsonKey: configJSON,
		},
	}

	if _, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
		if errors.IsAlreadyExists(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "image registry secret already exists"})
			return
		}
		logger.Error(err, "failed to create image registry secret")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create image registry: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "name": req.Name})
}

func (g *Gateway) handleUpdateImageRegistry(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	name := c.Param("name")
	var req updateImageRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	secret, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "image registry not found"})
			return
		}
		logger.Error(err, "failed to get image registry secret")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get image registry: %v", err)})
		return
	}

	if secret.Annotations == nil {
		secret.Annotations = make(map[string]string)
	}
	secret.Annotations[common.ImageRegistryAnnotationRegistry] = req.Registry
	secret.Annotations[common.ImageRegistryAnnotationUsername] = req.Username

	// Only update password if provided
	if req.Password != "" {
		configJSON, err := buildDockerConfigJSON(req.Registry, req.Username, req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("marshal docker config: %v", err)})
			return
		}
		secret.Data = map[string][]byte{
			corev1.DockerConfigJsonKey: configJSON,
		}
	}

	if _, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
		logger.Error(err, "failed to update image registry secret")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update image registry: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "name": name})
}

func (g *Gateway) handleDeleteImageRegistry(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "image registry not found"})
			return
		}
		logger.Error(err, "failed to delete image registry secret")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete image registry: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
