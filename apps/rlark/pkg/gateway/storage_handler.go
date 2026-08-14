package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"text/template"

	v1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/gin-gonic/gin"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/gateway/storage"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
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
}

// Provider 定义存储提供商信息.
type Provider struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// listStorageClass 处理StorageClass列表请求
// 通过Server代理向指定集群的Agent请求StorageClass列表.
func (g *Gateway) listStorageClass(c *gin.Context) {
	clustersParam := c.Query("clusters")

	var clusterIDs []string
	logger := log.FromContext(c.Request.Context()).WithValues("method", "listStorageClass")
	if clustersParam != "" {
		for _, cid := range strings.Split(clustersParam, ",") {
			cid = strings.TrimSpace(cid)
			if cid != "" {
				clusterIDs = append(clusterIDs, cid)
			}
		}
	} else if g.kubeClient != nil {
		// clustersParam 为空时，列出所有集群中的 StorageClass
		nodes, err := g.kubeClient.RlinfV1alpha1().Nodes(metav1.NamespaceAll).List(c.Request.Context(), metav1.ListOptions{})
		if err != nil {
			logger.Error(err, "failed to list rlark nodes for cluster ids")
		} else {
			seen := make(map[string]struct{})
			for _, node := range nodes.Items {
				cid := node.Labels[rlarkv1alpha1.LabelClusterID]
				if cid == "" {
					continue
				}
				if _, ok := seen[cid]; ok {
					continue
				}
				seen[cid] = struct{}{}
				clusterIDs = append(clusterIDs, cid)
			}
		}
	}

	if len(clusterIDs) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"data":    map[string]*StorageClassData{},
			"success": true,
		})
		return
	}

	if g.config.ServerAddress == "" {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "server-address is not configured, cannot proxy to cluster agents",
		})
		return
	}

	ssData := make(map[string]*StorageClassData)
	for _, clusterID := range clusterIDs {
		agentID, ok := strings.CutPrefix(clusterID, apis.RLarkAgentNamespacePrefix)
		if !ok {
			logger.Info("seems not a valid clusterID without prefix", "clusterID", clusterID)
			continue
		}
		agentSSList, err := g.fetchStorageClassesFromAgent(c.Request.Context(), agentID)
		if err != nil {
			logger.Error(err, "failed to fetch storage classes from agent", "agentID", agentID)
			continue
		}

		for _, ss := range agentSSList {
			if ss.Name == "default" || ss.Name == "local-path" || ss.Name == "hostpath" {
				continue
			}

			if ss.Provisioner != "rclone.csi.veloxpack.io" {
				continue
			}

			description := ""
			if ss.Annotations != nil {
				description = ss.Annotations["rlark.io/description"]
				if description == "" {
					if ss.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
						description = "Default StorageClass"
					}
				}
			}

			bucket := ""
			if ss.Parameters != nil {
				bucket = ss.Parameters["remotePath"]
				if bucket == "" {
					bucket = ss.Parameters["bucket"]
				}
			}

			if existing, ok := ssData[ss.Name]; ok {
				existing.Clusters = append(existing.Clusters, agentID)
			} else {
				data := &StorageClassData{
					Name:        ss.Name,
					Clusters:    []string{agentID},
					Description: description,
					Bucket:      bucket,
				}
				if sc, err := g.fetchStorageClassSecretConfig(c.Request.Context(), agentID, &ss); err == nil && sc != nil {
					data.Provider = sc.Provider
					data.Endpoint = sc.Endpoint
					data.Region = sc.Region
					data.PathStyle = sc.UsePathStyle
					data.AccessKeyId = sc.AccessKeyId
				}
				ssData[ss.Name] = data
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":    ssData,
		"success": true,
	})
}

// fetchStorageClassesFromAgent 通过Server代理从指定Agent获取StorageClass列表.
func (g *Gateway) fetchStorageClassesFromAgent(ctx context.Context, agentID string) ([]storagev1.StorageClass, error) {
	httpClient := &http.Client{Transport: g.serverTransport}
	url := fmt.Sprintf("%s/api/proxy/%s/api/kubernetes/apis/storage.k8s.io/v1/storageclasses",
		strings.TrimSuffix(g.config.ServerAddress, "/"),
		agentID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call server proxy: %w", err)
	}
	defer utils.CloseIO(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server proxy returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	var listResp storagev1.StorageClassList
	if err := json.Unmarshal(body, &listResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return listResp.Items, nil
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
// description of a rlark-managed StorageClass is stored. It matches the key
// read by listStorageClass.
const annoStorageClassDescription = "rlark.io/description"

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

// createStorageClass creates a rclone-backed StorageClass (and its accompanying
// Secret) on each requested cluster by proxying through the rlark-server to the
// target agent Kubernetes API. Existing resources are reconciled to the desired
// state.
func (g *Gateway) createStorageClass(c *gin.Context) {
	var req CreateStorageClassRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" || req.Bucket == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and bucket are required"})
		return
	}
	if req.AccessKeySecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "access_key_secret is required"})
		return
	}
	if len(req.Clusters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "clusters is required"})
		return
	}
	if g.config.ServerAddress == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server-address is not configured, cannot proxy to cluster agents"})
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}

	if strings.EqualFold(req.Provider, "Alibaba") {
		req.Endpoint = fmt.Sprintf("%s.oss-%s.aliyuncs.com", req.Bucket, req.Region)
	}

	logger := log.FromContext(c.Request.Context())

	for _, clusterID := range req.Clusters {
		agentID := normalizeStorageClusterID(clusterID)
		if err := g.applyStorageClassForAgent(c.Request.Context(), agentID, req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("cluster %s: %v", agentID, err)})
			return
		}
		logger.Info("applied storage class on cluster", "storageClass", req.Name, "cluster", agentID)
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// updateStorageClass reconciles a rlark-managed StorageClass by name. It applies
// the desired configuration to the selected clusters and removes it from
// previously associated clusters that are no longer selected.
func (g *Gateway) updateStorageClass(c *gin.Context) {
	name := c.Param("name")
	var req CreateStorageClassRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" {
		req.Name = name
	}
	if req.Name != name {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storage class name cannot be changed"})
		return
	}
	if req.Name == "" || req.Bucket == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and bucket are required"})
		return
	}
	if len(req.Clusters) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "clusters is required"})
		return
	}
	if g.config.ServerAddress == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server-address is not configured, cannot proxy to cluster agents"})
		return
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if strings.EqualFold(req.Provider, "Alibaba") {
		req.Endpoint = fmt.Sprintf("%s.oss-%s.aliyuncs.com", req.Bucket, req.Region)
	}

	ctx := c.Request.Context()
	desired := map[string]struct{}{}
	for _, clusterID := range req.Clusters {
		agentID := normalizeStorageClusterID(clusterID)
		if agentID == "" {
			continue
		}
		desired[agentID] = struct{}{}
	}
	if len(desired) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "clusters is required"})
		return
	}

	existing, err := g.findStorageClassClusters(ctx, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if req.AccessKeySecret == "" {
		for _, agentID := range existing {
			if err := g.fillStorageClassSecretFromExisting(ctx, agentID, &req); err == nil && req.AccessKeySecret != "" {
				break
			}
		}
		if req.AccessKeySecret == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "access_key_secret is required when the existing secret cannot be found"})
			return
		}
	}
	for _, agentID := range existing {
		if _, ok := desired[agentID]; ok {
			continue
		}
		if err := g.deleteStorageClassForAgent(ctx, agentID, name, req.Namespace); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("cluster %s: %v", agentID, err)})
			return
		}
	}
	for agentID := range desired {
		if err := g.applyStorageClassForAgent(ctx, agentID, req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("cluster %s: %v", agentID, err)})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// deleteStorageClass removes a rlark-managed StorageClass from all clusters
// where it exists, or from the comma-separated clusters query parameter.
func (g *Gateway) deleteStorageClass(c *gin.Context) {
	name := c.Param("name")
	namespace := c.DefaultQuery("namespace", "default")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if g.config.ServerAddress == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server-address is not configured, cannot proxy to cluster agents"})
		return
	}
	var clusterIDs []string
	if clustersParam := c.Query("clusters"); clustersParam != "" {
		for _, clusterID := range strings.Split(clustersParam, ",") {
			if agentID := normalizeStorageClusterID(strings.TrimSpace(clusterID)); agentID != "" {
				clusterIDs = append(clusterIDs, agentID)
			}
		}
	} else {
		existing, err := g.findStorageClassClusters(c.Request.Context(), name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		clusterIDs = existing
	}
	for _, agentID := range clusterIDs {
		if err := g.deleteStorageClassForAgent(c.Request.Context(), agentID, name, namespace); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("cluster %s: %v", agentID, err)})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// applyStorageClassForAgent creates or updates the Secret and StorageClass for a
// single agent cluster via the server Kubernetes proxy.
func (g *Gateway) applyStorageClassForAgent(ctx context.Context, agentID string, req CreateStorageClassRequest) error {
	if req.AccessKeySecret == "" {
		if err := g.fillStorageClassSecretFromExisting(ctx, agentID, &req); err != nil {
			return err
		}
	}
	var configBuf bytes.Buffer
	if err := ssSecretTmpl.Execute(&configBuf, req); err != nil {
		return fmt.Errorf("render secret config: %w", err)
	}
	rcloneConfig := configBuf.String()

	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name + "-secret",
			Namespace: req.Namespace,
			Labels: map[string]string{
				"controller": "rlark",
			},
		},
		StringData: map[string]string{
			"remote":     "s3",
			"remotePath": req.Bucket,
			"configData": rcloneConfig,
		},
	}
	secretJSON, err := json.Marshal(secret)
	if err != nil {
		return fmt.Errorf("marshal secret: %w", err)
	}
	secretPath := fmt.Sprintf("/api/v1/namespaces/%s/secrets", req.Namespace)
	if err := g.applyViaProxy(ctx, agentID, secretPath, secret.Name, secretJSON, func(existing []byte) ([]byte, error) {
		var ex v1.Secret
		if err := json.Unmarshal(existing, &ex); err != nil {
			return nil, err
		}
		ex.StringData = secret.StringData
		ex.Labels = secret.Labels
		return json.Marshal(&ex)
	}); err != nil {
		return err
	}

	ss := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				"controller": "rlark",
			},
			Annotations: map[string]string{
				annoStorageClassDescription: req.Description,
			},
		},
		Provisioner:       "rclone.csi.veloxpack.io",
		ReclaimPolicy:     ptr.To(v1.PersistentVolumeReclaimRetain),
		VolumeBindingMode: ptr.To(storagev1.VolumeBindingImmediate),
		Parameters: map[string]string{
			"remote":     "s3",
			"remotePath": req.Bucket,
			"csi.storage.k8s.io/node-publish-secret-name":      secret.Name,
			"csi.storage.k8s.io/node-publish-secret-namespace": secret.Namespace,
		},
		MountOptions: []string{
			"vfs-cache-mode=writes",
			"dir-cache-time=5m",
			"attr-timeout=1m",
			"poll-interval=5s",
			"vfs-write-back=1s",
			"vfs-cache-max-age=10m",
			"buffer-size=64M",
			"transfers=8",
			"checkers=16",
			"fast-list",
		},
	}
	ssJSON, err := json.Marshal(ss)
	if err != nil {
		return fmt.Errorf("marshal storage class: %w", err)
	}
	ssPath := "/apis/storage.k8s.io/v1/storageclasses"
	if err := g.applyViaProxy(ctx, agentID, ssPath, ss.Name, ssJSON, func(existing []byte) ([]byte, error) {
		var ex storagev1.StorageClass
		if err := json.Unmarshal(existing, &ex); err != nil {
			return nil, err
		}
		ex.Parameters = ss.Parameters
		ex.MountOptions = ss.MountOptions
		ex.Labels = ss.Labels
		ex.Annotations = ss.Annotations
		return json.Marshal(&ex)
	}); err != nil {
		return err
	}
	return nil
}

// applyViaProxy creates a Kubernetes resource via the server Kubernetes proxy.
// If the resource already exists (HTTP 409 Conflict), it fetches the existing
// object, invokes merge to copy the desired mutable fields onto it (preserving
// ResourceVersion and other fields), and PUTs the result. collectionPath is the
// Kubernetes API collection path (e.g. "/api/v1/namespaces/{ns}/secrets" or
// "/apis/storage.k8s.io/v1/storageclasses"); name is the resource name.
func (g *Gateway) applyViaProxy(ctx context.Context, agentID, collectionPath, name string, desiredJSON []byte, merge func(existing []byte) ([]byte, error)) error {
	postResp, err := g.proxyKubeRequest(ctx, http.MethodPost, agentID, collectionPath, desiredJSON)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	postBody, _ := io.ReadAll(postResp.Body)
	_ = postResp.Body.Close()
	if postResp.StatusCode == http.StatusCreated || postResp.StatusCode == http.StatusOK {
		return nil
	}
	if postResp.StatusCode != http.StatusConflict {
		return fmt.Errorf("create %s returned HTTP %d: %s", name, postResp.StatusCode, string(postBody))
	}

	// Resource already exists: fetch it, merge desired fields, and update.
	getResp, err := g.proxyKubeRequest(ctx, http.MethodGet, agentID, collectionPath+"/"+name, nil)
	if err != nil {
		return fmt.Errorf("get existing %s: %w", name, err)
	}
	existing, _ := io.ReadAll(getResp.Body)
	_ = getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		return fmt.Errorf("get existing %s returned HTTP %d: %s", name, getResp.StatusCode, string(existing))
	}

	merged, err := merge(existing)
	if err != nil {
		return fmt.Errorf("merge %s: %w", name, err)
	}

	putResp, err := g.proxyKubeRequest(ctx, http.MethodPut, agentID, collectionPath+"/"+name, merged)
	if err != nil {
		return fmt.Errorf("update %s: %w", name, err)
	}
	putBody, _ := io.ReadAll(putResp.Body)
	_ = putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK && putResp.StatusCode != http.StatusCreated {
		return fmt.Errorf("update %s returned HTTP %d: %s", name, putResp.StatusCode, string(putBody))
	}
	return nil
}

func normalizeStorageClusterID(clusterID string) string {
	clusterID = strings.TrimSpace(clusterID)
	if id, ok := strings.CutPrefix(clusterID, apis.RLarkAgentNamespacePrefix); ok {
		return id
	}
	return clusterID
}

func (g *Gateway) findStorageClassClusters(ctx context.Context, name string) ([]string, error) {
	if g.kubeClient == nil {
		return nil, nil
	}
	nodes, err := g.kubeClient.RlinfV1alpha1().Nodes(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list rlark nodes for cluster ids: %w", err)
	}
	seen := make(map[string]struct{})
	for _, node := range nodes.Items {
		clusterID := normalizeStorageClusterID(node.Labels[rlarkv1alpha1.LabelClusterID])
		if clusterID != "" {
			seen[clusterID] = struct{}{}
		}
	}
	var found []string
	for clusterID := range seen {
		items, err := g.fetchStorageClassesFromAgent(ctx, clusterID)
		if err != nil {
			continue
		}
		for _, item := range items {
			if item.Name == name {
				found = append(found, clusterID)
				break
			}
		}
	}
	return found, nil
}

func (g *Gateway) fillStorageClassSecretFromExisting(ctx context.Context, agentID string, req *CreateStorageClassRequest) error {
	ss, err := g.fetchStorageClassFromAgent(ctx, agentID, req.Name)
	if err != nil {
		return err
	}
	cfg, err := g.fetchStorageClassSecretConfig(ctx, agentID, ss)
	if err != nil {
		return err
	}
	if req.AccessKeySecret == "" {
		req.AccessKeySecret = cfg.SecretAccessKey
	}
	if req.AccessKeyId == "" {
		req.AccessKeyId = cfg.AccessKeyId
	}
	return nil
}

func (g *Gateway) fetchStorageClassFromAgent(ctx context.Context, agentID, name string) (*storagev1.StorageClass, error) {
	ssPath := "/apis/storage.k8s.io/v1/storageclasses/" + name
	resp, err := g.proxyKubeRequest(ctx, http.MethodGet, agentID, ssPath, nil)
	if err != nil {
		return nil, fmt.Errorf("get storage class %s: %w", name, err)
	}
	defer utils.CloseIO(resp.Body)
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read storage class response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get storage class %s returned HTTP %d: %s", name, resp.StatusCode, string(body))
	}
	var ss storagev1.StorageClass
	if err := json.Unmarshal(body, &ss); err != nil {
		return nil, fmt.Errorf("parse storage class: %w", err)
	}
	return &ss, nil
}

func (g *Gateway) deleteStorageClassForAgent(ctx context.Context, agentID, name, namespace string) error {
	ss, err := g.fetchStorageClassFromAgent(ctx, agentID, name)
	if err != nil {
		return nil
	}
	secretName := name + "-secret"
	secretNamespace := namespace
	if ss.Parameters != nil {
		if value := ss.Parameters["csi.storage.k8s.io/node-publish-secret-name"]; value != "" {
			secretName = value
		}
		if value := ss.Parameters["csi.storage.k8s.io/node-publish-secret-namespace"]; value != "" {
			secretNamespace = value
		}
	}
	if secretNamespace == "" {
		secretNamespace = "default"
	}
	if err := g.deleteViaProxy(ctx, agentID, "/apis/storage.k8s.io/v1/storageclasses/"+name); err != nil {
		return err
	}
	if secretName != "" {
		if err := g.deleteViaProxy(ctx, agentID, "/api/v1/namespaces/"+secretNamespace+"/secrets/"+secretName); err != nil {
			return err
		}
	}
	return nil
}

func (g *Gateway) deleteViaProxy(ctx context.Context, agentID, kubePath string) error {
	resp, err := g.proxyKubeRequest(ctx, http.MethodDelete, agentID, kubePath, nil)
	if err != nil {
		return fmt.Errorf("delete %s: %w", kubePath, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusOK ||
		resp.StatusCode == http.StatusAccepted ||
		resp.StatusCode == http.StatusNoContent ||
		resp.StatusCode == http.StatusNotFound {
		return nil
	}
	return fmt.Errorf("delete %s returned HTTP %d: %s", kubePath, resp.StatusCode, string(body))
}

// proxyKubeRequest sends a request to an agent Kubernetes API through the
// rlark-server proxy. kubePath is the Kubernetes API path (e.g.
// "/apis/storage.k8s.io/v1/storageclasses").
func (g *Gateway) proxyKubeRequest(ctx context.Context, method, agentID, kubePath string, body []byte) (*http.Response, error) {
	httpClient := &http.Client{Transport: g.serverTransport}
	url := fmt.Sprintf("%s/api/proxy/%s/api/kubernetes/%s",
		strings.TrimSuffix(g.config.ServerAddress, "/"),
		agentID,
		strings.TrimPrefix(kubePath, "/"),
	)
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return httpClient.Do(req)
}

// --- Storage class file operation handlers ---

// getStorageClient resolves the storage.Client for a given cluster and storage class.
// It fetches the StorageClass and its associated Secret via the server proxy,
// parses the S3 config from the secret, and returns a cached or newly created client.
func (g *Gateway) getStorageClient(ctx context.Context, cluster, name string) (*storage.Client, error) {
	cacheKey := cluster + "/" + name

	g.storageClientsMu.RLock()
	if client, ok := g.storageClients[cacheKey]; ok {
		g.storageClientsMu.RUnlock()
		return client, nil
	}
	g.storageClientsMu.RUnlock()

	agentID := cluster
	if id, ok := strings.CutPrefix(cluster, apis.RLarkAgentNamespacePrefix); ok {
		agentID = id
	}

	if g.config.ServerAddress == "" {
		return nil, fmt.Errorf("server-address is not configured")
	}

	ssPath := "/apis/storage.k8s.io/v1/storageclasses/" + name
	resp, err := g.proxyKubeRequest(ctx, http.MethodGet, agentID, ssPath, nil)
	if err != nil {
		return nil, fmt.Errorf("get storage class %s: %w", name, err)
	}
	defer utils.CloseIO(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read storage class response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get storage class %s returned HTTP %d: %s", name, resp.StatusCode, string(body))
	}

	var ss storagev1.StorageClass
	if err := json.Unmarshal(body, &ss); err != nil {
		return nil, fmt.Errorf("parse storage class: %w", err)
	}

	secretName := ss.Parameters["csi.storage.k8s.io/node-publish-secret-name"]
	secretNamespace := ss.Parameters["csi.storage.k8s.io/node-publish-secret-namespace"]
	bucketName := ss.Parameters["remotePath"]

	if secretName == "" || secretNamespace == "" {
		return nil, fmt.Errorf("storage class %s missing secret reference parameters", name)
	}

	secretPath := "/api/v1/namespaces/" + secretNamespace + "/secrets/" + secretName
	secretResp, err := g.proxyKubeRequest(ctx, http.MethodGet, agentID, secretPath, nil)
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", secretNamespace, secretName, err)
	}
	defer utils.CloseIO(secretResp.Body)

	secretBody, err := io.ReadAll(secretResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read secret response: %w", err)
	}
	if secretResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get secret %s returned HTTP %d: %s", secretName, secretResp.StatusCode, string(secretBody))
	}

	var secret v1.Secret
	if err := json.Unmarshal(secretBody, &secret); err != nil {
		return nil, fmt.Errorf("parse secret: %w", err)
	}

	configData, ok := secret.Data["configData"]
	if !ok {
		return nil, fmt.Errorf("secret %s missing configData", secretName)
	}

	s3Config, err := parseStorageConfigData(string(configData))
	if err != nil {
		return nil, fmt.Errorf("parse configData: %w", err)
	}
	s3Config.Bucket = bucketName

	client, err := storage.NewClient(s3Config)
	if err != nil {
		return nil, fmt.Errorf("create storage client: %w", err)
	}

	g.storageClientsMu.Lock()
	g.storageClients[cacheKey] = client
	g.storageClientsMu.Unlock()

	return client, nil
}

// fetchStorageClassSecretConfig fetches the Secret referenced by a StorageClass
// and parses the rclone configData into a storage.Config. It returns nil if any
// step fails (non-fatal, caller should proceed with empty fields).
func (g *Gateway) fetchStorageClassSecretConfig(ctx context.Context, agentID string, ss *storagev1.StorageClass) (*storage.Config, error) {
	if ss.Parameters == nil {
		return nil, fmt.Errorf("storage class %s has no parameters", ss.Name)
	}
	secretName := ss.Parameters["csi.storage.k8s.io/node-publish-secret-name"]
	secretNamespace := ss.Parameters["csi.storage.k8s.io/node-publish-secret-namespace"]
	if secretName == "" || secretNamespace == "" {
		return nil, fmt.Errorf("storage class %s missing secret reference parameters", ss.Name)
	}

	secretPath := "/api/v1/namespaces/" + secretNamespace + "/secrets/" + secretName
	secretResp, err := g.proxyKubeRequest(ctx, http.MethodGet, agentID, secretPath, nil)
	if err != nil {
		return nil, fmt.Errorf("get secret %s/%s: %w", secretNamespace, secretName, err)
	}
	defer utils.CloseIO(secretResp.Body)

	secretBody, err := io.ReadAll(secretResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read secret response: %w", err)
	}
	if secretResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get secret %s returned HTTP %d: %s", secretName, secretResp.StatusCode, string(secretBody))
	}

	var secret v1.Secret
	if err := json.Unmarshal(secretBody, &secret); err != nil {
		return nil, fmt.Errorf("parse secret: %w", err)
	}

	configData, ok := secret.Data["configData"]
	if !ok {
		return nil, fmt.Errorf("secret %s missing configData", secretName)
	}

	return parseStorageConfigData(string(configData))
}

// parseStorageConfigData parses the INI-format configData from a rclone Secret
// into a storage.Config.
func parseStorageConfigData(data string) (*storage.Config, error) {
	cfg := storage.DefaultConfig()
	lines := strings.Split(data, "\n")
	section := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		if section == "s3" {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			switch key {
			case "access_key_id":
				cfg.AccessKeyId = value
			case "secret_access_key":
				cfg.SecretAccessKey = value
			case "endpoint":
				cfg.Endpoint = value
			case "region":
				cfg.Region = value
			case "force_path_style":
				cfg.UsePathStyle = value == "true"
			case "provider":
				cfg.Provider = value
			}
		}
	}

	return cfg, nil
}

// listStorageClassFiles handles GET /api/v1/storage/storageclass/:cluster/:name/list
// with query parameters: prefix, delimiter, marker, maxKeys.
func (g *Gateway) listStorageClassFiles(c *gin.Context) {
	cluster := c.Param("cluster")
	name := c.Param("name")

	client, err := g.getStorageClient(c.Request.Context(), cluster, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	if maxKeys <= 0 {
		maxKeys = 100
	}

	options := &storage.ListOptions{
		Prefix:    prefix,
		MaxKeys:   maxKeys,
		Delimiter: delimiter,
		Marker:    marker,
	}

	result, err := client.ListObjectsWithDelimiter(options)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}
	defer utils.CloseIO(file)

	objectKey, err := client.UploadFileFromMultipart(header)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "object key is required"})
		return
	}

	client, err := g.getStorageClient(c.Request.Context(), cluster, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "object key is required"})
		return
	}

	client, err := g.getStorageClient(c.Request.Context(), cluster, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	err = client.DeleteObject(key)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Object deleted successfully"})
}
