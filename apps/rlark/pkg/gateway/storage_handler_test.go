package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

func TestBuildStorageClassReplicationSecret(t *testing.T) {
	req := CreateStorageClassRequest{
		Name: "object-storage", Description: "test storage", Clusters: []string{"rlark-cluster-a", "rlark-cluster-b"},
		Provider: "Minio", Endpoint: "https://minio.example.com", AccessKeyId: "access", AccessKeySecret: "secret", Bucket: "bucket",
	}
	stored, err := normalizeStorageClass(req)
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", stored)
	require.NoError(t, err)

	assert.Equal(t, "sc-0123456789abcdef-replication", secret.Name)
	assert.Equal(t, "management", secret.Namespace)
	assert.Equal(t, distribution.ReplicationSecretType, secret.Type)
	assert.Equal(t, "true", secret.Labels[storageClassReplicationLabel])

	replication, err := distribution.Parse(secret.Data)
	require.NoError(t, err)
	require.NotNil(t, replication.Config.Targets.Namespaces)
	assert.Equal(t, []string{"rlark-cluster-a", "rlark-cluster-b"}, replication.Config.Targets.Namespaces.Include)
	require.Len(t, replication.Objects, 1)

	delivery := replication.Objects[0]
	assert.Equal(t, "sc-0123456789abcdef-delivery", delivery.GetName())
	assert.Equal(t, string(distribution.DeliverySecretType), nestedString(t, delivery.Object, "type"))
	deliveryData := nestedData(t, delivery)
	deliveryDefinition, err := distribution.Parse(map[string][]byte{
		"config":    decodeStorageManifestData(t, deliveryData["config"]),
		"manifests": decodeStorageManifestData(t, deliveryData["manifests"]),
	})
	require.NoError(t, err)
	assert.True(t, deliveryDefinition.Config.Security.AllowClusterScoped)
	require.Len(t, deliveryDefinition.Objects, 2)

	var storageSecret, storageClass *unstructured.Unstructured
	for _, object := range deliveryDefinition.Objects {
		switch object.GetKind() {
		case "Secret":
			storageSecret = object
		case "StorageClass":
			storageClass = object
		}
	}
	require.NotNil(t, storageSecret)
	require.NotNil(t, storageClass)
	assert.Equal(t, "sc-0123456789abcdef-secret", storageSecret.GetName())
	assert.Equal(t, "kube-system", storageSecret.GetNamespace())
	assert.Equal(t, "sc-0123456789abcdef", storageClass.GetName())
	assert.Equal(t, "object-storage", storageClass.GetAnnotations()[annoStorageClassName])
	assert.Empty(t, storageClass.GetNamespace())
	assert.Equal(t, "rclone.csi.veloxpack.io", nestedString(t, storageClass.Object, "provisioner"))
	parameters, found, err := unstructured.NestedStringMap(storageClass.Object, "parameters")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "sc-0123456789abcdef-secret", parameters["csi.storage.k8s.io/node-publish-secret-name"])
	assert.Equal(t, "kube-system", parameters["csi.storage.k8s.io/node-publish-secret-namespace"])
}

func TestBuildStorageClassReplicationSecretSkipsInvalidNameAnnotation(t *testing.T) {
	req := CreateStorageClassRequest{
		Name: "invalid name", Clusters: []string{"rlark-cluster-a"},
		Provider: "Minio", Endpoint: "https://minio.example.com", AccessKeySecret: "secret", Bucket: "bucket",
	}
	stored, err := normalizeStorageClass(req)
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", stored)
	require.NoError(t, err)

	replication, err := distribution.Parse(secret.Data)
	require.NoError(t, err)
	deliveryData := nestedData(t, replication.Objects[0])
	deliveryDefinition, err := distribution.Parse(map[string][]byte{
		"config":    decodeStorageManifestData(t, deliveryData["config"]),
		"manifests": decodeStorageManifestData(t, deliveryData["manifests"]),
	})
	require.NoError(t, err)
	for _, object := range deliveryDefinition.Objects {
		if object.GetKind() == "StorageClass" {
			assert.NotContains(t, object.GetAnnotations(), annoStorageClassName)
			return
		}
	}
	t.Fatal("StorageClass manifest not found")
}

func TestListStorageClassReadsReplicationDeclarations(t *testing.T) {
	gin.SetMode(gin.TestMode)
	req := CreateStorageClassRequest{
		Name: "object-storage", Description: "test storage", Clusters: []string{"rlark-cluster-a", "rlark-cluster-b"},
		Provider: "Minio", Endpoint: "https://minio.example.com", AccessKeyId: "access", AccessKeySecret: "secret", Bucket: "bucket",
	}
	stored, err := normalizeStorageClass(req)
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", stored)
	require.NoError(t, err)
	gateway := &Gateway{
		config:    Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient: fake.NewSimpleClientset(secret),
	}
	router := gin.New()
	router.GET("/storage", gateway.listStorageClass)

	response := performJSONRequest(t, router, http.MethodGet, "/storage?clusters=rlark-cluster-b", nil)
	require.Equal(t, http.StatusOK, response.Code)
	var body struct {
		Data    map[string]StorageClassData `json:"data"`
		Success bool                        `json:"success"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.True(t, body.Success)
	item, ok := body.Data["object-storage"]
	require.True(t, ok)
	assert.Equal(t, []string{"rlark-cluster-b"}, item.Clusters)
	assert.Equal(t, "bucket", item.Bucket)
	assert.Equal(t, "Minio", item.Provider)

	_, err = gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), secret.Name, metav1.GetOptions{})
	require.NoError(t, err)
}

func nestedData(t *testing.T, object *unstructured.Unstructured) map[string]string {
	t.Helper()
	data, found, err := unstructured.NestedStringMap(object.Object, "data")
	require.NoError(t, err)
	require.True(t, found)
	return data
}

func decodeStorageManifestData(t *testing.T, value string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(value)
	require.NoError(t, err)
	return data
}
