//go:build darwin

package container

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

func (a *containerNetworkAdapter) getProcessNetworkInfo(ctx context.Context, pid int32) (string, string, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/mountinfo", pid))
	if err != nil {
		return "", "", fmt.Errorf("open mountinfo for pid %d: %w", pid, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}

		leftFields := strings.Fields(parts[0])

		if len(leftFields) < 5 {
			continue
		}

		if leftFields[4] != "/etc/hosts" { // mount point
			continue
		}

		podUID, ok := podUIDFromHostsSource(leftFields[3]) // root
		if ok {
			return "pod", podUID, nil
		}

		return "", "", fmt.Errorf("unsupported container runtime (/etc/hosts source: %s)", leftFields[3])
	}
	if err := scanner.Err(); err != nil {
		return "", "", fmt.Errorf("scan mountinfo for pid %d: %w", pid, err)
	}

	return "", "", fmt.Errorf("no /etc/hosts mount found for pid %d", pid)
}
