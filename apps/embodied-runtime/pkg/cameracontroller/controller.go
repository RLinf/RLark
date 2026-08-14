package cameracontroller

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ---------------------------------------------------------------------------
// Camera configuration
// ---------------------------------------------------------------------------

// CameraConfig defines the static configuration for a camera device.
type CameraConfig struct {
	ID           string            `yaml:"id"`
	Name         string            `yaml:"name"`
	CameraType   string            `yaml:"camera_type"`
	SerialNumber string            `yaml:"serial_number"`
	Width        int               `yaml:"width"`
	Height       int               `yaml:"height"`
	FPS          int               `yaml:"fps"`
	EnableDepth  bool              `yaml:"enable_depth"`
	Params       map[string]string `yaml:"params,omitempty"`
}

// Param returns the value of a type-specific parameter, falling back to
// the given default if the key is not set.
func (c CameraConfig) Param(key, defaultVal string) string {
	if v, ok := c.Params[key]; ok && v != "" {
		return v
	}
	return defaultVal
}

// SupportedResolutions returns the device's supported capture resolutions
// as "WxH" strings, parsed from params["supported_resolutions"]
// (a comma-separated list, e.g. "640x480,1280x720"). Returns nil when the
// parameter is unset, i.e. the device did not enumerate its capabilities
// (in which case validation is skipped).
func (c CameraConfig) SupportedResolutions() []string {
	v := c.Param("supported_resolutions", "")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SupportedFPS returns the device's supported frame rates, parsed from
// params["supported_fps"] (a comma-separated list, e.g. "15,30,60").
// Returns nil when the parameter is unset.
func (c CameraConfig) SupportedFPS() []int {
	v := c.Param("supported_fps", "")
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		f, err := strconv.Atoi(p)
		if err != nil || f <= 0 {
			continue
		}
		out = append(out, f)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// SupportsResolution reports whether widthxheight is among the device's
// supported resolutions. Returns true when the device did not enumerate
// its capabilities (no constraint to check against).
func (c CameraConfig) SupportsResolution(width, height int) bool {
	list := c.SupportedResolutions()
	if len(list) == 0 {
		return true
	}
	want := fmt.Sprintf("%dx%d", width, height)
	for _, r := range list {
		if r == want {
			return true
		}
	}
	return false
}

// SupportsFPS reports whether fps is among the device's supported frame
// rates. Returns true when the device did not enumerate its capabilities.
func (c CameraConfig) SupportsFPS(fps int) bool {
	list := c.SupportedFPS()
	if len(list) == 0 {
		return true
	}
	for _, f := range list {
		if f == fps {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Internal camera state
// ---------------------------------------------------------------------------

// cameraState holds the runtime state for a single camera device.
//
// Lifecycle:
//
//	RegisterCamera → CLOSED
//	OpenCamera     → OPEN (starts background capture loop)
//	CloseCamera    → CLOSED (stops capture loop, closes all subscribers)
//
// While OPEN, a background goroutine reads from the driver's FrameReader,
// stores the latest frame in lastFrame, and broadcasts to all subscribers.
// CaptureFrame reads lastFrame directly (no blocking).
// WatchFrames subscribes to the broadcast channel.
type cameraState struct {
	cfg    CameraConfig
	state  pb.CameraState
	driver CameraDriver
	reader FrameReader
	mu     sync.Mutex

	// Background capture — always running while state == OPEN.
	captureCtx    context.Context
	captureCancel context.CancelFunc
	captureWG     sync.WaitGroup
	lastFrame     atomicFrame // stores *Frame (always a copy, nil when CLOSED)
	actualEnc     string      // actual encoding from driver, set once at Open

	// paramSets caches the latest VPS/SPS/PPS NAL units for bitstream
	// sources (h264/h265). Written by the capture loop (which sees the
	// stream from OpenCamera onward) and read by WatchFrames to prime
	// late-joining subscribers that missed the camera's initial SPS/PPS.
	paramSets atomic.Pointer[paramSetsBlob]

	// Watch subscribers — fan-out broadcast.
	// The capture loop pushes a copy of each frame to every subscriber channel.
	// Slow subscribers are dropped (select with default).
	subscribers map[uint64]chan *Frame
	subMu       sync.Mutex
	subCounter  uint64
}

// atomicFrame provides safe concurrent access to the latest captured frame.
type atomicFrame struct {
	v atomic.Pointer[Frame]
}

func (a *atomicFrame) Load() *Frame {
	return a.v.Load()
}

func (a *atomicFrame) Store(f *Frame) {
	a.v.Store(f)
}

// Controller implements the gRPC CameraControllerServer.
type Controller struct {
	pb.UnimplementedCameraControllerServer

	mu      sync.RWMutex
	cameras map[string]*cameraState
}

// New creates an empty camera controller.
func New() *Controller {
	return &Controller{
		cameras: make(map[string]*cameraState),
	}
}

// RegisterCamera adds a camera device from static configuration.
// Must be called before starting the gRPC server.
func (cc *Controller) RegisterCamera(cfg CameraConfig) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	if cfg.ID == "" {
		return fmt.Errorf("camera config: ID is required")
	}
	if _, exists := cc.cameras[cfg.ID]; exists {
		return fmt.Errorf("camera %q already registered", cfg.ID)
	}

	driver, err := NewDriver(cfg)
	if err != nil {
		return fmt.Errorf("create driver for camera %q: %w", cfg.ID, err)
	}

	cc.cameras[cfg.ID] = &cameraState{
		cfg:         cfg,
		state:       pb.CameraState_CAMERA_STATE_CLOSED,
		driver:      driver,
		subscribers: make(map[uint64]chan *Frame),
	}
	log.Printf("[camera-controller] registered camera: %s (type=%s, driver=%T)",
		cfg.ID, cfg.CameraType, driver)
	return nil
}

// --------------------------------------------------------------------------
// gRPC handlers
// --------------------------------------------------------------------------

// ListCameras returns all managed cameras and their current state.
func (cc *Controller) ListCameras(
	ctx context.Context, req *pb.ListCamerasRequest,
) (*pb.ListCamerasResponse, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	cameras := make([]*pb.CameraDescriptor, 0, len(cc.cameras))
	for _, cs := range cc.cameras {
		cameras = append(cameras, cameraDescriptor(cs))
	}

	// Map iteration order is non-deterministic; sort by camera ID so
	// ListCameras returns a stable, reproducible ordering.
	sort.Slice(cameras, func(i, j int) bool {
		return cameras[i].GetCameraId() < cameras[j].GetCameraId()
	})

	return &pb.ListCamerasResponse{Cameras: cameras}, nil
}

// GetCameraInfo returns detailed information about a specific camera.
func (cc *Controller) GetCameraInfo(
	ctx context.Context, req *pb.GetCameraInfoRequest,
) (*pb.GetCameraInfoResponse, error) {
	cc.mu.RLock()
	defer cc.mu.RUnlock()

	cs, err := cc.lookup(req.CameraId)
	if err != nil {
		return nil, err
	}

	return &pb.GetCameraInfoResponse{Camera: cameraDescriptor(cs)}, nil
}

// OpenCamera starts frame capture on the specified camera.
// Once opened, the camera continuously captures frames in the background.
// CaptureFrame returns the latest frame instantly (no blocking I/O).
// WatchFrames subscribes to a broadcast of all captured frames.
func (cc *Controller) OpenCamera(
	ctx context.Context, req *pb.OpenCameraRequest,
) (*pb.OpenCameraResponse, error) {
	cc.mu.Lock()

	cs, err := cc.lookup(req.CameraId)
	cc.mu.Unlock()

	if err != nil {
		return nil, err
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.state == pb.CameraState_CAMERA_STATE_OPEN {
		return &pb.OpenCameraResponse{
			CameraId: req.CameraId,
			State:    cs.state,
			Encoding: cs.actualEnc,
		}, nil
	}

	// Only a CLOSED camera can be (re)opened. An ERROR camera still holds
	// a reader/subprocess that must be cleaned up via CloseCamera first;
	// reopening it directly would leak that reader (and its ffmpeg/zombie).
	if cs.state != pb.CameraState_CAMERA_STATE_CLOSED {
		return nil, status.Errorf(codes.FailedPrecondition,
			"camera %q is in state %v; close it before reopening",
			req.CameraId, cs.state)
	}

	// Validate explicitly-requested resolution/fps against the device's
	// enumerated capabilities. Only dimensions the caller set are checked;
	// unset dimensions fall back to the config default and are trusted
	// (autodetect already picked a valid default). Validation is skipped
	// entirely when the device did not report its supported modes.
	reqW := int(req.GetWidth())
	reqH := int(req.GetHeight())
	reqF := int(req.GetFps())
	if reqW > 0 || reqH > 0 {
		w, h, _ := resolveResolution(cs.cfg, reqW, reqH, 0)
		if !cs.cfg.SupportsResolution(w, h) {
			return nil, status.Errorf(codes.InvalidArgument,
				"camera %q: resolution %dx%d not supported (supported: %s)",
				req.CameraId, w, h, strings.Join(cs.cfg.SupportedResolutions(), ", "))
		}
	}
	if reqF > 0 && !cs.cfg.SupportsFPS(reqF) {
		return nil, status.Errorf(codes.InvalidArgument,
			"camera %q: fps %d not supported (supported: %v)",
			req.CameraId, reqF, cs.cfg.SupportedFPS())
	}

	// Open the driver — returns a FrameReader plus the encoding it actually
	// produces (the driver may fall back if the requested encoding is
	// unsupported by the hardware).
	encHint := req.GetEncoding()
	if encHint == "" {
		encHint = "jpeg"
	}

	// Create the capture context BEFORE opening the driver, so the driver
	// is tied to the capture lifecycle (not the gRPC client context).
	// This prevents the driver from being killed when the client disconnects.
	cs.captureCtx, cs.captureCancel = context.WithCancel(context.Background())

	reader, actualEnc, err := cs.driver.Open(cs.captureCtx, cs.cfg,
		int(req.GetWidth()),
		int(req.GetHeight()),
		int(req.GetFps()),
		encHint,
	)
	if err != nil {
		cs.captureCancel()
		cs.state = pb.CameraState_CAMERA_STATE_ERROR
		log.Printf("[camera-controller] OpenCamera %q failed: %v", req.CameraId, err)
		return nil, fmt.Errorf("open camera %q: %w", req.CameraId, err)
	}

	cs.reader = reader
	cs.actualEnc = actualEnc
	cs.state = pb.CameraState_CAMERA_STATE_OPEN

	// Seed cached parameter sets from config (params.sps/pps hex) so
	// devices that never emit SPS in-band are still decodable. In-band
	// updates merge on top of the seed during capture.
	if isBitstream(actualEnc) {
		seedParamSets(cs, cs.cfg)
	}

	// Start background capture loop.
	cs.captureWG.Add(1)
	go cs.captureLoop(cs.captureCtx, reader)

	log.Printf("[camera-controller] OpenCamera: %s (driver=%T, encoding=%s)", req.CameraId, cs.driver, actualEnc)

	return &pb.OpenCameraResponse{
		CameraId: req.CameraId,
		State:    cs.state,
		Encoding: actualEnc,
	}, nil
}

// CloseCamera stops frame capture on the specified camera.
// Stops the background capture goroutine, closes all subscriber channels,
// closes the driver, and resets state to CLOSED.
func (cc *Controller) CloseCamera(
	ctx context.Context, req *pb.CloseCameraRequest,
) (*pb.CloseCameraResponse, error) {
	cc.mu.Lock()

	cs, err := cc.lookup(req.CameraId)
	cc.mu.Unlock()

	if err != nil {
		return nil, err
	}

	cs.close()

	cs.mu.Lock()
	state := cs.state
	cs.mu.Unlock()

	log.Printf("[camera-controller] CloseCamera: %s", req.CameraId)

	return &pb.CloseCameraResponse{
		CameraId: req.CameraId,
		State:    state,
	}, nil
}

// CloseAll closes every open camera. Intended for graceful server shutdown so
// no ffmpeg/V4L2 subprocess is left behind.
func (cc *Controller) CloseAll() {
	cc.mu.RLock()
	cams := make([]*cameraState, 0, len(cc.cameras))
	for _, cs := range cc.cameras {
		cams = append(cams, cs)
	}
	cc.mu.RUnlock()

	for _, cs := range cams {
		cs.close()
	}
}

// close stops the capture loop and tears down the reader/driver for a single
// camera, leaving it in CLOSED state. It must NOT be called while holding
// cs.mu: it waits for the capture loop, whose panic handler locks cs.mu, so
// holding cs.mu across the wait could deadlock.
func (cs *cameraState) close() {
	// Snapshot + signal stop under cs.mu.
	cs.mu.Lock()
	if cs.state == pb.CameraState_CAMERA_STATE_CLOSED {
		cs.mu.Unlock()
		return
	}
	if cs.captureCancel != nil {
		cs.captureCancel()
	}
	reader := cs.reader
	cs.reader = nil
	cs.mu.Unlock()

	// Close the reader (kills ffmpeg / unblocks captureLoop on
	// <-reader.Frames()) outside cs.mu.
	if reader != nil {
		_ = reader.Close()
	}

	// Wait for the capture loop to finish — outside cs.mu so its panic
	// handler (which locks cs.mu) cannot deadlock against us.
	cs.captureWG.Wait()

	cs.mu.Lock()
	_ = cs.driver.Close()
	cs.state = pb.CameraState_CAMERA_STATE_CLOSED
	cs.lastFrame.Store(nil)
	cs.paramSets.Store(nil) // clear stale SPS/PPS so a reopened camera (especially with a different encoding) is not primed with old params
	cs.mu.Unlock()
}

// CaptureFrame returns the most recent frame from the specified camera.
// Non-blocking — reads from the background capture loop's latest frame.
// If no frame is available yet, returns FailedPrecondition.
func (cc *Controller) CaptureFrame(
	ctx context.Context, req *pb.CaptureFrameRequest,
) (*pb.CaptureFrameResponse, error) {
	cc.mu.RLock()
	cs, err := cc.lookup(req.CameraId)
	cc.mu.RUnlock()

	if err != nil {
		return nil, err
	}

	last, err := loadFrame(cs)
	if err != nil {
		return nil, err
	}

	// No per-request transcoding: the frame is returned in whatever
	// encoding the camera was opened with. The client opens the camera with
	// the encoding it wants and converts locally if needed.
	return &pb.CaptureFrameResponse{
		CameraId:    req.CameraId,
		Data:        copyFrameData(last.Data),
		Width:       int32(last.Width),
		Height:      int32(last.Height),
		Encoding:    last.Encoding,
		TimestampNs: last.Timestamp.UnixNano(),
	}, nil
}

// CaptureFrames returns the most recent frame from multiple cameras in a
// single request. Each camera is read concurrently (the per-camera frame
// pointers are loaded in parallel goroutines), so the latency is bounded by
// the slowest camera rather than the sum.
//
// Partial failures are reported per camera in CapturedFrame.error_code /
// error rather than failing the whole RPC. A camera that is not registered
// still produces an entry (NotFound) so the client can correlate by
// camera_id. Duplicate ids in the request are de-duplicated while preserving
// the order of first appearance.
func (cc *Controller) CaptureFrames(
	ctx context.Context, req *pb.CaptureFramesRequest,
) (*pb.CaptureFramesResponse, error) {
	// De-duplicate while preserving order of first appearance.
	seen := make(map[string]struct{}, len(req.CameraIds))
	ids := make([]string, 0, len(req.CameraIds))
	for _, id := range req.CameraIds {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, status.Errorf(codes.InvalidArgument,
			"camera_ids is empty")
	}

	// Snapshot the camera states under the read lock so concurrent
	// OpenCamera/CloseCamera can't invalidate the pointers mid-scan.
	cc.mu.RLock()
	states := make([]*cameraState, len(ids))
	notFound := make(map[string]bool)
	for i, id := range ids {
		cs, ok := cc.cameras[id]
		if !ok {
			notFound[id] = true
		}
		states[i] = cs
	}
	cc.mu.RUnlock()

	type result struct {
		idx int
		cf  *pb.CapturedFrame
	}

	var wg sync.WaitGroup
	results := make([]*pb.CapturedFrame, len(ids))
	ch := make(chan result, len(ids))

	for i, cs := range states {
		if cs == nil {
			// NotFound — filled below; no goroutine.
			continue
		}
		wg.Add(1)
		go func(idx int, cs *cameraState) {
			defer wg.Done()

			cf := &pb.CapturedFrame{CameraId: cs.cfg.ID}
			last, err := loadFrame(cs)
			if err != nil {
				cf.ErrorCode = int32(status.Code(err))
				cf.Error = err.Error()
			} else {
				cf.Data = copyFrameData(last.Data)
				cf.Width = int32(last.Width)
				cf.Height = int32(last.Height)
				cf.Encoding = last.Encoding
				cf.TimestampNs = last.Timestamp.UnixNano()
				cf.ErrorCode = int32(codes.OK)
			}
			ch <- result{idx: idx, cf: cf}
		}(i, cs)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	for r := range ch {
		results[r.idx] = r.cf
	}

	// Fill NotFound entries (no goroutine was launched for them).
	for i, id := range ids {
		if results[i] == nil {
			results[i] = &pb.CapturedFrame{
				CameraId:  id,
				ErrorCode: int32(codes.NotFound),
				Error:     fmt.Sprintf("camera %q not registered", id),
			}
		}
	}

	return &pb.CaptureFramesResponse{Frames: results}, nil
}

// loadFrame returns the latest buffered frame from an open camera, or a gRPC
// error (FailedPrecondition if not open, Unavailable if no frame yet,
// Unimplemented for bitstream encodings) describing why the frame could not be
// captured. The returned *Frame is the shared latest-frame slot; the caller
// must copyFrameData if it needs a stable byte buffer.
//
// This is the single source of truth for single-frame capture validation,
// shared by CaptureFrame (which surfaces the error as a gRPC status) and
// CaptureFrames (which encodes it per camera in CapturedFrame.error_code).
func loadFrame(cs *cameraState) (*Frame, error) {
	cs.mu.Lock()
	state := cs.state
	cs.mu.Unlock()

	cameraID := cs.cfg.ID

	if state != pb.CameraState_CAMERA_STATE_OPEN {
		return nil, status.Errorf(codes.FailedPrecondition,
			"camera %q is not open (state=%v)", cameraID, state)
	}

	// Get the latest frame from the background capture loop.
	last := cs.lastFrame.Load()
	if last == nil {
		return nil, status.Errorf(codes.Unavailable,
			"camera %q: no frame available yet", cameraID)
	}

	// Bitstream sources (h264/h265) cannot yield a single decodable frame
	// on demand — a single NAL chunk lacks the SPS/PPS/IDR context needed
	// to decode. The client must use WatchFrames, or open the camera in
	// frame-mode (jpeg/png/bmp/tiff).
	if isBitstream(last.Encoding) {
		return nil, status.Errorf(codes.Unimplemented,
			"camera %q captures a bitstream (%q); a frame-mode encoding (jpeg/png/bmp/tiff) is required",
			cameraID, last.Encoding)
	}

	return last, nil
}

// WatchFrames streams frames continuously from an open camera.
// Subscribes to the background capture loop's broadcast and forwards every
// frame in the encoding the camera was opened with.
//
// There is no per-subscriber transcoding: the request's encoding field is
// intentionally ignored. A bitstream source (h264/h265) is streamed verbatim
// (chunks of NAL units with keyframe flags); a frame-mode source
// (jpeg/png/bmp/tiff) streams independently-decodable frames. Clients that
// need a different encoding should open the camera with it (or convert the
// stream themselves).
//
// Multiple concurrent WatchFrames calls are allowed — each gets its own
// subscriber channel and receives all frames independently.
func (cc *Controller) WatchFrames(
	req *pb.WatchFramesRequest, stream pb.CameraController_WatchFramesServer,
) error {
	cc.mu.RLock()
	cs, err := cc.lookup(req.CameraId)
	cc.mu.RUnlock()

	if err != nil {
		return err
	}

	cs.mu.Lock()
	state := cs.state
	captureCtx := cs.captureCtx
	enc := cs.actualEnc
	cs.mu.Unlock()

	if state != pb.CameraState_CAMERA_STATE_OPEN {
		return status.Errorf(codes.FailedPrecondition,
			"camera %q is not open (state=%v)", req.CameraId, state)
	}

	// Create subscriber channel and register.
	sub := make(chan *Frame, 8)

	// Build a priming frame from cached parameter sets (if any) so a late
	// joiner receives SPS/PPS before the live stream and can decode the
	// next IDR. Pushed under subMu so it is the first message delivered,
	// ahead of any live frame the capture loop may push concurrently.
	// Only applies to bitstream sources (h264/h265); frame-mode encodings
	// (jpeg/png/bmp/tiff) emit independently decodable frames and need no
	// primer.
	var prime *Frame
	if isBitstream(enc) {
		blob := cs.paramSets.Load()
		if blob.hasParamSets() {
			pw, ph := 0, 0
			if last := cs.lastFrame.Load(); last != nil {
				pw, ph = last.Width, last.Height
			}
			prime = buildPrimeFrame(blob, enc, pw, ph)
			log.Printf("[camera-controller] priming WatchFrames for %s with cached SPS+PPS (%d bytes)",
				req.CameraId, len(prime.Data))
		} else if blob != nil && len(blob.pps) > 0 && len(blob.sps) == 0 {
			log.Printf("[camera-controller] WARNING: cannot prime %s — PPS cached but no SPS; "+
				"set params.sps (hex) in the camera config", req.CameraId)
		}
	}

	cs.subMu.Lock()
	id := cs.subCounter
	cs.subCounter++
	cs.subscribers[id] = sub
	if prime != nil {
		select {
		case sub <- prime:
		default:
		}
	}
	cs.subMu.Unlock()

	// Cleanup on exit: unregister the subscriber.
	defer func() {
		cs.subMu.Lock()
		delete(cs.subscribers, id)
		cs.subMu.Unlock()
	}()

	ctx := stream.Context()
	lastSeq := uint64(0)

	// For bitstream sources (h264/h265), discard frames before the first
	// IDR. P-slices before the first keyframe are not decodable (no
	// reference frame), and standalone SPS/PPS lack a slice. The first
	// IDR arrives with SPS/PPS prepended by injectBitstreamParams, so it
	// is self-contained and decodable. Frame-mode sources
	// (jpeg/png/bmp/tiff) emit independently decodable frames, so no
	// filtering is needed.
	seenKeyframe := !isBitstream(enc)

	// Read frames from the subscriber channel.
	// The capture loop pushes a shared copy of each frame; downstream only
	// reads it (no Release needed).
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-captureCtx.Done():
			return nil // camera closed
		case frame, ok := <-sub:
			if !ok {
				return nil
			}

			frameMode := !isBitstream(frame.Encoding)

			// Discard undecodable bitstream frames before the first IDR.
			if !seenKeyframe && !frameMode {
				if !frameContainsIDR(parseNALUnits(frame.Data), frame.Encoding) {
					continue
				}
				seenKeyframe = true
			}

			// In frame mode, skip duplicates (Sequence is a monotonic
			// counter). Bitstream Sequence is a byte offset and never
			// duplicates, so we don't skip.
			if frameMode && frame.Sequence > 0 && frame.Sequence <= lastSeq {
				continue
			}
			if frame.Sequence > lastSeq {
				lastSeq = frame.Sequence
			}

			// Frame-mode frames are independently decodable (keyframe).
			// For bitstream, flag IDR access points.
			keyframe := frameMode
			if !frameMode && len(frame.Data) >= 5 {
				keyframe = isKeyframeNAL(frame.Data, frame.Encoding)
			}

			msg := &pb.VideoFrame{
				Data:        frame.Data,
				Width:       int32(frame.Width),
				Height:      int32(frame.Height),
				Encoding:    frame.Encoding,
				TimestampNs: frame.Timestamp.UnixNano(),
				Sequence:    frame.Sequence,
				Keyframe:    keyframe,
			}

			if err := stream.Send(msg); err != nil {
				return fmt.Errorf("send frame: %w", err)
			}
		}
	}
}

// --------------------------------------------------------------------------
// Background capture loop
// --------------------------------------------------------------------------

// captureLoop runs continuously while the camera is OPEN.
// It reads frames from the driver's FrameReader, stores the latest frame,
// and broadcasts a copy to every subscriber.
//
// On exit (whether via ctx cancellation or reader failure), the capture
// context is cancelled so that all WatchFrames subscribers are unblocked.
func (cs *cameraState) captureLoop(ctx context.Context, reader FrameReader) {
	defer cs.captureWG.Done()
	defer cs.captureCancel() // Cancel context on any exit — unblocks watchers.

	// Recover from panics to avoid leaving the camera in a stuck OPEN state.
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[camera-controller] captureLoop panic: %v", r)
			cs.mu.Lock()
			cs.state = pb.CameraState_CAMERA_STATE_ERROR
			cs.mu.Unlock()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case frame, ok := <-reader.Frames():
			if !ok {
				// The reader stopped on its own while captureCtx is still
				// alive — the capture source failed (e.g. ffmpeg exited
				// with a non-zero status). Surface it on the camera state
				// and print the failure reason (including ffmpeg's stderr
				// tail). A clean Close() cancels captureCtx first, so this
				// branch is only reached for spontaneous failures.
				if err := reader.Err(); err != nil {
					log.Printf("[camera-controller] camera %q: capture source stopped: %v",
						cs.cfg.ID, err)
					cs.mu.Lock()
					cs.state = pb.CameraState_CAMERA_STATE_ERROR
					cs.mu.Unlock()
				}
				return
			}

			// One copy detaches the data from any zero-copy driver buffer
			// (e.g. V4L2 mmap). The same copy is shared with every
			// subscriber and the latest-frame slot — downstream only reads
			// it (CaptureFrame copies again; WatchFrames serializes it), so
			// sharing is safe and avoids N+1 copies per frame.
			shared := copyFrame(frame)
			frame.Release()

			// For bitstream sources (h264/h265), cache parameter sets seen in
			// the stream and make IDR keyframes self-contained by prepending
			// cached SPS/PPS. This lets late-joining WatchFrames clients
			// decode the next IDR even if the camera only emits SPS/PPS once
			// at stream start.
			if isBitstream(shared.Encoding) {
				injectBitstreamParams(cs, shared)
			}

			cs.subMu.Lock()
			for _, ch := range cs.subscribers {
				select {
				case ch <- shared:
				default:
					// Drop if subscriber is slow.
				}
			}
			cs.subMu.Unlock()

			cs.lastFrame.Store(shared)
		}
	}
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

// lookup returns the cameraState for a given camera_id.
// Caller must hold cc.mu (read or write).
func (cc *Controller) lookup(cameraID string) (*cameraState, error) {
	cs, ok := cc.cameras[cameraID]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "camera %q not registered", cameraID)
	}
	return cs, nil
}

// cameraDescriptor converts internal state to a protobuf CameraDescriptor.
func cameraDescriptor(cs *cameraState) *pb.CameraDescriptor {
	cs.mu.Lock()
	state := cs.state
	cs.mu.Unlock()

	resolutions := cs.cfg.SupportedResolutions()
	fpsList := cs.cfg.SupportedFPS()
	fps32 := make([]int32, len(fpsList))
	for i, f := range fpsList {
		fps32[i] = int32(f)
	}

	return &pb.CameraDescriptor{
		CameraId:             cs.cfg.ID,
		Name:                 cs.cfg.Name,
		CameraType:           cs.cfg.CameraType,
		SerialNumber:         cs.cfg.SerialNumber,
		Width:                int32(cs.cfg.Width),
		Height:               int32(cs.cfg.Height),
		Fps:                  int32(cs.cfg.FPS),
		EnableDepth:          cs.cfg.EnableDepth,
		State:                state,
		SupportedResolutions: resolutions,
		SupportedFps:         fps32,
		PixelFormat:          cs.cfg.Param("pixel_format", ""),
	}
}

// copyFrame creates a deep copy of a Frame, including the data buffer.
func copyFrame(src *Frame) *Frame {
	return &Frame{
		Data:      copyFrameData(src.Data),
		Width:     src.Width,
		Height:    src.Height,
		Encoding:  src.Encoding,
		Timestamp: src.Timestamp,
		Sequence:  src.Sequence,
	}
}

// copyFrameData creates a copy of a byte slice.
func copyFrameData(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
