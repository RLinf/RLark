package netmac

// MACVLANConfig specifies how to create a macvlan interface for the container.
type MACVLANConfig struct {
	// HostNIC is the host's physical NIC connected to the robot network.
	// e.g. "enp0s1", "eth0", "ens3"
	HostNIC string `yaml:"host_nic"`

	// Name is the macvlan interface name in the container (e.g. "macvlan0").
	Name string `yaml:"name"`

	// IP is the IP address with CIDR prefix (e.g. "172.16.0.100/24").
	IP string `yaml:"ip"`

	// Gateway is the optional default gateway for the robot subnet.
	Gateway string `yaml:"gateway,omitempty"`
}
