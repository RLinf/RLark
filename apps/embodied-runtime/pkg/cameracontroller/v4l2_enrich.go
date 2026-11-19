package cameracontroller

import "strings"

// fourccString decodes a V4L2 pixel-format FOURCC (packed little-endian as a
// uint32) into its 4-character representation, e.g. 0x47504A4D -> "MJPG".
// This is the inverse of the kernel's V4L2_PIX_FMT_* macros.
func fourccString(code uint32) string {
	return string([]byte{
		byte(code),
		byte(code >> 8),
		byte(code >> 16),
		byte(code >> 24),
	})
}

// fourccName maps a V4L2 pixel-format FOURCC to the short name used in
// CameraConfig.Params["pixel_format"], which the v4l2 driver reads to select
// the capture format. Known formats get a canonical name; unknown formats use
// the lowercased FOURCC.
func fourccName(code uint32) string {
	switch fourccString(code) {
	case "MJPG":
		return "mjpeg"
	case "H264":
		return "h264"
	default:
		return strings.ToLower(fourccString(code))
	}
}
