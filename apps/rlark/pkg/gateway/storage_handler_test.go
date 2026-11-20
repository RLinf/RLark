package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/rlinf/rlark/apps/rlark/pkg/gateway/storage"
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
	assert.Equal(t, "object-storage", item.Name)
	// The human-readable name is the map key; storageClassName is the actual
	// Kubernetes StorageClass name that task PVCs must reference.
	assert.Equal(t, "sc-0123456789abcdef", item.StorageClassName)
	assert.Equal(t, []string{"rlark-cluster-b"}, item.Clusters)
	assert.Equal(t, "bucket", item.Bucket)
	assert.Equal(t, "Minio", item.Provider)
	assert.Equal(t, "ac*****ss", item.AccessKeyId)
	assert.Equal(t, secret.CreationTimestamp.UTC().Format(time.RFC3339), item.CreatedAt)

	_, err = gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), secret.Name, metav1.GetOptions{})
	require.NoError(t, err)
}

func TestUpdateStorageClassTreatsMaskedAccessKeyIDAsUnchanged(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initial, err := normalizeStorageClass(CreateStorageClassRequest{
		Name: "object-storage", Clusters: []string{"rlark-cluster-a"}, Provider: "Minio",
		Endpoint: "https://minio.example.com", AccessKeyId: "access", AccessKeySecret: "stored-secret", Bucket: "bucket",
	})
	require.NoError(t, err)
	originalValidate := validateStorageClassCredentials
	var validated storedStorageClass
	validateStorageClassCredentials = func(_ context.Context, stored storedStorageClass) error {
		validated = stored
		return nil
	}
	t.Cleanup(func() { validateStorageClassCredentials = originalValidate })
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", initial)
	require.NoError(t, err)
	gateway := &Gateway{
		config:         Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient:      fake.NewSimpleClientset(secret),
		storageClients: map[string]*storage.Client{},
	}
	router := gin.New()
	router.PUT("/storage/:name", gateway.updateStorageClass)

	response := performJSONRequest(t, router, http.MethodPut, "/storage/object-storage", map[string]any{
		"name": "object-storage", "description": "updated description", "clusters": []string{"rlark-cluster-a"}, "provider": "Minio",
		"endpoint": "https://minio.example.com", "access_key_id": "ac*****ss", "access_key_secret": "", "bucket": "bucket", "path_style": true,
	})
	require.Equal(t, http.StatusOK, response.Code)

	updated, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), secret.Name, metav1.GetOptions{})
	require.NoError(t, err)
	_, stored, err := storageClassFromSecret(updated)
	require.NoError(t, err)
	assert.Equal(t, "updated description", stored.Description)
	assert.Equal(t, "stored-secret", stored.AccessKey)
	assert.Equal(t, "access", stored.AccessKeyID)
	assert.Equal(t, "stored-secret", validated.AccessKey)
	assert.Equal(t, "access", validated.AccessKeyID)
}

func TestUpdateStorageClassRetainsStoredAccessSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initial, err := normalizeStorageClass(CreateStorageClassRequest{
		Name: "object-storage", Clusters: []string{"rlark-cluster-a"}, Provider: "Minio",
		Endpoint: "https://minio.example.com", AccessKeyId: "access", AccessKeySecret: "stored-secret", Bucket: "bucket",
	})
	require.NoError(t, err)
	originalValidate := validateStorageClassCredentials
	var validated storedStorageClass
	validateStorageClassCredentials = func(_ context.Context, stored storedStorageClass) error {
		validated = stored
		return nil
	}
	t.Cleanup(func() { validateStorageClassCredentials = originalValidate })
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", initial)
	require.NoError(t, err)
	gateway := &Gateway{
		config:         Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient:      fake.NewSimpleClientset(secret),
		storageClients: map[string]*storage.Client{},
	}
	router := gin.New()
	router.PUT("/storage/:name", gateway.updateStorageClass)

	response := performJSONRequest(t, router, http.MethodPut, "/storage/object-storage", map[string]any{
		"name": "object-storage", "clusters": []string{"rlark-cluster-a"}, "provider": "Minio",
		"endpoint": "https://minio.example.com", "access_key_id": "", "access_key_secret": "", "bucket": "bucket", "path_style": true,
	})
	require.Equal(t, http.StatusOK, response.Code)

	updated, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), secret.Name, metav1.GetOptions{})
	require.NoError(t, err)
	_, stored, err := storageClassFromSecret(updated)
	require.NoError(t, err)
	assert.Equal(t, "stored-secret", stored.AccessKey)
	assert.Equal(t, "access", stored.AccessKeyID)
	assert.True(t, stored.PathStyle)
	assert.Equal(t, "stored-secret", validated.AccessKey)
	assert.Equal(t, "access", validated.AccessKeyID)
	assert.True(t, validated.PathStyle)
}

func TestCreateStorageClassValidatesCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalValidate := validateStorageClassCredentials
	var validated storedStorageClass
	validateStorageClassCredentials = func(_ context.Context, stored storedStorageClass) error {
		validated = stored
		return nil
	}
	t.Cleanup(func() { validateStorageClassCredentials = originalValidate })

	gateway := &Gateway{
		config:    Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient: fake.NewSimpleClientset(),
	}
	router := gin.New()
	router.POST("/storage", gateway.createStorageClass)

	response := performJSONRequest(t, router, http.MethodPost, "/storage", map[string]any{
		"name": "object-storage", "clusters": []string{"rlark-cluster-a"}, "provider": "Minio",
		"endpoint": "https://minio.example.com", "access_key_id": "access", "access_key_secret": "secret", "bucket": "bucket", "path_style": true,
	})
	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "Minio", validated.Provider)
	assert.Equal(t, "https://minio.example.com", validated.Endpoint)
	assert.Equal(t, "access", validated.AccessKeyID)
	assert.Equal(t, "secret", validated.AccessKey)
	assert.Equal(t, "bucket", validated.Bucket)
	assert.True(t, validated.PathStyle)
}

func TestCreateStorageClassRejectsInvalidCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalValidate := validateStorageClassCredentials
	validateStorageClassCredentials = func(context.Context, storedStorageClass) error {
		return errors.New("access denied")
	}
	t.Cleanup(func() { validateStorageClassCredentials = originalValidate })

	gateway := &Gateway{
		config:    Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient: fake.NewSimpleClientset(),
	}
	router := gin.New()
	router.POST("/storage", gateway.createStorageClass)

	response := performJSONRequest(t, router, http.MethodPost, "/storage", map[string]any{
		"name": "object-storage", "clusters": []string{"rlark-cluster-a"}, "provider": "Minio",
		"endpoint": "https://minio.example.com", "access_key_secret": "secret", "bucket": "bucket",
	})
	require.Equal(t, http.StatusBadRequest, response.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, CodeInvalidRequest, body.Code)
	assert.Equal(t, "storage credentials cannot access the configured bucket", body.Message)
}

func TestUpdateStorageClassRejectsInvalidCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initial, err := normalizeStorageClass(CreateStorageClassRequest{
		Name: "object-storage", Clusters: []string{"rlark-cluster-a"}, Provider: "Minio",
		Endpoint: "https://minio.example.com", AccessKeyId: "access", AccessKeySecret: "stored-secret", Bucket: "bucket",
	})
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", initial)
	require.NoError(t, err)
	originalValidate := validateStorageClassCredentials
	validateStorageClassCredentials = func(context.Context, storedStorageClass) error {
		return errors.New("access denied")
	}
	t.Cleanup(func() { validateStorageClassCredentials = originalValidate })

	gateway := &Gateway{
		config:         Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient:      fake.NewSimpleClientset(secret),
		storageClients: map[string]*storage.Client{},
	}
	router := gin.New()
	router.PUT("/storage/:name", gateway.updateStorageClass)

	response := performJSONRequest(t, router, http.MethodPut, "/storage/object-storage", map[string]any{
		"name": "object-storage", "clusters": []string{"rlark-cluster-a"}, "provider": "Minio",
		"endpoint": "https://minio.example.com", "access_key_id": "", "access_key_secret": "", "bucket": "bucket",
	})
	require.Equal(t, http.StatusBadRequest, response.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, CodeInvalidRequest, body.Code)
	assert.Equal(t, "storage credentials cannot access the configured bucket", body.Message)

	current, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), secret.Name, metav1.GetOptions{})
	require.NoError(t, err)
	_, stored, err := storageClassFromSecret(current)
	require.NoError(t, err)
	assert.Equal(t, initial, stored)
}

func TestUpdateStorageClassReportsStructuredRequiredFieldErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	initial, err := normalizeStorageClass(CreateStorageClassRequest{
		Name: "object-storage", Clusters: []string{"rlark-cluster-a"}, Provider: "Minio",
		Endpoint: "https://minio.example.com", AccessKeyId: "access", AccessKeySecret: "stored-secret", Bucket: "bucket",
	})
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", initial)
	require.NoError(t, err)
	gateway := &Gateway{
		config:         Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient:      fake.NewSimpleClientset(secret),
		storageClients: map[string]*storage.Client{},
	}
	router := gin.New()
	router.PUT("/storage/:name", gateway.updateStorageClass)

	response := performJSONRequest(t, router, http.MethodPut, "/storage/object-storage", map[string]any{
		"name": "object-storage", "clusters": []string{"rlark-cluster-a"}, "provider": "Minio",
		"endpoint": "https://minio.example.com", "bucket": "",
	})
	require.Equal(t, http.StatusBadRequest, response.Code)
	var body ErrorResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, CodeStorageClassRequiredFields, body.Code)
	assert.Equal(t, []any{"bucket"}, body.Details["fields"])
}

func TestNormalizeStorageClassResolvesProviderAndEndpoint(t *testing.T) {
	cases := []struct {
		name          string
		req           CreateStorageClassRequest
		wantProvider  string
		wantEndpoint  string
		wantPathStyle bool
	}{
		{
			name: "derives the regional endpoint when omitted",
			req: CreateStorageClassRequest{
				Name: "oss", Clusters: []string{"rlark-cluster-a"}, Provider: "Alibaba",
				Region: "cn-beijing", AccessKeySecret: "secret", Bucket: "bucket",
			},
			wantProvider: "Alibaba", wantEndpoint: "https://oss-cn-beijing.aliyuncs.com",
		},
		{
			name: "keeps the caller endpoint without embedding the bucket",
			req: CreateStorageClassRequest{
				Name: "oss", Clusters: []string{"rlark-cluster-a"}, Provider: "Alibaba",
				Endpoint: "https://oss-cn-beijing.aliyuncs.com", Region: "cn-beijing",
				AccessKeySecret: "secret", Bucket: "bucket",
			},
			wantProvider: "Alibaba", wantEndpoint: "https://oss-cn-beijing.aliyuncs.com",
		},
		{
			name: "rewrites a legacy display name to the canonical value",
			req: CreateStorageClassRequest{
				Name: "oss", Clusters: []string{"rlark-cluster-a"}, Provider: "Aliyun OSS",
				Region: "cn-beijing", AccessKeySecret: "secret", Bucket: "bucket",
			},
			wantProvider: "Alibaba", wantEndpoint: "https://oss-cn-beijing.aliyuncs.com",
		},
		{
			name: "forces virtual-hosted addressing for providers that reject path style",
			req: CreateStorageClassRequest{
				Name: "oss", Clusters: []string{"rlark-cluster-a"}, Provider: "Alibaba",
				Region: "cn-beijing", PathStyle: true, AccessKeySecret: "secret", Bucket: "bucket",
			},
			wantProvider: "Alibaba", wantEndpoint: "https://oss-cn-beijing.aliyuncs.com",
		},
		{
			name: "keeps path style for generic S3 backends",
			req: CreateStorageClassRequest{
				Name: "minio", Clusters: []string{"rlark-cluster-a"}, Provider: "Minio",
				Endpoint: "minio.example.com", PathStyle: true, AccessKeySecret: "secret", Bucket: "bucket",
			},
			wantProvider: "Minio", wantEndpoint: "https://minio.example.com", wantPathStyle: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stored, err := normalizeStorageClass(tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.wantProvider, stored.Provider)
			assert.Equal(t, tc.wantEndpoint, stored.Endpoint)
			assert.Equal(t, tc.wantPathStyle, stored.PathStyle)
			assert.NotContains(t, stored.Endpoint, tc.req.Bucket+".")
		})
	}
}

func TestNormalizeStorageClassRejectsInvalidProviderAndEndpoint(t *testing.T) {
	cases := []struct {
		name string
		req  CreateStorageClassRequest
	}{
		{
			name: "unknown provider",
			req: CreateStorageClassRequest{
				Name: "oss", Clusters: []string{"rlark-cluster-a"}, Provider: "Alibaba Cloud OSS",
				Endpoint: "https://oss-cn-beijing.aliyuncs.com", AccessKeySecret: "secret", Bucket: "bucket",
			},
		},
		{
			name: "empty provider",
			req: CreateStorageClassRequest{
				Name: "oss", Clusters: []string{"rlark-cluster-a"},
				Endpoint: "https://oss-cn-beijing.aliyuncs.com", AccessKeySecret: "secret", Bucket: "bucket",
			},
		},
		{
			name: "missing endpoint and region",
			req: CreateStorageClassRequest{
				Name: "oss", Clusters: []string{"rlark-cluster-a"}, Provider: "Alibaba",
				AccessKeySecret: "secret", Bucket: "bucket",
			},
		},
		{
			name: "missing endpoint for a generic backend",
			req: CreateStorageClassRequest{
				Name: "minio", Clusters: []string{"rlark-cluster-a"}, Provider: "Minio",
				AccessKeySecret: "secret", Bucket: "bucket",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeStorageClass(tc.req)
			require.Error(t, err)
		})
	}
}

func TestGetStorageClientDoesNotRequireConfiguredCluster(t *testing.T) {
	stored, err := normalizeStorageClass(CreateStorageClassRequest{
		Name: "object-storage", Clusters: []string{"rlark-cluster-a"}, Provider: "Minio",
		Endpoint: "https://minio.example.com", AccessKeyId: "access", AccessKeySecret: "secret", Bucket: "bucket",
	})
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", stored)
	require.NoError(t, err)
	gateway := &Gateway{
		config:         Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient:      fake.NewSimpleClientset(secret),
		storageClients: map[string]*storage.Client{},
	}

	client, err := gateway.getStorageClient(context.Background(), "object-storage")
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func TestStorageClassFromSecretCanonicalizesLegacyRecords(t *testing.T) {
	// 旧版 UI 提交的是展示名, 并且允许把 path_style 置为 true, 这里验证读取
	// 路径会把两者修正为与新写入记录一致的形式.
	stored := storedStorageClass{
		Version: storageClassVersion, Name: "object-storage", Clusters: []string{"rlark-cluster-a"},
		Provider: "Aliyun OSS", Endpoint: "https://oss-cn-beijing.aliyuncs.com", Region: "cn-beijing",
		Bucket: "bucket", PathStyle: true,
	}
	storedJSON, err := json.Marshal(stored)
	require.NoError(t, err)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      storageClassReplicationName("sc-0123456789abcdef"),
			Namespace: "management",
			Labels:    map[string]string{storageClassReplicationLabel: "true"},
		},
		Type: distribution.ReplicationSecretType,
		Data: map[string][]byte{"storageclass": storedJSON},
	}

	id, decoded, err := storageClassFromSecret(secret)
	require.NoError(t, err)
	assert.Equal(t, "sc-0123456789abcdef", id)
	assert.Equal(t, "Alibaba", decoded.Provider)
	assert.False(t, decoded.PathStyle)
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
