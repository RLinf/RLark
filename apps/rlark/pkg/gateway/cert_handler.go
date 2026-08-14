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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

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
}

func (g *Gateway) handleSignAgentCert(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())

	var req signAgentCertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_id is required"})
		return
	}

	if g.config.ServerAddress == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server-address is not configured"})
		return
	}

	_, _, caCertPEM, err := g.getKCPAdminCerts()
	if err != nil {
		logger.Error(err, "failed to get KCP admin certificates")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("get admin cert: %v", err)})
		return
	}

	agentCertPEM, agentKeyPEM, err := g.signCertViaServer(req.ClusterID)
	if err != nil {
		logger.Error(err, "failed to sign agent certificate")
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("sign agent cert: %v", err)})
		return
	}

	if err := g.storeAgentCertSecret(c.Request.Context(), req.ClusterID, caCertPEM, []byte(agentCertPEM), []byte(agentKeyPEM)); err != nil {
		logger.Error(err, "failed to store agent cert secret")
	} else {
		logger.Info("agent cert stored as secret", "cluster_id", req.ClusterID)
	}

	c.JSON(http.StatusOK, signAgentCertResponse{
		ClusterID:  req.ClusterID,
		CACert:     string(caCertPEM),
		AgentCert:  agentCertPEM,
		AgentKey:   agentKeyPEM,
		ServerAddr: g.config.ServerAddress,
	})
}

func (g *Gateway) storeAgentCertSecret(ctx context.Context, clusterID string, caCert, agentCert, agentKey []byte) error {
	if g.rawClient == nil {
		return fmt.Errorf("raw kubernetes client not initialized")
	}
	secretName := "rlark-agent-cert-" + clusterID
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: common.SecretNamespace,
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
	_, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		_, updateErr := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Update(ctx, secret, metav1.UpdateOptions{})
		if updateErr != nil {
			return fmt.Errorf("create/update secret %s: %w", secretName, updateErr)
		}
		return nil
	}
	return nil
}

type agentCertListItem struct {
	ClusterID  string `json:"cluster_id"`
	CreatedAt  string `json:"created_at"`
	ServerAddr string `json:"server_addr"`
}

func (g *Gateway) handleListAgentCerts(c *gin.Context) {
	if g.rawClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "raw kubernetes client not initialized"})
		return
	}
	ctx := c.Request.Context()
	secretList, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.FormatLabels(map[string]string{common.AgentCertLabelKey: common.AgentCertLabelValue}),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("list secrets: %v", err)})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "raw kubernetes client not initialized"})
		return
	}
	clusterID := c.Param("cluster_id")
	if clusterID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_id is required"})
		return
	}
	ctx := c.Request.Context()
	secretName := "rlark-agent-cert-" + clusterID
	secret, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("secret not found: %v", err)})
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

	adminSecret, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Get(ctx, common.AdminCertSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("get secret %s: %w", common.AdminCertSecretName, err)
	}

	caSecret, err := g.rawClient.CoreV1().Secrets(common.SecretNamespace).Get(ctx, common.TLSCASecretName, metav1.GetOptions{})
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
	if g.dbClient == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revocation not supported: database not configured"})
		return
	}
	// TODO: Implement certificate revocation logic
}
