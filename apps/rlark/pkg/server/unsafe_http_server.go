package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

func (s *Server) registerUnsafeHTTPHandlers(r *gin.Engine) {
	api := r.Group("/api")
	api.GET("peer/:target", s.handlePeerConnectProxy)

	r.GET("/healthz", s.handleHealthCheck)
	r.GET("/readyz", s.handleHealthCheck)
	r.GET("/livez", s.handleHealthCheck)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
}

func (s *Server) runUnsafeHTTPServer(ctx context.Context) error {
	logger := log.FromContext(ctx)
	r := gin.Default()
	s.registerUnsafeHTTPHandlers(r)

	server := http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.UnsafeHTTPPort),
		Handler: r,
	}

	l, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", server.Addr, err)
	}

	go func() {
		<-ctx.Done()
		logger.Info("Shutting down unsafe HTTP server", "port", s.config.UnsafeHTTPPort)
		if err := server.Shutdown(context.Background()); err != nil {
			logger.Error(nil, "Unsafe HTTP server shutdown error", "err", err)
		}
	}()

	logger.Info("Starting unsafe HTTP server", "port", s.config.UnsafeHTTPPort)
	return server.Serve(l)
}

func (s *Server) handleHealthCheck(ctx *gin.Context) {
	if s.peerBroadcasted {
		ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
	} else {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"status": "not ready"})
	}
}
