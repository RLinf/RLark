//go:build linux

package netmac

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// ---------------------------------------------------------------------------
// Active ARP probe (dependency-free)
//
// pkg/netmac used to pick an unused robot-subnet IP by reading the host's
// *passive* ARP cache. That is stale and TOCTOU-prone: the cache does not
// update synchronously when a macvlan is created or (crucially) when the pod
// holding it dies, so back-to-back selections could hand out the same IP and
// a dead pod's IP never gets reclaimed. An in-process registry would have
// the opposite problem — it leaks (there is no pod-death callback; the only
// RPC is Setup).
//
// Instead we *actively* ARP-probe each candidate from the host netns: an IP
// is free iff nothing on the host's L2 segment replies. This is live and
// self-cleaning — a dead pod's macvlan vanishes with its netns, so its IP
// stops replying and is reclaimable, with no table to leak. The probe is
// implemented with raw AF_PACKET sockets + an in-process netns switch
// (setns), so it needs no external binary (no arping, no nsenter here).
//
// The device plugin runs hostPID:true + privileged but NOT hostNetwork, so
// its own netns is the pod's; the macvlan lives on the host's physical NIC,
// hence the probe must run in the host netns. We enter it on a locked OS
// thread (setns is per-thread) only long enough to open the raw socket and
// read the NIC's index + MAC; the socket is pinned to the host netns it was
// created in, so it keeps talking to the host's physical NIC afterwards from
// any goroutine. If anything in that setup fails (no CAP_NET_RAW, host netns
// unreachable, bad interface) the caller falls back to the passive cache.
// ---------------------------------------------------------------------------

const (
	// arpProbeWindow is how long a batch listens for replies. On a healthy
	// LAN a used host replies within a millisecond, so a batch's cost is
	// ~arpProbeWindow (dominated by waiting to confirm a free candidate is
	// truly unanswered). Tuned for snappiness with a safety margin for busy
	// links.
	arpProbeWindow = 500 * time.Millisecond

	// arpProbeBatch is how many candidates are probed per round. A round
	// sends one ARP request per candidate back-to-back (broadcast) then
	// reads replies, so its cost is ~arpProbeWindow regardless of batch
	// size. Larger batches reduce the number of rounds when the subnet is
	// nearly full; 16 keeps concurrent memory/send pressure modest.
	arpProbeBatch = 16

	// arpRecvTimeout is the per-Recvfrom wait (SO_RCVTIMEO). Kept short so
	// the read loop stays responsive to the wall-clock batch deadline.
	arpRecvTimeout = 100 * time.Millisecond
)

// runInHostNetns runs fn with the calling thread switched into the host's
// network namespace (/proc/1/ns/net), restoring the original netns afterward.
// setns is per-thread, so the goroutine is locked to a dedicated OS thread
// for the duration; the original netns is always restored, even on error.
// A socket opened inside fn remains bound to the host netns after the thread
// returns — a socket's netns is fixed at creation — so it can be used later
// from any goroutine to send/receive on the host's physical NIC.
func runInHostNetns(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	orig, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open current netns: %w", err)
	}
	defer func() { _ = unix.Close(orig) }()

	target, err := unix.Open("/proc/1/ns/net", unix.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open host netns (/proc/1/ns/net): %w", err)
	}
	defer func() { _ = unix.Close(target) }()

	if err := unix.Setns(target, unix.CLONE_NEWNET); err != nil {
		return fmt.Errorf("setns into host netns: %w", err)
	}
	err = fn()
	_ = unix.Setns(orig, unix.CLONE_NEWNET) // always restore
	return err
}

// arpProbe sends ARP requests and reads replies on the host's physical NIC
// through a raw AF_PACKET socket. It is constructed inside the host netns
// (so the socket binds to the host interface) and may be used afterwards
// from any goroutine.
type arpProbe struct {
	fd      int
	ifindex int
	srcMAC  net.HardwareAddr
	srcIP   net.IP
}

// newARPProbe resolves the host NIC's index + MAC and opens a raw ARP socket,
// all inside the host netns. Returns an error if the host netns cannot be
// entered or the raw socket cannot be opened (e.g. insufficient privilege);
// the caller falls back to passive ARP-cache selection in that case.
func newARPProbe(nic string, srcIP net.IP) (*arpProbe, error) {
	p := &arpProbe{srcIP: srcIP.To4()}
	err := runInHostNetns(func() error {
		idx, err := readHostIfindex(nic)
		if err != nil {
			return fmt.Errorf("ifindex %s: %w", nic, err)
		}
		mac, err := readHostMAC(nic)
		if err != nil {
			return fmt.Errorf("mac %s: %w", nic, err)
		}
		fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_ARP)))
		if err != nil {
			return fmt.Errorf("socket AF_PACKET: %w", err)
		}
		// Bound the per-call receive wait so the read loop stays responsive
		// to the batch's wall-clock deadline (see probeBatchFree).
		if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO,
			&unix.Timeval{Sec: 0, Usec: int64(arpRecvTimeout / time.Microsecond)}); err != nil {
			_ = unix.Close(fd)
			return fmt.Errorf("setsockopt SO_RCVTIMEO: %w", err)
		}
		p.fd = fd
		p.ifindex = idx
		p.srcMAC = mac
		return nil
	})
	if err != nil {
		if p.fd > 0 {
			_ = unix.Close(p.fd)
		}
		return nil, err
	}
	return p, nil
}

// Close releases the raw socket.
func (p *arpProbe) Close() {
	if p != nil && p.fd > 0 {
		_ = unix.Close(p.fd)
		p.fd = -1
	}
}

// probeBatchFree sends a broadcast ARP request for each candidate (rapid-
// fire) and reads replies for arpProbeWindow, returning the subset that got
// NO reply (free). Used candidates typically reply within a millisecond on a
// healthy LAN. The loop exits early once every candidate has replied (all
// used → no point waiting out the window). Replies that arrive during the
// send burst are held in the socket's receive buffer.
//
// Best-effort, like all ARP-based conflict detection: a host that silently
// ignores ARP could be misreported as free. The Setup self-test (probing the
// host's own IP, which the host always answers) validates that the probe
// mechanism itself is working, not that every host replies.
func (p *arpProbe) probeBatchFree(candidates []net.IP) map[string]bool {
	free := make(map[string]bool, len(candidates))
	want := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		want[c.To4().String()] = true
	}

	pkt := make([]byte, 60) // 14 eth + 28 arp + 18 padding (Ethernet min frame)
	dst := &unix.SockaddrLinklayer{
		Protocol: htons(unix.ETH_P_ARP),
		Ifindex:  p.ifindex,
		Halen:    6,
		Addr:     [8]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
	}
	for _, c := range candidates {
		buildARPRequestInto(pkt, p.srcMAC, p.srcIP, c)
		if err := unix.Sendto(p.fd, pkt, 0, dst); err != nil {
			log.Printf("[netmac] arp send to %s: %v", c, err)
		}
	}

	buf := make([]byte, 1514)
	deadline := time.Now().Add(arpProbeWindow)
	replied := make(map[string]bool, len(candidates))
	for time.Now().Before(deadline) {
		if len(replied) >= len(want) {
			break // every candidate replied → none free in this batch
		}
		n, _, err := unix.Recvfrom(p.fd, buf, 0)
		if err != nil {
			if errors.Is(err, unix.EAGAIN) || errors.Is(err, unix.EWOULDBLOCK) {
				continue // SO_RCVTIMEO hit — keep going until the batch deadline
			}
			log.Printf("[netmac] arp recv: %v", err)
			break
		}
		if spa, ok := parseARPReply(buf[:n]); ok && want[spa] {
			replied[spa] = true
		}
	}

	for _, c := range candidates {
		if !replied[c.To4().String()] {
			free[c.To4().String()] = true
		}
	}
	return free
}

// buildARPRequestInto fills pkt (≥60 bytes) with a broadcast ARP request:
// "who has dstIP? tell srcIP (srcMAC)". The frame is zeroed first and padded
// to 60 bytes (Ethernet minimum payload). Layout: 14-byte Ethernet II
// header + 28-byte ARP; offsets:
//
//	[0:6]   dst MAC (broadcast)
//	[6:12]  src MAC
//	[12:14] ethertype 0x0806 (ARP)
//	[14:16] htype 0x0001 (Ethernet)
//	[16:18] ptype 0x0800 (IPv4)
//	[18]    hlen 6
//	[19]    plen 4
//	[20:22] oper 0x0001 (request)
//	[22:28] sha (sender MAC)
//	[28:32] spa (sender IP)
//	[32:38] tha (target MAC, 0 for a request)
//	[38:42] tpa (target IP)
func buildARPRequestInto(pkt []byte, srcMAC net.HardwareAddr, srcIP, dstIP net.IP) {
	for i := range pkt {
		pkt[i] = 0
	}
	copy(pkt[0:6], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
	copy(pkt[6:12], srcMAC)
	pkt[12], pkt[13] = 0x08, 0x06 // ethertype ARP
	pkt[14], pkt[15] = 0x00, 0x01 // htype Ethernet
	pkt[16], pkt[17] = 0x08, 0x00 // ptype IPv4
	pkt[18] = 6                   // hlen
	pkt[19] = 4                   // plen
	pkt[20], pkt[21] = 0x00, 0x01 // oper: request
	copy(pkt[22:28], srcMAC)
	copy(pkt[28:32], srcIP.To4())
	copy(pkt[38:42], dstIP.To4())
}

// parseARPReply returns the sender protocol address (spa) of an ARP reply
// frame as a dotted-quad string, or ok=false if the frame is not an ARP
// reply. The spa of a reply is the replying host's own IP — i.e. the
// candidate that was just probed — so a reply marks that candidate as used.
func parseARPReply(frame []byte) (spa string, ok bool) {
	if len(frame) < 42 {
		return "", false
	}
	if frame[12] != 0x08 || frame[13] != 0x06 { // not ARP
		return "", false
	}
	if frame[20] != 0x00 || frame[21] != 0x02 { // not reply
		return "", false
	}
	return net.IP(frame[28:32]).String(), true
}

// htons converts a uint16 from host to network byte order. AF_PACKET's
// protocol field is a __be16 stored raw in the sockaddr, so the value must
// be in network order regardless of CPU endianness.
func htons(v uint16) uint16 { return (v<<8)&0xff00 | (v>>8)&0xff }

// readHostIfindex reads the interface index from /sys/class/net/<nic>/ifindex.
// Called inside the host netns so the value reflects the host's interface.
func readHostIfindex(nic string) (int, error) {
	b, err := os.ReadFile("/sys/class/net/" + nic + "/ifindex")
	if err != nil {
		return 0, err
	}
	idx, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || idx <= 0 {
		return 0, fmt.Errorf("bad ifindex %q", strings.TrimSpace(string(b)))
	}
	return idx, nil
}

// readHostMAC reads the interface MAC from /sys/class/net/<nic>/address.
// Called inside the host netns so the value reflects the host's interface.
func readHostMAC(nic string) (net.HardwareAddr, error) {
	b, err := os.ReadFile("/sys/class/net/" + nic + "/address")
	if err != nil {
		return nil, err
	}
	return net.ParseMAC(strings.TrimSpace(string(b)))
}
