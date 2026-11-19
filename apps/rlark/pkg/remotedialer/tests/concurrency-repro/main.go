// Run Test: go run ./apps/rlark/pkg/remotedialer/tests/concurrency-repro \
//   -connections 8 -size 33554432 -start-read-delay 200ms -read-delay 1ms -timeout 20s -stall-threshold 1s

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/remotedialer"
)

const clientID = "load-client"

type config struct {
	connections int
	size        int64
	chunkSize   int
	readDelay   time.Duration
	startRead   time.Duration
	stall       time.Duration
	timeout     time.Duration
}

type result struct {
	id       int
	written  int64
	read     int64
	first    time.Duration
	maxGap   time.Duration
	duration time.Duration
	err      error
}

func main() {
	var cfg config
	flag.IntVar(&cfg.connections, "connections", 8, "number of concurrent full-duplex connections")
	flag.Int64Var(&cfg.size, "size", 64<<20, "bytes sent and echoed per connection")
	flag.IntVar(&cfg.chunkSize, "chunk-size", 64<<10, "application read and write size")
	flag.DurationVar(&cfg.startRead, "start-read-delay", 500*time.Millisecond, "delay before reading echoed data")
	flag.DurationVar(&cfg.readDelay, "read-delay", time.Millisecond, "delay after each read")
	flag.DurationVar(&cfg.stall, "stall-threshold", 2*time.Second, "report read or write gaps above this duration")
	flag.DurationVar(&cfg.timeout, "timeout", 2*time.Minute, "timeout per connection")
	flag.Parse()

	if cfg.connections < 1 || cfg.size < 1 || cfg.chunkSize < 1 {
		log.Fatal("connections, size, and chunk-size must be positive")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	backendAddr, closeBackend := startEchoServer(ctx)
	defer closeBackend()
	tunnel := remotedialer.New(func(*http.Request) (string, bool, error) {
		return clientID, true, nil
	}, remotedialer.DefaultErrorWriter)
	tunnelAddr, closeTunnel := startHTTPServer(ctx, tunnel)
	defer closeTunnel()

	connected := make(chan struct{})
	clientErr := make(chan error, 1)
	go func() {
		clientErr <- remotedialer.ConnectToProxy(ctx, "ws://"+tunnelAddr, nil,
			func(string, string) bool { return true }, nil,
			func(context.Context, *remotedialer.Session) error {
				close(connected)
				return nil
			})
	}()
	select {
	case <-connected:
	case err := <-clientErr:
		log.Fatalf("connect tunnel client: %v", err)
	case <-time.After(10 * time.Second):
		log.Fatal("timed out waiting for tunnel client")
	}

	// onConnect fires on the client as soon as the websocket handshake
	// completes, which may be a hair before the server-side session is
	// registered. Wait for the tunnel server to actually have a session for us
	// before we start opening streams.
	sessionDeadline := time.Now().Add(5 * time.Second)
	for !tunnel.HasSession(clientID) {
		if time.Now().After(sessionDeadline) {
			log.Fatal("tunnel server never registered the client session")
		}
		time.Sleep(50 * time.Millisecond)
	}

	results := make(chan result, cfg.connections)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for id := 1; id <= cfg.connections; id++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			results <- runConnection(ctx, tunnel.Dialer(clientID), backendAddr, id, cfg)
		}(id)
	}
	log.Printf("starting connections=%d duplex-bytes=%d MiB start-read-delay=%s read-delay=%s", cfg.connections, cfg.size>>20, cfg.startRead, cfg.readDelay)
	close(start)
	wg.Wait()
	close(results)

	failed := false
	for r := range results {
		log.Printf("connection=%02d written=%d read=%d first-byte=%s max-gap=%s duration=%s err=%v", r.id, r.written, r.read, r.first.Round(time.Millisecond), r.maxGap.Round(time.Millisecond), r.duration.Round(time.Millisecond), r.err)
		if r.err != nil || r.written != cfg.size || r.read != cfg.size {
			failed = true
		}
		if r.maxGap >= cfg.stall {
			log.Printf("STALL connection=%02d gap=%s threshold=%s", r.id, r.maxGap.Round(time.Millisecond), cfg.stall)
		}
	}
	if failed {
		os.Exit(1)
	}
}

func runConnection(parent context.Context, dial remotedialer.Dialer, address string, id int, cfg config) result {
	started := time.Now()
	r := result{id: id}
	ctx, cancel := context.WithTimeout(parent, cfg.timeout)
	defer cancel()
	conn, err := dial(ctx, "tcp", address)
	if err != nil {
		r.err = err
		return r
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(cfg.timeout))

	writeDone := make(chan error, 1)
	go func() {
		buf := make([]byte, cfg.chunkSize)
		for r.written < cfg.size {
			n := min(int64(len(buf)), cfg.size-r.written)
			written, err := conn.Write(buf[:n])
			r.written += int64(written)
			if err != nil {
				writeDone <- fmt.Errorf("write after %d bytes: %w", r.written, err)
				return
			}
		}
		writeDone <- nil
	}()

	time.Sleep(cfg.startRead)
	buf := make([]byte, cfg.chunkSize)
	last := started
	for r.read < cfg.size {
		n, err := conn.Read(buf)
		now := time.Now()
		if n > 0 {
			if r.read == 0 {
				r.first = now.Sub(started)
			}
			if gap := now.Sub(last); gap > r.maxGap {
				r.maxGap = gap
			}
			last = now
			r.read += int64(n)
			if cfg.readDelay > 0 {
				time.Sleep(cfg.readDelay)
			}
		}
		if err != nil {
			if gap := now.Sub(last); gap > r.maxGap {
				r.maxGap = gap
			}
			r.err = fmt.Errorf("read after %d bytes: %w", r.read, err)
			break
		}
	}
	if err := <-writeDone; r.err == nil {
		r.err = err
	}
	r.duration = time.Since(started)
	return r
}

func startEchoServer(ctx context.Context) (string, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go echoConn(conn)
		}
	}()
	return listener.Addr().String(), func() { _ = listener.Close() }
}

// echoConn echoes data back on conn using separate read and write goroutines.
//
// A naive io.Copy(conn, conn) uses a single goroutine that reads a chunk and
// then writes it back before reading again. Under a full-duplex load, where the
// peer floods both directions before draining either, the write side blocks on
// a full send buffer and the same goroutine can no longer read, wedging the
// connection. Decoupling reads from writes avoids that self-deadlock.
func echoConn(conn net.Conn) {
	defer func() { _ = conn.Close() }()

	pending := make(chan []byte, 1024)
	go func() {
		for b := range pending {
			if _, err := conn.Write(b); err != nil {
				return
			}
		}
	}()

	buf := make([]byte, 64<<10)
	for {
		n, err := conn.Read(buf)
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
}

func startHTTPServer(ctx context.Context, handler http.Handler) (string, func()) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("http server: %v", err)
		}
	}()
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	return listener.Addr().String(), func() { _ = server.Close() }
}
