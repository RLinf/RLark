package jumper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WSTerminal wraps a WebSocket connection as a jumper.Terminal.
//
// It follows the protocol defined in agent/terminal.go:
//   - Binary messages carry terminal I/O (stdin → server, server → stdout)
//   - Text JSON messages are control messages dispatched by "type":
//   - {"type":"resize","rows":N,"cols":N}  — resize the remote terminal
//   - {"type":"error","message":"..."}    — server error, injected into Read
//   - File-transfer control messages from the server are silently dropped
//     (this adapter is for terminal I/O only).
type WSTerminal struct {
	conn   *websocket.Conn
	wmu    sync.Mutex
	buf    []byte
	rcond  *sync.Cond
	closed bool
}

// DialWebSocket connects to a WebSocket URL and returns a Terminal.
func DialWebSocket(ctx context.Context, url string, header http.Header) (*WSTerminal, error) {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, url, header)
	if err != nil {
		return nil, fmt.Errorf("ws dial: %w", err)
	}
	return NewWSTerminalFromConn(conn), nil
}

// NewWSTerminalFromConn wraps an existing WebSocket connection as a Terminal.
func NewWSTerminalFromConn(conn *websocket.Conn) *WSTerminal {
	t := &WSTerminal{
		conn:  conn,
		rcond: sync.NewCond(&sync.Mutex{}),
	}
	go t.readLoop()
	return t
}

func (t *WSTerminal) readLoop() {
	for {
		msgType, data, err := t.conn.ReadMessage()
		if err != nil {
			t.rcond.L.Lock()
			t.closed = true
			t.rcond.Broadcast()
			t.rcond.L.Unlock()
			return
		}
		if msgType == websocket.TextMessage {
			// Parse control messages; inject error messages into the Read
			// buffer so they appear on the terminal. Drop everything else.
			if len(data) > 0 && data[0] == '{' {
				var probe struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(data, &probe) == nil && probe.Type == "error" {
					var msg wsErrorMsg
					if json.Unmarshal(data, &msg) == nil {
						text := "\r\n" + msg.Message + "\r\n"
						t.rcond.L.Lock()
						t.buf = append(t.buf, text...)
						t.rcond.Broadcast()
						t.rcond.L.Unlock()
					}
				}
			}
			continue
		}
		// Buffer Binary messages for Read().
		t.rcond.L.Lock()
		t.buf = append(t.buf, data...)
		t.rcond.Broadcast()
		t.rcond.L.Unlock()
	}
}

// Read implements io.Reader. It returns buffered data received from the
// WebSocket Binary messages, blocking until data is available or the
// connection is closed.
func (t *WSTerminal) Read(dst []byte) (int, error) {
	t.rcond.L.Lock()
	for len(t.buf) == 0 && !t.closed {
		t.rcond.Wait()
	}
	if len(t.buf) == 0 && t.closed {
		t.rcond.L.Unlock()
		return 0, io.EOF
	}
	n := copy(dst, t.buf)
	t.buf = t.buf[n:]
	t.rcond.L.Unlock()
	return n, nil
}

// Write implements io.Writer. It sends data as a WebSocket Binary message.
func (t *WSTerminal) Write(data []byte) (int, error) {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	if err := t.conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return 0, err
	}
	return len(data), nil
}

// Resize sends a resize control message to the server.
// The message format is: {"type":"resize","rows":N,"cols":N}.
func (t *WSTerminal) Resize(rows, cols uint16) error {
	msg, err := json.Marshal(resizeMsg{Type: "resize", Rows: rows, Cols: cols})
	if err != nil {
		return err
	}
	t.wmu.Lock()
	defer t.wmu.Unlock()
	return t.conn.WriteMessage(websocket.TextMessage, msg)
}

// Close closes the WebSocket connection.
func (t *WSTerminal) Close() error {
	t.rcond.L.Lock()
	t.closed = true
	t.rcond.Broadcast()
	t.rcond.L.Unlock()
	return t.conn.Close()
}

type resizeMsg struct {
	Type string `json:"type"`
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

type wsErrorMsg struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}
