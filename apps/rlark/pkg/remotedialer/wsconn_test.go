package remotedialer

import (
	"net"
	"testing"
)

func TestEnableTCPKeepAlive(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()

	serverConn := <-accepted
	defer func() { _ = serverConn.Close() }()

	// The helper must accept both TCP connections and unrelated net.Conn
	// implementations without panicking or changing the transport API.
	enableTCPKeepAlive(conn)
	local, remote := net.Pipe()
	defer func() { _ = local.Close() }()
	defer func() { _ = remote.Close() }()
	enableTCPKeepAlive(local)
}
