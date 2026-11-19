package cameracontroller

import (
	"context"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Frame — a single captured frame with Release() for zero-copy
// ---------------------------------------------------------------------------

// Frame represents a single captured frame from a camera.
// After processing, the consumer MUST call Release() to allow the
// driver to reuse the underlying buffer.
type Frame struct {
	Data      []byte
	Width     int
	Height    int
	Encoding  string
	Timestamp time.Time
	Sequence  uint64

	release func()
}

// Release marks the frame as consumed so the driver can reuse the buffer.
// Must be called exactly once per frame. Safe to call multiple times.
func (f *Frame) Release() {
	if f.release != nil {
		f.release()
		f.release = nil
	}
}

// ---------------------------------------------------------------------------
// FrameReader — a stream of frames from an open camera
// ---------------------------------------------------------------------------

// FrameReader provides a channel-based stream of captured frames.
// Multiple goroutines can read from Frames(), but typically there is
// one consumer that reads and releases frames in order.
//
//	reader, _ := driver.Open(ctx, cfg, 640, 480, 30)
//	defer reader.Close()
//	for f := range reader.Frames() {
//	    process(f.Data)
//	    f.Release()
//	}
type FrameReader interface {
	// Frames returns a channel that delivers captured frames as they
	// become available from the hardware. The driver pushes frames
	// into this channel; the consumer reads and releases them.
	Frames() <-chan *Frame

	// Close stops the frame stream and releases all resources.
	// After Close, the Frames() channel will be closed.
	Close() error

	// Err returns the error that caused the reader to stop on its own
	// (e.g. ffmpeg exiting with a non-zero status, or a remote stream
	// dropping), or nil if it stopped cleanly via Close(). The
	// controller's capture loop consults this when Frames() is closed
	// to decide whether to mark the camera ERROR.
	Err() error
}

// ---------------------------------------------------------------------------
// CameraDriver — interface for all camera capture implementations
// ---------------------------------------------------------------------------

// CameraDriver abstracts the capture mechanism for a camera device.
// Each camera type (v4l2, realsense, rtsp, ros_topic, remote) implements
// this interface.
//
// Usage:
//
//	driver, _ := NewDriver(cfg)
//	defer driver.Close()
//	reader, _ := driver.Open(ctx, cfg, 640, 480, 30, "h264")
//	defer reader.Close()
//	f := <-reader.Frames()
//	defer f.Release()
type CameraDriver interface {
	// Open starts the capture pipeline and returns a FrameReader plus the
	// actual encoding the reader produces. width/height/fps override the
	// camera config defaults (0 = use default).
	//
	// encodingHint tells the driver what encoding the consumer wants.
	// "jpeg", "png", "bmp", and "tiff" are frame-mode still-image encodings;
	// "h264" and "h265" are bitstream encodings. The ffmpeg driver captures
	// in whatever format the device provides and transcodes to the requested
	// output via ffmpeg's codec pipeline.
	Open(ctx context.Context, cfg CameraConfig, width, height, fps int, encodingHint string) (FrameReader, string, error)

	// Close stops the capture pipeline and releases all resources.
	Close() error
}

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

// NewDriver creates a CameraDriver for the given camera type.
// V4L2, RealSense, and RTSP cameras all use the ffmpeg driver, which
// handles pixel-format negotiation and transcoding via ffmpeg's codec
// pipeline — no native V4L2 code is needed.
func NewDriver(cfg CameraConfig) (CameraDriver, error) {
	switch cfg.CameraType {
	case "v4l2", "usb_cam":
		return newFFmpegDriver(buildV4L2Args), nil

	case "realsense":
		return newFFmpegDriver(buildRealsenseArgs), nil

	case "rtsp":
		return newFFmpegDriver(buildRTSPArgs), nil

	case "ros_topic":
		return newROSTopicDriver(), nil

	case "remote":
		return newRemoteDriver(cfg), nil

	default:
		return nil, fmt.Errorf("unsupported camera type: %q", cfg.CameraType)
	}
}

// ---------------------------------------------------------------------------
// FFmpeg argument builders
// ---------------------------------------------------------------------------

func resolveResolution(cfg CameraConfig, width, height, fps int) (w, h, f int) {
	w = width
	if w <= 0 {
		w = cfg.Width
	}
	if w <= 0 {
		w = 640
	}

	h = height
	if h <= 0 {
		h = cfg.Height
	}
	if h <= 0 {
		h = 480
	}

	f = fps
	if f <= 0 {
		f = cfg.FPS
	}
	if f <= 0 {
		f = 30
	}
	return
}

// buildV4L2Args constructs ffmpeg arguments for a V4L2 (USB) camera. No
// -input_format is passed; ffmpeg auto-detects the best format the device
// supports. The output is always one of the supported still-image or bitstream
// encodings — ffmpeg transcodes automatically if the input format differs
// from the requested output.
func buildV4L2Args(cfg CameraConfig, width, height, fps int, encodingHint string) []string {
	w, h, f := resolveResolution(cfg, width, height, fps)
	device := cfg.Param("device", "/dev/video0")

	outputFmt, outputCodec := encodingToFFmpegOutput(encodingHint)
	if outputFmt == "" {
		outputFmt = "image2pipe"
		outputCodec = "mjpeg"
	}

	return []string{
		"-f", "v4l2",
		"-video_size", fmt.Sprintf("%dx%d", w, h),
		"-framerate", fmt.Sprintf("%d", f),
		"-i", device,
		"-f", outputFmt,
		"-vcodec", outputCodec,
		"-q:v", "2",
		"-",
	}
}

func buildRealsenseArgs(cfg CameraConfig, width, height, fps int, encodingHint string) []string {
	w, h, f := resolveResolution(cfg, width, height, fps)
	device := cfg.Param("device", "/dev/video0")

	outputFmt, outputCodec := encodingToFFmpegOutput(encodingHint)
	if outputFmt == "" {
		outputFmt = "image2pipe"
		outputCodec = "mjpeg"
	}

	return []string{
		"-f", "v4l2",
		"-video_size", fmt.Sprintf("%dx%d", w, h),
		"-framerate", fmt.Sprintf("%d", f),
		"-i", device,
		"-f", outputFmt,
		"-vcodec", outputCodec,
		"-q:v", "2",
		"-",
	}
}

func buildRTSPArgs(cfg CameraConfig, width, height, fps int, encodingHint string) []string {
	url := cfg.Param("url", "")
	transport := cfg.Param("transport", "tcp")

	outputFmt, outputCodec := encodingToFFmpegOutput(encodingHint)
	if outputFmt == "" {
		outputFmt = "image2pipe"
		outputCodec = "mjpeg"
	}

	return []string{
		"-rtsp_transport", transport,
		"-i", url,
		"-f", outputFmt,
		"-vcodec", outputCodec,
		"-q:v", "2",
		"-",
	}
}

// encodingToFFmpegOutput maps our encoding names to ffmpeg output format and
// codec. The lossless still-image encodings (png, bmp, tiff) are emitted via
// ffmpeg's image2pipe muxer with the corresponding encoder; h264/h265 use the
// raw bitstream muxers. jpeg (and any other value, including the empty
// string) returns ("", ""), causing the caller to fall back to the JPEG pipe
// (image2pipe + mjpeg).
func encodingToFFmpegOutput(enc string) (format, codec string) {
	switch enc {
	case "h264":
		return "h264", "libx264"
	case "h265":
		return "hevc", "libx265"
	case "png":
		return "image2pipe", "png"
	case "bmp":
		return "image2pipe", "bmp"
	case "tiff":
		return "image2pipe", "tiff"
	default:
		return "", "" // fall back to JPEG pipe
	}
}

// isBitstream reports whether the encoding is a compressed video bitstream
// (h264/h265). Bitstream frames cannot be transcoded individually because a
// single NAL unit lacks the SPS/PPS/IDR context required to decode it.
// Still-image encodings (jpeg/png/bmp/tiff) are frame-mode: each frame is
// independently decodable.
func isBitstream(enc string) bool {
	return enc == "h264" || enc == "h265" || enc == "hevc"
}
