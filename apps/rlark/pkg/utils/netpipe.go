package utils

import (
	"context"
	"errors"
	"net"
	"sync"
)

type localAddrContextKey struct{}
type remoteAddrContextKey struct{}

// WithLocalAddr returns a modified value with the given localAddr.
func WithLocalAddr(ctx context.Context, addr net.Addr) context.Context {
	return context.WithValue(ctx, localAddrContextKey{}, addr)
}

// WithRemoteAddr returns a modified value with the given remoteAddr.
func WithRemoteAddr(ctx context.Context, addr net.Addr) context.Context {
	return context.WithValue(ctx, remoteAddrContextKey{}, addr)
}

type netConnWithValues struct {
	net.Conn
	ctx context.Context
}

// LocalAddr is an exported method.
func (c *netConnWithValues) LocalAddr() net.Addr {
	if addr, ok := c.ctx.Value(localAddrContextKey{}).(net.Addr); ok {
		return addr
	}
	return c.Conn.LocalAddr()
}

// RemoteAddr is an exported method.
func (c *netConnWithValues) RemoteAddr() net.Addr {
	if addr, ok := c.ctx.Value(remoteAddrContextKey{}).(net.Addr); ok {
		return addr
	}
	return c.Conn.RemoteAddr()
}

// Dial is a dial function type.
type Dial func(ctx context.Context) (net.Conn, error)

type netPipe struct {
	done      chan struct{}
	c         chan net.Conn
	onceClose sync.Once
}

// Accept is an exported method.
func (p *netPipe) Accept() (net.Conn, error) {
	select {
	case c, ok := <-p.c:
		if !ok {
			return nil, net.ErrClosed
		}
		return c, nil
	case <-p.done:
		return nil, net.ErrClosed
	}
}

// Close is an exported method.
func (p *netPipe) Close() error {
	p.onceClose.Do(func() {
		close(p.done)
		// 此处可以不关闭 p.c，以避免潜在的发送 panic
		// 当 channel 没有被引用时，GC 会回收它
		// close(p.c)
	})
	return nil
}

// Addr adds the r.
func (p *netPipe) Addr() net.Addr {
	return &net.UnixAddr{
		Net:  "net_pipe",
		Name: "net_pipe",
	}
}

// DialContext dials the context.
func (p *netPipe) DialContext(ctx context.Context) (net.Conn, error) {
	// 快速路径：已关闭则直接返回
	select {
	case <-p.done:
		return nil, errors.New("listen dialer closed")
	default:
	}

	c1, c2 := net.Pipe()
	select {
	case p.c <- &netConnWithValues{Conn: c1, ctx: ctx}:
		return c2, nil

	case <-ctx.Done():
		_ = c1.Close()
		_ = c2.Close()
		return nil, ctx.Err()

	case <-p.done:
		_ = c1.Close()
		_ = c2.Close()
		return nil, errors.New("listen dialer closed")
	}
}

// NetPipe 创建无缓冲的 NetPipe
// 注意：DialContext 会阻塞直到 Accept 消费，注意不要在同一个 goroutine 中先 Dial 再 Accept。
func NetPipe() (net.Listener, Dial) {
	return NetPipeWithChannel(make(chan net.Conn))
}

// NetPipeWithChannel 基于指定的 channel 创建 NetPipe，channel 用于传递 net.Conn 对象。
func NetPipeWithChannel(c chan net.Conn) (net.Listener, Dial) {
	ld := &netPipe{
		c:    c,
		done: make(chan struct{}),
	}
	return ld, ld.DialContext
}

// NetPipeWithBuffer 创建带缓冲的 NetPipe，缓冲区大小由 size 决定。
func NetPipeWithBuffer(size int) (net.Listener, Dial) {
	return NetPipeWithChannel(make(chan net.Conn, size))
}
