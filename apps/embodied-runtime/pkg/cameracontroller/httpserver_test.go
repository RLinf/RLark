package cameracontroller

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// newTestHTTPServer wires a fresh Controller-backed HTTPServer to an
// httptest.Server. It returns the server's base URL and the underlying
// Controller so tests can poke camera state directly. The httptest.Server
// is closed automatically via t.Cleanup, and all cameras are closed via
// CloseAll so no synthetic-reader / capture-loop goroutines leak across
// tests.
func newTestHTTPServer(t *testing.T) (base string, ctrl *Controller) {
	t.Helper()
	ctrl = New()
	hs := NewHTTPServer(ctrl, "")
	srv := httptest.NewServer(hs.Handler())
	t.Cleanup(srv.Close)
	t.Cleanup(ctrl.CloseAll)
	return srv.URL, ctrl
}

// httpJSON issues an HTTP request and decodes the JSON body into out.
// For non-2xx responses it returns the raw error body for inspection.
func httpJSON(t *testing.T, method, url string, body any, out any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })

	if out != nil && resp.StatusCode < 300 {
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if err := protojson.Unmarshal(raw, out.(proto.Message)); err != nil {
			t.Fatalf("unmarshal body: %v (body=%s)", err, raw)
		}
	}
	return resp
}

// ---------------------------------------------------------------------------
// ListCameras / GetCameraInfo
// ---------------------------------------------------------------------------

// TestHTTP_ListCameras verifies that GET /v1/cameras returns every registered
// camera in a stable order, mirroring the gRPC ListCameras RPC.
func TestHTTP_ListCameras(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	registerTestCamera(t, ctrl, "cam-1")
	registerTestCamera(t, ctrl, "cam-0")

	var resp pb.ListCamerasResponse
	httpJSON(t, http.MethodGet, base+"/v1/cameras", nil, &resp)

	if len(resp.Cameras) != 2 {
		t.Fatalf("got %d cameras, want 2", len(resp.Cameras))
	}
	// ListCameras sorts by camera_id — verify stable ordering.
	if resp.Cameras[0].GetCameraId() != "cam-0" {
		t.Errorf("cameras[0] = %q, want cam-0", resp.Cameras[0].GetCameraId())
	}
	if resp.Cameras[1].GetCameraId() != "cam-1" {
		t.Errorf("cameras[1] = %q, want cam-1", resp.Cameras[1].GetCameraId())
	}
	// State must come back as CLOSED for a freshly-registered camera.
	if resp.Cameras[0].GetState() != pb.CameraState_CAMERA_STATE_CLOSED {
		t.Errorf("cameras[0].state = %v, want CLOSED", resp.Cameras[0].GetState())
	}
}

// TestHTTP_GetCameraInfo verifies GET /v1/cameras/{id} returns that camera.
func TestHTTP_GetCameraInfo(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	registerTestCamera(t, ctrl, "cam-0")

	var resp pb.GetCameraInfoResponse
	httpJSON(t, http.MethodGet, base+"/v1/cameras/cam-0", nil, &resp)

	if resp.GetCamera().GetCameraId() != "cam-0" {
		t.Errorf("camera_id = %q, want cam-0", resp.GetCamera().GetCameraId())
	}
}

// TestHTTP_GetCameraInfo_NotFound verifies that an unknown camera_id yields
// a 404 with a JSON error body (gRPC NotFound → HTTP 404).
func TestHTTP_GetCameraInfo_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	resp := httpJSON(t, http.MethodGet, base+"/v1/cameras/ghost", nil, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	var errBody map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&errBody)
	if errBody["message"] == nil || errBody["message"] == "" {
		t.Errorf("error body missing message: %v", errBody)
	}
}

// ---------------------------------------------------------------------------
// OpenCamera / CloseCamera
// ---------------------------------------------------------------------------

// TestHTTP_OpenCamera verifies POST /v1/cameras/{id}/open decodes optional
// width/height/fps/encoding overrides and surfaces the resulting state.
// The ros_topic stub driver always returns "jpeg" (it ignores the encoding
// hint), so we assert the echoed encoding is the stub's actual output rather
// than the requested hint.
func TestHTTP_OpenCamera(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	registerTestCamera(t, ctrl, "cam-0")

	body := map[string]any{
		"width":    4,
		"height":   4,
		"fps":      1,
		"encoding": "png",
	}
	var resp pb.OpenCameraResponse
	httpJSON(t, http.MethodPost, base+"/v1/cameras/cam-0/open", body, &resp)

	if resp.GetCameraId() != "cam-0" {
		t.Errorf("camera_id = %q, want cam-0", resp.GetCameraId())
	}
	if resp.GetState() != pb.CameraState_CAMERA_STATE_OPEN {
		t.Errorf("state = %v, want OPEN", resp.GetState())
	}
	// Stub driver returns "jpeg" regardless of the hint.
	if resp.GetEncoding() != "jpeg" {
		t.Errorf("encoding = %q, want jpeg (stub ignores hint)", resp.GetEncoding())
	}
}

// TestHTTP_OpenCamera_PathIdWins verifies that the camera_id in the URL path
// always overrides any camera_id present in the JSON body.
func TestHTTP_OpenCamera_PathIdWins(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	registerTestCamera(t, ctrl, "cam-0")

	// Body deliberately says cam-99 — path says cam-0. Path must win.
	body := map[string]any{"cameraId": "cam-99"}
	var resp pb.OpenCameraResponse
	httpJSON(t, http.MethodPost, base+"/v1/cameras/cam-0/open", body, &resp)
	if resp.GetCameraId() != "cam-0" {
		t.Errorf("camera_id = %q, want cam-0 (path must win)", resp.GetCameraId())
	}
}

// TestHTTP_OpenCamera_BadBody verifies a malformed JSON body yields 400.
func TestHTTP_OpenCamera_BadBody(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	registerTestCamera(t, ctrl, "cam-0")

	req, _ := http.NewRequest(http.MethodPost, base+"/v1/cameras/cam-0/open",
		strings.NewReader("{not json"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// TestHTTP_CloseCamera verifies POST /v1/cameras/{id}/close closes an open
// camera and reports the CLOSED state.
func TestHTTP_CloseCamera(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	cs := registerTestCamera(t, ctrl, "cam-0")
	setOpen(cs)

	var resp pb.CloseCameraResponse
	httpJSON(t, http.MethodPost, base+"/v1/cameras/cam-0/close", nil, &resp)

	if resp.GetCameraId() != "cam-0" {
		t.Errorf("camera_id = %q, want cam-0", resp.GetCameraId())
	}
	if resp.GetState() != pb.CameraState_CAMERA_STATE_CLOSED {
		t.Errorf("state = %v, want CLOSED", resp.GetState())
	}
}

// TestHTTP_CloseCamera_NotOpen verifies that closing a CLOSED camera is
// still a successful idempotent call (mirrors gRPC CloseCamera).
func TestHTTP_CloseCamera_NotOpen(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	registerTestCamera(t, ctrl, "cam-0") // CLOSED

	var resp pb.CloseCameraResponse
	httpJSON(t, http.MethodPost, base+"/v1/cameras/cam-0/close", nil, &resp)
	if resp.GetState() != pb.CameraState_CAMERA_STATE_CLOSED {
		t.Errorf("state = %v, want CLOSED", resp.GetState())
	}
}

// ---------------------------------------------------------------------------
// CaptureFrame
// ---------------------------------------------------------------------------

// TestHTTP_CaptureFrame_Raw verifies that by default CaptureFrame returns the
// raw frame bytes with metadata in response headers, so `curl -o frame.jpg`
// works. Mirrors CaptureFrameResponse.data + width/height/encoding.
func TestHTTP_CaptureFrame_Raw(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	cs := registerTestCamera(t, ctrl, "cam-0")
	setOpen(cs)
	storeFrame(cs, "jpeg", []byte("jpegbytes"), time.Unix(1_700_000_000, 0))

	resp, err := http.Get(base + "/v1/cameras/cam-0/frame")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", ct)
	}
	if got := resp.Header.Get("X-Frame-Encoding"); got != "jpeg" {
		t.Errorf("X-Frame-Encoding = %q, want jpeg", got)
	}
	if got := resp.Header.Get("X-Frame-Width"); got != "4" {
		t.Errorf("X-Frame-Width = %q, want 4", got)
	}
	if got := resp.Header.Get("X-Frame-Height"); got != "4" {
		t.Errorf("X-Frame-Height = %q, want 4", got)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "jpegbytes" {
		t.Errorf("body = %q, want jpegbytes", body)
	}
}

// TestHTTP_CaptureFrame_JSON verifies that requesting JSON output returns the
// full CaptureFrameResponse with base64-encoded data (proto JSON convention).
func TestHTTP_CaptureFrame_JSON(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	cs := registerTestCamera(t, ctrl, "cam-0")
	setOpen(cs)
	storeFrame(cs, "jpeg", []byte("jpegbytes"), time.Unix(1_700_000_000, 0))

	req, _ := http.NewRequest(http.MethodGet, base+"/v1/cameras/cam-0/frame", nil)
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	raw, _ := io.ReadAll(resp.Body)
	var got pb.CaptureFrameResponse
	if err := protojson.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if string(got.GetData()) != "jpegbytes" {
		t.Errorf("data = %q, want jpegbytes", got.GetData())
	}
	if got.GetEncoding() != "jpeg" {
		t.Errorf("encoding = %q, want jpeg", got.GetEncoding())
	}
}

// TestHTTP_CaptureFrame_NotOpen verifies that capturing from a CLOSED camera
// yields 412 Precondition Failed (gRPC FailedPrecondition).
func TestHTTP_CaptureFrame_NotOpen(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	registerTestCamera(t, ctrl, "cam-0") // CLOSED

	resp, err := http.Get(base + "/v1/cameras/cam-0/frame")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412", resp.StatusCode)
	}
}

// TestHTTP_CaptureFrame_Bitstream verifies that a bitstream-encoding camera
// (h264) is rejected with 501 Not Implemented, matching the gRPC behavior
// (a single NAL unit is not decodable on its own).
func TestHTTP_CaptureFrame_Bitstream(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	cs := registerTestCamera(t, ctrl, "cam-0")
	setOpen(cs)
	storeFrame(cs, "h264", []byte("nalu"), time.Now())

	resp, err := http.Get(base + "/v1/cameras/cam-0/frame")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// CaptureFrames
// ---------------------------------------------------------------------------

// TestHTTP_CaptureFrames verifies POST /v1/cameras:captureFrames returns the
// latest frame per requested camera, with partial failures reported per
// entry rather than failing the whole request.
func TestHTTP_CaptureFrames(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	cs0 := registerTestCamera(t, ctrl, "cam-0")
	registerTestCamera(t, ctrl, "cam-1") // left CLOSED → partial failure
	setOpen(cs0)
	storeFrame(cs0, "jpeg", []byte("ok"), time.Now())

	body := map[string]any{"cameraIds": []string{"cam-0", "cam-1"}}
	var resp pb.CaptureFramesResponse
	httpJSON(t, http.MethodPost, base+"/v1/cameras:captureFrames", body, &resp)

	if len(resp.Frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(resp.Frames))
	}
	if string(resp.Frames[0].GetData()) != "ok" {
		t.Errorf("cam-0 data = %q, want ok", resp.Frames[0].GetData())
	}
	if resp.Frames[0].GetErrorCode() != int32(codes.OK) {
		t.Errorf("cam-0 error_code = %d, want OK", resp.Frames[0].GetErrorCode())
	}
	if resp.Frames[1].GetErrorCode() != int32(codes.FailedPrecondition) {
		t.Errorf("cam-1 error_code = %d, want FailedPrecondition",
			resp.Frames[1].GetErrorCode())
	}
}

// TestHTTP_CaptureFrames_NotFound verifies that an unregistered camera id is
// reported with NotFound per entry (HTTP 200 — the RPC itself succeeds; only
// the per-camera entry carries the error).
func TestHTTP_CaptureFrames_NotFound(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	body := map[string]any{"cameraIds": []string{"ghost"}}
	var resp pb.CaptureFramesResponse
	httpJSON(t, http.MethodPost, base+"/v1/cameras:captureFrames", body, &resp)

	if len(resp.Frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(resp.Frames))
	}
	if resp.Frames[0].GetErrorCode() != int32(codes.NotFound) {
		t.Errorf("error_code = %d, want NotFound", resp.Frames[0].GetErrorCode())
	}
}

// TestHTTP_CaptureFrames_Empty verifies that an empty request is rejected at
// the HTTP layer with 400 (gRPC InvalidArgument).
func TestHTTP_CaptureFrames_Empty(t *testing.T) {
	base, _ := newTestHTTPServer(t)

	body := map[string]any{"cameraIds": []string{}}
	resp := httpJSON(t, http.MethodPost, base+"/v1/cameras:captureFrames", body, nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

// ---------------------------------------------------------------------------
// WatchFrames (SSE)
// ---------------------------------------------------------------------------

// TestHTTP_WatchFrames_SSE verifies that GET /v1/cameras/{id}/watch streams
// frames as Server-Sent Events: one "event: frame" line per VideoFrame, with
// a JSON-serialized payload whose `data` field is base64-encoded.
//
// The test opens the camera via the real Controller.OpenCamera (which sets up
// captureCtx and starts the background capture loop) so the broadcast path is
// exercised end-to-end — the ros_topic stub driver's synthetic reader pushes
// real JPEG frames into the subscriber fan-out, and the SSE handler forwards
// each one as an event.
func TestHTTP_WatchFrames_SSE(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	registerTestCamera(t, ctrl, "cam-0")

	// Open via the real RPC so captureCtx + captureLoop are wired up
	// (setOpen alone leaves captureCtx nil and panics in WatchFrames).
	if _, err := ctrl.OpenCamera(context.Background(), &pb.OpenCameraRequest{
		CameraId: "cam-0",
	}); err != nil {
		t.Fatalf("open: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/v1/cameras/cam-0/watch", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read SSE events until we get a `data:` line carrying a frame payload.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var dataLine string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if dataLine == "" {
		t.Fatal("no SSE data line received before timeout")
	}

	var frame pb.VideoFrame
	if err := protojson.Unmarshal([]byte(dataLine), &frame); err != nil {
		t.Fatalf("unmarshal frame: %v (line=%s)", err, dataLine)
	}
	// Stub reader produces JPEG frames at the synthetic FPS.
	if frame.GetEncoding() != "jpeg" {
		t.Errorf("frame encoding = %q, want jpeg", frame.GetEncoding())
	}
	if len(frame.GetData()) == 0 {
		t.Errorf("frame data is empty")
	}
	if frame.GetKeyframe() != true {
		t.Errorf("frame keyframe = %v, want true (frame-mode)", frame.GetKeyframe())
	}
}

// TestHTTP_WatchFrames_NotOpen verifies that watching a CLOSED camera surfaces
// the gRPC FailedPrecondition as an SSE "error" event (the connection still
// opens — 200 OK with text/event-stream — and then ends with the error).
func TestHTTP_WatchFrames_NotOpen(t *testing.T) {
	base, ctrl := newTestHTTPServer(t)
	registerTestCamera(t, ctrl, "cam-0") // CLOSED

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet,
		base+"/v1/cameras/cam-0/watch", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	var dataLine string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataLine = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if dataLine == "" {
		t.Fatal("no SSE data line received (expected an error event)")
	}
	var errBody map[string]any
	_ = json.Unmarshal([]byte(dataLine), &errBody)
	if errBody["message"] == nil || errBody["message"] == "" {
		t.Errorf("error event missing message: %v", errBody)
	}
}

// storeAndBroadcast was removed: SSE tests now open the camera via the real
// OpenCamera RPC, which wires up captureCtx and the synthetic reader's
// broadcast path end-to-end (no manual fan-out needed).
