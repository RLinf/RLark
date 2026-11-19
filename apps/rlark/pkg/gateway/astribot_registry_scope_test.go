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
	"sigs.k8s.io/yaml"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
)

func scopeGateway(t *testing.T) (*Gateway, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	gateway := &Gateway{
		config:        Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}},
		rawClient:     fake.NewSimpleClientset(),
		jwtSigningKey: []byte("01234567890123456789012345678901"),
	}
	token, _, err := gateway.issueJWT("admin", "admin")
	require.NoError(t, err)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Request.Header.Set("Authorization", "Bearer "+token) })
	gateway.RegisterRoutes(router)
	return gateway, router
}

// A new cluster's installer is given the credentials of the registries whose selection takes the cluster in: the
// agent's own image is pulled with them before the agent delivers them.
func TestANewClusterIsGivenItsImageRegistryCredentials(t *testing.T) {
	gateway, router := scopeGateway(t)
	ids := map[string]string{}
	for name, selection := range map[string]map[string]any{
		"everywhere": {"mode": "All", "clusters": []string{}},
		"elsewhere":  {"mode": "Selected", "clusters": []string{"robot-s1u-01"}},
		"here":       {"mode": "Selected", "clusters": []string{"baidu-gpu", "robot-s1u-01"}},
		"nowhere":    {"mode": "None", "clusters": []string{}},
	} {
		created := performJSONRequest(t, router, http.MethodPost, "/api/v1/image-registries", map[string]any{
			"name": name, "registry": name + ".example.com:5443", "username": "site", "password": "pw-" + name, "clusterSelection": selection,
		})
		require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
		var item imageRegistryItem
		require.NoError(t, json.Unmarshal(created.Body.Bytes(), &item))
		ids[name] = item.ID
	}
	given, err := gateway.imagePullSecretsOf(context.Background(), "baidu-gpu")
	require.NoError(t, err)
	names := map[string]string{}
	for _, secret := range given {
		names[secret.Name] = secret.DockerConfigJSON
	}
	assert.Len(t, names, 2)
	assert.Contains(t, names[ids["everywhere"]], "everywhere.example.com:5443")
	assert.Contains(t, names[ids["here"]], "here.example.com:5443")

	// The delivery makes the same credential under the entry's own name; the agents refuse adoption and force (their
	// delivery policy), so the installer's copy has a name of its own (rlarkadm, <id>-install).
	replication, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), imageRegistryReplicationName(ids["here"]), metav1.GetOptions{})
	require.NoError(t, err)
	var deliveries []corev1.Secret
	require.NoError(t, yaml.Unmarshal(replication.Data["manifests"], &deliveries))
	var config distribution.Config
	require.NoError(t, yaml.Unmarshal(deliveries[0].Data["config"], &config))
	assert.False(t, config.Apply.AdoptExisting)
	assert.Equal(t, "Fail", config.Apply.ConflictPolicy)
	var delivered []corev1.Secret
	require.NoError(t, yaml.Unmarshal(deliveries[0].Data["manifests"], &delivered))
	assert.Equal(t, ids["here"], delivered[0].Name)
	assert.Equal(t, names[ids["here"]], string(delivered[0].Data[corev1.DockerConfigJsonKey]))
}

// A secret is given to another cluster without its values being handed over again.
func TestASecretGoesToAnotherClusterWithTheValuesItHas(t *testing.T) {
	gateway, router := scopeGateway(t)
	created := performJSONRequest(t, router, http.MethodPost, "/api/v1/secrets", map[string]any{
		"name": "site-registry", "description": "the cache's", "clusters": []string{"rlark-ksyun-gpu"}, "data": map[string]string{"tls.key": "k", "htpasswd": "h"},
	})
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	updated := performJSONRequest(t, router, http.MethodPut, "/api/v1/secrets/site-registry", map[string]any{"clusters": []string{"rlark-ksyun-gpu", "rlark-baidu-gpu"}})
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	replication, err := gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), jobSecretReplicationName("site-registry"), metav1.GetOptions{})
	require.NoError(t, err)
	values, err := jobSecretValues(replication)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"tls.key": "k", "htpasswd": "h"}, values)
	var config distribution.Config
	require.NoError(t, yaml.Unmarshal(replication.Data["config"], &config))
	assert.Equal(t, []string{"rlark-baidu-gpu", "rlark-ksyun-gpu"}, config.Targets.Namespaces.Include)
	var stored storedJobSecret
	require.NoError(t, json.Unmarshal(replication.Data["secret"], &stored))
	assert.Equal(t, "the cache's", stored.Description)
}
