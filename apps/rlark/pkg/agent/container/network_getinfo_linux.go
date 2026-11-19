//go:build linux

package container

import (
	"context"
	"fmt"
	"os"

	"github.com/moby/sys/mountinfo"
)

func (a *containerNetworkAdapter) getProcessNetworkInfo(ctx context.Context, pid int32) (string, string, error) {
	f, err := os.Open(fmt.Sprintf("/proc/%d/mountinfo", pid))
	if err != nil {
		return "", "", fmt.Errorf("open mountinfo for pid %d: %w", pid, err)
	}
	defer func() { _ = f.Close() }()

	mounts, err := mountinfo.GetMountsFromReader(f, mountinfo.SingleEntryFilter("/etc/hosts"))
	if err != nil {
		return "", "", fmt.Errorf("parse mountinfo for pid %d: %w", pid, err)
	}
	if len(mounts) == 0 {
		return "", "", fmt.Errorf("no /etc/hosts mount found for pid %d", pid)
	}

	podUID, ok := podUIDFromHostsSource(mounts[0].Root)
	if ok {
		return "pod", podUID, nil
	}

	return "", "", fmt.Errorf("unsupported container runtime (hosts source: %s)", mounts[0].Root)
}
