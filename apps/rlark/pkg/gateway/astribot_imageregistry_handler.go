package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"

	"github.com/gin-gonic/gin"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// auditRegistryPasswordAccess records authorization outcomes without credential values.
func (g *Gateway) auditRegistryPasswordAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
		log.FromContext(c.Request.Context()).WithName("audit").Info("image registry password access", "actor", c.GetString(authSubjectKey), "registryID", c.Param("id"), "status", c.Writer.Status())
	}
}

func (g *Gateway) handleGetImageRegistryPassword(c *gin.Context) {
	credential, ok := g.registryCredential(c)
	if !ok {
		return
	}
	if credential.Version != imageRegistryCredentialVersion || credential.Password == "" {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "invalid image registry data")
		return
	}
	c.JSON(http.StatusOK, gin.H{"password": credential.Password})
}

// agentImagePullSecret is an image registry credential as a cluster's pull Secret has it.
type agentImagePullSecret struct {
	Name             string `json:"name"`
	DockerConfigJSON string `json:"docker_config_json"`
}

// imagePullSecretsOf is every image registry credential whose cluster selection takes the cluster in, as the
// Secret the delivery makes of it in the cluster's rlark-system.
func (g *Gateway) imagePullSecretsOf(ctx context.Context, clusterID string) ([]agentImagePullSecret, error) {
	secrets, err := listImageRegistrySecrets(ctx, g.rawClient, g.managementNamespace())
	if err != nil {
		return nil, err
	}
	out := []agentImagePullSecret{}
	for i := range secrets {
		id, ok := parseImageRegistryID(secrets[i].Name)
		if !ok {
			continue
		}
		var credential storedImageRegistryCredential
		if err := json.Unmarshal(secrets[i].Data[common.ImageRegistryCredentialDataKey], &credential); err != nil {
			return nil, fmt.Errorf("decode image registry %s: %w", id, err)
		}
		selection := credential.ClusterSelection
		if selection.Mode != clusterSelectionAll && !(selection.Mode == clusterSelectionSelected && slices.Contains(selection.Clusters, clusterID)) {
			continue
		}
		config, err := buildDockerConfigJSON(credential.Registry, credential.Username, credential.Password)
		if err != nil {
			return nil, err
		}
		out = append(out, agentImagePullSecret{Name: imageRegistryCredentialName(id), DockerConfigJSON: string(config)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
