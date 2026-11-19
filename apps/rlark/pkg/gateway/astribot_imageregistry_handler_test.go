package gateway

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
)

func TestRegistryPasswordRequiresAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &Gateway{config: Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}}, rawClient: fake.NewSimpleClientset(), jwtSigningKey: []byte("01234567890123456789012345678901")}
	token, _, err := gateway.issueJWT("user", "user")
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Request.Header.Set("Authorization", "Bearer "+token) })
	gateway.RegisterRoutes(router)
	response := performJSONRequest(t, router, http.MethodGet, "/api/v1/image-registries/ir-0123456789abcdef/password", nil)
	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
}
