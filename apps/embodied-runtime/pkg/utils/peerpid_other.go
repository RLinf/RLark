//go:build !linux && !darwin

package utils

import (
	"fmt"
	"net"
)

// GetPeerPID is not available on this platform. Callers that need it only
// run on Linux nodes.
func GetPeerPID(conn net.Conn) (int, error) {
	return 0, fmt.Errorf("GetPeerPID is not supported on this platform")
}

// IsHostNetwork is not available on this platform.
func IsHostNetwork(pid int) (bool, error) {
	return false, fmt.Errorf("IsHostNetwork is not supported on this platform")
}
