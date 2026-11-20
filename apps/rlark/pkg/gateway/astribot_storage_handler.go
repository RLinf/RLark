package gateway

// What this site's storages add to the official storage handlers (storage_handler.go): the fields of the three
// storage types, and the functions the official flow calls at the places it has to branch.

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
	"github.com/rlinf/rlark/apps/rlark/pkg/gateway/storage"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// astribotStorageClassData is embedded in StorageClassData.
type astribotStorageClassData struct {
	Type                    string            `json:"type"`
	MountEndpoint           string            `json:"mount_endpoint,omitempty"`
	ClusterMountEndpoints   map[string]string `json:"cluster_mount_endpoints,omitempty"`
	Prefix                  string            `json:"prefix,omitempty"`
	VFSCacheMode            string            `json:"vfs_cache_mode,omitempty"`
	MountProfile            string            `json:"mount_profile,omitempty"`
	MountOptions            []string          `json:"mount_options"`
	EffectiveMountOptions   []string          `json:"effective_mount_options"`
	EffectiveBackendOptions map[string]string `json:"effective_backend_options,omitempty"`
	Host                    string            `json:"host,omitempty"`
	Share                   string            `json:"share,omitempty"`
	SubPath                 string            `json:"subPath,omitempty"`
	Username                string            `json:"username,omitempty"`
	Domain                  string            `json:"domain,omitempty"`
	Path                    string            `json:"path,omitempty"`
	FSType                  string            `json:"fs_type,omitempty"`
	// Profiles maps each mount profile a volume can choose to the
	// StorageClass it gets; the storage's own profile keeps storageClassName.
	Profiles       map[string]string `json:"profiles,omitempty"`
	DefaultProfile string            `json:"default_profile"`
}

// astribotStoredStorageClass is embedded in storedStorageClass. Every field is
// optional, so the record stays on version v1.
type astribotStoredStorageClass struct {
	// Type is "s3", "smb" or "host-path". A record without it is "s3".
	Type          string `json:"type,omitempty"`
	MountEndpoint string `json:"mount_endpoint,omitempty"`
	// ClusterMountEndpoints gives a cluster its own mount endpoint, for
	// example a cloud's internal address for the clusters in that cloud.
	ClusterMountEndpoints map[string]string `json:"cluster_mount_endpoints,omitempty"`
	Prefix                string            `json:"prefix,omitempty"`
	VFSCacheMode          string            `json:"vfs_cache_mode,omitempty"`
	MountProfile          string            `json:"mount_profile,omitempty"`
	MountOptions          []string          `json:"mount_options,omitempty"`
	Host                  string            `json:"host,omitempty"`
	Share                 string            `json:"share,omitempty"`
	SubPath               string            `json:"sub_path,omitempty"`
	Username              string            `json:"username,omitempty"`
	Password              string            `json:"password,omitempty"`
	Domain                string            `json:"domain,omitempty"`
	// Path and FSType describe a host-path storage: the directory the nodes
	// that have the storage have mounted, and the type of the file system it
	// must be on. Which nodes have it the driver's node plugin finds out.
	Path   string `json:"path,omitempty"`
	FSType string `json:"fs_type,omitempty"`
}

// astribotStorageClassRequest is embedded in CreateStorageClassRequest. Type
// selects the storage: "s3" (default) for S3-compatible object storage, "smb"
// for an SMB/CIFS share, "host-path" for a directory the nodes have mounted.
type astribotStorageClassRequest struct {
	Type string `json:"type"`

	MountEndpoint string `json:"mount_endpoint"`
	// ClusterMountEndpoints replaces the stored per-cluster endpoints when
	// given; omitted on update keeps them.
	ClusterMountEndpoints *map[string]string `json:"cluster_mount_endpoints"`
	Prefix                string             `json:"prefix"`
	VFSCacheMode          string             `json:"vfs_cache_mode"`
	MountProfile          string             `json:"mount_profile"`
	MountOptions          *[]string          `json:"mount_options"`
	// CredentialsFrom names an existing S3 storage whose provider, endpoint,
	// region and access key a new storage uses, so that several mounts of one
	// bucket (read, archive) are declared without entering the key again.
	CredentialsFrom string `json:"credentials_from"`

	Host     string `json:"host"`
	Share    string `json:"share"`
	SubPath  string `json:"sub_path"`
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain"`

	Path   string `json:"path"`
	FSType string `json:"fs_type"`
}

const (
	// storageTypeS3 is the default for records written before types existed.
	storageTypeS3       = "s3"
	storageTypeSMB      = "smb"
	storageTypeHostPath = storageref.TypeHostPath
)

// s3MountOptions and smbMountOptions are rclone VFS flags for the CSI mount.
// fast-list only applies to bucket-based remotes, so SMB omits it.
var (
	s3MountOptions  = []string{"vfs-cache-mode=writes", "dir-cache-time=5m", "attr-timeout=1m", "poll-interval=5s", "vfs-write-back=1s", "vfs-cache-max-age=10m", "buffer-size=64M", "transfers=8", "checkers=16", "fast-list"}
	smbMountOptions = []string{"vfs-cache-mode=writes", "dir-cache-time=5m", "attr-timeout=1m", "poll-interval=5s", "vfs-write-back=1s", "vfs-cache-max-age=10m", "buffer-size=64M", "transfers=8", "checkers=8"}
)

func effectiveVFSCacheMode(mode string) string {
	if mode == "" {
		return "writes"
	}
	return mode
}

func s3MountOptionsForCacheMode(mode string) []string {
	options := slices.Clone(s3MountOptions)
	if effectiveVFSCacheMode(mode) == "off" {
		for i, option := range options {
			if strings.HasPrefix(option, "vfs-cache-mode=") {
				options[i] = "vfs-cache-mode=off"
				break
			}
		}
	}
	return options
}

// rerenderStorageClasses rewrites every storage declaration whose rendered
// objects are not what this Gateway renders from its stored fields. A storage
// is otherwise rendered only when it is created or edited, so a change in the
// render (reclaim policy, mount options of a profile) would reach existing
// storages one manual edit at a time. One storage failing does not stop the rest.
func (g *Gateway) rerenderStorageClasses(ctx context.Context) {
	logger := log.FromContext(ctx)
	secrets, err := listStorageClassSecrets(ctx, g.rawClient, g.managementNamespace())
	if err != nil {
		logger.Error(err, "list storage class replications to re-render")
		return
	}
	for i := range secrets {
		secret := &secrets[i]
		id, stored, err := storageClassFromSecret(secret)
		if err != nil {
			logger.Error(err, "skip re-rendering storage class", "secret", secret.Name)
			continue
		}
		rebuilt, err := buildAstribotStorageReplicationSecret(secret.Namespace, id, stored)
		if err != nil {
			logger.Error(err, "re-render storage class", "storage", stored.Name)
			continue
		}
		if err := g.syncStorageEndpointSecrets(ctx, id, &stored, secret); err != nil {
			logger.Error(err, "re-render storage endpoint declarations", "storage", stored.Name)
		}
		if storageClassRenderKey(secret.Data) == storageClassRenderKey(rebuilt.Data) {
			continue
		}
		secret.Data = rebuilt.Data
		if _, err := g.rawClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{}); err != nil {
			logger.Error(err, "update re-rendered storage class", "storage", stored.Name)
			continue
		}
		logger.Info("re-rendered storage class", "storage", stored.Name, "storageClass", id)
	}
}

// describeAstribotStorage fills a listed storage's fields of this site. It
// answers the request and returns false when the stored mount policy is invalid.
func describeAstribotStorage(c *gin.Context, entry *StorageClassData, id string, stored storedStorageClass) bool {
	entry.Type, entry.MountEndpoint, entry.Prefix, entry.VFSCacheMode = stored.Type, stored.MountEndpoint, stored.Prefix, effectiveVFSCacheMode(stored.VFSCacheMode)
	entry.Host, entry.Share, entry.SubPath, entry.Username, entry.Domain = stored.Host, stored.Share, stored.SubPath, stored.Username, stored.Domain
	entry.Path, entry.FSType, entry.ClusterMountEndpoints = stored.Path, stored.FSType, stored.ClusterMountEndpoints
	view, err := storageView(id, stored)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "invalid storage mount policy")
		return false
	}
	entry.Profiles, entry.DefaultProfile = view.StorageClasses, view.DefaultProfile
	if stored.Type != storageTypeS3 {
		entry.AccessKeyId = ""
		return true
	}
	entry.MountProfile = stored.MountProfile
	if entry.MountProfile == "" {
		entry.MountProfile = "default"
	}
	entry.MountOptions = append([]string{}, stored.MountOptions...)
	entry.EffectiveMountOptions, entry.EffectiveBackendOptions, _, err = resolveS3MountOptions(stored.MountProfile, stored.VFSCacheMode, stored.MountOptions)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "invalid storage mount policy")
		return false
	}
	return true
}

// borrowStorageCredentials gives a new storage the provider, endpoint, region
// and access key of the storage its credentials_from names.
func (g *Gateway) borrowStorageCredentials(c *gin.Context, req *CreateStorageClassRequest) bool {
	if req.CredentialsFrom == "" {
		return true
	}
	secret, err := getStorageClassSecretByName(c.Request.Context(), g.rawClient, g.managementNamespace(), req.CredentialsFrom)
	if err != nil {
		respondKubeError(c, fmt.Sprintf("get storage class %q: %v", req.CredentialsFrom, err), err)
		return false
	}
	_, source, err := storageClassFromSecret(secret)
	if err != nil {
		respondKubeError(c, fmt.Sprintf("decode storage class replication: %v", err), err)
		return false
	}
	req.Provider, req.Endpoint, req.Region, req.PathStyle = source.Provider, source.Endpoint, source.Region, source.PathStyle
	req.AccessKeyId, req.AccessKeySecret = source.AccessKeyID, source.AccessKey
	return true
}

// storageNameFree answers 409 when a storage already has the name: volumes
// name a storage by its name.
func (g *Gateway) storageNameFree(c *gin.Context, name string) bool {
	_, err := getStorageClassSecretByName(c.Request.Context(), g.rawClient, g.managementNamespace(), name)
	if err == nil {
		respondError(c, http.StatusConflict, CodeResourceAlreadyExists, fmt.Sprintf("storage %q already exists", name))
		return false
	}
	if !errors.IsNotFound(err) {
		respondKubeError(c, fmt.Sprintf("get storage class replication: %v", err), err)
		return false
	}
	return true
}

// prepareStorageUpdate reads the declaration an update replaces, once, in
// place of the official name check and access-key lookup: the type cannot
// change; a different name in the body renames the storage (it keeps its id,
// so its StorageClasses keep their names, volumes name it by the new name, and
// Pods that already run keep their mounts); what the body omits keeps its
// stored value. It returns the declaration as it was.
func (g *Gateway) prepareStorageUpdate(c *gin.Context, name string, req *CreateStorageClassRequest) (*v1.Secret, bool) {
	secret, err := getStorageClassSecretByName(c.Request.Context(), g.rawClient, g.managementNamespace(), name)
	if err != nil {
		if errors.IsNotFound(err) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "storage class not found")
			return nil, false
		}
		respondKubeError(c, fmt.Sprintf("get storage class replication: %v", err), err)
		return nil, false
	}
	_, existing, err := storageClassFromSecret(secret)
	if err != nil {
		respondKubeError(c, fmt.Sprintf("decode storage class replication: %v", err), err)
		return nil, false
	}
	if existing.Type != normalizeStorageType(req.Type) {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "storage class type cannot be changed")
		return nil, false
	}
	if renamed := strings.TrimSpace(req.Name); renamed != name && !g.storageNameFree(c, renamed) {
		return nil, false
	}
	switch existing.Type {
	case storageTypeSMB:
		if req.Password == "" {
			req.Password = existing.Password
		}
	case storageTypeS3:
		if req.AccessKeyId == "" || isMaskedStorageAccessKeyID(req.AccessKeyId) {
			req.AccessKeyId = existing.AccessKeyID
		}
		if req.AccessKeySecret == "" {
			req.AccessKeySecret = existing.AccessKey
		}
		if req.MountOptions == nil {
			options := append([]string{}, existing.MountOptions...)
			req.MountOptions = &options
		}
		if req.VFSCacheMode == "" && (req.MountProfile == "" || req.MountProfile == existing.MountProfile) {
			req.VFSCacheMode = effectiveVFSCacheMode(existing.VFSCacheMode)
		}
		if req.MountProfile == "" {
			req.MountProfile = existing.MountProfile
		}
		if req.ClusterMountEndpoints == nil {
			endpoints := maps.Clone(existing.ClusterMountEndpoints)
			req.ClusterMountEndpoints = &endpoints
		}
	}
	return secret, true
}

// storageDeclarationWritten follows a created, updated or deleted (stored is
// nil) primary declaration: the object clients cached under names are dropped
// and the storage's endpoint declarations are made the rendered ones, owned as
// identity is.
func (g *Gateway) storageDeclarationWritten(c *gin.Context, identity *v1.Secret, stored *storedStorageClass, names ...string) bool {
	g.storageClientsMu.Lock()
	for _, name := range names {
		delete(g.storageClients, name)
	}
	g.storageClientsMu.Unlock()
	id, _ := parseStorageClassID(identity.Name)
	if err := g.syncStorageEndpointSecrets(c.Request.Context(), id, stored, identity); err != nil {
		respondKubeError(c, err.Error(), err)
		return false
	}
	return true
}

func checkStorageType(value string) error {
	switch normalizeStorageType(value) {
	case storageTypeS3, storageTypeSMB, storageTypeHostPath:
		return nil
	}
	return fmt.Errorf("unsupported storage type %q", value)
}

// requiredAstribotStorageFields lists the missing fields of a storage that is
// not S3, besides its name.
func requiredAstribotStorageFields(req CreateStorageClassRequest) []string {
	var fields []string
	if normalizeStorageType(req.Type) == storageTypeHostPath {
		if strings.TrimSpace(req.Path) == "" {
			fields = append(fields, "path")
		}
		return fields
	}
	for _, field := range []struct{ name, value string }{
		{"host", req.Host}, {"share", req.Share}, {"username", req.Username}, {"password", req.Password},
	} {
		if strings.TrimSpace(field.value) == "" {
			fields = append(fields, field.name)
		}
	}
	return fields
}

// validateAstribotStorage checks a storage that is not S3.
func validateAstribotStorage(ctx context.Context, stored storedStorageClass) error {
	if stored.Type == storageTypeSMB {
		return validateSMBReachable(ctx, stored.Host)
	}
	// A host-path directory lives on the nodes; a Pod that cannot mount it
	// reports FailedMount on its node.
	return nil
}

func storageCredentialErrorMessage(err error) string {
	if strings.Contains(err.Error(), "connect smb server") {
		return "smb server is not reachable on port " + smbDialPort
	}
	return "storage credentials cannot access the configured bucket"
}

// normalizeAstribotStorage normalizes a storage that is not S3 (ours is then
// true), and rejects the fields of another type on any storage.
func normalizeAstribotStorage(req CreateStorageClassRequest, name string, clusters []string) (stored storedStorageClass, ours bool, err error) {
	storageType := normalizeStorageType(req.Type)
	if storageType != storageTypeHostPath && (strings.TrimSpace(req.Path) != "" || strings.TrimSpace(req.FSType) != "") {
		return storedStorageClass{}, false, fmt.Errorf("path and fs_type are only supported for host-path storage")
	}
	if storageType != storageTypeS3 && req.ClusterMountEndpoints != nil && len(*req.ClusterMountEndpoints) != 0 {
		return storedStorageClass{}, false, fmt.Errorf("cluster_mount_endpoints is only supported for S3 storage")
	}
	switch storageType {
	case storageTypeHostPath:
		stored, err = normalizeHostPathStorage(req, name, clusters)
		return stored, true, err
	case storageTypeSMB:
		if strings.TrimSpace(req.MountEndpoint) != "" {
			return storedStorageClass{}, false, fmt.Errorf("mount_endpoint is only supported for S3 storage")
		}
		if strings.TrimSpace(req.Prefix) != "" || strings.TrimSpace(req.VFSCacheMode) != "" || req.MountProfile != "" || req.MountOptions != nil {
			return storedStorageClass{}, false, fmt.Errorf("prefix and vfs_cache_mode are only supported for S3 storage")
		}
		stored = storedStorageClass{Version: storageClassVersion, Name: name, Description: req.Description, Clusters: clusters}
		if err := normalizeSMBFields(req, &stored); err != nil {
			return storedStorageClass{}, false, err
		}
		return stored, true, nil
	}
	return storedStorageClass{}, false, nil
}

// normalizeS3StorageMount adds to an S3 storage the official flow normalized
// how it is mounted: endpoints, prefix, profile and mount options.
func normalizeS3StorageMount(req CreateStorageClassRequest, stored storedStorageClass) (storedStorageClass, error) {
	mountEndpoint := strings.TrimSpace(req.MountEndpoint)
	if mountEndpoint != "" {
		var err error
		mountEndpoint, err = storage.NormalizeEndpoint(stored.Provider, mountEndpoint, req.Region)
		if err != nil {
			return storedStorageClass{}, fmt.Errorf("mount_endpoint: %w", err)
		}
	}
	prefix := strings.Trim(strings.TrimSpace(req.Prefix), "/")
	if strings.Contains(prefix, "\\") || strings.Contains(prefix, "//") || strings.IndexFunc(prefix, unicode.IsControl) >= 0 {
		return storedStorageClass{}, fmt.Errorf("prefix must be a slash-separated object path")
	}
	for _, segment := range strings.Split(prefix, "/") {
		if segment == "." || segment == ".." {
			return storedStorageClass{}, fmt.Errorf("prefix must not contain dot segments")
		}
	}
	profile := strings.TrimSpace(req.MountProfile)
	if profile == "" {
		profile = "default"
	}
	var overrides []string
	if req.MountOptions != nil {
		overrides = append([]string{}, (*req.MountOptions)...)
	}
	_, _, cacheMode, err := resolveS3MountOptions(profile, strings.TrimSpace(req.VFSCacheMode), overrides)
	if err != nil {
		return storedStorageClass{}, err
	}
	// A volume may choose any profile, so the storage's mount options must
	// combine with each of them.
	for _, other := range storageref.Profiles(storageTypeS3) {
		if other == profile {
			continue
		}
		if _, _, _, err := resolveS3MountOptions(other, "", overrides); err != nil {
			return storedStorageClass{}, fmt.Errorf("mount_options with profile %s: %w", other, err)
		}
	}
	var clusterEndpoints map[string]string
	if req.ClusterMountEndpoints != nil && len(*req.ClusterMountEndpoints) != 0 {
		clusterEndpoints = make(map[string]string, len(*req.ClusterMountEndpoints))
		for cluster, endpoint := range *req.ClusterMountEndpoints {
			if !slices.Contains(stored.Clusters, cluster) {
				return storedStorageClass{}, fmt.Errorf("cluster_mount_endpoints names cluster %q, which is not one of the storage's clusters", cluster)
			}
			normalized, err := storage.NormalizeEndpoint(stored.Provider, strings.TrimSpace(endpoint), req.Region)
			if err != nil || strings.TrimSpace(endpoint) == "" {
				return storedStorageClass{}, fmt.Errorf("cluster_mount_endpoints[%s]: endpoint must be an http or https URL", cluster)
			}
			clusterEndpoints[cluster] = normalized
		}
	}
	stored.Type, stored.MountEndpoint, stored.ClusterMountEndpoints = storageTypeS3, mountEndpoint, clusterEndpoints
	stored.Prefix, stored.VFSCacheMode, stored.MountProfile, stored.MountOptions = prefix, cacheMode, profile, overrides
	return stored, nil
}
