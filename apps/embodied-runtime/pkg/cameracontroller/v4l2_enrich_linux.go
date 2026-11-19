//go:build linux

package cameracontroller

import (
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"

	"github.com/blackjack/webcam"
)

// EnrichV4L2Config opens the device and enumerates its supported pixel formats,
// frame sizes, and framerates via V4L2 ioctls (VIDIOC_ENUM_FMT /
// VIDIOC_ENUM_FRAMESIZES / VIDIOC_ENUM_FRAMEINTERVALS). It populates
// cfg.Width/Height/FPS with a sensible default and records the full capability
// set in cfg.Params so the device's supported modes are discoverable.
//
// Only V4L2-backed cameras (camera_type "v4l2" or "usb_cam") carry a
// /dev/videoN node to probe — RTSP, realsense, ROS-topic, and remote cameras
// are skipped, since they have no device node and a V4L2 ioctl query would
// fail or be misleading.
//
// Best-effort: on any error (no permission, busy device, no device node, or a
// non-capture node) the config is left with its sysfs-only values, so the
// camera is still registered and can be opened later by the camera-controller.
func EnrichV4L2Config(cfg *CameraConfig) {
	switch cfg.CameraType {
	case "v4l2", "usb_cam":
		// V4L2-backed — proceed to probe the device node.
	default:
		return // no /dev/videoN to probe
	}

	device := cfg.Param("device", "/dev/"+cfg.ID)

	cam, err := webcam.Open(device)
	if err != nil {
		log.Printf("[camera-controller] enrich %s: open %s: %v", cfg.ID, device, err)
		return
	}
	defer func() { _ = cam.Close() }()

	formats := cam.GetSupportedFormats()
	if len(formats) == 0 {
		return
	}

	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}

	// Pick a default format: prefer MJPEG (matches the v4l2 driver default),
	// then H264, then the first by ascending FOURCC for determinism.
	code, _ := pickDefaultFormat(formats)
	cfg.Params["pixel_format"] = fourccName(uint32(code))

	// Record all supported formats (sorted, deduped by FOURCC).
	cfg.Params["supported_formats"] = joinFormatNames(formats)

	// Pick a default size: prefer 640x480, then 1280x720, else the largest
	// discrete size supported by the chosen format.
	w, h, resolutions := pickDefaultSize(cam, code)
	if w > 0 && h > 0 {
		cfg.Width = w
		cfg.Height = h
	}
	if resolutions != "" {
		cfg.Params["supported_resolutions"] = resolutions
	}

	// Pick a default fps for the chosen size: prefer 30, else the highest.
	fps, fpsList := pickDefaultFPS(cam, code, uint32(w), uint32(h))
	if fps > 0 {
		cfg.FPS = fps
	}
	if fpsList != "" {
		cfg.Params["supported_fps"] = fpsList
	}

	log.Printf("[camera-controller] enriched %s: %s, %dx%d@%d (%s)",
		cfg.ID, cfg.Params["pixel_format"], cfg.Width, cfg.Height, cfg.FPS, device)
}

// pickDefaultFormat selects the pixel format to use as the config default.
// Preference order: MJPEG, H264, then the first format by ascending FOURCC.
func pickDefaultFormat(formats map[webcam.PixelFormat]string) (webcam.PixelFormat, string) {
	const (
		mjpeg = webcam.PixelFormat(uint32('M') | uint32('J')<<8 | uint32('P')<<16 | uint32('G')<<24)
		h264  = webcam.PixelFormat(uint32('H') | uint32('2')<<8 | uint32('6')<<16 | uint32('4')<<24)
	)
	if c, ok := formats[mjpeg]; ok {
		return mjpeg, c
	}
	if c, ok := formats[h264]; ok {
		return h264, c
	}
	codes := make([]webcam.PixelFormat, 0, len(formats))
	for c := range formats {
		codes = append(codes, c)
	}
	sort.Slice(codes, func(i, j int) bool { return codes[i] < codes[j] })
	c := codes[0]
	return c, formats[c]
}

// joinFormatNames returns a sorted, comma-separated list of the FOURCC names
// for every supported pixel format.
func joinFormatNames(formats map[webcam.PixelFormat]string) string {
	names := make([]string, 0, len(formats))
	for c := range formats {
		names = append(names, fourccName(uint32(c)))
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// pickDefaultSize selects a default frame size for the given pixel format.
// Discrete sizes only (stepwise/continuous ranges are skipped). Preference:
// 640x480, then 1280x720, otherwise the largest by pixel count. Also returns
// a sorted, comma-separated list of all supported discrete resolutions.
func pickDefaultSize(cam *webcam.Webcam, code webcam.PixelFormat) (w, h int, resolutions string) {
	type dim struct{ width, height uint32 }
	var sizes []dim
	for _, s := range cam.GetSupportedFrameSizes(code) {
		// Discrete sizes have a zero step and matching min/max bounds.
		if s.StepWidth != 0 || s.StepHeight != 0 {
			continue
		}
		if s.MinWidth != s.MaxWidth || s.MinHeight != s.MaxHeight {
			continue
		}
		if s.MaxWidth == 0 || s.MaxHeight == 0 {
			continue
		}
		sizes = append(sizes, dim{s.MaxWidth, s.MaxHeight})
	}
	if len(sizes) == 0 {
		return 0, 0, ""
	}
	// Sort by pixel count ascending so the list is deterministic.
	sort.Slice(sizes, func(i, j int) bool {
		return sizes[i].width*sizes[i].height < sizes[j].width*sizes[j].height
	})

	parts := make([]string, len(sizes))
	for i, d := range sizes {
		parts[i] = fmt.Sprintf("%dx%d", d.width, d.height)
	}
	resolutions = strings.Join(parts, ",")

	for _, want := range []dim{{640, 480}, {1280, 720}} {
		for _, d := range sizes {
			if d.width == want.width && d.height == want.height {
				return int(d.width), int(d.height), resolutions
			}
		}
	}
	largest := sizes[len(sizes)-1]
	return int(largest.width), int(largest.height), resolutions
}

// pickDefaultFPS selects a default framerate for the given format and size.
// Discrete intervals only. A V4L2 frame interval is numerator/denominator
// seconds per frame, so fps = denominator/numerator. Preference: 30, else the
// highest available. Also returns a sorted, deduped, comma-separated list.
func pickDefaultFPS(cam *webcam.Webcam, code webcam.PixelFormat, w, h uint32) (fps int, list string) {
	if w == 0 || h == 0 {
		return 0, ""
	}
	seen := map[int]struct{}{}
	var rates []int
	for _, r := range cam.GetSupportedFramerates(code, w, h) {
		if r.StepNumerator != 0 || r.StepDenominator != 0 {
			continue // skip stepwise/continuous ranges
		}
		if r.MinNumerator == 0 || r.MinDenominator == 0 {
			continue
		}
		f := int(r.MinDenominator) / int(r.MinNumerator)
		if f <= 0 {
			continue
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		rates = append(rates, f)
	}
	if len(rates) == 0 {
		return 0, ""
	}
	sort.Ints(rates)

	parts := make([]string, len(rates))
	for i, f := range rates {
		parts[i] = strconv.Itoa(f)
	}
	list = strings.Join(parts, ",")

	for _, f := range rates {
		if f == 30 {
			return 30, list
		}
	}
	return rates[len(rates)-1], list
}
