//go:build darwin

package utils

import (
	"fmt"
	"net"
	"syscall"
)

// SOL_LOCAL / LOCAL_PEERPID are not exported by golang.org/x/sys on darwin
// for this path; define them to match the kernel headers (see
// <sys/un.h>: LOCAL_PEERPID = 0x002).
const (
	solLocal     = 0
	localPeerPID = 0x002
)

// GetPeerPID returns the PID of the process on the other end of a Unix
// domain socket connection, read via LOCAL_PEERPID (macOS analogue of
// Linux SO_PEERCRED). Included so the package builds on macOS for
// development; the callers that need this only run on Linux nodes.
func GetPeerPID(conn net.Conn) (int, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("conn is not a UnixConn")
	}
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return 0, fmt.Errorf("syscall conn: %w", err)
	}
	var ret int
	var credErr error
	ctrlErr := rawConn.Control(func(fd uintptr) {
		ret, credErr = syscall.GetsockoptInt(
			int(fd), solLocal, localPeerPID)
	})
	if ctrlErr != nil {
		return 0, fmt.Errorf("call control: %w", ctrlErr)
	}
	if credErr != nil {
		return 0, fmt.Errorf("get sockopt peerpid: %w", credErr)
	}
	return ret, nil
}

// IsHostNetwork is not implemented on darwin (no /proc). Callers that need
// this only run on Linux nodes, so this returns an error rather than a
// misleading answer.
func IsHostNetwork(pid int) (bool, error) {
	return false, fmt.Errorf("IsHostNetwork is not supported on darwin")
}
