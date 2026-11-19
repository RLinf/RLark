package sshdialer

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

func (d *DialerManager) getOrCreate(info DomainInfo) *domainEntry {
	d.mu.RLock()
	entry, ok := d.domains[info.ID]
	d.mu.RUnlock()
	if !ok {
		d.mu.Lock()
		entry, ok = d.domains[info.ID]
		if !ok {
			entry = &domainEntry{
				id:             info.ID,
				domainInfo:     info,
				generation:     1,
				initialBackoff: d.cfg.InitialReconnectBackoff,
				maxBackoff:     d.cfg.MaxReconnectBackoff,
			}
			entry.touch()
			d.domains[info.ID] = entry
		}
		d.mu.Unlock()
	}

	entry.mu.Lock()
	entry.touch()
	if entry.domainInfo != info {
		entry.domainInfo = info
		entry.generation++
		entry.reconnectBackoff = 0
		if entry.reconnectCancel != nil {
			entry.reconnectCancel()
		}
		clients := append([]*pooledSSHClient(nil), entry.clients...)
		entry.mu.Unlock()
		for _, client := range clients {
			entry.drain(client, "domain-config-changed", d.cfg.KeepaliveDrainGrace)
		}
		return entry
	}
	entry.mu.Unlock()
	return entry
}

func (entry *domainEntry) borrow(ctx context.Context, d *DialerManager) (*pooledSSHClient, error) {
	for {
		entry.mu.Lock()
		if entry.removed {
			entry.mu.Unlock()
			return nil, ErrDomainRemoved
		}
		if d != nil && entry.activeChannelsLocked() >= d.cfg.MaxChannelsPerDomain {
			entry.mu.Unlock()
			return nil, ErrOverloaded
		}
		client := entry.borrowCandidateLocked()
		if client != nil {
			client.active++
			client.touch()
			entry.touch()
			if d != nil && client.active >= d.cfg.MaxChannelsPerConnection && len(entry.clients) < d.cfg.MaxConnectionsPerDomain && !entry.reconnecting {
				entry.startReconnectLocked(d)
			}
			entry.mu.Unlock()
			return client, nil
		}
		if d == nil {
			entry.mu.Unlock()
			return nil, fmt.Errorf("no SSH transport available")
		}
		if !entry.reconnecting {
			entry.startReconnectLocked(d)
			ch := entry.reconnectCh
			entry.mu.Unlock()
			if err := entry.waitForReconnect(ctx, d, ch); err != nil {
				return nil, err
			}
		} else {
			ch := entry.reconnectCh
			entry.mu.Unlock()
			if err := entry.waitForReconnect(ctx, d, ch); err != nil {
				return nil, err
			}
		}
	}
}

func (entry *domainEntry) startReconnectLocked(d *DialerManager) {
	entry.reconnecting = true
	entry.reconnectCh = make(chan struct{})
	reconnectCtx, reconnectCancel := context.WithCancel(d.ctx)
	entry.reconnectCancel = reconnectCancel
	info := entry.domainInfo
	generation := entry.generation
	backoff := entry.reconnectBackoff
	d.wg.Add(1)
	go entry.reconnect(reconnectCtx, d, info, generation, backoff)
}

func (entry *domainEntry) waitForReconnect(ctx context.Context, d *DialerManager, ch <-chan struct{}) error {
	select {
	case <-ch:
		entry.mu.Lock()
		err := entry.lastReconnectErr
		entry.mu.Unlock()
		if err != nil {
			return fmt.Errorf("ssh reconnect: %w", err)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-d.ctx.Done():
		return ErrClosed
	}
}

func (entry *domainEntry) activeChannelsLocked() int {
	active := 0
	for _, client := range entry.clients {
		active += client.active
	}
	return active
}

func (entry *domainEntry) leastLoadedLocked() *pooledSSHClient {
	return entry.selectLeastLoadedLocked(false)
}

func (entry *domainEntry) borrowCandidateLocked() *pooledSSHClient {
	return entry.selectLeastLoadedLocked(true)
}

func (entry *domainEntry) selectLeastLoadedLocked(advance bool) *pooledSSHClient {
	if len(entry.clients) == 0 {
		return nil
	}

	var selected *pooledSSHClient
	selectedIndex := -1
	for offset := range entry.clients {
		index := (entry.next + offset) % len(entry.clients)
		client := entry.clients[index]
		if client.draining || (client.generation != 0 && client.generation != entry.generation) {
			continue
		}
		if selected == nil || client.active < selected.active {
			selected = client
			selectedIndex = index
		}
	}
	if advance && selectedIndex >= 0 {
		entry.next = (selectedIndex + 1) % len(entry.clients)
	}
	return selected
}

func (entry *domainEntry) release(client *pooledSSHClient) {
	entry.mu.Lock()
	if client.active > 0 {
		client.active--
	}
	client.touch()
	entry.touch()
	if client.draining && client.active == 0 {
		entry.removeLocked(client)
		client.close()
	}
	entry.mu.Unlock()
}

func (entry *domainEntry) reconnect(ctx context.Context, d *DialerManager, info DomainInfo, generation uint64, backoff time.Duration) {
	defer d.wg.Done()
	var err error
	if backoff > 0 {
		wait := backoff + time.Duration(rand.Int63n(max(1, int64(backoff/2))))
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			err = ErrClosed
		}
	}

	var client *pooledSSHClient
	if err == nil {
		var sshClientErr error
		sshClient, dialErr := d.dialSSHWithMergedCtx(ctx, info)
		sshClientErr = dialErr
		err = sshClientErr
		if err == nil {
			client = newPooledSSHClient(sshClient)
			client.generation = generation
		}
	}

	entry.mu.Lock()
	stale := generation != entry.generation
	if client != nil && !stale && !d.closed.Load() {
		entry.clients = append(entry.clients, client)
		entry.reconnectBackoff = 0
		entry.lastReconnectErr = nil
	} else {
		if client != nil {
			client.close()
		}
		if stale {
			err = nil
		} else if !errors.Is(err, ErrClosed) {
			entry.reconnectBackoff = nextBackoff(entry.reconnectBackoff, entry.initialBackoff, entry.maxBackoff)
		}
		entry.lastReconnectErr = err
	}
	entry.reconnecting = false
	entry.reconnectCancel = nil
	ch := entry.reconnectCh
	entry.reconnectCh = nil
	entry.mu.Unlock()
	if ch != nil {
		close(ch)
	}

	if client != nil && !stale && !d.closed.Load() {
		go func() {
			defer close(client.keepaliveExit)
			entry.keepaliveLoop(client, d.cfg.KeepaliveInterval, d.cfg.KeepaliveTimeout, d.cfg.KeepaliveDrainGrace)
		}()
		if d.cfg.OnReconnect != nil {
			d.cfg.OnReconnect(entry.id)
		}
	}
}

func nextBackoff(current, initial, maxBackoff time.Duration) time.Duration {
	if initial <= 0 {
		initial = initialReconnectBackoff
	}
	if maxBackoff <= 0 {
		maxBackoff = maxReconnectBackoff
	}
	if current <= 0 {
		return min(initial, maxBackoff)
	}
	return min(current*2, maxBackoff)
}

func (entry *domainEntry) abort(client *pooledSSHClient, reason string) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if !entry.containsLocked(client) || client.draining {
		return
	}
	client.draining = true
	log.GetLogger().Info("SSH transport aborted",
		"domain", entry.id,
		"reason", reason,
		"activeChannels", client.active,
		"lastUsed", client.lastUsed(),
		"idleFor", time.Since(client.lastUsed()).Round(time.Second),
	)
	entry.removeLocked(client)
	client.close()
}

func (entry *domainEntry) drain(client *pooledSSHClient, reason string, grace time.Duration) {
	entry.mu.Lock()
	if !entry.containsLocked(client) || client.draining {
		entry.mu.Unlock()
		return
	}
	client.draining = true
	log.GetLogger().Info("SSH connection draining",
		"domain", entry.id,
		"reason", reason,
		"activeChannels", client.active,
		"grace", grace,
	)
	if client.active == 0 {
		entry.removeLocked(client)
		client.close()
		entry.mu.Unlock()
		return
	}
	entry.mu.Unlock()

	entry.mu.Lock()
	client.drainTimer = time.AfterFunc(grace, func() {
		entry.mu.Lock()
		defer entry.mu.Unlock()
		if entry.containsLocked(client) {
			entry.removeLocked(client)
			client.close()
		}
	})
	entry.mu.Unlock()
}

func (entry *domainEntry) containsLocked(client *pooledSSHClient) bool {
	for _, candidate := range entry.clients {
		if candidate == client {
			return true
		}
	}
	return false
}

func (entry *domainEntry) removeLocked(client *pooledSSHClient) {
	for i, candidate := range entry.clients {
		if candidate == client {
			entry.clients = append(entry.clients[:i], entry.clients[i+1:]...)
			if len(entry.clients) == 0 {
				entry.next = 0
			} else {
				if i < entry.next {
					entry.next--
				}
				entry.next %= len(entry.clients)
			}
			return
		}
	}
}
