package httpproto

import (
	"fmt"
	"net/url"
	"strings"
)

// RewriteProxyLocation rewrites a Location header from a proxied response
// so the client follows the redirect back through the gateway under its
// proxy mount prefix.
//
// Cases:
//   - Absolute URL pointing to the target host → rewrite to
//     <mountPrefix><path>
//   - Absolute path (/...) → prefix with <mountPrefix>
//   - Relative path (no leading /) → leave as-is (browser resolves relative
//     to the proxied URL, which is already correct)
//
// mountPrefix is the full path prefix the proxy is mounted under, e.g.
// "/v1/robots/franka-0/proxy". targetURL is the parsed backend URL.
func RewriteProxyLocation(location, mountPrefix string, targetURL *url.URL) string {
	locURL, err := url.Parse(location)
	if err != nil {
		return location
	}

	// Absolute URL: rewrite to a path under the proxy mount if it matches
	// the target host.
	if locURL.IsAbs() {
		if locURL.Host == targetURL.Host {
			return mountPrefix + locURL.Path
		}
		return location
	}

	// Absolute path: prefix with the proxy mount.
	if strings.HasPrefix(location, "/") {
		return mountPrefix + location
	}

	// Relative path — leave as-is.
	return location
}

// ProxyMountPrefix returns the path prefix a per-resource reverse proxy is
// mounted under. It mirrors the pattern used by ROS and camera controllers:
// "/v1/<resourcePlural>/<id>/proxy". Example:
//
//	ProxyMountPrefix("robots", "franka-0") → "/v1/robots/franka-0/proxy"
func ProxyMountPrefix(resourcePlural, id string) string {
	return fmt.Sprintf("/v1/%s/%s/proxy", resourcePlural, id)
}
