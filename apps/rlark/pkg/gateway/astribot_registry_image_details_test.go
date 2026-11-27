package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/lru"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
)

type fakeRegistryImage struct {
	config   []byte
	manifest []byte
}

func newFakeRegistryImage(t *testing.T, created, arch string, layers ...int64) fakeRegistryImage {
	t.Helper()
	config := []byte(fmt.Sprintf(`{"created":%q,"os":"linux","architecture":%q}`, created, arch))
	layerList := make([]map[string]any, 0, len(layers))
	for i, size := range layers {
		layerList = append(layerList, map[string]any{"mediaType": "application/vnd.oci.image.layer.v1.tar+gzip", "digest": "sha256:" + strings.Repeat(fmt.Sprint(i), 64), "size": size})
	}
	manifest, err := json.Marshal(map[string]any{
		"schemaVersion": 2, "mediaType": mediaTypeOCIManifest,
		"config": map[string]any{"mediaType": "application/vnd.oci.image.config.v1+json", "digest": contentDigest(config), "size": len(config)},
		"layers": layerList,
	})
	require.NoError(t, err)
	return fakeRegistryImage{config: config, manifest: manifest}
}

type registryDetailsFixture struct {
	router       *gin.Engine
	token        string
	configFetch  atomic.Int32
	amd64, arm64 fakeRegistryImage
}

func newRegistryDetailsFixture(t *testing.T, tags string) *registryDetailsFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &registryDetailsFixture{
		amd64: newFakeRegistryImage(t, "2026-11-05T11:14:00Z", "amd64", 100, 200),
		arm64: newFakeRegistryImage(t, "2026-11-05T11:15:00Z", "arm64", 300),
	}
	single := newFakeRegistryImage(t, "2026-11-11T07:12:00Z", "amd64", 1000)
	index, err := json.Marshal(map[string]any{
		"schemaVersion": 2, "mediaType": mediaTypeOCIIndex,
		"manifests": []map[string]any{
			{"mediaType": mediaTypeOCIManifest, "digest": contentDigest(f.arm64.manifest), "size": len(f.arm64.manifest), "platform": map[string]string{"os": "linux", "architecture": "arm64"}},
			{"mediaType": mediaTypeOCIManifest, "digest": contentDigest(f.amd64.manifest), "size": len(f.amd64.manifest), "platform": map[string]string{"os": "linux", "architecture": "amd64"}},
			{"mediaType": mediaTypeOCIManifest, "digest": "sha256:" + strings.Repeat("e", 64), "size": 10, "platform": map[string]string{"os": "unknown", "architecture": "unknown"}, "annotations": map[string]string{attestationReferenceType: attestationManifestType}},
		},
	})
	require.NoError(t, err)
	badConfig := newFakeRegistryImage(t, "2026-11-01T00:00:00Z", "amd64", 1)
	blobs := map[string][]byte{}
	for _, image := range []fakeRegistryImage{f.amd64, f.arm64, single} {
		blobs[contentDigest(image.config)] = image.config
	}
	blobs[contentDigest(badConfig.config)] = []byte(`{"created":"tampered"}`)
	manifests := map[string][]byte{
		"v1": single.manifest, "multi": index, "bad": badConfig.manifest,
		contentDigest(f.amd64.manifest): f.amd64.manifest, contentDigest(f.arm64.manifest): f.arm64.manifest,
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "robot" || password != "private-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/v2/rlark/agent/tags/list":
			_, _ = w.Write([]byte(tags))
		case strings.HasPrefix(r.URL.Path, "/v2/rlark/agent/manifests/"):
			reference := strings.TrimPrefix(r.URL.Path, "/v2/rlark/agent/manifests/")
			switch reference {
			case "big":
				_, _ = w.Write([]byte(strings.Repeat(" ", registryManifestLimit+1)))
				return
			case "moved":
				http.Redirect(w, r, "/v2/rlark/agent/manifests/v1", http.StatusFound)
				return
			case "lying":
				w.Header().Set("Docker-Content-Digest", "sha256:"+strings.Repeat("f", 64))
				_, _ = w.Write(single.manifest)
				return
			}
			body, ok := manifests[reference]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Docker-Content-Digest", contentDigest(body))
			_, _ = w.Write(body)
		case strings.HasPrefix(r.URL.Path, "/v2/rlark/agent/blobs/"):
			f.configFetch.Add(1)
			body, ok := blobs[strings.TrimPrefix(r.URL.Path, "/v2/rlark/agent/blobs/")]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(body)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	address := strings.TrimPrefix(server.URL, "https://")
	credential, err := normalizeImageRegistryCredential("Test", address, "robot", "private-secret", clusterSelection{Mode: clusterSelectionNone})
	require.NoError(t, err)
	secret, err := buildImageRegistryReplicationSecret("management", "ir-0123456789abcdef", credential)
	require.NoError(t, err)
	client := server.Client()
	// Mirror the production client, which never follows registry redirects.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	gateway := &Gateway{
		config:    Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}, AstribotConfig: AstribotConfig{RegistryBrowseAllowedHosts: []string{address}}},
		rawClient: fake.NewSimpleClientset(secret), astribotState: astribotState{registryHTTPClient: client, registryImages: lru.New(16)},
		jwtSigningKey: []byte("01234567890123456789012345678901"),
	}
	f.router = gin.New()
	gateway.RegisterRoutes(f.router)
	f.token, _, err = gateway.issueJWT("admin", "admin")
	require.NoError(t, err)
	return f
}

func (f *registryDetailsFixture) call(t *testing.T, ctx context.Context, query string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/image-registries/ir-0123456789abcdef/tag-details?"+query, nil).WithContext(ctx)
	request.Header.Set("Authorization", "Bearer "+f.token)
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	require.NotContains(t, recorder.Body.String(), "private-secret")
	return recorder
}

func TestRegistryTagDetails(t *testing.T) {
	f := newRegistryDetailsFixture(t, `{"name":"rlark/agent","tags":["v1","multi","bad","big","moved","lying","missing"]}`)
	recorder := f.call(t, context.Background(), "repository=rlark%2Fagent")
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	var response struct {
		Items []registryTagDetail `json:"items"`
		Next  string              `json:"next"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	byTag := map[string]registryTagDetail{}
	for _, item := range response.Items {
		byTag[item.Tag] = item
	}
	require.Len(t, response.Items, 7)
	require.Equal(t, "v1", response.Items[0].Tag, "order follows the registry tag list")

	single := byTag["v1"]
	require.Empty(t, single.Error)
	require.Equal(t, "2026-11-11T07:12:00Z", single.Created)
	require.Equal(t, []string{"linux/amd64"}, single.Platforms)
	require.Regexp(t, registryDigestPattern, single.Digest)

	multi := byTag["multi"]
	require.Empty(t, multi.Error)
	require.Equal(t, []string{"linux/arm64", "linux/amd64"}, multi.Platforms, "attestation entries are skipped")
	require.Equal(t, "2026-11-05T11:14:00Z", multi.Created, "linux/amd64 is described")
	require.Equal(t, int64(len(f.amd64.config))+300, multi.Size)

	for _, tag := range []string{"bad", "big", "moved", "lying", "missing"} {
		require.Equal(t, "unavailable", byTag[tag].Error, tag)
		require.Empty(t, byTag[tag].Created, tag)
	}

	fetched := f.configFetch.Load()
	require.Equal(t, http.StatusOK, f.call(t, context.Background(), "repository=rlark%2Fagent").Code)
	require.Equal(t, fetched+1, f.configFetch.Load(), "only the failed config is fetched again; cached digests skip the config")
}

func TestRegistryTagDetailsRejectsInvalidInputAndCancellation(t *testing.T) {
	f := newRegistryDetailsFixture(t, `{"name":"rlark/agent","tags":["v1"]}`)
	require.Equal(t, http.StatusBadRequest, f.call(t, context.Background(), "repository=..%2Ffoo").Code)
	require.Equal(t, http.StatusBadRequest, f.call(t, context.Background(), "repository=rlark%2Fagent&last=%2Fbad").Code)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.Equal(t, http.StatusBadGateway, f.call(t, ctx, "repository=rlark%2Fagent").Code)

	invalid := newRegistryDetailsFixture(t, `{"name":"rlark/agent","tags":["../escape"]}`)
	require.Equal(t, http.StatusBadGateway, invalid.call(t, context.Background(), "repository=rlark%2Fagent").Code)
}

func TestIndexPlatformsPrefersLinuxAMD64(t *testing.T) {
	entry := func(os, arch, digit string) registryDescriptor {
		d := registryDescriptor{Digest: "sha256:" + strings.Repeat(digit, 64)}
		d.Platform = &struct {
			OS           string `json:"os"`
			Architecture string `json:"architecture"`
			Variant      string `json:"variant"`
		}{OS: os, Architecture: arch}
		return d
	}
	platforms, chosen := indexPlatforms([]registryDescriptor{entry("linux", "arm64", "1"), entry("linux", "amd64", "2"), entry("windows", "amd64", "3")})
	require.Equal(t, []string{"linux/arm64", "linux/amd64", "windows/amd64"}, platforms)
	require.Equal(t, "sha256:"+strings.Repeat("2", 64), chosen.Digest)
	_, none := indexPlatforms([]registryDescriptor{entry("unknown", "unknown", "4")})
	require.Nil(t, none)
}
