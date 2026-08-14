package tun

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"sync"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/icmp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

// netstack 管理一个 gVisor 用户态 TCP/IP 协议栈实例。
//
// 该协议栈作为虚拟机和远端 Proxy 之间的中间层：
// - 接收来自 TUN 设备（经 net.Pipe）的 IP 包
// - 在协议栈内完成 TCP/UDP/ICMP 协议解析
// - 通过 dialer 回调建立到 Proxy 的 TCP 连接，转发原始流量
// - Proxy 发回的响应经由协议栈重组为 IP 包写回 TUN 设备
//
// 本质上，这是以 gVisor 协议栈作为用户态网络栈，将所有虚拟机的网络请求
// 劫持并通过 TCP 隧道转发到远端 Proxy 去处理。
type netstack struct {
	// ip 是分配给此协议栈的虚拟 IP 地址（对应 TUN 设备的 IP）。
	ip net.IP
	// mtu 是协议栈的最大传输单元，通常与 TUN 设备的 MTU 一致。
	mtu int
	// dialProxy 用于创建到 Proxy 的 TCP 连接。
	dialProxy utils.Dial
	// queryParams 用于在建立到 Proxy 的连接时附加参数
	queryParams map[string]string
	// writeToTUN 是向 TUN 设备写入原始 IP 包的回调函数。
	// 用于 ICMP Echo Reply：回复包必须直接写入 TUN 设备（绕过 gVisor 协议栈），
	// 因为注入到 gVisor 内部会导致 gVisor 将其作为本地包消费（dst 是协议栈自身 IP），
	// 而无法到达对端 VM。
	writeToTUN func([]byte) error
}

func newNetstack(ip net.IP, mtu int, dialProxy utils.Dial, queryParams map[string]string) *netstack {
	ns := &netstack{
		ip:          ip,
		mtu:         mtu,
		dialProxy:   dialProxy,
		queryParams: queryParams,
	}
	if ns.dialProxy == nil {
		// 默认 dialProxy 实现：连接到本地 5700 端口的 Proxy
		ns.dialProxy = func(ctx context.Context) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", "127.0.0.1:5700")
		}
	}
	return ns
}

// ipaddr 将 net.IP 转换为 gVisor 协议栈所需的 [4]byte 格式。
func (ns *netstack) ipaddr() [4]byte {
	var ip [4]byte
	copy(ip[:], ns.ip.To4())
	return ip
}

// handleTunnel 为一个 TCP 隧道连接创建独立的 gVisor 协议栈实例。
//
// 每个隧道连接（对应一个 TUN 设备）拥有独立的协议栈，包含：
// - IPv4 网络层
// - TCP、UDP、ICMP 传输层
// - channel endpoint 作为链路层接口
// - 完整的协议转发器（Forwarder）
//
// 数据传输在一个双向转发循环中完成：隧道 → gVisor（handleRecv）和
// gVisor → 隧道（handleSend），通过 sync.WaitGroup 同步退出。
func (ns *netstack) handleTunnel(tunnelConn net.Conn) error {
	defer func() { _ = tunnelConn.Close() }()

	// ─── 1. 创建 gVisor 协议栈 ───
	s := stack.New(stack.Options{
		NetworkProtocols: []stack.NetworkProtocolFactory{
			ipv4.NewProtocol,
		},
		TransportProtocols: []stack.TransportProtocolFactory{
			tcp.NewProtocol,
			udp.NewProtocol,
			icmp.NewProtocol4,
		},
	})

	// ─── 2. 创建通道链路层端点 ───
	// 这是 gVisor 和外部世界（TCP 隧道）之间的桥梁。
	// 队列深度 1024，MTU 与协议栈一致。
	ep := channel.New(1024, uint32(ns.mtu), "")
	defer ep.Close()

	// ─── 3. 创建 NIC ───
	nicID := s.NextNICID()
	if err := s.CreateNIC(nicID, ep); err != nil {
		return fmt.Errorf("creating nic: %v", err)
	}
	// 开启混杂模式以接收目标不是本机的包
	if err := s.SetPromiscuousMode(nicID, true); err != nil {
		return fmt.Errorf("setting promiscuous mode: %v", err)
	}
	// 开启欺骗模式以允许发送源地址不是本机地址的包
	if err := s.SetSpoofing(nicID, true); err != nil {
		return fmt.Errorf("setting spoofing: %v", err)
	}

	// ─── 4. 添加虚拟 IP 地址 ───
	if err := s.AddProtocolAddress(nicID, tcpip.ProtocolAddress{
		Protocol: ipv4.ProtocolNumber,
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   tcpip.AddrFrom4(ns.ipaddr()),
			PrefixLen: 24,
		},
	}, stack.AddressProperties{}); err != nil {
		return fmt.Errorf("adding protocol address: %v", err)
	}

	// ─── 5. 设置路由表：所有流量走本 NIC ───
	s.SetRouteTable([]tcpip.Route{
		{
			Destination: header.IPv4EmptySubnet,
			NIC:         nicID,
		},
	})

	// ─── 6. 注册传输层协议处理器 ───
	// 这些处理器将协议栈内的 TCP/UDP/ICMP 流量劫持并通过 dialer 转发到远端 Proxy。
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, ns.getTCPHandler(s))
	s.SetTransportProtocolHandler(udp.ProtocolNumber, ns.getUDPHandler(s))
	s.SetTransportProtocolHandler(icmp.ProtocolNumber4, ns.getICMPHandler(s, ep))

	// ─── 7. 启动双向数据传输 ───
	var wg sync.WaitGroup
	wg.Add(2)
	// 隧道 → gVisor：从 TCP 隧道读取 IP 包，注入 gVisor 协议栈
	go ns.handleRecv(tunnelConn, ep, &wg)
	// gVisor → 隧道：从 gVisor 协议栈读取 IP 包，通过隧道发回客户端
	go ns.handleSend(ep, tunnelConn, &wg)

	// ─── 8. 等待退出信号 ───
	wg.Wait()
	return nil
}

// handleRecv 从 TCP 隧道读取帧封装的 IP 包并注入到 gVisor 协议栈。
//
// 这是远端 → 本地虚拟机的方向：远端 Proxy 返回的数据经由 TCP 隧道，
// 在此函数中被还原为 IP 包并注入 gVisor 栈，最终由虚拟机接收。
func (ns *netstack) handleRecv(tunnelConn net.Conn, ep *channel.Endpoint, wg *sync.WaitGroup) {
	logger := log.GetLogger()
	defer wg.Done()
	for {
		data, err := RecvPacket(tunnelConn)
		if err != nil {
			logger.Error(nil, "Failed to receive packet from tunnel", "err", err)
			return
		}
		// 最小 IPv4 头长度为 20 字节，不足则丢弃
		if len(data) < header.IPv4MinimumSize {
			continue
		}

		// 解析 IP 版本号（高 4 位），当前仅支持 IPv4
		version := data[0] >> 4
		switch version {
		case 4:
			pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
				Payload: buffer.MakeWithData(data),
			})
			ep.InjectInbound(ipv4.ProtocolNumber, pkt)
		default:
			// ignore unsupported versions
		}
	}
}

// handleSend 从 gVisor 协议栈的通道链路层读取 IP 包并通过 TCP 隧道发回。
//
// 这是本地虚拟机 → 远端的方向：虚拟机发出的 IP 包经 gVisor 协议栈处理，
// 未匹配地址的包被路由到此函数，帧封装后通过 TCP 隧道发往远端 Proxy。
func (ns *netstack) handleSend(ep *channel.Endpoint, tunnelConn net.Conn, wg *sync.WaitGroup) {
	logger := log.GetLogger()
	defer wg.Done()
	for {
		// 从通道链路层读取出去的 IP 包
		pkt := ep.Read()
		if pkt == nil {
			continue
		}

		// 收集所有 buffer 中的数据
		pktBuf := pkt.ToBuffer()
		data := pktBuf.Flatten()
		if len(data) == 0 {
			continue
		}

		// 通过隧道发回客户端
		if err := SendPacket(tunnelConn, data); err != nil {
			logger.Error(nil, "Failed to send packet to tunnel", "err", err)
			return
		}
	}
}

func (ns *netstack) dial(ctx context.Context, network string, srcIP tcpip.Address, srcPort uint16, dstIP tcpip.Address, dstPort uint16) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	logger := log.GetLogger()
	logger.V(1).Info("Dialing proxy", "network", network, "src", net.JoinHostPort(srcIP.String(), fmt.Sprint(srcPort)), "dst", net.JoinHostPort(dstIP.String(), fmt.Sprint(dstPort)))
	realConn, err := ns.dialProxy(ctx)
	if err != nil {
		return nil, fmt.Errorf("dialing proxy for target %s: %w", dstIP.String(), err)
	}
	logger.V(1).Info("Connected to proxy", "network", network, "src", net.JoinHostPort(srcIP.String(), fmt.Sprint(srcPort)), "dst", net.JoinHostPort(dstIP.String(), fmt.Sprint(dstPort)))

	var src, dst string
	if srcPort > 0 {
		src = net.JoinHostPort(srcIP.String(), fmt.Sprint(srcPort))
	} else {
		src = srcIP.String()
	}
	if dstPort > 0 {
		dst = net.JoinHostPort(dstIP.String(), fmt.Sprint(dstPort))
	} else {
		dst = dstIP.String()
	}

	query := url.Values{}
	for k, v := range ns.queryParams {
		query.Set(k, v)
	}
	query.Set("source", src)
	targetUrl := &url.URL{
		Scheme:   network,
		Host:     dst,
		RawQuery: query.Encode(),
	}
	if _, err := realConn.Write([]byte(targetUrl.String() + "\n")); err != nil {
		_ = realConn.Close()
		return nil, fmt.Errorf("writing target to proxy for %s: %w", dst, err)
	}
	logger.V(1).Info("Sent target to proxy", "network", network, "src", src, "dst", dst, "query", query.Encode())
	return realConn, nil
}
