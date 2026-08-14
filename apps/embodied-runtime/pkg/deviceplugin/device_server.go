package deviceplugin

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/netmac"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/utils"
	devicepb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/device/v1"
	"google.golang.org/grpc"
)

// ---------------------------------------------------------------------------
// DeviceServer — on-demand device gRPC service exposed by the device
// plugin over a Unix socket (inside RunDir, already mounted into
// requesting pods). A pod's init container runs the `devinit` CLI, which
// connects to the socket and calls Setup; the server reads the caller's
// PID from the socket peer credentials (SO_PEERCRED) to target that PID's
// namespace.
//
// DeviceServer is the general host for on-demand device setup that cannot
// be delivered via Allocate (which only does device mounts). Macvlan setup
// is one such service — the only one today: a macvlan is a network
// interface that must be created inside the target netns, so it cannot be
// mounted. It is created via pkg/netmac (NewMACVLAN + WithPID), and pods
// using hostNetwork are detected (their netns == host netns) and skipped
// so a macvlan is never dropped into the host netns. As more on-demand
// device services are needed they can be hosted here alongside the
// macvlan setup RPC.
// ---------------------------------------------------------------------------

// DeviceServer implements device.v1.DeviceService (the macvlan setup RPC,
// the first service it hosts). It owns the Unix socket listener and gRPC
// server, and serves the node-level host_macvlans config to whichever pod
// asks.
type DeviceServer struct {
	devicepb.UnimplementedDeviceServiceServer

	cfgs   []netmac.MACVLANConfig
	socket string

	// srv is set in Start and read in Stop. mu guards started + srv so Stop
	// does not race with a concurrent/incomplete Start.
	mu      sync.Mutex
	srv     *grpc.Server
	started bool

	// setupMu serializes host-side macvlan work across concurrent Setup
	// calls (gRPC runs each RPC in its own goroutine, so multiple pods
	// starting on the same node race). Two TOCTOU windows exist: (1) the
	// per-call IP auto-pick in EnrichMACVLANConfig reads the host ARP cache,
	// so two pods enriching concurrently can both pick the same "unused" IP
	// and silently conflict on the robot subnet; (2) MACVLAN.Create adds the
	// macvlan to the host netns before moving it into the pod netns, so two
	// pods creating a same-named macvlan collide on `ip link add` during
	// that transient window. host_macvlans is node-level, so every pod
	// reuses the same Name — the collision is guaranteed, not theoretical.
	// One DeviceServer per node, so a server-level mutex fully serializes
	// host-side work here; the critical section is a handful of `ip`
	// commands, so the cost is negligible.
	setupMu sync.Mutex
}

// NewDeviceServer builds a DeviceServer for the given macvlan configs and
// socket path. Configs are stored AS-IS: EnrichMACVLANConfig (auto-detect
// HostNIC, auto-pick an unused IP from a network-address placeholder) and
// ValidateMACVLANConfig run PER Setup call instead. Enrichment picks an IP
// from the host's live ARP/neighbour table, so it must run per container —
// enriching once at startup would hand the same auto-picked IP to every pod
// and cause conflicts on the robot subnet. On non-Linux EnrichMACVLANConfig
// is a no-op, so configs must be fully specified there.
//
// An empty cfgs slice yields a nil-meaning server (the caller checks
// HasMacvlans instead of constructing one).
func NewDeviceServer(cfgs []netmac.MACVLANConfig, socketPath string) *DeviceServer {
	// Copy so a later mutation of the caller's slice can't affect the stored
	// configs; they are re-enriched per Setup from these pristine originals.
	stored := append([]netmac.MACVLANConfig(nil), cfgs...)
	return &DeviceServer{
		cfgs:   stored,
		socket: socketPath,
	}
}

// HasMacvlans reports whether the server has any usable macvlan config to
// serve. When false the device plugin does not start the service.
func (s *DeviceServer) HasMacvlans() bool { return s != nil && len(s.cfgs) > 0 }

// SocketPath returns the Unix socket path the service listens on.
func (s *DeviceServer) SocketPath() string { return s.socket }

// Start creates the socket, registers the gRPC service, and serves. It
// blocks until Stop is called or the listener errors. Safe to call once.
func (s *DeviceServer) Start() error {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("macvlan server already started")
	}
	s.started = true
	s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.socket), 0755); err != nil {
		return fmt.Errorf("create macvlan socket dir: %w", err)
	}
	if err := os.Remove(s.socket); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale macvlan socket: %w", err)
	}

	raw, err := net.Listen("unix", s.socket)
	if err != nil {
		return fmt.Errorf("listen macvlan socket %s: %w", s.socket, err)
	}
	// Wrap the listener so each accepted connection carries its peer PID
	// through to the gRPC handler (see utils.PeerPIDListener /
	// utils.PIDFromContext).
	listener := &utils.PeerPIDListener{Listener: raw}

	srv := grpc.NewServer()
	devicepb.RegisterDeviceServiceServer(srv, s)

	// Publish srv under the lock so Stop (which may be called concurrently)
	// sees a fully-constructed server to GracefulStop.
	s.mu.Lock()
	s.srv = srv
	s.mu.Unlock()

	log.Printf("[device-plugin/macvlan] gRPC listening on %s (%d macvlans)",
		s.socket, len(s.cfgs))
	return srv.Serve(listener)
}

// Stop gracefully shuts down the macvlan gRPC server. Safe to call even if
// Start was never called or is still in progress (no-op in those cases).
func (s *DeviceServer) Stop() {
	s.mu.Lock()
	srv := s.srv
	started := s.started
	s.mu.Unlock()
	if !started || srv == nil {
		return
	}
	srv.GracefulStop()
}

// Setup implements device.v1.DeviceService. It reads the caller's PID
// from the connection peer credentials, skips the request when the caller
// is in the host network namespace, and otherwise creates each configured
// macvlan inside the caller's network namespace.
func (s *DeviceServer) Setup(ctx context.Context, _ *devicepb.SetupRequest) (*devicepb.SetupResponse, error) {
	pid, err := utils.PIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("identify caller: %w", err)
	}

	hostNet, err := utils.IsHostNetwork(pid)
	if err != nil {
		return nil, fmt.Errorf("check host network for pid %d: %w", pid, err)
	}
	if hostNet {
		log.Printf("[device-plugin/macvlan] caller pid=%d is in host netns — skipping", pid)
		return &devicepb.SetupResponse{
			HostNetwork: true,
			Skipped:     macvlanNames(s.cfgs),
		}, nil
	}

	// Serialize host-side macvlan work across concurrent Setup calls — see
	// setupMu. The host-network skip above is per-pod and touches no shared
	// host state, so it stays outside the lock to avoid blocking other pods
	// behind a hostNetwork caller.
	s.setupMu.Lock()
	defer s.setupMu.Unlock()

	created := make([]string, 0, len(s.cfgs))
	mvs := make([]*netmac.MACVLAN, 0, len(s.cfgs))
	for _, c := range s.cfgs {
		// Enrich a per-container copy (auto-detect HostNIC, auto-pick an
		// unused IP) and validate. Done per Setup call — not once at startup
		// — so each pod gets its own freshly-enriched config; sharing a
		// startup-time enrichment would give every pod the same IP. A config
		// that cannot be completed is logged and skipped.
		cfg, ok := enrichForSetup(c)
		if !ok {
			continue
		}
		mv := netmac.NewMACVLAN(cfg, netmac.WithPID(pid))
		if err := mv.Create(); err != nil {
			// Roll back the macvlans already created in this call so a
			// partial setup does not leak interfaces in the pod netns.
			log.Printf("[device-plugin/macvlan] create %s for pid=%d failed: %v — rolling back",
				cfg.Name, pid, err)
			for _, done := range mvs {
				done.Destroy()
			}
			return nil, fmt.Errorf("create macvlan %s for pid %d: %w", cfg.Name, pid, err)
		}
		mvs = append(mvs, mv)
		created = append(created, cfg.Name)
	}
	log.Printf("[device-plugin/macvlan] set up %d macvlan(s) for pid=%d: %v",
		len(created), pid, created)
	return &devicepb.SetupResponse{Created: created}, nil
}

// enrichForSetup returns a per-container enriched and validated copy of cfg,
// ready to create a macvlan from. EnrichMACVLANConfig (auto-detect HostNIC,
// auto-pick an unused IP when IP is a network-address placeholder) runs on
// the copy, then ValidateMACVLANConfig. A config that cannot be completed is
// logged and dropped (ok=false). The caller's cfg is not mutated, so the
// pristine stored config can be re-enriched for the next pod — with a
// possibly different auto-picked IP, since enrichment re-reads the host's
// live ARP/neighbour table each call.
func enrichForSetup(cfg netmac.MACVLANConfig) (netmac.MACVLANConfig, bool) {
	netmac.EnrichMACVLANConfig(&cfg, "[device-plugin/macvlan]")
	if err := netmac.ValidateMACVLANConfig(cfg); err != nil {
		log.Printf("[device-plugin/macvlan] WARNING: skipping macvlan %q: %v", cfg.Name, err)
		return netmac.MACVLANConfig{}, false
	}
	return cfg, true
}

// macvlanNames returns the Name field of each config, for the skipped list.
func macvlanNames(cfgs []netmac.MACVLANConfig) []string {
	out := make([]string, 0, len(cfgs))
	for _, c := range cfgs {
		out = append(out, c.Name)
	}
	return out
}
