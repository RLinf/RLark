package gateway

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

func TestStorageMountProfiles(t *testing.T) {
	options, backend, cache, err := resolveS3MountOptions("archive", "", nil)
	require.NoError(t, err)
	require.Equal(t, "off", cache)
	require.Contains(t, options, "dir-cache-time=5s")
	require.Equal(t, map[string]string{"chunk_size": "32Mi", "upload_concurrency": "4"}, backend)
	require.Contains(t, options, "attr-timeout=1s")
	read, _, _, err := resolveS3MountOptions("read", "", []string{"low-level-retries=30"})
	require.NoError(t, err)
	require.Contains(t, read, "read-only=true")
	require.Contains(t, read, "low-level-retries=30")
	require.NotContains(t, read, "ignore-checksum=true")
	// Resolving one mount must not change another mount's defaults.
	defaults, _, _, err := resolveS3MountOptions("default", "", nil)
	require.NoError(t, err)
	require.NotContains(t, defaults, "read-only=true")
	require.Contains(t, defaults, "vfs-cache-mode=writes")
	for _, tc := range []struct {
		profile, cache string
		options        []string
	}{
		{"invalid", "", nil}, {"archive", "writes", nil}, {"read", "", []string{"read-only=false"}},
		{"default", "", []string{"ignore-checksum"}}, {"default", "", []string{"tpslimit=2"}},
		{"default", "", []string{"low-level-retries=0"}}, {"default", "", []string{"s3-chunk-size=4Mi"}},
		{"default", "", []string{"s3-chunk-size=6Gi"}}, {"default", "", []string{"dir-cache-time=-1s"}},
		{"default", "", []string{"transfers=0"}}, {"default", "", []string{"checkers=1", "checkers=2"}},
		{"default", "", []string{"config=/tmp/credentials"}}, {"default", "", []string{"read-only=true\nfoo=bar"}},
	} {
		_, _, _, err := resolveS3MountOptions(tc.profile, tc.cache, tc.options)
		require.Error(t, err, "%+v", tc)
	}
}

func TestStorageMountPolicyRenderBackendAndRoundTrip(t *testing.T) {
	options := []string{"s3-chunk-size=64Mi", "s3-upload-concurrency=2", "low-level-retries=30"}
	s, err := normalizeStorageClass(CreateStorageClassRequest{Name: "archive", Clusters: []string{"cloud"}, Provider: "Other", Endpoint: "https://s3.example.com", Bucket: "bucket", AccessKeySecret: "secret", MountProfile: "archive", MountOptions: &options})
	require.NoError(t, err)
	require.Equal(t, "off", s.VFSCacheMode)
	declaration, err := buildStorageClassReplicationSecret("default", "sc-0123456789abcdef", s)
	require.NoError(t, err)
	_, decoded, err := storageClassFromSecret(declaration)
	require.NoError(t, err)
	require.Equal(t, s, decoded)
	_, backend, _, err := resolveS3MountOptions(s.MountProfile, s.VFSCacheMode, s.MountOptions)
	require.NoError(t, err)
	require.Equal(t, "64Mi", backend["chunk_size"])
	require.Equal(t, "2", backend["upload_concurrency"])
}

func TestStorageClassRemoteNameIsStable(t *testing.T) {
	archive, err := normalizeStorageClass(CreateStorageClassRequest{
		Name: "archive", Clusters: []string{"cloud"}, Provider: "Other", Endpoint: "https://s3.example.com",
		Bucket: "bucket", AccessKeySecret: "archive-secret", MountProfile: "archive",
	})
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("default", "sc-archive", archive)
	require.NoError(t, err)
	manifest := parseStorageClassObjects(t, secret)
	config := nestedString(t, manifest[0].Object, "stringData", "configData")
	require.Contains(t, config, "[s3]")
	require.Contains(t, config, "chunk_size = 32Mi")
	require.Equal(t, "s3", nestedString(t, manifest[1].Object, "parameters", "remote"))
}

func parseStorageClassObjects(t *testing.T, secret *v1.Secret) []*unstructured.Unstructured {
	t.Helper()
	replication, err := distribution.Parse(secret.Data)
	require.NoError(t, err)
	deliveryData := nestedData(t, replication.Objects[0])
	decode := func(value string) []byte {
		data, err := base64.StdEncoding.DecodeString(value)
		require.NoError(t, err)
		return data
	}
	delivery, err := distribution.Parse(map[string][]byte{
		"config": decode(deliveryData["config"]), "manifests": decode(deliveryData["manifests"]),
	})
	require.NoError(t, err)
	return delivery.Objects
}

func TestStorageClassCredentialsFromExistingStorage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := validateStorageClassCredentials
	validateStorageClassCredentials = func(context.Context, storedStorageClass) error { return nil }
	t.Cleanup(func() { validateStorageClassCredentials = original })
	g := &Gateway{config: Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}}, rawClient: fake.NewSimpleClientset()}
	router := gin.New()
	router.POST("/storage", g.createStorageClass)
	response := performJSONRequest(t, router, http.MethodPost, "/storage", map[string]any{"name": "archive", "clusters": []string{"cloud"}, "provider": "Other", "endpoint": "https://public.example.com", "region": "cn", "access_key_id": "access", "access_key_secret": "secret", "bucket": "bucket", "mount_profile": "archive"})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	response = performJSONRequest(t, router, http.MethodPost, "/storage", map[string]any{"name": "read", "clusters": []string{"cloud"}, "credentials_from": "archive", "bucket": "bucket", "prefix": "data", "mount_profile": "read"})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	secret, err := getStorageClassSecretByName(context.Background(), g.rawClient, "management", "read")
	require.NoError(t, err)
	_, read, err := storageClassFromSecret(secret)
	require.NoError(t, err)
	require.Equal(t, []string{"https://public.example.com", "cn", "access", "secret", "data", "read"}, []string{read.Endpoint, read.Region, read.AccessKeyID, read.AccessKey, read.Prefix, read.MountProfile})
	response = performJSONRequest(t, router, http.MethodPost, "/storage", map[string]any{"name": "other", "clusters": []string{"cloud"}, "credentials_from": "missing", "bucket": "bucket"})
	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
}
