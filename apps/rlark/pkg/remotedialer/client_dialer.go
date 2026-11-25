package remotedialer

import (
	"context"
	"net"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// clientDial dials the requested target on behalf of the remote peer and pipes
// data between the smux stream and the dialed connection. Flow control and
// stream teardown are handled by smux.
func clientDial(ctx context.Context, dialer Dialer, stream net.Conn, proto, address string, admitted func()) {
	defer func() { _ = stream.Close() }()

	var (
		netConn net.Conn
		err     error
	)

	dialCtx, cancel := context.WithDeadline(ctx, time.Now().Add(time.Minute))
	if dialer == nil {
		d := net.Dialer{}
		netConn, err = d.DialContext(dialCtx, proto, address)
	} else {
		netConn, err = dialer(dialCtx, proto, address)
	}
	cancel()
	admitted()

	if err != nil {
		return
	}
	defer func() { _ = netConn.Close() }()
	if ctx.Err() != nil {
		return
	}
	// Closing smux alone cannot interrupt an idle target read. Cancel and join
	// only this stream's copies, never its shared WebSocket/mux transport.
	closed := make(chan struct{})
	stopClose := context.AfterFunc(ctx, func() {
		_ = stream.Close()
		_ = netConn.Close()
		close(closed)
	})
	defer func() {
		if !stopClose() {
			<-closed
		}
	}()
	_, _ = utils.RelayConnections(stream, netConn, "stream", "target")
}
