package gateway

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const (
	authSubjectKey = "auth.subject"
	authRoleKey    = "auth.role"
)

func (g *Gateway) requireJWT() gin.HandlerFunc {
	return func(c *gin.Context) {
		tokenString := ""
		if header := c.GetHeader("Authorization"); header != "" {
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
				respondError(c, http.StatusUnauthorized, CodeUnauthorized, "invalid authorization header")
				return
			}
			tokenString = parts[1]
		} else if cookie, err := c.Cookie("rlark_access_token"); err == nil {
			tokenString = cookie
		}
		if tokenString == "" {
			respondError(c, http.StatusUnauthorized, CodeUnauthorized, "authentication required")
			return
		}

		claims := &authClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
			return g.jwtSigningKey, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer("rlark-gateway"))
		if err != nil || !token.Valid || claims.Subject == "" || (claims.Role != "admin" && claims.Role != "user") {
			respondError(c, http.StatusUnauthorized, CodeUnauthorized, "invalid or expired access token")
			return
		}

		c.Set(authSubjectKey, claims.Subject)
		c.Set(authRoleKey, claims.Role)
		c.Next()
	}
}

func requireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(authRoleKey) != "admin" {
			respondError(c, http.StatusForbidden, CodeForbidden, "administrator access required")
			return
		}
		c.Next()
	}
}
