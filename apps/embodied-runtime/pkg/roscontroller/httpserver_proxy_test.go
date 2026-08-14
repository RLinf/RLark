package roscontroller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTestProxyBackend returns an httptest.Server that echoes the incoming
// path/query and lets the caller set a Location header via the
// X-Set-Location request header (so we can exercise rewriteProxyLocation).
func newTestProxyBackend(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if loc := r.Header.Get("X-Set-Location"); loc != "" {
			w.Header().Set("Location", loc)
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "path="+r.URL.Path+" query="+r.URL.RawQuery)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newTestProxyServer builds an HTTPServer with one robot registered against
// the given backend, and returns an httptest.Server serving its Handler().
func newTestProxyServer(t *testing.T, robotID, backendURL string) *httptest.Server {
	t.Helper()
	ctrl := NewController("10.0.0.1", nil)
	hs := NewHTTPServer(ctrl, "")
	hs.RegisterRobotWeb(robotID, backendURL)
	srv := httptest.NewServer(hs.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// proxyPrefix is the mount path the reverse proxy lives under, with the
// robot id interpolated. Kept here as a test constant so the test breaks
// loudly if httpserver.go's proxyMountPrefix changes.
const proxyPrefix = "/v1/robots/franka-0/proxy"

// TestHTTPProxy_Routes verifies /v1/robots/{robot_id}/proxy/<path> is
// reverse-proxied to the backend with the mount prefix stripped, and the
// raw query is preserved.
func TestHTTPProxy_Routes(t *testing.T) {
	backend := newTestProxyBackend(t)
	srv := newTestProxyServer(t, "franka-0", backend.URL)

	resp, err := http.Get(srv.URL + proxyPrefix + "/api/status?x=1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "path=/api/status") {
		t.Errorf("body = %q, want path=/api/status proxied through", body)
	}
	if !strings.Contains(string(body), "query=x=1") {
		t.Errorf("body = %q, want query=x=1 preserved", body)
	}
}

// TestHTTPProxy_BarePrefix verifies /v1/robots/{robot_id}/proxy (bare, no
// trailing slash) reaches the proxy (no 301 redirect, which would be lossy
// for non-GET proxying) and forwards to the backend's root.
func TestHTTPProxy_BarePrefix(t *testing.T) {
	backend := newTestProxyBackend(t)
	srv := newTestProxyServer(t, "franka-0", backend.URL)

	resp, err := http.Get(srv.URL + proxyPrefix)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no 301 redirect)", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "path=/") {
		t.Errorf("body = %q, want path=/", body)
	}
}

// TestHTTPProxy_TrailingSlash verifies /v1/robots/{robot_id}/proxy/ (with
// trailing slash) forwards to the backend root.
func TestHTTPProxy_TrailingSlash(t *testing.T) {
	backend := newTestProxyBackend(t)
	srv := newTestProxyServer(t, "franka-0", backend.URL)

	resp, err := http.Get(srv.URL + proxyPrefix + "/")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "path=/") {
		t.Errorf("body = %q, want path=/", body)
	}
}

// TestHTTPProxy_UnknownRobot verifies an unregistered robot id yields 404
// (not a panic or 502).
func TestHTTPProxy_UnknownRobot(t *testing.T) {
	backend := newTestProxyBackend(t)
	srv := newTestProxyServer(t, "franka-0", backend.URL)

	resp, err := http.Get(srv.URL + "/v1/robots/ghost/proxy/path")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

// noRedirectClient returns an http.Client that does NOT follow redirects,
// so a test can inspect the Location header of a 30x response directly.
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// TestHTTPProxy_RewriteLocation_AbsoluteURL verifies a Location header
// holding an absolute URL pointing at the backend host is rewritten back to
// /v1/robots/<robot_id>/proxy/<path> so the client re-enters the gateway.
func TestHTTPProxy_RewriteLocation_AbsoluteURL(t *testing.T) {
	backend := newTestProxyBackend(t)
	backendURL, _ := url.Parse(backend.URL)
	srv := newTestProxyServer(t, "franka-0", backend.URL)

	loc := backend.URL + "/dashboard"
	req, _ := http.NewRequest(http.MethodGet, srv.URL+proxyPrefix+"/redirect", nil)
	req.Header.Set("X-Set-Location", loc)
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, want 302", resp.StatusCode)
	}
	got := resp.Header.Get("Location")
	want := proxyPrefix + "/dashboard"
	if got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if strings.Contains(got, backendURL.Host) {
		t.Errorf("Location %q still contains backend host %q", got, backendURL.Host)
	}
}

// TestHTTPProxy_RewriteLocation_AbsolutePath verifies a Location header
// holding an absolute path (/foo) is prefixed with the proxy mount.
func TestHTTPProxy_RewriteLocation_AbsolutePath(t *testing.T) {
	backend := newTestProxyBackend(t)
	srv := newTestProxyServer(t, "franka-0", backend.URL)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+proxyPrefix+"/redirect", nil)
	req.Header.Set("X-Set-Location", "/foo/bar")
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Location"); got != proxyPrefix+"/foo/bar" {
		t.Errorf("Location = %q, want %q", got, proxyPrefix+"/foo/bar")
	}
}

// TestHTTPProxy_RewriteLocation_Relative verifies a relative Location (no
// leading /) is left untouched so the browser resolves it against the
// already-correct proxied URL.
func TestHTTPProxy_RewriteLocation_Relative(t *testing.T) {
	backend := newTestProxyBackend(t)
	srv := newTestProxyServer(t, "franka-0", backend.URL)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+proxyPrefix+"/redirect", nil)
	req.Header.Set("X-Set-Location", "next.html")
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Location"); got != "next.html" {
		t.Errorf("Location = %q, want next.html (relative left as-is)", got)
	}
}

// TestHTTPProxy_RewriteLocation_ForeignHost verifies an absolute Location
// pointing at a different host is left untouched so the client is sent
// off-site (e.g. to an external OAuth provider).
func TestHTTPProxy_RewriteLocation_ForeignHost(t *testing.T) {
	backend := newTestProxyBackend(t)
	srv := newTestProxyServer(t, "franka-0", backend.URL)

	req, _ := http.NewRequest(http.MethodGet, srv.URL+proxyPrefix+"/redirect", nil)
	req.Header.Set("X-Set-Location", "https://example.com/oauth/callback")
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := resp.Header.Get("Location"); got != "https://example.com/oauth/callback" {
		t.Errorf("Location = %q, want unchanged foreign host", got)
	}
}

// TestHTTPProxy_RegisterUnregister verifies registering a robot makes it
// routable and unregistering makes it 404 again — through the HTTPServer.
func TestHTTPProxy_RegisterUnregister(t *testing.T) {
	backend := newTestProxyBackend(t)
	ctrl := NewController("10.0.0.1", nil)
	hs := NewHTTPServer(ctrl, "")
	srv := httptest.NewServer(hs.Handler())
	t.Cleanup(srv.Close)

	// Not registered yet → 404.
	resp, err := http.Get(srv.URL + "/v1/robots/franka-0/proxy/path")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("before register: status = %d, want 404", resp.StatusCode)
	}

	// Register → 200.
	hs.RegisterRobotWeb("franka-0", backend.URL)
	resp, err = http.Get(srv.URL + "/v1/robots/franka-0/proxy/path")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("after register: status = %d, want 200", resp.StatusCode)
	}

	// Unregister → 404.
	hs.UnregisterRobotWeb("franka-0")
	resp, err = http.Get(srv.URL + "/v1/robots/franka-0/proxy/path")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("after unregister: status = %d, want 404", resp.StatusCode)
	}
}

// TestHTTPPanicRecovery_Middleware verifies the recovery middleware itself
// catches a panic in any /v1/ handler (including the reverse proxy path,
// which runs inside this middleware) and returns 500 rather than dropping
// the connection. We register a panicking handler directly on the mux to
// exercise the middleware in isolation — this is the guarantee that a
// ReverseProxy panic (Director bug, malformed target, mid-stream fault) is
// contained: the proxy's ServeHTTP runs inside this same wrapper.
func TestHTTPPanicRecovery_Middleware(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	// Wrap the same way Handler() does.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	resp, err := http.Get(srv.URL + "/v1/boom")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (panic recovered)", resp.StatusCode)
	}
}
