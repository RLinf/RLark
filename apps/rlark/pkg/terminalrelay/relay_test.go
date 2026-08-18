package terminalrelay

import (
	"errors"
	"io"
	"testing"

	"github.com/gorilla/websocket"
)

func TestClosePayload(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode int
	}{
		{name: "normal close", err: &websocket.CloseError{Code: websocket.CloseNormalClosure}, wantCode: websocket.CloseNormalClosure},
		{name: "going away", err: &websocket.CloseError{Code: websocket.CloseGoingAway}, wantCode: websocket.CloseGoingAway},
		{name: "eof", err: io.EOF, wantCode: websocket.CloseInternalServerErr},
		{name: "unexpected eof", err: io.ErrUnexpectedEOF, wantCode: websocket.CloseInternalServerErr},
		{name: "reserved abnormal code", err: &websocket.CloseError{Code: websocket.CloseAbnormalClosure}, wantCode: websocket.CloseInternalServerErr},
		{name: "proxy failure", err: errors.New("read failed"), wantCode: websocket.CloseInternalServerErr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := closePayload(tt.err)
			if len(payload) < 2 {
				t.Fatalf("close payload is too short: %d", len(payload))
			}
			gotCode := int(payload[0])<<8 | int(payload[1])
			if gotCode != tt.wantCode {
				t.Fatalf("close code = %d, want %d", gotCode, tt.wantCode)
			}
		})
	}
}
