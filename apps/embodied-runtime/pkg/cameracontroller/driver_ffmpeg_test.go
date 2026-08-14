package cameracontroller

import (
	"bytes"
	"testing"
)

// TestFFmpegBitstreamReader_IndependentCopies feeds a crafted Annex B stream
// to the ffmpeg bitstream reader and verifies each emitted frame is an intact,
// independently-owned NAL unit. Before the copy fix, every frame's Data
// aliased the shared nalBuf backing and was overwritten by the following NAL,
// corrupting the H.264 bitstream ("non-existing PPS", "no frame!").
func TestFFmpegBitstreamReader_IndependentCopies(t *testing.T) {
	// SPS + PPS + IDR + P-slice, then a trailing start code that flushes the
	// final NAL out of the reader's accumulator.
	stream := concat(sps4, pps4, idr4, ps4, sc4)

	r := newBitstreamReader(nil, bytes.NewReader(stream), 640, 480, "h264", nil)

	var got []*Frame
	for f := range r.Frames() {
		got = append(got, f)
	}

	want := [][]byte{sps4, pps4, idr4, ps4}
	if len(got) != len(want) {
		t.Fatalf("got %d frames, want %d: %v", len(got), len(want), got)
	}
	for i, wantNAL := range want {
		if !bytes.Equal(got[i].Data, wantNAL) {
			t.Errorf("frame %d = % x\n      want % x", i, got[i].Data, wantNAL)
		}
	}
}

// soi = JPEG Start Of Image marker (0xFFD8).
// eoi = JPEG End Of Image marker (0xFFD9).
var (
	soi = []byte{0xFF, 0xD8}
	eoi = []byte{0xFF, 0xD9}
)

// jpegFrame builds a synthetic JPEG frame: SOI + payload + EOI.
func jpegFrame(payload []byte) []byte {
	return concat(soi, payload, eoi)
}

// TestSplitJPEG verifies the bufio.SplitFunc that splits an ffmpeg JPEG pipe
// stream on SOI/EOI markers into individual frames.
func TestSplitJPEG(t *testing.T) {
	t.Run("complete frame", func(t *testing.T) {
		f := jpegFrame([]byte{1, 2, 3})
		adv, tok, err := splitJPEG(f, true)
		if err != nil || adv != len(f) || !bytes.Equal(tok, f) {
			t.Errorf("adv=%d tok=% x err=%v", adv, tok, err)
		}
	})
	t.Run("no SOI at EOF discards all", func(t *testing.T) {
		data := []byte{1, 2, 3}
		adv, tok, err := splitJPEG(data, true)
		if err != nil || adv != len(data) || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("no SOI requests more", func(t *testing.T) {
		adv, tok, err := splitJPEG([]byte{1, 2, 3}, false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("SOI without EOI at EOF returns partial", func(t *testing.T) {
		data := concat(soi, []byte{0xAA})
		adv, tok, err := splitJPEG(data, true)
		if err != nil || adv != len(data) || !bytes.Equal(tok, data) {
			t.Errorf("adv=%d tok=% x err=%v", adv, tok, err)
		}
	})
	t.Run("SOI without EOI requests more", func(t *testing.T) {
		data := concat(soi, []byte{0xAA})
		adv, tok, err := splitJPEG(data, false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("leading garbage before SOI", func(t *testing.T) {
		f := jpegFrame([]byte{0xBB})
		data := concat([]byte{0x00, 0x00}, f)
		adv, tok, err := splitJPEG(data, true)
		if err != nil || adv != len(data) || !bytes.Equal(tok, f) {
			t.Errorf("adv=%d tok=% x err=%v", adv, tok, err)
		}
	})
	t.Run("empty at EOF", func(t *testing.T) {
		adv, tok, err := splitJPEG(nil, true)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
}

// TestFFmpegJPEGReader splits a stream of two JPEG frames and verifies both
// are emitted intact with the correct metadata.
func TestFFmpegJPEGReader(t *testing.T) {
	f1 := jpegFrame([]byte{0xAA, 0xBB})
	f2 := jpegFrame([]byte{0xCC, 0xDD, 0xEE})
	stream := concat(f1, f2)

	r := newImageReader(nil, bytes.NewReader(stream), 640, 480, nil, "jpeg", splitJPEG)

	var got []*Frame
	for frame := range r.Frames() {
		got = append(got, frame)
	}
	if len(got) != 2 {
		t.Fatalf("got %d frames, want 2", len(got))
	}
	if !bytes.Equal(got[0].Data, f1) {
		t.Errorf("frame 0 = % x, want % x", got[0].Data, f1)
	}
	if !bytes.Equal(got[1].Data, f2) {
		t.Errorf("frame 1 = % x, want % x", got[1].Data, f2)
	}
	for i, f := range got {
		if f.Width != 640 || f.Height != 480 {
			t.Errorf("frame %d dims = %dx%d, want 640x480", i, f.Width, f.Height)
		}
		if f.Encoding != "jpeg" {
			t.Errorf("frame %d encoding = %q, want jpeg", i, f.Encoding)
		}
		if f.Sequence != uint64(i+1) {
			t.Errorf("frame %d seq = %d, want %d", i, f.Sequence, i+1)
		}
	}
}
