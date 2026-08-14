//go:build linux

package utils

import (
	"fmt"
	"net"
	"os"
	"syscall"
)

// GetPeerPID returns the PID of the process on the other end of a Unix
// domain socket connection, read from the kernel peer credentials
// (SO_PEERCRED). The PID is in the caller's PID namespace — with
// hostPID:true it is the host PID of the peer (e.g. a pod's init
// container).
//
// Mirrors apps/rlark/pkg/network/nodeserver.GetPeerProcess; duplicated
// here to avoid a cross-module dependency for a single syscall helper.
func GetPeerPID(conn net.Conn) (int, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("conn is not a UnixConn")
	}
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("syscall conn: %w", err)
	}
	var ucred *syscall.Ucred
	var credErr error
	ctrlErr := rawConn.Control(func(fd uintptr) {
		ucred, credErr = syscall.GetsockoptUcred(
			int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if ctrlErr != nil {
		return 0, fmt.Errorf("call control: %w", ctrlErr)
	}
	if credErr != nil {
		return 0, fmt.Errorf("get sockopt ucred: %w", credErr)
	}
	return int(ucred.Pid), nil
}

// IsHostNetwork reports whether pid shares the host's network namespace —
// i.e. the pod is running with hostNetwork: true. It compares the netns
// inode of pid against the host's init namespace (PID 1). Both PIDs must be
// visible to the caller (e.g. the device plugin pod runs with hostPID:true).
// When the peer is in the host netns, namespace-targeting setup (such as
// creating a macvlan) should be skipped — it would land in the host netns
// and disrupt the node instead of isolating the pod.
func IsHostNetwork(pid int) (bool, error) {
	return sameNetns(pid, 1)
}

// sameNetns reports whether a and b share a network namespace by comparing
// the (dev, inode) of /proc/<a>/ns/net and /proc/<b>/ns/net.
func sameNetns(a, b int) (bool, error) {
	aDev, aIno, err := netnsStat(a)
	if err != nil {
		return false, fmt.Errorf("stat netns of pid %d: %w", a, err)
	}
	bDev, bIno, err := netnsStat(b)
	if err != nil {
		return false, fmt.Errorf("stat netns of pid %d: %w", b, err)
	}
	return aDev == bDev && aIno == bIno, nil
}

// netnsStat returns the device and inode of the network namespace file for
// pid (/proc/<pid>/ns/net). stat() follows the nsfs symlink and returns the
// namespace's own (st_dev, st_ino); equal values mean equal namespaces.
func netnsStat(pid int) (uint64, uint64, error) {
	fi, err := os.Stat(fmt.Sprintf("/proc/%d/ns/net", pid))
	if err != nil {
		return 0, 0, err
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, fmt.Errorf("not *syscall.Stat_t")
	}
	return uint64(stat.Dev), stat.Ino, nil
}
