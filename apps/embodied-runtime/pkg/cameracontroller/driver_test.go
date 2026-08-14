package cameracontroller

import (
	"bytes"
	"testing"
	"time"
)

// TestCameraConfig_Param verifies that Param returns the configured value when
// present (and non-empty), and falls back to the default otherwise.
func TestCameraConfig_Param(t *testing.T) {
	cfg := CameraConfig{
		ID:     "cam0",
		Params: map[string]string{"device": "/dev/video0", "empty": ""},
	}
	if got := cfg.Param("device", "/dev/video1"); got != "/dev/video0" {
		t.Errorf("Param(device) = %q, want /dev/video0", got)
	}
	if got := cfg.Param("empty", "fallback"); got != "fallback" {
		t.Errorf("Param(empty) = %q, want fallback", got)
	}
	if got := cfg.Param("missing", "fallback"); got != "fallback" {
		t.Errorf("Param(missing) = %q, want fallback", got)
	}
	if got := (CameraConfig{}).Param("x", "def"); got != "def" {
		t.Errorf("Param on nil map = %q, want def", got)
	}
}

// TestFrameRelease verifies that Release invokes the release callback exactly
// once and is safe to call multiple times.
func TestFrameRelease(t *testing.T) {
	calls := 0
	f := &Frame{release: func() { calls++ }}
	f.Release()
	f.Release()
	if calls != 1 {
		t.Errorf("release called %d times, want 1", calls)
	}
	f2 := &Frame{}
	f2.Release()
}

// TestNewDriver verifies driver creation for each supported camera type and
// that unsupported types return an error.
func TestNewDriver(t *testing.T) {
	tests := []struct {
		cameraType string
		wantErr    bool
	}{
		{"v4l2", false},
		{"usb_cam", false},
		{"realsense", false},
		{"rtsp", false},
		{"ros_topic", false},
		{"remote", false},
		{"unknown_xyz", true},
		{"", true},
	}
	for _, tt := range tests {
		t.Run(tt.cameraType, func(t *testing.T) {
			d, err := NewDriver(CameraConfig{ID: "c", CameraType: tt.cameraType})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if d == nil {
				t.Fatal("driver is nil")
			}
			_ = d.Close()
		})
	}
}

// TestResolveResolution verifies the fallback chain: explicit args override
// config, config overrides defaults, and zero/missing values fall back to
// 640x480@30.
func TestResolveResolution(t *testing.T) {
	t.Run("explicit args", func(t *testing.T) {
		cfg := CameraConfig{Width: 320, Height: 240, FPS: 15}
		w, h, f := resolveResolution(cfg, 1280, 720, 60)
		if w != 1280 || h != 720 || f != 60 {
			t.Errorf("got %dx%d@%d, want 1280x720@60", w, h, f)
		}
	})
	t.Run("config fallback", func(t *testing.T) {
		cfg := CameraConfig{Width: 800, Height: 600, FPS: 24}
		w, h, f := resolveResolution(cfg, 0, 0, 0)
		if w != 800 || h != 600 || f != 24 {
			t.Errorf("got %dx%d@%d, want 800x600@24", w, h, f)
		}
	})
	t.Run("defaults", func(t *testing.T) {
		w, h, f := resolveResolution(CameraConfig{}, 0, 0, 0)
		if w != 640 || h != 480 || f != 30 {
			t.Errorf("got %dx%d@%d, want 640x480@30", w, h, f)
		}
	})
	t.Run("negative zeroed", func(t *testing.T) {
		w, h, f := resolveResolution(CameraConfig{Width: 100, Height: 50, FPS: 5}, -1, -1, -1)
		if w != 100 || h != 50 || f != 5 {
			t.Errorf("got %dx%d@%d, want 100x50@5", w, h, f)
		}
	})
}

// TestEncodingToFFmpegOutput verifies the encoding -> (format, codec) mapping.
// h264/h265 use raw bitstream muxers; png/bmp/tiff use image2pipe with the
// matching encoder; jpeg (and any other value, including the empty string)
// returns ("", "") so callers fall back to the JPEG pipe.
func TestEncodingToFFmpegOutput(t *testing.T) {
	tests := []struct {
		enc       string
		wantFmt   string
		wantCodec string
	}{
		{"h264", "h264", "libx264"},
		{"h265", "hevc", "libx265"},
		{"jpeg", "", ""},
		{"png", "image2pipe", "png"},
		{"bmp", "image2pipe", "bmp"},
		{"tiff", "image2pipe", "tiff"},
		{"bgr8", "", ""},
		{"rgb8", "", ""},
		{"", "", ""},
		{"unknown", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.enc, func(t *testing.T) {
			gotFmt, gotCodec := encodingToFFmpegOutput(tt.enc)
			if gotFmt != tt.wantFmt || gotCodec != tt.wantCodec {
				t.Errorf("encodingToFFmpegOutput(%q) = (%q,%q), want (%q,%q)",
					tt.enc, gotFmt, gotCodec, tt.wantFmt, tt.wantCodec)
			}
		})
	}
}

// TestIsBitstream verifies bitstream encoding detection. Still-image encodings
// (jpeg/png/bmp/tiff) are frame-mode and must not be reported as bitstreams.
func TestIsBitstream(t *testing.T) {
	tests := []struct {
		enc  string
		want bool
	}{
		{"h264", true},
		{"h265", true},
		{"hevc", true},
		{"jpeg", false},
		{"png", false},
		{"bmp", false},
		{"tiff", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.enc, func(t *testing.T) {
			if got := isBitstream(tt.enc); got != tt.want {
				t.Errorf("isBitstream(%q) = %v, want %v", tt.enc, got, tt.want)
			}
		})
	}
}

// TestBuildRealsenseArgs verifies the ffmpeg argument list for realsense
// cameras: v4l2 input, resolution and framerate, and the requested output
// encoding.
func TestBuildRealsenseArgs(t *testing.T) {
	cfg := CameraConfig{
		ID:     "rs0",
		Params: map[string]string{"device": "/dev/video4"},
	}
	args := buildRealsenseArgs(cfg, 640, 480, 30, "h264")
	want := []string{
		"-f", "v4l2",
		"-video_size", "640x480",
		"-framerate", "30",
		"-i", "/dev/video4",
		"-f", "h264",
		"-vcodec", "libx264",
		"-q:v", "2",
		"-",
	}
	if len(args) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

// TestBuildRealsenseArgs_DefaultDevice verifies the device fallback to
// /dev/video0 when not configured.
func TestBuildRealsenseArgs_DefaultDevice(t *testing.T) {
	args := buildRealsenseArgs(CameraConfig{}, 0, 0, 0, "")
	for i, v := range args {
		if v == "-i" && args[i+1] != "/dev/video0" {
			t.Errorf("default device = %q, want /dev/video0", args[i+1])
		}
	}
	if !containsPair(args, "-f", "image2pipe") || !containsPair(args, "-vcodec", "mjpeg") {
		t.Errorf("unknown encoding should fall back to mjpeg pipe: %v", args)
	}
}

// TestBuildRTSPArgs verifies the ffmpeg argument list for RTSP cameras,
// including the transport and URL passthrough.
func TestBuildRTSPArgs(t *testing.T) {
	cfg := CameraConfig{
		ID: "rtsp0",
		Params: map[string]string{
			"url":       "rtsp://10.0.0.1/stream",
			"transport": "udp",
		},
	}
	args := buildRTSPArgs(cfg, 1280, 720, 30, "h265")
	want := []string{
		"-rtsp_transport", "udp",
		"-i", "rtsp://10.0.0.1/stream",
		"-f", "hevc",
		"-vcodec", "libx265",
		"-q:v", "2",
		"-",
	}
	if len(args) != len(want) {
		t.Fatalf("got %d args, want %d: %v", len(args), len(want), args)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, args[i], want[i])
		}
	}
}

// TestBuildRTSPArgs_DefaultTransport verifies the transport defaults to tcp.
func TestBuildRTSPArgs_DefaultTransport(t *testing.T) {
	cfg := CameraConfig{Params: map[string]string{"url": "rtsp://x"}}
	args := buildRTSPArgs(cfg, 0, 0, 0, "")
	if !containsPair(args, "-rtsp_transport", "tcp") {
		t.Errorf("default transport should be tcp: %v", args)
	}
	if !containsPair(args, "-i", "rtsp://x") {
		t.Errorf("url not passed through: %v", args)
	}
}

// TestCopyFrame verifies that copyFrame produces a deep copy: the data slice
// is independent and metadata is preserved.
func TestCopyFrame(t *testing.T) {
	src := &Frame{
		Data:      []byte{1, 2, 3},
		Width:     640,
		Height:    480,
		Encoding:  "h264",
		Timestamp: time.Now(),
		Sequence:  42,
	}
	dst := copyFrame(src)
	if !bytes.Equal(dst.Data, src.Data) {
		t.Errorf("data mismatch")
	}
	dst.Data[0] = 99
	if src.Data[0] == 99 {
		t.Error("copyFrame did not deep-copy the data buffer")
	}
	if dst.Width != src.Width || dst.Height != src.Height ||
		dst.Encoding != src.Encoding || dst.Sequence != src.Sequence {
		t.Errorf("metadata mismatch: %+v", dst)
	}
	if !dst.Timestamp.Equal(src.Timestamp) {
		t.Errorf("timestamp mismatch: %v vs %v", dst.Timestamp, src.Timestamp)
	}
}

// TestCopyFrameData verifies that copyFrameData returns an independent slice
// and handles nil/empty input.
func TestCopyFrameData(t *testing.T) {
	src := []byte{1, 2, 3}
	dst := copyFrameData(src)
	if !bytes.Equal(dst, src) {
		t.Fatal("content mismatch")
	}
	dst[0] = 99
	if src[0] == 99 {
		t.Error("not independent")
	}
	if got := copyFrameData(nil); len(got) != 0 {
		t.Errorf("nil input should yield empty slice, got %d", len(got))
	}
	if got := copyFrameData([]byte{}); len(got) != 0 {
		t.Errorf("empty input should yield empty slice, got %d", len(got))
	}
}

// containsPair checks if a flat []string contains two consecutive elements.
func containsPair(args []string, k, v string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == k && args[i+1] == v {
			return true
		}
	}
	return false
}
