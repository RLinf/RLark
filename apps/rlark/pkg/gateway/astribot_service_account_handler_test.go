package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

// accessTestRouter serves the secrets and service-account routes and two
// probes: one any caller may read, one for administrators.
func accessTestRouter(t *testing.T) (*Gateway, http.Handler, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gateway := &Gateway{config: Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}}, rawClient: fake.NewSimpleClientset(), jwtSigningKey: []byte("01234567890123456789012345678901")}
	admin, _, err := gateway.issueJWT("admin", "admin")
	require.NoError(t, err)
	router := gin.New()
	api := router.Group("/api/v1", gateway.requireJWT())
	api.GET("/whoami", func(c *gin.Context) {
		scope, err := gateway.resolveTenantScope(c)
		c.JSON(http.StatusOK, gin.H{"subject": c.GetString(authSubjectKey), "role": c.GetString(authRoleKey), "tenants": scope.tenants, "admin": scope.admin, "scopeError": err != nil})
	})
	adminAPI := api.Group("", requireAdmin())
	adminAPI.GET("/admin-only", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	adminAPI.GET("/secrets", gateway.listJobSecrets)
	adminAPI.POST("/secrets", gateway.createJobSecret)
	adminAPI.PUT("/secrets/:name", gateway.updateJobSecret)
	adminAPI.DELETE("/secrets/:name", gateway.deleteJobSecret)
	adminAPI.GET("/service-accounts", gateway.listServiceAccounts)
	adminAPI.POST("/service-accounts", gateway.createServiceAccount)
	adminAPI.POST("/service-accounts/:name/token", gateway.rotateServiceAccountToken)
	adminAPI.DELETE("/service-accounts/:name", gateway.deleteServiceAccount)
	return gateway, router, admin
}

func accessRequest(handler http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

// deliveredSecret returns the Secret a job secret declaration puts on its clusters.
func deliveredSecret(t *testing.T, gateway *Gateway, name string) (clusters []string, namespace string, data map[string]string) {
	t.Helper()
	declaration, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), jobSecretReplicationName(name), metav1.GetOptions{})
	require.NoError(t, err)
	replication, err := distribution.Parse(declaration.Data)
	require.NoError(t, err)
	deliveryData := nestedData(t, replication.Objects[0])
	delivery, err := distribution.Parse(map[string][]byte{
		"config": decodeStorageManifestData(t, deliveryData["config"]), "manifests": decodeStorageManifestData(t, deliveryData["manifests"]),
	})
	require.NoError(t, err)
	require.Len(t, delivery.Objects, 1)
	object := delivery.Objects[0]
	require.Equal(t, "Secret", object.GetKind())
	require.Equal(t, name, object.GetName())
	data = map[string]string{}
	for key, value := range object.Object["stringData"].(map[string]interface{}) {
		data[key] = value.(string)
	}
	return replication.Config.Targets.Namespaces.Include, object.GetNamespace(), data
}

func TestServiceAccountTokenAuthenticatesUntilRotatedOrDeleted(t *testing.T) {
	gateway, router, admin := accessTestRouter(t)
	created := accessRequest(router, "POST", "/api/v1/service-accounts", admin,
		`{"name":"fleet-dashboard","description":"submits and stops Jobs","deliver":{"clusters":["rlark-ksyun-gpu"],"secret_name":"fleet-rlark-token"}}`)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var answer struct{ Token string }
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &answer))
	require.True(t, strings.HasPrefix(answer.Token, "rlsa.fleet-dashboard."))

	// It is a user of the default tenant: it may use the API, not administer it.
	who := accessRequest(router, "GET", "/api/v1/whoami", answer.Token, "")
	require.Equal(t, http.StatusOK, who.Code)
	assert.JSONEq(t, `{"subject":"service-account:fleet-dashboard","role":"user","tenants":["default"],"admin":false,"scopeError":false}`, who.Body.String())
	assert.Equal(t, http.StatusForbidden, accessRequest(router, "GET", "/api/v1/admin-only", answer.Token, "").Code)
	assert.Equal(t, http.StatusForbidden, accessRequest(router, "POST", "/api/v1/service-accounts", answer.Token, `{"name":"another"}`).Code)

	// The token is on the Job's cluster as a secret, and only its hash is stored.
	clusters, namespace, data := deliveredSecret(t, gateway, "fleet-rlark-token")
	assert.Equal(t, []string{"rlark-ksyun-gpu"}, clusters)
	assert.Equal(t, "rlark-system", namespace)
	assert.Equal(t, answer.Token, data["token"])
	account, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), "rlark-service-account-fleet-dashboard", metav1.GetOptions{})
	require.NoError(t, err)
	for _, value := range account.Data {
		assert.NotContains(t, string(value), strings.TrimPrefix(answer.Token, "rlsa.fleet-dashboard."))
	}
	listed := accessRequest(router, "GET", "/api/v1/service-accounts", admin, "")
	assert.Contains(t, listed.Body.String(), `"deliverSecret":"fleet-rlark-token"`)
	assert.NotContains(t, listed.Body.String(), "token-sha256")

	// A wrong or tampered token is refused.
	assert.Equal(t, http.StatusUnauthorized, accessRequest(router, "GET", "/api/v1/whoami", answer.Token+"x", "").Code)
	assert.Equal(t, http.StatusUnauthorized, accessRequest(router, "GET", "/api/v1/whoami", "rlsa.nobody.abc", "").Code)

	// Rotation: the old token stops at once, the delivered secret follows.
	rotated := accessRequest(router, "POST", "/api/v1/service-accounts/fleet-dashboard/token", admin, "")
	require.Equal(t, http.StatusOK, rotated.Code, rotated.Body.String())
	old := answer.Token
	require.NoError(t, json.Unmarshal(rotated.Body.Bytes(), &answer))
	assert.NotEqual(t, old, answer.Token)
	assert.Equal(t, http.StatusUnauthorized, accessRequest(router, "GET", "/api/v1/whoami", old, "").Code)
	assert.Equal(t, http.StatusOK, accessRequest(router, "GET", "/api/v1/whoami", answer.Token, "").Code)
	_, _, data = deliveredSecret(t, gateway, "fleet-rlark-token")
	assert.Equal(t, answer.Token, data["token"])

	// Deletion revokes it and removes the delivered secret.
	require.Equal(t, http.StatusOK, accessRequest(router, "DELETE", "/api/v1/service-accounts/fleet-dashboard", admin, "").Code)
	assert.Equal(t, http.StatusUnauthorized, accessRequest(router, "GET", "/api/v1/whoami", answer.Token, "").Code)
	_, err = gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), jobSecretReplicationName("fleet-rlark-token"), metav1.GetOptions{})
	assert.Error(t, err)

	for name, body := range map[string]string{
		"bad role":     `{"name":"x","role":"root"}`,
		"bad name":     `{"name":"Fleet Dashboard"}`,
		"bad delivery": `{"name":"y","deliver":{"clusters":[],"secret_name":"t"}}`,
	} {
		assert.Equal(t, http.StatusBadRequest, accessRequest(router, "POST", "/api/v1/service-accounts", admin, body).Code, name)
	}
	_, err = gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), "rlark-service-account-y", metav1.GetOptions{})
	assert.Error(t, err, "an account whose delivery is invalid is not created")
}

func TestJobSecretIsDeliveredByNameAndNeverListedWithValues(t *testing.T) {
	gateway, router, admin := accessTestRouter(t)
	body := `{"name":"fleet-site-token","description":"the site's device-layer token","clusters":["rlark-ksyun-gpu","rlark-robot-s1u-01"],"data":{"site_token":"s3cret-value"}}`
	require.Equal(t, http.StatusCreated, accessRequest(router, "POST", "/api/v1/secrets", admin, body).Code)
	assert.Equal(t, http.StatusConflict, accessRequest(router, "POST", "/api/v1/secrets", admin, body).Code)

	clusters, namespace, data := deliveredSecret(t, gateway, "fleet-site-token")
	assert.Equal(t, []string{"rlark-ksyun-gpu", "rlark-robot-s1u-01"}, clusters)
	assert.Equal(t, "rlark-system", namespace)
	assert.Equal(t, map[string]string{"site_token": "s3cret-value"}, data)

	listed := accessRequest(router, "GET", "/api/v1/secrets", admin, "")
	assert.Contains(t, listed.Body.String(), `"keys":["site_token"]`)
	assert.NotContains(t, listed.Body.String(), "s3cret-value")

	updated := accessRequest(router, "PUT", "/api/v1/secrets/fleet-site-token", admin, `{"clusters":["rlark-ksyun-gpu"],"data":{"site_token":"rotated"}}`)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	clusters, _, data = deliveredSecret(t, gateway, "fleet-site-token")
	assert.Equal(t, []string{"rlark-ksyun-gpu"}, clusters)
	assert.Equal(t, "rotated", data["site_token"])
	assert.Equal(t, http.StatusNotFound, accessRequest(router, "PUT", "/api/v1/secrets/absent", admin, `{"clusters":["c"],"data":{"k":"v"}}`).Code)

	for name, invalid := range map[string]string{
		"reserved name": `{"name":"rlark-agent-cert","clusters":["c"],"data":{"k":"v"}}`,
		"job's own":     `{"name":"jo-123-ssh","clusters":["c"],"data":{"k":"v"}}`,
		"no cluster":    `{"name":"a","data":{"k":"v"}}`,
		"no data":       `{"name":"a","clusters":["c"]}`,
		"bad key":       `{"name":"a","clusters":["c"],"data":{"a/b":"v"}}`,
	} {
		assert.Equal(t, http.StatusBadRequest, accessRequest(router, "POST", "/api/v1/secrets", admin, invalid).Code, name)
	}
	user, _, err := gateway.issueJWT("user", "user")
	require.NoError(t, err)
	assert.Equal(t, http.StatusForbidden, accessRequest(router, "GET", "/api/v1/secrets", user, "").Code)

	require.Equal(t, http.StatusOK, accessRequest(router, "DELETE", "/api/v1/secrets/fleet-site-token", admin, "").Code)
	assert.Equal(t, http.StatusNotFound, accessRequest(router, "DELETE", "/api/v1/secrets/fleet-site-token", admin, "").Code)
}
