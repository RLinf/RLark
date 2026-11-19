package server

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"k8s.io/client-go/rest"
)

// KubeProxy is a proxy.
type KubeProxy struct {
	apiServer *url.URL
	transport http.RoundTripper
}

// NewKubeProxy creates a new KubeProxy instance.
func NewKubeProxy(restConfig *rest.Config) (*KubeProxy, error) {
	// Build API Server URL
	apiURL, err := url.Parse(restConfig.Host)
	if err != nil {
		return nil, fmt.Errorf("parse API server URL: %w", err)
	}
	// Build Transport (automatically handles certificates, tokens, etc.)
	transport, err := rest.TransportFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create transport for API server: %w", err)
	}
	return &KubeProxy{
		apiServer: apiURL,
		transport: transport,
	}, nil
}

// GetHandler returns an http.Handler that proxies requests to the Kubernetes API server.
func (p *KubeProxy) GetHandler() http.Handler {
	logger := log.GetLogger()
	proxy := &httputil.ReverseProxy{}
	proxy.Transport = p.transport
	proxy.Rewrite = func(pr *httputil.ProxyRequest) {
		req := pr.Out
		req.URL.Scheme = p.apiServer.Scheme
		req.URL.Host = p.apiServer.Host
		req.URL.Path = path.Join(p.apiServer.Path, req.URL.Path)
		req.Host = p.apiServer.Host
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		logger.Error(nil, "KubeProxy error", "err", err)
		http.Error(w, "KubeProxy error: "+err.Error(), http.StatusBadGateway)
	}
	return proxy
}
