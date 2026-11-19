package cameracontroller

import (
	"context"
	"testing"
	"time"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// registerTestCamera registers a camera with the given id and returns its
// internal state so tests can drive it directly (set state, store frames).
func registerTestCamera(t *testing.T, cc *Controller, id string) *cameraState {
	t.Helper()
	if err := cc.RegisterCamera(CameraConfig{
		ID:         id,
		Name:       id,
		CameraType: "ros_topic", // stub driver — no real device needed
		Width:      4,
		Height:     4,
		FPS:        1,
	}); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	cc.mu.RLock()
	defer cc.mu.RUnlock()
	return cc.cameras[id]
}

func setOpen(cs *cameraState) {
	cs.mu.Lock()
	cs.state = pb.CameraState_CAMERA_STATE_OPEN
	cs.mu.Unlock()
}

func storeFrame(cs *cameraState, enc string, data []byte, ts time.Time) {
	cs.lastFrame.Store(&Frame{
		Data:      data,
		Width:     4,
		Height:    4,
		Encoding:  enc,
		Timestamp: ts,
		Sequence:  1,
	})
}

// TestCaptureFrames_AllOK verifies that a concurrent multi-camera capture
// returns each camera's latest frame in request order, with OK error codes.
func TestCaptureFrames_AllOK(t *testing.T) {
	cc := New()
	cs0 := registerTestCamera(t, cc, "cam-0")
	cs1 := registerTestCamera(t, cc, "cam-1")
	setOpen(cs0)
	setOpen(cs1)

	t0 := time.Unix(1_700_000_000, 0)
	t1 := t0.Add(3 * time.Millisecond)
	storeFrame(cs0, "jpeg", []byte("frame0"), t0)
	storeFrame(cs1, "jpeg", []byte("frame1"), t1)

	resp, err := cc.CaptureFrames(context.Background(), &pb.CaptureFramesRequest{
		CameraIds: []string{"cam-0", "cam-1"},
	})
	if err != nil {
		t.Fatalf("CaptureFrames: %v", err)
	}
	if len(resp.Frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(resp.Frames))
	}
	if got := resp.Frames[0].GetCameraId(); got != "cam-0" {
		t.Errorf("frames[0].camera_id = %q, want cam-0", got)
	}
	if got := resp.Frames[1].GetCameraId(); got != "cam-1" {
		t.Errorf("frames[1].camera_id = %q, want cam-1", got)
	}
	if string(resp.Frames[0].GetData()) != "frame0" {
		t.Errorf("frames[0].data = %q", resp.Frames[0].GetData())
	}
	if string(resp.Frames[1].GetData()) != "frame1" {
		t.Errorf("frames[1].data = %q", resp.Frames[1].GetData())
	}
	// Timestamps preserved per camera (slightly different is acceptable).
	if resp.Frames[0].GetTimestampNs() != t0.UnixNano() {
		t.Errorf("frames[0].timestamp_ns mismatch")
	}
	if resp.Frames[1].GetTimestampNs() != t1.UnixNano() {
		t.Errorf("frames[1].timestamp_ns mismatch")
	}
	for i, f := range resp.Frames {
		if f.GetErrorCode() != int32(codes.OK) {
			t.Errorf("frames[%d].error_code = %d, want 0", i, f.GetErrorCode())
		}
	}
}

// TestCaptureFrames_PartialFailure verifies that a camera which is not open
// is reported with an error in its CapturedFrame while the open camera still
// returns its frame — the whole RPC does not fail.
func TestCaptureFrames_PartialFailure(t *testing.T) {
	cc := New()
	cs0 := registerTestCamera(t, cc, "cam-0")
	registerTestCamera(t, cc, "cam-1")
	setOpen(cs0)
	// cam-1 left CLOSED.
	storeFrame(cs0, "jpeg", []byte("ok"), time.Now())

	resp, err := cc.CaptureFrames(context.Background(), &pb.CaptureFramesRequest{
		CameraIds: []string{"cam-0", "cam-1"},
	})
	if err != nil {
		t.Fatalf("CaptureFrames: %v", err)
	}
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
	if resp.Frames[1].GetError() == "" {
		t.Error("cam-1 error message is empty")
	}
}

// TestCaptureFrames_NotFound verifies that an unregistered camera id is
// returned with a NotFound error code rather than failing the RPC.
func TestCaptureFrames_NotFound(t *testing.T) {
	cc := New()
	resp, err := cc.CaptureFrames(context.Background(), &pb.CaptureFramesRequest{
		CameraIds: []string{"ghost"},
	})
	if err != nil {
		t.Fatalf("CaptureFrames: %v", err)
	}
	if len(resp.Frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(resp.Frames))
	}
	if resp.Frames[0].GetErrorCode() != int32(codes.NotFound) {
		t.Errorf("error_code = %d, want NotFound", resp.Frames[0].GetErrorCode())
	}
	if resp.Frames[0].GetCameraId() != "ghost" {
		t.Errorf("camera_id = %q, want ghost", resp.Frames[0].GetCameraId())
	}
}

// TestCaptureFrames_BitstreamRejected verifies that a bitstream-encoding
// camera is reported with Unimplemented while a jpeg camera in the same
// request succeeds.
func TestCaptureFrames_BitstreamRejected(t *testing.T) {
	cc := New()
	cs0 := registerTestCamera(t, cc, "cam-0")
	cs1 := registerTestCamera(t, cc, "cam-1")
	setOpen(cs0)
	setOpen(cs1)
	storeFrame(cs0, "jpeg", []byte("ok"), time.Now())
	storeFrame(cs1, "h264", []byte("nalu"), time.Now())

	resp, err := cc.CaptureFrames(context.Background(), &pb.CaptureFramesRequest{
		CameraIds: []string{"cam-0", "cam-1"},
	})
	if err != nil {
		t.Fatalf("CaptureFrames: %v", err)
	}
	if resp.Frames[0].GetErrorCode() != int32(codes.OK) {
		t.Errorf("cam-0 error_code = %d, want OK", resp.Frames[0].GetErrorCode())
	}
	if resp.Frames[1].GetErrorCode() != int32(codes.Unimplemented) {
		t.Errorf("cam-1 error_code = %d, want Unimplemented",
			resp.Frames[1].GetErrorCode())
	}
}

// TestCaptureFrames_NoFrameYet verifies that an open camera with no buffered
// frame yet is reported with Unavailable.
func TestCaptureFrames_NoFrameYet(t *testing.T) {
	cc := New()
	cs0 := registerTestCamera(t, cc, "cam-0")
	setOpen(cs0)
	// No frame stored.

	resp, err := cc.CaptureFrames(context.Background(), &pb.CaptureFramesRequest{
		CameraIds: []string{"cam-0"},
	})
	if err != nil {
		t.Fatalf("CaptureFrames: %v", err)
	}
	if resp.Frames[0].GetErrorCode() != int32(codes.Unavailable) {
		t.Errorf("error_code = %d, want Unavailable", resp.Frames[0].GetErrorCode())
	}
}

// TestCaptureFrames_Dedup verifies that duplicate camera ids are
// de-duplicated while preserving order of first appearance.
func TestCaptureFrames_Dedup(t *testing.T) {
	cc := New()
	cs0 := registerTestCamera(t, cc, "cam-0")
	setOpen(cs0)
	storeFrame(cs0, "jpeg", []byte("frame0"), time.Now())

	resp, err := cc.CaptureFrames(context.Background(), &pb.CaptureFramesRequest{
		CameraIds: []string{"cam-0", "cam-0", "cam-0"},
	})
	if err != nil {
		t.Fatalf("CaptureFrames: %v", err)
	}
	if len(resp.Frames) != 1 {
		t.Fatalf("got %d frames, want 1 (deduped)", len(resp.Frames))
	}
}

// TestCaptureFrames_Empty verifies that an empty request is rejected with
// InvalidArgument.
func TestCaptureFrames_Empty(t *testing.T) {
	cc := New()
	_, err := cc.CaptureFrames(context.Background(), &pb.CaptureFramesRequest{
		CameraIds: []string{},
	})
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %T: %v", err, err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", st.Code())
	}
}

// TestCaptureFrames_DataIsCopy verifies that the returned data slice is
// independent of the stored frame's buffer (mutating it doesn't affect the
// source).
func TestCaptureFrames_DataIsCopy(t *testing.T) {
	cc := New()
	cs0 := registerTestCamera(t, cc, "cam-0")
	setOpen(cs0)
	src := []byte("original")
	storeFrame(cs0, "jpeg", src, time.Now())

	resp, err := cc.CaptureFrames(context.Background(), &pb.CaptureFramesRequest{
		CameraIds: []string{"cam-0"},
	})
	if err != nil {
		t.Fatalf("CaptureFrames: %v", err)
	}
	resp.Frames[0].Data[0] = 'X'
	if src[0] == 'X' {
		t.Error("returned data aliases the stored frame buffer")
	}
}
