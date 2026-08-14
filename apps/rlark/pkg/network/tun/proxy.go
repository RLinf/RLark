package tun

import (
	"io"
	"net"

	"gvisor.dev/gvisor/pkg/tcpip/header"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// Proxy 运行在服务端，接收客户端（TUN + gVisor）转发的 TCP/UDP/ICMP 流量，
// 并通过真实的网络 socket 将流量发送到目标服务器。
//
// 工作流程：
//
//	                    Proxy
//	client TCP ──► handleTCP ──► target TCP
//	client TCP ──► handleUDP ──► target UDP
//	client TCP ──► handleICMP ──► target ICMP (Raw Socket)
//
// 客户端通过同一条 TCP 连接发送帧封装的协议数据，Proxy 解帧后通过对应的
// 真实网络协议进行转发，回复数据同样帧封装后沿 TCP 连接返回。
type Proxy struct{}

// NewProxy creates a new Proxy.
func NewProxy() *Proxy {
	return &Proxy{}
}

// Serve 开始监听并接受客户端的 TCP 连接。
//
// 每个连接在独立的 goroutine 中处理，互不干扰。
func (p *Proxy) Serve(listener net.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}
		go p.handleConnection(utils.NewWrapConn(conn))
	}
}

// handleConnection 处理单条客户端连接的生命周期。
//
// 首先从连接中读取目标信息（network、host、port），然后根据协议类型
// 分发到对应的处理函数。连接会在函数返回时自动关闭。
func (p *Proxy) handleConnection(conn *utils.WrapConn) {
	logger := log.GetLogger()
	defer func() { _ = conn.Close() }()

	network, host, port, _, err := utils.ReadTargetFromConn(conn)
	if err != nil {
		logger.Error(nil, "Failed to read target from connection", "err", err)
		return
	}
	switch network {
	case "tcp", "tcp4", "tcp6":
		logger.V(1).Info("Handling TCP proxy connection", "host", host, "port", port)
		p.handleTCP(conn, host, port)
	case "udp", "udp4", "udp6":
		logger.V(1).Info("Handling UDP proxy connection", "host", host, "port", port)
		p.handleUDP(conn, host, port)
	case "icmp", "icmp4":
		logger.V(1).Info("Handling ICMP proxy connection", "host", host)
		p.handleICMP(conn, host)
	default:
		logger.Error(nil, "Handling proxy: unsupported network protocol", "network", network)
	}
}

// handleTCP 在客户端 TCP 连接和目标 TCP 地址之间做双向数据转发。
//
// 使用两个 io.Copy 实现全双工转发：
//   - 一个 goroutine 负责 client → target
//   - 主 goroutine 负责 target → client
//
// 当任意方向拷贝结束，函数返回，defer 关闭的 target 连接会终止另一侧的 goroutine。
func (p *Proxy) handleTCP(conn *utils.WrapConn, host, port string) {
	logger := log.GetLogger()
	tunConn, err := net.Dial("tcp", net.JoinHostPort(host, port))
	if err != nil {
		logger.Error(nil, "Failed to connect to tcp target", "host", host, "port", port, "err", err)
		return
	}
	defer func() { _ = tunConn.Close() }()

	go func() {
		_, _ = io.Copy(tunConn, conn)
	}()
	_, _ = io.Copy(conn, tunConn)
}

// handleUDP 在客户端 TCP 连接和目标 UDP 地址之间做双向帧转发。
//
// 帧格式同 SendPacket/RecvPacket：[2 字节大端长度头][UDP 数据报]。
//
// 防泄漏机制：当 gVisor 端 goroutine 退出时关闭 udpConn 以终止主循环的
// udpConn.Read 阻塞；当主循环退出时函数返回，defer 关闭 conn，
// 从而终止 gVisor 端 goroutine 对 RecvPacket 的阻塞等待。
func (p *Proxy) handleUDP(conn *utils.WrapConn, host, port string) {
	logger := log.GetLogger()
	raddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
	if err != nil {
		logger.Error(nil, "Failed to resolve udp target", "host", host, "port", port, "err", err)
		return
	}
	udpConn, err := net.DialUDP("udp", nil, raddr)
	if err != nil {
		logger.Error(nil, "Failed to dial udp target", "host", host, "port", port, "err", err)
		return
	}
	defer func() { _ = udpConn.Close() }()

	// 读 TCP 帧 → 发 UDP 数据报
	go func() {
		defer func() { _ = udpConn.Close() }() // 关闭 UDP 以终止主循环的 Read 阻塞
		for {
			data, err := RecvPacket(conn)
			if err != nil {
				return
			}
			if data == nil {
				continue
			}
			if _, err := udpConn.Write(data); err != nil {
				return
			}
		}
	}()

	// 收 UDP 数据报 → 写 TCP 帧
	buf := make([]byte, 65535)
	for {
		n, err := udpConn.Read(buf)
		if err != nil {
			return
		}
		if err := SendPacket(conn, buf[:n]); err != nil {
			return
		}
	}
}

// handleICMP 在客户端 TCP 连接和目标主机之间做 ICMP Echo 帧转发。
//
// 使用原始 IP socket（ip4:icmp）发送和接收 ICMP 消息，需要 CAP_NET_RAW 或 root 权限。
// 帧格式同 SendPacket/RecvPacket：[2 字节大端长度头][ICMP 消息]。
//
// 防泄漏机制：当任一端 I/O 失败时，关闭对应 socket 以终止另一侧的阻塞等待。
func (p *Proxy) handleICMP(conn *utils.WrapConn, dst string) {
	logger := log.GetLogger()
	dstAddr, err := net.ResolveIPAddr("ip4", dst)
	if err != nil {
		logger.Error(nil, "Failed to resolve icmp target", "dst", dst, "err", err)
		return
	}
	icmpConn, err := net.DialIP("ip4:icmp", nil, dstAddr)
	if err != nil {
		logger.Error(nil, "Failed to dial icmp target", "dst", dst, "err", err)
		return
	}
	defer func() { _ = icmpConn.Close() }()

	// 读 TCP 帧 → 发 ICMP Echo Request
	go func() {
		defer func() { _ = icmpConn.Close() }() // 关闭 ICMP socket 以终止主循环的 Read 阻塞
		for {
			data, err := RecvPacket(conn)
			if err != nil {
				return
			}
			if data == nil {
				continue
			}
			if _, err := icmpConn.Write(data); err != nil {
				return
			}
		}
	}()

	// 收 ICMP Echo Reply → 写 TCP 帧
	buf := make([]byte, 1500)
	for {
		n, err := icmpConn.Read(buf)
		if err != nil {
			return
		}
		if n < header.IPv4MinimumSize+header.ICMPv4MinimumSize {
			continue
		}
		echoType := buf[header.IPv4MinimumSize]
		if echoType != 0 { // 仅转发 Echo Reply（类型 0），忽略其他 ICMP 消息
			continue
		}
		logger.V(1).Info("Received ICMP Echo Reply", "dst", dst, "len", n)
		if err := SendPacket(conn, buf[:n]); err != nil {
			return
		}
	}
}
