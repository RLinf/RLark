package gateway

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
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/retry"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

type signAgentCertRequest struct {
	ClusterID string `json:"cluster_id" binding:"required"`
}

type signAgentCertResponse struct {
	ClusterID  string `json:"cluster_id"`
	CACert     string `json:"ca_cert"`
	AgentCert  string `json:"agent_cert"`
	AgentKey   string `json:"agent_key"`
	ServerAddr string `json:"server_addr"`
	// ImagePullSecrets are the image registry credentials the cluster is given (their cluster selection takes it in):
	// whoever installs the agent puts them in before it starts, as its own image is pulled with them; the agent then
	// delivers each under its own name.
	ImagePullSecrets []agentImagePullSecret `json:"image_pull_secrets"`
}

func (g *Gateway) handleSignAgentCert(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())

	var req signAgentCertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "cluster_id is required")
		return
	}

	if g.config.ServerAddress == "" {
		respondError(c, http.StatusInternalServerError, CodeNotConfigured, "server-address is not configured")
		return
	}

	_, _, caCertPEM, err := g.getKCPAdminCerts()
	if err != nil {
		logger.Error(err, "failed to get KCP admin certificates")
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("get admin cert: %v", err))
		return
	}

	agentCertPEM, agentKeyPEM, err := g.signCertViaServer(req.ClusterID)
	if err != nil {
		logger.Error(err, "failed to sign agent certificate")
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, fmt.Sprintf("sign agent cert: %v", err))
		return
	}

	if err := g.storeAgentCertSecret(c.Request.Context(), req.ClusterID, caCertPEM, []byte(agentCertPEM), []byte(agentKeyPEM)); err != nil {
		logger.Error(err, "failed to store agent cert secret")
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to persist agent certificate")
		return
	}
	logger.Info("agent cert stored as secret", "cluster_id", req.ClusterID)

	pullSecrets, err := g.imagePullSecretsOf(c.Request.Context(), req.ClusterID)
	if err != nil {
		logger.Error(err, "failed to read the cluster's image registry credentials")
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to read the cluster's image registry credentials")
		return
	}
	c.JSON(http.StatusOK, signAgentCertResponse{
		ClusterID:        req.ClusterID,
		CACert:           string(caCertPEM),
		AgentCert:        agentCertPEM,
		AgentKey:         agentKeyPEM,
		ServerAddr:       g.config.ServerAddress,
		ImagePullSecrets: pullSecrets,
	})
}

func (g *Gateway) storeAgentCertSecret(ctx context.Context, clusterID string, caCert, agentCert, agentKey []byte) error {
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

type agentCertListItem struct {
	ClusterID  string `json:"cluster_id"`
	CreatedAt  string `json:"created_at"`
	ServerAddr string `json:"server_addr"`
}

func (g *Gateway) handleListAgentCerts(c *gin.Context) {
	if g.rawClient == nil {
		respondError(c, http.StatusInternalServerError, CodeNotConfigured, "raw kubernetes client not initialized")
		return
	}
	ctx := c.Request.Context()
	secretList, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).List(ctx, metav1.ListOptions{
		LabelSelector: labels.FormatLabels(map[string]string{common.AgentCertLabelKey: common.AgentCertLabelValue}),
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("list secrets: %v", err))
		return
	}
	items := make([]agentCertListItem, 0, len(secretList.Items))
	for _, s := range secretList.Items {
		clusterID := s.Annotations["rlark.io/cluster-id"]
		if clusterID == "" {
			clusterID = strings.TrimPrefix(s.Name, "rlark-agent-cert-")
		}
		items = append(items, agentCertListItem{
			ClusterID:  clusterID,
			CreatedAt:  s.CreationTimestamp.Format(time.RFC3339),
			ServerAddr: g.config.ServerAddress,
		})
	}
	c.JSON(http.StatusOK, items)
}

func (g *Gateway) handleGetAgentCert(c *gin.Context) {
	if g.rawClient == nil {
		respondError(c, http.StatusInternalServerError, CodeNotConfigured, "raw kubernetes client not initialized")
		return
	}
	clusterID := c.Param("cluster_id")
	if clusterID == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "cluster_id is required")
		return
	}
	ctx := c.Request.Context()
	secretName := "rlark-agent-cert-" + clusterID
	secret, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, fmt.Sprintf("secret not found: %v", err))
		return
	}
	c.JSON(http.StatusOK, signAgentCertResponse{
		ClusterID:  clusterID,
		CACert:     string(secret.Data["ca.crt"]),
		AgentCert:  string(secret.Data["tls.crt"]),
		AgentKey:   string(secret.Data["tls.key"]),
		ServerAddr: g.config.ServerAddress,
	})
}

func (g *Gateway) getKCPAdminCerts() (certPEM, keyPEM, caPEM []byte, err error) {
	if g.rawClient == nil {
		return nil, nil, nil, fmt.Errorf("raw kubernetes client not initialized")
	}

	ctx := context.Background()

	namespace := g.managementNamespace()
	adminSecret, err := g.rawClient.CoreV1().Secrets(namespace).Get(ctx, common.AdminCertSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get secret %s: %w", common.AdminCertSecretName, err)
	}

	caSecret, err := g.rawClient.CoreV1().Secrets(namespace).Get(ctx, common.TLSCASecretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get secret %s: %w", common.TLSCASecretName, err)
	}

	certPEM = adminSecret.Data["client.crt"]
	keyPEM = adminSecret.Data["client.key"]
	caPEM = caSecret.Data["ca.crt"]

	if len(certPEM) == 0 || len(keyPEM) == 0 || len(caPEM) == 0 {
		return nil, nil, nil, fmt.Errorf("incomplete certificate data in secrets")
	}

	return certPEM, keyPEM, caPEM, nil
}

func (g *Gateway) signCertViaServer(clusterID string) (certPEM, keyPEM string, err error) {
	httpClient := &http.Client{Transport: g.serverTransport}
	signReq := map[string]string{
		"role":      "agent",
		"client_id": clusterID,
	}
	reqBody, _ := json.Marshal(signReq)

	serverURL := strings.TrimSuffix(g.config.ServerAddress, "/") + "/api/sign"
	resp, err := httpClient.Post(serverURL, "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return "", "", fmt.Errorf("call server /api/sign: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read sign response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("server sign failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var signResp struct {
		CertPEM string `json:"cert_pem"`
		KeyPEM  string `json:"key_pem"`
	}
	if err := json.Unmarshal(body, &signResp); err != nil {
		return "", "", fmt.Errorf("unmarshal sign response: %w", err)
	}

	return signResp.CertPEM, signResp.KeyPEM, nil
}

func (g *Gateway) handleRevokeCertificate(c *gin.Context) {
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
