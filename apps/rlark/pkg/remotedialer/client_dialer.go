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
func clientDial(ctx context.Context, dialer Dialer, stream net.Conn, proto, address string) {
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

	if err != nil {
		return
	}
	defer func() { _ = netConn.Close() }()

	_, _ = utils.RelayConnections(stream, netConn, "stream", "target")
}
