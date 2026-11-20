package gateway

// Agent certificates as this site issues and revokes them: the official handleSignAgentCert ends in
// respondAgentCertStored, and router.go routes POST /certificates/revoke to handleRevokeCertificateViaServer (the
// official handler of that route is an unregistered stub).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

type agentCertWithPullSecretsResponse struct {
	signAgentCertResponse
	// ImagePullSecrets are the image registry credentials the cluster is given (their cluster selection takes it in):
	// whoever installs the agent puts them in before it starts, as its own image is pulled with them; the agent then
	// delivers each under its own name.
	ImagePullSecrets []agentImagePullSecret `json:"image_pull_secrets"`
}

// respondAgentCertStored ends the official handleSignAgentCert: it answers only once the signed certificate's Secret
// is stored, and adds the cluster's image registry credentials.
func (g *Gateway) respondAgentCertStored(c *gin.Context, cert signAgentCertResponse) {
	logger := log.FromContext(c.Request.Context())
	if err := g.storeOwnedAgentCertSecret(c.Request.Context(), cert.ClusterID, []byte(cert.CACert), []byte(cert.AgentCert), []byte(cert.AgentKey)); err != nil {
		logger.Error(err, "failed to store agent cert secret")
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to persist agent certificate")
		return
	}
	logger.Info("agent cert stored as secret", "cluster_id", cert.ClusterID)

	pullSecrets, err := g.imagePullSecretsOf(c.Request.Context(), cert.ClusterID)
	if err != nil {
		logger.Error(err, "failed to read the cluster's image registry credentials")
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to read the cluster's image registry credentials")
		return
	}
	c.JSON(http.StatusOK, agentCertWithPullSecretsResponse{signAgentCertResponse: cert, ImagePullSecrets: pullSecrets})
}

// storeOwnedAgentCertSecret creates the cluster's agent certificate Secret, or replaces the certificate in the one
// that is already this cluster's; a Secret of that name which is not is left alone and reported.
func (g *Gateway) storeOwnedAgentCertSecret(ctx context.Context, clusterID string, caCert, agentCert, agentKey []byte) error {
	if g.rawClient == nil {
		return fmt.Errorf("raw kubernetes client not initialized")
	}
	secretName := "rlark-agent-cert-" + clusterID
	namespace := g.managementNamespace()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
			Labels: map[string]string{
				common.AgentCertLabelKey: common.AgentCertLabelValue,
			},
			Annotations: map[string]string{
				"rlark.io/cluster-id": clusterID,
			},
		},
		Data: map[string][]byte{
			"ca.crt":  caCert,
			"tls.crt": agentCert,
			"tls.key": agentKey,
		},
	}
	secrets := g.rawClient.CoreV1().Secrets(namespace)
	_, err := secrets.Create(ctx, secret, metav1.CreateOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create secret %s: %w", secretName, err)
	}
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		existing, err := secrets.Get(ctx, secretName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get secret %s: %w", secretName, err)
		}
		if existing.Labels[common.AgentCertLabelKey] != common.AgentCertLabelValue || existing.Annotations["rlark.io/cluster-id"] != clusterID {
			return fmt.Errorf("secret %s ownership conflict", secretName)
		}
		if existing.Data == nil {
			existing.Data = make(map[string][]byte)
		}
		for k, v := range secret.Data {
			existing.Data[k] = v
		}
		_, err = secrets.Update(ctx, existing, metav1.UpdateOptions{})
		return err
	})
}

// handleRevokeCertificateViaServer has the Server revoke the certificate and answers once the Server confirms the
// revocation is stored.
func (g *Gateway) handleRevokeCertificateViaServer(c *gin.Context) {
	var req struct {
		CertType     string `json:"cert_type" binding:"required"`
		SerialNumber string `json:"serial_number" binding:"required"`
		SubjectKeyID string `json:"subject_key_id" binding:"required"`
		Reason       string `json:"reason"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "certificate identity is required")
		return
	}
	if g.config.ServerAddress == "" || g.serverTransport == nil {
		respondError(c, http.StatusServiceUnavailable, CodeNotConfigured, "server is not configured")
		return
	}
	body, err := json.Marshal(req)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "cannot encode request")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(g.config.ServerAddress, "/")+"/api/revoke", bytes.NewReader(body))
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, CodeNotConfigured, "invalid server address")
		return
	}
	request.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Transport: g.serverTransport}).Do(request)
	if err != nil {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "server revocation request failed; outcome may be unknown")
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusBadRequest {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "invalid certificate identity")
		return
	}
	if resp.StatusCode != http.StatusOK {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "server could not confirm revocation")
		return
	}
	var receipt struct {
		Persisted bool `json:"persisted"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&receipt); err != nil || !receipt.Persisted {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, "server did not confirm durable revocation; outcome may be unknown")
		return
	}
	c.JSON(http.StatusOK, gin.H{"persisted": true})
}
