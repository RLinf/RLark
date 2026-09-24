package gateway

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
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

	if subtle.ConstantTimeCompare([]byte(req.Password), []byte(expectedPW)) != 1 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	token, expiresAt, err := g.issueJWT(req.Username, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue access token"})
		return
	}

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     "rlark_access_token",
		Value:    token,
		Path:     "/api/",
		HttpOnly: true,
		Secure:   c.Request.TLS != nil,
		SameSite: http.SameSiteStrictMode,
		Expires:  expiresAt,
	})
	c.JSON(http.StatusOK, gin.H{
		"ok":        true,
		"role":      role,
		"token":     token,
		"expiresAt": expiresAt.UTC().Format(time.RFC3339),
	})
}

func (g *Gateway) readUIAuthSecret() (adminPW, userPW string, err error) {
	if g.rawClient == nil {
		return "", "", fmt.Errorf("raw kubernetes client not initialized")
	}

	ctx := context.Background()
	secret, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(ctx, common.UIAuthSecretName, metav1.GetOptions{})
	if err != nil {
		return "", "", err
	}

	adminPW = strings.TrimSpace(string(secret.Data[common.UIAuthAdminPasswordKey]))
	userPW = strings.TrimSpace(string(secret.Data[common.UIAuthUserPasswordKey]))
	return adminPW, userPW, nil
}

func (g *Gateway) loadJWTSigningKey(ctx context.Context) error {
	secret, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(ctx, common.UIAuthSecretName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	key := secret.Data[common.UIAuthJWTSigningKey]
	if len(key) < 32 {
		return fmt.Errorf("%s must contain at least 32 bytes", common.UIAuthJWTSigningKey)
	}
	g.jwtSigningKey = append([]byte(nil), key...)
	return nil
}

func (g *Gateway) issueJWT(username, role string) (string, time.Time, error) {
	if len(g.jwtSigningKey) < 32 {
		return "", time.Time{}, fmt.Errorf("JWT signing key is not initialized")
	}
	ttl := g.config.JWTTokenTTL
	if ttl <= 0 {
		ttl = 8 * time.Hour
	}
	now := time.Now()
	expiresAt := now.Add(ttl)
	claims := authClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "rlark-gateway",
			Subject:   username,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(g.jwtSigningKey)
	return token, expiresAt, err
}
