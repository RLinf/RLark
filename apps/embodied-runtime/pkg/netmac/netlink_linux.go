//go:build linux

package netmac

import (
	"errors"
	"fmt"

	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"
	"golang.org/x/sys/unix"
)

// newNetlinkHandle returns a netlink handle bound to the network namespace of
// the given PID — 0 selects the current netns (netlink.NewHandle binds to the
// calling process's netns), 1 the host, N the netns of PID N. The returned
// cleanup function releases the handle (and the netns reference when one was
// opened); callers must call it when done.
//
// The device plugin runs hostPID:true + privileged, so it can open any PID's
// netns and holds CAP_NET_ADMIN in every netns it can reach — which is what
// link/addr/route netlink operations require. vishvananda/netlink binds its
// socket to the target netns internally (via setns on a locked thread), so
// callers here do not manage threads or namespaces themselves. This replaces
// the former `nsenter --target <pid> --net -- ip ...` shell-outs.
func newNetlinkHandle(pid int) (h *netlink.Handle, cleanup func(), err error) {
	if pid == 0 {
		h, err = netlink.NewHandle()
		if err != nil {
			return nil, nil, fmt.Errorf("netlink handle (current netns): %w", err)
		}
		return h, func() { h.Close() }, nil
	}
	ns, err := netns.GetFromPid(pid)
	if err != nil {
		return nil, nil, fmt.Errorf("netns of pid %d: %w", pid, err)
	}
	h, err = netlink.NewHandleAt(ns)
	if err != nil {
		_ = ns.Close()
		return nil, nil, fmt.Errorf("netlink handle (pid %d): %w", pid, err)
	}
	return h, func() { h.Close(); _ = ns.Close() }, nil
}

// isAlreadyExists reports whether err is "already exists" (EEXIST). Used to
// keep macvlan configuration idempotent — reusing a leftover interface from a
// previous container instance must not fail on a duplicate addr/route. Netlink
// ACKs surface as a raw syscall.Errno, so errors.Is matches it directly (this
// replaces the former output-string "File exists" parsing).
func isAlreadyExists(err error) bool {
	return err != nil && errors.Is(err, unix.EEXIST)
}
