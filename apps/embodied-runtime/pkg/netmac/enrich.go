package netmac

import (
	"fmt"
	"net"
	"strings"
)

// ---------------------------------------------------------------------------
// MACVLAN auto-enrichment — pure helpers (no host access)
// ---------------------------------------------------------------------------

// parseIPNet parses an "ip[/prefix]" string into the IP and *net.IPNet.
// When the prefix is omitted it defaults to /24, the typical robot-network
// prefix. Only IPv4 is supported — anything else is an error.
func parseIPNet(s string) (net.IP, *net.IPNet, error) {
	if s == "" {
		return nil, nil, fmt.Errorf("empty ip")
	}
	if !strings.Contains(s, "/") {
		s = s + "/24"
	}
	ip, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return nil, nil, fmt.Errorf("parse %q: %w", s, err)
	}
	if ip.To4() == nil {
		return nil, nil, fmt.Errorf("not an IPv4 address: %s", s)
	}
	return ip, ipNet, nil
}

// isNetworkAddress reports whether ip is the all-zero host part of ipNet —
// i.e. the caller wrote a placeholder like "172.16.0.0/24" instead of a
// concrete host address.
func isNetworkAddress(ip net.IP, ipNet *net.IPNet) bool {
	return ip.Equal(ipNet.IP)
}

// isBroadcast reports whether ip is the broadcast address of ipNet
// (network | ~mask). Only meaningful for IPv4.
func isBroadcast(ip net.IP, ipNet *net.IPNet) bool {
	ip4 := ip.To4()
	mask := ipNet.Mask
	base := ipNet.IP.To4()
	if ip4 == nil || len(mask) != 4 || base == nil {
		return false
	}
	bcast := net.IPv4(
		base[0]|^mask[0],
		base[1]|^mask[1],
		base[2]|^mask[2],
		base[3]|^mask[3],
	).To4()
	return ip4.Equal(bcast)
}

// ValidateMACVLANConfig reports whether a MACVLANConfig is ready to be
// created — Name and HostNIC must be set, and IP must be a concrete IPv4
// host address (not the network or broadcast address, which signal an
// unresolved placeholder). Call after EnrichMACVLANConfig to fail fast
// on configs that could not be auto-completed.
func ValidateMACVLANConfig(cfg MACVLANConfig) error {
	if cfg.Name == "" {
		return fmt.Errorf("macvlan: name is required")
	}
	if cfg.HostNIC == "" {
		return fmt.Errorf("macvlan %q: host_nic is required", cfg.Name)
	}
	ip, ipNet, err := parseIPNet(cfg.IP)
	if err != nil {
		return fmt.Errorf("macvlan %q: invalid ip %q: %w", cfg.Name, cfg.IP, err)
	}
	if isNetworkAddress(ip, ipNet) {
		return fmt.Errorf("macvlan %q: ip %s is the network address (unresolved placeholder)", cfg.Name, cfg.IP)
	}
	if isBroadcast(ip, ipNet) {
		return fmt.Errorf("macvlan %q: ip %s is the broadcast address", cfg.Name, cfg.IP)
	}
	return nil
}

// hostNIC is a host network interface with its first IPv4 address and mask,
// as reported by `ip -o -4 addr show`.
type hostNIC struct {
	Name string
	IP   net.IP
	Mask net.IPMask
}

// findNICForSubnet returns the first NIC whose subnet contains ip. Used to
// auto-detect HostNIC when the user leaves it blank: the configured IP pins
// the subnet, and the host interface in that subnet is the one to attach
// the macvlan to.
func findNICForSubnet(nics []hostNIC, ip net.IP) (string, net.IP, bool) {
	for _, n := range nics {
		nicNet := &net.IPNet{IP: n.IP, Mask: n.Mask}
		if nicNet.Contains(ip) {
			return n.Name, n.IP, true
		}
	}
	return "", nil, false
}

// ipv4OnNIC returns the IPv4 address of the named NIC, if present.
func ipv4OnNIC(nics []hostNIC, name string) (net.IP, bool) {
	for _, n := range nics {
		if n.Name == name {
			return n.IP, true
		}
	}
	return nil, false
}

// pickUnusedIP returns the first IPv4 in ipNet not in `used`, scanning the
// last octet from startOctet upward and wrapping at 254 back to 1. Any
// candidate outside the subnet (e.g. for narrow prefixes like /30) is
// skipped. Returns nil if every candidate host IP is taken.
//
// Best-effort for the common /24 robot network: candidates are limited to
// the last-octet range of the network base, so for prefixes wider than /24
// only the first /24 slice is scanned.
func pickUnusedIP(ipNet *net.IPNet, used map[string]bool, startOctet int) net.IP {
	for _, c := range candidateIPs(ipNet, nil, startOctet) {
		if !used[c.String()] {
			return c
		}
	}
	return nil
}

// candidateIPs returns the host IPs of ipNet in priority order for ARP-probe
// based selection: the last octet scans from startOctet upward, wrapping at
// 254 back to 1, skipping the network and broadcast addresses. hostIP (the
// host's own address, never handed out) is excluded when non-nil. Used by
// both the passive fallback (pickUnusedIP, via a `used` set filter) and the
// active probe (probeFreeIP, which probes each candidate in turn). Best-
// effort for the common /24 robot network — like pickUnusedIP, only the
// first /24 slice is scanned for prefixes wider than /24.
func candidateIPs(ipNet *net.IPNet, hostIP net.IP, startOctet int) []net.IP {
	base := ipNet.IP.To4()
	if base == nil {
		return nil
	}
	m := ipNet.Mask
	if len(m) != 4 {
		return nil
	}
	// Broadcast = network | ~mask — not a valid host, must be skipped.
	broadcast := net.IPv4(
		base[0]|^m[0],
		base[1]|^m[1],
		base[2]|^m[2],
		base[3]|^m[3],
	).To4()
	var hostStr string
	if hostIP != nil {
		if h4 := hostIP.To4(); h4 != nil {
			hostStr = h4.String()
		}
	}
	if startOctet < 1 {
		startOctet = 1
	}
	if startOctet > 254 {
		startOctet = 100
	}
	out := make([]net.IP, 0, 254)
	for n := 0; n < 254; n++ {
		octet := startOctet + n
		if octet > 254 {
			octet -= 254
		}
		cand := net.IPv4(base[0], base[1], base[2], byte(octet)).To4()
		if !ipNet.Contains(cand) {
			continue // outside subnet (e.g. /30, /29)
		}
		if cand.Equal(broadcast) {
			continue // broadcast, not a host
		}
		if hostStr != "" && cand.String() == hostStr {
			continue // the host's own address — never hand out
		}
		out = append(out, cand)
	}
	return out
}
