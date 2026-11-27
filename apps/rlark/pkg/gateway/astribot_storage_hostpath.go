package gateway

// Host-path storage: a file system that nodes have already mounted at one
// directory (for example Kingsoft Cloud KPFS at /mnt/xc_kpfs/regular, of type
// kingstorage). RLark does not mount it: the node directory CSI driver
// (apps/rlark/astribot/nodepath) binds the directory into Pods, and its node plugin finds
// which nodes have it.

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
)

var fsTypePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func normalizeHostPathStorage(req CreateStorageClassRequest, name string, clusters []string) (storedStorageClass, error) {
	for field, value := range map[string]string{
		"provider": req.Provider, "endpoint": req.Endpoint, "mount_endpoint": req.MountEndpoint, "bucket": req.Bucket,
		"prefix": req.Prefix, "vfs_cache_mode": req.VFSCacheMode, "mount_profile": req.MountProfile, "access_key_id": req.AccessKeyId,
		"access_key_secret": req.AccessKeySecret, "host": req.Host, "share": req.Share, "sub_path": req.SubPath,
		"username": req.Username, "password": req.Password, "domain": req.Domain, "credentials_from": req.CredentialsFrom,
	} {
		if strings.TrimSpace(value) != "" {
			return storedStorageClass{}, fmt.Errorf("%s is not supported for host-path storage", field)
		}
	}
	if req.MountOptions != nil && len(*req.MountOptions) != 0 {
		return storedStorageClass{}, fmt.Errorf("mount_options is not supported for host-path storage")
	}
	directory := strings.TrimSpace(req.Path)
	if !path.IsAbs(directory) || path.Clean(directory) != directory || directory == "/" || strings.IndexFunc(directory, unicode.IsControl) >= 0 {
		return storedStorageClass{}, fmt.Errorf("path must be a clean absolute directory other than /")
	}
	fsType := strings.TrimSpace(req.FSType)
	if fsType != "" && !fsTypePattern.MatchString(fsType) {
		return storedStorageClass{}, fmt.Errorf("fs_type must be a file system type as /proc/mounts lists it, such as kingstorage")
	}
	stored := storedStorageClass{Version: storageClassVersion, Name: name, Description: req.Description, Clusters: clusters}
	stored.Type, stored.Path, stored.FSType = storageTypeHostPath, directory, fsType
	return stored, nil
}
