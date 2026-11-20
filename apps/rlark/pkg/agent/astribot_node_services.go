package agent

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"path"

	"github.com/gin-gonic/gin"

	"github.com/rlinf/rlark/apps/rlark/astribot/nodeagent"
)

// astribotLocalRoutes adds the node's own services to the Agent's local server, which the Server reaches through
// this node Agent's tunnel only (/api/nodeproxy/<agent>/<node>/...): a service an Addon runs on the node listens on
// a Unix socket under the host's /run/rlark/services (a robot's power, s1u-power.sock), never on the network, and is
// reached as /api/astribot/services/<name>/<its path>. Who may call it is the Gateway's to decide.
// nodeServices is the host's /run/rlark/services as the node Agent sees it.
var nodeServices = nodeagent.HostRun + "/rlark/services"

func (a *Agent) astribotLocalRoutes(r gin.IRouter) {
	r.Any("/api/astribot/services/:service/*path", handleNodeService)
}

func handleNodeService(ctx *gin.Context) {
	socket := path.Join(nodeServices, ctx.Param("service")+".sock")
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL.Scheme, r.Out.URL.Host, r.Out.URL.Path, r.Out.URL.RawPath = "http", "localhost", ctx.Param("path"), ""
			r.Out.Host = "localhost"
		},
		Transport: &http.Transport{
			DialContext: func(c context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(c, "unix", socket)
			},
			DisableKeepAlives: true,
		},
	}
	proxy.ServeHTTP(ctx.Writer, ctx.Request)
}
