package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

func (s *Server) registerHTTPSHandlers(r *gin.Engine) {
	api := r.Group("/api", s.handleCertCheck)

	// Peer connection and proxy endpoints
	api.GET("connect", s.handleProxyConnect)
	api.Any("proxy/:target/*path", s.handleProxy)
	api.Any("podproxy/:target/*path", s.handlePodProxy)
	api.Any("taskproxy/:target/*path", s.handleTaskProxy)
	api.GET("terminal/:target/:namespace/:pod", s.handleTerminalProxy)

	// Sign and revoke certificate endpoints
	api.POST("sign", s.handleSignCertificate)
	api.POST("revoke", s.handleRevokeCertificate)

	// Kubernetes API proxy endpoint
	kubernetes := api.Group("/kubernetes")
	kubernetes.Any("*path", s.handleKubernetesProxy)
}

func (s *Server) runHTTPSServer(ctx context.Context) error {
	logger := log.FromContext(ctx)
	r := gin.Default()
	s.registerHTTPSHandlers(r)

	certPool := x509.NewCertPool()
	for _, ca := range s.ca {
		if ca.Cert != nil {
			certPool.AddCert(ca.Cert)
		}
	}
	tlsCert, err := tls.X509KeyPair(s.tls.CertPEM, s.tls.KeyPEM)
	if err != nil {
		return fmt.Errorf("failed to load server TLS certificate: %w", err)
	}
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.HTTPSPort),
		Handler: r,
		TLSConfig: &tls.Config{
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    certPool,
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{tlsCert},
			NextProtos:   []string{"http/1.1"},
		},
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
	}

	l, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", server.Addr, err)
	}
	tlsListener := tls.NewListener(l, server.TLSConfig)

	go func() {
		<-ctx.Done()
		logger.Info("Shutting down HTTP server", "port", s.config.HTTPSPort)
		if err := server.Shutdown(context.Background()); err != nil {
			logger.Error(nil, "HTTP server shutdown error", "err", err)
		}
	}()

	logger.Info("Starting HTTPS server", "port", s.config.HTTPSPort)
	return server.Serve(tlsListener)
}
