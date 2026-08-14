package netmac

import (
	"net"
	"testing"
)

// TestIsBroadcast covers the /24 and /30 broadcast detection.
func TestIsBroadcast(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"172.16.0.255/24", true},
		{"172.16.0.0/24", false}, // network, not broadcast
		{"172.16.0.5/24", false},
		{"192.168.1.3/30", true}, // /30 broadcast
		{"192.168.1.1/30", false},
	}
	for _, c := range cases {
		ip, ipNet, err := parseIPNet(c.ip)
		if err != nil {
			t.Fatalf("parseIPNet(%q): %v", c.ip, err)
		}
		if got := isBroadcast(ip, ipNet); got != c.want {
			t.Errorf("isBroadcast(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// TestValidateMACVLANConfig verifies that unresolved placeholders and
// missing required fields are rejected, and a fully-resolved config passes.
func TestValidateMACVLANConfig(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := MACVLANConfig{Name: "macvlan0", HostNIC: "eno1", IP: "172.16.0.100/24"}
		if err := ValidateMACVLANConfig(cfg); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("missing_name", func(t *testing.T) {
		cfg := MACVLANConfig{HostNIC: "eno1", IP: "172.16.0.100/24"}
		if err := ValidateMACVLANConfig(cfg); err == nil {
			t.Error("expected error for empty name")
		}
	})
	t.Run("missing_host_nic", func(t *testing.T) {
		cfg := MACVLANConfig{Name: "macvlan0", IP: "172.16.0.100/24"}
		if err := ValidateMACVLANConfig(cfg); err == nil {
			t.Error("expected error for empty host_nic")
		}
	})
	t.Run("invalid_ip", func(t *testing.T) {
		cfg := MACVLANConfig{Name: "macvlan0", HostNIC: "eno1", IP: "not-an-ip"}
		if err := ValidateMACVLANConfig(cfg); err == nil {
			t.Error("expected error for invalid ip")
		}
	})
	t.Run("network_address_placeholder", func(t *testing.T) {
		cfg := MACVLANConfig{Name: "macvlan0", HostNIC: "eno1", IP: "172.16.0.0/24"}
		if err := ValidateMACVLANConfig(cfg); err == nil {
			t.Error("expected error for unresolved network-address placeholder")
		}
	})
	t.Run("broadcast_address", func(t *testing.T) {
		cfg := MACVLANConfig{Name: "macvlan0", HostNIC: "eno1", IP: "172.16.0.255/24"}
		if err := ValidateMACVLANConfig(cfg); err == nil {
			t.Error("expected error for broadcast address")
		}
	})
}

// TestParseIPNet covers the default-prefix (/24), explicit-prefix, and
// rejection of empty / IPv6 inputs.
func TestParseIPNet(t *testing.T) {
	t.Run("no_prefix_defaults_to_24", func(t *testing.T) {
		ip, ipNet, err := parseIPNet("172.16.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ip.Equal(net.IPv4(172, 16, 0, 0)) {
			t.Errorf("ip = %v", ip)
		}
		ones, _ := ipNet.Mask.Size()
		if ones != 24 {
			t.Errorf("prefix = %d, want 24", ones)
		}
	})
	t.Run("explicit_prefix", func(t *testing.T) {
		_, ipNet, err := parseIPNet("172.16.0.5/16")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		ones, _ := ipNet.Mask.Size()
		if ones != 16 {
			t.Errorf("prefix = %d, want 16", ones)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if _, _, err := parseIPNet(""); err == nil {
			t.Error("expected error for empty ip")
		}
	})
	t.Run("ipv6_rejected", func(t *testing.T) {
		if _, _, err := parseIPNet("2001:db8::1/64"); err == nil {
			t.Error("expected error for IPv6")
		}
	})
	t.Run("invalid", func(t *testing.T) {
		if _, _, err := parseIPNet("not-an-ip"); err == nil {
			t.Error("expected error for invalid ip")
		}
	})
}

// TestIsNetworkAddress verifies the placeholder detection: a network
// address triggers enrichment, a concrete host address does not.
func TestIsNetworkAddress(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"172.16.0.0/24", true},  // network address → placeholder
		{"172.16.0.5/24", false}, // concrete host
		{"172.16.5.0/24", true},  // network of a different /24
		{"10.0.0.0/16", true},    // network /16
		{"10.0.0.5/16", false},
	}
	for _, c := range cases {
		ip, ipNet, err := parseIPNet(c.ip)
		if err != nil {
			t.Fatalf("parseIPNet(%q): %v", c.ip, err)
		}
		if got := isNetworkAddress(ip, ipNet); got != c.want {
			t.Errorf("isNetworkAddress(%s) = %v, want %v", c.ip, got, c.want)
		}
	}
}

// TestPickUnusedIP covers the common /24 case: default start, used entries,
// host avoidance, wrap-around, and exhaustion.
func TestPickUnusedIP(t *testing.T) {
	_, ipNet, _ := parseIPNet("172.16.0.0/24")

	t.Run("empty_picks_100", func(t *testing.T) {
		got := pickUnusedIP(ipNet, nil, 100)
		if got == nil || got.String() != "172.16.0.100" {
			t.Errorf("got %v, want 172.16.0.100", got)
		}
	})
	t.Run("skip_used", func(t *testing.T) {
		used := map[string]bool{"172.16.0.100": true, "172.16.0.101": true}
		got := pickUnusedIP(ipNet, used, 100)
		if got == nil || got.String() != "172.16.0.102" {
			t.Errorf("got %v, want 172.16.0.102", got)
		}
	})
	t.Run("wrap_from_254_to_1", func(t *testing.T) {
		// Fill 100..254 — next should wrap to 1.
		used := map[string]bool{}
		for o := 100; o <= 254; o++ {
			used[net.IPv4(172, 16, 0, byte(o)).String()] = true
		}
		got := pickUnusedIP(ipNet, used, 100)
		if got == nil || got.String() != "172.16.0.1" {
			t.Errorf("got %v, want 172.16.0.1", got)
		}
	})
	t.Run("full_returns_nil", func(t *testing.T) {
		used := map[string]bool{}
		for o := 1; o <= 254; o++ {
			used[net.IPv4(172, 16, 0, byte(o)).String()] = true
		}
		if got := pickUnusedIP(ipNet, used, 100); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
	t.Run("start_below_1_clamped", func(t *testing.T) {
		got := pickUnusedIP(ipNet, nil, 0)
		if got == nil || got.String() != "172.16.0.1" {
			t.Errorf("got %v, want 172.16.0.1", got)
		}
	})
}

// TestPickUnusedIP_NarrowPrefix verifies that candidates outside a narrow
// subnet (e.g. /30) are skipped — the picker must not return an IP that is
// not a valid host in the subnet.
func TestPickUnusedIP_NarrowPrefix(t *testing.T) {
	_, ipNet, _ := parseIPNet("192.168.1.0/30") // only .1 and .2 are hosts
	got := pickUnusedIP(ipNet, nil, 100)
	if got == nil || got.String() != "192.168.1.1" {
		t.Errorf("got %v, want 192.168.1.1", got)
	}
	// .1 taken → .2
	got = pickUnusedIP(ipNet, map[string]bool{"192.168.1.1": true}, 100)
	if got == nil || got.String() != "192.168.1.2" {
		t.Errorf("got %v, want 192.168.1.2", got)
	}
	// both taken → nil
	got = pickUnusedIP(ipNet, map[string]bool{
		"192.168.1.1": true, "192.168.1.2": true,
	}, 100)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

// TestFindNICForSubnet verifies that the NIC whose subnet contains the
// configured IP is selected.
func TestFindNICForSubnet(t *testing.T) {
	nics := []hostNIC{
		{Name: "docker0", IP: net.IPv4(172, 17, 0, 1), Mask: net.CIDRMask(16, 32)},
		{Name: "eno1", IP: net.IPv4(172, 16, 0, 5), Mask: net.CIDRMask(24, 32)},
		{Name: "eth0", IP: net.IPv4(10, 0, 0, 5), Mask: net.CIDRMask(24, 32)},
	}
	cases := []struct {
		ip      string
		wantNic string
		wantOK  bool
	}{
		{"172.16.0.0", "eno1", true}, // network addr, still in eno1's subnet
		{"172.16.0.50", "eno1", true},
		{"10.0.0.99", "eth0", true},
		{"192.168.1.1", "", false}, // no matching NIC
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip).To4()
		name, _, ok := findNICForSubnet(nics, ip)
		if ok != c.wantOK || name != c.wantNic {
			t.Errorf("findNICForSubnet(%s) = %q,%v; want %q,%v",
				c.ip, name, ok, c.wantNic, c.wantOK)
		}
	}
}

// TestIPv4OnNIC verifies lookup by interface name.
func TestIPv4OnNIC(t *testing.T) {
	nics := []hostNIC{
		{Name: "eno1", IP: net.IPv4(172, 16, 0, 5), Mask: net.CIDRMask(24, 32)},
		{Name: "eth0", IP: net.IPv4(10, 0, 0, 5), Mask: net.CIDRMask(24, 32)},
	}
	if ip, ok := ipv4OnNIC(nics, "eno1"); !ok || ip.String() != "172.16.0.5" {
		t.Errorf("ipv4OnNIC(eno1) = %v,%v", ip, ok)
	}
	if _, ok := ipv4OnNIC(nics, "missing"); ok {
		t.Error("expected ok=false for missing NIC")
	}
}

// TestCandidateIPs covers priority order, host-IP exclusion, broadcast skip,
// wrap-around, and narrow-prefix behaviour. candidateIPs feeds the active
// ARP probe (and the passive fallback via pickUnusedIP).
func TestCandidateIPs(t *testing.T) {
	_, ipNet, _ := parseIPNet("172.16.0.0/24")

	t.Run("starts_at_100", func(t *testing.T) {
		got := candidateIPs(ipNet, nil, 100)
		if len(got) == 0 || got[0].String() != "172.16.0.100" {
			t.Errorf("first = %v, want 172.16.0.100", got)
		}
	})

	t.Run("excludes_broadcast", func(t *testing.T) {
		for _, c := range candidateIPs(ipNet, nil, 100) {
			if c.Equal(net.IPv4(172, 16, 0, 255)) {
				t.Fatal("broadcast 172.16.0.255 must not be a candidate")
			}
		}
	})

	t.Run("excludes_host_ip", func(t *testing.T) {
		host := net.IPv4(172, 16, 0, 5)
		for _, c := range candidateIPs(ipNet, host, 100) {
			if c.Equal(host) {
				t.Fatal("host IP 172.16.0.5 must not be a candidate")
			}
		}
	})

	t.Run("wraps_254_to_1", func(t *testing.T) {
		got := candidateIPs(ipNet, nil, 100)
		// Scan octets 100..254 (155) then wrap 1..99 (99) = 254; the .0
		// network and .255 broadcast are outside the 1..254 octet range.
		if len(got) != 254 {
			t.Fatalf("len = %d, want 254", len(got))
		}
		if got[155].String() != "172.16.0.1" { // index 155 = first after 100..254
			t.Errorf("after wrap got[155] = %v, want 172.16.0.1", got[155])
		}
	})

	t.Run("narrow_prefix_only_hosts", func(t *testing.T) {
		_, ipNet30, _ := parseIPNet("192.168.1.0/30") // hosts: .1, .2 (.3 bcast)
		got := candidateIPs(ipNet30, nil, 100)
		if len(got) != 2 || got[0].String() != "192.168.1.1" || got[1].String() != "192.168.1.2" {
			t.Errorf("got %v, want [192.168.1.1 192.168.1.2]", got)
		}
	})
}

// TestNewMACVLAN verifies the constructor copies config fields.
func TestNewMACVLAN(t *testing.T) {
	cfg := MACVLANConfig{
		Name:    "macvlan0",
		HostNIC: "eno1",
		IP:      "172.16.0.100/24",
		Gateway: "172.16.0.1",
	}
	m := NewMACVLAN(cfg)
	if m.Name != "macvlan0" || m.HostNIC != "eno1" ||
		m.IP != "172.16.0.100/24" || m.Gateway != "172.16.0.1" {
		t.Errorf("NewMACVLAN = %+v", m)
	}
}
