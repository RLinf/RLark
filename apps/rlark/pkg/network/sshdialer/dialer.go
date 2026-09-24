package sshdialer

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"golang.org/x/crypto/ssh"
)

// DialerManager owns independent SSH transport pools keyed by domain.
//
// Design principles:
//   - GetDialer binds domain identity and credentials; Dialer only handles target dialing.
//   - Each domain reuses multiplexed transports and grows the pool by load up to the configured limit.
//   - Concurrent transport creation is coordinated, and failed reconnects use exponential backoff.
//   - Definite transport failures are closed immediately; keepalive timeouts drain active channels first.
//   - Idle transports are reclaimed in the background and shared state is concurrency-safe.

// pooledSSHClient tracks one multiplexed physical SSH transport.
type pooledSSHClient struct {
	client        *ssh.Client
	generation    uint64
	active        int
	draining      bool
	lastUsedNanos atomic.Int64
	createdAt     time.Time
	keepaliveDone chan struct{}
	keepaliveExit chan struct{}
	drainTimer    *time.Timer
	closeOnce     sync.Once
}

func newPooledSSHClient(client *ssh.Client) *pooledSSHClient {
	p := &pooledSSHClient{
		client:        client,
		createdAt:     time.Now(),
		keepaliveDone: make(chan struct{}),
		keepaliveExit: make(chan struct{}),
	}
	p.touch()
	return p
}

func (p *pooledSSHClient) touch() {
	now := time.Now().UnixNano()
	last := p.lastUsedNanos.Load()
	if now-last >= int64(activityUpdateInterval) {
		p.lastUsedNanos.CompareAndSwap(last, now)
	}
}

func (p *pooledSSHClient) lastUsed() time.Time {
	return time.Unix(0, p.lastUsedNanos.Load())
}

func (p *pooledSSHClient) close() {
	p.closeOnce.Do(func() {
		if p.drainTimer != nil {
			p.drainTimer.Stop()
		}
		close(p.keepaliveDone)
		_ = p.client.Close()
	})
}

type domainEntry struct {
	id            string
	domainInfo    DomainInfo
	generation    uint64
	lastUsedNanos atomic.Int64

	mu      sync.Mutex
	clients []*pooledSSHClient
	next    int

	// Coordinates one manager-owned recovery attempt for this domain.
	reconnecting     bool
	reconnectCh      chan struct{}
	reconnectCancel  context.CancelFunc
	lastReconnectErr error
	reconnectBackoff time.Duration
	initialBackoff   time.Duration
	maxBackoff       time.Duration
	removed          bool
}

func (entry *domainEntry) touch() {
	entry.lastUsedNanos.Store(time.Now().UnixNano())
}

func (entry *domainEntry) lastUsed() time.Time {
	return time.Unix(0, entry.lastUsedNanos.Load())
}

// DialerManager owns SSH transports for all domains.
type DialerManager struct {
	cfg    Config
	closed atomic.Bool

	mu      sync.RWMutex
	domains map[string]*domainEntry

	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeDone chan struct{}
}

// Dialer opens connections through one domain's SSH transports.
type Dialer struct {
	manager *DialerManager
	entry   *domainEntry
}

// NewDialerManager creates and starts an SSH connection manager.
func NewDialerManager(cfg Config) *DialerManager {
	cfg.setDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	d := &DialerManager{
		cfg:       cfg,
		domains:   make(map[string]*domainEntry),
		ctx:       ctx,
		cancel:    cancel,
		closeDone: make(chan struct{}),
	}
	d.wg.Add(1)
	go d.cleanupLoop()
	return d
}

type activityConn struct {
	net.Conn
	onActivity  func()
	onRelease   func()
	onError     func(error)
	releaseOnce sync.Once
	errorOnce   sync.Once
}

func (c *activityConn) release() {
	c.releaseOnce.Do(c.onRelease)
}

func (c *activityConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.onActivity()
	}
	if err != nil && isSSHChannelTransportError(err) {
		c.errorOnce.Do(func() { c.onError(err) })
	}
	if err != nil {
		c.release()
	}
	return n, err
}

func (c *activityConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		c.onActivity()
	}
	if err != nil && isSSHChannelTransportError(err) {
		c.errorOnce.Do(func() { c.onError(err) })
	}
	if err != nil {
		c.release()
	}
	return n, err
}

func (c *activityConn) Close() error {
	err := c.Conn.Close()
	c.release()
	return err
}

// GetDialer returns a dialer bound to a domain. The latest DomainInfo is used
// when a new physical SSH transport is established.
func (d *DialerManager) GetDialer(info DomainInfo) *Dialer {
	entry := d.getOrCreate(info)
	return &Dialer{manager: d, entry: entry}
}

// RemoveDomain removes a domain, closes its transports, and clears credentials.
func (d *DialerManager) RemoveDomain(id string) {
	d.mu.Lock()
	entry := d.domains[id]
	delete(d.domains, id)
	d.mu.Unlock()
	if entry == nil {
		return
	}
	entry.mu.Lock()
	entry.removed = true
	entry.domainInfo = DomainInfo{ID: id}
	entry.mu.Unlock()
	entry.close()
}

// DialContext opens a connection through the domain's SSH transport pool.
func (d *Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("ssh dialer: unsupported network %q", network)
	}
	manager := d.manager
	if manager.closed.Load() {
		return nil, ErrClosed
	}

	entry := d.entry
	pooled, err := entry.borrow(ctx, manager)
	if err != nil {
		return nil, fmt.Errorf("ssh dialer: %w", err)
	}

	conn, err := pooled.client.DialContext(ctx, "tcp", addr)
	if err != nil {
		entry.release(pooled)
		if isSSHTransportError(err) {
			log.GetLogger().Info("SSH channel dial failed with transport error, aborting transport",
				"domain", entry.id,
				"target", addr,
				"err", err,
				"errType", fmt.Sprintf("%T", err),
			)
			entry.abort(pooled, "channel-dial-error")
		}
		return nil, fmt.Errorf("ssh proxy to %s: %w", addr, err)
	}

	return &activityConn{
		Conn:       conn,
		onActivity: pooled.touch,
		onRelease:  func() { entry.release(pooled) },
		onError: func(err error) {
			log.GetLogger().Info("SSH channel I/O failed with transport error, aborting transport",
				"domain", entry.id,
				"target", addr,
				"err", err,
				"errType", fmt.Sprintf("%T", err),
			)
			entry.abort(pooled, "channel-io-error")
		},
	}, nil
}

// Dial opens a connection through the domain's SSH transport pool.
func (d *Dialer) Dial(network, addr string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, addr)
}

// Close closes every managed transport and stops background maintenance.
func (d *DialerManager) Close() error {
	d.closeOnce.Do(func() {
		d.closed.Store(true)
		d.cancel()
		d.mu.RLock()
		entries := make([]*domainEntry, 0, len(d.domains))
		for _, entry := range d.domains {
			entries = append(entries, entry)
		}
		d.mu.RUnlock()
		for _, entry := range entries {
			entry.close()
		}
		d.wg.Wait()
		close(d.closeDone)
	})
	<-d.closeDone
	return nil
}

// Stats returns the number of transports available for new channels.
func (d *DialerManager) Stats() (open int) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, entry := range d.domains {
		entry.mu.Lock()
		for _, client := range entry.clients {
			if !client.draining {
				open++
			}
		}
		entry.mu.Unlock()
	}
	return
}

// PoolStats describes current physical transports and multiplexed channels.
type PoolStats struct {
	Domains      int
	Open         int
	Draining     int
	Reconnecting int
	Channels     int
}

// PoolStats returns a concurrency-safe pool snapshot.
func (d *DialerManager) PoolStats() PoolStats {
	var stats PoolStats
	d.mu.RLock()
	defer d.mu.RUnlock()
	stats.Domains = len(d.domains)
	for _, entry := range d.domains {
		entry.mu.Lock()
		if entry.reconnecting {
			stats.Reconnecting++
		}
		for _, client := range entry.clients {
			stats.Channels += client.active
			if client.draining {
				stats.Draining++
			} else {
				stats.Open++
			}
		}
		entry.mu.Unlock()
	}
	return stats
}
