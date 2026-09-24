package remotedialer

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestExceedBuffer(t *testing.T) {
	ctx := t.Context()

	producerAddress, err := newTestProducer(ctx)
	if err != nil {
		t.Fatal(err)
	}

	serverAddress, server, err := newTestServer(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if err := newTestClient(ctx, "ws://"+serverAddress); err != nil {
		t.Fatal(err)
	}
	// onConnect fires as soon as the client-side websocket handshake completes;
	// the server-side session may still be registering. Wait for it to be
	// visible before issuing dials.
	if err := waitForServerSession(server, "client", 5*time.Second); err != nil {
		t.Fatal(err)
	}

	client := http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, proto, address string) (net.Conn, error) {
				return server.Dialer("client")(ctx, proto, address)
			},
		},
	}

	producerURL := "http://" + producerAddress

	// Drain both responses concurrently. smux applies session-wide flow control,
	// so an undrained stream must not be left blocking while another is fully
	// consumed; reading in parallel reflects correct multiplexed usage.
	type readResult struct {
		n   int
		err error
	}
	drain := func(url string) <-chan readResult {
		ch := make(chan readResult, 1)
		go func() {
			resp, err := client.Get(url)
			if err != nil {
				ch <- readResult{err: err}
				return
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			ch <- readResult{n: len(body), err: err}
		}()
		return ch
	}

	c1 := drain(producerURL)
	c2 := drain(producerURL)

	r1 := <-c1
	if r1.err != nil {
		t.Fatal(r1.err)
	}
	r2 := <-c2
	if r2.err != nil {
		t.Fatal(r2.err)
	}

	assert.Equal(t, 4096*4096, r1.n)
	assert.Equal(t, 4096*4096, r2.n)
}

func newTestServer(ctx context.Context) (string, *Server, error) {
	auth := func(req *http.Request) (clientKey string, authed bool, err error) {
		return "client", true, nil
	}

	server := New(auth, DefaultErrorWriter)
	address, err := newServer(ctx, server)
	return address, server, err
}

func newTestClient(ctx context.Context, url string) error {
	result := make(chan error, 2)
	go func() {
		err := ConnectToProxy(ctx, url, nil, func(proto, address string) bool {
			return true
		}, nil, func(ctx context.Context, session *Session) error {
			result <- nil
			return nil
		})
		result <- err
	}()
	return <-result
}

// waitForServerSession polls until the server registers a session for
// clientKey or the timeout expires. Bridges the gap between the client-side
// onConnect callback (which fires on handshake) and the server-side session
// bookkeeping.
func waitForServerSession(server *Server, clientKey string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if server.HasSession(clientKey) {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("server never registered session for %q within %s", clientKey, timeout)
}

func newServer(ctx context.Context, handler http.Handler) (string, error) {
	server := http.Server{
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
		Handler: handler,
	}
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", err
	}
	go func() {
		<-ctx.Done()
		_ = listener.Close()
		_ = server.Shutdown(context.Background())
	}()
	go func() { _ = server.Serve(listener) }()
	return listener.Addr().String(), nil
}

func newTestProducer(ctx context.Context) (string, error) {
	buffer := make([]byte, 4096)
	return newServer(ctx, http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		for i := 0; i < 4096; i++ {
			if _, err := resp.Write(buffer); err != nil {
				panic(err)
			}
		}
	}))
}
