package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestLoginIssuesJWTAndProtectedRoutesValidateIt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := []byte("01234567890123456789012345678901")
	g := &Gateway{
		config: Config{
			KubeClientConfig: configs.KubernetesClientConfig{Namespace: "test"},
			JWTTokenTTL:      time.Hour,
		},
		rawClient: fake.NewSimpleClientset(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: common.UIAuthSecretName, Namespace: "test"},
			Data: map[string][]byte{
				common.UIAuthAdminPasswordKey: []byte("admin-password"),
				common.UIAuthUserPasswordKey:  []byte("user-password"),
				common.UIAuthJWTSigningKey:    key,
			},
		}),
		jwtSigningKey: key,
	}

	router := gin.New()
	router.POST("/api/v1/auth/login", g.handleLogin)
	router.GET("/api/v1/protected", g.requireJWT(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"subject": c.GetString(authSubjectKey), "role": c.GetString(authRoleKey)})
	})

	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"user","password":"user-password"}`))
	login.Header.Set("Content-Type", "application/json")
	loginResponse := httptest.NewRecorder()
	router.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", loginResponse.Code, loginResponse.Body.String())
	}
	var body struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Token == "" || body.Role != "user" {
		t.Fatalf("unexpected login response: %#v", body)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/protected", nil)
	request.Header.Set("Authorization", "Bearer "+body.Token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"subject":"user"`) {
		t.Fatalf("protected status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestJWTMiddlewareRejectsMissingExpiredAndWrongAlgorithmTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := []byte("01234567890123456789012345678901")
	g := &Gateway{jwtSigningKey: key}
	router := gin.New()
	router.GET("/protected", g.requireJWT(), func(c *gin.Context) { c.Status(http.StatusOK) })

	expiredClaims := authClaims{
		Role: "user",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: "rlark-gateway", Subject: "user", ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	none := jwt.NewWithClaims(jwt.SigningMethodNone, expiredClaims)
	unsigned, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	for name, authorization := range map[string]string{
		"missing":          "",
		"malformed header": "Token abc",
		"expired":          "Bearer " + expired,
		"wrong algorithm":  "Bearer " + unsigned,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			request.Header.Set("Authorization", authorization)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", response.Code)
			}
		})
	}
}

func TestAdminMiddlewareEnforcesRole(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := []byte("01234567890123456789012345678901")
	g := &Gateway{config: Config{JWTTokenTTL: time.Hour}, jwtSigningKey: key}
	router := gin.New()
	router.GET("/admin", g.requireJWT(), requireAdmin(), func(c *gin.Context) { c.Status(http.StatusOK) })

	for role, want := range map[string]int{"user": http.StatusForbidden, "admin": http.StatusOK} {
		t.Run(role, func(t *testing.T) {
			token, _, err := g.issueJWT(role, role)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "/admin", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != want {
				t.Fatalf("status = %d, want %d", response.Code, want)
			}
		})
	}
}

func TestRegisteredRoutePermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := []byte("01234567890123456789012345678901")
	g := &Gateway{config: Config{JWTTokenTTL: time.Hour}, jwtSigningKey: key, rawClient: fake.NewSimpleClientset()}
	router := gin.New()
	g.RegisterRoutes(router)
	userToken, _, err := g.issueJWT("user", "user")
	if err != nil {
		t.Fatal(err)
	}
	adminToken, _, err := g.issueJWT("admin", "admin")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		method string
		path   string
		user   int
		admin  int
	}{
		{name: "user route", method: http.MethodGet, path: "/api/v1/rlinf.io/v1alpha1/jobs/anything/metrics", user: http.StatusNotImplemented, admin: http.StatusNotImplemented},
		{name: "shared system config read", method: http.MethodGet, path: "/api/v1/system-config", user: http.StatusOK, admin: http.StatusOK},
		{name: "admin system config write", method: http.MethodPut, path: "/api/v1/system-config", user: http.StatusForbidden, admin: http.StatusBadRequest},
		{name: "shared ssh keys", method: http.MethodGet, path: "/api/v1/ssh-user-keys", user: http.StatusOK, admin: http.StatusOK},
		{name: "admin write on shared resource", method: http.MethodPost, path: "/api/v1/storage/storageclass", user: http.StatusForbidden, admin: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for role, tc := range map[string]struct {
				token string
				want  int
			}{"user": {userToken, test.user}, "admin": {adminToken, test.admin}} {
				t.Run(role, func(t *testing.T) {
					request := httptest.NewRequest(test.method, test.path, nil)
					request.Header.Set("Authorization", "Bearer "+tc.token)
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if response.Code != tc.want {
						t.Fatalf("status = %d, want %d, body = %s", response.Code, tc.want, response.Body.String())
					}
				})
			}
		})
	}
}
