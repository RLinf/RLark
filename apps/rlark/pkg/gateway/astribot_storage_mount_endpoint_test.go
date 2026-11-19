package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

func TestStorageMountEndpointLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := validateStorageClassCredentials
	var validated storedStorageClass
	validateStorageClassCredentials = func(_ context.Context, stored storedStorageClass) error {
		validated = stored
		return nil
	}
	t.Cleanup(func() { validateStorageClassCredentials = original })
	g := &Gateway{config: Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}}, rawClient: fake.NewSimpleClientset()}
	router := gin.New()
	router.POST("/storage", g.createStorageClass)
	router.PUT("/storage/:name", g.updateStorageClass)
	router.GET("/storage", g.listStorageClass)
	payload := map[string]any{"name": "cloud", "clusters": []string{"cloud"}, "provider": "Other", "endpoint": "https://public.example.com", "mount_endpoint": "internal.example.com/", "access_key_id": "access", "access_key_secret": "secret", "bucket": "bucket", "prefix": "ks3-regular", "vfs_cache_mode": "off", "mount_profile": "archive", "mount_options": []string{"s3-chunk-size=32Mi", "s3-upload-concurrency=4"}}
	response := performJSONRequest(t, router, http.MethodPost, "/storage", payload)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "https://public.example.com", validated.Endpoint)
	assert.Equal(t, "https://internal.example.com", validated.MountEndpoint)
	for _, mount := range []string{"https://internal.example.com", ""} {
		if mount == "" {
			payload["mount_endpoint"] = ""
			payload["access_key_id"] = ""
			payload["access_key_secret"] = ""
			delete(payload, "vfs_cache_mode")
			response = performJSONRequest(t, router, http.MethodPut, "/storage/cloud", payload)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		}
		secret, err := getStorageClassSecretByName(context.Background(), g.rawClient, "management", "cloud")
		require.NoError(t, err)
		_, stored, err := storageClassFromSecret(secret)
		require.NoError(t, err)
		assert.Equal(t, mount, stored.MountEndpoint)
		assert.Equal(t, "ks3-regular", stored.Prefix)
		assert.Equal(t, "off", stored.VFSCacheMode)
		assert.Equal(t, "archive", stored.MountProfile)
		assert.Equal(t, []string{"s3-chunk-size=32Mi", "s3-upload-concurrency=4"}, stored.MountOptions)
		assert.Equal(t, "secret", stored.AccessKey)
		replication, err := distribution.Parse(secret.Data)
		require.NoError(t, err)
		data := nestedData(t, replication.Objects[0])
		delivery, err := distribution.Parse(map[string][]byte{"config": decodeStorageManifestData(t, data["config"]), "manifests": decodeStorageManifestData(t, data["manifests"])})
		require.NoError(t, err)
		effective := mount
		if effective == "" {
			effective = stored.Endpoint
		}
		for _, object := range delivery.Objects {
			if object.GetKind() == "Secret" {
				config := nestedString(t, object.Object, "stringData", "configData")
				assert.Contains(t, config, "endpoint = "+effective+"\n")
				if mount != "" {
					assert.NotContains(t, config, stored.Endpoint)
				}
			}
		}
		response = performJSONRequest(t, router, http.MethodGet, "/storage", nil)
		require.Equal(t, http.StatusOK, response.Code)
		var body struct {
			Data map[string]StorageClassData `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		assert.Equal(t, mount, body.Data["cloud"].MountEndpoint)
		assert.Equal(t, "ks3-regular", body.Data["cloud"].Prefix)
		assert.Equal(t, "off", body.Data["cloud"].VFSCacheMode)
		assert.Equal(t, "archive", body.Data["cloud"].MountProfile)
		assert.Contains(t, body.Data["cloud"].EffectiveMountOptions, "dir-cache-time=5s")
		assert.Equal(t, "32Mi", body.Data["cloud"].EffectiveBackendOptions["chunk_size"])
		assert.Equal(t, "https://public.example.com", body.Data["cloud"].Endpoint)
		assert.NotContains(t, response.Body.String(), "\"secret\"")
	}
	secrets, err := g.rawClient.CoreV1().Secrets("management").List(context.Background(), metav1.ListOptions{})
	require.NoError(t, err)
	assert.Len(t, secrets.Items, 1)
}

func TestNormalizeMountEndpointRejectsInvalidAndSMB(t *testing.T) {
	for _, endpoint := range []string{"ftp://internal.example.com", "https://", "https://internal.example.com\nkey=value"} {
		_, err := normalizeStorageClass(CreateStorageClassRequest{Name: "cloud", Clusters: []string{"cloud"}, Provider: "Other", Endpoint: "https://public.example.com", MountEndpoint: endpoint, AccessKeySecret: "secret", Bucket: "bucket"})
		require.ErrorContains(t, err, "mount_endpoint")
	}
	_, err := normalizeStorageClass(CreateStorageClassRequest{Name: "nas", Clusters: []string{"cloud"}, Type: "smb", Host: "nas.example.com", Share: "share", Username: "user", Password: "password", MountEndpoint: "https://internal.example.com"})
	require.ErrorContains(t, err, "mount_endpoint is only supported for S3")
}
