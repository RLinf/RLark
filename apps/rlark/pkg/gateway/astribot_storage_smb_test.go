package gateway

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"

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

func smbTestRequest() CreateStorageClassRequest {
	return CreateStorageClassRequest{
		Name: "nas-post-train", Description: "local nas", Clusters: []string{"rlark-cluster-a"}, Type: "smb",
		Host: "10.11.5.12", Share: "post-train_group", SubPath: "/RL/", Username: "user", Password: "p@ss=word", Domain: "ad.example.com",
	}
}

// smbDeliveryObjects unwraps the replication -> delivery -> manifests layers.
func smbDeliveryObjects(t *testing.T, secret *v1.Secret) (*unstructured.Unstructured, *unstructured.Unstructured) {
	t.Helper()
	replication, err := distribution.Parse(secret.Data)
	require.NoError(t, err)
	// The Delivery, then the storage's View.
	require.Len(t, replication.Objects, 2)
	require.Equal(t, "ConfigMap", replication.Objects[1].GetKind())
	deliveryData := nestedData(t, replication.Objects[0])
	delivery, err := distribution.Parse(map[string][]byte{
		"config":    decodeStorageManifestData(t, deliveryData["config"]),
		"manifests": decodeStorageManifestData(t, deliveryData["manifests"]),
	})
	require.NoError(t, err)
	// The storage's own profile comes first: its Secret, then its StorageClass.
	require.Len(t, delivery.Objects, 4)
	require.Equal(t, "Secret", delivery.Objects[0].GetKind())
	require.Equal(t, "StorageClass", delivery.Objects[1].GetKind())
	require.Equal(t, "sc-0123456789abcdef", delivery.Objects[1].GetName())
	return delivery.Objects[0], delivery.Objects[1]
}

func newSMBTestGateway(t *testing.T, objects ...*v1.Secret) *Gateway {
	t.Helper()
	gin.SetMode(gin.TestMode)
	clientset := fake.NewSimpleClientset()
	for _, object := range objects {
		_, err := clientset.CoreV1().Secrets(object.Namespace).Create(context.Background(), object, metav1.CreateOptions{})
		require.NoError(t, err)
	}
	return &Gateway{
		config:         Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient:      clientset,
		storageClients: map[string]*storage.Client{},
	}
}

func stubStorageValidation(t *testing.T) *storedStorageClass {
	t.Helper()
	original := validateStorageClassCredentials
	var validated storedStorageClass
	validateStorageClassCredentials = func(_ context.Context, stored storedStorageClass) error {
		validated = stored
		return nil
	}
	t.Cleanup(func() { validateStorageClassCredentials = original })
	return &validated
}

func TestBuildSMBStorageClassRendersRcloneSMBRemote(t *testing.T) {
	stored, err := normalizeStorageClass(smbTestRequest())
	require.NoError(t, err)
	assert.Equal(t, storageTypeSMB, stored.Type)
	assert.Equal(t, "RL", stored.SubPath)
	assert.Empty(t, stored.Provider)
	assert.Empty(t, stored.Bucket)

	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", stored)
	require.NoError(t, err)
	storageSecret, storageClass := smbDeliveryObjects(t, secret)

	stringData, found, err := unstructured.NestedStringMap(storageSecret.Object, "stringData")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "smb", stringData["remote"])
	assert.Equal(t, "post-train_group/RL", stringData["remotePath"])

	config := map[string]string{}
	for _, line := range strings.Split(stringData["configData"], "\n") {
		if key, value, ok := strings.Cut(line, " = "); ok {
			config[key] = value
		}
	}
	assert.Contains(t, stringData["configData"], "[smb]")
	assert.Equal(t, "smb", config["type"])
	assert.Equal(t, "10.11.5.12", config["host"])
	assert.Equal(t, "user", config["user"])
	assert.Equal(t, "ad.example.com", config["domain"])
	assert.NotContains(t, stringData["configData"], "p@ss=word", "password must be obscured")
	assert.NotEmpty(t, config["pass"])

	assert.Equal(t, "rclone.csi.veloxpack.io", nestedString(t, storageClass.Object, "provisioner"))
	parameters, _, err := unstructured.NestedStringMap(storageClass.Object, "parameters")
	require.NoError(t, err)
	assert.Equal(t, "smb", parameters["remote"])
	assert.Equal(t, "post-train_group/RL", parameters["remotePath"])
	mountOptions, _, err := unstructured.NestedStringSlice(storageClass.Object, "mountOptions")
	require.NoError(t, err)
	assert.NotContains(t, mountOptions, "fast-list")
}

func TestBuildSMBStorageClassWithoutSubPathOrDomain(t *testing.T) {
	req := smbTestRequest()
	req.SubPath, req.Domain = "", ""
	stored, err := normalizeStorageClass(req)
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", stored)
	require.NoError(t, err)
	storageSecret, _ := smbDeliveryObjects(t, secret)
	stringData, _, err := unstructured.NestedStringMap(storageSecret.Object, "stringData")
	require.NoError(t, err)
	assert.Equal(t, "post-train_group", stringData["remotePath"])
	assert.NotContains(t, stringData["configData"], "domain")
}

func TestNormalizeSMBStorageClassValidation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*CreateStorageClassRequest)
		fields []string
		err    string
	}{
		{name: "required fields", mutate: func(r *CreateStorageClassRequest) { r.Host, r.Share, r.Username, r.Password = "", "", "", "" },
			fields: []string{"host", "share", "username", "password"}},
		{name: "scheme stripped", mutate: func(r *CreateStorageClassRequest) { r.Host = "//10.11.5.12" }},
		{name: "host with port", mutate: func(r *CreateStorageClassRequest) { r.Host = "10.11.5.12:445" }, err: "invalid smb host"},
		{name: "host with path", mutate: func(r *CreateStorageClassRequest) { r.Host = "10.11.5.12/share" }, err: "invalid smb host"},
		{name: "nested share", mutate: func(r *CreateStorageClassRequest) { r.Share = "a/b" }, err: "single share name"},
		{name: "traversal", mutate: func(r *CreateStorageClassRequest) { r.SubPath = "RL/../../etc" }, err: "invalid sub_path"},
		{name: "config injection", mutate: func(r *CreateStorageClassRequest) { r.Username = "user\nhost = evil" }, err: "line breaks"},
		{name: "unknown type", mutate: func(r *CreateStorageClassRequest) { r.Type = "nfs" }, err: "unsupported storage type"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := smbTestRequest()
			test.mutate(&req)
			_, err := normalizeStorageClass(req)
			switch {
			case test.fields != nil:
				var validationErr *storageClassValidationError
				require.ErrorAs(t, err, &validationErr)
				assert.Equal(t, test.fields, validationErr.fields)
			case test.err != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), test.err)
			default:
				require.NoError(t, err)
			}
		})
	}
}

func TestListSMBStorageClassOmitsPassword(t *testing.T) {
	stored, err := normalizeStorageClass(smbTestRequest())
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", stored)
	require.NoError(t, err)
	gateway := newSMBTestGateway(t, secret)
	router := gin.New()
	router.GET("/storage", gateway.listStorageClass)

	response := performJSONRequest(t, router, http.MethodGet, "/storage", nil)
	require.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, response.Body.String(), "p@ss=word")
	assert.NotContains(t, response.Body.String(), `"password"`)
	var body struct {
		Data map[string]StorageClassData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	item := body.Data["nas-post-train"]
	assert.Equal(t, "smb", item.Type)
	assert.Equal(t, "10.11.5.12", item.Host)
	assert.Equal(t, "post-train_group", item.Share)
	assert.Equal(t, "RL", item.SubPath)
	assert.Equal(t, "user", item.Username)
	assert.Equal(t, "ad.example.com", item.Domain)
	assert.Empty(t, item.AccessKeyId)
}

func TestCreateSMBStorageClass(t *testing.T) {
	validated := stubStorageValidation(t)
	gateway := newSMBTestGateway(t)
	router := gin.New()
	router.POST("/storage", gateway.createStorageClass)

	response := performJSONRequest(t, router, http.MethodPost, "/storage", map[string]any{
		"name": "nas-post-train", "clusters": []string{"rlark-cluster-a"}, "type": "smb",
		"host": "10.11.5.12", "share": "post-train_group", "sub_path": "RL", "username": "user", "password": "secret",
	})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, storageTypeSMB, validated.Type)
	assert.Equal(t, "10.11.5.12", validated.Host)

	secret, err := getStorageClassSecretByName(context.Background(), gateway.rawClient, "management", "nas-post-train")
	require.NoError(t, err)
	_, stored, err := storageClassFromSecret(secret)
	require.NoError(t, err)
	assert.Equal(t, "secret", stored.Password)
}

func TestUpdateSMBStorageClassRetainsStoredPassword(t *testing.T) {
	validated := stubStorageValidation(t)
	initial, err := normalizeStorageClass(smbTestRequest())
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", initial)
	require.NoError(t, err)
	gateway := newSMBTestGateway(t, secret)
	router := gin.New()
	router.PUT("/storage/:name", gateway.updateStorageClass)

	response := performJSONRequest(t, router, http.MethodPut, "/storage/nas-post-train", map[string]any{
		"name": "nas-post-train", "description": "updated", "clusters": []string{"rlark-cluster-a"}, "type": "smb",
		"host": "10.11.5.12", "share": "post-train_group", "sub_path": "RL/user", "username": "user", "password": "",
	})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, "p@ss=word", validated.Password)

	updated, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), secret.Name, metav1.GetOptions{})
	require.NoError(t, err)
	_, stored, err := storageClassFromSecret(updated)
	require.NoError(t, err)
	assert.Equal(t, "updated", stored.Description)
	assert.Equal(t, "RL/user", stored.SubPath)
	assert.Equal(t, "p@ss=word", stored.Password)
}

func TestUpdateStorageClassRejectsTypeChange(t *testing.T) {
	stubStorageValidation(t)
	initial, err := normalizeStorageClass(smbTestRequest())
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", initial)
	require.NoError(t, err)
	gateway := newSMBTestGateway(t, secret)
	router := gin.New()
	router.PUT("/storage/:name", gateway.updateStorageClass)

	response := performJSONRequest(t, router, http.MethodPut, "/storage/nas-post-train", map[string]any{
		"name": "nas-post-train", "clusters": []string{"rlark-cluster-a"}, "provider": "Minio",
		"endpoint": "https://minio.example.com", "access_key_id": "", "access_key_secret": "", "bucket": "bucket",
	})
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "type cannot be changed")
}

func TestSMBStorageClassFileBrowsingIsUnsupported(t *testing.T) {
	stored, err := normalizeStorageClass(smbTestRequest())
	require.NoError(t, err)
	secret, err := buildStorageClassReplicationSecret("management", "sc-0123456789abcdef", stored)
	require.NoError(t, err)
	gateway := newSMBTestGateway(t, secret)
	router := gin.New()
	router.GET("/storage/:name/list", gateway.listStorageClassFiles)

	response := performJSONRequest(t, router, http.MethodGet, "/storage/nas-post-train/list", nil)
	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Contains(t, response.Body.String(), "does not support file browsing")
}

func TestStorageClassFromSecretDefaultsLegacyRecordsToS3(t *testing.T) {
	legacy := storedStorageClass{
		Version: storageClassVersion, Name: "object-storage", Clusters: []string{"rlark-cluster-a"},
		Provider: "Minio", Endpoint: "https://minio.example.com", Bucket: "bucket",
	}
	storedJSON, err := json.Marshal(legacy)
	require.NoError(t, err)
	assert.NotContains(t, string(storedJSON), `"type"`)
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: storageClassReplicationName("sc-0123456789abcdef"), Namespace: "management",
			Labels: map[string]string{storageClassReplicationLabel: "true"},
		},
		Type: distribution.ReplicationSecretType,
		Data: map[string][]byte{"storageclass": storedJSON},
	}
	_, decoded, err := storageClassFromSecret(secret)
	require.NoError(t, err)
	assert.Equal(t, storageTypeS3, decoded.Type)
}

func TestValidateSMBReachable(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	host, port, err := net.SplitHostPort(listener.Addr().String())
	require.NoError(t, err)

	original := smbDialPort
	smbDialPort = port
	t.Cleanup(func() { smbDialPort = original })
	assert.NoError(t, validateSMBReachable(context.Background(), host))

	require.NoError(t, listener.Close())
	assert.Error(t, validateSMBReachable(context.Background(), host))
}
