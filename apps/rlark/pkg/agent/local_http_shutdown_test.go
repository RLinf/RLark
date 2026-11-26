package agent

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

func TestServeHTTPForceClosesActiveRequest(t *testing.T) {
	listener, dial := utils.NetPipe()
	defer func() { _ = listener.Close() }()
	started := make(chan struct{})
	handlerDone := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		close(started)
		<-r.Context().Done()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- serveHTTP(ctx, server, func() error { return server.Serve(listener) }) }()
	t.Cleanup(func() { _ = server.Close() })
	dialCtx, stopDial := context.WithTimeout(context.Background(), time.Second)
	defer stopDial()
	conn, err := dial(dialCtx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(6 * time.Second))
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: local\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("handler did not start")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatalf("shutdown skipped active-request grace period: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP shutdown did not force close and join")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("force close did not cancel request context")
	}
	data := make([]byte, 1)
	if _, err := conn.Read(data); err == nil {
		t.Fatal("request connection remained open")
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		t.Fatal("request connection survived forced shutdown")
	}
}
