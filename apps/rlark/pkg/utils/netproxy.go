package utils

import (
	"bufio"
	"fmt"
	"net"
	"net/url"
)

// 实现一个简单的代理协议：客户端通过 TCP 连接发送目标信息，格式为 "network://address"，直到换行符结束。
// 服务端解析目标信息后，建立对应的网络连接进行转发。

// WrapConn wraps a connection with buffered reading.
type WrapConn struct {
	net.Conn
	r *bufio.Reader
}

// NewWrapConn creates a new WrapConn.
func NewWrapConn(conn net.Conn) *WrapConn {
	return &WrapConn{
		Conn: conn,
		r:    bufio.NewReader(conn),
	}
}

// Read is an exported method.
func (c *WrapConn) Read(p []byte) (n int, err error) {
	return c.r.Read(p)
}

// ReadLine reads the line.
func (c *WrapConn) ReadLine() (string, error) {
	buf := make([]byte, 0, 1024)
	for {
		b, err := c.r.ReadByte()
		if err != nil {
			return "", err
		}
		if b == '\n' {
			break
		}
		buf = append(buf, b)
		if len(buf) >= 1024 {
			return "", fmt.Errorf("line too long")
		}
	}
	return string(buf), nil
}

// ReadTargetFromConn reads the targetFromConn.
func ReadTargetFromConn(conn *WrapConn) (string, string, string, url.Values, error) {
	// 这里的协议非常简单，直接读取一行文本，格式为 "network://address"，例如 "tcp://127.0.0.1:8080"
	// 读取到 \n 结束
	line, err := conn.ReadLine()
	if err != nil {
		return "", "", "", nil, err
	}
	u, err := url.Parse(line)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("failed to parse target: %w", err)
	}
	return u.Scheme, u.Hostname(), u.Port(), u.Query(), nil
}
