package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/terminalrelay"
)

// Browser traffic is same-origin, while the Gateway-to-Server connection does
// not send an Origin header. Gorilla's default origin check supports both.
var wsTerminalUpgrader = websocket.Upgrader{}

func (s *Server) handleTerminalProxy(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())

	target := c.Param("target") // agentID
	certMeta := GetCertMetaFromContext(c)
	if !auth.PermissionChecker.HasAgentProxyPermission(certMeta, target) {
		c.JSON(http.StatusForbidden, gin.H{"error": "client certificate does not have proxy permission for the target"})
		return
	}

	namespace := c.Param("namespace")
	podName := c.Param("pod")
	nodeName := c.Param("node")
	container := c.DefaultQuery("container", "main")
	command := c.DefaultQuery("command", "/bin/bash")

	if namespace == "" || podName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "namespace and pod are required"})
		return
	}

	browserWs, err := wsTerminalUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error(err, "failed to upgrade browser WebSocket connection")
		return
	}
	defer func() { _ = browserWs.Close() }()

	agentDialer := s.getAgentDialer(c.Request.Context(), target, nodeName)
	agentWsDialer := &websocket.Dialer{
		NetDialContext: agentDialer,
	}

	agentURL := fmt.Sprintf("ws://0.0.0.0:1/api/terminal/%s/%s?container=%s&command=%s",
		namespace, podName, container, command)

	agentWs, _, err := agentWsDialer.Dial(agentURL, nil)
	if err != nil {
		logger.Error(err, "failed to dial agent terminal WebSocket")
		_ = browserWs.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("failed to connect to agent: %v\r\n", err)))
		return
	}
	defer func() { _ = agentWs.Close() }()

	terminalrelay.Relay(browserWs, agentWs)
}
