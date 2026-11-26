package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// proxyKubeRequest sends a request to an agent Kubernetes API through the
// rlark-server proxy. kubePath is the Kubernetes API path.
func (g *Gateway) proxyKubeRequest(ctx context.Context, method, agentID, kubePath string, body []byte) (*http.Response, error) {
	return g.proxyKubeRequestWithHeaders(ctx, method, agentID, kubePath, body, nil)
}

// proxyKubeRequestWithHeaders is like proxyKubeRequest but allows the caller
// to override headers (e.g. Content-Type for strategic merge patches).
func (g *Gateway) proxyKubeRequestWithHeaders(ctx context.Context, method, agentID, kubePath string, body []byte, extraHeaders map[string]string) (*http.Response, error) {
	httpClient := &http.Client{Transport: g.serverTransport}
	url := fmt.Sprintf("%s/api/proxy/%s/api/kubernetes/%s",
		strings.TrimSuffix(g.config.ServerAddress, "/"),
		agentID,
		strings.TrimPrefix(kubePath, "/"),
	)
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return httpClient.Do(req)
}
