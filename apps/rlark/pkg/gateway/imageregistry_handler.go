package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/yaml"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

const (
	imageRegistryCredentialVersion = "v1"
	imageRegistryCreateAttempts    = 3
	imageRegistryNoTargetsPattern  = "/"
	imageRegistryNamespacePrefix   = "rlark-"
)

var imageRegistryIDPattern = regexp.MustCompile(`^ir-[0-9a-f]{16}$`)

type clusterSelectionMode string

const (
	clusterSelectionNone     clusterSelectionMode = "None"
	clusterSelectionSelected clusterSelectionMode = "Selected"
	clusterSelectionAll      clusterSelectionMode = "All"
)

type clusterSelection struct {
	Mode     clusterSelectionMode `json:"mode"`
	Clusters []string             `json:"clusters"`
}

type storedImageRegistryCredential struct {
	Version          string           `json:"version"`
	Name             string           `json:"name"`
	Registry         string           `json:"registry"`
	Username         string           `json:"username"`
	Password         string           `json:"password"`
	ClusterSelection clusterSelection `json:"clusterSelection"`
}

type imageRegistryItem struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Registry         string           `json:"registry"`
	Username         string           `json:"username"`
	ClusterSelection clusterSelection `json:"clusterSelection"`
}

type createImageRegistryRequest struct {
	Name             string           `json:"name"`
	Registry         string           `json:"registry"`
	Username         string           `json:"username"`
	Password         string           `json:"password"`
	ClusterSelection clusterSelection `json:"clusterSelection"`
}

type updateImageRegistryRequest struct {
	Name             string           `json:"name"`
	Registry         string           `json:"registry"`
	Username         string           `json:"username"`
	Password         *string          `json:"password,omitempty"`
	ClusterSelection clusterSelection `json:"clusterSelection"`
}

func newImageRegistryID() string {
	return "ir-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
}

func imageRegistryReplicationName(id string) string { return id + "-replication" }
func imageRegistryDeliveryName(id string) string    { return id + "-delivery" }
func imageRegistryCredentialName(id string) string  { return id }

func parseImageRegistryID(name string) (string, bool) {
	const suffix = "-replication"
	if !strings.HasSuffix(name, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(name, suffix)
	return id, imageRegistryIDPattern.MatchString(id)
}

func normalizeClusterSelection(selection clusterSelection) (clusterSelection, error) {
	clusters := make([]string, 0, len(selection.Clusters))
	for _, cluster := range selection.Clusters {
		cluster = strings.TrimSpace(cluster)
		cluster = strings.TrimPrefix(cluster, imageRegistryNamespacePrefix)
		if cluster == "" {
			return clusterSelection{}, fmt.Errorf("cluster names must not be empty")
		}
		if problems := validation.IsDNS1123Label(imageRegistryNamespacePrefix + cluster); len(problems) != 0 {
			return clusterSelection{}, fmt.Errorf("invalid cluster name %q: %s", cluster, strings.Join(problems, "; "))
		}
		clusters = append(clusters, cluster)
	}
	sort.Strings(clusters)
	clusters = slices.Compact(clusters)
	selection.Clusters = clusters

	switch selection.Mode {
	case clusterSelectionNone, clusterSelectionAll:
		if len(clusters) != 0 {
			return clusterSelection{}, fmt.Errorf("clusters must be empty for mode %s", selection.Mode)
		}
	case clusterSelectionSelected:
		if len(clusters) == 0 {
			return clusterSelection{}, fmt.Errorf("at least one cluster is required for mode Selected")
		}
	default:
		return clusterSelection{}, fmt.Errorf("invalid cluster selection mode %q", selection.Mode)
	}
	return selection, nil
}

func normalizeImageRegistryCredential(name, registry, username, password string, selection clusterSelection) (storedImageRegistryCredential, error) {
	name = strings.TrimSpace(name)
	registry = common.NormalizeRegistry(registry)
	username = strings.TrimSpace(username)
	if name == "" || registry == "" || username == "" || password == "" {
		return storedImageRegistryCredential{}, fmt.Errorf("name, registry, username, and password are required")
	}
	selection, err := normalizeClusterSelection(selection)
	if err != nil {
		return storedImageRegistryCredential{}, err
	}
	return storedImageRegistryCredential{
		Version:          imageRegistryCredentialVersion,
		Name:             name,
		Registry:         registry,
		Username:         username,
		Password:         password,
		ClusterSelection: selection,
	}, nil
}

func buildDockerConfigJSON(registry, username, password string) ([]byte, error) {
	dockerConfig := map[string]map[string]map[string]string{
		"auths": {
			common.NormalizeRegistry(registry): {
				"auth": base64.StdEncoding.EncodeToString([]byte(username + ":" + password)),
			},
		},
	}
	return json.Marshal(dockerConfig)
}

func buildImageRegistryReplicationSecret(namespace, id string, credential storedImageRegistryCredential) (*corev1.Secret, error) {
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		return nil, fmt.Errorf("marshal credential: %w", err)
	}
	dockerConfigJSON, err := buildDockerConfigJSON(credential.Registry, credential.Username, credential.Password)
	if err != nil {
		return nil, fmt.Errorf("marshal docker config: %w", err)
	}

	dockerSecret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      imageRegistryCredentialName(id),
			Namespace: "rlark-system",
			Labels:    map[string]string{common.ImageRegistryCredentialLabel: "true"},
			Annotations: map[string]string{
				common.ImageRegistryAnnotationRegistry: credential.Registry,
				common.ImageRegistryAnnotationUsername: credential.Username,
			},
		},
		Type: corev1.SecretTypeDockerConfigJson,
		Data: map[string][]byte{corev1.DockerConfigJsonKey: dockerConfigJSON},
	}
	dockerManifest, err := yaml.Marshal([]corev1.Secret{dockerSecret})
	if err != nil {
		return nil, fmt.Errorf("marshal docker secret manifest: %w", err)
	}

	deliveryConfig := distribution.Config{Apply: distribution.ApplyPolicy{
		Mode: "ServerSideApply", ConflictPolicy: "Fail", DeletionPolicy: "Delete",
	}}
	deliveryConfigYAML, err := yaml.Marshal(deliveryConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal delivery config: %w", err)
	}
	deliverySecret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   imageRegistryDeliveryName(id),
			Labels: map[string]string{common.ImageRegistryDeliveryLabel: "true"},
		},
		Type: distribution.DeliverySecretType,
		Data: map[string][]byte{"config": deliveryConfigYAML, "manifests": dockerManifest},
	}
	deliveryManifest, err := yaml.Marshal([]corev1.Secret{deliverySecret})
	if err != nil {
		return nil, fmt.Errorf("marshal delivery secret manifest: %w", err)
	}

	include := []string{imageRegistryNoTargetsPattern}
	switch credential.ClusterSelection.Mode {
	case clusterSelectionSelected:
		include = make([]string, 0, len(credential.ClusterSelection.Clusters))
		for _, cluster := range credential.ClusterSelection.Clusters {
			include = append(include, imageRegistryNamespacePrefix+cluster)
		}
	case clusterSelectionAll:
		include = []string{imageRegistryNamespacePrefix + "*"}
	}
	replicationConfig := distribution.Config{Apply: distribution.ApplyPolicy{
		Mode: "ServerSideApply", ConflictPolicy: "Fail", DeletionPolicy: "Delete",
	}}
	replicationConfig.Targets.Namespaces = &distribution.GlobSelector{
		Include: include,
		Exclude: []string{"rlark-system"},
	}
	replicationConfigYAML, err := yaml.Marshal(replicationConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal replication config: %w", err)
	}

	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imageRegistryReplicationName(id),
			Namespace: namespace,
			Labels:    map[string]string{common.ImageRegistryReplicationLabel: "true"},
		},
		Type: distribution.ReplicationSecretType,
		Data: map[string][]byte{
			common.ImageRegistryCredentialDataKey: credentialJSON,
			"config":                              replicationConfigYAML,
			"manifests":                           deliveryManifest,
		},
	}, nil
}

func imageRegistryItemFromSecret(secret *corev1.Secret) (imageRegistryItem, error) {
	id, ok := parseImageRegistryID(secret.Name)
	if !ok {
		return imageRegistryItem{}, fmt.Errorf("invalid image registry secret name %q", secret.Name)
	}
	if secret.Labels[common.ImageRegistryReplicationLabel] != "true" || secret.Type != distribution.ReplicationSecretType {
		return imageRegistryItem{}, fmt.Errorf("secret %q is not an image registry replication", secret.Name)
	}
	var credential storedImageRegistryCredential
	if err := json.Unmarshal(secret.Data[common.ImageRegistryCredentialDataKey], &credential); err != nil {
		return imageRegistryItem{}, fmt.Errorf("decode credential: %w", err)
	}
	if credential.Version != imageRegistryCredentialVersion {
		return imageRegistryItem{}, fmt.Errorf("unsupported credential version %q", credential.Version)
	}
	selection, err := normalizeClusterSelection(credential.ClusterSelection)
	if err != nil {
		return imageRegistryItem{}, fmt.Errorf("invalid cluster selection: %w", err)
	}
	return imageRegistryItem{
		ID: id, Name: credential.Name, Registry: credential.Registry, Username: credential.Username, ClusterSelection: selection,
	}, nil
}

func listImageRegistrySecrets(ctx context.Context, rawClient kubernetes.Interface, namespace string) ([]corev1.Secret, error) {
	secretList, err := rawClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.Set{common.ImageRegistryReplicationLabel: "true"}.AsSelector().String(),
	})
	if err != nil {
		return nil, err
	}
	return secretList.Items, nil
}

func validateImageRegistryID(id string) error {
	if !imageRegistryIDPattern.MatchString(id) {
		return fmt.Errorf("id must use the format ir- followed by 16 lowercase hexadecimal characters")
	}
	return nil
}

func getImageRegistrySecret(ctx context.Context, rawClient kubernetes.Interface, namespace, id string) (*corev1.Secret, error) {
	secret, err := rawClient.CoreV1().Secrets(namespace).Get(ctx, imageRegistryReplicationName(id), metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	if _, err := imageRegistryItemFromSecret(secret); err != nil {
		return nil, errors.NewNotFound(corev1.Resource("secrets"), secret.Name)
	}
	return secret, nil
}

func (g *Gateway) handleListImageRegistries(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	secrets, err := listImageRegistrySecrets(c.Request.Context(), g.rawClient, g.managementNamespace())
	if err != nil {
		logger.Error(err, "failed to list image registry secrets")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to list image registries: %v", err)})
		return
	}
	items := make([]imageRegistryItem, 0, len(secrets))
	for i := range secrets {
		item, err := imageRegistryItemFromSecret(&secrets[i])
		if err != nil {
			logger.Error(err, "invalid image registry secret", "secret", secrets[i].Name)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid image registry data"})
			return
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name == items[j].Name {
			return items[i].ID < items[j].ID
		}
		return items[i].Name < items[j].Name
	})
	c.JSON(http.StatusOK, items)
}

func (g *Gateway) handleGetImageRegistry(c *gin.Context) {
	id := c.Param("id")
	if err := validateImageRegistryID(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	secret, err := getImageRegistrySecret(c.Request.Context(), g.rawClient, g.managementNamespace(), id)
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "image registry not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get image registry: %v", err)})
		return
	}
	item, err := imageRegistryItemFromSecret(secret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid image registry data"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (g *Gateway) handleCreateImageRegistry(c *gin.Context) {
	var req createImageRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	credential, err := normalizeImageRegistryCredential(req.Name, req.Registry, req.Username, req.Password, req.ClusterSelection)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for range imageRegistryCreateAttempts {
		id := newImageRegistryID()
		secret, err := buildImageRegistryReplicationSecret(g.managementNamespace(), id, credential)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if _, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Create(c.Request.Context(), secret, metav1.CreateOptions{}); err != nil {
			if errors.IsAlreadyExists(err) {
				continue
			}
			log.FromContext(c.Request.Context()).Error(err, "failed to create image registry replication")
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to create image registry: %v", err)})
			return
		}
		item, _ := imageRegistryItemFromSecret(secret)
		c.JSON(http.StatusCreated, item)
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to allocate image registry id"})
}

func (g *Gateway) handleUpdateImageRegistry(c *gin.Context) {
	id := c.Param("id")
	if err := validateImageRegistryID(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var req updateImageRegistryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if req.Password != nil && *req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must not be empty"})
		return
	}
	selection, err := normalizeClusterSelection(req.ClusterSelection)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	password := "placeholder"
	if req.Password != nil {
		password = *req.Password
	}
	if _, err := normalizeImageRegistryCredential(req.Name, req.Registry, req.Username, password, selection); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var item imageRegistryItem
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		secret, err := getImageRegistrySecret(c.Request.Context(), g.rawClient, g.managementNamespace(), id)
		if err != nil {
			return err
		}
		var old storedImageRegistryCredential
		if err := json.Unmarshal(secret.Data[common.ImageRegistryCredentialDataKey], &old); err != nil {
			return fmt.Errorf("decode stored credential: %w", err)
		}
		password := old.Password
		if req.Password != nil {
			password = *req.Password
		}
		credential, err := normalizeImageRegistryCredential(req.Name, req.Registry, req.Username, password, req.ClusterSelection)
		if err != nil {
			return err
		}
		rebuilt, err := buildImageRegistryReplicationSecret(secret.Namespace, id, credential)
		if err != nil {
			return err
		}
		secret.Data = rebuilt.Data
		updated, err := g.rawClient.CoreV1().Secrets(secret.Namespace).Update(c.Request.Context(), secret, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		item, err = imageRegistryItemFromSecret(updated)
		return err
	})
	if err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "image registry not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to update image registry: %v", err)})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (g *Gateway) handleDeleteImageRegistry(c *gin.Context) {
	id := c.Param("id")
	if err := validateImageRegistryID(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if _, err := getImageRegistrySecret(c.Request.Context(), g.rawClient, g.managementNamespace(), id); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "image registry not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get image registry: %v", err)})
		return
	}
	if err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Delete(c.Request.Context(), imageRegistryReplicationName(id), metav1.DeleteOptions{}); err != nil {
		if errors.IsNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "image registry not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to delete image registry: %v", err)})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"ok": true})
}
