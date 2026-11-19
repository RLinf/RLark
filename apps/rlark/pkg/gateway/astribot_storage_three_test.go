package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/astribot/nodepath"
	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/rlinf/rlark/apps/rlark/pkg/gateway/storage"
)

func newStorageTestGateway(t *testing.T) (*Gateway, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	original := validateStorageClassCredentials
	validateStorageClassCredentials = func(context.Context, storedStorageClass) error { return nil }
	t.Cleanup(func() { validateStorageClassCredentials = original })
	g := &Gateway{config: Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}}, rawClient: fake.NewSimpleClientset(), storageClients: map[string]*storage.Client{}}
	router := gin.New()
	router.GET("/storage", g.listStorageClass)
	router.POST("/storage", g.createStorageClass)
	router.PUT("/storage/:name", g.updateStorageClass)
	router.DELETE("/storage/:name", g.deleteStorageClass)
	return g, router
}

func ks3Request() map[string]any {
	return map[string]any{
		"name": "ks3", "clusters": []string{"rlark-ksyun-gpu", "rlark-robot"}, "provider": "Other",
		"endpoint": "https://ks3-cn-ningxia.ksyuncs.com", "region": "cn-ningxia", "bucket": "stardust", "prefix": "ks3-regular/post_train",
		"access_key_id": "access", "access_key_secret": "secret", "mount_profile": "archive",
		"cluster_mount_endpoints": map[string]string{"rlark-ksyun-gpu": "https://ks3-cn-ningxia-internal.ksyuncs.com"},
	}
}

// replicationView returns the View a declaration delivers and its target clusters.
func replicationView(t *testing.T, secret *v1.Secret) (storageref.View, []string) {
	t.Helper()
	replication, err := distribution.Parse(secret.Data)
	require.NoError(t, err)
	last := replication.Objects[len(replication.Objects)-1]
	require.Equal(t, "ConfigMap", last.GetKind())
	data, _, err := unstructured.NestedStringMap(last.Object, "data")
	require.NoError(t, err)
	var view storageref.View
	require.NoError(t, json.Unmarshal([]byte(data[storageref.ViewKey]), &view))
	assert.Equal(t, "true", last.GetLabels()[storageref.ViewLabel])
	return view, replication.Config.Targets.Namespaces.Include
}

func endpointSecrets(t *testing.T, g *Gateway) []v1.Secret {
	t.Helper()
	list, err := g.rawClient.CoreV1().Secrets("management").List(context.Background(), metav1.ListOptions{LabelSelector: storageClassEndpointLabel})
	require.NoError(t, err)
	return list.Items
}

func TestStorageClusterEndpointGetsItsOwnDeclaration(t *testing.T) {
	g, router := newStorageTestGateway(t)
	response := performJSONRequest(t, router, http.MethodPost, "/storage", ks3Request())
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	primary, err := getStorageClassSecretByName(context.Background(), g.rawClient, "management", "ks3")
	require.NoError(t, err)
	id, _, err := storageClassFromSecret(primary)
	require.NoError(t, err)
	view, clusters := replicationView(t, primary)
	assert.Equal(t, []string{"rlark-robot"}, clusters, "the primary declaration reaches only clusters on the default endpoint")
	assert.Equal(t, storageref.View{Name: "ks3", Type: "s3", DefaultProfile: "archive", StorageClasses: map[string]string{
		"archive": id, "default": id + "-default", "read": id + "-read",
	}}, view)
	assert.Contains(t, string(parseStorageClassObjects(t, primary)[0].Object["stringData"].(map[string]any)["configData"].(string)), "endpoint = https://ks3-cn-ningxia.ksyuncs.com\n")

	parts := endpointSecrets(t, g)
	require.Len(t, parts, 1)
	assert.Equal(t, id+"-at-rlark-ksyun-gpu", parts[0].Name)
	assert.Equal(t, distribution.ReplicationSecretType, parts[0].Type)
	assert.Empty(t, parts[0].Labels[storageClassReplicationLabel], "an endpoint declaration is not listed as a storage")
	partView, partClusters := replicationView(t, &parts[0])
	assert.Equal(t, []string{"rlark-ksyun-gpu"}, partClusters)
	assert.Equal(t, view, partView)
	objects := parseStorageClassObjects(t, &parts[0])
	config := objects[0].Object["stringData"].(map[string]any)["configData"].(string)
	assert.Contains(t, config, "endpoint = https://ks3-cn-ningxia-internal.ksyuncs.com\n")
	assert.Contains(t, config, "chunk_size = 32Mi", "the archive class carries the archive backend options")

	// Dropping the cluster's own endpoint moves it back to the primary.
	update := ks3Request()
	update["cluster_mount_endpoints"] = map[string]string{}
	response = performJSONRequest(t, router, http.MethodPut, "/storage/ks3", update)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Empty(t, endpointSecrets(t, g))
	primary, err = getStorageClassSecretByName(context.Background(), g.rawClient, "management", "ks3")
	require.NoError(t, err)
	_, clusters = replicationView(t, primary)
	assert.Equal(t, []string{"rlark-ksyun-gpu", "rlark-robot"}, clusters)
}

func TestStorageUpdateWithoutClusterEndpointsKeepsThem(t *testing.T) {
	g, router := newStorageTestGateway(t)
	require.Equal(t, http.StatusOK, performJSONRequest(t, router, http.MethodPost, "/storage", ks3Request()).Code)
	update := ks3Request()
	delete(update, "cluster_mount_endpoints")
	delete(update, "access_key_secret")
	update["description"] = "team storage"
	response := performJSONRequest(t, router, http.MethodPut, "/storage/ks3", update)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Len(t, endpointSecrets(t, g), 1)
}

func TestStorageRenameKeepsIDAndRejectsTakenName(t *testing.T) {
	g, router := newStorageTestGateway(t)
	request := ks3Request()
	request["name"] = "fleet-ks3-post-train"
	require.Equal(t, http.StatusOK, performJSONRequest(t, router, http.MethodPost, "/storage", request).Code)
	other := ks3Request()
	other["name"] = "taken"
	require.Equal(t, http.StatusOK, performJSONRequest(t, router, http.MethodPost, "/storage", other).Code)
	before, err := getStorageClassSecretByName(context.Background(), g.rawClient, "management", "fleet-ks3-post-train")
	require.NoError(t, err)

	rename := ks3Request()
	rename["name"] = "taken"
	response := performJSONRequest(t, router, http.MethodPut, "/storage/fleet-ks3-post-train", rename)
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())

	// A rename needs no credentials: the stored ones are kept.
	rename = ks3Request()
	delete(rename, "access_key_id")
	delete(rename, "access_key_secret")
	response = performJSONRequest(t, router, http.MethodPut, "/storage/fleet-ks3-post-train", rename)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	after, err := getStorageClassSecretByName(context.Background(), g.rawClient, "management", "ks3")
	require.NoError(t, err)
	assert.Equal(t, before.Name, after.Name, "the storage keeps its id and StorageClasses")
	_, stored, err := storageClassFromSecret(after)
	require.NoError(t, err)
	assert.Equal(t, "secret", stored.AccessKey)
	view, _ := replicationView(t, after)
	assert.Equal(t, "ks3", view.Name)
	for _, part := range endpointSecrets(t, g) {
		if part.Labels[storageClassEndpointLabel] == before.Name[:len(before.Name)-len("-replication")] {
			partView, _ := replicationView(t, &part)
			assert.Equal(t, "ks3", partView.Name, "the endpoint declaration's View follows the rename")
		}
	}
	_, err = getStorageClassSecretByName(context.Background(), g.rawClient, "management", "fleet-ks3-post-train")
	assert.Error(t, err)
}

func TestStorageCreateRejectsTakenName(t *testing.T) {
	_, router := newStorageTestGateway(t)
	require.Equal(t, http.StatusOK, performJSONRequest(t, router, http.MethodPost, "/storage", ks3Request()).Code)
	response := performJSONRequest(t, router, http.MethodPost, "/storage", ks3Request())
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
}

func TestHostPathStorageDeliversNodeDirectoryStorageClasses(t *testing.T) {
	g, router := newStorageTestGateway(t)
	response := performJSONRequest(t, router, http.MethodPost, "/storage", map[string]any{
		"name": "kpfs", "type": "hostpath", "clusters": []string{"rlark-ksyun-gpu"}, "path": "/mnt/xc_kpfs/regular/post_train",
		"fs_type": "kingstorage",
	})
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	secret, err := getStorageClassSecretByName(context.Background(), g.rawClient, "management", "kpfs")
	require.NoError(t, err)
	id, _, err := storageClassFromSecret(secret)
	require.NoError(t, err)
	view, clusters := replicationView(t, secret)
	assert.Equal(t, []string{"rlark-ksyun-gpu"}, clusters)
	assert.Equal(t, storageref.View{Name: "kpfs", Type: "hostpath", DefaultProfile: "default",
		StorageClasses: map[string]string{"default": id, "read": id + "-read"},
		// The nodes the driver's node plugin labels as having the storage.
		NodeSelector: map[string]string{"storage.rlark.io/" + id: "true"}}, view)
	// Like any storage: one StorageClass per profile, here of the node
	// directory driver, bound where the Pod is scheduled, without a Secret.
	objects := parseStorageClassObjects(t, secret)
	require.Len(t, objects, 2)
	for i, profile := range []string{"default", "read"} {
		class := objects[i]
		assert.Equal(t, "StorageClass", class.GetKind())
		assert.Equal(t, profile, class.GetAnnotations()[annoStorageProfile])
		assert.Equal(t, nodepath.Name, nestedString(t, class.Object, "provisioner"))
		assert.Equal(t, "WaitForFirstConsumer", nestedString(t, class.Object, "volumeBindingMode"))
		assert.Equal(t, "/mnt/xc_kpfs/regular/post_train", nestedString(t, class.Object, "parameters", "root"))
		assert.Equal(t, "kingstorage", nestedString(t, class.Object, "parameters", "fsType"))
		assert.Equal(t, id, class.GetLabels()[nodepath.StorageIDLabel])
	}
	assert.Equal(t, "true", nestedString(t, objects[1].Object, "parameters", "readOnly"))

	response = performJSONRequest(t, router, http.MethodGet, "/storage", nil)
	require.Equal(t, http.StatusOK, response.Code)
	var listed struct {
		Data map[string]StorageClassData `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &listed))
	assert.Equal(t, "/mnt/xc_kpfs/regular/post_train", listed.Data["kpfs"].Path)
	assert.Equal(t, "kingstorage", listed.Data["kpfs"].FSType)
	assert.Equal(t, "hostpath", listed.Data["kpfs"].Type)
}

func TestHostPathStorageValidation(t *testing.T) {
	for name, request := range map[string]map[string]any{
		"relative path":   {"path": "mnt/kpfs"},
		"unclean path":    {"path": "/mnt/kpfs/../etc"},
		"root":            {"path": "/"},
		"no path":         {"path": ""},
		"bad fs type":     {"path": "/mnt/kpfs", "fs_type": "King Storage"},
		"object fields":   {"path": "/mnt/kpfs", "bucket": "b"},
		"mount profile":   {"path": "/mnt/kpfs", "mount_profile": "archive"},
		"cluster address": {"path": "/mnt/kpfs", "cluster_mount_endpoints": map[string]string{"c": "https://x"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, router := newStorageTestGateway(t)
			request["name"], request["type"], request["clusters"] = "kpfs", "hostpath", []string{"c"}
			response := performJSONRequest(t, router, http.MethodPost, "/storage", request)
			require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		})
	}
}

func TestStorageClusterEndpointMustNameAStorageCluster(t *testing.T) {
	_, router := newStorageTestGateway(t)
	request := ks3Request()
	request["cluster_mount_endpoints"] = map[string]string{"rlark-other": "https://x.example.com"}
	response := performJSONRequest(t, router, http.MethodPost, "/storage", request)
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "not one of the storage's clusters")
}

func TestStorageMountOptionsMustCombineWithEveryProfile(t *testing.T) {
	_, router := newStorageTestGateway(t)
	request := ks3Request()
	request["mount_profile"] = "default"
	request["mount_options"] = []string{"vfs-cache-mode=writes"}
	response := performJSONRequest(t, router, http.MethodPost, "/storage", request)
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "profile archive")
}

func TestStorageDeleteRemovesEndpointDeclarations(t *testing.T) {
	g, router := newStorageTestGateway(t)
	require.Equal(t, http.StatusOK, performJSONRequest(t, router, http.MethodPost, "/storage", ks3Request()).Code)
	require.Len(t, endpointSecrets(t, g), 1)
	response := performJSONRequest(t, router, http.MethodDelete, "/storage/ks3", nil)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Empty(t, endpointSecrets(t, g))
}
