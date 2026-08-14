package sidecar

import (
	"time"

	"github.com/spf13/pflag"
)

// Config 配置 sidecar 服务。
type Config struct {
	// UnixSocketAddress 是 NodeServer 监听的 Unix socket 路径。
	// TUN client 通过此 socket 连接到本节点的 NodeServer 进行出站流量转发。
	UnixSocketAddress string

	// TunName 是 TUN 设备名称，留空由系统自动分配。
	TunName string

	// TunMTU 是 TUN 设备的 MTU。
	TunMTU int

	// ProxyListenAddress 是 Proxy 的 TCP 监听地址。
	// 其他 Pod 的 NodeServer 通过此地址连接到本 Pod 的 Proxy 进行入站流量转发。
	// 格式示例：":5700" 或 "0.0.0.0:5700"
	ProxyListenAddress string

	// HostsSyncEnabled 控制是否定期从 NodeServer 同步 hosts 到本地 hosts 文件。
	HostsSyncEnabled bool

	// HostsSyncInterval 是两次 hosts 同步之间的间隔。
	HostsSyncInterval time.Duration

	// HostsFile 是需要更新的 hosts 文件路径。
	HostsFile string
}

// DefaultConfig returns the default config.
func DefaultConfig() Config {
	return Config{
		UnixSocketAddress:  "/var/run/rlark/nodeserver.sock",
		TunName:            "gnet0",
		TunMTU:             1500,
		ProxyListenAddress: ":5700",
		HostsSyncEnabled:   true,
		HostsSyncInterval:  30 * time.Second,
		HostsFile:          "/etc/hosts",
	}
}

// SetupFlags sets the upFlags.
func (c *Config) SetupFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.UnixSocketAddress, "sidecar-unix-socket", c.UnixSocketAddress, "NodeServer Unix socket path")
	fs.StringVar(&c.TunName, "sidecar-tun-name", c.TunName, "TUN device name (empty=auto)")
	fs.IntVar(&c.TunMTU, "sidecar-tun-mtu", c.TunMTU, "TUN device MTU")
	fs.StringVar(&c.ProxyListenAddress, "sidecar-proxy-listen", c.ProxyListenAddress, "Proxy TCP listen address (for inbound traffic from other pods)")
	fs.BoolVar(&c.HostsSyncEnabled, "sidecar-hosts-sync-enabled", c.HostsSyncEnabled, "Enable periodic hosts file sync from NodeServer")
	fs.DurationVar(&c.HostsSyncInterval, "sidecar-hosts-sync-interval", c.HostsSyncInterval, "Interval between hosts sync attempts")
	fs.StringVar(&c.HostsFile, "sidecar-hosts-file", c.HostsFile, "Path to the hosts file to update")
}
