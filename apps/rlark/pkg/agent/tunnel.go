package agent

import (
	"context"
	"math/rand"
	"net"
	"net/http"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/remotedialer"
)

func (a *Agent) runTunnel(ctx context.Context, role string) error {
	return a.runRegistrationTunnel(ctx, role, nil)
}

func (a *Agent) runRegistrationTunnel(ctx context.Context, role string, registered func()) error {
	logger := log.FromContext(ctx)
	auth := func(proto, address string) bool {
		return true
	}
	netDialer := func(ctx context.Context, network, addr string) (net.Conn, error) {
		var d net.Dialer
		if addr == "0.0.0.0:1" { // 约定的 local server 地址
			return a.localDialer(ctx)
		}
		return d.DialContext(ctx, network, addr)
	}
	header := make(http.Header)
	if role != "" {
		header.Set(apis.RemoteDialerRoleHeader, role)
	}
	connect := func() error {
		startedAt := time.Now()
		ws, _, err := a.serverClient.DialWebsocket(ctx, header)
		if err != nil {
			return err
		}
		defer func() { _ = ws.Close() }()
		// Serve can block in mux.AcceptStream, and session construction can
		// block in the control handshake. Context cancellation alone cannot
		// interrupt either transport read: actively close the WebSocket.
		connectionCtx, stopConnection := context.WithCancel(ctx)
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			<-connectionCtx.Done()
			_ = ws.Close()
		}()
		defer func() { stopConnection(); <-closed }()
		// Server completes registration before accepting the WebSocket upgrade.
		if registered != nil {
			registered()
		}

		sessCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		session := remotedialer.NewClientSessionWithDialer(auth, ws, netDialer)
		defer session.Close()

		_, err = session.Serve(sessCtx)
		stage, duration := session.Diagnostics()
		logger.Info("tunnel session ended", "role", role, "duration", duration, "stage", stage)
		_ = startedAt
		return err
	}

	for {
		if err := connect(); err != nil {
			logger.Error(err, "tunnel connection error")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(4_000+rand.Intn(2_001)) * time.Millisecond):
		}
	}
}
