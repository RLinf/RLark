package agent

import (
	"context"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"golang.org/x/sync/errgroup"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// probeRouter answers the agent's probes on --health-probe-bind-address.
func (a *Agent) probeRouter() http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/livez", func(ctx *gin.Context) {
		ctx.Status(http.StatusOK)
	})
	r.GET("/readyz", gin.WrapF(a.handleReady))
	r.POST("/drain", func(ctx *gin.Context) {
		a.startDrain()
		ctx.Status(http.StatusOK)
	})
	return r
}

// metricsRouter serves the agent's metrics, its controllers' included, and pprof on
// --metrics-bind-address.
func (a *Agent) metricsRouter() http.Handler {
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/metrics", gin.WrapH(promhttp.HandlerFor(ctrlmetrics.Registry, promhttp.HandlerOpts{})))
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

// serve runs an HTTP server on address until ctx ends.
func serve(ctx context.Context, address string, handler http.Handler) error {
	srv := &http.Server{Addr: address, Handler: handler}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.FromContext(ctx).Info("Listening", "address", address)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// handleReady answers 200 once every part of the agent has said it is ready.
func (a *Agent) handleReady(w http.ResponseWriter, _ *http.Request) {
	if !a.ready.Load() {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// startAstribot starts what the agent runs in every mode, in the group of its other components. The group has a
// context (agent.go): a component that fails stops the others, so the agent exits and its Pod restarts instead of
// running on (tunnel and lease alive, so the Node shows Online) without that component. Every part this mode runs
// is not ready until it says so. One probe server and one metrics server serve whichever parts this mode runs.
func (a *Agent) startAstribot(ctx context.Context, eg *errgroup.Group) {
	if a.config.Mode == "cluster" || a.config.Mode == "both" {
		a.ready.set(partClusterWatches, false)
	}
	if a.config.Mode == "node" || a.config.Mode == "both" {
		a.ready.set(partNodeWatches, false)
		a.ready.set(partNodeServer, false)
	}
	eg.Go(func() error {
		return serve(ctx, a.config.HealthProbeBindAddress, a.probeRouter())
	})
	eg.Go(func() error {
		return serve(ctx, a.config.MetricsBindAddress, a.metricsRouter())
	})
}
