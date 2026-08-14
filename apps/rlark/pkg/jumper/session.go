package jumper

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/term"
)

// runSession dials the target and runs a raw interactive terminal session on
// its own alternate screen page, isolated from the menu. It returns when the
// remote session ends or the context is cancelled. The caller closes the
// reader afterwards to guarantee no goroutine is left blocked on stdin.
func runSession(ctx context.Context, dialer TerminalDialer, target Target,
	reader *muxReader, stdout io.Writer, pump *windowPump) error {

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()

	terminal, err := dialer.Dial(sctx, target)
	if err != nil {
		return fmt.Errorf("dial %s: %w", target.Name, err)
	}

	// Enter a fresh alternate screen for the session, so it renders as its own
	// clean page rather than mixing with the menu.
	_, _ = stdout.Write([]byte("\x1b[?1049h"))
	_, _ = stdout.Write([]byte("\x1b[2J"))
	_, _ = stdout.Write([]byte("\x1b[?25h"))
	_, _ = fmt.Fprintf(stdout, "\r\n Connected to %s\r\n type 'exit' to end the session\r\n\r\n", target.Name)

	// Raw mode: forward all keys untouched. Only managed when the underlying
	// source is a real *os.File (local terminal or real pty slave); the SSH
	// emulated pty is already a raw byte stream.
	restore := func() {}
	if f := reader.rawFile(); f != nil {
		if old, e := term.MakeRaw(int(f.Fd())); e == nil {
			restore = func() { _ = term.Restore(int(f.Fd()), old) }
		}
	}
	defer restore()

	// Replay the last known size so the session starts at the correct size,
	// then keep forwarding resize events to the remote terminal.
	pump.replay()
	go func() {
		for {
			select {
			case <-sctx.Done():
				return
			case w, ok := <-pump.ch:
				if !ok {
					return
				}
				_ = terminal.Resize(uint16(w.Height), uint16(w.Width))
			}
		}
	}()

	// Remote output -> local stdout.
	remoteDone := make(chan struct{})
	go func() {
		defer close(remoteDone)
		_, _ = io.Copy(stdout, terminal)
	}()

	// Local stdin -> remote. The reader is a muxReader; closing it (done by
	// the caller after runSession returns) unblocks Read so no goroutine is
	// left stealing keys from the next phase.
	stdinDone := make(chan struct{})
	go func() {
		defer close(stdinDone)
		stdinCopy(terminal, reader)
	}()

	// Wait until the session ends: remote EOF or ctx done.
	select {
	case <-remoteDone:
	case <-sctx.Done():
	}

	// Stop the stdin copier (close the reader to unblock its Read), and close
	// the terminal to unblock the remote copier, then give both a bounded
	// chance to finish.
	reader.Close()
	_ = terminal.Close()
	select {
	case <-stdinDone:
	case <-time.After(200 * time.Millisecond):
	}
	select {
	case <-remoteDone:
	case <-time.After(2 * time.Second):
	}

	// Keep the session's alternate screen active so the reconnect prompt can
	// reuse it (the menu re-enters its own screen when returning to the list).
	_, _ = stdout.Write([]byte("\x1b[?25h"))
	return nil
}

// stdinCopy forwards stdin to the terminal until the reader returns an error
// (EOF when the caller closes the muxReader) or the terminal write fails.
func stdinCopy(terminal Terminal, stdin io.Reader) {
	buf := make([]byte, 256)
	for {
		n, err := stdin.Read(buf)
		if err != nil || n == 0 {
			return
		}
		if _, werr := terminal.Write(buf[:n]); werr != nil {
			return
		}
	}
}

// promptAfterSession shows a message on the session's alternate screen after a
// session ends and reads a single keypress. It reports whether the user wants
// to reconnect to the same target ('r'/'R'); any other key returns to the
// list. The caller is responsible for closing the reader (e.g. via a timeout
// goroutine) to unblock the Read if the user does not press a key.
func promptAfterSession(stdout io.Writer, stdin *muxReader, err error) bool {
	// Clear residual input first so the prompt truly waits for a new key.
	stdin.drain()

	_, _ = stdout.Write([]byte("\x1b[2J"))
	_, _ = stdout.Write([]byte("\x1b[?25h"))
	if err != nil {
		_, _ = fmt.Fprintf(stdout, "\r\n Connection failed: %v\r\n", err)
	} else {
		_, _ = fmt.Fprint(stdout, "\r\n Connection closed\r\n")
	}
	_, _ = fmt.Fprint(stdout, "\r\n [r] Reconnect   •   any other key back to list\r\n")

	buf := make([]byte, 1)
	n, _ := stdin.Read(buf)
	return n > 0 && (buf[0] == 'r' || buf[0] == 'R')
}
