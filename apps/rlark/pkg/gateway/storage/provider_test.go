package storage

import "testing"

func TestLookupProviderAcceptsCanonicalDisplayNameAndAlias(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "canonical value", input: "Alibaba", want: "Alibaba"},
		{name: "case insensitive value", input: "alibaba", want: "Alibaba"},
		{name: "display name", input: "阿里云 OSS", want: "Alibaba"},
		{name: "legacy alias", input: "Aliyun OSS", want: "Alibaba"},
		{name: "surrounding spaces", input: "  Minio  ", want: "Minio"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider, ok := LookupProvider(tc.input)
			if !ok {
				t.Fatalf("LookupProvider(%q) reported unknown provider", tc.input)
			}
			if provider.Value != tc.want {
				t.Fatalf("LookupProvider(%q) = %q, want %q", tc.input, provider.Value, tc.want)
			}
		})
	}
}

func TestLookupProviderRejectsUnknownValue(t *testing.T) {
	for _, input := range []string{"", "   ", "S3 兼容", "Alibaba Cloud OSS"} {
		if _, ok := LookupProvider(input); ok {
			t.Fatalf("LookupProvider(%q) accepted an unknown provider", input)
		}
	}
}

func TestEffectivePathStyle(t *testing.T) {
	cases := []struct {
		name      string
		provider  string
		requested bool
		want      bool
	}{
		{name: "virtual hosted only forces off", provider: "Alibaba", requested: true, want: false},
		{name: "virtual hosted only by alias", provider: "Aliyun OSS", requested: true, want: false},
		{name: "generic provider keeps request", provider: "Minio", requested: true, want: true},
		{name: "generic provider keeps request off", provider: "Minio", requested: false, want: false},
		{name: "unknown provider keeps request", provider: "nonexistent", requested: true, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EffectivePathStyle(tc.provider, tc.requested); got != tc.want {
				t.Fatalf("EffectivePathStyle(%q, %t) = %t, want %t", tc.provider, tc.requested, got, tc.want)
			}
		})
	}
}

func TestNormalizeEndpointDerivesRegionalEndpoint(t *testing.T) {
	endpoint, err := NormalizeEndpoint("Alibaba", "", "cn-beijing")
	if err != nil {
		t.Fatalf("NormalizeEndpoint returned error: %v", err)
	}
	if endpoint != "https://oss-cn-beijing.aliyuncs.com" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
}

func TestNormalizeEndpointKeepsCallerEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		endpoint string
		want     string
	}{
		{name: "keeps explicit endpoint", provider: "Alibaba", endpoint: "https://oss-cn-hangzhou.aliyuncs.com", want: "https://oss-cn-hangzhou.aliyuncs.com"},
		{name: "adds https scheme", provider: "Minio", endpoint: "minio.example.com", want: "https://minio.example.com"},
		{name: "keeps http scheme", provider: "Minio", endpoint: "http://minio.example.com:9000", want: "http://minio.example.com:9000"},
		{name: "trims trailing slash", provider: "Minio", endpoint: "https://minio.example.com/", want: "https://minio.example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			endpoint, err := NormalizeEndpoint(tc.provider, tc.endpoint, "cn-beijing")
			if err != nil {
				t.Fatalf("NormalizeEndpoint returned error: %v", err)
			}
			if endpoint != tc.want {
				t.Fatalf("NormalizeEndpoint = %q, want %q", endpoint, tc.want)
			}
		})
	}
}

func TestNormalizeEndpointRejectsIncompleteInput(t *testing.T) {
	cases := []struct {
		name     string
		provider string
		endpoint string
		region   string
	}{
		{name: "missing endpoint without regional format", provider: "Minio"},
		{name: "missing region for derived endpoint", provider: "Alibaba"},
		{name: "unsupported scheme", provider: "Minio", endpoint: "ftp://minio.example.com"},
		{name: "empty host", provider: "Minio", endpoint: "https:///bucket"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeEndpoint(tc.provider, tc.endpoint, tc.region); err == nil {
				t.Fatalf("NormalizeEndpoint(%q, %q, %q) accepted invalid input", tc.provider, tc.endpoint, tc.region)
			}
		})
	}
}
