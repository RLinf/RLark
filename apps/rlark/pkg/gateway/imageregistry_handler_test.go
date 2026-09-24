package gateway

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

func TestBuildImageRegistryReplicationSecret(t *testing.T) {
	tests := []struct {
		name      string
		selection clusterSelection
		include   []string
	}{
		{name: "none", selection: clusterSelection{Mode: clusterSelectionNone}, include: []string{"/"}},
		{name: "selected", selection: clusterSelection{Mode: clusterSelectionSelected, Clusters: []string{"cluster-a", "cluster-b"}}, include: []string{"rlark-cluster-a", "rlark-cluster-b"}},
		{name: "all", selection: clusterSelection{Mode: clusterSelectionAll}, include: []string{"rlark-*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credential, err := normalizeImageRegistryCredential("Harbor", "https://harbor.example.com/v2/", "robot", "secret", tt.selection)
			require.NoError(t, err)
			secret, err := buildImageRegistryReplicationSecret("management", "ir-0123456789abcdef", credential)
			require.NoError(t, err)

			assert.Equal(t, "ir-0123456789abcdef-replication", secret.Name)
			assert.Equal(t, "management", secret.Namespace)
			assert.Equal(t, distribution.ReplicationSecretType, secret.Type)
			assert.Equal(t, "true", secret.Labels[common.ImageRegistryReplicationLabel])
			assert.NotContains(t, string(secret.Data[common.ImageRegistryCredentialDataKey]), `"password":""`)

			replication, err := distribution.Parse(secret.Data)
			require.NoError(t, err)
			require.NotNil(t, replication.Config.Targets.Namespaces)
			assert.Equal(t, tt.include, replication.Config.Targets.Namespaces.Include)
			assert.Equal(t, []string{"rlark-system"}, replication.Config.Targets.Namespaces.Exclude)
			require.Len(t, replication.Objects, 1)

			delivery := replication.Objects[0]
			assert.Equal(t, "ir-0123456789abcdef-delivery", delivery.GetName())
			assert.Equal(t, string(distribution.DeliverySecretType), nestedString(t, delivery.Object, "type"))
			deliveryData, found, err := unstructured.NestedStringMap(delivery.Object, "data")
			require.NoError(t, err)
			require.True(t, found)
			deliveryDefinition, err := distribution.Parse(map[string][]byte{
				"config":    decodeManifestData(t, deliveryData["config"]),
				"manifests": decodeManifestData(t, deliveryData["manifests"]),
			})
			require.NoError(t, err)
			require.Len(t, deliveryDefinition.Objects, 1)
			dockerSecret := deliveryDefinition.Objects[0]
			assert.Equal(t, "ir-0123456789abcdef", dockerSecret.GetName())
			assert.Equal(t, "rlark-system", dockerSecret.GetNamespace())
			assert.Equal(t, string(corev1.SecretTypeDockerConfigJson), nestedString(t, dockerSecret.Object, "type"))
			assert.Equal(t, "harbor.example.com", dockerSecret.GetAnnotations()[common.ImageRegistryAnnotationRegistry])

			dockerData, found, err := unstructured.NestedStringMap(dockerSecret.Object, "data")
			require.NoError(t, err)
			require.True(t, found)
			var config map[string]map[string]map[string]string
			require.NoError(t, json.Unmarshal(decodeManifestData(t, dockerData[corev1.DockerConfigJsonKey]), &config))
			assert.Equal(t, base64.StdEncoding.EncodeToString([]byte("robot:secret")), config["auths"]["harbor.example.com"]["auth"])
		})
	}
}

func TestNormalizeClusterSelection(t *testing.T) {
	selection, err := normalizeClusterSelection(clusterSelection{Mode: clusterSelectionSelected, Clusters: []string{" rlark-b ", "a", "a"}})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, selection.Clusters)

	for _, selection := range []clusterSelection{
		{Mode: clusterSelectionSelected},
		{Mode: clusterSelectionNone, Clusters: []string{"a"}},
		{Mode: clusterSelectionAll, Clusters: []string{"a"}},
		{Mode: "invalid"},
		{Mode: clusterSelectionSelected, Clusters: []string{"*"}},
		{Mode: clusterSelectionSelected, Clusters: []string{"invalid_name"}},
	} {
		_, err := normalizeClusterSelection(selection)
		assert.Error(t, err)
	}
	matched, err := path.Match(imageRegistryNoTargetsPattern, "rlark-cluster-a")
	require.NoError(t, err)
	assert.False(t, matched)
}

func TestImageRegistryHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gateway := &Gateway{
		config:        Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient:     fake.NewSimpleClientset(),
		jwtSigningKey: []byte("01234567890123456789012345678901"),
	}
	token, _, err := gateway.issueJWT("admin", "admin")
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request.Header.Set("Authorization", "Bearer "+token)
	})
	gateway.RegisterRoutes(router)

	created := performJSONRequest(t, router, http.MethodPost, "/api/v1/image-registries", map[string]any{
		"name": "Harbor", "registry": "https://harbor.example.com/", "username": "robot", "password": "secret",
		"clusterSelection": map[string]any{"mode": "Selected", "clusters": []string{"cluster-b", "cluster-a", "cluster-a"}},
	})
	assert.Equal(t, http.StatusCreated, created.Code)
	var item imageRegistryItem
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &item))
	assert.Regexp(t, `^ir-[0-9a-f]{16}$`, item.ID)
	assert.Equal(t, []string{"cluster-a", "cluster-b"}, item.ClusterSelection.Clusters)
	assert.NotContains(t, created.Body.String(), "secret")

	secret, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), imageRegistryReplicationName(item.ID), metav1.GetOptions{})
	require.NoError(t, err)
	secret.Finalizers = []string{"example.com/finalizer"}
	secret.Annotations = map[string]string{distribution.InventoryAnnotation: "inventory"}
	_, err = gateway.rawClient.CoreV1().Secrets("management").Update(context.Background(), secret, metav1.UpdateOptions{})
	require.NoError(t, err)

	updated := performJSONRequest(t, router, http.MethodPut, "/api/v1/image-registries/"+item.ID, map[string]any{
		"name": "Renamed Harbor", "registry": "harbor.example.com/team", "username": "new-robot",
		"clusterSelection": map[string]any{"mode": "All", "clusters": []string{}},
	})
	assert.Equal(t, http.StatusOK, updated.Code)
	assert.NotContains(t, updated.Body.String(), "secret")
	secret, err = gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), imageRegistryReplicationName(item.ID), metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com/finalizer"}, secret.Finalizers)
	assert.Equal(t, "inventory", secret.Annotations[distribution.InventoryAnnotation])
	var stored storedImageRegistryCredential
	require.NoError(t, json.Unmarshal(secret.Data[common.ImageRegistryCredentialDataKey], &stored))
	assert.Equal(t, "secret", stored.Password)
	assert.Equal(t, "Renamed Harbor", stored.Name)

	badPassword := performJSONRequest(t, router, http.MethodPut, "/api/v1/image-registries/"+item.ID, map[string]any{
		"name": "Harbor", "registry": "harbor.example.com", "username": "robot", "password": "",
		"clusterSelection": map[string]any{"mode": "None", "clusters": []string{}},
	})
	assert.Equal(t, http.StatusBadRequest, badPassword.Code)

	invalidID := performJSONRequest(t, router, http.MethodGet, "/api/v1/image-registries/not-an-id", nil)
	assert.Equal(t, http.StatusBadRequest, invalidID.Code)

	deleted := performJSONRequest(t, router, http.MethodDelete, "/api/v1/image-registries/"+item.ID, nil)
	assert.Equal(t, http.StatusAccepted, deleted.Code)
}

func nestedString(t *testing.T, object map[string]any, fields ...string) string {
	t.Helper()
	value, found, err := unstructured.NestedString(object, fields...)
	require.NoError(t, err)
	require.True(t, found)
	return value
}

func decodeManifestData(t *testing.T, value string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(value)
	require.NoError(t, err)
	return data
}

func performJSONRequest(t *testing.T, handler http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var requestBody bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&requestBody).Encode(body))
	}
	request := httptest.NewRequest(method, target, &requestBody)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
