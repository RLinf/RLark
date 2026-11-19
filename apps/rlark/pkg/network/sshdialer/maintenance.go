package sshdialer

import (
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

func (entry *domainEntry) keepaliveLoop(client *pooledSSHClient, interval, timeout, drainGrace time.Duration) {
	logger := log.GetLogger()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			result := make(chan error, 1)
			go func() {
				_, _, err := client.client.SendRequest(keepaliveRequest, true, nil)
				result <- err
			}()
			timer := time.NewTimer(timeout)
			select {
			case err := <-result:
				if !timer.Stop() {
					<-timer.C
				}
				if err == nil {
					continue
				}
				logger.Info("SSH keepalive failed, aborting transport",
					"domain", entry.id,
					"err", err,
					"errType", fmt.Sprintf("%T", err),
				)
				entry.abort(client, "keepalive-failed")
				return
			case <-timer.C:
				logger.Info("SSH keepalive timed out, draining connection",
					"domain", entry.id,
					"timeout", timeout,
				)
				entry.drain(client, "keepalive-timeout", drainGrace)
				return
			case <-client.keepaliveDone:
				if !timer.Stop() {
					<-timer.C
				}
				return
			}
		case <-client.keepaliveDone:
			return
		}
	}
}

func (entry *domainEntry) close() {
	entry.mu.Lock()
	clients := entry.clients
	entry.clients = nil
	if entry.reconnectCancel != nil {
		entry.reconnectCancel()
	}
	for _, client := range clients {
		client.close()
	}
	entry.mu.Unlock()
	for _, client := range clients {
		select {
		case <-client.keepaliveExit:
		default:
			// Test-created clients may not have a maintenance goroutine.
		}
	}
}

func (d *DialerManager) cleanupLoop() {
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

func (d *DialerManager) cleanup() {
	cutoff := time.Now().Add(-d.cfg.IdleTimeout)
	d.mu.RLock()
	entries := make([]*domainEntry, 0, len(d.domains))
	for _, entry := range d.domains {
		entries = append(entries, entry)
	}
	d.mu.RUnlock()

	for _, entry := range entries {
		entry.mu.Lock()
		kept := entry.clients[:0]
		for _, client := range entry.clients {
			if client.active == 0 && (client.lastUsed().Before(cutoff) || time.Since(client.createdAt) >= d.cfg.MaxConnectionAge) {
				client.close()
				continue
			}
			if !client.draining && time.Since(client.createdAt) >= d.cfg.MaxConnectionAge {
				client.draining = true
			}
			kept = append(kept, client)
		}
		entry.clients = kept
		removeEntry := len(entry.clients) == 0 && !entry.reconnecting && entry.lastUsed().Before(cutoff)
		entry.mu.Unlock()
		if removeEntry {
			d.mu.Lock()
			if d.domains[entry.id] == entry {
				delete(d.domains, entry.id)
				entry.mu.Lock()
				entry.domainInfo = DomainInfo{ID: entry.id}
				entry.mu.Unlock()
			}
			d.mu.Unlock()
		}
	}
}
