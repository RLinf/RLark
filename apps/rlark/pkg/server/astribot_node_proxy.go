package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"

	"github.com/gin-gonic/gin"

	"github.com/rlinf/rlark/apps/rlark/pkg/auth"
)

// handleNodeProxy answers /api/nodeproxy/:target/:node/*path: the request to one node Agent's local server, through
// that node's own tunnel (agent ID target, Node node) and no other Agent's, so that what is on a node (its services,
// agent.astribotLocalRoutes) is reached on that node or not at all.
func (s *Server) handleNodeProxy(ctx *gin.Context) {
	target, node := ctx.Param("target"), ctx.Param("node")
	if !auth.PermissionChecker.HasAgentProxyPermission(GetCertMetaFromContext(ctx), target) {
		metrics.IncProxyRequest(target, "forbidden")
		ctx.JSON(http.StatusForbidden, gin.H{"error": "client certificate does not have proxy permission for the target"})
		return
	}
	dial, err := s.dialerFactory.GetDialer(ctx, target+":node-agent:"+node)
	if err != nil {
		ctx.JSON(http.StatusBadGateway, gin.H{"error": "the node agent of " + node + " is not connected: " + err.Error()})
		return
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			// The local server's address by convention (GetDial).
			r.Out.URL.Scheme, r.Out.URL.Host, r.Out.URL.Path, r.Out.URL.RawPath = "http", "0.0.0.0:1", ctx.Param("path"), ""
		},
		Transport: &http.Transport{
			DialContext: func(c context.Context, network, _ string) (net.Conn, error) {
				return dial(c, network, "0.0.0.0:1")
			},
			DisableKeepAlives: true,
		},
	}
	proxy.ServeHTTP(ctx.Writer, ctx.Request)
	metrics.IncProxyRequest(target, "ok")
}
