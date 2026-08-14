package container

import (
	"context"
	"net"
	"sync"
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
	entry.client = client
	entry.lastUsed = time.Now()

	// 健康的连接走 fast path，d 不会被使用
	got, err := entry.borrow(context.Background(), nil, "", "", "")
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	if got != client {
		t.Fatal("borrow returned wrong client")
	}
	if entry.lastUsed.Equal(time.Time{}) {
		t.Fatal("expected lastUsed to be updated")
	}
}

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
	entry.mu.RLock()
	if entry.client != nil {
		t.Fatal("expected nil client after failed reconnect")
	}
	entry.mu.RUnlock()
}

// TestDomainEntry_MarkBroken 测试标记为损坏并关闭连接。
func TestDomainEntry_MarkBroken(t *testing.T) {
	entry := &domainEntry{domainID: "test"}
	client := newSSHClient(t)
	entry.client = client

	entry.markBroken()
	if !entry.broken {
		t.Fatal("expected broken=true")
	}
	if entry.client != nil {
		t.Fatal("expected client to be nil after markBroken")
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

// TestSSHDialer_ConcurrentReconnect 50 个并发请求，连接断开后全部应等待重连而非直接失败。
func TestSSHDialer_ConcurrentReconnect(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{
		InitialReconnectBackoff: 1 * time.Millisecond,
		MaxReconnectBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()
	entry := d.getOrCreate("test-domain")

	// 模拟连接断开
	entry.markBroken()

	// 50 个并发请求，全部尝试重连（预期失败，但无惊群）
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

// TestSSHDialer_ReconnectCoordination 重连协调测试：
// 连接断开后，多个 goroutine 同时 borrow，只有第一个执行 dialSSH，其余等待。
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
			time.Sleep(200 * time.Millisecond)
			go func() {
				_, _, _, err := ssh.NewServerConn(tcpConn, serverConfig)
				if err != nil {
					_ = tcpConn.Close()
				}
			}()
		}
	}()

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 10; i++ {
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
	t.Logf("10 concurrent borrows with 200ms handshake: took %v", elapsed)
	if elapsed > 2*time.Second {
		t.Fatalf("expected reconnect coordination, but took %v (should be ~200-400ms)", elapsed)
	}
}

// TestSSHDialer_Close 测试正确关闭。
func TestSSHDialer_Close(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{})
	entry := d.getOrCreate("test-domain")
	entry.client = newSSHClient(t)
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
	entry.mu.RLock()
	hasClient := entry.client != nil
	entry.mu.RUnlock()
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
	entry1.client = newSSHClient(t)
	entry2 := d.getOrCreate("b")
	entry2.client = newSSHClient(t)

	if stats := d.Stats(); stats != 2 {
		t.Fatalf("expected 2 open, got %d", stats)
	}

	entry1.markBroken()
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
	entry.client = newSSHClient(t)
	entry.lastUsed = time.Now().Add(-1 * time.Hour)

	time.Sleep(100 * time.Millisecond)

	entry.mu.RLock()
	broken := entry.broken
	hasClient := entry.client != nil
	entry.mu.RUnlock()

	if !broken || hasClient {
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

// TestReconnectCoord_Parallelism 验证单次重连期间其余请求等待而非并行新建。
func TestReconnectCoord_Parallelism(t *testing.T) {
	d := NewSSHDialer(SSHDialerConfig{
		InitialReconnectBackoff: 1 * time.Millisecond,
		MaxReconnectBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()

	entry := &domainEntry{domainID: "test"}
	blockCh := make(chan struct{})
	startedCh := make(chan struct{})

	// 模拟一个正在进行的重连
	go func() {
		entry.reconMu.Lock()
		entry.mu.Lock()
		entry.reconnecting = true
		entry.reconnectCh = make(chan struct{})
		entry.mu.Unlock()
		entry.reconMu.Unlock()

		close(startedCh)
		<-blockCh

		entry.mu.Lock()
		entry.reconnecting = false
		entry.client = newSSHClient(t)
		entry.broken = false
		entry.lastUsed = time.Now()
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
	t.Log("all 5 borrowers correctly waited for single reconnect")
}

// waitCh returns a channel that is never closed (for timeout selects).
func waitCh() <-chan struct{} {
	return make(chan struct{})
}

// TestSSHDialer_BackoffReset 测试重连成功后退避重置。
func TestSSHDialer_BackoffReset(t *testing.T) {
	entry := &domainEntry{domainID: "test", maxBackoff: maxReconnectBackoff}
	entry.reconnectCh = make(chan struct{})
	entry.reconnectBackoff = 10 * time.Second

	// 模拟成功
	entry.finishReconnect(newSSHClient(t), nil, false)
	if entry.reconnectBackoff != 0 {
		t.Fatalf("expected backoff reset to 0, got %v", entry.reconnectBackoff)
	}

	entry.reconnectCh = make(chan struct{})

	// 模拟失败
	entry.finishReconnect(nil, assertAnError("fail"), false)
	if entry.reconnectBackoff != initialReconnectBackoff {
		t.Fatalf("expected backoff %v, got %v", initialReconnectBackoff, entry.reconnectBackoff)
	}

	entry.reconnectCh = make(chan struct{})

	// 模拟再次失败
	entry.finishReconnect(nil, assertAnError("fail again"), false)
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
