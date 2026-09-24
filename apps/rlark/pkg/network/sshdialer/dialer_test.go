package sshdialer

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
// DialerManager and domain Dialer tests.
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
func testDialer(t *testing.T) *DialerManager {
	t.Helper()
	d := NewDialerManager(Config{
		IdleTimeout:     100 * time.Millisecond,
		CleanupInterval: 50 * time.Millisecond,
	})
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func newDomainEntry(id string) *domainEntry {
	return &domainEntry{id: id, domainInfo: DomainInfo{ID: id}, generation: 1}
}

func TestConfig_KeepaliveTimeoutDefault(t *testing.T) {
	cfg := Config{}
	cfg.setDefaults()
	if cfg.KeepaliveTimeout != defaultKeepaliveTimeout {
		t.Fatalf("KeepaliveTimeout = %v, want %v", cfg.KeepaliveTimeout, defaultKeepaliveTimeout)
	}
	if cfg.KeepaliveDrainGrace != defaultKeepaliveDrainGrace {
		t.Fatalf("KeepaliveDrainGrace = %v, want %v", cfg.KeepaliveDrainGrace, defaultKeepaliveDrainGrace)
	}
}

func TestDomainEntry_KeepaliveTimeoutDrains(t *testing.T) {
	entry := newDomainEntry("test")
	pooled := newPooledSSHClient(newSSHClient(t))
	pooled.active = 1
	entry.clients = []*pooledSSHClient{pooled}

	done := make(chan struct{})
	go func() {
		entry.keepaliveLoop(pooled, time.Millisecond, 10*time.Millisecond, time.Second)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("keepalive loop did not time out")
	}

	entry.mu.Lock()
	if len(entry.clients) != 1 || !pooled.draining {
		entry.mu.Unlock()
		t.Fatal("timed-out keepalive should drain the active SSH transport")
	}
	entry.mu.Unlock()
	entry.release(pooled)
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if len(entry.clients) != 0 {
		t.Fatal("draining transport should close after its last channel exits")
	}
}

func TestDomainEntry_DrainGraceForcesClose(t *testing.T) {
	entry := newDomainEntry("test")
	pooled := newPooledSSHClient(newSSHClient(t))
	pooled.active = 1
	entry.clients = []*pooledSSHClient{pooled}

	entry.drain(pooled, "test", 10*time.Millisecond)
	deadline := time.Now().Add(time.Second)
	for {
		entry.mu.Lock()
		removed := len(entry.clients) == 0
		entry.mu.Unlock()
		if removed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("drain grace did not force-close the transport")
		}
		time.Sleep(time.Millisecond)
	}

	entry.release(pooled)
	if pooled.active != 0 {
		t.Fatalf("expected delayed release to remain safe, got active=%d", pooled.active)
	}
}

// TestDomainEntry_Borrow 测试正常路径：借用健康的连接。
func TestDomainEntry_Borrow(t *testing.T) {
	entry := newDomainEntry("test")
	client := newSSHClient(t)
	pooled := newPooledSSHClient(client)
	entry.clients = []*pooledSSHClient{pooled}

	// 健康的连接走 fast path，d 不会被使用
	got, err := entry.borrow(context.Background(), nil)
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
	d := NewDialerManager(Config{MaxConnectionsPerDomain: 2})
	t.Cleanup(func() { _ = d.Close() })
	entry := newDomainEntry("test")
	first := newPooledSSHClient(newSSHClient(t))
	second := newPooledSSHClient(newSSHClient(t))
	first.active = 2
	second.active = 1
	entry.clients = []*pooledSSHClient{first, second}

	got, err := entry.borrow(context.Background(), d)
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

func TestDomainEntry_RoundRobinsEquallyLoadedClients(t *testing.T) {
	entry := newDomainEntry("test")
	first := newPooledSSHClient(newSSHClient(t))
	second := newPooledSSHClient(newSSHClient(t))
	first.generation = entry.generation
	second.generation = entry.generation
	entry.clients = []*pooledSSHClient{first, second}

	got, err := entry.borrow(context.Background(), nil)
	if err != nil {
		t.Fatalf("first borrow: %v", err)
	}
	if got != first {
		t.Fatal("expected first borrow to use first transport")
	}
	entry.release(got)

	got, err = entry.borrow(context.Background(), nil)
	if err != nil {
		t.Fatalf("second borrow: %v", err)
	}
	if got != second {
		t.Fatal("expected retry to rotate to equally loaded second transport")
	}
	entry.release(got)
}

func TestDomainEntry_LoadTriggersBackgroundExpansion(t *testing.T) {
	d := NewDialerManager(Config{
		MaxConnectionsPerDomain:  2,
		MaxChannelsPerConnection: 2,
	})
	t.Cleanup(func() { _ = d.Close() })
	entry := newDomainEntry("test")
	entry.domainInfo = DomainInfo{ID: "test", SSHAddress: "127.0.0.1:1", PrivateKey: testPrivateKeyPEM}
	client := newPooledSSHClient(newSSHClient(t))
	client.generation = entry.generation
	client.active = 1
	entry.clients = []*pooledSSHClient{client}

	started := time.Now()
	got, err := entry.borrow(context.Background(), d)
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	if got != client {
		t.Fatal("expected the existing transport while expansion runs")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("borrow waited for background expansion: %v", elapsed)
	}
	entry.mu.Lock()
	reconnecting := entry.reconnecting
	entry.mu.Unlock()
	if !reconnecting {
		t.Fatal("expected channel load to start background expansion")
	}
	entry.release(got)
}

func TestDomainEntry_LoadExpansionRespectsConnectionLimit(t *testing.T) {
	d := NewDialerManager(Config{
		MaxConnectionsPerDomain:  1,
		MaxChannelsPerConnection: 1,
	})
	t.Cleanup(func() { _ = d.Close() })
	entry := newDomainEntry("test")
	client := newPooledSSHClient(newSSHClient(t))
	client.generation = entry.generation
	entry.clients = []*pooledSSHClient{client}

	got, err := entry.borrow(context.Background(), d)
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	entry.mu.Lock()
	reconnecting := entry.reconnecting
	entry.mu.Unlock()
	if reconnecting {
		t.Fatal("connection limit should prevent background expansion")
	}
	entry.release(got)
}

func TestDomainEntry_LowLoadDoesNotExpand(t *testing.T) {
	d := NewDialerManager(Config{
		MaxConnectionsPerDomain:  2,
		MaxChannelsPerConnection: 2,
	})
	t.Cleanup(func() { _ = d.Close() })
	entry := newDomainEntry("test")
	client := newPooledSSHClient(newSSHClient(t))
	client.generation = entry.generation
	entry.clients = []*pooledSSHClient{client}

	got, err := entry.borrow(context.Background(), d)
	if err != nil {
		t.Fatalf("borrow: %v", err)
	}
	entry.mu.Lock()
	reconnecting := entry.reconnecting
	entry.mu.Unlock()
	if reconnecting {
		t.Fatal("load below the soft limit should not expand the pool")
	}
	entry.release(got)
}

func TestActivityConn_CloseReleasesOnce(t *testing.T) {
	left, right := net.Pipe()
	t.Cleanup(func() { _ = right.Close() })
	releases := 0
	conn := &activityConn{
		Conn:       left,
		onActivity: func() {},
		onRelease:  func() { releases++ },
		onError:    func(error) {},
	}

	_ = conn.Close()
	_ = conn.Close()
	if releases != 1 {
		t.Fatalf("expected one release, got %d", releases)
	}
}

func TestActivityConn_ChannelTimeoutReleasesWithoutTransportError(t *testing.T) {
	transportErr := &net.OpError{Op: "write", Err: syscall.ETIMEDOUT}
	errorsReported := 0
	releases := 0
	conn := &activityConn{
		Conn:       &errorConn{err: transportErr},
		onActivity: func() {},
		onRelease:  func() { releases++ },
		onError: func(err error) {
			if !errors.Is(err, transportErr) {
				t.Errorf("unexpected transport error: %v", err)
			}
			errorsReported++
		},
	}

	_, _ = conn.Write(nil)
	_, _ = conn.Read(nil)
	if errorsReported != 0 {
		t.Fatalf("expected channel timeout not to abort transport, got %d reports", errorsReported)
	}
	if releases != 1 {
		t.Fatalf("expected terminal channel error to release once, got %d", releases)
	}
}

func TestActivityConn_EOFDoesNotReportTransportError(t *testing.T) {
	errorsReported := 0
	conn := &activityConn{
		Conn:       &errorConn{err: io.EOF},
		onActivity: func() {},
		onRelease:  func() {},
		onError:    func(error) { errorsReported++ },
	}

	_, _ = conn.Read(nil)
	_, _ = conn.Write(nil)
	if errorsReported != 0 {
		t.Fatalf("expected channel EOF not to report a transport error, got %d", errorsReported)
	}
}

func TestActivityConn_UnexpectedEOFReportsTransportError(t *testing.T) {
	errorsReported := 0
	conn := &activityConn{
		Conn:       &errorConn{err: io.ErrUnexpectedEOF},
		onActivity: func() {},
		onRelease:  func() {},
		onError:    func(error) { errorsReported++ },
	}

	_, _ = conn.Read(nil)
	if errorsReported != 1 {
		t.Fatalf("expected unexpected EOF to report a transport error, got %d", errorsReported)
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
	entry := newDomainEntry("test")

	// 没有可用连接 → 尝试重连 → 失败
	entry.domainInfo = DomainInfo{ID: "test", SSHAddress: "127.0.0.1:1", PrivateKey: testPrivateKeyPEM}
	_, err := entry.borrow(context.Background(), d)
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
	entry := newDomainEntry("test")
	client := newSSHClient(t)
	pooled := newPooledSSHClient(client)
	entry.clients = []*pooledSSHClient{pooled}

	entry.abort(pooled, "test")
	if len(entry.clients) != 0 {
		t.Fatal("expected transport to be removed after abort")
	}
}

func TestDomainEntry_MarkBrokenClosesActiveTransport(t *testing.T) {
	entry := newDomainEntry("test")
	pooled := newPooledSSHClient(newSSHClient(t))
	pooled.active = 2
	entry.clients = []*pooledSSHClient{pooled}

	entry.abort(pooled, "test")
	if len(entry.clients) != 0 || !pooled.draining {
		t.Fatal("broken client should be removed even with active channels")
	}
	if got := entry.leastLoadedLocked(); got != nil {
		t.Fatal("draining client must not accept new channels")
	}

	entry.release(pooled)
	entry.release(pooled)
	if pooled.active != 0 {
		t.Fatalf("expected active references to drain, got %d", pooled.active)
	}
}

// TestDialerManager_ConcurrentSafety 高并发下不 panic 不死锁。
func TestDialerManager_ConcurrentSafety(t *testing.T) {
	d := NewDialerManager(Config{
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
			dialer := d.GetDialer(DomainInfo{ID: "test-domain", SSHAddress: "127.0.0.1:1", PrivateKey: testPrivateKeyPEM})
			_, _ = dialer.DialContext(context.Background(), "tcp", "127.0.0.1:80")
		}()
	}
	wg.Wait()

	time.Sleep(150 * time.Millisecond)
	t.Log("50 concurrent dials completed without panic")
}

// TestDialerManager_ConcurrentReconnect 50 个并发请求失败后都应及时返回。
func TestDialerManager_ConcurrentReconnect(t *testing.T) {
	d := NewDialerManager(Config{
		InitialReconnectBackoff: 1 * time.Millisecond,
		MaxReconnectBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()
	entry := d.getOrCreate(DomainInfo{ID: "test", SSHAddress: "127.0.0.1:1", PrivateKey: testPrivateKeyPEM})

	// 模拟连接断开
	entry.close()

	// 50 个并发请求，最多并行建立连接池容量个连接。
	var wg sync.WaitGroup
	errCh := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := entry.borrow(context.Background(), d)
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

// TestDialerManager_ReconnectCoordination verifies concurrent requests share
// one recovery handshake instead of creating a connection storm.
func TestDialerManager_ReconnectCoordination(t *testing.T) {
	d := NewDialerManager(Config{
		InitialReconnectBackoff: 1 * time.Millisecond,
		MaxReconnectBackoff:     10 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()

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
	entry := d.getOrCreate(DomainInfo{ID: "test", SSHAddress: ln.Addr().String(), PrivateKey: testPrivateKeyPEM})

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
			_, err := entry.borrow(context.Background(), d)
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
	if got := d.Stats(); got != 1 {
		t.Fatalf("expected one shared recovery connection, got %d", got)
	}
}

// TestDialerManager_Close 测试正确关闭。
func TestDialerManager_Close(t *testing.T) {
	d := NewDialerManager(Config{})
	entry := d.getOrCreate(DomainInfo{ID: "test-domain"})
	entry.clients = []*pooledSSHClient{newPooledSSHClient(newSSHClient(t))}
	_ = d.Close()
}

// TestDialerManager_CloseRacesReconnect 测试 Close 与重连的竞态：
// 重连中调用 Close，不应产生泄漏。
func TestDialerManager_CloseRacesReconnect(t *testing.T) {
	d := NewDialerManager(Config{})
	entry := d.getOrCreate(DomainInfo{ID: "test", SSHAddress: "127.0.0.1:1", PrivateKey: testPrivateKeyPEM})

	// 在一个 goroutine 中启动重连（但阻塞在退避或拨号中）
	done := make(chan struct{})
	go func() {
		_, _ = entry.borrow(context.Background(), d)
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

// TestDialerManager_Stats 测试统计。
func TestDialerManager_Stats(t *testing.T) {
	d := NewDialerManager(Config{})
	defer func() { _ = d.Close() }()

	entry1 := d.getOrCreate(DomainInfo{ID: "a"})
	entry1.clients = []*pooledSSHClient{newPooledSSHClient(newSSHClient(t))}
	entry2 := d.getOrCreate(DomainInfo{ID: "b"})
	entry2.clients = []*pooledSSHClient{newPooledSSHClient(newSSHClient(t))}

	if stats := d.Stats(); stats != 2 {
		t.Fatalf("expected 2 open, got %d", stats)
	}

	entry1.abort(entry1.clients[0], "test")
	if stats := d.Stats(); stats != 1 {
		t.Fatalf("expected 1 open after broken, got %d", stats)
	}
}

func TestDialerManager_ChannelLimit(t *testing.T) {
	d := NewDialerManager(Config{MaxChannelsPerDomain: 1})
	defer func() { _ = d.Close() }()
	entry := d.getOrCreate(DomainInfo{ID: "test"})
	client := newPooledSSHClient(newSSHClient(t))
	client.generation = entry.generation
	client.active = 1
	entry.clients = []*pooledSSHClient{client}
	_, err := entry.borrow(context.Background(), d)
	if !errors.Is(err, ErrOverloaded) {
		t.Fatalf("borrow error = %v, want ErrOverloaded", err)
	}
}

func TestDialerManager_RemoveDomain(t *testing.T) {
	d := NewDialerManager(Config{})
	defer func() { _ = d.Close() }()
	dialer := d.GetDialer(DomainInfo{ID: "test", PrivateKey: "secret"})
	d.RemoveDomain("test")
	if _, err := dialer.DialContext(context.Background(), "tcp", "localhost:1"); !errors.Is(err, ErrDomainRemoved) {
		t.Fatalf("dial error = %v, want ErrDomainRemoved", err)
	}
	if stats := d.PoolStats(); stats.Domains != 0 {
		t.Fatalf("expected domain removal, got %+v", stats)
	}
}

// TestDialerManager_GC 测试空闲连接回收。
func TestDialerManager_GC(t *testing.T) {
	d := NewDialerManager(Config{
		IdleTimeout:     50 * time.Millisecond,
		CleanupInterval: 20 * time.Millisecond,
	})
	defer func() { _ = d.Close() }()

	entry := d.getOrCreate(DomainInfo{ID: "test-domain"})
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

// TestDialerManager_GetOrCreate 测试 domain 创建和复用。
func TestDialerManager_GetOrCreate(t *testing.T) {
	d := NewDialerManager(Config{})
	defer func() { _ = d.Close() }()

	e1 := d.getOrCreate(DomainInfo{ID: "test", SSHAddress: "first"})
	e2 := d.getOrCreate(DomainInfo{ID: "test", SSHAddress: "second"})
	e3 := d.getOrCreate(DomainInfo{ID: "other"})

	if e1 != e2 {
		t.Fatal("expected same instance for same ID")
	}
	if e1 == e3 {
		t.Fatal("expected different instance for different ID")
	}
	if e1.domainInfo.SSHAddress != "second" {
		t.Fatal("expected the latest domain info to be used")
	}
	if e1.generation != 2 {
		t.Fatalf("expected changed domain info to advance generation, got %d", e1.generation)
	}
}

func TestDialerManager_ConfigChangeDrainsOldTransport(t *testing.T) {
	d := NewDialerManager(Config{KeepaliveDrainGrace: time.Second})
	defer func() { _ = d.Close() }()
	entry := d.getOrCreate(DomainInfo{ID: "test", SSHAddress: "first"})
	client := newPooledSSHClient(newSSHClient(t))
	client.generation = entry.generation
	client.active = 1
	entry.clients = []*pooledSSHClient{client}

	d.getOrCreate(DomainInfo{ID: "test", SSHAddress: "second"})
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if !client.draining {
		t.Fatal("old-generation transport should drain after endpoint change")
	}
	if entry.leastLoadedLocked() != nil {
		t.Fatal("old-generation transport must not accept new channels")
	}
}

func TestDialerManager_UnchangedConfigKeepsGeneration(t *testing.T) {
	d := NewDialerManager(Config{})
	defer func() { _ = d.Close() }()
	info := DomainInfo{ID: "test", SSHAddress: "same", PrivateKey: "key"}
	entry := d.getOrCreate(info)
	generation := entry.generation
	d.getOrCreate(info)
	if entry.generation != generation {
		t.Fatalf("unchanged config advanced generation from %d to %d", generation, entry.generation)
	}
}

func TestBorrowCancellationDoesNotChangeBackoff(t *testing.T) {
	d := NewDialerManager(Config{InitialReconnectBackoff: time.Second})
	defer func() { _ = d.Close() }()
	entry := d.getOrCreate(DomainInfo{ID: "test", SSHAddress: "127.0.0.1:1", PrivateKey: testPrivateKeyPEM})
	entry.mu.Lock()
	entry.reconnectBackoff = time.Second
	entry.mu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := entry.borrow(ctx, d)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("borrow error = %v, want context canceled", err)
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.reconnectBackoff != time.Second {
		t.Fatalf("caller cancellation changed shared backoff to %v", entry.reconnectBackoff)
	}
}

func TestDialerManager_GetDialerBindsDomain(t *testing.T) {
	manager := NewDialerManager(Config{})
	defer func() { _ = manager.Close() }()

	first := manager.GetDialer(DomainInfo{ID: "test", SSHAddress: "first"})
	second := manager.GetDialer(DomainInfo{ID: "test", SSHAddress: "second"})
	if first.entry != second.entry {
		t.Fatal("dialers for the same domain should share a connection pool")
	}
	if second.entry.domainInfo.SSHAddress != "second" {
		t.Fatal("domain dialer should use the latest domain info")
	}
	if _, err := second.DialContext(context.Background(), "udp", "localhost:1"); err == nil {
		t.Fatal("expected unsupported network error")
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

func TestDialSSH_HandshakeTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, acceptErr := ln.Accept()
		if acceptErr == nil {
			accepted <- conn
		}
	}()

	cfg := Config{SSHTimeout: 25 * time.Millisecond, HostKeyCallback: ssh.InsecureIgnoreHostKey()}
	started := time.Now()
	_, err = dialSSH(context.Background(), ln.Addr().String(), "", testPrivateKeyPEM, cfg)
	if err == nil {
		t.Fatal("expected stalled SSH handshake to time out")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("SSH handshake exceeded configured timeout: %v", elapsed)
	}
	select {
	case conn := <-accepted:
		_ = conn.Close()
	default:
	}
}

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
		got := nextBackoff(tt.current, initialReconnectBackoff, tt.max)
		if got != tt.expected {
			t.Errorf("nextBackoff(%v, %v, %v) = %v, want %v", tt.current, initialReconnectBackoff, tt.max, got, tt.expected)
		}
	}
}

func TestNextBackoffUsesConfiguredInitial(t *testing.T) {
	initial := 25 * time.Millisecond
	if got := nextBackoff(0, initial, time.Second); got != initial {
		t.Fatalf("nextBackoff initial = %v, want %v", got, initial)
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
		{"ETIMEDOUT", syscall.ETIMEDOUT, false},
		{"wrapped ECONNRESET", fmt.Errorf("dial: %w", syscall.ECONNRESET), true},
		{"net.OpError reset", &net.OpError{Op: "read", Err: syscall.ECONNRESET}, true},
		{"net.OpError timeout", &net.OpError{Op: "read", Err: syscall.ETIMEDOUT}, false},
		{"ssh transport closed", errors.New("ssh: tcp transport closed"), true},
		{"context.Canceled", context.Canceled, false},
		{"context.DeadlineExceeded", context.DeadlineExceeded, false},
		{"wrapped context.Canceled", fmt.Errorf("dial: %w", context.Canceled), false},
		{"wrapped context.DeadlineExceeded", fmt.Errorf("dial: %w", context.DeadlineExceeded), false},
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

// TestDomainEntry_AbortIgnoresRemovedTransport verifies stale maintenance work
// cannot abort a replacement transport.
func TestDomainEntry_AbortIgnoresRemovedTransport(t *testing.T) {
	entry := newDomainEntry("test")

	old := newPooledSSHClient(newSSHClient(t))
	entry.clients = []*pooledSSHClient{old}

	// Replace the old transport as if reconnect had succeeded.
	newClient := newPooledSSHClient(newSSHClient(t))
	entry.clients = []*pooledSSHClient{newClient}

	entry.abort(old, "test")
	if len(entry.clients) != 1 || entry.clients[0] != newClient {
		t.Fatal("replacement transport should remain untouched")
	}

	entry.abort(newClient, "test")
	if len(entry.clients) != 0 {
		t.Fatal("expected the current transport to be aborted")
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
