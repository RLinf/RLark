package gateway

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

// resolveS3MountOptions validates and resolves an operator's mount policy.
// Backend options are rendered into the remote config, not passed as VFS flags.
func resolveS3MountOptions(profile, cacheMode string, overrides []string) ([]string, map[string]string, string, error) {
	options := make(map[string]string)
	for _, option := range s3MountOptionsForCacheMode(cacheMode) {
		key, value, ok := strings.Cut(option, "=")
		if !ok {
			value = "true"
		}
		options[key] = value
	}
	backend := make(map[string]string)
	switch profile {
	case "", "default":
	case "read":
		options["read-only"] = "true"
	case "archive":
		options["vfs-cache-mode"] = "off"
		options["dir-cache-time"] = "5s"
		options["attr-timeout"] = "1s"
		backend["chunk_size"] = "32Mi"
		backend["upload_concurrency"] = "4"
	default:
		return nil, nil, "", fmt.Errorf("mount_profile must be default, read or archive")
	}
	seen := make(map[string]bool)
	for _, raw := range overrides {
		key, value, hasValue := strings.Cut(raw, "=")
		if !hasValue {
			value = "true"
		}
		if seen[key] {
			return nil, nil, "", fmt.Errorf("duplicate mount option %q", key)
		}
		seen[key] = true
		if err := validateS3MountOption(key, value); err != nil {
			return nil, nil, "", err
		}
		if strings.HasPrefix(key, "s3-") {
			backend[strings.ReplaceAll(strings.TrimPrefix(key, "s3-"), "-", "_")] = value
		} else {
			options[key] = value
		}
	}
	if profile == "archive" && options["vfs-cache-mode"] != "off" {
		return nil, nil, "", fmt.Errorf("archive profile requires vfs-cache-mode=off")
	}
	if profile == "read" && options["read-only"] != "true" {
		return nil, nil, "", fmt.Errorf("read profile requires read-only=true")
	}
	if cacheMode != "" && options["vfs-cache-mode"] != cacheMode {
		return nil, nil, "", fmt.Errorf("vfs_cache_mode conflicts with mount profile/options")
	}
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]string, 0, len(keys))
	for _, key := range keys {
		result = append(result, key+"="+options[key])
	}
	return result, backend, options["vfs-cache-mode"], nil
}

func validateS3MountOption(key, value string) error {
	bad := func() error { return fmt.Errorf("invalid or unsupported mount option %q", key) }
	if strings.ContainsAny(value, "\r\n\x00") {
		return bad()
	}
	switch key {
	case "vfs-cache-mode":
		if value != "writes" && value != "off" {
			return bad()
		}
	case "read-only", "fast-list":
		if value != "true" && value != "false" {
			return bad()
		}
	case "dir-cache-time", "attr-timeout", "poll-interval", "vfs-write-back", "vfs-cache-max-age":
		d, err := time.ParseDuration(value)
		if err != nil || d < 0 {
			return bad()
		}
	case "transfers", "checkers", "low-level-retries", "s3-upload-concurrency":
		n, err := strconv.ParseInt(value, 10, 32)
		if err != nil || n < 1 {
			return bad()
		}
	case "buffer-size", "s3-chunk-size":
		q, err := resource.ParseQuantity(value)
		if err != nil || q.Sign() < 0 || q.AsApproximateFloat64() != float64(q.Value()) {
			return bad()
		}
		if key == "s3-chunk-size" && (q.Value() < 5*1024*1024 || q.Value() > 5_000_000_000) {
			return bad()
		}
	default:
		// In particular, ignore-checksum, credentials, arbitrary paths and
		// remote-control flags cannot be injected through a mount declaration.
		// tpslimit is not a mount option either: the node plugin has one
		// transaction limiter per process (csi-driver-rclone Addon, nodeTpsLimit).
		return bad()
	}
	return nil
}
