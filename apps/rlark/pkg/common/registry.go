package common

import "strings"

// NormalizeRegistry makes a registry address comparable with container image
// references. Users commonly paste a URL while Kubernetes images use a bare
// host[:port] prefix.
func NormalizeRegistry(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "https://")
	value = strings.TrimPrefix(value, "http://")
	value = strings.TrimRight(value, "/")
	value = strings.TrimSuffix(value, "/v2")
	return strings.TrimRight(value, "/")
}
