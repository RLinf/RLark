package gateway

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
	"github.com/rlinf/rlark/apps/rlark/pkg/terminalrelay"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const websocketDialTimeout = 15 * time.Second

// The default Gorilla origin check accepts requests without an Origin header
// and otherwise requires Origin.Host to equal Request.Host.
var gwTerminalUpgrader = websocket.Upgrader{}

func (g *Gateway) handlePodTerminal(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	podCRName := c.Param("name")
	if podCRName == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "pod name is required")
		return
	}

	podList, err := g.kubeClient.RlinfV1alpha1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", podCRName),
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("list pods: %v", err))
		return
	}
	if len(podList.Items) == 0 {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, fmt.Sprintf("pod %s not found", podCRName))
		return
	}
	pod := podList.Items[0]

	// The tenant boundary is checked before the connection is upgraded, so that
	// a refusal is still an ordinary HTTP response.
	scope, scopeOK := g.tenantScopeFor(c, "pods")
	if !scopeOK {
		return
	}
	if !authorizeTenantObject(c, scope, tenant.IDOf(&pod)) {
		return
	}

	agentID, ok := strings.CutPrefix(pod.Namespace, apis.RLarkAgentNamespacePrefix)
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "pod is not in an agent namespace")
		return
	}

	podNamespace := pod.Spec.PodNamespace
	podName := pod.Spec.PodName
	if podNamespace == "" || podName == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "pod namespace or name is empty")
		return
	}

	container := c.DefaultQuery("container", "main")
	command := c.DefaultQuery("command", "/bin/bash")

	browserWs, err := gwTerminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error(err, "failed to upgrade browser WebSocket connection")
		return
	}
	defer func() { _ = browserWs.Close() }()

	if g.config.ServerAddress == "" {
		_ = browserWs.WriteMessage(websocket.TextMessage, []byte("server-address is not configured\r\n"))
		return
	}

	serverBase := strings.TrimSuffix(g.config.ServerAddress, "/")
	serverBase = strings.TrimPrefix(serverBase, "https://")
	serverBase = strings.TrimPrefix(serverBase, "http://")

	serverURL := fmt.Sprintf("wss://%s/api/terminal/%s/%s/%s?container=%s&command=%s",
		serverBase, agentID, podNamespace, podName, container, command)

	tlsConfig := g.serverTransport.TLSClientConfig
	if tlsConfig == nil {
		tlsConfig = &tls.Config{InsecureSkipVerify: true}
	}

	serverDialer := &websocket.Dialer{
		TLSClientConfig:  tlsConfig,
		HandshakeTimeout: websocketDialTimeout,
	}

	serverWs, _, err := serverDialer.DialContext(ctx, serverURL, nil)
	if err != nil {
		logger.Error(err, "failed to dial server terminal WebSocket")
		_ = browserWs.WriteMessage(websocket.TextMessage, fmt.Appendf(nil, "failed to connect to server: %v\r\n", err))
		return
	}
	defer func() { _ = serverWs.Close() }()

	terminalrelay.Relay(browserWs, serverWs)
}
