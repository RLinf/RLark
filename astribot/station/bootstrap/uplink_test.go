package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Run only as root of a disposable network namespace that can make another
// (unshare -rnm, /run/netns a tmpfs): a robot's computer behind its router.
func TestKernelUplinkCheck(t *testing.T) {
	if os.Getenv("STATION_NETNS_TEST") != "1" {
		t.Skip("requires a disposable privileged network namespace")
	}
	ip := func(arguments string) {
		t.Helper()
		if output, err := exec.Command("ip", strings.Fields(arguments)...).CombinedOutput(); err != nil {
			t.Fatalf("ip %s: %v: %s", arguments, err, output)
		}
	}
	ip("netns add router")
	ip("link add inner type veth peer name eth0 netns router")
	ip("addr add 192.168.0.10/24 dev inner")
	ip("link set inner up")
	ip("-n router addr add 192.168.0.11/24 dev eth0")
	ip("-n router link set eth0 up")
	ip("-n router link set lo up")
	ip("route add default via 192.168.0.11")
	const server = "https://10.11.14.128:18443"

	if err := uplinkCheck(server, ""); err != nil {
		t.Fatalf("a machine that dials from its own address behind the router: %v", err)
	}
	if err := uplinkCheck("https://192.168.0.11:18443", ""); err != nil {
		t.Fatalf("a server in the machine's own network: %v", err)
	}

	// The address of the venue's network that nothing gave the machine.
	ip("addr add 192.168.31.251/32 dev inner")
	ip("route add 10.11.0.0/16 via 192.168.0.11 src 192.168.31.251")
	err := uplinkCheck(server, "")
	if err == nil || !strings.Contains(err.Error(), "dials out from 192.168.31.251") || !strings.Contains(err.Error(), "gateway 192.168.0.11 (there it has 192.168.0.10)") {
		t.Fatalf("a machine with a foreign source address: %v", err)
	}
}
