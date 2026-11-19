package remotedialer

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSConn adapts a websocket connection to an io.ReadWriteCloser so it can be
// used as the transport for a smux session. The websocket is a message-oriented
// protocol, so reads drain the current binary frame before fetching the next
// one, and every write is sent as a single binary frame.
//
// Keepalive is delegated entirely to smux: smux emits NOP frames on the
// interval configured in smuxConfig() and tears the session down if no data
// arrives within KeepAliveTimeout. Layering a second websocket-level ping/pong
// keepalive on top would race with smux's timers and, more importantly, would
// require someone to actually send websocket pings, which nothing in this
// package does. We therefore leave the underlying websocket without a read
// deadline and let smux's timeout govern liveness.
type WSConn interface {
	io.ReadWriteCloser
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}

type wsWrapper struct {
	conn *websocket.Conn

	readMu sync.Mutex
	reader io.Reader

	writeMu sync.Mutex
	closeMu sync.Once
}

// NewWSConn wraps a gorilla websocket connection as a WSConn.
func NewWSConn(conn *websocket.Conn) WSConn {
	enableTCPKeepAlive(conn.UnderlyingConn())
	return &wsWrapper{conn: conn}
}

func enableTCPKeepAlive(conn net.Conn) {
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(15 * time.Second)
	}
}

func (w *wsWrapper) Read(p []byte) (int, error) {
	w.readMu.Lock()
	defer w.readMu.Unlock()

	for {
		if w.reader == nil {
			msgType, reader, err := w.conn.NextReader()
			if err != nil {
				return 0, err
			}
			if msgType != websocket.BinaryMessage {
				continue
			}
			w.reader = reader
		}

		n, err := w.reader.Read(p)
		if err == io.EOF {
			// Current frame exhausted, move on to the next one.
			w.reader = nil
			if n > 0 {
				return n, nil
			}
			continue
		}
		return n, err
	}
}

func (w *wsWrapper) Write(p []byte) (int, error) {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()

	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (w *wsWrapper) Close() error {
	var err error
	w.closeMu.Do(func() {
		w.writeMu.Lock()
		_ = w.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
			time.Now().Add(time.Second))
		w.writeMu.Unlock()
		err = w.conn.Close()
	})
	return err
}

func (w *wsWrapper) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

func (w *wsWrapper) SetWriteDeadline(t time.Time) error {
	w.writeMu.Lock()
	defer w.writeMu.Unlock()
	return w.conn.SetWriteDeadline(t)
}
