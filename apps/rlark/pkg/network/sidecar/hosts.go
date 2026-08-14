package sidecar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

const (
	// hostsBeginMarker / hostsEndMarker delimit the section of the hosts file
	// managed by the rlark sidecar. Only the content between these markers is
	// modified on each sync; everything outside is preserved untouched.
	hostsBeginMarker = "# BEGIN rlark sidecar"
	hostsEndMarker   = "# END rlark sidecar"
)

// hostsSyncer periodically fetches host entries from the NodeServer and
// updates the managed section of the hosts file.
type hostsSyncer struct {
	transport http.RoundTripper
	hostsFile string
	interval  time.Duration
}

// newHostsSyncer creates a new hostsSyncer.
func newHostsSyncer(transport http.RoundTripper, hostsFile string, interval time.Duration) *hostsSyncer {
	return &hostsSyncer{
		transport: transport,
		hostsFile: hostsFile,
		interval:  interval,
	}
}

// Run starts the periodic sync loop. It performs an initial sync immediately,
// then retries at every interval. It returns nil when ctx is cancelled.
func (h *hostsSyncer) Run(ctx context.Context) error {
	logger := log.FromContext(ctx)

	if err := h.syncOnce(ctx); err != nil {
		logger.Info("Initial hosts sync failed", "err", err)
	}

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := h.syncOnce(ctx); err != nil {
				logger.Info("Hosts sync failed", "err", err)
			}
		case <-ctx.Done():
			return nil
		}
	}
}

// syncOnce fetches hosts from the NodeServer and, on success, updates the
// managed section of the hosts file.
func (h *hostsSyncer) syncOnce(ctx context.Context) error {
	logger := log.FromContext(ctx)

	hosts, err := h.fetchHosts(ctx)
	if err != nil {
		return fmt.Errorf("fetch hosts: %w", err)
	}

	if err := h.updateHostsFile(hosts); err != nil {
		return fmt.Errorf("update hosts file: %w", err)
	}

	logger.V(1).Info("Hosts file synced", "entries", len(hosts))
	return nil
}

// fetchHosts calls the NodeServer /get_hosts endpoint, which returns a JSON
// object mapping hostname to IP.
func (h *hostsSyncer) fetchHosts(ctx context.Context) (map[string]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/get_hosts", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	client := &http.Client{
		Transport: h.transport,
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

	var hosts map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&hosts); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return hosts, nil
}

// updateHostsFile replaces the managed section (between the BEGIN/END markers)
// of the hosts file with the given entries. Content outside the markers is
// preserved untouched. If no managed section exists, one is appended.
func (h *hostsSyncer) updateHostsFile(hosts map[string]string) error {
	section := buildManagedSection(hosts)

	content, err := os.ReadFile(h.hostsFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read hosts file: %w", err)
		}
		// File doesn't exist; create it with just our section.
		return os.WriteFile(h.hostsFile, []byte(section), 0644)
	}

	newContent, err := replaceManagedSection(string(content), section)
	if err != nil {
		return err
	}

	// Skip writing if nothing changed.
	if newContent == string(content) {
		return nil
	}

	return os.WriteFile(h.hostsFile, []byte(newContent), 0644)
}

// buildManagedSection renders the managed block (including markers) from a
// hostname-to-IP map. Entries are sorted by hostname for deterministic output.
func buildManagedSection(hosts map[string]string) string {
	type entry struct {
		host string
		ip   string
	}
	entries := make([]entry, 0, len(hosts))
	for host, ip := range hosts {
		entries = append(entries, entry{host: host, ip: ip})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].host < entries[j].host
	})

	var b strings.Builder
	b.WriteString(hostsBeginMarker)
	b.WriteByte('\n')
	for _, e := range entries {
		b.WriteString(e.ip)
		b.WriteByte('\t')
		b.WriteString(e.host)
		b.WriteByte('\n')
	}
	b.WriteString(hostsEndMarker)
	b.WriteByte('\n')
	return b.String()
}

// replaceManagedSection inserts or replaces the managed section within the
// existing hosts file content, preserving everything outside the markers.
func replaceManagedSection(content, section string) (string, error) {
	lines := strings.Split(content, "\n")

	beginIdx, endIdx := -1, -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if beginIdx == -1 && trimmed == hostsBeginMarker {
			beginIdx = i
			continue
		}
		if beginIdx != -1 && trimmed == hostsEndMarker {
			endIdx = i
			break
		}
	}

	switch {
	case beginIdx == -1:
		// No managed section found; append one.
		trimmed := strings.TrimRight(content, "\n")
		if trimmed != "" {
			trimmed += "\n"
		}
		return trimmed + section, nil

	case endIdx == -1:
		// Begin marker found but no end marker: treat everything from the
		// begin marker to EOF as the managed section and replace it.
		var b strings.Builder
		for i := 0; i < beginIdx; i++ {
			b.WriteString(lines[i])
			b.WriteByte('\n')
		}
		b.WriteString(section)
		return b.String(), nil

	default:
		// Replace the existing managed section in place.
		var b strings.Builder
		for i := 0; i < beginIdx; i++ {
			b.WriteString(lines[i])
			b.WriteByte('\n')
		}
		b.WriteString(section)
		for i := endIdx + 1; i < len(lines); i++ {
			b.WriteString(lines[i])
			if i < len(lines)-1 {
				b.WriteByte('\n')
			}
		}
		return b.String(), nil
	}
}
