//go:build !linux

package netmac

import "fmt"

// ErrMacvlanLinuxOnly is returned by MACVLAN setup on non-Linux platforms,
// where netlink and macvlan interfaces are unavailable. The struct and
// constructor remain usable cross-platform (they hold no state needing the
// kernel); only the lifecycle methods are Linux-only.
var ErrMacvlanLinuxOnly = fmt.Errorf("netmac: macvlan setup is only supported on linux")

// Create on non-Linux always fails — netlink/macvlan are Linux-only. The
// negative-PID guard (and only it) runs first, matching the Linux path, so a
// programming error is caught before this platform error surfaces.
func (m *MACVLAN) Create() error {
	if m.PID < 0 {
		return fmt.Errorf("create macvlan %s: invalid pid %d (must be >= 0)", m.Name, m.PID)
	}
	return ErrMacvlanLinuxOnly
}

// Destroy is a no-op on non-Linux (Create never succeeded, so nothing was
// created).
func (m *MACVLAN) Destroy() {}
