package domain

import (
	"testing"
)

func TestNewIPPool_Allocate(t *testing.T) {
	pool, err := NewIPPool("10.0.0.0/30")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ip1, err := pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip1 != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %s", ip1)
	}

	ip2, err := pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip2 != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.2, got %s", ip2)
	}

	// /30 has 4 IPs total, 2 usable after excluding network/broadcast
	_, err = pool.Allocate()
	if err == nil {
		t.Fatal("expected error (pool exhausted), got nil")
	}
}

func TestNewIPPool_Allocate_31(t *testing.T) {
	pool, err := NewIPPool("10.0.0.0/31")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ip1, err := pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip1 != "10.0.0.0" {
		t.Fatalf("expected 10.0.0.0, got %s", ip1)
	}

	ip2, err := pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip2 != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %s", ip2)
	}

	_, err = pool.Allocate()
	if err == nil {
		t.Fatal("expected error (pool exhausted), got nil")
	}
}

func TestNewIPPool_Allocate_32(t *testing.T) {
	pool, err := NewIPPool("10.0.0.5/32")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ip, err := pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.5" {
		t.Fatalf("expected 10.0.0.5, got %s", ip)
	}

	_, err = pool.Allocate()
	if err == nil {
		t.Fatal("expected error (pool exhausted), got nil")
	}
}

func TestNewIPPool_MarkAllocated(t *testing.T) {
	pool, err := NewIPPool("10.0.0.0/29")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	pool.MarkAllocated("10.0.0.2")

	// Should skip 10.0.0.2
	ip, err := pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.1" {
		t.Fatalf("expected 10.0.0.1, got %s", ip)
	}

	ip, err = pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "10.0.0.3" {
		t.Fatalf("expected 10.0.0.3, got %s", ip)
	}

	// MarkAllocated with out-of-range IP should be no-op
	pool.MarkAllocated("10.1.0.1")
	pool.MarkAllocated("invalid-ip")
}

func TestNewIPPool_MarkAllocated_BeforeAllocate(t *testing.T) {
	pool, err := NewIPPool("10.0.0.0/29")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Mark all IPs first, then allocate should fail
	pool.MarkAllocated("10.0.0.1")
	pool.MarkAllocated("10.0.0.2")
	pool.MarkAllocated("10.0.0.3")
	pool.MarkAllocated("10.0.0.4")
	pool.MarkAllocated("10.0.0.5")
	pool.MarkAllocated("10.0.0.6")

	_, err = pool.Allocate()
	if err == nil {
		t.Fatal("expected error (pool exhausted), got nil")
	}
}

func TestNewIPPool_AllocateSequential(t *testing.T) {
	pool, err := NewIPPool("192.168.1.0/28")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	} // 14 usable IPs (192.168.1.1 - 192.168.1.14)

	var ips []string
	for i := 0; i < 14; i++ {
		ip, err := pool.Allocate()
		if err != nil {
			t.Fatalf("unexpected error at iteration %d: %v", i, err)
		}
		ips = append(ips, ip)
	}

	expected := []string{
		"192.168.1.1", "192.168.1.2", "192.168.1.3", "192.168.1.4",
		"192.168.1.5", "192.168.1.6", "192.168.1.7", "192.168.1.8",
		"192.168.1.9", "192.168.1.10", "192.168.1.11", "192.168.1.12",
		"192.168.1.13", "192.168.1.14",
	}
	for i, ip := range ips {
		if ip != expected[i] {
			t.Fatalf("index %d: expected %s, got %s", i, expected[i], ip)
		}
	}

	// Pool should be exhausted
	_, err = pool.Allocate()
	if err == nil {
		t.Fatal("expected error (pool exhausted), got nil")
	}
}

func TestNewIPPool_V6(t *testing.T) {
	pool, err := NewIPPool("2001:db8::/126")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	} // 4 IPs total, 3 usable after skipping subnet-router

	ip1, err := pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip1 != "2001:db8::1" {
		t.Fatalf("expected 2001:db8::1, got %s", ip1)
	}

	_, _ = pool.Allocate()
	_, _ = pool.Allocate()

	_, err = pool.Allocate()
	if err == nil {
		t.Fatal("expected error (pool exhausted), got nil")
	}
}

func TestNewIPPool_V6_127(t *testing.T) {
	pool, err := NewIPPool("2001:db8::/127")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ip1, err := pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip1 != "2001:db8::" {
		t.Fatalf("expected 2001:db8::, got %s", ip1)
	}

	_, _ = pool.Allocate()

	_, err = pool.Allocate()
	if err == nil {
		t.Fatal("expected error (pool exhausted), got nil")
	}
}

func TestNewIPPool_V6_128(t *testing.T) {
	pool, err := NewIPPool("2001:db8::1/128")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ip, err := pool.Allocate()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ip != "2001:db8::1" {
		t.Fatalf("expected 2001:db8::1, got %s", ip)
	}

	_, err = pool.Allocate()
	if err == nil {
		t.Fatal("expected error (pool exhausted), got nil")
	}
}

func TestNewIPPool_EmptyCIDR(t *testing.T) {
	_, err := NewIPPool("")
	if err == nil {
		t.Fatal("expected error for empty CIDR, got nil")
	}
}

func TestNewIPPool_InvalidCIDR(t *testing.T) {
	_, err := NewIPPool("not-a-cidr")
	if err == nil {
		t.Fatal("expected error for invalid CIDR, got nil")
	}
}
