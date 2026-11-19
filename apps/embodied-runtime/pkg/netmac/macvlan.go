package netmac

// MACVLAN manages a macvlan interface that connects the container to the
// robot's physical network. The interface is created on the host's physical
// NIC and moved into the target network namespace.
//
// By default the target namespace is the current one — i.e. the network
// namespace of the process running the controller (thanks to hostPID:true,
// the container's PID equals its host PID, so moving the link into the
// current PID's netns lands it in the container). Set PID to a non-zero
// value to operate on the network namespace of another process — e.g. a
// robot container whose PID is visible on the host. A PID of 0 means "the
// current network namespace" (the original behaviour).
//
// Layout:
//
//	Host
//	┌────────────────────────────────────────────┐
//	│ Physical NIC: enp0s1 (172.16.0.0/24)       │
//	│   └── macvlan-franka-0 ────────────────────┼──→ Target netns (PID)
//	│                                              │
//	│ Physical Robot: 172.16.0.2                  │
//	└──────────────────────────────────────────────┘
//
//	Target netns (current process, or PID-specified container)
//	┌────────────────────────────────────────────┐
//	│ eth0: 10.1.0.5 (pod network)              │
//	│ macvlan0: 172.16.0.100/24 (robot network) │
//	│                                            │
//	│ launch process (runs directly)             │
//	│   → pod network: ROS middleware            │
//	│   → macvlan0:   physical robot             │
//	└────────────────────────────────────────────┘
type MACVLAN struct {
	// Name is the macvlan interface name (e.g. "macvlan0").
	Name string

	// HostNIC is the host's physical NIC connected to the robot network.
	// e.g. "enp0s1", "eth0", "ens3"
	HostNIC string

	// IP is the IP address with CIDR prefix for the macvlan interface.
	// e.g. "172.16.0.100/24"
	IP string

	// Gateway is the optional default gateway for the robot subnet.
	Gateway string

	// PID optionally targets another process's network namespace.
	// When 0 (the default) the current network namespace is used —
	// i.e. the namespace of the controller process itself, which (with
	// hostPID:true) is the container's own netns. When non-zero, all
	// namespace-local operations (link/addr/route) are performed inside the
	// network namespace of the given PID. The host-side create/move steps
	// always enter PID 1.
	//
	// Set with the WithPID creation option rather than the yaml config,
	// since the PID is a runtime parameter (not a static, declarative
	// value).
	PID int

	created bool
}

// CreateOption is a creation-time option for NewMACVLAN. It carries the
// runtime parameters that are NOT part of the declarative MACVLANConfig
// (which is yaml-deserialised) — e.g. the PID whose network namespace the
// macvlan should be moved into.
type CreateOption func(*MACVLAN)

// WithPID targets the network namespace of the given PID instead of the
// current one. A PID of 0 selects the current network namespace (the
// default). The PID must be visible on the host — e.g. the controller pod
// runs with hostPID:true, so a robot container's host PID can be passed in
// to manage that container's netns.
func WithPID(pid int) CreateOption {
	return func(m *MACVLAN) { m.PID = pid }
}

// NewMACVLAN creates a MACVLAN from a MACVLANConfig plus optional creation
// options. The returned MACVLAN targets the current network namespace
// unless WithPID is supplied. Existing callers that pass only the config
// (NewMACVLAN(cfg)) are unchanged.
func NewMACVLAN(cfg MACVLANConfig, opts ...CreateOption) *MACVLAN {
	m := &MACVLAN{
		Name:    cfg.Name,
		HostNIC: cfg.HostNIC,
		IP:      cfg.IP,
		Gateway: cfg.Gateway,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	return m
}
