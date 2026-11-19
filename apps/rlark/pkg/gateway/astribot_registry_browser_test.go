package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
)

func TestRegistryBrowse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var requests atomic.Int32
	var failing atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		user, password, ok := request.BasicAuth()
		if !ok || user != "robot" || password != "private-secret" {
			writer.WriteHeader(http.StatusUnauthorized)
			return
		}
		if failing.Load() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		switch request.URL.Path {
		case "/v2/_catalog":
			if request.URL.Query().Get("last") == "" {
				writer.Header().Set("Link", `</v2/_catalog?last=rlark%2Fagent&n=50>; rel="next"`)
				_, _ = writer.Write([]byte(`{"repositories":["rlark/agent"]}`))
			} else {
				_, _ = writer.Write([]byte(`{"repositories":["rlark/gateway"]}`))
			}
		case "/v2/rlark/agent/tags/list":
			_, _ = writer.Write([]byte(`{"name":"rlark/agent","tags":["v1"]}`))
		case "/v2/rlark/agent/manifests/v1":
			if request.Method != http.MethodHead {
				writer.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			writer.Header().Set("Docker-Content-Digest", "sha256:"+strings.Repeat("a", 64))
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "https://")
	credential, err := normalizeImageRegistryCredential("Test", address, "robot", "private-secret", clusterSelection{Mode: clusterSelectionNone})
	require.NoError(t, err)
	secret, err := buildImageRegistryReplicationSecret("management", "ir-0123456789abcdef", credential)
	require.NoError(t, err)
	gateway := &Gateway{
		config:    Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}, AstribotConfig: AstribotConfig{RegistryBrowseAllowedHosts: []string{address}}},
		rawClient: fake.NewSimpleClientset(secret), astribotState: astribotState{registryHTTPClient: server.Client()},
		jwtSigningKey: []byte("01234567890123456789012345678901"),
	}
	router := gin.New()
	gateway.RegisterRoutes(router)
	admin, _, err := gateway.issueJWT("admin", "admin")
	require.NoError(t, err)
	user, _, err := gateway.issueJWT("user", "user")
	require.NoError(t, err)
	call := func(path, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/image-registries/ir-0123456789abcdef/"+path, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.NotContains(t, recorder.Body.String(), "private-secret")
		return recorder
	}
	require.Equal(t, http.StatusForbidden, call("repositories", user).Code)
	require.Zero(t, requests.Load())
	first := call("repositories", admin)
	require.Equal(t, http.StatusOK, first.Code)
	require.JSONEq(t, `{"items":["rlark/agent"],"next":"rlark/agent"}`, first.Body.String())
	second := call("repositories?last="+url.QueryEscape("rlark/agent"), admin)
	require.Equal(t, http.StatusOK, second.Code)
	require.JSONEq(t, `{"items":["rlark/gateway"],"next":""}`, second.Body.String())
	require.Equal(t, http.StatusOK, call("tags?repository=rlark%2Fagent", admin).Code)
	require.Contains(t, call("manifest?repository=rlark%2Fagent&tag=v1", admin).Body.String(), strings.Repeat("a", 64))
	require.Equal(t, http.StatusBadRequest, call("tags?repository=..%2Ffoo", admin).Code)
	failing.Store(true)
	require.Equal(t, http.StatusBadGateway, call("repositories", admin).Code)
	failing.Store(false)
	require.Equal(t, http.StatusOK, call("repositories", admin).Code)
	_, err = gateway.rawClient.CoreV1().Secrets("management").Get(context.Background(), secret.Name, metav1.GetOptions{})
	require.NoError(t, err)
}

func TestRegistryBrowserRejectsUntrustedTLSAndCancellation(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { writer.WriteHeader(http.StatusOK) }))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "https://")
	credential := storedImageRegistryCredential{Registry: address, Username: "robot", Password: "secret"}
	browser := (&Gateway{config: Config{AstribotConfig: AstribotConfig{RegistryBrowseAllowedHosts: []string{address}}}}).registryBrowser()
	_, err := browser.request(context.Background(), credential, http.MethodGet, "/v2/_catalog", nil)
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = browser.request(ctx, credential, http.MethodGet, "/v2/_catalog", nil)
	require.ErrorIs(t, err, context.Canceled)
}

func TestRegistryBrowserDoesNotFollowRedirect(t *testing.T) {
	var targetRequests atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		targetRequests.Add(1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, target.URL, http.StatusFound)
	}))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "https://")
	browser := (&Gateway{config: Config{AstribotConfig: AstribotConfig{RegistryBrowseAllowedHosts: []string{address}}}}).registryBrowser()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	browser.client.Transport = &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	response, err := browser.request(context.Background(), storedImageRegistryCredential{Registry: address, Username: "robot", Password: "secret"}, http.MethodGet, "/v2/_catalog", nil)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusFound, response.StatusCode)
	require.Zero(t, targetRequests.Load())
}

func TestRegistryHostAllowedCIDR(t *testing.T) {
	allowed := []string{"10.0.0.0/8", "192.168.0.0/16", "registry.example:5443"}
	for _, tc := range []struct {
		authority, host string
		want            bool
	}{
		{"10.11.14.128:5443", "10.11.14.128", true},
		{"192.168.31.2:5443", "192.168.31.2", true},
		{"192.0.2.1:5443", "192.0.2.1", false},
		{"registry.example:5443", "registry.example", true},
		{"registry.example:443", "registry.example", false},
		{"127.0.0.1:5443", "127.0.0.1", false},
		{"example.test:5443", "example.test", false},
	} {
		t.Run(tc.authority, func(t *testing.T) { require.Equal(t, tc.want, registryHostAllowed(allowed, tc.authority, tc.host)) })
	}
}
