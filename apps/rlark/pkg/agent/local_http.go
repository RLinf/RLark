package agent

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/http/pprof"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

func (a *Agent) runLocalHTTPServer(ctx context.Context) error {
	logger := log.FromContext(ctx)
	r := a.localHTTPRouter()

	server := http.Server{
		Handler: r,
	}

	logger.Info("Starting local HTTP server")
	return serveHTTP(ctx, &server, func() error { return server.Serve(a.localListener) })
}

// serveHTTP joins its shutdown worker even when binding fails. Shutdown has a
// deadline; force-close remaining connections rather than hanging role join.
func serveHTTP(ctx context.Context, server *http.Server, serve func() error) error {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		<-ctx.Done()
		shutdownCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.FromContext(ctx).Error(err, "HTTP shutdown failed; closing remaining connections")
			if err := server.Close(); err != nil {
				log.FromContext(ctx).Error(err, "HTTP close failed")
			}
		}
	}()
	err := serve()
	cancel()
	<-done
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (a *Agent) localHTTPRouter() http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Any("/api/kubernetes/*path", a.handleKubernetesProxy)
	r.GET("/api/terminal/:namespace/:pod", a.handleTerminal)
	r.Any("/api/proxy/*path", a.handleProxy)
	return r
}

func (a *Agent) healthRouter() http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/livez", func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})
	r.GET("/readyz", func(ctx *gin.Context) {
		if !a.ready.Load() {
			ctx.String(http.StatusServiceUnavailable, "not ready")
			return
		}
		ctx.Status(http.StatusOK)
	})
	r.POST("/drain", func(ctx *gin.Context) {
		a.startDrain()
		ctx.Status(http.StatusOK)
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/debug/pprof/", gin.WrapF(pprof.Index))
	r.GET("/debug/pprof/cmdline", gin.WrapF(pprof.Cmdline))
	r.GET("/debug/pprof/profile", gin.WrapF(pprof.Profile))
	r.POST("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
	r.GET("/debug/pprof/symbol", gin.WrapF(pprof.Symbol))
	r.GET("/debug/pprof/trace", gin.WrapF(pprof.Trace))
	r.GET("/debug/pprof/:profile", func(ctx *gin.Context) {
		pprof.Handler(ctx.Param("profile")).ServeHTTP(ctx.Writer, ctx.Request)
	})
	return r
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
