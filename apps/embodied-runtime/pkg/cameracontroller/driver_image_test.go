package cameracontroller

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// ---------------------------------------------------------------------------
// Shared synthetic frame builders for the still-image splitters
// ---------------------------------------------------------------------------

var (
	pngSig = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	// IEND chunk: zero-length + "IEND" + its fixed CRC (0xAE426082).
	pngIEND = []byte{0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}
)

// pngFrame builds a minimal synthetic PNG frame: signature + payload + IEND.
func pngFrame(payload []byte) []byte {
	return concat(pngSig, payload, pngIEND)
}

// bmpFrame builds a synthetic BMP frame with a full 14-byte BITMAPFILEHEADER
// whose size field at offset 2-5 (little-endian uint32) carries the real total
// size. The remaining header fields (reserved, pixel-data offset) are zeroed.
func bmpFrame(payload []byte) []byte {
	hdr := make([]byte, 14) // BITMAPFILEHEADER
	hdr[0] = 0x42           // 'B'
	hdr[1] = 0x4D           // 'M'
	binary.LittleEndian.PutUint32(hdr[2:6], uint32(14+len(payload)))
	return concat(hdr, payload)
}

// tiffLEFrame builds a minimal little-endian TIFF: 8-byte header pointing at
// an IFD (at the given offset) followed by that IFD with nEntries empty-style
// entries and a next-IFD offset of nextOff. The returned slice is exactly the
// computed file size, so splitTIFF should consume it whole.
func tiffLEFrame(nEntries int, ifdOff, nextOff int) []byte {
	hdr := make([]byte, 8)
	hdr[0] = 0x49 // 'I'
	hdr[1] = 0x49 // 'I'
	binary.LittleEndian.PutUint16(hdr[2:4], 42)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(ifdOff))

	ifd := make([]byte, 2+nEntries*12+4)
	binary.LittleEndian.PutUint16(ifd[0:2], uint16(nEntries))
	binary.LittleEndian.PutUint32(ifd[2+nEntries*12:], uint32(nextOff))

	// Pad between header-end (8) and IFD if the IFD does not immediately follow.
	var gap []byte
	if pad := ifdOff - 8; pad > 0 {
		gap = make([]byte, pad)
	}
	return concat(hdr, gap, ifd)
}

// tiffLEFrameWithExtEntry builds a little-endian TIFF with one IFD entry that
// stores its value out-of-line (count*sizeof(type) > 4 bytes). The entry
// points to an offset right after the IFD; dataLen bytes of payload follow.
// The total file size is 8 (header) + 2+1*12+4 (IFD) + dataLen.
func tiffLEFrameWithExtEntry(typ, count, dataLen int) []byte {
	const ifdOff = 8
	hdr := make([]byte, 8)
	hdr[0], hdr[1] = 0x49, 0x49
	binary.LittleEndian.PutUint16(hdr[2:4], 42)
	binary.LittleEndian.PutUint32(hdr[4:8], uint32(ifdOff))

	ifd := make([]byte, 2+1*12+4)
	binary.LittleEndian.PutUint16(ifd[0:2], 1) // one entry
	// entry: tag(2), type(2), count(4), value/offset(4)
	binary.LittleEndian.PutUint16(ifd[2:4], 0x100) // tag (arbitrary)
	binary.LittleEndian.PutUint16(ifd[4:6], uint16(typ))
	binary.LittleEndian.PutUint32(ifd[6:10], uint32(count))
	dataOff := ifdOff + 2 + 1*12 + 4 // right after the IFD
	binary.LittleEndian.PutUint32(ifd[10:14], uint32(dataOff))
	binary.LittleEndian.PutUint32(ifd[14:18], 0) // next IFD = 0

	payload := bytes.Repeat([]byte{0xAB}, dataLen)
	return concat(hdr, ifd, payload)
}

// ---------------------------------------------------------------------------
// splitPNG
// ---------------------------------------------------------------------------

func TestSplitPNG(t *testing.T) {
	t.Run("complete frame", func(t *testing.T) {
		f := pngFrame([]byte{1, 2, 3})
		adv, tok, err := splitPNG(f, true)
		if err != nil || adv != len(f) || !bytes.Equal(tok, f) {
			t.Errorf("adv=%d tok=% x err=%v", adv, tok, err)
		}
	})
	t.Run("no sig at EOF discards all", func(t *testing.T) {
		data := []byte{1, 2, 3}
		adv, tok, err := splitPNG(data, true)
		if err != nil || adv != len(data) || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("no sig requests more", func(t *testing.T) {
		adv, tok, err := splitPNG([]byte{1, 2, 3}, false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("sig without IEND at EOF returns partial", func(t *testing.T) {
		data := concat(pngSig, []byte{0xAA})
		adv, tok, err := splitPNG(data, true)
		if err != nil || adv != len(data) || !bytes.Equal(tok, data) {
			t.Errorf("adv=%d tok=% x err=%v", adv, tok, err)
		}
	})
	t.Run("sig without IEND requests more", func(t *testing.T) {
		data := concat(pngSig, []byte{0xAA})
		adv, tok, err := splitPNG(data, false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("leading garbage before sig", func(t *testing.T) {
		f := pngFrame([]byte{0xBB})
		data := concat([]byte{0x00, 0x00}, f)
		adv, tok, err := splitPNG(data, true)
		if err != nil || adv != len(data) || !bytes.Equal(tok, f) {
			t.Errorf("adv=%d tok=% x err=%v", adv, tok, err)
		}
	})
	t.Run("partial sig requests more", func(t *testing.T) {
		// First 4 bytes of the signature only.
		adv, tok, err := splitPNG(pngSig[:4], false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("empty at EOF", func(t *testing.T) {
		adv, tok, err := splitPNG(nil, true)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
}

// ---------------------------------------------------------------------------
// splitBMP
// ---------------------------------------------------------------------------

func TestSplitBMP(t *testing.T) {
	t.Run("complete frame", func(t *testing.T) {
		f := bmpFrame([]byte{1, 2, 3})
		adv, tok, err := splitBMP(f, true)
		if err != nil || adv != len(f) || !bytes.Equal(tok, f) {
			t.Errorf("adv=%d tok=% x err=%v", adv, tok, err)
		}
	})
	t.Run("no magic at EOF discards all", func(t *testing.T) {
		data := []byte{1, 2, 3}
		adv, tok, err := splitBMP(data, true)
		if err != nil || adv != len(data) || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("no magic requests more", func(t *testing.T) {
		adv, tok, err := splitBMP([]byte{1, 2, 3}, false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("magic without full size field requests more", func(t *testing.T) {
		// "BM" + 3 bytes of the size field — not enough to read uint32.
		data := []byte{0x42, 0x4D, 0x01, 0x02, 0x03}
		adv, tok, err := splitBMP(data, false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("size larger than available requests more", func(t *testing.T) {
		f := bmpFrame([]byte{1, 2, 3}) // size = 9
		// Truncate the payload by one byte.
		adv, tok, err := splitBMP(f[:len(f)-1], false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("invalid size skips magic", func(t *testing.T) {
		// "BM" + size field claiming 5 bytes (< 14-byte header) — invalid.
		data := []byte{0x42, 0x4D, 0x05, 0x00, 0x00, 0x00, 0x99}
		adv, tok, err := splitBMP(data, true)
		if err != nil || adv != 2 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("leading garbage before magic", func(t *testing.T) {
		f := bmpFrame([]byte{0xBB})
		data := concat([]byte{0x00, 0x00}, f)
		adv, tok, err := splitBMP(data, true)
		if err != nil || adv != len(data) || !bytes.Equal(tok, f) {
			t.Errorf("adv=%d tok=% x err=%v", adv, tok, err)
		}
	})
	t.Run("empty at EOF", func(t *testing.T) {
		adv, tok, err := splitBMP(nil, true)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
}

// ---------------------------------------------------------------------------
// splitTIFF / tiffFileSize
// ---------------------------------------------------------------------------

func TestSplitTIFF_LE(t *testing.T) {
	t.Run("complete minimal frame", func(t *testing.T) {
		// IFD immediately follows the 8-byte header; 0 entries; next IFD = 0.
		f := tiffLEFrame(0, 8, 0) // size = 8 + 2 + 4 = 14
		adv, tok, err := splitTIFF(f, true)
		if err != nil || adv != len(f) || !bytes.Equal(tok, f) {
			t.Errorf("adv=%d tok=% x err=%v", adv, tok, err)
		}
	})
	t.Run("frame with gap before IFD", func(t *testing.T) {
		// Place the IFD 4 bytes after the header end (offset 12).
		f := tiffLEFrame(0, 12, 0) // size = max(8, 12+6) = 18
		adv, tok, err := splitTIFF(f, true)
		if err != nil || adv != len(f) || !bytes.Equal(tok, f) {
			t.Errorf("adv=%d tok=% x err=%v (len=%d)", adv, tok, err, len(f))
		}
	})
	t.Run("frame with out-of-line entry", func(t *testing.T) {
		// One LONG entry (type 4), count 2 → 8 bytes stored out-of-line.
		f := tiffLEFrameWithExtEntry(4, 2, 8) // size = 8 + 18 + 8 = 34
		adv, tok, err := splitTIFF(f, true)
		if err != nil || adv != len(f) || !bytes.Equal(tok, f) {
			t.Errorf("adv=%d tok=% x err=%v (len=%d)", adv, tok, err, len(f))
		}
	})
	t.Run("no magic at EOF discards all", func(t *testing.T) {
		adv, tok, err := splitTIFF([]byte{1, 2, 3}, true)
		if err != nil || adv != 3 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("no magic requests more", func(t *testing.T) {
		adv, tok, err := splitTIFF([]byte{1, 2, 3}, false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
	t.Run("partial header requests more", func(t *testing.T) {
		// First 4 bytes of the LE magic only.
		adv, tok, err := splitTIFF([]byte{0x49, 0x49, 0x2A, 0x00}, false)
		if err != nil || adv != 0 || tok != nil {
			t.Errorf("adv=%d tok=%v err=%v", adv, tok, err)
		}
	})
}

func TestSplitTIFF_BE(t *testing.T) {
	// Build a minimal big-endian TIFF by hand: "MM" + 0x002A + IFD@8 + 0 entries + next=0.
	hdr := []byte{0x4D, 0x4D, 0x00, 0x2A, 0x00, 0x00, 0x00, 0x08}
	ifd := []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00} // count=0, next=0
	f := concat(hdr, ifd)                             // 14 bytes
	adv, tok, err := splitTIFF(f, true)
	if err != nil || adv != len(f) || !bytes.Equal(tok, f) {
		t.Errorf("BE: adv=%d tok=% x err=%v", adv, tok, err)
	}
}

func TestTiffFileSize_CycleDetection(t *testing.T) {
	// IFD at offset 8 points to itself (next IFD = 8) → cycle.
	data := []byte{
		0x49, 0x49, 0x2A, 0x00, // "II" + 42 LE
		0x08, 0x00, 0x00, 0x00, // first IFD at offset 8
		0x00, 0x00, // count = 0 entries
		0x08, 0x00, 0x00, 0x00, // next IFD = 8 (cycle!)
	}
	size, ok := tiffFileSize(data, binary.LittleEndian.Uint16, binary.LittleEndian.Uint32)
	if ok || size != 0 {
		t.Errorf("expected cycle to be rejected, got size=%d ok=%v", size, ok)
	}
}

func TestTiffFileSize_Truncated(t *testing.T) {
	// Header claims the IFD is at offset 100 but the data is only 14 bytes.
	hdr := []byte{0x49, 0x49, 0x2A, 0x00, 0x64, 0x00, 0x00, 0x00}
	data := concat(hdr, make([]byte, 6))
	size, ok := tiffFileSize(data, binary.LittleEndian.Uint16, binary.LittleEndian.Uint32)
	if ok || size != 0 {
		t.Errorf("expected truncation to be rejected, got size=%d ok=%v", size, ok)
	}
}

// ---------------------------------------------------------------------------
// ffmpegImageReader end-to-end (PNG/BMP/TIFF)
// ---------------------------------------------------------------------------

func TestFFmpegImageReader_PNG(t *testing.T) {
	f1 := pngFrame([]byte{0xAA, 0xBB})
	f2 := pngFrame([]byte{0xCC, 0xDD, 0xEE})
	stream := concat(f1, f2)

	r := newImageReader(nil, bytes.NewReader(stream), 640, 480, nil, "png", splitPNG)

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
		if f.Encoding != "png" {
			t.Errorf("frame %d encoding = %q, want png", i, f.Encoding)
		}
		if f.Sequence != uint64(i+1) {
			t.Errorf("frame %d seq = %d, want %d", i, f.Sequence, i+1)
		}
	}
}

func TestFFmpegImageReader_BMP(t *testing.T) {
	f1 := bmpFrame([]byte{0xAA, 0xBB})
	f2 := bmpFrame([]byte{0xCC})
	stream := concat(f1, f2)

	r := newImageReader(nil, bytes.NewReader(stream), 320, 240, nil, "bmp", splitBMP)

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
		if f.Encoding != "bmp" {
			t.Errorf("frame %d encoding = %q, want bmp", i, f.Encoding)
		}
	}
}

func TestFFmpegImageReader_TIFF(t *testing.T) {
	f1 := tiffLEFrame(0, 8, 0)
	f2 := tiffLEFrameWithExtEntry(4, 2, 8)
	stream := concat(f1, f2)

	r := newImageReader(nil, bytes.NewReader(stream), 640, 480, nil, "tiff", splitTIFF)

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
		if f.Encoding != "tiff" {
			t.Errorf("frame %d encoding = %q, want tiff", i, f.Encoding)
		}
	}
}
