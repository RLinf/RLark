package gateway

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestRewriteTensorBoardResponse_JSPassthrough(t *testing.T) {
	// TensorBoard's JS bundle contains string literals like "/tags", "/scalars"
	// that are route FRAGMENTS passed to pluginRoute(), not absolute URLs.
	// The rewriter must NOT prepend the proxy prefix to them.
	original := []byte(`let t=_e().pluginRoute("histograms","/tags");this._sendRequest("/experiment",t)`)
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/javascript; charset=utf-8"}},
		Body:       io.NopCloser(bytes.NewReader(original)),
	}
	if err := rewriteTensorBoardResponse(resp, "/api/v1/rlinf.io/v1alpha1/tasks/foo/tensorboard"); err != nil {
		t.Fatalf("rewriteTensorBoardResponse: %v", err)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("JS body was modified (must be passed through unchanged):\n got: %s\nwant: %s", got, original)
	}
}

func TestRewriteTensorBoardResponse_HTMLRewritesCSSURL(t *testing.T) {
	// Absolute url() paths in inline CSS (e.g. font URLs) must still be rewritten.
	original := []byte(`<style>src:url(/font-roboto/foo.woff2) format('woff2')</style>`)
	want := []byte(`<style>src:url(/api/v1/rlinf.io/v1alpha1/tasks/foo/tensorboard/font-roboto/foo.woff2) format('woff2')</style>`)
	resp := &http.Response{
		StatusCode: 200,
		Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
		Body:       io.NopCloser(bytes.NewReader(original)),
	}
	if err := rewriteTensorBoardResponse(resp, "/api/v1/rlinf.io/v1alpha1/tasks/foo/tensorboard"); err != nil {
		t.Fatalf("rewriteTensorBoardResponse: %v", err)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("HTML body not rewritten as expected:\n got: %s\nwant: %s", got, want)
	}
}

func TestRewriteTensorBoardResponse_RedirectLocation(t *testing.T) {
	resp := &http.Response{
		StatusCode: 302,
		Header:     http.Header{"Location": []string{"/some/path"}},
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}
	if err := rewriteTensorBoardResponse(resp, "/api/v1/rlinf.io/v1alpha1/tasks/foo/tensorboard"); err != nil {
		t.Fatalf("rewriteTensorBoardResponse: %v", err)
	}
	if got := resp.Header.Get("Location"); got != "/api/v1/rlinf.io/v1alpha1/tasks/foo/tensorboard/some/path" {
		t.Errorf("Location header = %q, want rewritten prefix", got)
	}
}
