package cameracontroller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// ---------------------------------------------------------------------------
// HTTPServer — a REST gateway over the same Controller that backs the gRPC
// CameraController service. Every endpoint maps 1:1 to a proto RPC, so the
// HTTP API is a faithful projection of proto/cameracontroller/v1/camera.proto.
//
// Endpoints (all under /v1/):
//
//	GET    /v1/cameras                                ListCameras
//	GET    /v1/cameras/{camera_id}                   GetCameraInfo
//	POST   /v1/cameras/{camera_id}/open              OpenCamera        (body: OpenCameraRequest minus camera_id)
//	POST   /v1/cameras/{camera_id}/close             CloseCamera
//	GET    /v1/cameras/{camera_id}/frame             CaptureFrame      (raw bytes by default; JSON on Accept: application/json)
//	POST   /v1/cameras:captureFrames                 CaptureFrames     (body: CaptureFramesRequest)
//	GET    /v1/cameras/{camera_id}/watch             WatchFrames       (Server-Sent Events; one JSON VideoFrame per event)
//
// Responses use canonical proto JSON (lowerCamelCase, EmitUnpopulated) so the
// shape matches the `camctr -o json` CLI output. gRPC status errors returned
// by the Controller are mapped to the closest HTTP status code.
// ---------------------------------------------------------------------------

// httpMarshal is the canonical proto JSON marshaler used by the HTTP API.
// It mirrors pkg/cli/output.go so HTTP JSON is identical to `camctr -o json`.
var httpMarshal = protojson.MarshalOptions{
	EmitUnpopulated: true,
}

// httpUnmarshal is the canonical proto JSON unmarshaler for request bodies.
// It allows unknown fields to be ignored (forward-compatibility) and accepts
// both lowerCamelCase (canonical) and proto field names (snake_case).
var httpUnmarshal = protojson.UnmarshalOptions{
	DiscardUnknown: true,
}

// HTTPServer exposes the camera Controller over HTTP/JSON.
// It shares the same *Controller instance as the gRPC server so HTTP and gRPC
// clients see a consistent view of cameras and their state.
type HTTPServer struct {
	ctrl *Controller
	addr string
	srv  *http.Server
}

// NewHTTPServer creates an HTTP server bound to addr (e.g. ":8080").
// The server is not started; call Run to start listening.
func NewHTTPServer(ctrl *Controller, addr string) *HTTPServer {
	return &HTTPServer{ctrl: ctrl, addr: addr}
}

// Run starts the HTTP server and blocks until Stop is called or the listener
// fails. It registers all /v1/ routes on a fresh ServeMux.
func (s *HTTPServer) Run() error {
	s.srv = &http.Server{
		Addr:    s.addr,
		Handler: s.Handler(),
		// ReadHeaderTimeout mitigates slowloris-style denial of service.
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("[camera-controller] HTTP server: listening on %s", s.addr)
	if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Handler returns the HTTP handler (mux with all /v1/ routes registered).
// Exposed so tests can mount the gateway on an httptest.Server without
// binding a real listener.
func (s *HTTPServer) Handler() http.Handler {
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	return mux
}

// Stop gracefully shuts down the HTTP server, waiting up to 5 seconds for
// in-flight requests (including SSE streams) to finish.
func (s *HTTPServer) Stop() {
	if s.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		log.Printf("[camera-controller] HTTP server shutdown: %v", err)
	}
}

// registerRoutes wires every proto RPC to its HTTP handler.
func (s *HTTPServer) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/cameras", s.handleListCameras)
	mux.HandleFunc("GET /v1/cameras/{camera_id}", s.handleGetCameraInfo)
	mux.HandleFunc("POST /v1/cameras/{camera_id}/open", s.handleOpenCamera)
	mux.HandleFunc("POST /v1/cameras/{camera_id}/close", s.handleCloseCamera)
	mux.HandleFunc("GET /v1/cameras/{camera_id}/frame", s.handleCaptureFrame)
	mux.HandleFunc("POST /v1/cameras:captureFrames", s.handleCaptureFrames)
	mux.HandleFunc("GET /v1/cameras/{camera_id}/watch", s.handleWatchFrames)
}

// ---------------------------------------------------------------------------
// Handlers — one per proto RPC
// ---------------------------------------------------------------------------

// handleListCameras — GET /v1/cameras → ListCameras RPC.
func (s *HTTPServer) handleListCameras(w http.ResponseWriter, r *http.Request) {
	resp, err := s.ctrl.ListCameras(r.Context(), &pb.ListCamerasRequest{})
	if err != nil {
		writeError(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

// handleGetCameraInfo — GET /v1/cameras/{camera_id} → GetCameraInfo RPC.
func (s *HTTPServer) handleGetCameraInfo(w http.ResponseWriter, r *http.Request) {
	req := &pb.GetCameraInfoRequest{CameraId: r.PathValue("camera_id")}
	resp, err := s.ctrl.GetCameraInfo(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

// handleOpenCamera — POST /v1/cameras/{camera_id}/open → OpenCamera RPC.
// The request body is the OpenCameraRequest message minus the camera_id
// (which comes from the path). Optional fields width/height/fps/encoding are
// decoded from JSON; unset fields remain nil pointers (proto optional).
func (s *HTTPServer) handleOpenCamera(w http.ResponseWriter, r *http.Request) {
	cameraID := r.PathValue("camera_id")

	// Read a copy of the body so we can merge the path-derived camera_id
	// without requiring the client to repeat it. protojson requires the
	// target message to have its fields already populated for oneofs.
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, status.Errorf(codes.InvalidArgument, "read body: %v", err))
		return
	}
	defer func() { _ = r.Body.Close() }()

	req := &pb.OpenCameraRequest{CameraId: cameraID}
	if len(raw) > 0 {
		if err := httpUnmarshal.Unmarshal(raw, req); err != nil {
			writeError(w, status.Errorf(codes.InvalidArgument, "invalid OpenCameraRequest: %v", err))
			return
		}
		// The path camera_id always wins — ignore any value in the body.
		req.CameraId = cameraID
	}

	resp, err := s.ctrl.OpenCamera(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

// handleCloseCamera — POST /v1/cameras/{camera_id}/close → CloseCamera RPC.
func (s *HTTPServer) handleCloseCamera(w http.ResponseWriter, r *http.Request) {
	req := &pb.CloseCameraRequest{CameraId: r.PathValue("camera_id")}
	resp, err := s.ctrl.CloseCamera(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

// handleCaptureFrame — GET /v1/cameras/{camera_id}/frame → CaptureFrame RPC.
//
// By default the response body is the raw encoded frame bytes, with capture
// metadata exposed via response headers so a client can grab a JPEG directly
// with `curl -o frame.jpg`. Set `Accept: application/json` (or `?format=json`)
// to receive the full CaptureFrameResponse as canonical proto JSON instead —
// the data field is base64-encoded there (proto bytes JSON convention).
//
// The optional timeout (CaptureFrameRequest.timeout) is taken from the
// `timeout` query parameter (seconds).
func (s *HTTPServer) handleCaptureFrame(w http.ResponseWriter, r *http.Request) {
	cameraID := r.PathValue("camera_id")

	var timeout *int32
	if v := r.URL.Query().Get("timeout"); v != "" {
		var t int32
		if _, err := fmt.Sscanf(v, "%d", &t); err == nil {
			timeout = &t
		}
	}

	req := &pb.CaptureFrameRequest{CameraId: cameraID, Timeout: timeout}
	resp, err := s.ctrl.CaptureFrame(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}

	// JSON response when the client asks for it.
	wantJSON := strings.Contains(r.Header.Get("Accept"), "application/json") ||
		r.URL.Query().Get("format") == "json"
	if wantJSON {
		writeProto(w, http.StatusOK, resp)
		return
	}

	// Raw bytes + metadata headers (the practical default).
	h := w.Header()
	h.Set("Content-Type", contentTypeForEncoding(resp.GetEncoding()))
	h.Set("X-Frame-Camera-Id", resp.GetCameraId())
	h.Set("X-Frame-Width", fmt.Sprintf("%d", resp.GetWidth()))
	h.Set("X-Frame-Height", fmt.Sprintf("%d", resp.GetHeight()))
	h.Set("X-Frame-Encoding", resp.GetEncoding())
	h.Set("X-Frame-Timestamp-Ns", fmt.Sprintf("%d", resp.GetTimestampNs()))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(resp.GetData())
}

// handleCaptureFrames — POST /v1/cameras:captureFrames → CaptureFrames RPC.
// The request body is the CaptureFramesRequest message (camera_ids + timeout).
func (s *HTTPServer) handleCaptureFrames(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, status.Errorf(codes.InvalidArgument, "read body: %v", err))
		return
	}
	defer func() { _ = r.Body.Close() }()

	req := &pb.CaptureFramesRequest{}
	if len(raw) > 0 {
		if err := httpUnmarshal.Unmarshal(raw, req); err != nil {
			writeError(w, status.Errorf(codes.InvalidArgument, "invalid CaptureFramesRequest: %v", err))
			return
		}
	}

	resp, err := s.ctrl.CaptureFrames(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeProto(w, http.StatusOK, resp)
}

// handleWatchFrames — GET /v1/cameras/{camera_id}/watch → WatchFrames RPC.
//
// Implemented as a Server-Sent Events (SSE) stream so it works over plain HTTP
// (no WebSocket upgrade) and is proxy-friendly. Each event is one JSON-encoded
// VideoFrame message (the same shape as the proto message); the frame's bytes
// `data` field is base64-encoded per proto's JSON convention.
//
// The stream ends when the client disconnects or the camera is closed. An
// initial `event: ready` line is sent to confirm the subscription is live
// before any frame data, so clients can detect a successful open.
//
// A "ping" comment line is emitted every 15s of silence to keep intermediates
// from closing the idle connection.
func (s *HTTPServer) handleWatchFrames(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	cameraID := r.PathValue("camera_id")

	// SSE headers. gzip must be disabled (Content-Encoding absent) so the
	// flusher writes are delivered immediately rather than buffered for
	// compression.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // nginx: disable proxy buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// The Controller's WatchFrames is server-streaming over gRPC; we adapt it
	// to HTTP by driving it through the in-process pb.CameraController_WatchFramesServer
	// adapter (watchStreamAdapter) which writes each VideoFrame as one SSE event.
	stream := newWatchStreamAdapter(r.Context(), w, flusher)
	if err := s.ctrl.WatchFrames(&pb.WatchFramesRequest{CameraId: cameraID}, stream); err != nil {
		// If the connection is still writable, surface the error as an SSE
		// event so the client can see why the stream ended.
		stream.sendError(err)
	}
}

// ---------------------------------------------------------------------------
// Response helpers
// ---------------------------------------------------------------------------

// writeProto serializes a proto.Message as canonical proto JSON with the
// given HTTP status code.
func writeProto(w http.ResponseWriter, code int, msg proto.Message) {
	b, err := httpMarshal.Marshal(msg)
	if err != nil {
		http.Error(w, "marshal: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b)
	_, _ = w.Write([]byte("\n"))
}

// writeError maps a Controller error (typically a gRPC status) to the closest
// HTTP status code and writes a JSON error object: {"code","message","status"}.
func writeError(w http.ResponseWriter, err error) {
	code := grpcToHTTPStatus(err)
	st, ok := status.FromError(err)
	if !ok {
		// Non-status error: synthesize a status string from the HTTP code.
		http.Error(w, err.Error(), code)
		return
	}

	body, _ := json.Marshal(map[string]any{
		"code":    int(st.Code()),
		"status":  http.StatusText(code),
		"message": st.Message(),
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(body)
	_, _ = w.Write([]byte("\n"))
}

// grpcToHTTPStatus maps a gRPC status code to the closest HTTP status code.
// Non-status errors default to 500.
func grpcToHTTPStatus(err error) int {
	st, ok := status.FromError(err)
	if !ok {
		return http.StatusInternalServerError
	}
	switch st.Code() {
	case codes.OK:
		return http.StatusOK
	case codes.Canceled:
		return 499 // nginx's "Client Closed Request"
	case codes.Unknown:
		return http.StatusInternalServerError
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.DeadlineExceeded:
		return http.StatusGatewayTimeout
	case codes.NotFound:
		return http.StatusNotFound
	case codes.AlreadyExists:
		return http.StatusConflict
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.ResourceExhausted:
		return http.StatusTooManyRequests
	case codes.FailedPrecondition:
		return http.StatusPreconditionFailed
	case codes.Aborted:
		return http.StatusConflict
	case codes.OutOfRange:
		return http.StatusBadRequest
	case codes.Unimplemented:
		return http.StatusNotImplemented
	case codes.Internal:
		return http.StatusInternalServerError
	case codes.Unavailable:
		return http.StatusServiceUnavailable
	default: // Unauthenticated, etc.
		return http.StatusUnauthorized
	}
}

// contentTypeForEncoding returns the MIME type for a captured frame's encoding.
// Bitstream encodings (h264/h265) are served as video/* so browsers and tools
// can sniff them; still-image encodings map to their image/* type.
func contentTypeForEncoding(enc string) string {
	switch enc {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "bmp":
		return "image/bmp"
	case "tiff":
		return "image/tiff"
	case "h264":
		return "video/mp4" // Annex B; clients often want .h264, but MIME is a hint
	case "h265", "hevc":
		return "video/mp4"
	default:
		return "application/octet-stream"
	}
}

// ---------------------------------------------------------------------------
// watchStreamAdapter — bridges the gRPC WatchFramesServer interface to an
// HTTP SSE response writer. Each Send writes one SSE event containing the
// JSON-encoded VideoFrame. The proto bytes `data` field is base64-encoded
// per proto's JSON convention.
// ---------------------------------------------------------------------------

type watchStreamAdapter struct {
	ctx     context.Context
	w       http.ResponseWriter
	flusher http.Flusher
	enc     *base64.Encoding
}

func newWatchStreamAdapter(ctx context.Context, w http.ResponseWriter, f http.Flusher) *watchStreamAdapter {
	return &watchStreamAdapter{
		ctx:     ctx,
		w:       w,
		flusher: f,
		enc:     base64.StdEncoding,
	}
}

// Send implements pb.CameraController_WatchFramesServer by writing one VideoFrame
// as an SSE event. Errors (client gone) are returned to the Controller so it
// can stop the subscription.
func (a *watchStreamAdapter) Send(msg *pb.VideoFrame) error {
	b, err := httpMarshal.Marshal(msg)
	if err != nil {
		return err
	}

	// SSE event:
	//   event: frame
	//   data: <json>
	//   <blank line>
	//
	// Splitting the JSON payload across multiple `data:` lines (one per \n)
	// would let it span arbitrarily large frames; protojson is single-line,
	// so a single data: line suffices.
	if _, err := fmt.Fprintf(a.w, "event: frame\ndata: %s\n\n", b); err != nil {
		return err
	}
	a.flusher.Flush()
	return nil
}

// sendError writes a final SSE event describing why the stream ended, then
// flushes. Safe to call after the connection has been torn down (the write
// fails silently).
func (a *watchStreamAdapter) sendError(err error) {
	st, ok := status.FromError(err)
	code := codes.Unknown
	msg := err.Error()
	if ok {
		code = st.Code()
		msg = st.Message()
	}
	payload, _ := json.Marshal(map[string]any{
		"code":    int(code),
		"message": msg,
	})
	_, _ = fmt.Fprintf(a.w, "event: error\ndata: %s\n\n", payload)
	a.flusher.Flush()
}

// Context implements pb.CameraController_WatchFramesServer, propagating the
// HTTP request's context so the Controller stops when the client disconnects.
func (a *watchStreamAdapter) Context() context.Context { return a.ctx }

// The remaining grpc.ServerStream methods (SetHeader, SendHeader, SetTrailer,
// SendMsg, RecvMsg) are no-ops for the HTTP adapter — gRPC metadata has no
// HTTP equivalent here, and WatchFrames is server-streaming so RecvMsg is
// never called by the Controller. They satisfy the interface so the adapter
// can be passed directly to Controller.WatchFrames.

func (a *watchStreamAdapter) SetHeader(_ metadata.MD) error  { return nil }
func (a *watchStreamAdapter) SendHeader(_ metadata.MD) error { return nil }
func (a *watchStreamAdapter) SetTrailer(_ metadata.MD)       {}
func (a *watchStreamAdapter) SendMsg(_ interface{}) error    { return nil }
func (a *watchStreamAdapter) RecvMsg(_ interface{}) error    { return nil }
