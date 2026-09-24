// Run from apps/rlark:
//
//	go run ./pkg/remotedialer/tests/disconnect-repro -mode hard -timeout 20s
//	go run ./pkg/remotedialer/tests/disconnect-repro -mode idle -timeout 20s -duration 45s
//
// hard simulates a middlebox with a fixed connection lifetime and should
// produce "connection reset by peer" on both tunnel endpoints. idle resets only
// when no TCP payload crosses the proxy; smux's five-second NOP should keep it
// alive when timeout is 20 seconds.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/remotedialer"
)

const clientID = "disconnect-repro"

type resetProxy struct {
	mode    string
	timeout time.Duration
	target  string
}

func main() {
	mode := flag.String("mode", "hard", "disconnect mode: hard or idle")
	timeout := flag.Duration("timeout", 20*time.Second, "fixed lifetime or TCP payload idle timeout")
	duration := flag.Duration("duration", 45*time.Second, "maximum reproduction duration")
	flag.Parse()

	if (*mode != "hard" && *mode != "idle") || *timeout <= 0 || *duration <= 0 {
		log.Fatal("mode must be hard or idle; timeout and duration must be positive")
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(rootCtx, *duration)
	defer cancel()

	tunnel := remotedialer.New(func(*http.Request) (string, bool, error) {
		return clientID, true, nil
	}, remotedialer.DefaultErrorWriter)
	serverAddr, closeServer, err := startHTTPServer(tunnel)
	if err != nil {
		log.Fatal(err)
	}
	defer closeServer()

	proxyAddr, closeProxy, err := startResetProxy(ctx, resetProxy{
		mode: *mode, timeout: *timeout, target: serverAddr,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer closeProxy()

	started := time.Now()
	log.Printf("starting mode=%s timeout=%s duration=%s path=client -> %s -> %s", *mode, *timeout, *duration, proxyAddr, serverAddr)
	err = remotedialer.ConnectToProxy(ctx, "ws://"+proxyAddr, nil,
		func(string, string) bool { return true }, nil,
		func(context.Context, *remotedialer.Session) error {
			log.Printf("tunnel connected after %s; leaving it idle", time.Since(started).Round(time.Millisecond))
			return nil
		})
	elapsed := time.Since(started).Round(time.Millisecond)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) && err == nil {
		log.Printf("tunnel remained connected for %s", elapsed)
		if *mode == "idle" {
			log.Printf("PASS: smux traffic prevented the %s TCP payload idle timeout", *timeout)
		}
		return
	}
	if err != nil {
		log.Printf("tunnel disconnected after %s: %v", elapsed, err)
		if *mode == "hard" {
			log.Printf("PASS: reproduced forced middlebox reset")
		}
		return
	}
	log.Printf("tunnel stopped after %s: %v", elapsed, ctx.Err())
}

func startHTTPServer(handler http.Handler) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	server := &http.Server{Handler: handler}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("tunnel server: %v", err)
		}
	}()
	return listener.Addr().String(), func() { _ = server.Close() }, nil
}

func startResetProxy(ctx context.Context, proxy resetProxy) (string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			go proxy.handle(ctx, client)
		}
	}()
	return listener.Addr().String(), func() { _ = listener.Close() }, nil
}

func (p resetProxy) handle(ctx context.Context, client net.Conn) {
	server, err := net.Dial("tcp", p.target)
	if err != nil {
		log.Printf("proxy dial server: %v", err)
		_ = client.Close()
		return
	}

	started := time.Now()
	var lastActivity atomic.Int64
	lastActivity.Store(started.UnixNano())
	errCh := make(chan error, 2)
	go proxyCopy(server, client, &lastActivity, errCh)
	go proxyCopy(client, server, &lastActivity, errCh)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			closePair(client, server, false)
			return
		case err := <-errCh:
			closePair(client, server, false)
			if err != nil && !errors.Is(err, net.ErrClosed) {
				log.Printf("proxy copy stopped after %s: %v", time.Since(started).Round(time.Millisecond), err)
			}
			return
		case <-ticker.C:
			elapsed := time.Since(started)
			idle := time.Since(time.Unix(0, lastActivity.Load()))
			if p.mode == "hard" && elapsed >= p.timeout || p.mode == "idle" && idle >= p.timeout {
				log.Printf("proxy injecting TCP RST mode=%s connected=%s payload-idle=%s", p.mode, elapsed.Round(time.Millisecond), idle.Round(time.Millisecond))
				closePair(client, server, true)
				return
			}
		}
	}
}

func proxyCopy(dst, src net.Conn, activity *atomic.Int64, result chan<- error) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			activity.Store(time.Now().UnixNano())
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				result <- writeErr
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				result <- nil
			} else {
				result <- err
			}
			return
		}
	}
}

func closePair(a, b net.Conn, reset bool) {
	var wg sync.WaitGroup
	for _, conn := range []net.Conn{a, b} {
		wg.Add(1)
		go func(conn net.Conn) {
			defer wg.Done()
			if reset {
				if tcp, ok := conn.(*net.TCPConn); ok {
					if err := tcp.SetLinger(0); err != nil {
						log.Printf("set SO_LINGER on %s: %v", conn.LocalAddr(), err)
					}
				}
			}
			if err := conn.Close(); err != nil {
				log.Printf("close %s: %v", fmt.Sprint(conn.LocalAddr()), err)
			}
		}(conn)
	}
	wg.Wait()
}
