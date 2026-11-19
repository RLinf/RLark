// Package idlerelay is an HTTP CONNECT proxy that closes a tunnel in which nothing has moved in either
// direction for a while. Go's HTTP clients (a registry fetching from its upstream or from S3) read a
// response body without any deadline, so a connection that stays open but goes silent holds the request
// forever; behind this relay (HTTPS_PROXY) such a read fails after the idle time and the request can be
// made again.
//
// With a list of allowed targets it is also a site's way out to the package indexes a build needs: nodes
// without a route to the Internet reach those hosts, and no other, through a node that has one.
package idlerelay

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	rlarklog "github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// Allowed reports whether the CONNECT target host:port is one of allow: a host name, or ".suffix" for a
// domain and every name under it. An empty list allows every target.
func Allowed(allow []string, target string) bool {
	host, _, err := net.SplitHostPort(target)
	if err != nil {
		return false
	}
	for _, name := range allow {
		if host == strings.TrimPrefix(name, ".") || strings.HasPrefix(name, ".") && strings.HasSuffix(host, name) {
			return true
		}
	}
	return len(allow) == 0
}

// InterfaceAddress is the IPv4 address of the network interface name: a relay that listens there is reached
// through that interface's network alone (a node's site tunnel).
func InterfaceAddress(name string) (string, error) {
	device, err := net.InterfaceByName(name)
	if err != nil {
		return "", err
	}
	addresses, err := device.Addrs()
	if err != nil {
		return "", err
	}
	for _, address := range addresses {
		if network, ok := address.(*net.IPNet); ok && network.IP.To4() != nil {
			return network.IP.String(), nil
		}
	}
	return "", fmt.Errorf("interface %s has no IPv4 address", name)
}

// Serve relays CONNECT tunnels accepted on ln, to the targets of allow, until ctx ends.
func Serve(ctx context.Context, ln net.Listener, idle time.Duration, allow []string) error {
	go func() { <-ctx.Done(); ln.Close() }()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go relay(ctx, conn, idle, allow)
	}
}

func relay(ctx context.Context, client net.Conn, idle time.Duration, allow []string) {
	defer client.Close()
	reader := bufio.NewReader(client)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	if request.Method != http.MethodConnect {
		io.WriteString(client, "HTTP/1.1 405 Method Not Allowed\r\nConnection: close\r\n\r\n")
		return
	}
	if !Allowed(allow, request.Host) {
		rlarklog.GetLogger().Info("refusing a target that is not allowed", "target", request.Host)
		io.WriteString(client, "HTTP/1.1 403 Forbidden\r\nConnection: close\r\n\r\n")
		return
	}
	upstream, err := (&net.Dialer{Timeout: idle}).DialContext(ctx, "tcp", request.Host)
	if err != nil {
		io.WriteString(client, "HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n")
		return
	}
	defer upstream.Close()
	if _, err := io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}

	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	done := make(chan struct{}, 2)
	pipe := func(dst io.Writer, src io.Reader) {
		buf := make([]byte, 64<<10)
		for {
			n, err := src.Read(buf)
			if n > 0 {
				last.Store(time.Now().UnixNano())
				if _, werr := dst.Write(buf[:n]); werr != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
		done <- struct{}{}
	}
	go pipe(upstream, reader)
	go pipe(client, upstream)

	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-tick.C:
			if silent := time.Since(time.Unix(0, last.Load())); silent > idle {
				rlarklog.GetLogger().Info("closing a silent tunnel", "target", request.Host, "silent", silent.Round(time.Second))
				return
			}
		}
	}
}
