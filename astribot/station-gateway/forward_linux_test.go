//go:build linux

package main

import (
	"net"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/vishvananda/netlink"
)

// chain lists one of this program's chains and the entries into it, as iptables -S does.
func chain(t *testing.T, table, hook, name string) []string {
	t.Helper()
	listed, err := iptables("", "-w", "5", "-t", table, "-S")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, line := range strings.Split(strings.TrimSpace(listed), "\n") {
		if strings.Contains(line, name) || strings.Contains(line, "rlark-station-forwarder-v1") {
			lines = append(lines, line)
		}
	}
	return lines
}

// Run only in a disposable network namespace with CAP_NET_ADMIN, with iptables.
func TestKernelForwards(t *testing.T) {
	if os.Getenv("STATION_NETNS_TEST") != "1" {
		t.Skip("requires disposable privileged network namespace")
	}
	for name, address := range map[string]string{"lan-test": "192.0.2.10/24", "tun-test": "10.253.0.1/32"} {
		link := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: name}}
		addr, _ := netlink.ParseAddr(address)
		if err := netlink.LinkAdd(link); err != nil {
			t.Fatal(err)
		}
		if err := netlink.AddrAdd(link, addr); err != nil {
			t.Fatal(err)
		}
		if err := netlink.LinkSetUp(link); err != nil {
			t.Fatal(err)
		}
	}
	lan, _ := netlink.LinkByName("lan-test")
	tun, _ := netlink.LinkByName("tun-test")
	_, beyond, _ := net.ParseCIDR("198.51.100.0/24")
	_, back, _ := net.ParseCIDR("203.0.113.0/24")
	for _, route := range []netlink.Route{{LinkIndex: lan.Attrs().Index, Dst: beyond, Gw: net.ParseIP("192.0.2.1")}, {LinkIndex: tun.Attrs().Index, Dst: back}} {
		if err := netlink.RouteAdd(&route); err != nil {
			t.Fatal(err)
		}
	}

	forwards := []forward{{Destination: "198.51.100.0/24", Sources: []string{"10.253.0.2", "10.253.0.4"}}}
	if err := applyForwards(forwards, "tun-test"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"-N RLARK-STATION-FORWARD",
		"-A FORWARD -j RLARK-STATION-FORWARD",
		"-A RLARK-STATION-FORWARD -s 10.253.0.2/32 -d 198.51.100.0/24 -i tun-test -o lan-test -j ACCEPT",
		"-A RLARK-STATION-FORWARD -s 10.253.0.4/32 -d 198.51.100.0/24 -i tun-test -o lan-test -j ACCEPT",
		"-A RLARK-STATION-FORWARD -s 198.51.100.0/24 -i lan-test -o tun-test -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT",
	}
	wantNAT := []string{
		"-N RLARK-STATION-SNAT",
		"-A POSTROUTING -j RLARK-STATION-SNAT",
		"-A RLARK-STATION-SNAT -s 10.253.0.2/32 -d 198.51.100.0/24 -o lan-test -j SNAT --to-source 192.0.2.10",
		"-A RLARK-STATION-SNAT -s 10.253.0.4/32 -d 198.51.100.0/24 -o lan-test -j SNAT --to-source 192.0.2.10",
	}
	if got := chain(t, "filter", "FORWARD", forwardChain); !slices.Equal(got, want) {
		t.Fatalf("filter:\n%s", strings.Join(got, "\n"))
	}
	if got := chain(t, "nat", "POSTROUTING", snatChain); !slices.Equal(got, wantNAT) {
		t.Fatalf("nat:\n%s", strings.Join(got, "\n"))
	}
	// What is as stated is not written again: the rules keep their counters.
	if _, err := iptables("", "-w", "5", "-Z", forwardChain); err != nil {
		t.Fatal(err)
	}
	if _, err := iptables("", "-w", "5", "-A", "FORWARD", "-s", "10.253.0.4", "-d", "198.51.100.7", "-p", "tcp", "--dport", "445",
		"-m", "comment", "--comment", "rlark-station-forwarder-v1", "-j", "ACCEPT"); err != nil {
		t.Fatal(err)
	}
	// A source fewer, and the retired forwarder's rule goes with the change.
	forwards[0].Sources = forwards[0].Sources[1:]
	if err := applyForwards(forwards, "tun-test"); err != nil {
		t.Fatal(err)
	}
	if got := chain(t, "filter", "FORWARD", forwardChain); !slices.Equal(got, slices.Delete(slices.Clone(want), 2, 3)) {
		t.Fatalf("filter after a source left:\n%s", strings.Join(got, "\n"))
	}
	if err := applyForwards(forwards, "tun-test"); err != nil {
		t.Fatal(err)
	}

	if err := applyForwards([]forward{{Destination: "203.0.113.0/24", Sources: []string{"10.253.0.2"}}}, "tun-test"); err == nil {
		t.Fatal("forwarded a network routed back into the tunnel")
	}
	if err := applyForwards(nil, "tun-test"); err != nil {
		t.Fatal(err)
	}
	if got := slices.Concat(chain(t, "filter", "FORWARD", forwardChain), chain(t, "nat", "POSTROUTING", snatChain)); len(got) > 0 {
		t.Fatalf("left after forwarding nothing:\n%s", strings.Join(got, "\n"))
	}
	if err := applyForwards(nil, "tun-test"); err != nil {
		t.Fatal(err)
	}
}

// The hub of tests/forward-namespace.sh, which sends packets through what this sets.
func TestKernelForwardsTheNAS(t *testing.T) {
	if os.Getenv("STATION_FORWARD_TOPOLOGY") != "1" {
		t.Skip("run by tests/forward-namespace.sh")
	}
	if err := applyForwards([]forward{{Destination: "10.11.5.12/32", Sources: []string{"10.254.240.2", "10.254.240.4"}}}, "wg-test"); err != nil {
		t.Fatal(err)
	}
}
