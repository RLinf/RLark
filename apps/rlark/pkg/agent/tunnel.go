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
