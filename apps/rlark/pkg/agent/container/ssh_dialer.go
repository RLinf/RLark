package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"golang.org/x/crypto/ssh"
)

// ---------------------------------------------------------------------------
// SSHDialer — 全局 SSH 连接管理器
//
// 设计原则：
//   - 每个 domain 至多维护一个 SSH 连接（ssh.Client 支持多路复用）
//   - 连接断开时自动重连，重连期间并发请求等待而非各自新建
//   - 重连失败指数退避，避免高频重试
//   - 后台 GC 关闭空闲超时的连接
//   - 线程安全，正常路径读锁无阻塞
// ---------------------------------------------------------------------------

const (
	defaultIdleTimeout       = 24 * time.Hour
	defaultCleanupInterval   = 1 * time.Minute
	defaultSSHUser           = "root"
	defaultSSHTimeout        = 10 * time.Second
	defaultKeepaliveInterval = 30 * time.Second
	maxReconnectBackoff      = 30 * time.Second
	initialReconnectBackoff  = 1 * time.Second
)

const keepaliveRequest = "keepalive@openssh.com"

// SSHDialerConfig 配置全局 SSH 连接管理器。
type SSHDialerConfig struct {
	// IdleTimeout 关闭空闲超过此时长的 SSH 连接。零值使用默认值（24 小时）。
	IdleTimeout time.Duration `json:"idleTimeout,omitempty" yaml:"idleTimeout,omitempty"`
	// CleanupInterval 垃圾回收周期。零值使用默认值（1 分钟）。
	CleanupInterval time.Duration `json:"cleanupInterval,omitempty" yaml:"cleanupInterval,omitempty"`
	// SSHUser SSH 登录用户名。空值使用默认值 "root"。
	SSHUser string `json:"sshUser,omitempty" yaml:"sshUser,omitempty"`
	// SSHTimeout SSH 拨号超时。零值使用默认值（10 秒）。
	SSHTimeout time.Duration `json:"sshTimeout,omitempty" yaml:"sshTimeout,omitempty"`
	// InitialReconnectBackoff 首次重连失败后的等待时间。零值使用默认值（1 秒）。
	InitialReconnectBackoff time.Duration `json:"initialReconnectBackoff,omitempty" yaml:"initialReconnectBackoff,omitempty"`
	// MaxReconnectBackoff 重连等待时间的上限。零值使用默认值（30 秒）。
	MaxReconnectBackoff time.Duration `json:"maxReconnectBackoff,omitempty" yaml:"maxReconnectBackoff,omitempty"`
	// KeepaliveInterval 应用层 SSH 保活间隔。零值使用默认值（30 秒）。
	KeepaliveInterval time.Duration `json:"keepaliveInterval,omitempty" yaml:"keepaliveInterval,omitempty"`
	// OnReconnect 重连成功后的回调（用于 metrics 埋点）。可为 nil。
	OnReconnect func(domainID string) `json:"-" yaml:"-"`
	// HostKeyCallback SSH 主机密钥验证回调。nil 时使用 InsecureIgnoreHostKey（仅开发环境）。
	HostKeyCallback ssh.HostKeyCallback `json:"-" yaml:"-"`
}

func (c *SSHDialerConfig) setDefaults() {
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = defaultIdleTimeout
	}
	if c.CleanupInterval <= 0 {
		c.CleanupInterval = defaultCleanupInterval
	}
	if c.SSHUser == "" {
		c.SSHUser = defaultSSHUser
	}
	if c.SSHTimeout <= 0 {
		c.SSHTimeout = defaultSSHTimeout
	}
	if c.InitialReconnectBackoff <= 0 {
		c.InitialReconnectBackoff = initialReconnectBackoff
	}
	if c.MaxReconnectBackoff <= 0 {
		c.MaxReconnectBackoff = maxReconnectBackoff
	}
	if c.KeepaliveInterval <= 0 {
		c.KeepaliveInterval = defaultKeepaliveInterval
	}
	if c.HostKeyCallback == nil {
		c.HostKeyCallback = ssh.InsecureIgnoreHostKey()
	}
}

// domainEntry 管理一个 domain 的 SSH 连接和重连协调。
type domainEntry struct {
	domainID string

	mu       sync.RWMutex
	client   *ssh.Client
	lastUsed time.Time
	broken   bool

	// 重连协调
	reconMu          sync.Mutex
	reconnecting     bool
	reconnectCh      chan struct{}
	lastReconnectAt  time.Time
	reconnectBackoff time.Duration
	maxBackoff       time.Duration

	// keepaliveDone 关闭时通知当前 keepalive goroutine 退出。
	// 每次 finishReconnect 成功时重建,markBroken/close 时关闭。
	keepaliveDone chan struct{}
}

// SSHDialer 提供按 domain 分组的全局 SSH 连接池。
type SSHDialer struct {
	cfg    SSHDialerConfig
	closed atomic.Bool

	mu      sync.RWMutex
	domains map[string]*domainEntry

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSSHDialer 创建并启动 SSHDialer。
func NewSSHDialer(cfg SSHDialerConfig) *SSHDialer {
	cfg.setDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	d := &SSHDialer{
		cfg:     cfg,
		domains: make(map[string]*domainEntry),
		ctx:     ctx,
		cancel:  cancel,
	}
	d.wg.Add(1)
	go d.cleanupLoop()
	return d
}

// touch 刷新 lastUsed,表示该 domain 上有真实的业务数据流动。
// 线程安全,用于 activityConn 在 Read/Write 成功时调用。
func (entry *domainEntry) touch() {
	entry.mu.Lock()
	entry.lastUsed = time.Now()
	entry.mu.Unlock()
}

type activityConn struct {
	net.Conn
	onActivity func()
}

func (c *activityConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.onActivity()
	}
	return n, err
}

func (c *activityConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		c.onActivity()
	}
	return n, err
}

// DialContext 通过 SSH 隧道连接到目标 addr。
func (d *SSHDialer) DialContext(ctx context.Context, domainID, sshAddr, cert, key, addr string) (net.Conn, error) {
	if d.closed.Load() {
		return nil, fmt.Errorf("ssh dialer: closed")
	}

	entry := d.getOrCreate(domainID)
	client, err := entry.borrow(ctx, d, sshAddr, cert, key)
	if err != nil {
		return nil, fmt.Errorf("ssh dialer: %w", err)
	}

	conn, err := client.DialContext(ctx, "tcp", addr)
	if err != nil {
		if isSSHTransportError(err) {
			log.GetLogger().Info("SSH channel dial failed with transport error, marking broken",
				"domain", domainID,
				"target", addr,
				"err", err,
				"errType", fmt.Sprintf("%T", err),
			)
			entry.markBroken("channel-dial-error")
		}
		return nil, fmt.Errorf("ssh proxy to %s: %w", addr, err)
	}

	return &activityConn{
		Conn:       conn,
		onActivity: entry.touch,
	}, nil
}

// Close 关闭所有 SSH 连接并停止后台 GC。
func (d *SSHDialer) Close() error {
	d.closed.Store(true)
	d.cancel()
	d.wg.Wait()

	d.mu.Lock()
	defer d.mu.Unlock()
	for _, entry := range d.domains {
		// 加 reconMu 确保没有 in-flight 的 dialSSH 正在设置新连接
		entry.reconMu.Lock()
		entry.close()
		entry.reconMu.Unlock()
	}
	return nil
}

// Stats 返回活跃的 SSH 连接数。
func (d *SSHDialer) Stats() (open int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, entry := range d.domains {
		entry.mu.RLock()
		if !entry.broken && entry.client != nil {
			open++
		}
		entry.mu.RUnlock()
	}
	return
}

// ===========================================================================
// domain 管理
// ===========================================================================

func (d *SSHDialer) getOrCreate(domainID string) *domainEntry {
	d.mu.RLock()
	entry, ok := d.domains[domainID]
	d.mu.RUnlock()
	if ok {
		return entry
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if entry, ok := d.domains[domainID]; ok {
		return entry
	}
	entry = &domainEntry{
		domainID:   domainID,
		maxBackoff: d.cfg.MaxReconnectBackoff,
	}
	d.domains[domainID] = entry
	return entry
}

// ===========================================================================
// 连接借用
// ===========================================================================

func (entry *domainEntry) borrow(ctx context.Context, d *SSHDialer, sshAddr, cert, key string) (*ssh.Client, error) {
	entry.mu.RLock()
	if !entry.broken && entry.client != nil {
		entry.lastUsed = time.Now()
		client := entry.client
		entry.mu.RUnlock()
		return client, nil
	}
	entry.mu.RUnlock()
	return entry.reconnect(ctx, d, sshAddr, cert, key)
}

// reconnect 协调重连，确保同一时刻只有一个 goroutine 执行 SSH 拨号。
// 返回的 finish 函数必须在拨号完成后调用，以更新状态并通知等待者。
func (entry *domainEntry) reconnect(ctx context.Context, d *SSHDialer, sshAddr, cert, key string) (*ssh.Client, error) {
	entry.reconMu.Lock()
	entry.mu.Lock()

	// 双检：可能有人在我们之前修好了
	if !entry.broken && entry.client != nil {
		entry.lastUsed = time.Now()
		client := entry.client
		entry.mu.Unlock()
		entry.reconMu.Unlock()
		return client, nil
	}

	if entry.reconnecting {
		// 有人已经在拨号，等待结果
		ch := entry.reconnectCh
		entry.mu.Unlock()
		entry.reconMu.Unlock()
		select {
		case <-ch:
			return entry.borrow(ctx, d, sshAddr, cert, key)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// 我就是拨号者
	entry.reconnecting = true
	entry.reconnectCh = make(chan struct{})
	backoff := entry.reconnectBackoff
	entry.mu.Unlock()
	entry.reconMu.Unlock()

	// ---- 退避等待 ----
	if backoff > 0 {
		jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
		wait := backoff + jitter
		select {
		case <-time.After(wait):
			// 退避结束，继续拨号
		case <-ctx.Done():
			entry.finishReconnect(nil, ctx.Err(), d.closed.Load(), d)
			return nil, ctx.Err()
		case <-d.ctx.Done():
			entry.finishReconnect(nil, fmt.Errorf("ssh dialer closed"), true, d)
			return nil, fmt.Errorf("ssh dialer: closed")
		}
	}

	// ---- 执行拨号 ----
	// 合并 caller ctx 和 dialer ctx：dialer 关闭时立即取消拨号
	client, err := d.dialSSHWithMergedCtx(ctx, sshAddr, cert, key)
	entry.finishReconnect(client, err, d.closed.Load(), d)
	if err != nil {
		return nil, fmt.Errorf("ssh reconnect: %w", err)
	}
	return client, nil
}

// finishReconnect 在拨号完成后更新状态并通知等待者。
// dialerClosed 为 true 时，即使拨号成功也丢弃新连接，防止泄漏。
func (entry *domainEntry) finishReconnect(client *ssh.Client, err error, dialerClosed bool, d *SSHDialer) {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	entry.reconnecting = false
	entry.lastReconnectAt = time.Now()

	if err == nil && !dialerClosed {
		// 成功且 dialer 未关闭 → 替换连接
		if entry.client != nil && !entry.broken {
			_ = entry.client.Close()
		}
		entry.client = client
		entry.broken = false
		entry.reconnectBackoff = 0
		// 关闭上一个 keepalive goroutine(如果有),避免泄漏
		if entry.keepaliveDone != nil {
			close(entry.keepaliveDone)
		}
		entry.keepaliveDone = make(chan struct{})
		go entry.keepaliveLoop(client, d.cfg.KeepaliveInterval, entry.keepaliveDone)
		if d.cfg.OnReconnect != nil {
			d.cfg.OnReconnect(entry.domainID)
		}
	} else {
		// 失败或 dialer 已关闭 → 丢弃新连接
		if client != nil {
			_ = client.Close()
		}
		entry.reconnectBackoff = nextBackoff(entry.reconnectBackoff, entry.maxBackoff)
	}
	close(entry.reconnectCh)
}

// nextBackoff 指数退避，上限 maxReconnectBackoff。
func nextBackoff(current time.Duration, max time.Duration) time.Duration {
	if max <= 0 {
		max = maxReconnectBackoff
	}
	if current <= 0 {
		return initialReconnectBackoff
	}
	next := current * 2
	if next > max {
		return max
	}
	return next
}

// markBroken 标记连接为损坏，下次 borrow 触发重连。
func (entry *domainEntry) markBroken(reason string) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.markBrokenLocked(reason)
}

// markBrokenLocked 在持有 entry.mu 的情况下标记连接损坏。
// reason 记录触发关闭的路径(cleanup/keepalive/dial-error/close),便于定位断连根因。
func (entry *domainEntry) markBrokenLocked(reason string) {
	if entry.client == nil {
		return
	}
	log.GetLogger().Info("SSH connection marked broken",
		"domain", entry.domainID,
		"reason", reason,
		"lastUsed", entry.lastUsed,
		"idleFor", time.Since(entry.lastUsed).Round(time.Second),
	)
	if entry.keepaliveDone != nil {
		close(entry.keepaliveDone)
		entry.keepaliveDone = nil
	}
	entry.broken = true
	_ = entry.client.Close()
	entry.client = nil
}

// markBrokenIfCurrent 仅当 client 仍是当前连接时才标记损坏。
// keepalive goroutine 检测失败时，entry 可能已被重连成新 client，
// 防止误标新连接。
func (entry *domainEntry) markBrokenIfCurrent(client *ssh.Client, reason string) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.client == client && client != nil {
		entry.markBrokenLocked(reason)
	}
}

// keepaliveLoop 按 KeepaliveInterval 发送 SSH 应用层保活请求。
// 任一失败（SendRequest 报错或底层连接断开）即标记 broken 并退出。
// 通过 done channel 在连接被替换/关闭时退出,避免 goroutine 泄漏。
func (entry *domainEntry) keepaliveLoop(client *ssh.Client, interval time.Duration, done chan struct{}) {
	logger := log.GetLogger()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if _, _, err := client.SendRequest(keepaliveRequest, true, nil); err != nil {
				// 记录具体错误类型,便于定位断连根因:
				// - i/o timeout: 对端无响应,像会话被中间设备静默丢
				// - connection reset by peer: 被主动 RST,像有设备踢连接
				// - EOF: 对端正常关闭
				logger.Info("SSH keepalive failed, marking connection broken",
					"domain", entry.domainID,
					"err", err,
					"errType", fmt.Sprintf("%T", err),
				)
				entry.markBrokenIfCurrent(client, "keepalive-failed")
				return
			}
		case <-done:
			return
		}
	}
}

// close 关闭连接。
func (entry *domainEntry) close() {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	entry.markBrokenLocked("dialer-close")
}

// ===========================================================================
// 垃圾回收
// ===========================================================================

func (d *SSHDialer) cleanupLoop() {
	defer d.wg.Done()
	ticker := time.NewTicker(d.cfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.cleanup()
		}
	}
}

func (d *SSHDialer) cleanup() {
	cutoff := time.Now().Add(-d.cfg.IdleTimeout)
	d.mu.RLock()
	entries := make([]*domainEntry, 0, len(d.domains))
	for _, entry := range d.domains {
		entries = append(entries, entry)
	}
	d.mu.RUnlock()

	for _, entry := range entries {
		entry.mu.Lock()
		if entry.client != nil && !entry.broken && entry.lastUsed.Before(cutoff) {
			// 走统一关闭逻辑,确保 keepalive goroutine 被通知退出
			entry.markBrokenLocked("idle-cleanup")
		}
		entry.mu.Unlock()
	}
}

// ===========================================================================
// SSH 拨号
// ===========================================================================

// dialSSH 使用 cert/key 建立到 address 的 SSH 连接，user 为 SSH 登录用户名。
// 支持 sshAddr（user@host:port）和 address（host:port）两种格式。
func dialSSH(ctx context.Context, sshAddr, certPEM, keyPEM string, cfg SSHDialerConfig) (*ssh.Client, error) {
	user, address := parseSSHAddr(sshAddr, cfg.SSHUser)

	signer, err := ssh.ParsePrivateKey([]byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("parse ssh key: %w", err)
	}

	var auth ssh.AuthMethod
	if certPEM != "" {
		cert, err := cert.DecodeSSHCertificateFromPEM([]byte(certPEM))
		if err != nil {
			return nil, fmt.Errorf("parse ssh cert: %w", err)
		}
		certSigner, err := ssh.NewCertSigner(cert, signer)
		if err != nil {
			return nil, fmt.Errorf("new cert signer: %w", err)
		}
		auth = ssh.PublicKeys(certSigner)
	} else {
		auth = ssh.PublicKeys(signer)
	}

	config := &ssh.ClientConfig{
		User:            string(user),
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: cfg.HostKeyCallback,
		Timeout:         cfg.SSHTimeout,
	}

	dialer := &net.Dialer{
		Timeout:   cfg.SSHTimeout,
		KeepAlive: 30 * time.Second,
	}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("dial ssh server %s: %w", address, err)
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake with %s: %w", address, err)
	}

	return ssh.NewClient(c, chans, reqs), nil
}

// parseSSHAddr 解析 user@host:port 格式的地址，返回 (user, host:port)。
// 如果没有 @ 符号，使用默认用户名。
func parseSSHAddr(addr string, defaultUser string) (string, string) {
	user, hostPort, ok := strings.Cut(addr, "@")
	if ok {
		return user, hostPort
	}
	return defaultUser, addr
}

// isSSHTransportError 返回 true 当错误指示 SSH 传输层连接本身已断开，
// 而非远端目标连接失败（如 target unreachable）。
// 调用方主动取消（context.Canceled/DeadlineExceeded）不算传输错误，
// 避免误标健康连接。
func isSSHTransportError(err error) bool {
	if err == nil {
		return false
	}
	// 调用方主动取消不应标 broken
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	// SSH 底层 TCP 断开会返回 net.OpError
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	// 优雅关闭/对端中途断开
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// 内核层 TCP 错误
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}
	// x/crypto/ssh 内部传输错误（无导出 sentinel）
	if strings.Contains(err.Error(), "ssh: tcp transport closed") {
		return true
	}
	return false
}

// dialSSHWithMergedCtx 是 dialSSH 的包装，合并 caller ctx 与 dialer 的 d.ctx。
// 当任一 ctx 被取消时拨号取消。goroutine 生命周期绑定在 dialSSH 调用期间，不泄漏。
func (d *SSHDialer) dialSSHWithMergedCtx(ctx context.Context, sshAddr, cert, key string) (*ssh.Client, error) {
	dialCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		select {
		case <-d.ctx.Done():
			cancel()
		case <-dialCtx.Done():
		}
	}()

	return dialSSH(dialCtx, sshAddr, cert, key, d.cfg)
}
