package roscontroller

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/netmac"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc"
)

// Server wraps the gRPC server and manages the lifecycle of the controller,
// MACVLAN interfaces, per-robot roscore instances, and the HTTP gateway.
type Server struct {
	controller *Controller
	httpSrv    *HTTPServer
	srv        *grpc.Server
	socket     string
	macvlans   []*netmac.MACVLAN // created at startup, destroyed at shutdown
}

// ServerConfig holds configuration for the ros-controller server.
type ServerConfig struct {
	// SocketPath is the Unix socket path the gRPC server listens on.
	// Default: "/var/run/rlark/ros-ctrl.sock"
	SocketPath string

	// PodIP is the IP address of this pod on the container network.
	// The robot node, running on host network via nsenter, uses this IP
	// to connect back to the ROS Master and to register itself.
	// Injected as ROS_IP into every roslaunch process.
	PodIP string

	// HTTPAddr is the optional TCP address ("host:port" or ":port") for the
	// HTTP/JSON gateway. Copied from ControllerConfig.HTTPAddr (config file)
	// and overridable via --http-addr. Empty disables HTTP; when set, the
	// HTTP server serves the RobotController REST API under /v1/ and the
	// per-robot reverse proxy under /v1/robots/{robot_id}/proxy/*.
	// Mirrors pkg/cameracontroller.ServerConfig.HTTPAddr.
	HTTPAddr string

	// MACVLANs are created at startup and destroyed at shutdown.
	// They give the container direct L2 access to the robot's physical
	// network. Multiple robots can share the same macvlan interface.
	MACVLANs []MACVLANConfig

	// Types are robot type configurations to register at startup.
	Types []*RobotTypeConfig

	// Robots are robot instance configurations to register at startup.
	Robots []*RobotConfig

	// AllowedLaunchPackages is a whitelist of ROS packages that can be used
	// in custom modes (rosctr start --package). Empty means no packages are
	// allowed (custom modes disabled). Use "*" to allow all packages.
	// This is a security control to prevent arbitrary package execution.
	AllowedLaunchPackages []string
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		SocketPath: "/var/run/rlark/ros-ctrl.sock",
	}
}

// NewServer creates a new ros-controller server and registers any types and
// robots provided in the config.
func NewServer(cfg ServerConfig) *Server {
	ctrl := NewController(cfg.PodIP, cfg.AllowedLaunchPackages)

	// Set up the unified HTTP gateway (REST + web proxy) if configured.
	// The gateway serves both the /v1/ RobotController REST API and the
	// per-robot reverse proxy at /v1/robots/{robot_id}/proxy/*. Robots
	// register their web_service URLs with the gateway as they're
	// registered below; if no robot has a web_service the proxy simply
	// returns 404 and the REST API still works.
	var httpSrv *HTTPServer
	if cfg.HTTPAddr != "" {
		httpSrv = NewHTTPServer(ctrl, cfg.HTTPAddr)
	}

	// Register types from bootstrap config.
	for _, t := range cfg.Types {
		if err := ctrl.RegisterType(t); err != nil {
			log.Printf("[ros-controller] WARNING: register type %q: %v", t.Type, err)
		}
	}

	// Register robots from bootstrap config.
	for _, r := range cfg.Robots {
		if err := ctrl.RegisterRobot(r); err != nil {
			log.Printf("[ros-controller] WARNING: register robot %q: %v", r.ID, err)
		}
		// Register robot web service with the HTTP gateway's proxy table.
		if httpSrv != nil && r.WebService != "" {
			httpSrv.RegisterRobotWeb(r.ID, r.WebService)
		}
	}

	// Build MACVLAN objects from config (created later in Run).
	macvlans := make([]*netmac.MACVLAN, 0, len(cfg.MACVLANs))
	for _, m := range cfg.MACVLANs {
		macvlans = append(macvlans, netmac.NewMACVLAN(m))
	}

	return &Server{
		controller: ctrl,
		httpSrv:    httpSrv,
		socket:     cfg.SocketPath,
		macvlans:   macvlans,
	}
}

// RegisterType adds a robot type configuration. Must be called before Run().
func (s *Server) RegisterType(cfg *RobotTypeConfig) error {
	return s.controller.RegisterType(cfg)
}

// RegisterRobot adds a robot instance configuration. Must be called before Run().
func (s *Server) RegisterRobot(cfg *RobotConfig) error {
	return s.controller.RegisterRobot(cfg)
}

// Run starts the gRPC server, creates MACVLAN interfaces, optionally starts
// the web proxy, then blocks until a shutdown signal is received. Per-robot
// roscore instances are started on-demand when robots are launched, and
// cleaned up on shutdown.
func (s *Server) Run() error {
	// 1. Create MACVLAN interfaces (infrastructure — live for the entire pod).
	for _, cfg := range s.macvlans {
		if err := cfg.Create(); err != nil {
			s.destroyAllMACVLANs()
			return fmt.Errorf("create macvlan %s: %w", cfg.Name, err)
		}
	}
	defer s.destroyAllMACVLANs()

	// 2. Start gRPC server
	if err := s.startGRPC(); err != nil {
		return fmt.Errorf("start gRPC server: %w", err)
	}
	defer s.srv.GracefulStop()

	log.Printf("[ros-controller] gRPC listening on %s", s.socket)

	// 3. Start HTTP gateway (REST + web proxy, optional). Shares the same
	// Controller as gRPC so HTTP and gRPC clients see a consistent view.
	httpErr := make(chan error, 1)
	if s.httpSrv != nil {
		go func() {
			if err := s.httpSrv.Run(); err != nil {
				httpErr <- err
			}
		}()
	}

	// 4. Wait for shutdown signal (or an HTTP server failure).
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-sigCh:
		log.Printf("[ros-controller] received %v, shutting down...", sig)
	case err := <-httpErr:
		log.Printf("[ros-controller] HTTP server exited: %v", err)
	}

	// 5. Stop HTTP gateway so no new requests arrive while gRPC drains.
	if s.httpSrv != nil {
		s.httpSrv.Stop()
	}

	// 6. Stop all per-robot roscore instances
	s.controller.Shutdown()

	return nil
}

// startGRPC initialises the gRPC server and begins listening on the Unix socket.
func (s *Server) startGRPC() error {
	// Ensure the socket directory exists.
	dir := filepath.Dir(s.socket)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create socket dir %s: %w", dir, err)
	}

	// Remove stale socket file.
	if err := os.Remove(s.socket); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket %s: %w", s.socket, err)
	}

	listener, err := net.Listen("unix", s.socket)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socket, err)
	}

	s.srv = grpc.NewServer()
	pb.RegisterRobotControllerServer(s.srv, s.controller)

	go func() {
		if err := s.srv.Serve(listener); err != nil {
			log.Printf("[ros-controller] gRPC server error: %v", err)
		}
	}()

	return nil
}

// destroyAllMACVLANs destroys all MACVLAN interfaces. Safe to call on any
// state (nil, partial creation, etc.) and on any error.
func (s *Server) destroyAllMACVLANs() {
	for _, m := range s.macvlans {
		m.Destroy()
	}
}
