package domain

import (
	"encoding/binary"
	"fmt"
	"net/netip"
)

// IPPool manages IP address allocation from a CIDR range.
//
// It tracks which IPs have been allocated and hands out the next free one
// via Allocate(). MarkAllocated() reserves a specific IP (useful for restoring
// prior allocations from DomainStatus on reconciliation).
//
// Usable IP range logic:
//   - IPv4 /31 and /32: all IPs usable (RFC 3021 PtP).
//   - Other IPv4: first (network) and last (broadcast) excluded.
//   - IPv6 /127 and /128: all IPs usable.
//   - Other IPv6: first (subnet-router anycast) excluded.
type IPPool struct {
	cidr   string
	prefix netip.Prefix
	first  netip.Addr
	last   netip.Addr

	allocated map[string]bool
}

// NewIPPool creates an IPPool from a CIDR string.
// Returns an error if the CIDR is empty or malformed.
func NewIPPool(cidr string) (*IPPool, error) {
	if cidr == "" {
		return nil, fmt.Errorf("CIDR cannot be empty")
	}
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %q: %w", cidr, err)
	}
	prefix = prefix.Masked()

	first := prefix.Addr()
	last := lastAddr(prefix)
	bits := prefix.Bits()

	if first.Is4() {
		// IPv4: skip network + broadcast address unless /31 or /32
		if bits < 31 {
			first = first.Next()
			last = last.Prev()
		}
	} else {
		// IPv6: skip subnet-router anycast unless /127 or /128
		if bits < 127 {
			first = first.Next()
		}
	}

	return &IPPool{
		cidr:      cidr,
		prefix:    prefix,
		first:     first,
		last:      last,
		allocated: make(map[string]bool),
	}, nil
}

// MarkAllocated marks ip as already in use.
// No-op if the IP is outside the pool's CIDR or if already allocated.
func (p *IPPool) MarkAllocated(ip string) {
	addr, err := netip.ParseAddr(ip)
	if err != nil || !p.prefix.Contains(addr) {
		return
	}
	p.allocated[ip] = true
}

// Allocate returns the next free IP from the pool (linear scan from first).
// Returns an error when every IP is exhausted.
func (p *IPPool) Allocate() (string, error) {
	for cur := p.first; ; cur = cur.Next() {
		ipStr := cur.String()
		if !p.allocated[ipStr] {
			p.allocated[ipStr] = true
			return ipStr, nil
		}
		if cur == p.last {
			break
		}
	}
	return "", fmt.Errorf("no available IP in pool %s", p.cidr)
}

// PrefixLength returns the prefix length.
func (p *IPPool) PrefixLength() int {
	return p.prefix.Bits()
}

// lastAddr computes the last (broadcast for IPv4) address in the given prefix.
func lastAddr(p netip.Prefix) netip.Addr {
	addr := p.Addr()
	bits := p.Bits()

	if addr.Is4() {
		n := addr.As4()
		hostMask := uint32(0xFFFFFFFF) >> bits
		ip := binary.BigEndian.Uint32(n[:])
		ip |= hostMask
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], ip)
		return netip.AddrFrom4(b)
	}

	// IPv6
	n := addr.As16()
	hostBits := 128 - bits
	for i := 15; i >= 0 && hostBits > 0; i-- {
		if hostBits >= 8 {
			n[i] = 0xFF
			hostBits -= 8
		} else {
			n[i] |= byte((1 << hostBits) - 1)
			hostBits = 0
		}
	}
	return netip.AddrFrom16(n)
}
