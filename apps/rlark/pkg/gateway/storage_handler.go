package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/rlinf/rlark/apps/rlark/pkg/gateway/storage"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"
)

// StorageClassData 定义StorageClass的响应数据结构.
type StorageClassData struct {
	Name        string   `json:"name"`
	Clusters    []string `json:"clusters"`
	Description string   `json:"description"`
	Bucket      string   `json:"bucket"`
	Provider    string   `json:"provider"`
	Endpoint    string   `json:"endpoint"`
	Region      string   `json:"region"`
	PathStyle   bool     `json:"pathStyle"`
	AccessKeyId string   `json:"accessKeyId"`
	TenantID    string   `json:"tenantID"`
	CreatedBy   string   `json:"createdBy,omitempty"`
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
		_, stored, err := storageClassFromSecret(&secrets[i])
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
		data[stored.Name] = &StorageClassData{
			Name: stored.Name, Clusters: clusters, Description: stored.Description, Bucket: stored.Bucket,
			Provider: stored.Provider, Endpoint: stored.Endpoint, Region: stored.Region, PathStyle: stored.PathStyle,
			AccessKeyId: stored.AccessKeyID, TenantID: tenant.IDOf(&secrets[i]), CreatedBy: secrets[i].Annotations[tenant.CreatedByAnnotation],
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": data, "success": true})
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

// listProvider 列出支持的存储提供商.
func (g *Gateway) listProvider(c *gin.Context) {
	providers := []Provider{
		{Name: "AWS S3", Value: "AWS"},
		{Name: "阿里云 OSS", Value: "Alibaba"},
		{Name: "腾讯云 COS", Value: "TencentCOS"},
		{Name: "华为云 OBS", Value: "HuaweiOBS"},
		{Name: "七牛云 Kodo", Value: "Qiniu"},
		{Name: "移动云 EOS", Value: "ChinaMobile"},
		{Name: "网易 NOS", Value: "Netease"},
		{Name: "UCloud US3", Value: "Ucloud"},
		{Name: "MinIO", Value: "Minio"},
		{Name: "Ceph RGW", Value: "Ceph"},
		{Name: "SeaweedFS", Value: "SeaweedFS"},
		{Name: "Cloudflare R2", Value: "Cloudflare"},
		{Name: "Google Cloud Storage", Value: "GCS"},
		{Name: "DigitalOcean Spaces", Value: "DigitalOcean"},
		{Name: "Wasabi", Value: "Wasabi"},
		{Name: "Backblaze B2 (S3)", Value: "Other"},
		{Name: "IBM COS", Value: "IBMCOS"},
		{Name: "Scaleway", Value: "Scaleway"},
		{Name: "OVHcloud", Value: "OVH"},
		{Name: "Linode / Akamai", Value: "Linode"},
		{Name: "IDrive e2", Value: "IDrive"},
		{Name: "Seagate Lyve Cloud", Value: "LyveCloud"},
		{Name: "Storj (S3 网关)", Value: "Storj"},
		{Name: "Synology C2", Value: "Synology"},
		{Name: "ArvanCloud", Value: "ArvanCloud"},
		{Name: "Hetzner", Value: "Hetzner"},
		{Name: "IONOS", Value: "IONOS"},
		{Name: "MEGA S4", Value: "Mega"},
		{Name: "RackCorp", Value: "RackCorp"},
		{Name: "Dreamhost", Value: "Dreamhost"},
		{Name: "其他 S3 兼容", Value: "Other"},
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
	storageClassVersion          = "v1"
	storageClassCreateAttempts   = 3
	storageClassNamespacePrefix  = "rlark-"
	storageClassReplicationLabel = "rlark.io/storageclass-replication"
	storageClassDeliveryLabel    = "rlark.io/storageclass-delivery"
)

var storageClassIDPattern = regexp.MustCompile(`^sc-[0-9a-f]{16}$`)

type storedStorageClass struct {
	Version     string   `json:"version"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Clusters    []string `json:"clusters"`
	Provider    string   `json:"provider"`
	Endpoint    string   `json:"endpoint"`
	AccessKeyID string   `json:"access_key_id"`
	AccessKey   string   `json:"access_key_secret"`
	Bucket      string   `json:"bucket"`
	Region      string   `json:"region"`
	PathStyle   bool     `json:"path_style"`
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
// StorageClass across one or more clusters. The S3-compatible remote config is
// stored in a per-class Secret referenced by the StorageClass.
type CreateStorageClassRequest struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Description string `json:"description"`

	Clusters []string `json:"clusters"`

	Provider        string `json:"provider"`
	Endpoint        string `json:"endpoint"`
	AccessKeyId     string `json:"access_key_id"`
	AccessKeySecret string `json:"access_key_secret"`
	Bucket          string `json:"bucket"`
	Region          string `json:"region"`
	PathStyle       bool   `json:"path_style"`
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
	stored, err := normalizeStorageClass(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
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
		c.JSON(http.StatusOK, gin.H{"success": true})
		return
	}
	respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to allocate storage class id")
}

// updateStorageClass updates the Replication declaration selected by the legacy
// name route while retaining its stable internal identity.
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
	if req.Name != name {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "storage class name cannot be changed")
		return
	}
	stored, err := normalizeStorageClass(req)
	if err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	if err := g.updateStorageClassByName(c.Request.Context(), name, stored); err != nil {
		if errors.IsNotFound(err) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "storage class not found")
			return
		}
		respondKubeError(c, fmt.Sprintf("update storage class replication: %v", err), err)
		return
	}
	g.invalidateStorageClients(name)
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
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func normalizeStorageClass(req CreateStorageClassRequest) (storedStorageClass, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" || req.Bucket == "" || req.AccessKeySecret == "" {
		return storedStorageClass{}, fmt.Errorf("name, bucket, and access_key_secret are required")
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
	if strings.EqualFold(req.Provider, "Alibaba") {
		req.Endpoint = fmt.Sprintf("%s.oss-%s.aliyuncs.com", req.Bucket, req.Region)
	}
	return storedStorageClass{
		Version: storageClassVersion, Name: name, Description: req.Description, Clusters: clusters,
		Provider: req.Provider, Endpoint: req.Endpoint, AccessKeyID: req.AccessKeyId, AccessKey: req.AccessKeySecret,
		Bucket: req.Bucket, Region: req.Region, PathStyle: req.PathStyle,
	}, nil
}

func buildStorageClassReplicationSecret(namespace, id string, stored storedStorageClass) (*v1.Secret, error) {
	req := CreateStorageClassRequest{
		Name: stored.Name, Namespace: metav1.NamespaceSystem, Description: stored.Description, Clusters: stored.Clusters,
		Provider: stored.Provider, Endpoint: stored.Endpoint, AccessKeyId: stored.AccessKeyID, AccessKeySecret: stored.AccessKey,
		Bucket: stored.Bucket, Region: stored.Region, PathStyle: stored.PathStyle,
	}
	var config strings.Builder
	if err := ssSecretTmpl.Execute(&config, req); err != nil {
		return nil, fmt.Errorf("render secret config: %w", err)
	}
	secretName := id + "-secret"
	storageSecret := v1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: req.Namespace, Labels: map[string]string{"controller": "rlark"}},
		StringData: map[string]string{"remote": "s3", "remotePath": req.Bucket, "configData": config.String()},
	}
	annotations := map[string]string{annoStorageClassDescription: req.Description}
	if problems := validation.IsValidLabelValue(req.Name); len(problems) == 0 {
		annotations[annoStorageClassName] = req.Name
	}
	storageClass := storagev1.StorageClass{
		TypeMeta: metav1.TypeMeta{APIVersion: "storage.k8s.io/v1", Kind: "StorageClass"},
		ObjectMeta: metav1.ObjectMeta{
			Name: id, Labels: map[string]string{"controller": "rlark"}, Annotations: annotations,
		},
		Provisioner: "rclone.csi.veloxpack.io", ReclaimPolicy: ptr.To(v1.PersistentVolumeReclaimRetain),
		VolumeBindingMode: ptr.To(storagev1.VolumeBindingImmediate), AllowVolumeExpansion: ptr.To(true),
		Parameters: map[string]string{
			"remote": "s3", "remotePath": req.Bucket,
			"csi.storage.k8s.io/node-publish-secret-name":      secretName,
			"csi.storage.k8s.io/node-publish-secret-namespace": req.Namespace,
		},
		MountOptions: []string{"vfs-cache-mode=writes", "dir-cache-time=5m", "attr-timeout=1m", "poll-interval=5s", "vfs-write-back=1s", "vfs-cache-max-age=10m", "buffer-size=64M", "transfers=8", "checkers=16", "fast-list"},
	}
	storageManifests, err := yaml.Marshal([]any{storageSecret, storageClass})
	if err != nil {
		return nil, fmt.Errorf("marshal storage class manifests: %w", err)
	}
	deliveryConfig := distribution.Config{Apply: distribution.ApplyPolicy{Mode: "ServerSideApply", ConflictPolicy: "Fail", DeletionPolicy: "Delete"}}
	deliveryConfig.Security.AllowClusterScoped = true
	deliveryConfigYAML, err := yaml.Marshal(deliveryConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal delivery config: %w", err)
	}
	delivery := v1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   storageClassDeliveryName(id),
			Labels: map[string]string{storageClassDeliveryLabel: "true"},
		},
		Type: distribution.DeliverySecretType,
		Data: map[string][]byte{"config": deliveryConfigYAML, "manifests": storageManifests},
	}
	deliveryManifest, err := yaml.Marshal([]v1.Secret{delivery})
	if err != nil {
		return nil, fmt.Errorf("marshal delivery secret manifest: %w", err)
	}
	replicationConfig := distribution.Config{Apply: distribution.ApplyPolicy{Mode: "ServerSideApply", ConflictPolicy: "Fail", DeletionPolicy: "Delete"}}
	replicationConfig.Targets.Namespaces = &distribution.GlobSelector{Exclude: []string{"rlark-system"}}
	replicationConfig.Targets.Namespaces.Include = append(replicationConfig.Targets.Namespaces.Include, req.Clusters...)
	replicationConfigYAML, err := yaml.Marshal(replicationConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal replication config: %w", err)
	}
	storedJSON, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("marshal storage class: %w", err)
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      storageClassReplicationName(id),
			Namespace: namespace,
			Labels:    map[string]string{storageClassReplicationLabel: "true"},
		},
		Type: distribution.ReplicationSecretType,
		Data: map[string][]byte{"storageclass": storedJSON, "config": replicationConfigYAML, "manifests": deliveryManifest},
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

func (g *Gateway) updateStorageClassByName(ctx context.Context, name string, stored storedStorageClass) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		secret, err := getStorageClassSecretByName(ctx, g.rawClient, g.managementNamespace(), name)
		if err != nil {
			return err
		}
		id, _, err := storageClassFromSecret(secret)
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
}

// --- Storage class file operation handlers ---

// getStorageClient resolves the storage.Client from the management-plane
// StorageClass declaration rather than reading data-plane resources.
func (g *Gateway) invalidateStorageClients(name string) {
	g.storageClientsMu.Lock()
	defer g.storageClientsMu.Unlock()
	for cacheKey := range g.storageClients {
		if strings.HasSuffix(cacheKey, "/"+name) {
			delete(g.storageClients, cacheKey)
		}
	}
}

func (g *Gateway) getStorageClient(ctx context.Context, cluster, name string) (*storage.Client, error) {
	cluster = strings.TrimPrefix(strings.TrimSpace(cluster), storageClassNamespacePrefix)
	cacheKey := cluster + "/" + name
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
	if !slices.Contains(stored.Clusters, cluster) {
		return nil, fmt.Errorf("storage class %s is not configured for cluster %s", name, cluster)
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

// listStorageClassFiles handles GET /api/v1/storage/storageclass/:cluster/:name/list
// with query parameters: prefix, delimiter, marker, maxKeys.
func (g *Gateway) listStorageClassFiles(c *gin.Context) {
	cluster := c.Param("cluster")
	name := c.Param("name")

	client, err := g.getStorageClient(c.Request.Context(), cluster, name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
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

// uploadStorageClassFile handles POST /api/v1/storage/storageclass/:cluster/:name/upload.
func (g *Gateway) uploadStorageClassFile(c *gin.Context) {
	cluster := c.Param("cluster")
	name := c.Param("name")

	client, err := g.getStorageClient(c.Request.Context(), cluster, name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
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

// getStorageClassObject handles GET /api/v1/storage/storageclass/:cluster/:name/object/*key
// It returns a presigned URL for downloading the object.
func (g *Gateway) getStorageClassObject(c *gin.Context) {
	cluster := c.Param("cluster")
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

	client, err := g.getStorageClient(c.Request.Context(), cluster, name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
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

// deleteStorageClassObject handles DELETE /api/v1/storage/storageclass/:cluster/:name/object/*key.
func (g *Gateway) deleteStorageClassObject(c *gin.Context) {
	cluster := c.Param("cluster")
	name := c.Param("name")
	key := strings.TrimPrefix(c.Param("key"), "/")
	if key == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "object key is required")
		return
	}

	client, err := g.getStorageClient(c.Request.Context(), cluster, name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
		return
	}

	err = client.DeleteObject(key)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Object deleted successfully"})
}
