package tun

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
)

// tunClient 管理一个本地 TUN 设备及其与 gVisor netstack 之间的双向数据转发。
//
// 工作流程：
//  1. 创建 TUN 设备并配置 IP/MTU/路由
//  2. 通过 net.Pipe() 与 netstack 建立本地连接
//  3. 双方向转发：
//     TUN → net.Pipe → gVisor 协议栈（收自物理网络的 IP 包）
//     gVisor 协议栈 → net.Pipe → TUN（发往物理网络的 IP 包）
//  4. gVisor 协议栈将虚拟机的 IP 流量通过 tcpDialer/udpDialer/icmpDialer 转发到远端 Proxy
type tunClient struct {
	// name 是 TUN 设备名称（如 "tun0"），空字符串则由系统自动分配。
	name string
	// ip 是分配给 TUN 设备的虚拟 IP 地址。
	ip net.IP
	// prefixLength 是 IP 地址的子网前缀长度（如 24 对应 255.255.255.0）。
	prefixLength int
	// mtu 是 TUN 设备的 MTU 值（通常为 1500）。
	mtu int

	dialProxy   utils.Dial
	queryParams map[string]string
}

// NewTunClient creates a new TunClient.
func NewTunClient(name string, ip net.IP, prefixLength int, mtu int, dialProxy utils.Dial, queryParams map[string]string) *tunClient {
	return &tunClient{
		name:         name,
		ip:           ip,
		prefixLength: prefixLength,
		mtu:          mtu,
		dialProxy:    dialProxy,
		queryParams:  queryParams,
	}
}

// Run 启动 TUN 客户端，包含以下阶段：
//
//  1. 创建 TUN 设备
//  2. 配置 IP/路由/MTU
//  3. 通过 net.Pipe() 连接 gVisor netstack
//  4. 启动双向转发 goroutine（TUN ↔ gVisor）
//  5. 等待退出信号（SIGINT/SIGTERM）或转发结束
func (tc *tunClient) Run(ctx context.Context) error {
	logger := log.FromContext(ctx)
	// ─── 1. 创建 TUN 设备 ───
	if tc.mtu <= 0 {
		tc.mtu = 1500
	}
	iface, err := newWater(tc.name)
	if err != nil {
		return fmt.Errorf("create TUN device: %w", err)
	}
	defer func() { _ = iface.Close() }()
	logger.Info("TUN device created", "name", iface.Name())

	// ─── 2. 配置 TUN 设备的 IP 和路由 ───
	if err := tc.setupTUN(iface.Name()); err != nil {
		return fmt.Errorf("setup TUN device: %w", err)
	}

	// ─── 3. 通过 net.Pipe() 连接到 netstack ───
	// 使用内存管道模拟 TUN 设备和 gVisor 栈之间的链路层
	c1, c2 := net.Pipe()
	ns := newNetstack(tc.ip, tc.mtu, tc.dialProxy, tc.queryParams)
	// 设置 TUN 写入回调：ICMP Echo Reply 直接写入 TUN 设备（绕过 gVisor）
	ns.writeToTUN = func(data []byte) error {
		_, err := iface.Write(data)
		return err
	}
	go func() {
		err := ns.handleTunnel(c1)
		if err != nil {
			logger.Error(nil, "Failed to handle tunnel connection", "err", err)
		}
	}()

	// ─── 4. 启动双向转发 ───
	// TUN → gVisor：从 TUN 读 IP 包 → SendPacket 写入管道
	// gVisor → TUN：RecvPacket 从管道读 → 写入 TUN
	var wg sync.WaitGroup
	wg.Add(2)
	go tc.handleRead(iface, c2, &wg)
	go tc.handleWrite(c2, iface, &wg)

	// ─── 5. 等待退出信号 ───
	select {
	case <-ctx.Done():
		logger.Info("Context cancelled, shutting down...")
		_ = c2.Close()
		_ = iface.Close()
	case <-waitDone(&wg):
		// 自然退出
	}
	wg.Wait()
	logger.Info("Client shutdown complete")
	return nil
}

// waitDone 返回一个 channel，当 WaitGroup 计数归零时关闭。
func waitDone(wg *sync.WaitGroup) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		wg.Wait()
		close(ch)
	}()
	return ch
}

// setupTUN 通过 netlink 配置 TUN 设备的 IP 地址、MTU 并启用设备。
func (tc *tunClient) setupTUN(name string) error {
	link, err := netlink.LinkByName(name)
	if err != nil {
		return fmt.Errorf("tun device not found: %w", err)
	}

	// 设置 IP 地址
	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   tc.ip,
			Mask: net.CIDRMask(tc.prefixLength, 32),
		},
	}
	if err := netlink.AddrAdd(link, addr); err != nil {
		return fmt.Errorf("add ip address: %w", err)
	}

	// 设置 MTU
	if err := netlink.LinkSetMTU(link, tc.mtu); err != nil {
		return fmt.Errorf("set mtu: %w", err)
	}

	// 启动 TUN 设备
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("bring up tun device: %w", err)
	}
	return nil
}

// handleRead 从 TUN 设备读取 IP 包并通过管道发送给 gVisor 协议栈。
//
// 这是物理网络 → 虚拟网络的方向。
func (tc *tunClient) handleRead(iface *water.Interface, tunnelConn net.Conn, wg *sync.WaitGroup) {
	logger := log.GetLogger()
	defer wg.Done()
	buf := make([]byte, tc.mtu)
	for {
		n, err := iface.Read(buf)
		if err != nil {
			logger.Error(nil, "Failed to read from TUN device", "err", err)
			return
		}
		if n == 0 {
			continue
		}

		// 通过 TCP 隧道发送 IP 包
		if err := SendPacket(tunnelConn, buf[:n]); err != nil {
			logger.Error(nil, "Failed to send IP packet", "err", err)
			return
		}
	}
}

// handleWrite 从 gVisor 协议栈接收 IP 包（通过管道）并写入 TUN 设备。
//
// 这是虚拟网络 → 物理网络的方向。
func (tc *tunClient) handleWrite(tunnelConn net.Conn, iface *water.Interface, wg *sync.WaitGroup) {
	logger := log.GetLogger()
	defer wg.Done()
	for {
		data, err := RecvPacket(tunnelConn)
		if err != nil {
			logger.Error(nil, "Failed to receive IP packet", "err", err)
			return
		}
		if data == nil {
			continue
		}

		// 写入 TUN 设备
		if _, err := iface.Write(data); err != nil {
			logger.Error(nil, "Failed to write to TUN device", "err", err)
			return
		}
	}
}
