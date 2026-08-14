package agent

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

func (a *Agent) runLocalHTTPServer(ctx context.Context) error {
	logger := log.FromContext(ctx)
	r := gin.Default()
	r.Any("/api/kubernetes/*path", a.handleKubernetesProxy)
	r.GET("/api/terminal/:namespace/:pod", a.handleTerminal)
	r.Any("/api/proxy/*path", a.handleProxy)

	server := http.Server{
		Handler: r,
	}

	go func() {
		<-ctx.Done()
		logger.Info("Shutting down local HTTP server")
		if err := server.Shutdown(context.Background()); err != nil {
			logger.Error(nil, "Local HTTP server shutdown error", "err", err)
		}
	}()

	logger.Info("Starting local HTTP server")
	return server.Serve(a.localListener)
}

func (a *Agent) handleKubernetesProxy(ctx *gin.Context) {
	if a.localKubeHandler == nil {
		ctx.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	ctx.Request.URL.Path = ctx.Param("path")
	a.localKubeHandler.ServeHTTP(ctx.Writer, ctx.Request)
}

func (a *Agent) handleProxy(ctx *gin.Context) {
	target := strings.TrimPrefix(ctx.Param("path"), "/")
	targetUrl, err := url.Parse(target)
	if err != nil {
		ctx.AbortWithStatus(http.StatusBadRequest)
		return
	}
	// Reset the request path to the target URL's path so that
	// NewSingleHostReverseProxy doesn't concatenate the proxy prefix
	// (e.g. /api/proxy/http://host:port) onto the upstream path.
	// Clear targetUrl.Path afterwards, otherwise the reverse proxy joins it
	// with the (now identical) request path again and duplicates it
	// (e.g. /index.js -> /index.js/index.js), which 404s at the upstream.
	// The original query string stays on ctx.Request.URL and is forwarded
	// as-is; do not copy it onto targetUrl or it gets merged twice.
	ctx.Request.URL.Path = targetUrl.Path
	targetUrl.Path = ""
	proxy := httputil.NewSingleHostReverseProxy(targetUrl)
	proxy.ServeHTTP(ctx.Writer, ctx.Request)
}
