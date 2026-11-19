//go:build linux

package netmac

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/vishvananda/netlink"
)

// Create attaches a macvlan interface to the target network namespace.
//
// A container restart within the same pod reuses the pause container's
// network namespace, so a macvlan created by a previous container instance
// may still exist here. Create is idempotent: if the interface already
// exists in the target netns it is reused and only its configuration is
// (re)applied, instead of failing on a duplicate add.
//
// Steps (when not reusing an existing interface):
//  1. In the host netns (PID 1), create a macvlan in bridge mode on the
//     host's physical NIC.
//  2. Move the macvlan into the target network namespace (current PID when
//     m.PID == 0, otherwise m.PID) by PID.
//  3. Configure IP, bring up, add route inside the target netns.
//
// All netlink operations use vishvananda/netlink handles bound to the
// relevant netns (host for create/move, target for configure/destroy), so
// no `ip`/`nsenter` binaries are needed — the former nsenter+ip shell-outs
// are gone.
func (m *MACVLAN) Create() error {
	// PID is a host PID; negative values are programming errors that would
	// otherwise surface as confusing netlink/nsenter failures. Treat them as
	// invalid up front.
	if m.PID < 0 {
		return fmt.Errorf("create macvlan %s: invalid pid %d (must be >= 0)", m.Name, m.PID)
	}

	// A leftover macvlan in the target netns means a previous instance
	// already created it. Reuse it and just (re)apply config.
	if m.linkExists() {
		log.Printf("[netmac] %s already exists in netns(pid=%d); reusing", m.Name, m.PID)
		m.created = true
		if err := m.configure(); err != nil {
			m.Destroy()
			return err
		}
		log.Printf("[netmac] reused %s: %s", m.Name, m.IP)
		return nil
	}

	// The PID whose netns we move the macvlan into. PID 0 means "current
	// netns" — resolve it to the current process's PID on the host
	// (hostPID:true makes container PID == host PID).
	targetPID := m.PID
	if targetPID == 0 {
		targetPID = os.Getpid()
	}

	// 1. Create the macvlan on the host's physical NIC (host netns, PID 1).
	hostH, hostCleanup, err := newNetlinkHandle(1)
	if err != nil {
		return fmt.Errorf("create macvlan on host: %w", err)
	}
	defer hostCleanup()

	parent, err := hostH.LinkByName(m.HostNIC)
	if err != nil {
		return fmt.Errorf("create macvlan %s: host nic %q: %w", m.Name, m.HostNIC, err)
	}
	mv := &netlink.Macvlan{
		LinkAttrs: netlink.LinkAttrs{
			Name:        m.Name,
			ParentIndex: parent.Attrs().Index,
		},
		Mode: netlink.MACVLAN_MODE_BRIDGE,
	}
	if err := hostH.LinkAdd(mv); err != nil {
		return fmt.Errorf("create macvlan %s on host %s: %w", m.Name, m.HostNIC, err)
	}

	// 2. Move the macvlan into the target netns. Re-fetch by name to obtain
	// the new link's index — LinkAdd does not populate the struct's Index.
	link, err := hostH.LinkByName(m.Name)
	if err != nil {
		_ = hostH.LinkDel(mv) // best-effort rollback
		return fmt.Errorf("create macvlan %s: re-fetch after add: %w", m.Name, err)
	}
	if err := hostH.LinkSetNsPid(link, targetPID); err != nil {
		_ = hostH.LinkDel(link) // rollback: delete the orphaned macvlan on host
		return fmt.Errorf("move macvlan %s to netns(pid=%d): %w", m.Name, targetPID, err)
	}

	// Mark created before configuring so a configure failure is still cleaned
	// up by Destroy() — the interface now lives in the target netns.
	m.created = true
	if err := m.configure(); err != nil {
		m.Destroy()
		return err
	}
	log.Printf("[netmac] created %s on host %s → netns(pid=%d): %s",
		m.Name, m.HostNIC, targetPID, m.IP)
	return nil
}

// configure assigns the IP, brings the interface up, and adds the default
// route inside the target network namespace. It is idempotent so re-running
// on a reused interface (container restart within the same pod) is safe:
//   - Existing IPv4 addresses are flushed first, so a stale IP from a
//     previous run (e.g. a config change between restarts) is removed instead
//     of accumulating as a second address. Best-effort: a flush failure falls
//     back to the EEXIST tolerance below.
//   - EEXIST from `addr add` / `route add` (already present on a reused
//     interface) is tolerated. `route replace` is deliberately NOT used — it
//     could clobber the pod's eth0 default route.
func (m *MACVLAN) configure() error {
	h, cleanup, err := newNetlinkHandle(m.PID)
	if err != nil {
		return fmt.Errorf("configure macvlan %s: %w", m.Name, err)
	}
	defer cleanup()

	link, err := h.LinkByName(m.Name)
	if err != nil {
		return fmt.Errorf("configure macvlan %s: not found in netns(pid=%d): %w", m.Name, m.PID, err)
	}
	idx := link.Attrs().Index

	// Drop any addresses already on the interface so the subsequent AddrAdd
	// always applies cleanly and never leaves a duplicate.
	if addrs, aerr := h.AddrList(link, netlink.FAMILY_V4); aerr == nil {
		for _, a := range addrs {
			if derr := h.AddrDel(link, &a); derr != nil && !isAlreadyExists(derr) {
				log.Printf("[netmac] WARNING: del addr on %s: %v", m.Name, derr)
			}
		}
	} else {
		log.Printf("[netmac] WARNING: list addrs on %s: %v", m.Name, aerr)
	}

	ip, ipNet, err := net.ParseCIDR(m.IP)
	if err != nil {
		return fmt.Errorf("configure macvlan %s: ip %q: %w", m.Name, m.IP, err)
	}
	// net.ParseCIDR returns the network address in ipNet.IP; override with the
	// parsed host IP so IFA_LOCAL is the host address, not the subnet.
	addr := &netlink.Addr{IPNet: &net.IPNet{IP: ip.To4(), Mask: ipNet.Mask}}
	if err := h.AddrAdd(link, addr); err != nil && !isAlreadyExists(err) {
		return fmt.Errorf("configure macvlan %s: add addr %s: %w", m.Name, m.IP, err)
	}

	if err := h.LinkSetUp(link); err != nil {
		return fmt.Errorf("configure macvlan %s: up: %w", m.Name, err)
	}

	if m.Gateway != "" {
		gw := net.ParseIP(m.Gateway)
		if gw == nil {
			return fmt.Errorf("configure macvlan %s: gateway %q: invalid", m.Name, m.Gateway)
		}
		// Dst == nil ⇒ default route (0.0.0.0/0). NLM_F_EXCL ⇒ EEXIST when a
		// default route already exists (tolerated, see above).
		route := &netlink.Route{LinkIndex: idx, Gw: gw}
		if err := h.RouteAdd(route); err != nil && !isAlreadyExists(err) {
			return fmt.Errorf("configure macvlan %s: route default via %s: %w", m.Name, m.Gateway, err)
		}
	}
	return nil
}

// linkExists reports whether a network interface named m.Name exists in the
// target network namespace (current netns when PID is 0).
func (m *MACVLAN) linkExists() bool {
	h, cleanup, err := newNetlinkHandle(m.PID)
	if err != nil {
		return false
	}
	defer cleanup()
	_, err = h.LinkByName(m.Name)
	return err == nil
}

// Destroy deletes the macvlan interface from the target network namespace.
func (m *MACVLAN) Destroy() {
	if !m.created {
		return
	}
	if h, cleanup, err := newNetlinkHandle(m.PID); err == nil {
		defer cleanup()
		if link, lerr := h.LinkByName(m.Name); lerr == nil {
			if derr := h.LinkDel(link); derr != nil {
				log.Printf("[netmac] delete %s: %v", m.Name, derr)
			}
		}
	} else {
		log.Printf("[netmac] delete %s: netlink handle: %v", m.Name, err)
	}
	m.created = false
	log.Printf("[netmac] deleted %s", m.Name)
}
