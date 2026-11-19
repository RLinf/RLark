package jumper

import (
	"context"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// inputMux wraps a single io.Reader so that reads can be handed off between
// sequential phases (menu → session → prompt → menu) without competing
// goroutines.
//
// A single background goroutine reads from the underlying source and dispatches
// data into a buffered channel. Each phase obtains a fresh *muxReader via
// reader(); closing the previous reader before creating a new one ensures
// that only one consumer is active at a time, so no phase steals input from
// another (e.g. a leaked bubbletea input goroutine after the menu exits).
//
// For *os.File sources the background goroutine uses select() to poll the fd
// (no goroutine leak on context cancellation); for other readers (e.g. an SSH
// channel) it uses a blocking read that is unblocked when the source is
// closed.
type inputMux struct {
	ch  chan []byte
	raw io.Reader
}

func newInputMux(ctx context.Context, stdin io.Reader) *inputMux {
	im := &inputMux{
		ch:  make(chan []byte, 64),
		raw: stdin,
	}
	if stdin == nil {
		close(im.ch)
		return im
	}
	go im.loop(ctx, stdin)
	return im
}

func (im *inputMux) loop(ctx context.Context, stdin io.Reader) {
	buf := make([]byte, 256)
	for {
		n, err := readInput(ctx, stdin, buf)
		if n > 0 {
			data := make([]byte, n)
			copy(data, buf[:n])
			select {
			case im.ch <- data:
			case <-ctx.Done():
				return
			}
		}
		if err != nil {
			close(im.ch)
			return
		}
	}
}

// reader returns a fresh io.Reader backed by the multiplexer. Only one reader
// should be active at a time; close the previous reader before obtaining a
// new one.
func (im *inputMux) reader() *muxReader {
	return &muxReader{im: im, closed: make(chan struct{})}
}

// rawFile returns the underlying *os.File, or nil if the source is not a file.
// Used by the session phase to set raw mode on a real pty slave fd.
func (im *inputMux) rawFile() *os.File {
	f, _ := im.raw.(*os.File)
	return f
}

type muxReader struct {
	im     *inputMux
	closed chan struct{}
	once   sync.Once
	buf    []byte
}

// Read is an exported method.
func (r *muxReader) Read(p []byte) (int, error) {
	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}
	select {
	case data, ok := <-r.im.ch:
		if !ok {
			return 0, io.EOF
		}
		r.buf = data
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	case <-r.closed:
		return 0, io.EOF
	}
}

// Close unblocks any pending Read on this reader so that the next phase can
// take over input. Safe to call multiple times.
func (r *muxReader) Close() {
	r.once.Do(func() { close(r.closed) })
}

// drain discards any data currently buffered in the multiplexer's channel so
// that a subsequent Read waits for a fresh keypress instead of consuming
// leftover session input.
func (r *muxReader) drain() {
	for {
		select {
		case _, ok := <-r.im.ch:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// rawFile delegates to the multiplexer to expose the underlying *os.File.
func (r *muxReader) rawFile() *os.File { return r.im.rawFile() }

// readInput reads from stdin, blocking until data is available or ctx is
// cancelled. For *os.File it uses select() on the fd (no goroutine leak); for
// other readers it spawns a goroutine for the blocking read (the goroutine is
// unblocked when the underlying reader is closed, e.g. SSH channel drop).
func readInput(ctx context.Context, stdin io.Reader, buf []byte) (int, error) {
	if f, ok := stdin.(*os.File); ok {
		return readFd(ctx, f, buf)
	}
	type result struct {
		n   int
		err error
	}
	ch := make(chan result, 1)
	go func() {
		n, err := stdin.Read(buf)
		ch <- result{n, err}
	}()
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case res := <-ch:
		return res.n, res.err
	}
}

// readFd reads from f using select() with a short timeout so the read is
// context-cancellable without spawning a goroutine that would compete with
// other readers on the same fd.
func readFd(ctx context.Context, f *os.File, buf []byte) (int, error) {
	fd := int(f.Fd())
	for {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		var rset unix.FdSet
		rset.Zero()
		rset.Set(fd)
		tv := unix.NsecToTimeval(100 * int64(time.Millisecond))
		n, err := unix.Select(fd+1, &rset, nil, nil, &tv)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		rn, rerr := f.Read(buf)
		if rn > 0 {
			return rn, nil
		}
		if rerr != nil {
			return 0, rerr
		}
	}
}
