package server

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rancher/remotedialer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/server/reverseproxy"
)

// GetDial returns a remotedialer.Dialer based on the provided dialType and address.
// It also returns the address to be used for dialing and any error encountered during the process.
func (s *Server) GetDial(ctx context.Context, dialType, address string, certMeta map[string]string) (remotedialer.Dialer, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, "", fmt.Errorf("split host and port from address %s: %w", address, err)
	}

	switch dialType {
	case "default":
		// default 对应默认的代理，一般用于连接 agent 的 local server
		// 这种情况下，host 即为 agent ID，此时应该返回 agent 对应的 dialer 和 agent 的 local server 地址
		// 这种情况下不会将请求的 certMeta 传进来，因此需要在外层完成权限检查
		agentID := host
		dialer := s.getAgentDialer(ctx, agentID, "")
		return dialer, "0.0.0.0:1", nil // 约定 local server 地址为 0.0.0.0:1

	case "ssh":
		if certMeta == nil || certMeta[apis.MetaCertRole] == "" {
			return nil, "", fmt.Errorf("client certificate does not contain required role for ssh proxying")
		}
		switch certMeta[apis.MetaCertRole] {
		case "domain":
			// 以 domain 角色连接的客户端，允许连接到同一个 domain 下的 pod
			targetAgent, targetNode, targetHost, err := s.getTargetFromDomainHost(ctx, host)
			if err != nil {
				return nil, "", fmt.Errorf("failed to get target from domain host %s: %w", host, err)
			}
			// 检查证书的 domain 身份是否具有访问目标的权限
			if domainID, ok := auth.PermissionChecker.HasDomainProxyPermission(certMeta); ok {
				hasPermission, domainCheckErr := s.checkHostInDomain(ctx, targetHost, domainID, targetAgent)
				if domainCheckErr != nil {
					return nil, "", fmt.Errorf("failed to check domain proxy permission: %w", domainCheckErr)
				}
				if !hasPermission {
					return nil, "", fmt.Errorf("client certificate does not have proxy permission for %s on %s", targetHost, targetAgent)
				}
			} else {
				return nil, "", fmt.Errorf("client certificate does not have domain proxy permission for %s on %s", targetHost, targetAgent)
			}
			dialer := s.getAgentDialer(ctx, targetAgent, targetNode)

			return dialer, net.JoinHostPort(targetHost, port), nil

		case "ssh-guest":
			// 以 ssh-guest 角色连接的客户端，允许连接到该用户有权限访问的 Pod
			// 连接目标格式：podName
			// 需要自动识别目标 Pod 所在的 agent，并使用该 agent 的 dialer 来连接
			agentID, nodeName, targetHost, err := s.getPodDialInfoByUser(ctx, host, certMeta[apis.MetaUserID])
			if err != nil {
				return nil, "", fmt.Errorf("failed to get pod %v: %w", host, err)
			}
			dialer := s.getAgentDialer(ctx, agentID, nodeName)
			return dialer, net.JoinHostPort(targetHost, port), nil
		}
		return nil, "", fmt.Errorf("unsupported role %s for ssh proxying", certMeta[apis.MetaCertRole])
	}
	return nil, "", fmt.Errorf("unsupported dial type: %s", dialType)
}

// getAgentDialer returns a remotedialer.Dialer for the specified agentID and nodeName.
// If nodeName is provided, it will prioritize the dialer for that specific node.
func (s *Server) getAgentDialer(ctx context.Context, agentID, nodeName string) remotedialer.Dialer {
	candidateClientKeys := []string{}
	if nodeName != "" {
		candidateClientKeys = append(candidateClientKeys, agentID+":node-agent:"+nodeName)
	}
	candidateClientKeys = append(candidateClientKeys, agentID)

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		for _, clientKey := range candidateClientKeys {
			d := s.dialerFactory.GetDialer(ctx, clientKey)
			conn, err := d(ctx, network, addr)
			if err == nil {
				return conn, nil
			}
			if !strings.Contains(err.Error(), "failed to find Session") {
				return nil, err
			}
		}
		return nil, fmt.Errorf("no available dialer found for candidate client keys: %v", candidateClientKeys)
	}
}

// checkHostInDomain 检查指定的 host 是否在指定的 domain 下，并返回该 host 的 LocalIP（如果存在）。
func (s *Server) checkHostInDomain(ctx context.Context, host string, domainID, agentID string) (bool, error) {
	namespace := apis.RLarkAgentNamespacePrefix + agentID
	dp, err := s.rlarkClient.RlinfV1alpha1().DomainPeers(namespace).Get(ctx, domainID, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("get domain peer %s: %w", domainID, err)
	}
	for _, pod := range dp.Spec.Pods {
		if pod.LocalIP == host {
			return true, nil
		}
	}
	return false, nil
}

// getTargetFromDomainHost 解析 domain host，返回对应的 targetAgent、targetNode 和 targetHost。
func (s *Server) getTargetFromDomainHost(ctx context.Context, host string) (string, string, string, error) {
	fields := strings.Split(host, ".")
	if len(fields) < 3 {
		return "", "", "", fmt.Errorf("invalid host format: %s", host)
	}
	switch fields[len(fields)-1] {
	case "agent":
		// 连接目标格式：<targetHost>.<targetAgentID>.agent
		targetID := fields[len(fields)-2]
		targetHost := strings.Join(fields[:len(fields)-2], ".")
		return targetID, "", targetHost, nil

	case "agent-node":
		// 连接目标格式：<targetHost>.<targetNodeName>.<targetAgentID>.agent-node
		targetID := fields[len(fields)-2]
		targetNode := fields[len(fields)-3]
		targetHost := strings.Join(fields[:len(fields)-3], ".")
		return targetID, targetNode, targetHost, nil
	}

	return "", "", "", fmt.Errorf("invalid target type: %s", fields[len(fields)-1])
}

// getPodInfoByUser 根据 podName 和 userName 获取对应的 Pod 信息，
// 返回 agentID、nodeName、podIP，如果 Pod 不存在或不属于该用户，则返回错误。
func (s *Server) getPodDialInfoByUser(ctx context.Context, podName, userName string) (string, string, string, error) {
	pod, ok := s.podCache.GetPodByName(podName)
	if !ok {
		return "", "", "", fmt.Errorf("pod %s not found", podName)
	}
	if pod.Status.IP == "" {
		return "", "", "", fmt.Errorf("pod %s not ready", podName)
	}
	// TODO: 根据 userName 来检查该用户是否有权限访问该 Pod。
	// 暂时完全放行
	if agentID, ok := strings.CutPrefix(pod.Namespace, apis.RLarkAgentNamespacePrefix); ok {
		return agentID, pod.Status.Node, pod.Status.IP, nil
	}
	return "", "", "", fmt.Errorf("unknown error")
}

// handleProxyConnect 处理反向代理隧道的连接请求。它会根据客户端证书中的元数据来确定是代理连接还是 Peer-to-Peer 连接，并设置相应的请求头。
func (s *Server) handleProxyConnect(ctx *gin.Context) {
	logger := log.FromContext(ctx)

	certMeta := GetCertMetaFromContext(ctx)
	clientID := certMeta[apis.MetaRemoteDialerClientID]
	if clientID != "" {
		clientKey := clientID
		if role := ctx.Request.Header.Get(apis.RemoteDialerRoleHeader); role != "" {
			clientKey = clientID + ":" + role
		}
		reverseproxy.SetClientHeader(ctx.Request, clientKey)
	} else {
		peerID := certMeta[apis.MetaRemoteDialerPeerID]
		peerToken := certMeta[apis.MetaRemoteDialerPeerToken]
		if peerID != "" && peerToken != "" {
			reverseproxy.SetPeerHeaders(ctx.Request, peerID, peerToken)
		} else {
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "client certificate does not contain required permissions"})
			return
		}
	}

	if agentID := certMeta[apis.MetaAgentID]; agentID != "" {
		// 如果是 Agent 接入，需要检查是否完成该 Agent 的注册流程
		if err := s.registerAgent(ctx.Request.Context(), agentID); err != nil {
			logger.Error(err, "Agent registration failed", "agentID", agentID)
			ctx.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("agent registration failed: %v", err)})
			return
		}
		// 在接入期间，启动 Broadcast 机制，向集群广播该 Agent 的存在信息
		role := ctx.Request.Header.Get(apis.RemoteDialerRoleHeader)
		if err := s.startAgentBroadcaster(ctx.Request.Context(), agentID, role, uuid.NewString()); err != nil {
			logger.Error(err, "Start agent broadcaster failed", "agentID", agentID)
			ctx.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("start agent broadcaster failed: %v", err)})
			return
		}
	}

	s.dialerFactory.ServeHTTP(ctx.Writer, ctx.Request)
}

// 由于 remotedialer 默认的 Peer-to-Peer 连接无法进行任何证书配置，所以这里在 http server 实现了一个代理
// 会将请求转发到目标 Peer 的 /api/connect 接口上，并且在请求中会携带专用于 Peer-to-Peer 连接的证书元数据.
func (s *Server) handlePeerConnectProxy(ctx *gin.Context) {
	target := ctx.Param("target")
	url := &url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("%s:%d", target, s.config.HTTPSPort),
		Path:   "/api/connect",
	}
	proxy := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL = url
			req.Host = url.Host
		},
		Transport: s.defaultPeerTransport,
	}
	proxy.ServeHTTP(ctx.Writer, ctx.Request)
	metrics.IncProxyRequest(target, "peer")
}

// handleProxy 处理通过服务器代理的 HTTP 请求。它会根据请求的目标和路径，使用默认的代理传输将请求转发到目标地址。
// target: 目标集群名称
// path: 要访问的目标集群 Agent 的 Local Server 的路径
func (s *Server) handleProxy(ctx *gin.Context) {
	// 对应 GetDial 参数为
	// - dialType = "default"
	// - address = ctx.Param("target")
	target := ctx.Param("target")
	certMeta := GetCertMetaFromContext(ctx)
	if !auth.PermissionChecker.HasAgentProxyPermission(certMeta, target) {
		metrics.IncProxyRequest(target, "forbidden")
		ctx.JSON(http.StatusForbidden, gin.H{"error": "client certificate does not have proxy permission for the target"})
		return
	}

	path := ctx.Param("path")

	ctx.Request.URL.Path = path
	url := &url.URL{
		Scheme: cmp.Or(ctx.Request.Header.Get("Proxy-Scheme"), "http"),
		Host:   target,
	}
	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Transport = s.defaultProxyTransport
	proxy.ServeHTTP(ctx.Writer, ctx.Request)
	metrics.IncProxyRequest(target, "ok")
}

func (s *Server) handlePodProxyForPod(ctx *gin.Context, pod *v1alpha1.Pod, port string) {
	if pod.Status.IP == "" {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": fmt.Sprintf("pod %s not ready", pod.Spec.PodName)})
		return
	}
	targetAgent := strings.TrimPrefix(pod.Namespace, apis.RLarkAgentNamespacePrefix)
	certMeta := GetCertMetaFromContext(ctx)
	if !auth.PermissionChecker.HasAgentProxyPermission(certMeta, targetAgent) {
		metrics.IncProxyRequest(targetAgent, "forbidden")
		ctx.JSON(http.StatusForbidden, gin.H{"error": "client certificate does not have proxy permission for the target"})
		return
	}

	// 构造从目标 Agent 的 /api/proxy 接口转发到 Pod 的请求路径
	path := ctx.Param("path")
	ctx.Request.URL.Path = "/api/proxy/http://" + pod.Status.IP + ":" + port + path
	url := &url.URL{
		Scheme: cmp.Or(ctx.Request.Header.Get("Proxy-Scheme"), "http"),
		Host:   targetAgent,
	}
	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.Transport = s.defaultProxyTransport
	proxy.ServeHTTP(ctx.Writer, ctx.Request)
	metrics.IncProxyRequest(targetAgent, "ok")
}

// handlePodProxy 处理通过服务器代理的 Pod 请求。
// target: 目标 Pod 的名称 + 端口
// path: 要访问的目标 Pod 的路径
func (s *Server) handlePodProxy(ctx *gin.Context) {
	target := ctx.Param("target")
	podName, port, err := net.SplitHostPort(target)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid target format: %v", err)})
		return
	}
	pod, ok := s.podCache.GetPodByName(podName)
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("pod %s not found", podName)})
		return
	}
	s.handlePodProxyForPod(ctx, pod, port)
}

// handleTaskProxy 处理通过服务器代理的 Task 请求。
// target: 目标 Task 的名称 + 端口
// path: 要访问的目标 Task 的路径
func (s *Server) handleTaskProxy(ctx *gin.Context) {
	target := ctx.Param("target")
	taskName, port, err := net.SplitHostPort(target)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid target format: %v", err)})
		return
	}
	pod, ok := s.podCache.GetPodByTaskName(taskName)
	if !ok {
		ctx.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("task %s not found", taskName)})
		return
	}
	s.handlePodProxyForPod(ctx, pod, port)
}

// handleKubernetesProxy 处理通过服务器代理的 Kubernetes API 请求。它会根据客户端证书中的元数据，
// 设置相应的 impersonation 头，并将请求转发到 Kubernetes API 服务器。
func (s *Server) handleKubernetesProxy(ctx *gin.Context) {
	// 只有提供了 "kubernetes-impersonation" 元数据的证书才允许使用 Kubernetes 代理功能
	// 如果证书中 "kubernetes-impersonation" 的值为 "-"，则表示不进行任何 impersonation

	certMeta := GetCertMetaFromContext(ctx)
	isAdmin := auth.PermissionChecker.IsAdmin(certMeta)
	if certMeta[apis.MetaKubernetesImpersonation] == "" {
		if isAdmin {
			certMeta[apis.MetaKubernetesImpersonation] = "-"
		} else {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "client certificate does not allow Kubernetes proxying"})
			return
		}
	}

	header := make(http.Header)
	if impersonation := certMeta[apis.MetaKubernetesImpersonation]; impersonation != "-" {
		header.Set("Impersonate-User", impersonation)
	}
	if impersonationGroup := certMeta[apis.MetaKubernetesImpersonationGroup]; impersonationGroup != "" {
		groups := strings.Split(impersonationGroup, ",")
		for i := range groups {
			groups[i] = strings.TrimSpace(groups[i])
		}
		for _, impersonationGroup := range groups {
			header.Add("Impersonate-Group", impersonationGroup)
		}
	}
	if impersonationUid := certMeta[apis.MetaKubernetesImpersonationUID]; impersonationUid != "" {
		header.Set("Impersonate-Uid", impersonationUid)
	}
	for k, v := range certMeta {
		if after, ok := strings.CutPrefix(k, apis.MetaKubernetesImpersonationExtraPrefix); ok {
			header.Set("Impersonate-Extra-"+after, v)
		}
	}

	ctx.Request.URL.Path = ctx.Param("path")
	maps.Copy(ctx.Request.Header, header)
	s.kubeHandler.ServeHTTP(ctx.Writer, ctx.Request)
}
