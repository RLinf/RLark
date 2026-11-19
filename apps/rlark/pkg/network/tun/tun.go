package tun

import (
	"context"
	"fmt"
	"net"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
	"github.com/vishvananda/netlink"
)

// tunClient 管理一个本地 TUN 设备及其与 gVisor netstack 之间的双向数据转发。
//
// 工作流程：
//  1. 创建 TUN 设备并配置 IP/MTU/路由
//  2. 将 TUN 数据包直接注入 gVisor channel endpoint
//  3. 将 gVisor 输出包直接写回 TUN
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
//  3. 启动 gVisor netstack 并直接桥接 TUN 数据包
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

	ns := newNetstack(tc.ip, tc.mtu, tc.dialProxy, tc.queryParams)
	// 设置 TUN 写入回调：ICMP Echo Reply 直接写入 TUN 设备（绕过 gVisor）
	ns.writeToTUN = func(data []byte) error {
		_, err := iface.Write(data)
		return err
	}
	if err := ns.run(ctx, iface); err != nil && ctx.Err() == nil {
		return fmt.Errorf("run netstack: %w", err)
	}
	logger.Info("Client shutdown complete")
	return nil
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
