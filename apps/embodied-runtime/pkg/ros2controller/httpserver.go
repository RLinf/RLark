package ros2controller

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/httpproto"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
)

// ---------------------------------------------------------------------------
// HTTPServer — a REST gateway over the same Controller that backs the gRPC
// RobotController service. It also serves a per-robot reverse proxy under
// /v1/robots/{robot_id}/proxy/* so the REST API and the proxy are one tree.
//
// Endpoints:
//
//   Robots (proto RobotController):
//     GET    /v1/robots                                ListRobots
//     GET    /v1/robots/{robot_id}                     GetRobotStatus
//     POST   /v1/robots/{robot_id}/start               StartRobot  (body: mode, mode_config)
//     POST   /v1/robots/{robot_id}/stop                StopRobot
//     POST   /v1/robots/{robot_id}/mode                SwitchMode   (body: mode, mode_config)
//     POST   /v1/robots/{robot_id}/reset               ResetRobot
//     GET    /v1/robots/{robot_id}/modes               ListModes
//     GET    /v1/robots/{robot_id}/logs?tail=N        GetRobotLogs
//     ANY    /v1/robots/{robot_id}/proxy/*            → robot's web_service
//
//   Packages (proto RobotController):
//     GET    /v1/packages                             ListPackages
//     GET    /v1/packages/{name}                      GetPackageInfo
//     GET    /v1/packages/{name}/launch-files         GetPackageLaunchFiles
//     GET    /v1/packages/{name}/launch-files/{launch_file}/args  GetLaunchFileArgs
// ---------------------------------------------------------------------------

// HTTPServer exposes the Controller (and the per-robot web proxy) over
// HTTP/JSON. It shares the same *Controller as the gRPC server, so HTTP and
// gRPC clients see a consistent view of robots and their state.
type HTTPServer struct {
	ctrl *Controller
	addr string
	srv  *http.Server

	proxyMu     sync.RWMutex
	webServices map[string]string
	transport   *http.Transport
}

// NewHTTPServer creates an HTTP server bound to addr (e.g. ":8080").
func NewHTTPServer(ctrl *Controller, addr string) *HTTPServer {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	return &HTTPServer{
		ctrl:        ctrl,
		addr:        addr,
		webServices: make(map[string]string),
		transport:   tr,
	}
}

// RegisterRobotWeb registers a robot's web service URL so requests under
// /v1/robots/{robot_id}/proxy/* are reverse-proxied to it.
func (s *HTTPServer) RegisterRobotWeb(id, webServiceURL string) {
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	if webServiceURL != "" {
		s.webServices[id] = strings.TrimRight(webServiceURL, "/")
	} else {
		delete(s.webServices, id)
	}
}

// UnregisterRobotWeb removes a robot's web service from the proxy table.
func (s *HTTPServer) UnregisterRobotWeb(id string) {
	s.proxyMu.Lock()
	defer s.proxyMu.Unlock()
	delete(s.webServices, id)
}

// Run starts the HTTP server and blocks until Stop is called or the listener
// fails.
func (s *HTTPServer) Run() error {
	s.srv = &http.Server{
		Addr:              s.addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("[ros2-controller] HTTP server: listening on %s", s.addr)
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Handler returns the HTTP handler (mux with all /v1/ routes and the proxy
// subtree registered), wrapped in a panic-recovery middleware.
func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[ros2-controller] HTTP panic: %v (%s %s)", rec, r.Method, r.URL.Path)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		mux.ServeHTTP(w, r)
	})
}

// Stop gracefully shuts down the HTTP server.
func (s *HTTPServer) Stop() {
	if s.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		log.Printf("[ros2-controller] HTTP server shutdown: %v", err)
	}
}

// registerRoutes wires every proto RPC to its HTTP handler and mounts the
// per-robot web proxy under /v1/robots/{robot_id}/proxy.
func (s *HTTPServer) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/robots", s.handleListRobots)
	mux.HandleFunc("GET /v1/robots/{robot_id}", s.handleGetRobotStatus)
	mux.HandleFunc("POST /v1/robots/{robot_id}/start", s.handleStartRobot)
	mux.HandleFunc("POST /v1/robots/{robot_id}/stop", s.handleStopRobot)
	mux.HandleFunc("POST /v1/robots/{robot_id}/mode", s.handleSwitchMode)
	mux.HandleFunc("POST /v1/robots/{robot_id}/reset", s.handleResetRobot)
	mux.HandleFunc("GET /v1/robots/{robot_id}/modes", s.handleListModes)
	mux.HandleFunc("GET /v1/robots/{robot_id}/logs", s.handleGetRobotLogs)

	mux.HandleFunc("/v1/robots/{robot_id}/proxy/", s.handleProxy)
	mux.HandleFunc("/v1/robots/{robot_id}/proxy", s.handleProxy)

	mux.HandleFunc("GET /v1/packages", s.handleListPackages)
	mux.HandleFunc("GET /v1/packages/{name}", s.handleGetPackageInfo)
	mux.HandleFunc("GET /v1/packages/{name}/launch-files", s.handleGetPackageLaunchFiles)
	mux.HandleFunc("GET /v1/packages/{name}/launch-files/{launch_file}/args", s.handleGetLaunchFileArgs)
}

// ---------------------------------------------------------------------------
// Handlers — Robots
// ---------------------------------------------------------------------------

func (s *HTTPServer) handleListRobots(w http.ResponseWriter, r *http.Request) {
	resp, err := s.ctrl.ListRobots(r.Context(), &pb.ListRobotsRequest{})
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleGetRobotStatus(w http.ResponseWriter, r *http.Request) {
	req := &pb.GetRobotStatusRequest{RobotId: r.PathValue("robot_id")}
	resp, err := s.ctrl.GetRobotStatus(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleStartRobot(w http.ResponseWriter, r *http.Request) {
	robotID := r.PathValue("robot_id")
	req := &pb.StartRobotRequest{}
	if err := httpproto.DecodeBody(r, req); err != nil {
		httpproto.WriteError(w, err)
		return
	}
	req.RobotId = robotID
	resp, err := s.ctrl.StartRobot(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleStopRobot(w http.ResponseWriter, r *http.Request) {
	req := &pb.StopRobotRequest{RobotId: r.PathValue("robot_id")}
	resp, err := s.ctrl.StopRobot(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleSwitchMode(w http.ResponseWriter, r *http.Request) {
	robotID := r.PathValue("robot_id")
	req := &pb.SwitchModeRequest{}
	if err := httpproto.DecodeBody(r, req); err != nil {
		httpproto.WriteError(w, err)
		return
	}
	req.RobotId = robotID
	resp, err := s.ctrl.SwitchMode(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleResetRobot(w http.ResponseWriter, r *http.Request) {
	req := &pb.ResetRobotRequest{RobotId: r.PathValue("robot_id")}
	resp, err := s.ctrl.ResetRobot(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleListModes(w http.ResponseWriter, r *http.Request) {
	req := &pb.ListModesRequest{RobotId: r.PathValue("robot_id")}
	resp, err := s.ctrl.ListModes(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleGetRobotLogs(w http.ResponseWriter, r *http.Request) {
	var tail int32
	if v := r.URL.Query().Get("tail"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil {
			tail = int32(n)
		}
	}
	req := &pb.GetRobotLogsRequest{RobotId: r.PathValue("robot_id"), Tail: tail}
	resp, err := s.ctrl.GetRobotLogs(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Handlers — Packages
// ---------------------------------------------------------------------------

func (s *HTTPServer) handleListPackages(w http.ResponseWriter, r *http.Request) {
	resp, err := s.ctrl.ListPackages(r.Context(), &pb.ListPackagesRequest{})
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleGetPackageInfo(w http.ResponseWriter, r *http.Request) {
	req := &pb.GetPackageInfoRequest{Name: r.PathValue("name")}
	resp, err := s.ctrl.GetPackageInfo(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleGetPackageLaunchFiles(w http.ResponseWriter, r *http.Request) {
	req := &pb.GetPackageLaunchFilesRequest{Name: r.PathValue("name")}
	resp, err := s.ctrl.GetPackageLaunchFiles(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

func (s *HTTPServer) handleGetLaunchFileArgs(w http.ResponseWriter, r *http.Request) {
	req := &pb.GetLaunchFileArgsRequest{
		Package:    r.PathValue("name"),
		LaunchFile: r.PathValue("launch_file"),
	}
	resp, err := s.ctrl.GetLaunchFileArgs(r.Context(), req)
	if err != nil {
		httpproto.WriteError(w, err)
		return
	}
	httpproto.WriteProto(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Handler — per-robot web proxy
// ---------------------------------------------------------------------------

func (s *HTTPServer) handleProxy(w http.ResponseWriter, r *http.Request) {
	robotID := r.PathValue("robot_id")

	s.proxyMu.RLock()
	targetBase, ok := s.webServices[robotID]
	s.proxyMu.RUnlock()

	if !ok || targetBase == "" {
		http.NotFound(w, r)
		return
	}

	targetURL, err := url.Parse(targetBase)
	if err != nil {
		http.Error(w, "invalid target URL", http.StatusInternalServerError)
		return
	}

	mountPrefix := httpproto.ProxyMountPrefix("robots", robotID)
	rewritePath := strings.TrimPrefix(r.URL.Path, mountPrefix)

	proxy := &httputil.ReverseProxy{
		Transport: s.transport,
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			req.URL.Path = rewritePath
			if r.URL.RawQuery != "" {
				req.URL.RawQuery = r.URL.RawQuery
			}
			if _, ok := req.Header["User-Agent"]; !ok {
				req.Header.Set("User-Agent", "")
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			loc := resp.Header.Get("Location")
			if loc == "" {
				return nil
			}
			resp.Header.Set("Location", httpproto.RewriteProxyLocation(loc, mountPrefix, targetURL))
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[ros2-controller] web proxy: %s → %s: %v", robotID, targetBase, err)
			http.Error(w, "bad gateway", http.StatusBadGateway)
		},
	}

	proxy.ServeHTTP(w, r)
}
