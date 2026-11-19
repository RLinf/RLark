package agent

import (
	"context"
	"math/rand"
	"net"
	"net/http"
	"strconv"
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
	// The tunnel says whether the agent is ready, and the Server keeps the
	// heartbeat of a ready agent only. The agent connects before it is ready
	// (the Server makes the agent's namespace and permissions when it
	// connects, and the first watches need them), and connects anew whenever
	// its readiness changes.
	connect := func() (bool, error) {
		ready, changed := a.readiness.state()
		header.Set(apis.RemoteDialerReadyHeader, strconv.FormatBool(ready))
		ws, _, err := a.serverClient.DialWebsocket(ctx, header)
		if err != nil {
			return false, err
		}
		defer func() { _ = ws.Close() }()

		sessCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		session := remotedialer.NewClientSessionWithDialer(auth, ws, netDialer)
		defer session.Close()
		// A session waits for the Server's next stream and does not look at its context meanwhile: when
		// the agent's readiness changes, or the agent stops, it is closed, which ends the wait.
		go func() {
			select {
			case <-changed:
			case <-sessCtx.Done():
			}
			session.Close()
		}()

		_, err = session.Serve(sessCtx)
		stage, duration := session.Diagnostics()
		logger.Info("tunnel session ended", "role", role, "ready", ready, "duration", duration, "stage", stage)
		select {
		case <-changed:
			return true, nil
		default:
			return false, err
		}
	}

	for {
		readinessChanged, err := connect()
		if readinessChanged && ctx.Err() == nil {
			continue
		}
		if err != nil {
			logger.Error(err, "tunnel connection error")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(4_000+rand.Intn(2_001)) * time.Millisecond):
		}
	}
}
