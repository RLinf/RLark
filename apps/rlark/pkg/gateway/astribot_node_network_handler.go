package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/astribot/stationidentity"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type nodeNetworkSummaryClient interface {
	Read(context.Context, string, string) (stationidentity.NodeNetworkSummary, error)
}

type stationNetworkClient struct {
	endpoint, credentialFile string
	http                     *http.Client
}

var errStationNetworkCredentialRejected = errors.New("Station network service credential rejected")

func newStationNetworkClient(address, caFile, credentialFile string) (*stationNetworkClient, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || credentialFile == "" {
		return nil, errors.New("Station API HTTPS origin and credential file are required")
	}
	client, err := stationHTTPClient(caFile)
	if err != nil {
		return nil, err
	}
	return &stationNetworkClient{endpoint: strings.TrimRight(address, "/") + "/v1/internal/node-network", credentialFile: credentialFile, http: client}, nil
}

func (s *stationNetworkClient) Read(ctx context.Context, cluster, node string) (stationidentity.NodeNetworkSummary, error) {
	var result stationidentity.NodeNetworkSummary
	f, err := os.Open(s.credentialFile)
	if err != nil {
		return result, err
	}
	credential, err := io.ReadAll(io.LimitReader(f, 4097))
	_ = f.Close()
	if err != nil {
		return result, err
	}
	token := strings.TrimSpace(string(credential))
	if len(token) < 32 || len(token) > 4096 || strings.ContainsAny(token, " \t\r\n") {
		return result, errors.New("invalid Station credential")
	}
	body, err := json.Marshal(map[string]string{"cluster_id": cluster, "node_name": node})
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return result, errStationNetworkCredentialRejected
	}
	if resp.StatusCode != http.StatusOK {
		return result, errors.New("Station network query failed")
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, 16385))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		return result, err
	}
	var extra any
	if dec.Decode(&extra) != io.EOF || (result.State != "current" && result.State != "stale" && result.State != "unreported") {
		return result, errors.New("invalid Station network response")
	}
	return result, nil
}

func (g *Gateway) handleNodeNetwork(c *gin.Context) {
	clusterID, nodeName := c.Query("cluster_id"), c.Param("name")
	if clusterID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cluster_id is required"})
		return
	}
	cluster, err := g.kubeClient.RlinfV1alpha1().Clusters().Get(c.Request.Context(), clusterID, metav1.GetOptions{})
	if err != nil || (cluster.Spec.Delisted && c.GetString(authRoleKey) != "admin") {
		c.JSON(http.StatusNotFound, gin.H{"error": "cluster not found"})
		return
	}
	namespace, err := g.stationNamespace(c, clusterID)
	if err != nil {
		return
	}
	if _, err := g.kubeClient.RlinfV1alpha1().Nodes(namespace).Get(c.Request.Context(), nodeName, metav1.GetOptions{}); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "node not found"})
		return
	}
	summary, err := g.networkSummary.Read(c.Request.Context(), clusterID, nodeName)
	if err != nil {
		if errors.Is(err, errStationNetworkCredentialRejected) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "network summary service authentication failed"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "network summary unavailable"})
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, summary)
}
