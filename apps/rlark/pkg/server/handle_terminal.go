package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

var wsTerminalUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

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

	relayWebSockets(browserWs, agentWs)
}

func relayWebSockets(a, b *websocket.Conn) {
	done := make(chan struct{}, 2)

	copyLoop := func(dst, src *websocket.Conn) {
		defer func() { done <- struct{}{} }()
		for {
			msgType, data, err := src.ReadMessage()
			if err != nil {
				_ = dst.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("connection closed: %v\r\n", err)))
				return
			}
			if err := dst.WriteMessage(msgType, data); err != nil {
				return
			}
		}
	}

	go copyLoop(a, b)
	go copyLoop(b, a)

	<-done
}
