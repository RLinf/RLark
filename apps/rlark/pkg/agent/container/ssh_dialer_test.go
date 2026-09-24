package container

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// ---------------------------------------------------------------------------
// SSHDialer 单元测试
// ---------------------------------------------------------------------------

const testPrivateKeyPEM = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
QyNTUxOQAAACDYgEohV8cyTPhXqw3J4KJZ814GmHJAVqXy5IkEH6RBBgAAAKCK3Czsitws
7AAAAAtzc2gtZWQyNTUxOQAAACDYgEohV8cyTPhXqw3J4KJZ814GmHJAVqXy5IkEH6RBBg
AAAEDun/wMJd+XLqbF/nKfrayvmXeLhHjzLd4L+yQ/yFAgD9iASiFXxzJM+FerDcngolnz
XgaYckBWpfLkiQQfpEEGAAAAGmxpZ2h0bmluZ0BDaGVueHVNYWNib29rQWlyAQID
-----END OPENSSH PRIVATE KEY-----`

const testPublicKeyPEM = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAINiASiFXxzJM+FerDcngolnzXgaYckBWpfLkiQQfpEEG"

// newSSHClient creates a *ssh.Client with a fully initialized transport
// by going through a real SSH handshake with a local test server.
func newSSHClient(t *testing.T) *ssh.Client {
	t.Helper()

	signer, err := ssh.ParsePrivateKey([]byte(testPrivateKeyPEM))
	if err != nil {
		t.Fatalf("parse server key: %v", err)
	}

	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	serverConfig.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		tcpConn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _, _, err = ssh.NewServerConn(tcpConn, serverConfig)
		if err != nil {
			_ = tcpConn.Close()
		}
	}()

	tcpConn, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}

	clientConfig := &ssh.ClientConfig{
		User: "test",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}

	c, _, _, err := ssh.NewClientConn(tcpConn, ln.Addr().String(), clientConfig)
	if err != nil {
		t.Fatalf("client handshake: %v", err)
	}
	client := ssh.NewClient(c, nil, nil)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// testDialer returns a minimal dialer for tests that need to call entry.borrow() directly.
func testDialer(t *testing.T) *SSHDialer {
	t.Helper()
	d := NewSSHDialer(SSHDialerConfig{
		IdleTimeout:     100 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	})
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// TestDomainEntry_Borrow 测试正常路径：借用健康的连接。
func TestDomainEntry_Borrow(t *testing.T) {
	entry := &domainEntry{domainID: "test"}
	client := newSSHClient(t)
	pooled := newPooledSSHClient(client)
	entry.clients = []*pooledSSHClient{pooled}

	// 健康的连接走 fast path，d 不会被使用
	got, err := entry.borrow(context.Background(), nil, "", "", "")
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	if got != pooled {
		t.Fatal("borrow returned wrong client")
	}
	if pooled.lastUsed().Equal(time.Time{}) {
		t.Fatal("expected lastUsed to be updated")
	}
	entry.release(got)
}

func TestDomainEntry_AdaptivePoolSelection(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{MaxConnectionsPerDomain: 2})
	t.Cleanup(func() { _ = d.Close() })
	entry := &domainEntry{domainID: "test"}
	first := newPooledSSHClient(newSSHClient(t))
	second := newPooledSSHClient(newSSHClient(t))
	first.active = 2
	second.active = 1
	entry.clients = []*pooledSSHClient{first, second}

	got, err := entry.borrow(context.Background(), d, "", "", "")
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	if got != second {
		t.Fatal("expected least-loaded SSH connection")
	}
	if second.active != 2 {
		t.Fatalf("expected active=2, got %d", second.active)
	}
	entry.release(got)
	if second.active != 1 {
		t.Fatalf("expected active=1 after release, got %d", second.active)
	}
}

func TestActivityConn_CloseReleasesOnce(t *testing.T) {
	left, right := net.Pipe()
	t.Cleanup(func() { _ = right.Close() })
	releases := 0
	conn := &activityConn{
		Conn:       left,
		onActivity: func() {},
		onClose:    func() { releases++ },
		onError:    func(error) {},
	}

	_ = conn.Close()
	_ = conn.Close()
	if releases != 1 {
		t.Fatalf("expected one release, got %d", releases)
	}
}

func TestActivityConn_TransportErrorReportedOnce(t *testing.T) {
	transportErr := &net.OpError{Op: "write", Err: syscall.ETIMEDOUT}
	errorsReported := 0
	conn := &activityConn{
		Conn:       &errorConn{err: transportErr},
		onActivity: func() {},
		onClose:    func() {},
		onError: func(err error) {
			if !errors.Is(err, transportErr) {
				t.Errorf("unexpected transport error: %v", err)
			}
			errorsReported++
		},
	}

	_, _ = conn.Write(nil)
	_, _ = conn.Read(nil)
	if errorsReported != 1 {
		t.Fatalf("expected one transport error report, got %d", errorsReported)
	}
}

type errorConn struct {
	net.Conn
	err error
}

func (c *errorConn) Read([]byte) (int, error)  { return 0, c.err }
func (c *errorConn) Write([]byte) (int, error) { return 0, c.err }

// TestDomainEntry_BorrowBroken 测试连接损坏后触发重连。
func TestDomainEntry_BorrowBroken(t *testing.T) {
	d := testDialer(t)
	entry := &domainEntry{domainID: "test"}

	// 没有可用连接 → 尝试重连 → 失败
	_, err := entry.borrow(context.Background(), d, "127.0.0.1:1", "", testPrivateKeyPEM)
	if err == nil {
		t.Fatal("expected error when no SSH server")
	}

	// 重连失败后不应有 client
	entry.mu.Lock()
	if len(entry.clients) != 0 {
		t.Fatal("expected nil client after failed reconnect")
	}
	entry.mu.Unlock()
}

// TestDomainEntry_MarkBroken 测试标记为损坏并关闭连接。
func TestDomainEntry_MarkBroken(t *testing.T) {
	entry := &domainEntry{domainID: "test"}
	client := newSSHClient(t)
	pooled := newPooledSSHClient(client)
	entry.clients = []*pooledSSHClient{pooled}

	entry.markBroken(pooled, "test")
	if len(entry.clients) != 0 {
		t.Fatal("expected client to be nil after markBroken")
	}
}

func TestDomainEntry_MarkBrokenDrainsActiveChannels(t *testing.T) {
	entry := &domainEntry{domainID: "test"}
	pooled := newPooledSSHClient(newSSHClient(t))
	pooled.active = 2
	entry.clients = []*pooledSSHClient{pooled}

	entry.markBroken(pooled, "test")
	if len(entry.clients) != 1 || !pooled.draining {
		t.Fatal("active client should remain tracked while draining")
	}
	if got := entry.leastLoadedLocked(); got != nil {
		t.Fatal("draining client must not accept new channels")
	}

	entry.release(pooled)
	if len(entry.clients) != 1 {
		t.Fatal("client should remain until all channels are released")
	}
	entry.release(pooled)
	if len(entry.clients) != 0 {
		t.Fatal("client should close after its last channel is released")
	}
}

// TestSSHDialer_ConcurrentSafety 高并发下不 panic 不死锁。
func TestSSHDialer_ConcurrentSafety(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{
		IdleTimeout:             100 * time.Millisecond,
		CleanupInterval:         50 * time.Millisecond,
		InitialReconnectBackoff: 1 * time.Millisecond,
		MaxReconnectBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = d.DialContext(context.Background(), "test-domain", "127.0.0.1:1", "", testPrivateKeyPEM, "127.0.0.1:80")
		}()
	}
	wg.Wait()

	time.Sleep(150 * time.Millisecond)
	t.Log("50 concurrent dials completed without panic")
}

// TestSSHDialer_ConcurrentReconnect 50 个并发请求失败后都应及时返回。
func TestSSHDialer_ConcurrentReconnect(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{
		InitialReconnectBackoff: 1 * time.Millisecond,
		MaxReconnectBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()
	entry := d.getOrCreate("test-domain")

	// 模拟连接断开
	entry.close()

	// 50 个并发请求，最多并行建立连接池容量个连接。
	var wg sync.WaitGroup
	errCh := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := entry.borrow(context.Background(), d, "127.0.0.1:1", "", testPrivateKeyPEM)
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)

	count := 0
	for err := range errCh {
		if err == nil {
			t.Fatal("all should fail - no SSH server")
		}
		count++
	}
	if count != 50 {
		t.Fatalf("expected 50 results, got %d", count)
	}
	t.Logf("50 concurrent reconnects: all completed, none deadlocked")
}

// TestSSHDialer_ReconnectCoordination 验证并发请求可并行填充连接池。
func TestSSHDialer_ReconnectCoordination(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{
		InitialReconnectBackoff: 1 * time.Millisecond,
		MaxReconnectBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()
	entry := d.getOrCreate("test-domain")

	// 启动一个真实的 SSH 服务器，慢速握手
	signer, err := ssh.ParsePrivateKey([]byte(testPrivateKeyPEM))
	if err != nil {
		t.Fatalf("parse key: %v", err)
	}
	serverConfig := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return &ssh.Permissions{}, nil
		},
	}
	serverConfig.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			tcpConn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				time.Sleep(200 * time.Millisecond)
				_, _, _, err := ssh.NewServerConn(tcpConn, serverConfig)
				if err != nil {
					_ = tcpConn.Close()
				}
			}()
		}
	}()

	const requestCount = defaultMaxConnections * 10
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < requestCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := entry.borrow(context.Background(), d, ln.Addr().String(), "", testPrivateKeyPEM)
			if err != nil {
				t.Errorf("borrow failed: %v", err)
			}
		}()
	}
	wg.Wait()

	elapsed := time.Since(start)
	t.Logf("%d concurrent borrows with 200ms handshake: took %v", requestCount, elapsed)
	if elapsed > time.Second {
		t.Fatalf("expected parallel connection setup, but took %v", elapsed)
	}
	if got := d.Stats(); got != defaultMaxConnections {
		t.Fatalf("expected pool capped at %d connections, got %d", defaultMaxConnections, got)
	}
}

// TestSSHDialer_Close 测试正确关闭。
func TestSSHDialer_Close(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{})
	entry := d.getOrCreate("test-domain")
	entry.clients = []*pooledSSHClient{newPooledSSHClient(newSSHClient(t))}
	_ = d.Close()
}

// TestSSHDialer_CloseRacesReconnect 测试 Close 与重连的竞态：
// 重连中调用 Close，不应产生泄漏。
func TestSSHDialer_CloseRacesReconnect(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{})
	entry := d.getOrCreate("test-domain")

	// 在一个 goroutine 中启动重连（但阻塞在退避或拨号中）
	done := make(chan struct{})
	go func() {
		_, _ = entry.borrow(context.Background(), d, "127.0.0.1:1", "", testPrivateKeyPEM)
		close(done)
	}()

	// 立即 Close
	time.Sleep(5 * time.Millisecond)
	_ = d.Close()

	// 等待重连返回
	<-done

	// Close 后不应有 client
	entry.mu.Lock()
	hasClient := len(entry.clients) != 0
	entry.mu.Unlock()
	if hasClient {
		t.Fatal("expected no client after Close, got leaked connection")
	}

	// 再次调用 Close 应安全（幂等）
	_ = d.Close()
}

// TestSSHDialer_Stats 测试统计。
func TestSSHDialer_Stats(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{})
	defer func() { _ = d.Close() }()

	entry1 := d.getOrCreate("a")
	entry1.clients = []*pooledSSHClient{newPooledSSHClient(newSSHClient(t))}
	entry2 := d.getOrCreate("b")
	entry2.clients = []*pooledSSHClient{newPooledSSHClient(newSSHClient(t))}

	if stats := d.Stats(); stats != 2 {
		t.Fatalf("expected 2 open, got %d", stats)
	}

	entry1.markBroken(entry1.clients[0], "test")
	if stats := d.Stats(); stats != 1 {
		t.Fatalf("expected 1 open after broken, got %d", stats)
	}
}

// TestSSHDialer_GC 测试空闲连接回收。
func TestSSHDialer_GC(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{
		IdleTimeout:     50 * time.Millisecond,
		CleanupInterval: 20 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()

	entry := d.getOrCreate("test-domain")
	client := newSSHClient(t)
	entry.mu.Lock()
	pooled := newPooledSSHClient(client)
	pooled.lastUsedNanos.Store(time.Now().Add(-1 * time.Hour).UnixNano())
	entry.clients = []*pooledSSHClient{pooled}
	entry.mu.Unlock()

	time.Sleep(100 * time.Millisecond)

	entry.mu.Lock()
	hasClient := len(entry.clients) != 0
	entry.mu.Unlock()

	if hasClient {
		t.Fatal("expected idle connection to be GC'd")
	}
}

// TestSSHDialer_GetOrCreate 测试 domain 创建和复用。
func TestSSHDialer_GetOrCreate(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{})
	defer func() { _ = d.Close() }()

	e1 := d.getOrCreate("test")
	e2 := d.getOrCreate("test")
	e3 := d.getOrCreate("other")

	if e1 != e2 {
		t.Fatal("expected same instance for same ID")
	}
	if e1 == e3 {
		t.Fatal("expected different instance for different ID")
	}
}

// TestDialSSH_ParseKey 测试密钥解析。
func TestDialSSH_ParseKey(t *testing.T) {
	signer, err := ssh.ParsePrivateKey([]byte(testPrivateKeyPEM))
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	if signer.PublicKey().Type() != ssh.KeyAlgoED25519 {
		t.Fatalf("expected ed25519, got %s", signer.PublicKey().Type())
	}
}

// TestDialSSH_ParsePubkey 测试公钥解析。
func TestDialSSH_ParsePubkey(t *testing.T) {
	_, _, _, _, err := ssh.ParseAuthorizedKey([]byte(testPublicKeyPEM))
	if err != nil {
		t.Fatalf("parse authorized key: %v", err)
	}
}

// TestReconnectCoord_Parallelism 验证池满时请求等待当前一轮建连。
func TestReconnectCoord_Parallelism(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{
		InitialReconnectBackoff: 1 * time.Millisecond,
		MaxReconnectBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()

	entry := &domainEntry{domainID: "test"}
	blockCh := make(chan struct{})
	startedCh := make(chan struct{})

	// 模拟所有建连槽都正在使用。
	go func() {
		entry.mu.Lock()
		entry.reconnecting = d.cfg.MaxConnectionsPerDomain
		entry.reconnectCh = make(chan struct{})
		entry.mu.Unlock()

		close(startedCh)
		<-blockCh

		entry.mu.Lock()
		entry.reconnecting = 0
		entry.clients = []*pooledSSHClient{newPooledSSHClient(newSSHClient(t))}
		close(entry.reconnectCh)
		entry.mu.Unlock()
	}()

	<-startedCh
	time.Sleep(10 * time.Millisecond)

	// 5 个 borrower 应全部等待
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			client, err := entry.borrow(context.Background(), d, "", "", "")
			if err != nil || client == nil {
				t.Error("expected successful borrow after reconnect")
			}
			if client != nil {
				entry.release(client)
			}
		}()
	}

	// 验证 borrower 被阻塞
	select {
	case <-time.After(50 * time.Millisecond):
		// 正常
	case <-waitCh():
		t.Fatal("borrowers returned before reconnect completed")
	}

	close(blockCh)
	wg.Wait()
	t.Log("all 5 borrowers correctly waited for the active connection attempts")
}

// waitCh returns a channel that is never closed (for timeout selects).
func waitCh() <-chan struct{} {
	return make(chan struct{})
}

// TestSSHDialer_BackoffReset 测试重连成功后退避重置。
func TestSSHDialer_BackoffReset(t *testing.T) {
	d := testDialer(t)
	entry := &domainEntry{domainID: "test", maxBackoff: maxReconnectBackoff}
	entry.reconnectCh = make(chan struct{})
	entry.reconnecting = 1
	entry.reconnectBackoff = 10 * time.Second

	// 模拟成功
	entry.finishReconnect(newSSHClient(t), nil, false, d)
	if entry.reconnectBackoff != 0 {
		t.Fatalf("expected backoff reset to 0, got %v", entry.reconnectBackoff)
	}

	entry.reconnectCh = make(chan struct{})
	entry.reconnecting = 1

	// 模拟失败
	entry.finishReconnect(nil, assertAnError("fail"), false, d)
	if entry.reconnectBackoff != initialReconnectBackoff {
		t.Fatalf("expected backoff %v, got %v", initialReconnectBackoff, entry.reconnectBackoff)
	}

	entry.reconnectCh = make(chan struct{})
	entry.reconnecting = 1

	// 模拟再次失败
	entry.finishReconnect(nil, assertAnError("fail again"), false, d)
	expected := initialReconnectBackoff * 2
	if entry.reconnectBackoff != expected {
		t.Fatalf("expected backoff %v, got %v", expected, entry.reconnectBackoff)
	}
}

// assertAnError returns a non-nil error for testing.
func assertAnError(msg string) error {
	return &testError{msg: msg}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// TestNextBackoff 测试退避计算。
func TestNextBackoff(t *testing.T) {
	tests := []struct {
		current  time.Duration
		max      time.Duration
		expected time.Duration
	}{
		{0, maxReconnectBackoff, initialReconnectBackoff},
		{initialReconnectBackoff, maxReconnectBackoff, initialReconnectBackoff * 2},
		{maxReconnectBackoff / 2, maxReconnectBackoff, maxReconnectBackoff},
		{maxReconnectBackoff, maxReconnectBackoff, maxReconnectBackoff},
		{maxReconnectBackoff * 2, maxReconnectBackoff, maxReconnectBackoff},
	}
	for _, tt := range tests {
		got := nextBackoff(tt.current, tt.max)
		if got != tt.expected {
			t.Errorf("nextBackoff(%v, %v) = %v, want %v", tt.current, tt.max, got, tt.expected)
		}
	}
}

// TestIsSSHTransportError 覆盖各类传输错误与误判场景。
func TestIsSSHTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"io.EOF", io.EOF, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"ECONNRESET", syscall.ECONNRESET, true},
		{"EPIPE", syscall.EPIPE, true},
		{"ETIMEDOUT", syscall.ETIMEDOUT, true},
		{"wrapped ECONNRESET", fmt.Errorf("dial: %w", syscall.ECONNRESET), true},
		{"net.OpError", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true},
		{"wrapped net.OpError", fmt.Errorf("proxy: %w", &net.OpError{Op: "read", Err: syscall.ECONNRESET}), true},
		{"ssh transport closed", errors.New("ssh: tcp transport closed"), true},
		{"context.Canceled", context.Canceled, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"wrapped context.Canceled", fmt.Errorf("dial: %w", context.Canceled), false},
		{"wrapped context.DeadlineExceeded", fmt.Errorf("dial: %w", context.DeadlineExceeded), true},
		{"generic error", errors.New("connection refused"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSSHTransportError(tt.err); got != tt.want {
				t.Errorf("isSSHTransportError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestDomainEntry_MarkBrokenIfCurrent 防止 keepalive goroutine 误标新连接。
func TestDomainEntry_MarkBrokenIfCurrent(t *testing.T) {
	entry := &domainEntry{domainID: "test"}

	old := newPooledSSHClient(newSSHClient(t))
	entry.clients = []*pooledSSHClient{old}

	// 换一个"新"client 进来（模拟重连成功）
	newClient := newPooledSSHClient(newSSHClient(t))
	entry.clients = []*pooledSSHClient{newClient}

	// keepalive 拿旧 client 来标 broken,不应影响新连接
	entry.markBroken(old, "test")
	if len(entry.clients) != 1 || entry.clients[0] != newClient {
		t.Fatal("current client should remain untouched")
	}

	// 用当前 client 标 broken,应生效
	entry.markBroken(newClient, "test")
	if len(entry.clients) != 0 {
		t.Fatal("expected client to be nil after markBrokenIfCurrent")
	}
}

// TestParseSSHAddr 测试 user@host:port 解析。
func TestParseSSHAddr(t *testing.T) {
	tests := []struct {
		addr        string
		defaultUser string
		wantUser    string
		wantHost    string
	}{
		{"root@192.168.1.1:22", "admin", "root", "192.168.1.1:22"},
		{"app@10.0.0.5:2222", "root", "app", "10.0.0.5:2222"},
		{"192.168.1.1:22", "root", "root", "192.168.1.1:22"},
		{"10.0.0.5:2222", "admin", "admin", "10.0.0.5:2222"},
		{"user@host:0", "", "user", "host:0"},
		{"@host:22", "root", "", "host:22"},
	}
	for _, tt := range tests {
		gotUser, gotHost := parseSSHAddr(tt.addr, tt.defaultUser)
		if gotUser != tt.wantUser || gotHost != tt.wantHost {
			t.Errorf("parseSSHAddr(%q, %q) = (%q, %q), want (%q, %q)",
				tt.addr, tt.defaultUser, gotUser, gotHost, tt.wantUser, tt.wantHost)
		}
	}
}
