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
		c.JSON(http.StatusBadRequest, gin.H{"error": "pod name is required"})
		return
	}

	podList, err := g.kubeClient.RlinfV1alpha1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", podCRName),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("list pods: %v", err)})
		return
	}
	if len(podList.Items) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("pod %s not found", podCRName)})
		return
	}
	pod := podList.Items[0]

	agentID, ok := strings.CutPrefix(pod.Namespace, apis.RLarkAgentNamespacePrefix)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pod is not in an agent namespace"})
		return
	}

	podNamespace := pod.Spec.PodNamespace
	podName := pod.Spec.PodName
	if podNamespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pod namespace or name is empty"})
		return
	}

	container := c.DefaultQuery("container", "main")
	command := c.DefaultQuery("command", "/bin/sh")

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
		_ = browserWs.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("failed to connect to server: %v\r\n", err)))
		return
	}
	defer func() { _ = serverWs.Close() }()

	terminalrelay.Relay(browserWs, serverWs)
}
