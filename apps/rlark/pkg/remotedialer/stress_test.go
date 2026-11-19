package remotedialer

import (
	"context"
	"crypto/rand"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestFullLinkDuplexStress exercises the complete transport stack (HTTP-style
// dialer -> smux tunnel over websocket -> pipe -> backend TCP) under a
// full-duplex load with delayed reads. It is the regression guard for the
// concurrency-repro scenario that previously deadlocked with smux v1 and
// surfaced echo-backend self-deadlocks with a naive io.Copy(conn,conn).
//
// The test is skipped in -short mode because it moves ~256MiB of data.
func TestFullLinkDuplexStress(t *testing.T) {
	if testing.Short() {
		t.Skip("stress test; skipped in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	backend := startDuplexSafeEcho(t)
	defer func() { _ = backend.Close() }()

	auth := func(req *http.Request) (string, bool, error) { return "client", true, nil }
	server := New(auth, DefaultErrorWriter)
	tunnelAddr, err := newServer(ctx, server)
	if err != nil {
		t.Fatal(err)
	}
	if err := newTestClient(ctx, "ws://"+tunnelAddr); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return server.HasSession("client") }, "tunnel session never established")

	const (
		conns          = 8
		size           = 32 << 20
		chunk          = 64 << 10
		startReadDelay = 200 * time.Millisecond
		readDelay      = time.Millisecond
	)

	var wg sync.WaitGroup
	errCh := make(chan error, conns)

	for i := 0; i < conns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			conn, err := server.Dialer("client")(ctx, "tcp", backend.Addr().String())
			if err != nil {
				errCh <- err
				return
			}
			defer func() { _ = conn.Close() }()
			_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

			writeDone := make(chan error, 1)
			go func() {
				buf := make([]byte, chunk)
				_, _ = rand.Read(buf)
				var written int64
				for written < size {
					n := int64(len(buf))
					if size-written < n {
						n = size - written
					}
					m, err := conn.Write(buf[:n])
					written += int64(m)
					if err != nil {
						writeDone <- err
						return
					}
				}
				writeDone <- nil
			}()

			time.Sleep(startReadDelay)
			buf := make([]byte, chunk)
			var read int64
			for read < size {
				n, err := conn.Read(buf)
				read += int64(n)
				if err != nil {
					errCh <- err
					return
				}
				time.Sleep(readDelay)
			}
			if err := <-writeDone; err != nil {
				errCh <- err
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for e := range errCh {
		if e != nil {
			t.Fatalf("stream failed: %v", e)
		}
	}
}

// startDuplexSafeEcho starts a TCP echo backend that decouples reads and
// writes with separate goroutines. A single io.Copy(conn, conn) can self
// deadlock under a full-duplex load once both send buffers are full, because
// the same goroutine handles both directions.
func startDuplexSafeEcho(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				pending := make(chan []byte, 1024)
				go func() {
					for b := range pending {
						if _, err := c.Write(b); err != nil {
							return
						}
					}
				}()
				buf := make([]byte, 64<<10)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						chunk := make([]byte, n)
						copy(chunk, buf[:n])
						pending <- chunk
					}
					if err != nil {
						close(pending)
						return
					}
				}
			}(c)
		}
	}()
	return ln
}
