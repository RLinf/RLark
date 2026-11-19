// Package terminalrelay forwards WebSocket terminal traffic between proxies.
package terminalrelay

import (
	"errors"
	"io"
	"time"

	"github.com/gorilla/websocket"
)

const closeWriteTimeout = time.Second

// Relay forwards messages bidirectionally until either connection closes.
// Read failures are represented as WebSocket close frames instead of terminal
// output, so a normal shell exit cannot be mistaken for command output.
func Relay(a, b *websocket.Conn) {
	done := make(chan struct{}, 2)

	copyLoop := func(dst, src *websocket.Conn) {
		defer func() { done <- struct{}{} }()
		for {
			msgType, data, err := src.ReadMessage()
			if err != nil {
				_ = dst.WriteControl(
					websocket.CloseMessage,
					closePayload(err),
					time.Now().Add(closeWriteTimeout),
				)
				return
			}
			if err := dst.WriteMessage(msgType, data); err != nil {
				return
			}
		}
	}

	go copyLoop(a, b)
	go copyLoop(b, a)

	<-done
}

func closePayload(err error) []byte {
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) && validCloseCode(closeErr.Code) {
		return websocket.FormatCloseMessage(closeErr.Code, closeErr.Text)
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return websocket.FormatCloseMessage(
			websocket.CloseInternalServerErr,
			"terminal proxy connection lost",
		)
	}
	return websocket.FormatCloseMessage(
		websocket.CloseInternalServerErr,
		"terminal proxy connection closed",
	)
}

func validCloseCode(code int) bool {
	return code != websocket.CloseNoStatusReceived &&
		code != websocket.CloseAbnormalClosure &&
		code != websocket.CloseTLSHandshake
}
