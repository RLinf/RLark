package httpproto

import (
	"net/url"
	"testing"
)

func TestRewriteProxyLocation(t *testing.T) {
	target, _ := url.Parse("https://172.16.0.2:8443")
	prefix := "/v1/robots/franka-0/proxy"

	cases := []struct {
		name     string
		location string
		want     string
	}{
		{
			name:     "absolute_url_same_host",
			location: "https://172.16.0.2:8443/dashboard",
			want:     "/v1/robots/franka-0/proxy/dashboard",
		},
		{
			name:     "absolute_url_different_host",
			location: "https://other.example.com/page",
			want:     "https://other.example.com/page",
		},
		{
			name:     "absolute_path",
			location: "/api/status",
			want:     "/v1/robots/franka-0/proxy/api/status",
		},
		{
			name:     "relative_path",
			location: "next-page",
			want:     "next-page",
		},
		{
			name:     "empty_path_absolute_url",
			location: "https://172.16.0.2:8443",
			want:     "/v1/robots/franka-0/proxy",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := RewriteProxyLocation(c.location, prefix, target)
			if got != c.want {
				t.Errorf("RewriteProxyLocation(%q) = %q, want %q", c.location, got, c.want)
			}
		})
	}
}

func TestProxyMountPrefix(t *testing.T) {
	got := ProxyMountPrefix("robots", "franka-0")
	want := "/v1/robots/franka-0/proxy"
	if got != want {
		t.Errorf("ProxyMountPrefix = %q, want %q", got, want)
	}
}
