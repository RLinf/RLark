package gateway

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// A job secret is a set of key/value pairs created in RLark and delivered, as
// a Kubernetes Secret of the same name, to the namespace Jobs run in on every
// selected cluster. A Job refers to it by name; its values never appear in a
// Job. It travels the way storage class credentials do: a Replication
// declaration on the management plane, applied by each cluster's agent.
const (
	jobSecretLabel   = "rlark.io/job-secret"
	jobSecretVersion = "v1"
)

// Names RLark gives its own Secrets in that namespace are not available.
var jobSecretReservedPrefixes = []string{"rlark-", "jo-", "sc-", "ir-", "agent-"}

type jobSecretRequest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Clusters    []string          `json:"clusters"`
	Data        map[string]string `json:"data"`
}

// storedJobSecret is what a listing returns: never the values.
type storedJobSecret struct {
	Version     string   `json:"version"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Clusters    []string `json:"clusters"`
	Keys        []string `json:"keys"`
	CreatedBy   string   `json:"createdBy,omitempty"`
	CreatedAt   string   `json:"createdAt,omitempty"`
}

func jobSecretReplicationName(name string) string { return "js-" + name + "-replication" }

func buildJobSecretReplication(namespace string, req jobSecretRequest) (*v1.Secret, error) {
	name := strings.TrimSpace(req.Name)
	if problems := validation.IsDNS1123Subdomain(name); len(problems) != 0 || len(name) > 63 {
		return nil, fmt.Errorf("invalid secret name %q", req.Name)
	}
	for _, prefix := range jobSecretReservedPrefixes {
		if strings.HasPrefix(name, prefix) {
			return nil, fmt.Errorf("secret names starting with %q are reserved", prefix)
		}
	}
	if len(req.Clusters) == 0 || len(req.Data) == 0 {
		return nil, fmt.Errorf("clusters and data are required")
	}
	clusters := append([]string(nil), req.Clusters...)
	sort.Strings(clusters)
	for _, cluster := range clusters {
		if problems := validation.IsDNS1123Label(cluster); len(problems) != 0 {
			return nil, fmt.Errorf("invalid cluster ID %q", cluster)
		}
	}
	keys := make([]string, 0, len(req.Data))
	size := 0
	for key, value := range req.Data {
		if problems := validation.IsConfigMapKey(key); len(problems) != 0 {
			return nil, fmt.Errorf("invalid key %q", key)
		}
		keys = append(keys, key)
		size += len(value)
	}
	if size > 512*1024 {
		return nil, fmt.Errorf("secret data exceeds 512 KiB")
	}
	sort.Strings(keys)

	delivered := v1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: apis.Namespace, Labels: map[string]string{"controller": "rlark", jobSecretLabel: "true"}},
		Type:       v1.SecretTypeOpaque, StringData: req.Data,
	}
	manifests, err := yaml.Marshal([]any{delivered})
	if err != nil {
		return nil, err
	}
	apply := distribution.ApplyPolicy{Mode: "ServerSideApply", ConflictPolicy: "Fail", DeletionPolicy: "Delete"}
	deliveryConfig, err := yaml.Marshal(distribution.Config{Apply: apply})
	if err != nil {
		return nil, err
	}
	delivery := v1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: "js-" + name + "-delivery", Labels: map[string]string{jobSecretLabel: "true"}},
		Type:       distribution.DeliverySecretType,
		Data:       map[string][]byte{"config": deliveryConfig, "manifests": manifests},
	}
	deliveryManifest, err := yaml.Marshal([]v1.Secret{delivery})
	if err != nil {
		return nil, err
	}
	replication := distribution.Config{Apply: apply}
	replication.Targets.Namespaces = &distribution.GlobSelector{Include: clusters, Exclude: []string{apis.Namespace}}
	replicationConfig, err := yaml.Marshal(replication)
	if err != nil {
		return nil, err
	}
	stored, err := json.Marshal(storedJobSecret{Version: jobSecretVersion, Name: name, Description: req.Description, Clusters: clusters, Keys: keys})
	if err != nil {
		return nil, err
	}
	return &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: jobSecretReplicationName(name), Namespace: namespace, Labels: map[string]string{jobSecretLabel: "true"}},
		Type:       distribution.ReplicationSecretType,
		Data:       map[string][]byte{"secret": stored, "config": replicationConfig, "manifests": deliveryManifest},
	}, nil
}

// putJobSecret creates the declaration or replaces the one of the same name.
func putJobSecret(ctx context.Context, client kubernetes.Interface, namespace, subject string, req jobSecretRequest, replace bool) error {
	secret, err := buildJobSecretReplication(namespace, req)
	if err != nil {
		return &jobSecretInvalid{err}
	}
	tenant.SetIdentity(secret, tenant.DefaultID, subject)
	secrets := client.CoreV1().Secrets(namespace)
	if !replace {
		_, err = secrets.Create(ctx, secret, metav1.CreateOptions{})
		return err
	}
	current, err := secrets.Get(ctx, secret.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err = secrets.Create(ctx, secret, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return err
	}
	current.Data = secret.Data
	_, err = secrets.Update(ctx, current, metav1.UpdateOptions{})
	return err
}

type jobSecretInvalid struct{ error }

func (g *Gateway) respondJobSecretError(c *gin.Context, err error) {
	switch invalid := (*jobSecretInvalid)(nil); {
	case stderrors.As(err, &invalid):
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
	case errors.IsAlreadyExists(err):
		respondError(c, http.StatusConflict, CodeInvalidRequest, "a secret of this name exists")
	case errors.IsNotFound(err):
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "secret not found")
	default:
		respondKubeError(c, fmt.Sprintf("job secret: %v", err), err)
	}
}

func (g *Gateway) createJobSecret(c *gin.Context) {
	var req jobSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	if err := putJobSecret(c.Request.Context(), g.rawClient, g.managementNamespace(), c.GetString(authSubjectKey), req, false); err != nil {
		g.respondJobSecretError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "name": strings.TrimSpace(req.Name)})
}

// updateJobSecret replaces the clusters and all the values of a secret. A request without values gives the secret
// to the clusters it names with the values it has: a cluster is added to a secret without anybody handing its
// values over again.
func (g *Gateway) updateJobSecret(c *gin.Context) {
	var req jobSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	req.Name = c.Param("name")
	current, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(c.Request.Context(), jobSecretReplicationName(req.Name), metav1.GetOptions{})
	if err != nil {
		g.respondJobSecretError(c, err)
		return
	}
	if len(req.Data) == 0 {
		stored, err := readJobSecret(current)
		if err != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError, "the stored secret cannot be read")
			return
		}
		if req.Data = stored.Data; req.Description == "" {
			req.Description = stored.Description
		}
	}
	if err := putJobSecret(c.Request.Context(), g.rawClient, g.managementNamespace(), c.GetString(authSubjectKey), req, true); err != nil {
		g.respondJobSecretError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "name": req.Name})
}

func (g *Gateway) listJobSecrets(c *gin.Context) {
	secrets, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).List(c.Request.Context(), metav1.ListOptions{LabelSelector: jobSecretLabel + "=true"})
	if err != nil {
		respondKubeError(c, fmt.Sprintf("list job secrets: %v", err), err)
		return
	}
	items := make([]storedJobSecret, 0, len(secrets.Items))
	for i := range secrets.Items {
		var stored storedJobSecret
		if secrets.Items[i].Type != distribution.ReplicationSecretType || json.Unmarshal(secrets.Items[i].Data["secret"], &stored) != nil {
			continue
		}
		stored.CreatedBy = secrets.Items[i].Annotations[tenant.CreatedByAnnotation]
		stored.CreatedAt = secrets.Items[i].CreationTimestamp.UTC().Format("2006-01-02T15:04:05Z")
		items = append(items, stored)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

func (g *Gateway) deleteJobSecret(c *gin.Context) {
	if err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Delete(c.Request.Context(), jobSecretReplicationName(c.Param("name")), metav1.DeleteOptions{}); err != nil {
		g.respondJobSecretError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// readJobSecret is the request a job secret's declaration was built from: what it stores of itself, and the values
// of the Secret its delivery declares.
func readJobSecret(declaration *v1.Secret) (jobSecretRequest, error) {
	var stored storedJobSecret
	var deliveries, delivered []v1.Secret
	if err := json.Unmarshal(declaration.Data["secret"], &stored); err != nil {
		return jobSecretRequest{}, err
	}
	if err := yaml.Unmarshal(declaration.Data["manifests"], &deliveries); err != nil || len(deliveries) != 1 {
		return jobSecretRequest{}, fmt.Errorf("the declaration of %s holds no delivery: %v", declaration.Name, err)
	}
	if err := yaml.Unmarshal(deliveries[0].Data["manifests"], &delivered); err != nil || len(delivered) != 1 {
		return jobSecretRequest{}, fmt.Errorf("the delivery of %s holds no secret: %v", declaration.Name, err)
	}
	return jobSecretRequest{Name: stored.Name, Description: stored.Description, Clusters: stored.Clusters, Data: delivered[0].StringData}, nil
}
