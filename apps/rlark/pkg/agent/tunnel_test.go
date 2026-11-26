package agent

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rlinf/rlark/apps/rlark/pkg/remotedialer"
	"github.com/rlinf/rlark/apps/rlark/pkg/server"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIdleRegistrationTunnelJoinsAfterPreparationFailure(t *testing.T) {
	for _, stage := range []string{"preflight", "build"} {
		t.Run(stage, func(t *testing.T) {
			serving, serverDone := make(chan struct{}), make(chan struct{})
			upgrader := websocket.Upgrader{}
			endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(serverDone)
				ws, err := upgrader.Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer func() { _ = ws.Close() }()
				session := remotedialer.NewServerSession(1, "test-agent", remotedialer.NewWSConn(ws))
				defer session.Close()
				close(serving)
				_, _ = session.Serve(r.Context())
			}))
			defer endpoint.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			a := &Agent{serverClient: server.NewClient(endpoint.URL, nil, nil)}
			fatal := errors.New(stage + " failed")
			done := make(chan error, 1)
			go func() {
				done <- runWithRegistration(ctx, func(ctx context.Context, ready func()) error {
					return a.runRegistrationTunnel(ctx, "", ready)
				}, func(ctx context.Context) ([]func(context.Context) error, error) {
					select {
					case <-serving:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					// Allow the client to enter Serve's accept loop after both
					// sides finish construction. No business streams are opened.
					select {
					case <-time.After(50 * time.Millisecond):
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					if stage == "preflight" {
						checkCtx, stop := context.WithTimeout(ctx, 50*time.Millisecond)
						defer stop()
						return nil, waitManagementAccess(checkCtx, func(context.Context) error {
							return apierrors.NewForbidden(schema.GroupResource{Resource: "tasks"}, "", fatal)
						})
					}
					return buildRoles(ctx, DefaultConfig(), &testStartupRole{build: func(context.Context) error { return fatal }}, nil)
				})
			}()
			select {
			case err := <-done:
				want := fatal
				if stage == "preflight" {
					want = context.DeadlineExceeded
				}
				if !errors.Is(err, want) {
					t.Fatalf("failure lost: %v", err)
				}
			case <-time.After(2 * time.Second):
				cancel()
				t.Fatal("preparation failure did not join idle tunnel")
			}
			select {
			case <-serverDone:
			case <-time.After(time.Second):
				t.Fatal("WebSocket server session was not closed")
			}
		})
	}
}

func TestRegistrationTunnelCancellationClosesUnresponsivePeer(t *testing.T) {
	connected, serverDone := make(chan struct{}), make(chan struct{})
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(serverDone)
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = ws.Close() }()
		close(connected)
		// Accept the transport but do not serve the remotedialer protocol.
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer endpoint.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &Agent{serverClient: server.NewClient(endpoint.URL, nil, nil)}
	done := make(chan error, 1)
	go func() { done <- a.runRegistrationTunnel(ctx, "", nil) }()
	select {
	case <-connected:
	case <-time.After(time.Second):
		t.Fatal("transport did not connect")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not close unresponsive tunnel")
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("transport remained open")
	}
}
