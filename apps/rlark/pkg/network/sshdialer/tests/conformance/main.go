// Run with:
//
//	go run ./pkg/network/sshdialer/tests/conformance
package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/network/sshdialer"
	"golang.org/x/crypto/ssh"
)

const domainID = "conformance-domain"

type options struct {
	connections int
	payloadSize int
	rounds      int
	duration    time.Duration
	timeout     time.Duration
}

func main() {
	var opts options
	flag.IntVar(&opts.connections, "connections", 32, "number of concurrent tunneled connections")
	flag.IntVar(&opts.payloadSize, "payload-size", 4<<20, "bytes transferred by each connection")
	flag.IntVar(&opts.rounds, "rounds", 3, "concurrent rounds per scenario")
	flag.DurationVar(&opts.duration, "duration", 10*time.Second, "minimum duration of the sustained traffic scenario")
	flag.DurationVar(&opts.timeout, "timeout", 30*time.Second, "timeout for each connection")
	flag.Parse()

	scenarios := []struct {
		name string
		run  func(context.Context, options) error
	}{
		{"high concurrency and sustained traffic", scenarioConcurrentTraffic},
		{"transport interruption and automatic reconnect", scenarioTransportInterruption},
		{"SSH service restart and recovery", scenarioServerRestart},
	}

	failures := 0
	for _, scenario := range scenarios {
		fmt.Printf("== %s\n", scenario.name)
		started := time.Now()
		if err := scenario.run(context.Background(), opts); err != nil {
			failures++
			fmt.Printf("   FAIL (%s): %v\n", time.Since(started).Round(time.Millisecond), err)
			continue
		}
		fmt.Printf("   ok   (%s)\n", time.Since(started).Round(time.Millisecond))
	}
	if failures > 0 {
		fmt.Printf("\n%d scenario(s) failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("\nall scenarios passed")
}

func scenarioConcurrentTraffic(ctx context.Context, opts options) error {
	env, err := startEnvironment(opts)
	if err != nil {
		return err
	}
	defer env.close()

	deadline := time.Now().Add(opts.duration)
	for round := 1; round <= opts.rounds || time.Now().Before(deadline); round++ {
		if err := runConcurrentRound(ctx, env.dialer, env.backend.Addr().String(), opts); err != nil {
			return fmt.Errorf("round %d: %w", round, err)
		}
	}
	if open := env.manager.Stats(); open < 1 || open > 4 {
		return fmt.Errorf("unexpected available transport count: %d", open)
	}
	return nil
}

func scenarioTransportInterruption(ctx context.Context, opts options) error {
	env, err := startEnvironment(opts)
	if err != nil {
		return err
	}
	defer env.close()

	if err := roundTrip(ctx, env.dialer, env.backend.Addr().String(), opts.payloadSize, opts.timeout); err != nil {
		return fmt.Errorf("warm-up: %w", err)
	}
	before := env.server.accepted.Load()
	env.server.dropTransports()

	if err := retry(opts.timeout, func() error {
		return roundTrip(ctx, env.dialer, env.backend.Addr().String(), opts.payloadSize, opts.timeout)
	}); err != nil {
		return fmt.Errorf("recover after transport interruption: %w", err)
	}
	if env.server.accepted.Load() <= before {
		return fmt.Errorf("dialer recovered without establishing a replacement transport")
	}
	return runConcurrentRound(ctx, env.dialer, env.backend.Addr().String(), opts)
}

func scenarioServerRestart(ctx context.Context, opts options) error {
	backend, err := startEchoServer()
	if err != nil {
		return err
	}
	defer func() { _ = backend.Close() }()

	server, err := newSSHTestServer()
	if err != nil {
		return err
	}
	address, err := server.start("127.0.0.1:0")
	if err != nil {
		return err
	}
	defer server.close()

	manager, dialer := newManager(address, opts)
	defer func() { _ = manager.Close() }()
	if err := roundTrip(ctx, dialer, backend.Addr().String(), opts.payloadSize, opts.timeout); err != nil {
		return fmt.Errorf("before restart: %w", err)
	}

	server.stop()
	failCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	conn, dialErr := dialer.DialContext(failCtx, "tcp", backend.Addr().String())
	cancel()
	if dialErr == nil {
		_ = conn.Close()
		return fmt.Errorf("expected dial to fail while SSH service is stopped")
	}

	if _, err := server.start(address); err != nil {
		return fmt.Errorf("restart SSH service: %w", err)
	}
	if err := retry(opts.timeout, func() error {
		return roundTrip(ctx, dialer, backend.Addr().String(), opts.payloadSize, opts.timeout)
	}); err != nil {
		return fmt.Errorf("recover after SSH service restart: %w", err)
	}
	return runConcurrentRound(ctx, dialer, backend.Addr().String(), opts)
}

type testEnvironment struct {
	backend net.Listener
	server  *sshTestServer
	manager *sshdialer.DialerManager
	dialer  *sshdialer.Dialer
}

func startEnvironment(opts options) (*testEnvironment, error) {
	backend, err := startEchoServer()
	if err != nil {
		return nil, err
	}
	server, err := newSSHTestServer()
	if err != nil {
		_ = backend.Close()
		return nil, err
	}
	address, err := server.start("127.0.0.1:0")
	if err != nil {
		_ = backend.Close()
		return nil, err
	}
	manager, dialer := newManager(address, opts)
	return &testEnvironment{backend: backend, server: server, manager: manager, dialer: dialer}, nil
}

func (e *testEnvironment) close() {
	_ = e.manager.Close()
	e.server.close()
	_ = e.backend.Close()
}

func newManager(address string, opts options) (*sshdialer.DialerManager, *sshdialer.Dialer) {
	manager := sshdialer.NewDialerManager(sshdialer.Config{
		SSHTimeout:              opts.timeout,
		InitialReconnectBackoff: 10 * time.Millisecond,
		MaxReconnectBackoff:     250 * time.Millisecond,
		KeepaliveInterval:       100 * time.Millisecond,
		KeepaliveTimeout:        100 * time.Millisecond,
		KeepaliveDrainGrace:     500 * time.Millisecond,
		MaxConnectionsPerDomain: 4,
	})
	dialer := manager.GetDialer(sshdialer.DomainInfo{
		ID:         domainID,
		SSHAddress: address,
		PrivateKey: testPrivateKeyPEM,
	})
	return manager, dialer
}

func runConcurrentRound(ctx context.Context, dialer *sshdialer.Dialer, address string, opts options) error {
	errs := make(chan error, opts.connections)
	var wg sync.WaitGroup
	for i := 0; i < opts.connections; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if err := roundTrip(ctx, dialer, address, opts.payloadSize, opts.timeout); err != nil {
				errs <- fmt.Errorf("connection %d: %w", id, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		return err
	}
	return nil
}

func roundTrip(parent context.Context, dialer *sshdialer.Dialer, address string, size int, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	payload := make([]byte, size)
	if _, err := rand.Read(payload); err != nil {
		return err
	}
	writeDone := make(chan error, 1)
	go func() {
		_, err := conn.Write(payload)
		writeDone <- err
	}()
	response := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, response); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if err := <-writeDone; err != nil {
		return fmt.Errorf("write: %w", err)
	}
	for i := range payload {
		if payload[i] != response[i] {
			return fmt.Errorf("payload mismatch at byte %d", i)
		}
	}
	return nil
}

func retry(timeout time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return lastErr
}

type sshTestServer struct {
	config *ssh.ServerConfig

	mu       sync.Mutex
	listener net.Listener
	conns    map[*ssh.ServerConn]struct{}
	accepted atomic.Int64
	wg       sync.WaitGroup
}

func newSSHTestServer() (*sshTestServer, error) {
	signer, err := ssh.ParsePrivateKey([]byte(testPrivateKeyPEM))
	if err != nil {
		return nil, err
	}
	config := &ssh.ServerConfig{PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
		return &ssh.Permissions{}, nil
	}}
	config.AddHostKey(signer)
	return &sshTestServer{config: config, conns: make(map[*ssh.ServerConn]struct{})}, nil
}

func (s *sshTestServer) start(address string) (string, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()
	s.wg.Add(1)
	go s.serve(ln)
	return ln.Addr().String(), nil
}

func (s *sshTestServer) serve(listener net.Listener) {
	defer s.wg.Done()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go s.serveConn(conn)
	}
}

func (s *sshTestServer) serveConn(conn net.Conn) {
	defer s.wg.Done()
	serverConn, channels, requests, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		_ = conn.Close()
		return
	}
	s.mu.Lock()
	s.conns[serverConn] = struct{}{}
	s.mu.Unlock()
	s.accepted.Add(1)
	defer func() {
		s.mu.Lock()
		delete(s.conns, serverConn)
		s.mu.Unlock()
		_ = serverConn.Close()
	}()
	go ssh.DiscardRequests(requests)
	for channel := range channels {
		if channel.ChannelType() != "direct-tcpip" {
			_ = channel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		var request directTCPIPRequest
		if err := ssh.Unmarshal(channel.ExtraData(), &request); err != nil {
			_ = channel.Reject(ssh.ConnectionFailed, "invalid direct-tcpip request")
			continue
		}
		upstream, err := net.DialTimeout("tcp", net.JoinHostPort(request.Host, fmt.Sprint(request.Port)), 5*time.Second)
		if err != nil {
			_ = channel.Reject(ssh.ConnectionFailed, err.Error())
			continue
		}
		sshChannel, channelRequests, err := channel.Accept()
		if err != nil {
			_ = upstream.Close()
			continue
		}
		go ssh.DiscardRequests(channelRequests)
		go proxy(sshChannel, upstream)
	}
}

type directTCPIPRequest struct {
	Host       string
	Port       uint32
	OriginHost string
	OriginPort uint32
}

func proxy(left io.ReadWriteCloser, right net.Conn) {
	defer func() { _ = left.Close() }()
	defer func() { _ = right.Close() }()
	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(left, right)
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(right, left)
		done <- struct{}{}
	}()
	<-done
}

func (s *sshTestServer) dropTransports() {
	s.mu.Lock()
	conns := make([]*ssh.ServerConn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	s.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

func (s *sshTestServer) stop() {
	s.mu.Lock()
	listener := s.listener
	s.listener = nil
	s.mu.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	s.dropTransports()
	s.wg.Wait()
}

func (s *sshTestServer) close() {
	s.stop()
}

func startEchoServer() (net.Listener, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go echo(conn)
		}
	}()
	return listener, nil
}

func echo(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	pending := make(chan []byte, 128)
	go func() {
		for data := range pending {
			if _, err := conn.Write(data); err != nil {
				return
			}
		}
	}()
	buffer := make([]byte, 64<<10)
	for {
		n, err := conn.Read(buffer)
		if n > 0 {
			data := append([]byte(nil), buffer[:n]...)
			pending <- data
		}
		if err != nil {
			close(pending)
			return
		}
	}
}

func init() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
}

const testPrivateKeyPEM = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDYgEohV8cyTPhXqw3J4KJZ814GmHJAVqXy5IkEH6RBBgAAAKCK3Czsitws
7AAAAAtzc2gtZWQyNTUxOQAAACDYgEohV8cyTPhXqw3J4KJZ814GmHJAVqXy5IkEH6RBBg
AAAEDun/wMJd+XLqbF/nKfrayvmXeLhHjzLd4L+yQ/yFAgD9iASiFXxzJM+FerDcngolnz
XgaYckBWpfLkiQQfpEEGAAAAGmxpZ2h0bmluZ0BDaGVueHVNYWNib29rQWlyAQID
-----END OPENSSH PRIVATE KEY-----`
