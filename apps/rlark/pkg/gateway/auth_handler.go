package gateway

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (g *Gateway) handleLogin(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}

	adminPW, userPW, err := g.readUIAuthSecret()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to read auth secret, err: %v", err)})
		return
	}

	role := ""
	expectedPW := ""
	switch req.Username {
	case "admin":
		role = "admin"
		expectedPW = adminPW
	case "user":
		role = "user"
		expectedPW = userPW
	default:
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	if req.Password != expectedPW {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":   true,
		"role": role,
	})
}

func (g *Gateway) readUIAuthSecret() (adminPW, userPW string, err error) {
	if g.rawClient == nil {
		return "", "", fmt.Errorf("raw kubernetes client not initialized")
	}

	ctx := context.Background()
	secret, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Get(ctx, common.UIAuthSecretName, metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}

	adminPW = strings.TrimSpace(string(secret.Data["admin-password"]))
	userPW = strings.TrimSpace(string(secret.Data["user-password"]))
	return adminPW, userPW, nil
}
