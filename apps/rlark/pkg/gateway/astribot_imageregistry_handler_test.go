package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
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

// The cluster's agent and Station's Pods each have the credential where they run.
func TestWithStationSecret(t *testing.T) {
	agent := corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ir-0123456789abcdef", Namespace: "rlark-system"}, Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("{}")}}
	secrets := withStationSecret(agent)
	require.Len(t, secrets, 2)
	assert.Equal(t, agent, secrets[0])
	station := agent
	station.Namespace = "station-system"
	assert.Equal(t, station, secrets[1])
}

// An administrator reads a registry's password back; an update that leaves out the account keeps it, so a new
// registry name and clusters need nobody to know the account.
func TestImageRegistryPasswordAndKeptAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &Gateway{config: Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}}, rawClient: fake.NewSimpleClientset(), jwtSigningKey: []byte("01234567890123456789012345678901")}
	token, _, err := gateway.issueJWT("admin", "admin")
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Request.Header.Set("Authorization", "Bearer "+token) })
	gateway.RegisterRoutes(router)

	created := performJSONRequest(t, router, http.MethodPost, "/api/v1/image-registries", map[string]any{
		"name": "Harbor", "registry": "https://harbor.example.com/", "username": "robot", "password": "secret",
		"clusterSelection": map[string]any{"mode": "Selected", "clusters": []string{"cluster-a"}},
	})
	require.Equal(t, http.StatusCreated, created.Code)
	var item imageRegistryItem
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &item))

	password := performJSONRequest(t, router, http.MethodGet, "/api/v1/image-registries/"+item.ID+"/password", nil)
	assert.Equal(t, http.StatusOK, password.Code)
	assert.Equal(t, "no-store", password.Header().Get("Cache-Control"))
	assert.JSONEq(t, `{"password":"secret"}`, password.Body.String())
	missing := performJSONRequest(t, router, http.MethodGet, "/api/v1/image-registries/ir-0000000000000000/password", nil)
	assert.Equal(t, http.StatusNotFound, missing.Code)

	moved := performJSONRequest(t, router, http.MethodPut, "/api/v1/image-registries/"+item.ID, map[string]any{
		"name": "Harbor by name", "registry": "registry.rlark.internal:5443",
		"clusterSelection": map[string]any{"mode": "Selected", "clusters": []string{"ksyun-gpu"}},
	})
	assert.Equal(t, http.StatusOK, moved.Code)
	secret, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), imageRegistryReplicationName(item.ID), metav1.GetOptions{})
	require.NoError(t, err)
	var stored storedImageRegistryCredential
	require.NoError(t, json.Unmarshal(secret.Data[common.ImageRegistryCredentialDataKey], &stored))
	assert.Equal(t, storedImageRegistryCredential{Version: imageRegistryCredentialVersion, Name: "Harbor by name", Registry: "registry.rlark.internal:5443",
		Username: "robot", Password: "secret", ClusterSelection: clusterSelection{Mode: clusterSelectionSelected, Clusters: []string{"ksyun-gpu"}}}, stored)
}
