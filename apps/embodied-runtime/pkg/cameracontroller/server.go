package cameracontroller

import (
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"google.golang.org/grpc"
)

// Server wraps the gRPC server and manages camera lifecycle.
type Server struct {
	ctrl    *Controller
	srv     *grpc.Server
	httpSrv *HTTPServer
	socket  string
}

// ServerConfig holds the runtime configuration for the camera-controller
// server. It is assembled by the cmd from a ControllerConfig (loaded from the
// YAML file) plus CLI flags (SocketPath, HTTPAddr) — it is NOT the file
// format and carries no yaml tags. Mirrors roscontroller.ServerConfig.
type ServerConfig struct {
	// SocketPath is the Unix socket path the gRPC server listens on.
	// Default: "/var/run/rlark/camera-ctrl.sock" (set by the cmd from the
	// --socket-path flag; not a config-file field).
	SocketPath string

	// HTTPAddr is the optional TCP address ("host:port" or ":port") for the
	// HTTP/JSON gateway. Copied from ControllerConfig.HTTPAddr (config file)
	// and overridable via --http-addr. Empty disables HTTP.
	HTTPAddr string

	// Cameras are camera device configurations to register at startup
	// (copied from ControllerConfig.Cameras).
	Cameras []CameraConfig
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		SocketPath: "/var/run/rlark/camera-ctrl.sock",
	}
}

// NewServer creates a new camera-controller server from configuration.
// Cameras are registered locally from cfg.Cameras.
func NewServer(cfg ServerConfig) (*Server, error) {
	ctrl := New()
	for _, c := range cfg.Cameras {
		if err := ctrl.RegisterCamera(c); err != nil {
			log.Printf("[camera-controller] WARNING: register camera %q: %v", c.ID, err)
		}
	}

	return &Server{
		ctrl:    ctrl,
		socket:  cfg.SocketPath,
		httpSrv: NewHTTPServer(ctrl, cfg.HTTPAddr),
	}, nil
}

// Run starts the gRPC server (and the HTTP gateway if HTTPAddr is set) and
// blocks until a shutdown signal is received.
func (s *Server) Run() error {
	if err := s.startGRPC(); err != nil {
		return err
	}
	defer s.srv.GracefulStop()

	log.Printf("[camera-controller] gRPC listening on %s", s.socket)

	// Start the HTTP gateway in the background. It shares the same Controller
	// as gRPC, so HTTP and gRPC clients see a consistent view of the cameras.
	// A non-empty HTTPAddr is the only trigger; otherwise HTTP is disabled and
	// the server behaves exactly as before.
	httpErr := make(chan error, 1)
	if s.httpSrv.addr != "" {
		go func() {
			if err := s.httpSrv.Run(); err != nil {
				httpErr <- err
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		log.Printf("[camera-controller] received %v, shutting down...", sig)
	case err := <-httpErr:
		log.Printf("[camera-controller] HTTP server exited: %v", err)
	}

	// Stop the HTTP gateway first so no new requests arrive while gRPC is
	// draining. Then close all capture pipelines (kill ffmpeg/V4L2
	// subprocesses) before stopping gRPC so no child process is left behind.
	s.httpSrv.Stop()
	s.ctrl.CloseAll()

	return nil
}

// startGRPC initialises the gRPC server and begins listening on the Unix socket.
func (s *Server) startGRPC() error {
	dir := filepath.Dir(s.socket)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if err := os.Remove(s.socket); err != nil && !os.IsNotExist(err) {
		return err
	}

	listener, err := net.Listen("unix", s.socket)
	if err != nil {
		return err
	}

	s.srv = grpc.NewServer()
	pb.RegisterCameraControllerServer(s.srv, s.ctrl)

	go func() {
		if err := s.srv.Serve(listener); err != nil {
			log.Printf("[camera-controller] gRPC server error: %v", err)
		}
	}()

	return nil
}
