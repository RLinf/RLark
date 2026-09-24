package remotedialer

import "time"

const (
	// PingWriteInterval is the smux keepalive interval: how often a NOP frame
	// is emitted on an idle tunnel to prove the transport is still alive.
	PingWriteInterval = 5 * time.Second
	// PingWaitDuration is the smux keepalive timeout: if no traffic (including
	// NOP frames) arrives within this window, smux tears the session down.
	// The name is retained for API compatibility with the pre-smux era.
	PingWaitDuration = 60 * time.Second
	// MaxRead is the legacy maximum read size retained for API compatibility.
	MaxRead = 8192
	// HandshakeTimeOut is the timeout used when establishing a peer WebSocket.
	HandshakeTimeOut = 10 * time.Second
	// ControlStreamTimeout bounds the initial reserved stream handshake.
	ControlStreamTimeout = 10 * time.Second
	// MaxBuffer is the smux session-wide receive buffer size. It bounds the
	// total amount of unread data buffered across all streams of a session.
	MaxBuffer = 64 * 1024 * 1024
	// MaxStreamBuffer is the per-stream receive buffer size used for flow control.
	MaxStreamBuffer = 4 * 1024 * 1024
)
