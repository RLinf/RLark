package tun

import (
	"context"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"
)

// getTCPHandler 返回 TCP 连接转发处理函数。
//
// 使用 gVisor 的 tcp.NewForwarder 拦截所有向本协议栈发起的 TCP 连接请求。
// 对每个虚拟的（源 IP:端口 → 目标 IP:端口）TCP 连接：
//  1. 在 gVisor 栈内创建 TCP endpoint 并完成握手
//  2. 通过 ns.tcpDialer 连接到 Proxy
//  3. 在 gVisor TCP 连接和 Proxy TCP 连接之间双向拷贝数据
//
// 设置 TCP Keepalive 以防止闲置连接被中间设备断开，
// 同时同步设置收发缓冲区大小以匹配协议栈默认值。
func (ns *netstack) getTCPHandler(s *stack.Stack) func(id stack.TransportEndpointID, pkt *stack.PacketBuffer) bool {
	tcpForwarder := tcp.NewForwarder(s, 65536, 65536, func(r *tcp.ForwarderRequest) {
		id := r.ID()
		srcIP := id.RemoteAddress
		srcPort := id.RemotePort
		dstIP := id.LocalAddress
		dstPort := id.LocalPort

		var wq waiter.Queue
		ep, err := r.CreateEndpoint(&wq)
		if err != nil {
			// CreateEndpoint 失败时发送 RST 以清理半开连接
			r.Complete(true)
			return
		}
		// 设置 TCP 连接参数（Keepalive、缓冲区大小）
		if err := setSocketOptions(s, ep); err != nil {
			ep.Close()
			r.Complete(true)
			return
		}
		conn := gonet.NewTCPConn(&wq, ep)
		r.Complete(false)

		go func() {
			defer func() { _ = conn.Close() }()

			logger := log.GetLogger()
			logger.V(1).Info("Forwarding TCP", "srcIP", srcIP, "srcPort", srcPort, "dstIP", dstIP, "dstPort", dstPort)
			realConn, derr := ns.dial(context.Background(), "tcp", srcIP, srcPort, dstIP, dstPort)
			if derr != nil {
				logger.Error(nil, "Failed to dial TCP", "dstIP", dstIP, "dstPort", dstPort, "err", derr)
				ep.Close()
				return
			}
			defer func() { _ = realConn.Close() }()

			// 在 gVisor TCP 连接和 Proxy TCP 连接之间全双工转发
			ns.handleTCPConnection(conn, realConn)
		}()
	})
	return tcpForwarder.HandlePacket
}

// handleTCPConnection 在 gVisor TCP 连接和 Proxy TCP 连接之间双向转发数据。
func (ns *netstack) handleTCPConnection(local, remote net.Conn) {
	logger := log.GetLogger()
	var wg sync.WaitGroup
	wg.Add(2)

	// local(gVisor) → remote(Proxy)
	go func() {
		defer wg.Done()
		if _, err := io.Copy(remote, local); err != nil {
			logger.Error(nil, "Error copying from gVisor to Proxy", "err", err)
		}
	}()

	// remote(Proxy) → local(gVisor)
	go func() {
		defer wg.Done()
		if _, err := io.Copy(local, remote); err != nil {
			logger.Error(nil, "Error copying from Proxy to gVisor", "err", err)
		}
	}()

	wg.Wait()
}

// setSocketOptions 为 TCP endpoint 设置 Socket 选项。
//
// 配置项包括：
// - TCP Keepalive（空闲 60s 后开始探测，间隔 30s，最多 32 次）
// - 发送/接收缓冲区大小（从协议栈的默认配置中读取）.
func setSocketOptions(s *stack.Stack, ep tcpip.Endpoint) tcpip.Error {
	{ /* TCP keepalive 选项 */
		ep.SocketOptions().SetKeepAlive(true)

		idle := tcpip.KeepaliveIdleOption(60 * time.Second)
		if err := ep.SetSockOpt(&idle); err != nil {
			return err
		}

		interval := tcpip.KeepaliveIntervalOption(30 * time.Second)
		if err := ep.SetSockOpt(&interval); err != nil {
			return err
		}

		if err := ep.SetSockOptInt(tcpip.KeepaliveCountOption, 32); err != nil {
			return err
		}
	}
	{ /* TCP 收发缓冲区大小 */
		var ss tcpip.TCPSendBufferSizeRangeOption
		if err := s.TransportProtocolOption(header.TCPProtocolNumber, &ss); err == nil {
			ep.SocketOptions().SetSendBufferSize(int64(ss.Default), false)
		}

		var rs tcpip.TCPReceiveBufferSizeRangeOption
		if err := s.TransportProtocolOption(header.TCPProtocolNumber, &rs); err == nil {
			ep.SocketOptions().SetReceiveBufferSize(int64(rs.Default), false)
		}
	}
	return nil
}
