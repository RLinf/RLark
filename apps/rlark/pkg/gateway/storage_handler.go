package gateway

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/rlinf/rlark/apps/rlark/pkg/gateway/storage"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

// StorageClassData 定义StorageClass的响应数据结构.
type StorageClassData struct {
	Name                    string            `json:"name"`
	StorageClassName        string            `json:"storageClassName"`
	Clusters                []string          `json:"clusters"`
	Description             string            `json:"description"`
	Type                    string            `json:"type"`
	Bucket                  string            `json:"bucket"`
	Provider                string            `json:"provider"`
	Endpoint                string            `json:"endpoint"`
	MountEndpoint           string            `json:"mount_endpoint,omitempty"`
	ClusterMountEndpoints   map[string]string `json:"cluster_mount_endpoints,omitempty"`
	Prefix                  string            `json:"prefix,omitempty"`
	VFSCacheMode            string            `json:"vfs_cache_mode,omitempty"`
	MountProfile            string            `json:"mount_profile,omitempty"`
	MountOptions            []string          `json:"mount_options"`
	EffectiveMountOptions   []string          `json:"effective_mount_options"`
	EffectiveBackendOptions map[string]string `json:"effective_backend_options,omitempty"`
	Region                  string            `json:"region"`
	PathStyle               bool              `json:"pathStyle"`
	AccessKeyId             string            `json:"accessKeyId"`
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
	TenantID       string            `json:"tenantID"`
	CreatedBy      string            `json:"createdBy,omitempty"`
	CreatedAt      string            `json:"createdAt"`
}

// Provider 定义存储提供商信息.
type Provider struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// listStorageClass reads StorageClass declarations from the management plane.
// The distribution controllers own delivery to the data plane, so listing does
// not need to contact cluster agents.
func (g *Gateway) listStorageClass(c *gin.Context) {
	secrets, err := listStorageClassSecrets(c.Request.Context(), g.rawClient, g.managementNamespace())
	if err != nil {
		respondKubeError(c, fmt.Sprintf("list storage class replications: %v", err), err)
		return
	}
	requestedClusters := parseStorageClassClusters(c.Query("clusters"))
	data := make(map[string]*StorageClassData, len(secrets))
	for i := range secrets {
		id, stored, err := storageClassFromSecret(&secrets[i])
		if err != nil {
			log.FromContext(c.Request.Context()).Error(err, "invalid storage class replication", "secret", secrets[i].Name)
			respondError(c, http.StatusInternalServerError, CodeInternalError, "invalid storage class data")
			return
		}
		clusters := stored.Clusters
		if len(requestedClusters) != 0 {
			clusters = intersectStorageClassClusters(clusters, requestedClusters)
			if len(clusters) == 0 {
				continue
			}
		}
		entry := &StorageClassData{
			Name: stored.Name, StorageClassName: id, Clusters: clusters, Description: stored.Description, Type: stored.Type,
			Bucket: stored.Bucket, Provider: stored.Provider, Endpoint: stored.Endpoint, MountEndpoint: stored.MountEndpoint, Prefix: stored.Prefix, VFSCacheMode: effectiveVFSCacheMode(stored.VFSCacheMode), Region: stored.Region, PathStyle: stored.PathStyle,
			Host: stored.Host, Share: stored.Share, SubPath: stored.SubPath, Username: stored.Username, Domain: stored.Domain,
			TenantID: tenant.IDOf(&secrets[i]), CreatedBy: secrets[i].Annotations[tenant.CreatedByAnnotation], CreatedAt: secrets[i].CreationTimestamp.UTC().Format(time.RFC3339),
		}
		entry.Path, entry.FSType, entry.ClusterMountEndpoints = stored.Path, stored.FSType, stored.ClusterMountEndpoints
		view, err := storageView(id, stored)
		if err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError, "invalid storage mount policy")
			return
		}
		entry.Profiles, entry.DefaultProfile = view.StorageClasses, view.DefaultProfile
		if stored.Type == storageTypeS3 {
			entry.AccessKeyId = maskStorageAccessKeyID(stored.AccessKeyID)
			entry.MountProfile = stored.MountProfile
			if entry.MountProfile == "" {
				entry.MountProfile = "default"
			}
			entry.MountOptions = append([]string{}, stored.MountOptions...)
			entry.EffectiveMountOptions, entry.EffectiveBackendOptions, _, err = resolveS3MountOptions(stored.MountProfile, stored.VFSCacheMode, stored.MountOptions)
			if err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError, "invalid storage mount policy")
				return
			}
		}
		data[stored.Name] = entry
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "success": true})
}

const storageAccessKeyMask = "*****"

func maskStorageAccessKeyID(value string) string {
	if len(value) <= 4 {
		return storageAccessKeyMask
	}
	return value[:2] + storageAccessKeyMask + value[len(value)-2:]
}

func isMaskedStorageAccessKeyID(value string) bool {
	return strings.Contains(value, storageAccessKeyMask)
}

func parseStorageClassClusters(value string) map[string]struct{} {
	clusters := map[string]struct{}{}
	for _, cluster := range strings.Split(value, ",") {
		cluster = strings.TrimSpace(cluster)
		if cluster != "" {
			clusters[cluster] = struct{}{}
		}
	}
	return clusters
}

func intersectStorageClassClusters(clusters []string, requested map[string]struct{}) []string {
	filtered := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		if _, ok := requested[cluster]; ok {
			filtered = append(filtered, cluster)
		}
	}
	return filtered
}

// listProvider 列出支持的存储提供商. 目录由 storage.Providers 单一维护,
// 与创建/更新时的 provider 校验共用同一份数据, 避免客户端提交 rclone 无法
// 识别的取值.
func (g *Gateway) listProvider(c *gin.Context) {
	providers := make([]Provider, 0, len(storage.Providers))
	for _, provider := range storage.Providers {
		providers = append(providers, Provider{Name: provider.DisplayName, Value: provider.Value})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":    providers,
		"success": true,
	})
}

// annoStorageClassDescription is the annotation key under which a human-readable
// description of a rlark-managed StorageClass is stored.
const annoStorageClassDescription = "rlark.io/description"

const annoStorageClassName = "rlark.io/name"

const (
	storageClassVersion           = "v1"
	storageClassCreateAttempts    = 3
	storageClassValidationTimeout = 10 * time.Second
	storageClassNamespacePrefix   = "rlark-"
	storageClassReplicationLabel  = "rlark.io/storageclass-replication"

	storageClassDeliveryLabel = "rlark.io/storageclass-delivery"

	CodeStorageClassRequiredFields = "STORAGE_CLASS_REQUIRED_FIELDS"
)

var storageClassIDPattern = regexp.MustCompile(`^sc-[0-9a-f]{16}$`)

type storedStorageClass struct {
	Version     string   `json:"version"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Clusters    []string `json:"clusters"`
	// Type is "s3" or "smb". Records written before SMB support omit it and
	// are read back as "s3"; all new fields are optional to stay on v1.
	Type          string `json:"type,omitempty"`
	Provider      string `json:"provider"`
	Endpoint      string `json:"endpoint"`
	MountEndpoint string `json:"mount_endpoint,omitempty"`
	// ClusterMountEndpoints gives a cluster its own mount endpoint, for
	// example a cloud's internal address for the clusters in that cloud.
	ClusterMountEndpoints map[string]string `json:"cluster_mount_endpoints,omitempty"`
	Prefix                string            `json:"prefix,omitempty"`
	VFSCacheMode          string            `json:"vfs_cache_mode,omitempty"`
	MountProfile          string            `json:"mount_profile,omitempty"`
	MountOptions          []string          `json:"mount_options,omitempty"`
	AccessKeyID           string            `json:"access_key_id"`
	AccessKey             string            `json:"access_key_secret"`
	Bucket                string            `json:"bucket"`
	Region                string            `json:"region"`
	PathStyle             bool              `json:"path_style"`
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

func newStorageClassID() string {
	return "sc-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
}

func storageClassReplicationName(id string) string { return id + "-replication" }
func storageClassDeliveryName(id string) string    { return id + "-delivery" }

func parseStorageClassID(name string) (string, bool) {
	const suffix = "-replication"
	if !strings.HasSuffix(name, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(name, suffix)
	return id, storageClassIDPattern.MatchString(id)
}

// CreateStorageClassRequest defines the payload for creating a rclone-backed
// StorageClass across one or more clusters. Type selects the rclone remote:
// "s3" (default) for S3-compatible object storage or "smb" for an SMB/CIFS
// share. The remote config is stored in a per-class Secret referenced by the
// StorageClass.
type CreateStorageClassRequest struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`

	Clusters []string `json:"clusters"`

	Provider      string `json:"provider"`
	Endpoint      string `json:"endpoint"`
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
	AccessKeyId     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	PathStyle       bool   `json:"path_style"`

	Type     string `json:"type"`
	Host     string `json:"host"`
	Share    string `json:"share"`
	SubPath  string `json:"sub_path"`
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain"`

	Path   string `json:"path"`
	FSType string `json:"fs_type"`
}

// ssSecretTemplate is the rclone remote config rendered into the StorageClass
// Secret from a CreateStorageClassRequest.
const ssSecretTemplate = `
[s3]
type = s3
provider = {{ .Provider }}
endpoint = {{ .Endpoint }}
access_key_id = {{ .AccessKeyId }}
secret_access_key = {{ .AccessKeySecret }}
{{- if .Region }}
region = {{ .Region}}
{{- end }}
force_path_style = {{ .PathStyle }}
no_check_bucket = true
`

var ssSecretTmpl = template.Must(template.New("ss-secret").Parse(ssSecretTemplate))

// createStorageClass stores a Replication declaration. The Replication and
// Delivery controllers apply the StorageClass bundle to selected clusters.
func (g *Gateway) createStorageClass(c *gin.Context) {
	var req CreateStorageClassRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	if req.CredentialsFrom != "" {
		secret, err := getStorageClassSecretByName(c.Request.Context(), g.rawClient, g.managementNamespace(), req.CredentialsFrom)
		if err != nil {
			respondKubeError(c, fmt.Sprintf("get storage class %q: %v", req.CredentialsFrom, err), err)
			return
		}
		_, source, err := storageClassFromSecret(secret)
		if err != nil {
			respondKubeError(c, fmt.Sprintf("decode storage class replication: %v", err), err)
			return
		}
		req.Provider, req.Endpoint, req.Region, req.PathStyle = source.Provider, source.Endpoint, source.Region, source.PathStyle
		req.AccessKeyId, req.AccessKeySecret = source.AccessKeyID, source.AccessKey
	}
	stored, err := normalizeStorageClass(req)
	if err != nil {
		respondStorageClassValidationError(c, err)
		return
	}
	if err := validateStorageClassCredentials(c.Request.Context(), stored); err != nil {
		respondStorageClassCredentialValidationError(c, err)
		return
	}
	if _, err := getStorageClassSecretByName(c.Request.Context(), g.rawClient, g.managementNamespace(), stored.Name); err == nil {
		respondError(c, http.StatusConflict, CodeResourceAlreadyExists, fmt.Sprintf("storage %q already exists", stored.Name))
		return
	} else if !errors.IsNotFound(err) {
		respondKubeError(c, fmt.Sprintf("get storage class replication: %v", err), err)
		return
	}
	for range storageClassCreateAttempts {
		id := newStorageClassID()
		secret, err := buildStorageClassReplicationSecret(g.managementNamespace(), id, stored)
		if err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
			return
		}
		tenant.SetIdentity(secret, tenant.DefaultID, c.GetString(authSubjectKey))
		if _, err := g.rawClient.CoreV1().Secrets(secret.Namespace).Create(c.Request.Context(), secret, metav1.CreateOptions{}); err != nil {
			if errors.IsAlreadyExists(err) {
				continue
			}
			respondKubeError(c, fmt.Sprintf("create storage class replication: %v", err), err)
			return
		}
		if err := g.syncStorageEndpointSecrets(c.Request.Context(), id, &stored, secret); err != nil {
			respondKubeError(c, err.Error(), err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to allocate storage class id")
}

// updateStorageClass updates the declaration selected by name while keeping
// its stable id, so its StorageClasses keep their names. A different name in
// the body renames the storage: volumes then name it by the new name, and
// Pods that already run keep their mounts.
func (g *Gateway) updateStorageClass(c *gin.Context) {
	name := c.Param("name")
	var req CreateStorageClassRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	if req.Name == "" {
		req.Name = name
	}
	secret, err := getStorageClassSecretByName(c.Request.Context(), g.rawClient, g.managementNamespace(), name)
	if err != nil {
		if errors.IsNotFound(err) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "storage class not found")
			return
		}
		respondKubeError(c, fmt.Sprintf("get storage class replication: %v", err), err)
		return
	}
	_, existing, err := storageClassFromSecret(secret)
	if err != nil {
		respondKubeError(c, fmt.Sprintf("decode storage class replication: %v", err), err)
		return
	}
	if existing.Type != normalizeStorageType(req.Type) {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "storage class type cannot be changed")
		return
	}
	if strings.TrimSpace(req.Name) != name {
		if _, err := getStorageClassSecretByName(c.Request.Context(), g.rawClient, g.managementNamespace(), strings.TrimSpace(req.Name)); err == nil {
			respondError(c, http.StatusConflict, CodeResourceAlreadyExists, fmt.Sprintf("storage %q already exists", strings.TrimSpace(req.Name)))
			return
		} else if !errors.IsNotFound(err) {
			respondKubeError(c, fmt.Sprintf("get storage class replication: %v", err), err)
			return
		}
	}
	switch existing.Type {
	case storageTypeSMB:
		// An empty password on update keeps the stored one.
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
	stored, err := normalizeStorageClass(req)
	if err != nil {
		respondStorageClassValidationError(c, err)
		return
	}
	if err := validateStorageClassCredentials(c.Request.Context(), stored); err != nil {
		respondStorageClassCredentialValidationError(c, err)
		return
	}
	id, err := g.updateStorageClassByName(c.Request.Context(), name, stored)
	if err != nil {
		if errors.IsNotFound(err) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "storage class not found")
			return
		}
		respondKubeError(c, fmt.Sprintf("update storage class replication: %v", err), err)
		return
	}
	g.invalidateStorageClients(name)
	g.invalidateStorageClients(stored.Name)
	if err := g.syncStorageEndpointSecrets(c.Request.Context(), id, &stored, secret); err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// deleteStorageClass deletes the Replication declaration selected by the legacy
// name route. Distribution finalizers clean up Delivery declarations and local objects.
func (g *Gateway) deleteStorageClass(c *gin.Context) {
	name := c.Param("name")
	secret, err := getStorageClassSecretByName(c.Request.Context(), g.rawClient, g.managementNamespace(), name)
	if err != nil {
		if errors.IsNotFound(err) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "storage class not found")
			return
		}
		respondKubeError(c, fmt.Sprintf("get storage class replication: %v", err), err)
		return
	}
	if err := g.rawClient.CoreV1().Secrets(secret.Namespace).Delete(c.Request.Context(), secret.Name, metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "storage class not found")
			return
		}
		respondKubeError(c, fmt.Sprintf("delete storage class replication: %v", err), err)
		return
	}
	g.invalidateStorageClients(name)
	id, _, _ := storageClassFromSecret(secret)
	if err := g.syncStorageEndpointSecrets(c.Request.Context(), id, nil, secret); err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

type storageClassValidationError struct {
	fields []string
}

func (e *storageClassValidationError) Error() string {
	return strings.Join(e.fields, ", ") + " are required"
}

func requiredStorageClassFields(req CreateStorageClassRequest) []string {
	var fields []string
	if strings.TrimSpace(req.Name) == "" {
		fields = append(fields, "name")
	}
	if normalizeStorageType(req.Type) == storageTypeHostPath {
		if strings.TrimSpace(req.Path) == "" {
			fields = append(fields, "path")
		}
		return fields
	}
	if normalizeStorageType(req.Type) == storageTypeSMB {
		for _, field := range []struct{ name, value string }{
			{"host", req.Host}, {"share", req.Share}, {"username", req.Username}, {"password", req.Password},
		} {
			if strings.TrimSpace(field.value) == "" {
				fields = append(fields, field.name)
			}
		}
		return fields
	}
	if strings.TrimSpace(req.Bucket) == "" {
		fields = append(fields, "bucket")
	}
	if strings.TrimSpace(req.AccessKeySecret) == "" {
		fields = append(fields, "access_key_secret")
	}
	return fields
}

func respondStorageClassValidationError(c *gin.Context, err error) {
	var validationErr *storageClassValidationError
	if stderrors.As(err, &validationErr) {
		respondErrorWithDetails(c, http.StatusBadRequest, CodeStorageClassRequiredFields, validationErr.Error(), map[string]any{"fields": validationErr.fields})
		return
	}
	respondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
}

var validateStorageClassCredentials = func(ctx context.Context, stored storedStorageClass) error {
	if stored.Type == storageTypeHostPath {
		// The directory lives on the nodes; a Pod that cannot mount it reports
		// FailedMount on its node.
		return nil
	}
	if stored.Type == storageTypeSMB {
		return validateSMBReachable(ctx, stored.Host)
	}
	config := storage.DefaultConfig()
	config.Provider = stored.Provider
	config.Endpoint = stored.Endpoint
	config.AccessKeyId = stored.AccessKeyID
	config.SecretAccessKey = stored.AccessKey
	config.Region = stored.Region
	config.UsePathStyle = stored.PathStyle
	config.Bucket = stored.Bucket
	client, err := storage.NewClient(config)
	if err != nil {
		return fmt.Errorf("create storage client: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, storageClassValidationTimeout)
	defer cancel()
	if err := client.ValidateBucketCredentials(ctx); err != nil {
		return fmt.Errorf("head storage bucket: %w", err)
	}
	return nil
}

func respondStorageClassCredentialValidationError(c *gin.Context, err error) {
	log.FromContext(c.Request.Context()).Error(err, "storage credential validation failed")
	message := "storage credentials cannot access the configured bucket"
	if strings.Contains(err.Error(), "connect smb server") {
		message = "smb server is not reachable on port " + smbDialPort
	}
	respondError(c, http.StatusBadRequest, CodeInvalidRequest, message)
}

func normalizeStorageClass(req CreateStorageClassRequest) (storedStorageClass, error) {
	name := strings.TrimSpace(req.Name)
	storageType := normalizeStorageType(req.Type)
	if storageType != storageTypeS3 && storageType != storageTypeSMB && storageType != storageTypeHostPath {
		return storedStorageClass{}, fmt.Errorf("unsupported storage type %q", req.Type)
	}
	if fields := requiredStorageClassFields(req); len(fields) != 0 {
		return storedStorageClass{}, &storageClassValidationError{fields: fields}
	}
	clusters := make([]string, 0, len(req.Clusters))
	for _, cluster := range req.Clusters {
		cluster = strings.TrimSpace(cluster)
		if cluster == "" {
			return storedStorageClass{}, fmt.Errorf("cluster IDs must not be empty")
		}
		if problems := validation.IsDNS1123Label(cluster); len(problems) != 0 {
			return storedStorageClass{}, fmt.Errorf("invalid cluster ID %q: %s", cluster, strings.Join(problems, "; "))
		}
		clusters = append(clusters, cluster)
	}
	if len(clusters) == 0 {
		return storedStorageClass{}, fmt.Errorf("clusters is required")
	}
	sort.Strings(clusters)
	clusters = slices.Compact(clusters)
	if storageType != storageTypeHostPath && (strings.TrimSpace(req.Path) != "" || strings.TrimSpace(req.FSType) != "") {
		return storedStorageClass{}, fmt.Errorf("path and fs_type are only supported for host-path storage")
	}
	if storageType != storageTypeS3 && req.ClusterMountEndpoints != nil && len(*req.ClusterMountEndpoints) != 0 {
		return storedStorageClass{}, fmt.Errorf("cluster_mount_endpoints is only supported for S3 storage")
	}
	if storageType == storageTypeHostPath {
		return normalizeHostPathStorage(req, name, clusters)
	}
	if storageType == storageTypeSMB {
		if strings.TrimSpace(req.MountEndpoint) != "" {
			return storedStorageClass{}, fmt.Errorf("mount_endpoint is only supported for S3 storage")
		}
		if strings.TrimSpace(req.Prefix) != "" || strings.TrimSpace(req.VFSCacheMode) != "" || req.MountProfile != "" || req.MountOptions != nil {
			return storedStorageClass{}, fmt.Errorf("prefix and vfs_cache_mode are only supported for S3 storage")
		}
		stored := storedStorageClass{Version: storageClassVersion, Name: name, Description: req.Description, Clusters: clusters}
		if err := normalizeSMBFields(req, &stored); err != nil {
			return storedStorageClass{}, err
		}
		return stored, nil
	}
	// Provider 必须来自已知目录: rclone 对未知 provider 只记录告警并回退到通用
	// 配置, 错误取值在数据面只表现为挂载后的 I/O 错误. 这里同时把历史客户端提交
	// 的展示名(如 "Aliyun OSS")改写为 canonical 取值.
	provider, ok := storage.LookupProvider(req.Provider)
	if !ok {
		return storedStorageClass{}, fmt.Errorf("unsupported provider %q", req.Provider)
	}
	// Endpoint 只描述服务端地址, 不含 bucket: AWS SDK 与 rclone 都会自行按
	// virtual-hosted 规则拼接 bucket.
	endpoint, err := storage.NormalizeEndpoint(provider.Value, req.Endpoint, req.Region)
	if err != nil {
		return storedStorageClass{}, err
	}
	mountEndpoint := strings.TrimSpace(req.MountEndpoint)
	if mountEndpoint != "" {
		mountEndpoint, err = storage.NormalizeEndpoint(provider.Value, mountEndpoint, req.Region)
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
			if !slices.Contains(clusters, cluster) {
				return storedStorageClass{}, fmt.Errorf("cluster_mount_endpoints names cluster %q, which is not one of the storage's clusters", cluster)
			}
			normalized, err := storage.NormalizeEndpoint(provider.Value, strings.TrimSpace(endpoint), req.Region)
			if err != nil || strings.TrimSpace(endpoint) == "" {
				return storedStorageClass{}, fmt.Errorf("cluster_mount_endpoints[%s]: endpoint must be an http or https URL", cluster)
			}
			clusterEndpoints[cluster] = normalized
		}
	}

	return storedStorageClass{
		Version: storageClassVersion, Name: name, Description: req.Description, Clusters: clusters, Type: storageTypeS3,
		Provider: provider.Value, Endpoint: endpoint, MountEndpoint: mountEndpoint, ClusterMountEndpoints: clusterEndpoints, Prefix: prefix, VFSCacheMode: cacheMode, MountProfile: profile, MountOptions: overrides, AccessKeyID: req.AccessKeyId, AccessKey: req.AccessKeySecret,
		Bucket: req.Bucket, Region: strings.TrimSpace(req.Region),
		PathStyle: storage.EffectivePathStyle(provider.Value, req.PathStyle),
	}, nil
}

func storageClassFromSecret(secret *v1.Secret) (string, storedStorageClass, error) {
	id, ok := parseStorageClassID(secret.Name)
	if !ok || secret.Labels[storageClassReplicationLabel] != "true" || secret.Type != distribution.ReplicationSecretType {
		return "", storedStorageClass{}, fmt.Errorf("secret %q is not a storage class replication", secret.Name)
	}
	var stored storedStorageClass
	if err := json.Unmarshal(secret.Data["storageclass"], &stored); err != nil {
		return "", storedStorageClass{}, fmt.Errorf("decode stored storage class: %w", err)
	}
	if stored.Version != storageClassVersion {
		return "", storedStorageClass{}, fmt.Errorf("unsupported storage class version %q", stored.Version)
	}
	stored.Type = normalizeStorageType(stored.Type)
	if stored.Type != storageTypeS3 {
		return id, stored, nil
	}
	// 读取历史记录时把 provider 的展示名改写为 canonical 取值, 并按该 provider
	// 实际支持的寻址方式修正 path_style, 使旧记录与新写入的记录行为一致.
	if provider, ok := storage.LookupProvider(stored.Provider); ok {
		stored.Provider = provider.Value
		stored.PathStyle = storage.EffectivePathStyle(provider.Value, stored.PathStyle)
	}
	return id, stored, nil
}

func listStorageClassSecrets(ctx context.Context, rawClient kubernetes.Interface, namespace string) ([]v1.Secret, error) {
	secrets, err := rawClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{storageClassReplicationLabel: "true"}.AsSelector().String(),
	})
	if err != nil {
		return nil, err
	}
	return secrets.Items, nil
}

func getStorageClassSecretByName(ctx context.Context, rawClient kubernetes.Interface, namespace, name string) (*v1.Secret, error) {
	secrets, err := listStorageClassSecrets(ctx, rawClient, namespace)
	if err != nil {
		return nil, err
	}
	for i := range secrets {
		_, stored, err := storageClassFromSecret(&secrets[i])
		if err != nil {
			continue
		}
		if stored.Name == name {
			return &secrets[i], nil
		}
	}
	return nil, errors.NewNotFound(v1.Resource("secrets"), name)
}

func (g *Gateway) updateStorageClassByName(ctx context.Context, name string, stored storedStorageClass) (string, error) {
	var id string
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		secret, err := getStorageClassSecretByName(ctx, g.rawClient, g.managementNamespace(), name)
		if err != nil {
			return err
		}
		id, _, err = storageClassFromSecret(secret)
		if err != nil {
			return errors.NewNotFound(v1.Resource("secrets"), secret.Name)
		}
		rebuilt, err := buildStorageClassReplicationSecret(secret.Namespace, id, stored)
		if err != nil {
			return err
		}
		secret.Data = rebuilt.Data
		_, err = g.rawClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
		return err
	})
	return id, err
}

// --- Storage class file operation handlers ---

// getStorageClient resolves the storage.Client from the management-plane
// StorageClass declaration rather than reading data-plane resources.
func (g *Gateway) invalidateStorageClients(name string) {
	g.storageClientsMu.Lock()
	defer g.storageClientsMu.Unlock()
	// getStorageClient caches by storage class name.
	delete(g.storageClients, name)
}

func (g *Gateway) getStorageClient(ctx context.Context, name string) (*storage.Client, error) {
	cacheKey := name
	g.storageClientsMu.RLock()
	if client, ok := g.storageClients[cacheKey]; ok {
		g.storageClientsMu.RUnlock()
		return client, nil
	}
	g.storageClientsMu.RUnlock()

	secret, err := getStorageClassSecretByName(ctx, g.rawClient, g.managementNamespace(), name)
	if err != nil {
		return nil, fmt.Errorf("get storage class declaration: %w", err)
	}
	_, stored, err := storageClassFromSecret(secret)
	if err != nil {
		return nil, fmt.Errorf("decode storage class declaration: %w", err)
	}
	if stored.Type != storageTypeS3 {
		return nil, errStorageBrowsingUnsupported
	}
	config := storage.DefaultConfig()
	config.Provider = stored.Provider
	config.Endpoint = stored.Endpoint
	config.AccessKeyId = stored.AccessKeyID
	config.SecretAccessKey = stored.AccessKey
	config.Region = stored.Region
	config.UsePathStyle = stored.PathStyle
	config.Bucket = stored.Bucket
	client, err := storage.NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("create storage client: %w", err)
	}
	g.storageClientsMu.Lock()
	g.storageClients[cacheKey] = client
	g.storageClientsMu.Unlock()
	return client, nil
}

// listStorageClassFiles handles GET /api/v1/storage/storageclass/:name/list
// with query parameters: prefix, delimiter, marker, maxKeys.
func (g *Gateway) listStorageClassFiles(c *gin.Context) {
	name := c.Param("name")

	client, err := g.getStorageClient(c.Request.Context(), name)
	if err != nil {
		respondStorageClientError(c, err)
		return
	}

	prefix := c.Query("prefix")
	delimiter := c.Query("delimiter")
	marker := c.Query("marker")
	maxKeysStr := c.Query("maxKeys")

	// Default to "/" so folders are grouped as common prefixes. Callers that
	// want a flat listing can explicitly pass an empty delimiter.
	if _, ok := c.GetQuery("delimiter"); !ok {
		delimiter = "/"
	}

	var maxKeys int
	if maxKeysStr != "" {
		if mk, err := strconv.Atoi(maxKeysStr); err == nil {
			maxKeys = mk
		}
	}

	options := &storage.ListOptions{
		Prefix:    prefix,
		MaxKeys:   maxKeys,
		Delimiter: delimiter,
		Marker:    marker,
	}

	result, err := client.ListObjectsWithDelimiter(options)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"objects":         result.Objects,
		"common_prefixes": result.CommonPrefixes,
		"is_truncated":    result.IsTruncated,
		"next_marker":     result.NextMarker,
		"max_keys":        result.MaxKeys,
	})
}

// uploadStorageClassFile handles POST /api/v1/storage/storageclass/:name/upload.
func (g *Gateway) uploadStorageClassFile(c *gin.Context) {
	name := c.Param("name")

	client, err := g.getStorageClient(c.Request.Context(), name)
	if err != nil {
		respondStorageClientError(c, err)
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "No file uploaded")
		return
	}
	defer utils.CloseIO(file)

	objectKey, err := client.UploadFileFromMultipart(header)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"key": objectKey})
}

// getStorageClassObject handles GET /api/v1/storage/storageclass/:name/object/*key
// It returns a presigned URL for downloading the object.
func (g *Gateway) getStorageClassObject(c *gin.Context) {
	name := c.Param("name")
	// gin's *key catch-all includes the leading slash; strip it so the object
	// key doesn't start with "/", which would produce a "//" in the presigned
	// URL path. Browsers/HTTP clients normalize "//" to "/", breaking the
	// signature and causing SignatureDoesNotMatch from OSS.
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "object key is required")
		return
	}

	client, err := g.getStorageClient(c.Request.Context(), name)
	if err != nil {
		respondStorageClientError(c, err)
		return
	}

	expireStr := c.Query("expire")
	var expireSeconds int64 = 3600
	if expireStr != "" {
		if exp, err := strconv.ParseInt(expireStr, 10, 64); err == nil {
			expireSeconds = exp
		}
	}

	url, err := client.GetObjectURL(key, expireSeconds)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

// deleteStorageClassObject handles DELETE /api/v1/storage/storageclass/:name/object/*key.
func (g *Gateway) deleteStorageClassObject(c *gin.Context) {
	name := c.Param("name")
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "object key is required")
		return
	}

	client, err := g.getStorageClient(c.Request.Context(), name)
	if err != nil {
		respondStorageClientError(c, err)
		return
	}

	err = client.DeleteObject(key)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Object deleted successfully"})
}
