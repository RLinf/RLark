package cameracontroller

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// ---------------------------------------------------------------------------
// FFmpeg-based camera driver
// ---------------------------------------------------------------------------

// ffmpegDriver implements CameraDriver using an ffmpeg subprocess.
// It reads frames from ffmpeg's stdout and pushes them into a channel.
// The output format depends on the encoding hint — JPEG, H264, raw video, etc.
type ffmpegDriver struct {
	buildArgs func(CameraConfig, int, int, int, string) []string

	mu   sync.Mutex
	cmds []*exec.Cmd
}

// newFFmpegDriver creates a new ffmpeg-based driver with the given argument
// builder function.
func newFFmpegDriver(buildArgs func(CameraConfig, int, int, int, string) []string) *ffmpegDriver {
	return &ffmpegDriver{buildArgs: buildArgs}
}

// Open starts the ffmpeg subprocess and returns a FrameReader.
func (d *ffmpegDriver) Open(ctx context.Context, cfg CameraConfig, width, height, fps int, encodingHint string) (FrameReader, string, error) {
	args := d.buildArgs(cfg, width, height, fps, encodingHint)
	log.Printf("[camera-controller] starting ffmpeg: %v", args)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("create stdout pipe: %w", err)
	}

	// Capture ffmpeg's stderr so the failure reason (e.g. "No such device",
	// "Could not open input") is available when the process exits non-zero.
	// A bounded tail buffer keeps memory stable during long-running
	// successful captures — ffmpeg emits per-frame progress to stderr.
	stderr := newTailWriter(8 * 1024)
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("start ffmpeg: %w", err)
	}

	d.mu.Lock()
	d.cmds = append(d.cmds, cmd)
	d.mu.Unlock()

	w, h, _ := resolveResolution(cfg, width, height, fps)

	// Choose the right reader based on encoding.
	enc := encodingHint
	if enc == "" {
		enc = "jpeg"
	}

	var r FrameReader
	switch enc {
	case "jpeg":
		r = newImageReader(cmd, stdout, w, h, stderr, "jpeg", splitJPEG)
	case "png":
		r = newImageReader(cmd, stdout, w, h, stderr, "png", splitPNG)
	case "bmp":
		r = newImageReader(cmd, stdout, w, h, stderr, "bmp", splitBMP)
	case "tiff":
		r = newImageReader(cmd, stdout, w, h, stderr, "tiff", splitTIFF)
	case "h264", "h265":
		r = newBitstreamReader(cmd, stdout, w, h, enc, stderr)
	default:
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, "", fmt.Errorf("unsupported encoding: %q (use jpeg, png, bmp, tiff, h264, or h265)", enc)
	}

	return r, enc, nil
}

// Close is a safety net: it kills any ffmpeg subprocess this driver started
// that the reader did not already reap. The reader owns the cmd's lifecycle
// (Kill+Wait); this only Kill's (best-effort, no Wait) so it never races with
// the reader's Wait.
func (d *ffmpegDriver) Close() error {
	d.mu.Lock()
	cmds := d.cmds
	d.cmds = nil
	d.mu.Unlock()
	for _, cmd := range cmds {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// ffmpeg subprocess lifecycle (shared by the JPEG and bitstream readers)
// ---------------------------------------------------------------------------

// tailWriter is an io.Writer that retains only the last N bytes written to
// it. ffmpeg's stderr is piped through it so the tail — which carries the
// failure reason — can be surfaced when the process exits non-zero, without
// unbounded growth during long-running successful captures (ffmpeg emits
// per-frame progress to stderr by default).
type tailWriter struct {
	mu  sync.Mutex
	buf []byte
	cap int
}

func newTailWriter(n int) *tailWriter {
	return &tailWriter{cap: n}
}

func (t *tailWriter) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.cap {
		t.buf = t.buf[len(t.buf)-t.cap:]
	}
	return len(p), nil
}

func (t *tailWriter) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// ffmpegState holds the ffmpeg subprocess lifecycle shared by the JPEG and
// bitstream readers: the captured stderr tail, the "closed" flag that
// distinguishes an intentional Close()-initiated shutdown from a spontaneous
// ffmpeg failure, and the recorded exit error exposed via Err().
type ffmpegState struct {
	cmd    *exec.Cmd
	stderr *tailWriter
	closed atomic.Bool // set before Close()'s Kill → suppress Err
	errMu  sync.Mutex
	err    error
}

// Err returns the error that caused ffmpeg to stop (non-zero exit), or nil
// if it exited cleanly or was shut down via Close(). The controller's
// capture loop reads this when the frame stream ends to decide whether to
// mark the camera ERROR.
func (s *ffmpegState) Err() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	return s.err
}

// kill signals ffmpeg to stop and reaps the process. This is the Close()
// path: setting `closed` first makes reap() treat the exit as expected and
// leave Err() nil. Safe to call when cmd is nil (test paths).
func (s *ffmpegState) kill() {
	s.closed.Store(true)
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
		_ = s.cmd.Wait()
	}
}

// reap waits for the ffmpeg process and, on a non-zero exit, records an
// error (with the stderr tail folded in) for Err() to surface. When Close()
// initiated the shutdown (closed is set) the exit is expected and is left
// nil — Close() owns the Wait.
func (s *ffmpegState) reap(encoding string) {
	if s.cmd == nil {
		return
	}
	if s.closed.Load() {
		return // Close() owns reaping the process.
	}
	if err := s.cmd.Wait(); err != nil {
		tail := ""
		if s.stderr != nil {
			tail = s.stderr.String()
		}
		s.errMu.Lock()
		s.err = fmt.Errorf("ffmpeg %s exited: %w\n--- stderr ---\n%s", encoding, err, tail)
		s.errMu.Unlock()
	}
}

// ---------------------------------------------------------------------------
// ffmpegImageReader — frame-delimited image reader (JPEG/PNG/BMP/TIFF)
// ---------------------------------------------------------------------------

// ffmpegImageReader reads a stream of frame-delimited still images from
// ffmpeg's image2pipe stdout. A format-specific bufio.SplitFunc locates the
// frame boundaries (JPEG SOI/EOI markers, the PNG IEND chunk, the size field
// in the BMP file header, or the TIFF IFD chain). Each emitted Frame carries
// an independently-owned copy of the bytes; the consumer does not Release().
type ffmpegImageReader struct {
	*ffmpegState

	splitFunc bufio.SplitFunc
	encoding  string

	frames  chan *Frame
	done    chan struct{}
	once    sync.Once
	counter uint64
	width   int
	height  int
}

// newImageReader constructs a frame-delimited image reader that splits the
// ffmpeg stdout stream with the provided splitFunc and tags each frame with
// encoding.
func newImageReader(
	cmd *exec.Cmd,
	stdout io.Reader,
	width, height int,
	stderr *tailWriter,
	encoding string,
	splitFunc bufio.SplitFunc,
) *ffmpegImageReader {
	r := &ffmpegImageReader{
		ffmpegState: &ffmpegState{cmd: cmd, stderr: stderr},
		splitFunc:   splitFunc,
		encoding:    encoding,
		frames:      make(chan *Frame, 8),
		done:        make(chan struct{}),
		width:       width,
		height:      height,
	}
	go r.readLoop(stdout)
	return r
}

func (r *ffmpegImageReader) Frames() <-chan *Frame { return r.frames }

func (r *ffmpegImageReader) Close() error {
	r.once.Do(r.kill)
	<-r.done
	return nil
}

func (r *ffmpegImageReader) readLoop(src io.Reader) {
	defer close(r.done)
	defer close(r.frames)

	scanner := bufio.NewScanner(src)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	scanner.Split(r.splitFunc)

	for scanner.Scan() {
		data := scanner.Bytes()
		if len(data) < 4 {
			continue
		}

		frame := &Frame{
			Data:      append([]byte(nil), data...),
			Width:     r.width,
			Height:    r.height,
			Encoding:  r.encoding,
			Timestamp: time.Now(),
			Sequence:  atomic.AddUint64(&r.counter, 1),
		}

		select {
		case r.frames <- frame:
		default:
			select {
			case dropped := <-r.frames:
				dropped.Release()
			default:
			}
			r.frames <- frame
		}
	}

	r.reap(r.encoding)
}

// splitJPEG splits on JPEG SOI (0xFFD8) / EOI (0xFFD9) markers.
func splitJPEG(data []byte, atEOF bool) (advance int, token []byte, err error) {
	soi := bytes.Index(data, []byte{0xFF, 0xD8})
	if soi < 0 {
		if atEOF {
			return len(data), nil, nil
		}
		return 0, nil, nil
	}

	eoi := bytes.Index(data[soi+2:], []byte{0xFF, 0xD9})
	if eoi < 0 {
		if atEOF {
			return len(data), data[soi:], nil
		}
		return soi, nil, nil
	}

	eoi += soi + 2 + 2
	return eoi, data[soi:eoi], nil
}

// splitPNG splits on the PNG signature (8-byte) at the start and the IEND
// chunk (12-byte: zero-length + "IEND" + its fixed CRC) at the end. Every
// PNG file ends with exactly this chunk, so locating it yields an unambiguous
// frame boundary — no need to parse intermediate chunks.
func splitPNG(data []byte, atEOF bool) (advance int, token []byte, err error) {
	sig := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	start := bytes.Index(data, sig)
	if start < 0 {
		if atEOF {
			return len(data), nil, nil
		}
		return 0, nil, nil
	}

	// IEND chunk: length=0 (4 bytes), type="IEND" (4 bytes), no data, CRC=0xAE426082.
	iend := []byte{0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42, 0x60, 0x82}
	rel := bytes.Index(data[start+len(sig):], iend)
	if rel < 0 {
		if atEOF {
			return len(data), data[start:], nil
		}
		return start, nil, nil
	}

	end := start + len(sig) + rel + len(iend)
	return end, data[start:end], nil
}

// splitBMP splits on the "BM" magic and reads the file size from the 14-byte
// BITMAPFILEHEADER (offset 2-5, little-endian uint32). The size field lets us
// delimit frames without scanning for an end marker (BMP has none).
func splitBMP(data []byte, atEOF bool) (advance int, token []byte, err error) {
	magic := []byte{0x42, 0x4D} // "BM"
	start := bytes.Index(data, magic)
	if start < 0 {
		if atEOF {
			return len(data), nil, nil
		}
		return 0, nil, nil
	}

	// The file size field lives at offset 2-5 of the BMP file header; we need
	// at least 6 bytes from start to read it.
	if start+6 > len(data) {
		if atEOF {
			return len(data), nil, nil
		}
		return start, nil, nil
	}

	size := int(binary.LittleEndian.Uint32(data[start+2 : start+6]))
	if size < 14 {
		// Invalid header (smaller than the file header itself); skip past the
		// magic so we don't loop forever on a stray "BM" in the stream.
		return start + 2, nil, nil
	}

	end := start + size
	if end > len(data) {
		if atEOF {
			return len(data), nil, nil
		}
		return start, nil, nil
	}
	return end, data[start:end], nil
}

// splitTIFF splits on the TIFF magic ("II"\x2A\x00 for little-endian or
// "MM"\x00\x2A for big-endian) and walks the IFD chain to compute the file
// size. TIFF files have no explicit end marker, so the size is the maximum end
// offset of any IFD or out-of-line data block referenced by an IFD entry.
func splitTIFF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	leSig := []byte{0x49, 0x49, 0x2A, 0x00} // "II" + version 42 (little-endian)
	beSig := []byte{0x4D, 0x4D, 0x00, 0x2A} // "MM" + version 42 (big-endian)

	var start int
	var le bool
	if idx := bytes.Index(data, leSig); idx >= 0 {
		start, le = idx, true
	} else if idx := bytes.Index(data, beSig); idx >= 0 {
		start, le = idx, false
	} else {
		if atEOF {
			return len(data), nil, nil
		}
		return 0, nil, nil
	}

	// The first IFD offset lives at bytes 4-7 of the header.
	if start+8 > len(data) {
		if atEOF {
			return len(data), nil, nil
		}
		return start, nil, nil
	}

	var u16 func([]byte) uint16
	var u32 func([]byte) uint32
	if le {
		u16 = binary.LittleEndian.Uint16
		u32 = binary.LittleEndian.Uint32
	} else {
		u16 = binary.BigEndian.Uint16
		u32 = binary.BigEndian.Uint32
	}

	size, ok := tiffFileSize(data[start:], u16, u32)
	if !ok {
		if atEOF {
			return len(data), nil, nil
		}
		return start, nil, nil
	}

	end := start + size
	if end > len(data) {
		if atEOF {
			return len(data), nil, nil
		}
		return start, nil, nil
	}
	return end, data[start:end], nil
}

// tiffFileSize walks the IFD chain of a TIFF stream (starting at offset 0 in
// data) and returns the file size — the maximum end offset of any IFD or
// out-of-line data block. Returns ok=false if the stream is too short to
// parse or an IFD chain cycle is detected.
func tiffFileSize(data []byte, u16 func([]byte) uint16, u32 func([]byte) uint32) (int, bool) {
	if len(data) < 8 {
		return 0, false
	}

	// TIFF data type -> byte size. Index 0 is unused (types start at 1).
	typeSizes := [...]int{0, 1, 1, 2, 4, 8, 1, 1, 2, 4, 8, 4, 8}

	maxEnd := 8 // 8-byte header is the minimum.
	ifdOff := int(u32(data[4:8]))

	// Guard against cyclic IFD chains (malformed input) by capping the number
	// of IFDs we walk. Real TIFFs have at most a handful.
	const maxIFDs = 64
	visited := make(map[int]struct{}, maxIFDs)

	for ifdOff != 0 {
		if ifdOff < 0 || ifdOff+2 > len(data) {
			return 0, false
		}
		if _, seen := visited[ifdOff]; seen {
			return 0, false // cycle
		}
		if len(visited) >= maxIFDs {
			return 0, false
		}
		visited[ifdOff] = struct{}{}

		n := int(u16(data[ifdOff : ifdOff+2]))
		ifdEnd := ifdOff + 2 + n*12 + 4
		if ifdEnd > len(data) {
			return 0, false
		}
		if ifdEnd > maxEnd {
			maxEnd = ifdEnd
		}

		// Entries whose value does not fit in 4 bytes are stored out-of-line
		// at an offset; account for their end to size the file.
		for i := 0; i < n; i++ {
			entryOff := ifdOff + 2 + i*12
			typ := int(u16(data[entryOff+2 : entryOff+4]))
			count := int(u32(data[entryOff+4 : entryOff+8]))
			if typ < 1 || typ >= len(typeSizes) {
				continue
			}
			dataLen := count * typeSizes[typ]
			if dataLen <= 4 {
				continue // value fits inline; no out-of-line block.
			}
			off := int(u32(data[entryOff+8 : entryOff+12]))
			end := off + dataLen
			if end < off { // overflow guard
				return 0, false
			}
			if end > len(data) {
				return 0, false
			}
			if end > maxEnd {
				maxEnd = end
			}
		}

		// Next IFD offset (0 terminates the chain).
		ifdOff = int(u32(data[ifdOff+2+n*12 : ifdOff+2+n*12+4]))
	}

	return maxEnd, true
}

// ---------------------------------------------------------------------------
// ffmpegBitstreamReader — reads H264/H265 Annex B NAL units
// ---------------------------------------------------------------------------

type ffmpegBitstreamReader struct {
	*ffmpegState

	frames   chan *Frame
	done     chan struct{}
	once     sync.Once
	width    int
	height   int
	encoding string
}

func newBitstreamReader(cmd *exec.Cmd, stdout io.Reader, width, height int, encoding string, stderr *tailWriter) *ffmpegBitstreamReader {
	r := &ffmpegBitstreamReader{
		ffmpegState: &ffmpegState{cmd: cmd, stderr: stderr},
		frames:      make(chan *Frame, 8),
		done:        make(chan struct{}),
		width:       width,
		height:      height,
		encoding:    encoding,
	}
	go r.readLoop(stdout)
	return r
}

func (r *ffmpegBitstreamReader) Frames() <-chan *Frame { return r.frames }

func (r *ffmpegBitstreamReader) Close() error {
	r.once.Do(r.kill)
	<-r.done
	return nil
}

func (r *ffmpegBitstreamReader) readLoop(src io.Reader) {
	defer close(r.done)
	defer close(r.frames)

	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	nalBuf := make([]byte, 0, 64*1024)
	readBuf := make([]byte, 32*1024)
	bytePos := uint64(0)

	for {
		n, err := src.Read(readBuf)
		if err != nil {
			break
		}

		data := readBuf[:n]
		offset := 0

		for offset < len(data) {
			idx := bytes.Index(data[offset:], startCode)
			if idx < 0 {
				nalBuf = append(nalBuf, data[offset:]...)
				bytePos += uint64(len(data) - offset)
				break
			}

			if len(nalBuf) > 0 || offset < idx {
				// Independent copy: nalBuf is reused for the next NAL, so the
				// emitted frame must not alias it. Without this copy, every
				// frame's data is overwritten by the following NAL, corrupting
				// the H.264 bitstream ("non-existing PPS", "no frame!").
				chunk := append([]byte(nil), nalBuf...)
				chunk = append(chunk, data[offset:offset+idx]...)
				nalBuf = nalBuf[:0]

				if len(chunk) > 0 {
					frame := &Frame{
						Data:      chunk,
						Width:     r.width,
						Height:    r.height,
						Encoding:  r.encoding,
						Timestamp: time.Now(),
						Sequence:  bytePos,
					}

					select {
					case r.frames <- frame:
					default:
						select {
						case dropped := <-r.frames:
							_ = dropped
						default:
						}
						r.frames <- frame
					}
				}
			}

			offset += idx + 4
			bytePos += uint64(idx + 4)
			nalBuf = append(nalBuf[:0], startCode...)
		}
	}

	r.reap(r.encoding)
}
