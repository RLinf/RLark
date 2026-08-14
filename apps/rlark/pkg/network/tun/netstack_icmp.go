package tun

import (
	"context"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

// getICMPHandler 返回 ICMP Echo Request（Ping）转发处理函数。
//
// 对每个 Echo Request 的处理流程：
//  1. 通过 ns.icmpDialer 创建 TCP 连接到 Proxy，发送 "icmp://host"
//  2. 将 Echo Request 包（ICMP 头 + payload）帧封装后发送到 Proxy
//  3. Proxy 通过原始 ICMP socket 转发到真实目标并等待 Echo Reply
//  4. Proxy 将 Echo Reply 帧封装后回复
//  5. 本端收到后构造完整 IPv4+ICMP 包，通过 ns.writeToTUN 直接写入 TUN 设备
//
// 注意：回复包不经过 gVisor 协议栈内部。因为 gVisor 与 VM 共享同一个 IP 地址，
// 若注入到 gVisor（InjectInbound），gVisor 会将 dst=自身IP 的包作为本地包消费，
// 而 gVisor 内部没有 ICMP transport endpoint 等待这个回复（ping 运行在 VM 上），
// 导致回复包被丢弃，ping 永远收不到回包。
//
// 非 Echo Request 的其他 ICMP 类型（如 Destination Unreachable）返回 false，
// 交由 gVisor 协议栈默认处理。
//
// 需要 ns.icmpDialer 已配置（通常由 tunClient 在建立时根据配置设定）。
func (ns *netstack) getICMPHandler(_ *stack.Stack, ep *channel.Endpoint) func(id stack.TransportEndpointID, pkt *stack.PacketBuffer) bool {
	return func(id stack.TransportEndpointID, pkt *stack.PacketBuffer) bool {
		// 解析 ICMP 头（8 字节：Type + Code + Checksum + ID + Sequence）
		icmpHdr := header.ICMPv4(pkt.TransportHeader().Slice())
		if len(icmpHdr) < header.ICMPv4MinimumSize {
			return false
		}

		// 只处理 Echo Request（Type 8, Code 0）
		if icmpHdr.Type() != header.ICMPv4Echo || icmpHdr.Code() != 0 {
			return false
		}

		// 获取 IP 头中的源/目的地址
		netHdr := pkt.NetworkHeader().Slice()
		if len(netHdr) < header.IPv4MinimumSize {
			return false
		}
		ipHdr := header.IPv4(netHdr)
		srcAddr := ipHdr.SourceAddress()
		dstAddr := ipHdr.DestinationAddress()

		// 提取 ICMP payload（8 字节头之后的数据，如 ping 的时间戳和标识数据）
		buf := pkt.Data().ToBuffer()
		payload := buf.Flatten()

		// 拷贝完整的 ICMP Echo Request 消息
		// 格式：[Type(1)][Code(1)][Checksum(2)][ID(2)][Sequence(2)][Payload...]
		reqData := make([]byte, header.ICMPv4MinimumSize+len(payload))
		copy(reqData, icmpHdr)
		copy(reqData[header.ICMPv4MinimumSize:], payload)

		go ns.handleICMPEcho(ep, srcAddr, dstAddr, reqData)
		return true
	}
}

// handleICMPEcho 通过 Proxy 转发单个 ICMP Echo Request 并等待 Reply。
//
// 此函数运行在独立的 goroutine 中，通过网络阻塞等待 Echo Reply，
// 收到后构造完整的 IPv4+ICMP 包注入回 gVisor 协议栈，使 ping 命令能收到回复。
func (ns *netstack) handleICMPEcho(ep *channel.Endpoint, srcAddr, dstAddr tcpip.Address, reqData []byte) {
	logger := log.GetLogger()
	logger.V(1).Info("Forwarding ICMP Echo", "src", srcAddr.String(), "dst", dstAddr.String())
	realConn, err := ns.dial(context.Background(), "icmp", srcAddr, 0, dstAddr, 0)
	if err != nil {
		logger.Error(nil, "Failed to dial ICMP", "dst", dstAddr.String(), "err", err)
		return
	}
	defer func() { _ = realConn.Close() }()

	// 将 Echo Request 帧封装后发送到 Proxy
	if err := SendPacket(realConn, reqData); err != nil {
		logger.Error(nil, "Failed to send ICMP Echo Request to proxy", "dst", dstAddr.String(), "err", err)
		return
	}

	// 读取 Echo Reply 帧
	var icmpPayload []byte
	for {
		replyData, err := RecvPacket(realConn)
		if err != nil {
			logger.Error(nil, "Failed to receive ICMP Echo Reply from proxy", "dst", dstAddr.String(), "err", err)
			return
		}
		if len(replyData) < header.ICMPv4MinimumSize {
			continue
		}
		icmpPayload = replyData

		// Proxy 可能返回纯 ICMP 或 IPv4+ICMP，自动剥离 IP 头
		replyHdr := header.ICMPv4(icmpPayload)
		if replyHdr.Type() == 0x45 || (len(icmpPayload) >= header.IPv4MinimumSize && header.IPv4(icmpPayload).IsValid(len(icmpPayload))) {
			// replyData 以 IP 头开头（type=0x45 = IPv4 版本+头长），剥离 IP 头
			ipHdr := header.IPv4(icmpPayload)
			hdrLen := int(ipHdr.HeaderLength())
			if hdrLen < header.IPv4MinimumSize || hdrLen >= len(icmpPayload) {
				continue
			}
			icmpPayload = icmpPayload[hdrLen:]
			replyHdr = header.ICMPv4(icmpPayload)
		}
		if len(icmpPayload) < header.ICMPv4MinimumSize {
			continue
		}

		// 非 Echo Reply（Type 0, Code 0）继续等待，直到收到正确的回复
		if replyHdr.Type() != header.ICMPv4EchoReply || replyHdr.Code() != 0 {
			continue
		}
		break
	}

	totalLen := header.IPv4MinimumSize + len(icmpPayload)
	ipBuf := make([]byte, totalLen)

	ipOut := header.IPv4(ipBuf)
	ipOut.Encode(&header.IPv4Fields{
		TotalLength: uint16(totalLen),
		TTL:         64,
		Protocol:    uint8(header.ICMPv4ProtocolNumber),
		SrcAddr:     dstAddr, // Echo Reply 的源 = 原始目标地址
		DstAddr:     srcAddr, // Echo Reply 的目的 = 原始源地址（虚拟机虚拟 IP）
	})
	// 计算并设置 IPv4 头校验和（必须，否则 gVisor 协议栈会丢弃注入的包）
	ipOut.SetChecksum(0)
	rawSum := ipOut.CalculateChecksum()
	ipv4Checksum := ^rawSum
	ipOut.SetChecksum(ipv4Checksum)

	copy(ipBuf[header.IPv4MinimumSize:], icmpPayload)

	if ns.writeToTUN != nil {
		// ★ 直接写入 TUN 设备（绕过 gVisor），使 Echo Reply 到达 VM
		if err := ns.writeToTUN(ipBuf); err != nil {
			logger.Error(nil, "Failed to write ICMP Echo Reply to TUN", "dst", dstAddr.String(), "err", err)
		}
	} else {
		// Fallback：注入 gVisor 协议栈（通常不可用，因为 gVisor 会将包作为本地包消费）
		injectPkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
			Payload: buffer.MakeWithData(ipBuf),
		})
		ep.InjectInbound(ipv4.ProtocolNumber, injectPkt)
	}
}
