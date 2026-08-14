package sidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/network/nodeserver"
	"github.com/rlinf/rlark/apps/rlark/pkg/network/tun"
)

// Sidecar 是运行在每个 Pod 中的网络代理 sidecar。
//
// 同时运行两个组件：
//
//  1. Proxy（入站）：监听 TCP 端口，接收其他 Pod 的 NodeServer 转发来的连接，
//     读取目标地址后连接到 Pod 内的真实目标进程。
//
//  2. TUN client（出站）：创建 TUN 设备 + gVisor 协议栈，拦截 Pod 的出站流量，
//     通过 Unix socket 连接到本节点 NodeServer，由 NodeServer 路由到目标 Pod 的 Proxy。
//
// 完整数据流（Pod A 到 Pod B）：
//
//	Pod A 进程
//	  → TUN 设备
//	  → gVisor 协议栈
//	  → dialProxy (Unix socket)
//	  → NodeServer A
//	  → TCP (连接 Pod B 的 Proxy)
//	  → Proxy B
//	  → 真实目标 (Pod B 内进程)
type Sidecar struct {
	config Config

	transport http.RoundTripper
}

// NewSidecar creates a new Sidecar.
func NewSidecar(config Config) *Sidecar {
	return &Sidecar{
		config: config,
		transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, err := net.DialTimeout("unix", config.UnixSocketAddress, 5*time.Second)
				if err != nil {
					return nil, fmt.Errorf("dial nodeserver: %w", err)
				}
				// 写入目标行 0.0.0.0 表示路由到 NodeServer 的本地 Gin 服务
				if _, err := fmt.Fprintf(conn, "tcp://0.0.0.0:0\n"); err != nil {
					_ = conn.Close()
					return nil, fmt.Errorf("write target line: %w", err)
				}
				return conn, nil
			},
		},
	}
}

// Run 启动 sidecar，同时运行 Proxy（入站）和 TUN client（出站）。
//
// 启动顺序：
//  1. 启动时先从 NodeServer 获取本 Pod 的 IP 和子网前缀长度（内建重试）
//  2. 再启动 Proxy 监听（入站连接随时可能到达）
//  3. 再启动 TUN client（出站流量拦截）
//  4. 任一组件退出时，整个 sidecar 退出
func (s *Sidecar) Run(ctx context.Context) error {
	logger := log.FromContext(ctx)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// ─── 1. 从 NodeServer 获取本 Pod 的 IP 与子网前缀 ───
	ipInfo, err := s.waitForPodIPInfo(ctx)
	if err != nil {
		return fmt.Errorf("get pod IP from nodeserver: %w", err)
	}
	ip := net.ParseIP(ipInfo.IP)
	if ip == nil {
		return fmt.Errorf("invalid pod IP from nodeserver: %s", ipInfo.IP)
	}

	logger.Info("Retrieved pod IP info from nodeserver",
		"ip", ipInfo.IP,
		"prefixLength", ipInfo.PrefixLength,
	)

	// ─── 2. 启动 Proxy（入站） ───
	proxyListener, err := net.Listen("tcp", s.config.ProxyListenAddress)
	if err != nil {
		return fmt.Errorf("listen proxy address %s: %w", s.config.ProxyListenAddress, err)
	}
	defer func() { _ = proxyListener.Close() }()

	logger.Info("Proxy listening for inbound connections",
		"address", s.config.ProxyListenAddress,
	)

	proxy := tun.NewProxy()
	proxyErr := make(chan error, 1)
	go func() {
		if err := proxy.Serve(proxyListener); err != nil {
			proxyErr <- fmt.Errorf("proxy serve: %w", err)
		}
	}()

	// ─── 3. 启动 TUN client（出站） ───

	// dialProxy 创建到本节点 NodeServer 的 Unix socket 连接。
	// gVisor 协议栈在拦截到 TCP/UDP/ICMP 连接时调用此函数，
	// 将原始的目标地址信息通过 Unix socket 发送给 NodeServer，
	// 由 NodeServer 路由到目标 Pod 的 Proxy。
	dialProxy := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", s.config.UnixSocketAddress)
	}

	logger.Info("Starting TUN client",
		"tunIP", ipInfo.IP,
		"tunPrefix", ipInfo.PrefixLength,
		"tunMTU", s.config.TunMTU,
		"unixSocket", s.config.UnixSocketAddress,
	)

	tc := tun.NewTunClient(
		s.config.TunName,
		ip,
		ipInfo.PrefixLength,
		s.config.TunMTU,
		dialProxy,
		nil, // queryParams — 由 netstack.dial 自动设置 source
	)

	tunErr := make(chan error, 1)
	go func() {
		if err := tc.Run(ctx); err != nil {
			tunErr <- fmt.Errorf("tun client: %w", err)
		}
	}()

	// ─── 4. 启动 Hosts 同步（定期从 NodeServer 获取 hosts 并更新本地 hosts 文件） ───
	if s.config.HostsSyncEnabled {
		hs := newHostsSyncer(s.transport, s.config.HostsFile, s.config.HostsSyncInterval)
		go func() {
			if err := hs.Run(ctx); err != nil {
				logger.Error(nil, "Hosts syncer stopped", "err", err)
			}
		}()
		logger.Info("Hosts sync started",
			"interval", s.config.HostsSyncInterval,
			"hostsFile", s.config.HostsFile,
		)
	}

	// ─── 5. 等待任一组件退出 ───
	select {
	case err := <-proxyErr:
		logger.Error(nil, "Proxy stopped", "err", err)
		cancel()
		return err
	case err := <-tunErr:
		logger.Error(nil, "TUN client stopped", "err", err)
		cancel()
		return err
	case <-ctx.Done():
		logger.Info("Sidecar shutting down")
		return nil
	}
}

// getPodIPInfo 通过 NodeServer 的 /get_ip 端点获取本 Pod 的 IP 地址和子网前缀长度。
// 通过自定义 DialContext 在 HTTP 握手前写入目标行，使 NodeServer 路由到本地 Gin 服务。
func (s *Sidecar) getPodIPInfo(ctx context.Context) (*nodeserver.PodIPInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/get_ip", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	client := &http.Client{
		Transport: s.transport,
		Timeout:   10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var info nodeserver.PodIPInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &info, nil
}

// waitForPodIPInfo 带指数退避重试地获取 Pod IP 信息，直到成功或 context 取消。
func (s *Sidecar) waitForPodIPInfo(ctx context.Context) (*nodeserver.PodIPInfo, error) {
	logger := log.FromContext(ctx)
	const maxBackoff = 10 * time.Second
	backoff := 500 * time.Millisecond

	for {
		info, err := s.getPodIPInfo(ctx)
		if err == nil {
			return info, nil
		}

		// 检查 ctx 是否已取消，避免无效重试
		if ctx.Err() != nil {
			return nil, fmt.Errorf("context cancelled while waiting for nodeserver: %w", ctx.Err())
		}

		logger.Info("Failed to get pod IP from nodeserver, retrying",
			"err", err,
			"backoff", backoff,
		)

		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
		}

		// 指数退避，上限 maxBackoff
		backoff = time.Duration(math.Min(
			float64(backoff*2),
			float64(maxBackoff),
		))
	}
}
