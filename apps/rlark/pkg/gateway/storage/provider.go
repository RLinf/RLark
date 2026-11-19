package storage

import (
	"fmt"
	"net/url"
	"strings"
)

// Provider describes one selectable S3-compatible backend.
//
// Value is the canonical rclone `provider` setting written into the
// StorageClass Secret. rclone does not fail on an unknown provider: it logs a
// warning and silently falls back to its generic "Other" profile, so a wrong
// value is only visible as a mount-time I/O error on the data plane. Every
// value accepted by the API must therefore come from this catalog.
type Provider struct {
	// DisplayName is the human-readable label presented to clients.
	DisplayName string
	// Value is the canonical rclone s3 provider value.
	Value string
	// Aliases are additional spellings accepted on input, kept so that records
	// written by older clients can still be read and rewritten canonically.
	Aliases []string
	// VirtualHostedOnly reports that the backend rejects path-style addressing.
	VirtualHostedOnly bool
	// RegionEndpointFormat derives the endpoint from the region when the caller
	// leaves the endpoint empty. An empty format means the endpoint is required.
	RegionEndpointFormat string
}

// Providers is the ordered catalog of selectable providers. Several entries
// intentionally share the rclone value "Other", so the catalog is an ordered
// slice rather than a map keyed by value.
var Providers = []Provider{
	{DisplayName: "AWS S3", Value: "AWS", Aliases: []string{"Amazon S3"}},
	{
		DisplayName:          "阿里云 OSS",
		Value:                "Alibaba",
		Aliases:              []string{"Aliyun OSS", "Aliyun", "AlibabaOSS"},
		VirtualHostedOnly:    true,
		RegionEndpointFormat: "https://oss-%s.aliyuncs.com",
	},
	{DisplayName: "腾讯云 COS", Value: "TencentCOS", Aliases: []string{"Tencent COS"}, VirtualHostedOnly: true},
	{DisplayName: "华为云 OBS", Value: "HuaweiOBS", Aliases: []string{"Huawei OBS"}, VirtualHostedOnly: true},
	{DisplayName: "七牛云 Kodo", Value: "Qiniu"},
	{DisplayName: "移动云 EOS", Value: "ChinaMobile"},
	{DisplayName: "网易 NOS", Value: "Netease"},
	{DisplayName: "UCloud US3", Value: "Ucloud"},
	{DisplayName: "MinIO", Value: "Minio"},
	{DisplayName: "Ceph RGW", Value: "Ceph"},
	{DisplayName: "SeaweedFS", Value: "SeaweedFS"},
	{DisplayName: "Cloudflare R2", Value: "Cloudflare"},
	{DisplayName: "Google Cloud Storage", Value: "GCS", Aliases: []string{"Google Cloud"}},
	{DisplayName: "DigitalOcean Spaces", Value: "DigitalOcean"},
	{DisplayName: "Wasabi", Value: "Wasabi"},
	{DisplayName: "Backblaze B2 (S3)", Value: "Other"},
	{DisplayName: "IBM COS", Value: "IBMCOS"},
	{DisplayName: "Scaleway", Value: "Scaleway"},
	{DisplayName: "OVHcloud", Value: "OVH"},
	{DisplayName: "Linode / Akamai", Value: "Linode"},
	{DisplayName: "IDrive e2", Value: "IDrive"},
	{DisplayName: "Seagate Lyve Cloud", Value: "LyveCloud"},
	{DisplayName: "Storj (S3 网关)", Value: "Storj"},
	{DisplayName: "Synology C2", Value: "Synology"},
	{DisplayName: "ArvanCloud", Value: "ArvanCloud"},
	{DisplayName: "Hetzner", Value: "Hetzner"},
	{DisplayName: "IONOS", Value: "IONOS"},
	{DisplayName: "MEGA S4", Value: "Mega"},
	{DisplayName: "RackCorp", Value: "RackCorp"},
	{DisplayName: "Dreamhost", Value: "Dreamhost"},
	{DisplayName: "其他 S3 兼容", Value: "Other"},
}

// LookupProvider resolves a provider spelling to its catalog entry. The canonical
// value, the display name, and the recorded aliases are all accepted so that
// records written by older clients remain readable.
func LookupProvider(provider string) (Provider, bool) {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return Provider{}, false
	}
	for _, candidate := range Providers {
		if strings.EqualFold(candidate.Value, provider) || strings.EqualFold(candidate.DisplayName, provider) {
			return candidate, true
		}
		for _, alias := range candidate.Aliases {
			if strings.EqualFold(alias, provider) {
				return candidate, true
			}
		}
	}
	return Provider{}, false
}

// EffectivePathStyle reports the addressing style to use for provider. Backends
// that only answer virtual-hosted requests always force path-style off, so the
// gateway client and the rclone config rendered for the data plane agree even
// when a stored record still carries the requested value.
func EffectivePathStyle(provider string, requested bool) bool {
	if entry, ok := LookupProvider(provider); ok && entry.VirtualHostedOnly {
		return false
	}
	return requested
}

// NormalizeEndpoint returns the canonical endpoint URL for provider.
//
// The result never embeds the bucket name: both the AWS SDK and rclone compose
// virtual-hosted URLs themselves, so a pre-composed endpoint is addressed as
// bucket.bucket.<host>. An empty endpoint is derived from the region when the
// provider publishes a regional endpoint format.
func NormalizeEndpoint(provider, endpoint, region string) (string, error) {
	entry, _ := LookupProvider(provider)
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		if entry.RegionEndpointFormat == "" {
			return "", fmt.Errorf("endpoint is required")
		}
		if strings.TrimSpace(region) == "" {
			return "", fmt.Errorf("region is required to derive the %s endpoint", entry.DisplayName)
		}
		return fmt.Sprintf(entry.RegionEndpointFormat, strings.TrimSpace(region)), nil
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint %q: %w", endpoint, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid endpoint %q: scheme must be http or https", endpoint)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid endpoint %q: host is empty", endpoint)
	}
	return strings.TrimSuffix(parsed.String(), "/"), nil
}
