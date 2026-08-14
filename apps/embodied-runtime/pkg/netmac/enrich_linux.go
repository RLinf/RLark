//go:build linux

package netmac

import (
	"fmt"
	"log"
	"net"

	"github.com/vishvananda/netlink"
)

// EnrichMACVLANConfig fills in placeholders in a MACVLANConfig by inspecting
// the host's network interfaces and ARP cache. The host is reached via a
// netlink handle bound to PID 1's netns (vishvananda/netlink manages the
// setns internally), which works because the controller pod runs with
// hostPID:true + privileged (and in local mode the controller is already on
// the host).
//
// Two placeholders are resolved:
//
//  1. If HostNIC is empty, the host interface whose subnet contains the
//     configured IP is selected automatically.
//
//  2. If the configured IP is the network address (e.g. "172.16.0.0/24"),
//     an unused IP in the subnet is picked automatically, starting at .100
//     and skipping the host's own IP plus any IP with a resolved MAC in the
//     host's ARP cache. The host IP is always avoided even when the ARP
//     cache is empty or unreachable — matching the "avoid the host's IP"
//     fallback.
//
// Best-effort: any error is logged and the config is left with whatever was
// resolved so far; the caller surfaces the failure when the macvlan is
// created.
func EnrichMACVLANConfig(cfg *MACVLANConfig, logPrefix string) {
	if cfg == nil {
		return
	}
	prefix := logPrefix
	if prefix == "" {
		prefix = "[netmac]"
	}
	ip, ipNet, err := parseIPNet(cfg.IP)
	if err != nil {
		log.Printf("%s enrich macvlan %q: %v", prefix, cfg.Name, err)
		return
	}

	nics, err := queryHostNICs()
	if err != nil {
		log.Printf("%s enrich macvlan %q: query host NICs: %v", prefix, cfg.Name, err)
		return
	}

	// Step 1: resolve HostNIC if missing.
	var hostIP net.IP
	if cfg.HostNIC == "" {
		name, nicIP, ok := findNICForSubnet(nics, ip)
		if !ok {
			log.Printf("%s enrich macvlan %q: no host NIC in subnet of %s", prefix, cfg.Name, ip)
			return
		}
		cfg.HostNIC = name
		hostIP = nicIP
		log.Printf("%s enrich macvlan %q: auto-detected host NIC %s (%s)",
			prefix, cfg.Name, name, hostIP)
	} else if nicIP, ok := ipv4OnNIC(nics, cfg.HostNIC); ok {
		hostIP = nicIP
	}

	// Step 2: if IP is the network address, auto-pick an unused IP.
	if !isNetworkAddress(ip, ipNet) {
		return
	}
	newIP := probeFreeIP(ipNet, cfg.HostNIC, hostIP, 100)
	if newIP == nil {
		log.Printf("%s enrich macvlan %q: no unused IP in %s", prefix, cfg.Name, ipNet)
		return
	}
	ones, _ := ipNet.Mask.Size()
	cfg.IP = fmt.Sprintf("%s/%d", newIP.String(), ones)
	log.Printf("%s enrich macvlan %q: auto-picked IP %s", prefix, cfg.Name, cfg.IP)
}

// probeFreeIP returns the first free IPv4 in ipNet (priority order from
// startOctet) confirmed by an active ARP probe run in the host netns: an IP
// is free iff nothing on the host's L2 segment replies to an ARP request for
// it. This is live and self-cleaning — when a pod's netns is torn down its
// macvlan disappears, so the IP stops replying and is reclaimable, with no
// in-process table to leak.
//
// Candidates are probed in parallel batches: a batch sends one ARP request
// per candidate back-to-back (broadcast), then reads replies for a short
// window, so a batch's cost is ~one window regardless of how many candidates
// it holds — it stays fast even when the subnet is nearly full. The
// lowest-index free candidate in the first batch with any free candidate is
// returned (deterministic priority).
//
// A self-test guards against a silently-broken probe: the host replies to
// ARP for its own IP, so if the probe reports the host IP as free we know
// the probe is unusable (wrong netns, no CAP_NET_RAW, bad interface) and fall
// back to the passive ARP-cache selection — never handing out an unverified
// IP. hostIP is also excluded from the candidate list. nic must be the
// host's physical interface on the robot subnet.
func probeFreeIP(ipNet *net.IPNet, nic string, hostIP net.IP, startOctet int) net.IP {
	candidates := candidateIPs(ipNet, hostIP, startOctet)
	if len(candidates) == 0 {
		return nil
	}
	probe, err := newARPProbe(nic, hostIP)
	if err != nil {
		log.Printf("[netmac] active arp probe unavailable (%v); falling back to passive ARP cache", err)
		return passivePick(ipNet, hostIP, startOctet)
	}
	defer probe.Close()

	// Self-test: the host must reply to ARP for its own IP. A "free" result
	// here means the probe is not actually seeing host traffic → unusable.
	if hostIP != nil {
		if free := probe.probeBatchFree([]net.IP{hostIP.To4()}); free[hostIP.To4().String()] {
			log.Printf("[netmac] arp self-test: host IP %s reported free — probe unusable, falling back to passive ARP cache", hostIP)
			return passivePick(ipNet, hostIP, startOctet)
		}
	}

	for i := 0; i < len(candidates); i += arpProbeBatch {
		end := i + arpProbeBatch
		if end > len(candidates) {
			end = len(candidates)
		}
		free := probe.probeBatchFree(candidates[i:end])
		for _, c := range candidates[i:end] {
			if free[c.To4().String()] {
				return c
			}
		}
	}
	return nil
}

// passivePick selects an IP using the passive host ARP cache — the original,
// non-live method. Used as a fallback when the active ARP probe is
// unavailable or fails its self-test. Unlike the active probe it cannot
// reclaim an IP whose pod has died (the ARP cache may still list it), so it
// is best-effort.
func passivePick(ipNet *net.IPNet, hostIP net.IP, startOctet int) net.IP {
	used := readHostARPCache()
	if hostIP != nil {
		used[hostIP.String()] = true
	}
	return pickUnusedIP(ipNet, used, startOctet)
}

// queryHostNICs lists the host's IPv4 interfaces via netlink (LinkList +
// AddrList per link), replacing the former `ip -o -4 addr show` shell-out.
// The host netns is reached through a handle bound to PID 1, so no nsenter is
// needed. The loopback is skipped.
func queryHostNICs() ([]hostNIC, error) {
	h, cleanup, err := newNetlinkHandle(1)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	links, err := h.LinkList()
	if err != nil {
		return nil, fmt.Errorf("netlink link list: %w", err)
	}
	var nics []hostNIC
	for _, l := range links {
		name := l.Attrs().Name
		if name == "lo" {
			continue
		}
		addrs, aerr := h.AddrList(l, netlink.FAMILY_V4)
		if aerr != nil {
			continue
		}
		for _, a := range addrs {
			if a.IPNet == nil {
				continue
			}
			ip := a.IP.To4() // promoted from the embedded *net.IPNet
			if ip == nil {
				continue
			}
			nics = append(nics, hostNIC{Name: name, IP: ip, Mask: a.Mask})
		}
	}
	return nics, nil
}

// readHostARPCache returns the set of IPs that have a resolved MAC in the
// host's neighbor table, via netlink (NeighList) — replacing the former
// `ip neigh show` shell-out. Entries without a MAC (INCOMPLETE/FAILED probes)
// are skipped, since they signal a free address rather than a used one.
// Returns nil on error; the host IP (added by the caller) is still avoided.
func readHostARPCache() map[string]bool {
	h, cleanup, err := newNetlinkHandle(1)
	if err != nil {
		return nil
	}
	defer cleanup()

	neighs, err := h.NeighList(0, netlink.FAMILY_V4)
	if err != nil {
		return nil
	}
	used := map[string]bool{}
	for _, n := range neighs {
		if len(n.HardwareAddr) == 0 || n.IP == nil {
			continue
		}
		used[n.IP.String()] = true
	}
	return used
}
