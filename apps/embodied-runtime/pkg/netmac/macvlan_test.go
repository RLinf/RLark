package netmac

import (
	"testing"
)

// TestNewMACVLANDefaultPID verifies the default (no creation option) targets
// the current network namespace (PID 0), preserving the original behaviour.
func TestNewMACVLANDefaultPID(t *testing.T) {
	cfg := MACVLANConfig{
		Name:    "macvlan0",
		HostNIC: "eno1",
		IP:      "172.16.0.100/24",
		Gateway: "172.16.0.1",
	}
	m := NewMACVLAN(cfg)
	if m.PID != 0 {
		t.Errorf("default PID = %d, want 0 (current netns)", m.PID)
	}
	if m.Name != "macvlan0" || m.HostNIC != "eno1" ||
		m.IP != "172.16.0.100/24" || m.Gateway != "172.16.0.1" {
		t.Errorf("NewMACVLAN config not copied: %+v", m)
	}
}

// TestNewMACVLANWithPID verifies that the WithPID creation option targets
// another process's network namespace while leaving the declarative config
// fields untouched.
func TestNewMACVLANWithPID(t *testing.T) {
	cfg := MACVLANConfig{
		Name:    "macvlan0",
		HostNIC: "eno1",
		IP:      "172.16.0.100/24",
		Gateway: "172.16.0.1",
	}
	m := NewMACVLAN(cfg, WithPID(4321))
	if m.PID != 4321 {
		t.Errorf("WithPID(4321): PID = %d, want 4321", m.PID)
	}
	if m.Name != "macvlan0" || m.HostNIC != "eno1" ||
		m.IP != "172.16.0.100/24" || m.Gateway != "172.16.0.1" {
		t.Errorf("WithPID clobbered config fields: %+v", m)
	}

	// WithPID(0) explicitly selects the current netns.
	m = NewMACVLAN(cfg, WithPID(0))
	if m.PID != 0 {
		t.Errorf("WithPID(0): PID = %d, want 0", m.PID)
	}
}

// TestCreateRejectsNegativePID verifies that a negative PID — a programming
// error — fails fast at Create before any netlink operation runs, so the
// guard is safe on any platform (on non-Linux Create returns a platform
// error, but only after the same guard).
func TestCreateRejectsNegativePID(t *testing.T) {
	cfg := MACVLANConfig{Name: "macvlan0", HostNIC: "eno1", IP: "172.16.0.100/24"}
	m := NewMACVLAN(cfg, WithPID(-1))
	if err := m.Create(); err == nil {
		t.Error("Create with PID -1: expected error, got nil")
	}
	if m.created {
		t.Error("Create with PID -1: should not mark created")
	}
}
