package ros2controller

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
// MACVLAN interfaces, and the HTTP gateway.
type Server struct {
	controller *Controller
	httpSrv    *HTTPServer
	srv        *grpc.Server
	socket     string
	macvlans   []*netmac.MACVLAN // created at startup, destroyed at shutdown
}

// ServerConfig holds configuration for the ros2-controller server.
type ServerConfig struct {
	// SocketPath is the Unix socket path the gRPC server listens on.
	// Default: "/var/run/rlark/ros2-ctrl.sock"
	SocketPath string

	// PodIP is the IP address of this pod on the container network.
	// Kept for parity with ROS 1's ServerConfig; in ROS 2 it is not used
	// as ROS_IP (there is no master) but may be used for logging.
	PodIP string

	// HTTPAddr is the optional TCP address ("host:port" or ":port") for the
	// HTTP/JSON gateway. Empty disables HTTP; when set, the HTTP server
	// serves the RobotController REST API under /v1/ and the per-robot
	// reverse proxy under /v1/robots/{robot_id}/proxy/*.
	HTTPAddr string

	// MACVLANs are created at startup and destroyed at shutdown.
	MACVLANs []netmac.MACVLANConfig

	// Types are robot type configurations to register at startup.
	Types []*RobotTypeConfig

	// Robots are robot instance configurations to register at startup.
	Robots []*RobotConfig

	// AllowedLaunchPackages is a whitelist of ROS 2 packages that can be
	// used in custom modes. Empty means no packages are allowed. Use "*"
	// to allow all packages.
	AllowedLaunchPackages []string
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		SocketPath: "/var/run/rlark/ros2-ctrl.sock",
	}
}

// NewServer creates a new ros2-controller server and registers any types and
// robots provided in the config.
func NewServer(cfg ServerConfig) *Server {
	ctrl := NewController(cfg.PodIP, cfg.AllowedLaunchPackages)

	var httpSrv *HTTPServer
	if cfg.HTTPAddr != "" {
		httpSrv = NewHTTPServer(ctrl, cfg.HTTPAddr)
	}

	for _, t := range cfg.Types {
		if err := ctrl.RegisterType(t); err != nil {
			log.Printf("[ros2-controller] WARNING: register type %q: %v", t.Type, err)
		}
	}

	for _, r := range cfg.Robots {
		if err := ctrl.RegisterRobot(r); err != nil {
			log.Printf("[ros2-controller] WARNING: register robot %q: %v", r.ID, err)
		}
		if httpSrv != nil && r.WebService != "" {
			httpSrv.RegisterRobotWeb(r.ID, r.WebService)
		}
	}

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
// the web proxy, then blocks until a shutdown signal is received.
func (s *Server) Run() error {
	for _, cfg := range s.macvlans {
		if err := cfg.Create(); err != nil {
			s.destroyAllMACVLANs()
			return fmt.Errorf("create macvlan %s: %w", cfg.Name, err)
		}
	}
	defer s.destroyAllMACVLANs()

	if err := s.startGRPC(); err != nil {
		return fmt.Errorf("start gRPC server: %w", err)
	}
	defer s.srv.GracefulStop()

	log.Printf("[ros2-controller] gRPC listening on %s", s.socket)

	httpErr := make(chan error, 1)
	if s.httpSrv != nil {
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
		log.Printf("[ros2-controller] received %v, shutting down...", sig)
	case err := <-httpErr:
		log.Printf("[ros2-controller] HTTP server exited: %v", err)
	}

	if s.httpSrv != nil {
		s.httpSrv.Stop()
	}

	s.controller.Shutdown()

	return nil
}

// startGRPC initialises the gRPC server and begins listening on the Unix socket.
func (s *Server) startGRPC() error {
	dir := filepath.Dir(s.socket)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create socket dir %s: %w", dir, err)
	}

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
			log.Printf("[ros2-controller] gRPC server error: %v", err)
		}
	}()

	return nil
}

// destroyAllMACVLANs destroys all MACVLAN interfaces.
func (s *Server) destroyAllMACVLANs() {
	for _, m := range s.macvlans {
		m.Destroy()
	}
}
