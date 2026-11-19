// Run Test: go run ./apps/rlark/pkg/remotedialer/tests/conformance

package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/remotedialer"
)

const (
	clientKey    = "conformance-client"
	peerAToken   = "peer-a-token"
	peerBToken   = "peer-b-token"
	waitDeadline = 10 * time.Second
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	failures := 0
	runCase := func(name string, fn func(context.Context) error) {
		fmt.Printf("== %s\n", name)
		start := time.Now()
		err := fn(ctx)
		dur := time.Since(start).Round(time.Millisecond)
		if err != nil {
			failures++
			fmt.Printf("   FAIL (%s): %v\n", dur, err)
			return
		}
		fmt.Printf("   ok   (%s)\n", dur)
	}

	runCase("direct proxy: single connection round-trip", scenarioDirectRoundTrip)
	runCase("direct proxy: concurrent connections", scenarioConcurrent)
	runCase("direct proxy: dial unreachable backend returns error", scenarioBackendError)
	runCase("peer forwarding: cross-server dial", scenarioPeerForwarding)
	runCase("teardown: dialer fails after client disconnects", scenarioTeardown)

	if failures > 0 {
		fmt.Printf("\n%d case(s) failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("\nall cases passed")
}

// ---------------------------------------------------------------------------
// Scenarios
// ---------------------------------------------------------------------------

func scenarioDirectRoundTrip(ctx context.Context) error {
	env, err := startSingleServerEnv(ctx)
	if err != nil {
		return err
	}
	defer env.close()

	return roundTrip(ctx, env.server.Dialer(clientKey), env.backend.Addr().String(), 64<<10)
}

func scenarioConcurrent(ctx context.Context) error {
	env, err := startSingleServerEnv(ctx)
	if err != nil {
		return err
	}
	defer env.close()

	const conns = 8
	const size = 256 << 10
	errs := make(chan error, conns)
	var wg sync.WaitGroup
	for i := 0; i < conns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- roundTrip(ctx, env.server.Dialer(clientKey), env.backend.Addr().String(), size)
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func scenarioBackendError(ctx context.Context) error {
	env, err := startSingleServerEnv(ctx)
	if err != nil {
		return err
	}
	defer env.close()

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Port 1 on loopback is essentially never bound; the client-side dial
	// should fail and the tunnel must surface that failure to the caller.
	conn, err := env.server.Dialer(clientKey)(dialCtx, "tcp", "127.0.0.1:1")
	if err == nil {
		_ = conn.Close()
		// smux may accept the stream and close it when the client-side dial
		// fails; a subsequent read should return an error.
		buf := make([]byte, 1)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		if _, rerr := conn.Read(buf); rerr == nil {
			return fmt.Errorf("expected dial or read to fail for unreachable backend, got success")
		}
	}
	return nil
}

func scenarioPeerForwarding(ctx context.Context) error {
	env, err := startPeerEnv(ctx)
	if err != nil {
		return err
	}
	defer env.close()

	// The client only connects to serverA. Reaching the backend via serverB
	// exercises the peer routing path: serverB.Dialer -> peer session ->
	// serverA -> client -> backend.
	if err := waitFor(waitDeadline, func() bool {
		return env.serverA.HasSession(clientKey) && env.serverB.HasSession(clientKey)
	}); err != nil {
		return fmt.Errorf("peer routing table did not converge: %w", err)
	}

	// Verify both paths independently.
	if err := roundTrip(ctx, env.serverA.Dialer(clientKey), env.backend.Addr().String(), 64<<10); err != nil {
		return fmt.Errorf("serverA -> client: %w", err)
	}
	if err := roundTrip(ctx, env.serverB.Dialer(clientKey), env.backend.Addr().String(), 64<<10); err != nil {
		return fmt.Errorf("serverB -> client (via peer): %w", err)
	}
	return nil
}

func scenarioTeardown(ctx context.Context) error {
	env, err := startSingleServerEnv(ctx)
	if err != nil {
		return err
	}
	// Bring down the client explicitly.
	env.stopClient()

	if err := waitFor(waitDeadline, func() bool { return !env.server.HasSession(clientKey) }); err != nil {
		env.close()
		return fmt.Errorf("session was not removed after client disconnect: %w", err)
	}

	dialCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err = env.server.Dialer(clientKey)(dialCtx, "tcp", env.backend.Addr().String())
	env.close()
	if err == nil {
		return fmt.Errorf("expected dial to fail after client disconnect")
	}
	if !strings.Contains(err.Error(), "failed to find session") {
		return fmt.Errorf("unexpected teardown error: %v", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Environments
// ---------------------------------------------------------------------------

type singleServerEnv struct {
	server     *remotedialer.Server
	backend    net.Listener
	closeFns   []func()
	clientStop context.CancelFunc
}

func (e *singleServerEnv) stopClient() {
	if e.clientStop != nil {
		e.clientStop()
		e.clientStop = nil
	}
}

func (e *singleServerEnv) close() {
	e.stopClient()
	for _, v := range slices.Backward(e.closeFns) {
		v()
	}
}

func startSingleServerEnv(ctx context.Context) (*singleServerEnv, error) {
	backend, err := startEchoBackend()
	if err != nil {
		return nil, err
	}

	server, srvAddr, srvClose, err := startTunnelServer(func(req *http.Request) (string, bool, error) {
		return req.Header.Get("X-Client-Key"), req.Header.Get("X-Client-Key") != "", nil
	})
	if err != nil {
		_ = backend.Close()
		return nil, err
	}

	clientCtx, clientCancel := context.WithCancel(ctx)
	clientReady, err := startTunnelClient(clientCtx, "ws://"+srvAddr, clientKey)
	if err != nil {
		clientCancel()
		srvClose()
		_ = backend.Close()
		return nil, err
	}
	<-clientReady

	if err := waitFor(waitDeadline, func() bool { return server.HasSession(clientKey) }); err != nil {
		clientCancel()
		srvClose()
		_ = backend.Close()
		return nil, fmt.Errorf("session not registered: %w", err)
	}

	return &singleServerEnv{
		server:     server,
		backend:    backend,
		clientStop: clientCancel,
		closeFns:   []func(){srvClose, func() { _ = backend.Close() }},
	}, nil
}

type peerEnv struct {
	serverA, serverB *remotedialer.Server
	backend          net.Listener
	closeFns         []func()
	clientStop       context.CancelFunc
}

func (e *peerEnv) close() {
	if e.clientStop != nil {
		e.clientStop()
	}
	for _, v := range slices.Backward(e.closeFns) {
		v()
	}
}

func startPeerEnv(ctx context.Context) (*peerEnv, error) {
	backend, err := startEchoBackend()
	if err != nil {
		return nil, err
	}

	auth := func(req *http.Request) (string, bool, error) {
		return req.Header.Get("X-Client-Key"), req.Header.Get("X-Client-Key") != "", nil
	}
	serverA, addrA, closeA, err := startTunnelServer(auth)
	if err != nil {
		_ = backend.Close()
		return nil, err
	}
	serverA.PeerID = "peer-A"
	serverA.PeerToken = peerAToken

	serverB, addrB, closeB, err := startTunnelServer(auth)
	if err != nil {
		closeA()
		_ = backend.Close()
		return nil, err
	}
	serverB.PeerID = "peer-B"
	serverB.PeerToken = peerBToken

	// Peers authenticate reciprocally with each other's ID/token.
	serverA.AddPeer("ws://"+addrB, serverB.PeerID, serverB.PeerToken)
	serverB.AddPeer("ws://"+addrA, serverA.PeerID, serverA.PeerToken)

	clientCtx, clientCancel := context.WithCancel(ctx)
	clientReady, err := startTunnelClient(clientCtx, "ws://"+addrA, clientKey)
	if err != nil {
		clientCancel()
		closeB()
		closeA()
		_ = backend.Close()
		return nil, err
	}
	<-clientReady

	return &peerEnv{
		serverA:    serverA,
		serverB:    serverB,
		backend:    backend,
		clientStop: clientCancel,
		closeFns:   []func(){closeB, closeA, func() { _ = backend.Close() }},
	}, nil
}

// ---------------------------------------------------------------------------
// Building blocks
// ---------------------------------------------------------------------------

// startEchoBackend starts a TCP listener that echoes bytes back on every
// connection. Reads and writes are decoupled so a full-duplex sender cannot
// self-deadlock a single io.Copy(conn, conn) goroutine.
func startEchoBackend() (net.Listener, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go echoConn(c)
		}
	}()
	return ln, nil
}

func echoConn(c net.Conn) {
	defer func() { _ = c.Close() }()
	pending := make(chan []byte, 128)
	go func() {
		for b := range pending {
			if _, err := c.Write(b); err != nil {
				return
			}
		}
	}()
	buf := make([]byte, 32<<10)
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
}

// startTunnelServer starts a remotedialer.Server bound to a random localhost
// port and returns the server, its address, and a closer.
func startTunnelServer(auth remotedialer.Authorizer) (*remotedialer.Server, string, func(), error) {
	handler := remotedialer.New(auth, remotedialer.DefaultErrorWriter)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", nil, err
	}
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	return handler, ln.Addr().String(), func() { _ = srv.Close() }, nil
}

// startTunnelClient connects to url and reports readiness on the returned
// channel once the client-side onConnect callback has fired.
func startTunnelClient(ctx context.Context, url, key string) (<-chan struct{}, error) {
	ready := make(chan struct{})
	headers := http.Header{"X-Client-Key": []string{key}}
	go func() {
		err := remotedialer.ConnectToProxy(ctx, url, headers,
			func(string, string) bool { return true }, nil,
			func(context.Context, *remotedialer.Session) error {
				closeOnce(ready)
				return nil
			})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("tunnel client %s: %v", url, err)
		}
	}()
	select {
	case <-ready:
		return ready, nil
	case <-time.After(waitDeadline):
		return nil, fmt.Errorf("timed out waiting for tunnel client to connect to %s", url)
	}
}

// roundTrip opens one tunneled connection, sends a random payload of `size`
// bytes and verifies the echoed response matches byte for byte.
func roundTrip(ctx context.Context, dial remotedialer.Dialer, addr string, size int) error {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := dial(dialCtx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	payload := make([]byte, size)
	if _, err := rand.Read(payload); err != nil {
		return err
	}

	writeErr := make(chan error, 1)
	go func() {
		_, err := conn.Write(payload)
		writeErr <- err
	}()

	got := make([]byte, size)
	if _, err := io.ReadFull(conn, got); err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if err := <-writeErr; err != nil {
		return fmt.Errorf("write: %w", err)
	}
	for i := range payload {
		if payload[i] != got[i] {
			return fmt.Errorf("payload mismatch at byte %d", i)
		}
	}
	return nil
}

func waitFor(timeout time.Duration, cond func() bool) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("condition not met within %s", timeout)
}

func closeOnce(ch chan struct{}) {
	defer func() { _ = recover() }()
	close(ch)
}
