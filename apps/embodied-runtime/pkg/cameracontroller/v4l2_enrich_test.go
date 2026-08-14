package cameracontroller

import "testing"

// TestFourccString verifies V4L2 FOURCC decoding (little-endian packed uint32).
func TestFourccString(t *testing.T) {
	cases := map[uint32]string{
		uint32('M') | uint32('J')<<8 | uint32('P')<<16 | uint32('G')<<24: "MJPG",
		uint32('H') | uint32('2')<<8 | uint32('6')<<16 | uint32('4')<<24: "H264",
		uint32('Y') | uint32('U')<<8 | uint32('Y')<<16 | uint32('V')<<24: "YUYV",
	}
	for code, want := range cases {
		if got := fourccString(code); got != want {
			t.Errorf("fourccString(0x%X) = %q, want %q", code, got, want)
		}
	}
}

// TestFourccName verifies the FOURCC -> driver param name mapping.
func TestFourccName(t *testing.T) {
	cases := map[uint32]string{
		uint32('M') | uint32('J')<<8 | uint32('P')<<16 | uint32('G')<<24: "mjpeg",
		uint32('H') | uint32('2')<<8 | uint32('6')<<16 | uint32('4')<<24: "h264",
		uint32('Y') | uint32('U')<<8 | uint32('Y')<<16 | uint32('V')<<24: "yuyv",
	}
	for code, want := range cases {
		if got := fourccName(code); got != want {
			t.Errorf("fourccName(0x%X) = %q, want %q", code, got, want)
		}
	}
}
