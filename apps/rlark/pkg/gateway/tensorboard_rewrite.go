package gateway

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// rewriteTensorBoardResponse rewrites absolute URL paths in TensorBoard's
// HTML/CSS responses and redirect Location headers so that resources
// referenced via href, src, url(), etc. are served through the proxy prefix
// instead of the site root.
//
// Without this rewriting, the browser would request /font-roboto/...
// (TensorBoard's absolute asset path in inline CSS) instead of
// /api/v1/.../tensorboard/font-roboto/..., resulting in 404 errors.
//
// JavaScript responses are intentionally NOT rewritten: TensorBoard computes
// all runtime URLs (data API, plugin assets) via pluginRoute() using the
// tb-relative-root meta tag (content="./"), which resolves relative to the
// document URL. The string literals inside the JS bundle that start with "/"
// (e.g. "/tags", "/scalars", "/experiment") are route FRAGMENTS passed to
// pluginRoute, not absolute URLs. Prepending the proxy prefix to them breaks
// every backend API call and leaves the page blank.
//
// proxyPrefix is the gateway URL path that maps to TensorBoard's root,
// WITHOUT a trailing slash, e.g. "/api/v1/rlinf.io/v1alpha1/tasks/foo/tensorboard".
func rewriteTensorBoardResponse(resp *http.Response, proxyPrefix string) error {
	if proxyPrefix == "" || proxyPrefix == "/" {
		return nil
	}

	// Rewrite redirect Location header (3xx) so the browser follows
	// the redirect through the proxy prefix rather than the site root.
	if loc := resp.Header.Get("Location"); loc != "" {
		if rewritten, ok := rewriteAbsolutePath(loc, proxyPrefix); ok {
			resp.Header.Set("Location", rewritten)
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}

	ct := resp.Header.Get("Content-Type")
	mediaType := strings.ToLower(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]))

	var rewrite func([]byte) []byte
	switch mediaType {
	case "text/html":
		rewrite = func(b []byte) []byte { return rewriteHTML(b, proxyPrefix) }
	case "text/css":
		rewrite = func(b []byte) []byte { return rewriteCSS(b, proxyPrefix) }
	default:
		return nil
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	rewritten := rewrite(body)
	resp.Body = io.NopCloser(bytes.NewReader(rewritten))
	resp.ContentLength = int64(len(rewritten))
	resp.Header.Set("Content-Length", strconv.Itoa(len(rewritten)))
	resp.Header.Del("Content-Encoding")
	resp.Header.Del("Transfer-Encoding")
	return nil
}

// readResponseBody reads and optionally decompresses the response body.
// Intermediate reverse proxies may auto-decompress gzip (when their transport
// added Accept-Encoding), but we handle it here as a fallback for cases where
// the upstream sends Content-Encoding: gzip explicitly.
func readResponseBody(resp *http.Response) ([]byte, error) {
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			utils.CloseIO(resp.Body)
			return nil, err
		}
		defer utils.CloseIO(gr)
		defer utils.CloseIO(resp.Body)
		return io.ReadAll(gr)
	}
	defer utils.CloseIO(resp.Body)
	return io.ReadAll(resp.Body)
}

// rewriteAbsolutePath prepends proxyPrefix to an absolute path (starting with
// "/" but not "//"). Returns (rewritten, true) if rewritten, or (original,
// false) if no rewrite was needed (relative URLs, full URLs, fragments, etc.).
func rewriteAbsolutePath(val, proxyPrefix string) (string, bool) {
	val = strings.TrimSpace(val)
	if len(val) == 0 || val[0] != '/' {
		return val, false // relative URL, full URL, fragment, etc.
	}
	if len(val) > 1 && val[1] == '/' {
		return val, false // protocol-relative URL (//host/path)
	}
	return proxyPrefix + val, true
}

// ---------------------------------------------------------------------------
// HTML rewriting
// ---------------------------------------------------------------------------

// htmlURLAttrRe matches HTML attributes that commonly contain URL values.
// Uses alternation (instead of backreferences, which Go's RE2 doesn't support)
// to ensure matching quote pairs.
// Captures: [1]=attr name, [2]=value (double-quoted), [3]=value (single-quoted).
var htmlURLAttrRe = regexp.MustCompile(
	`(?i)\b(href|src|action|poster|data-src|data-href|cite|longdesc|usemap|profile|manifest|background|icon)\s*=\s*(?:"(/[^"]*)"|'(/[^']*)')`)

// htmlMetaRefreshRe matches the url= directive in <meta http-equiv="refresh">.
// Captures: [1]="url=", [2]=path (starts with /).
var htmlMetaRefreshRe = regexp.MustCompile(
	`(?i)(url=)(/[^"'\s>]*)`)

// cssUrlRe matches url(/...) with optional quotes — used in both CSS files and
// HTML inline style attributes. Captures: [1]=path (starts with /).
var cssUrlRe = regexp.MustCompile(
	`(?i)url\(\s*["']?(/[^"')\s]*)["']?\s*\)`)

func rewriteHTML(body []byte, prefix string) []byte {
	// Rewrite URL attributes: href="/...", src="/...", etc.
	body = htmlURLAttrRe.ReplaceAllFunc(body, func(match []byte) []byte {
		sub := htmlURLAttrRe.FindSubmatch(match)
		// sub[1]=attr, sub[2]=double-quoted value, sub[3]=single-quoted value
		var path, quote string
		if len(sub[2]) > 0 {
			path = string(sub[2])
			quote = "\""
		} else if len(sub[3]) > 0 {
			path = string(sub[3])
			quote = "'"
		} else {
			return match
		}
		rewritten, ok := rewriteAbsolutePath(path, prefix)
		if !ok {
			return match
		}
		return []byte(string(sub[1]) + "=" + quote + rewritten + quote)
	})

	// Rewrite <meta http-equiv="refresh" content="0; url=/...">
	body = htmlMetaRefreshRe.ReplaceAllFunc(body, func(match []byte) []byte {
		sub := htmlMetaRefreshRe.FindSubmatch(match)
		rewritten, ok := rewriteAbsolutePath(string(sub[2]), prefix)
		if !ok {
			return match
		}
		return []byte(string(sub[1]) + rewritten)
	})

	// Rewrite url(/...) in inline style attributes and <style> blocks.
	body = cssUrlRe.ReplaceAllFunc(body, func(match []byte) []byte {
		sub := cssUrlRe.FindSubmatch(match)
		rewritten, ok := rewriteAbsolutePath(string(sub[1]), prefix)
		if !ok {
			return match
		}
		return []byte("url(" + rewritten + ")")
	})

	return body
}

// ---------------------------------------------------------------------------
// CSS rewriting
// ---------------------------------------------------------------------------

// cssImportRe matches @import "/..." or @import url("/...").
// Captures: [1]=path (starts with /).
var cssImportRe = regexp.MustCompile(
	`(?i)@import\s+(?:url\(\s*)?["'](/[^"']*)["']`)

func rewriteCSS(body []byte, prefix string) []byte {
	// Rewrite url(/...) — handles url(/path), url("/path"), url('/path')
	body = cssUrlRe.ReplaceAllFunc(body, func(match []byte) []byte {
		sub := cssUrlRe.FindSubmatch(match)
		rewritten, ok := rewriteAbsolutePath(string(sub[1]), prefix)
		if !ok {
			return match
		}
		return []byte("url(" + rewritten + ")")
	})

	// Rewrite @import "/..." or @import url("/...")
	body = cssImportRe.ReplaceAllFunc(body, func(match []byte) []byte {
		sub := cssImportRe.FindSubmatch(match)
		rewritten, ok := rewriteAbsolutePath(string(sub[1]), prefix)
		if !ok {
			return match
		}
		return []byte("@import \"" + rewritten + "\"")
	})

	return body
}
