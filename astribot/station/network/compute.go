// Package network derives every node's WireGuard configuration from a declared
// site topology and the set of enrolled nodes. It is pure: no Kubernetes, no
// clock and no I/O, so the same inputs always give the same document.
package network

import (
	"cmp"
	"fmt"
	"net/netip"
	"slices"
	"sort"
)

type Role string

const (
	// RoleLeaf nodes have no dialable endpoint; they dial out to hubs.
	RoleLeaf Role = "Leaf"
	// RoleHub nodes have at least one endpoint other nodes can dial.
	RoleHub Role = "Hub"
)

type EndpointScope string

const (
	// ScopePublic endpoints are dialable from every site.
	ScopePublic EndpointScope = "Public"
	// ScopeSite endpoints are dialable from the node's own site and from
	// sites that declare it in Reaches.
	ScopeSite EndpointScope = "Site"
)

type Topology struct {
	Pool             string
	MTU              int
	Interface        string
	ListenPort       int
	KeepaliveSeconds int
	Sites            []Site
	// Names are resolved by the members of every site; a site's own follow.
	Names []HostName
}

type Site struct {
	Name string
	// Mesh links the hubs of this site to each other over the tunnel. Leave it
	// false when they reach each other on their NodeAddress.
	Mesh bool
	// Reaches lists the sites whose Site-scoped endpoints this site can dial.
	Reaches []string
	// Relay is the ID of the hub that relays for the leaves of this site.
	// Without it a leaf takes a relay of its own site, or else the first
	// relay it can dial.
	Relay string
	Names []HostName
	// DNS are the site\'s name servers, for the pods of its members.
	DNS []string
	// LeafUplink is the uplink of the site's leaves that have none of their own.
	LeafUplink *Uplink
	// Discovery has the hubs of the site look for machines that are not nodes
	// yet; DiscoveryFrom is the ID of the node the management plane's
	// station-controller dials from, whose tunnel address becomes RelayFrom.
	Discovery     *Discovery
	DiscoveryFrom string
}

// DiscoveryRelayPort is the TCP port, on a hub's tunnel address, at which the
// hub connects the management plane to a host of its site's discovery ranges.
const DiscoveryRelayPort = 18022

// Discovery is what a hub looks for on its site's own network, and what it
// lets the management plane reach there. Two sites may use the same prefix;
// a connection is to an address of one hub's site, not to a prefix.
type Discovery struct {
	// Ranges are the IPv4 prefixes to look in.
	Ranges          []string `json:"ranges"`
	IntervalSeconds int      `json:"intervalSeconds"`
	// Ports are the SSH ports to look at; HostKeys the fingerprints of the
	// ED25519 host keys of machines that have not joined. A host that shows
	// one is reported; no host is asked to authenticate anybody.
	Ports    []int    `json:"ports"`
	HostKeys []string `json:"hostKeys"`
	// RelayPort is where the hub relays connections to Ranges and Ports,
	// RelayFrom the one tunnel address it relays for: that of the node the
	// management plane's station-controller dials from.
	RelayPort int    `json:"relayPort"`
	RelayFrom string `json:"relayFrom"`
}

type Endpoint struct {
	Address string
	Scope   EndpointScope
}

type Advertisement struct {
	CIDR string
	// Sites limits the receivers; empty means every other site.
	Sites []string
}

type Uplink struct {
	Gateway string
}

type Member struct {
	// ID is the stable key of the node, "<clusterID>/<nodeName>".
	ID        string
	Site      string
	Role      Role
	PublicKey string
	// Address is the allocated tunnel address, without a prefix length.
	Address string
	// NodeAddress is the one address other nodes use for this node when it is
	// not the tunnel address (a cloud VPC address). Other sites route it
	// through the tunnel; the node's own site reaches it directly.
	NodeAddress string
	// Relay makes a hub carry traffic between the leaves that dial it.
	Relay      bool
	Interface  string
	ListenPort int
	Endpoints  []Endpoint
	Advertise  []Advertisement
	Uplink     *Uplink
}

// DocumentLevel is the level of NodeConfig as it is written here: a gateway
// takes the document when its own documentLevel (astribot/station-gateway)
// is at least this. A gateway refuses a field it does not know, so the commit
// that gives NodeConfig a field raises this, the gateway's level and the gate
// of stage controller in apps/rlark/astribot/release/stages.yaml together, and the
// gateways are rolled out before the controller that writes the field.
const DocumentLevel = 2

// NodeConfig is the document station-gateway reconciles. Field names match
// astribot/station-gateway.
type NodeConfig struct {
	Interface    string        `json:"interface"`
	Address      string        `json:"address"`
	ListenPort   int           `json:"listenPort"`
	MTU          int           `json:"mtu"`
	Peers        []Peer        `json:"peers"`
	Routes       []string      `json:"routes"`
	UplinkRoutes []UplinkRoute `json:"uplinkRoutes,omitempty"`
	// Names are the lines station-gateway keeps in the node's /etc/hosts: with
	// none, it keeps none.
	Names []HostName `json:"names,omitempty"`
	// DNS are the name servers station-gateway keeps in the node's
	// /etc/rlark/station/resolv.conf, which the kubelet gives to pods; with
	// none, it keeps the machine's own there.
	DNS []string `json:"dns,omitempty"`
	// Discovery is set on the hubs of a site that declares it.
	Discovery *Discovery `json:"discovery,omitempty"`
}

type HostName struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type Peer struct {
	PublicKey        string   `json:"publicKey"`
	Endpoint         string   `json:"endpoint,omitempty"`
	AllowedIPs       []string `json:"allowedIPs"`
	KeepaliveSeconds int      `json:"keepaliveSeconds,omitempty"`
}

type UplinkRoute struct {
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
}

type Result struct {
	// Configs holds one configuration per member that takes part.
	Configs map[string]NodeConfig
	// Relays names, for every leaf that has one, the hub that relays for it.
	Relays map[string]string
	// Problems are per-member findings that did not stop the computation.
	Problems map[string][]string
	// Unreachable lists ordered pairs "from -> to" with no path at the
	// target's address. Ray needs this list to be empty for a Run's nodes.
	Unreachable []string
}

type node struct {
	Member
	site    Site
	address netip.Addr
	// routable is the address other members use for this node.
	routable netip.Addr
	// relay is the hub that carries this leaf's traffic to the nodes it has
	// no tunnel to.
	relay *node
}

// Compute returns the configuration of every member. An invalid topology is an
// error; an invalid member is skipped and reported in Problems.
func Compute(topology Topology, members []Member) (Result, error) {
	pool, err := netip.ParsePrefix(topology.Pool)
	if err != nil || !pool.Addr().Is4() {
		return Result{}, fmt.Errorf("pool must be an IPv4 prefix")
	}
	pool = pool.Masked()
	if topology.MTU < 576 || topology.MTU > 9000 || topology.ListenPort < 1 || topology.ListenPort > 65535 || topology.Interface == "" || topology.KeepaliveSeconds < 1 || topology.KeepaliveSeconds > 3600 {
		return Result{}, fmt.Errorf("interface, MTU, listen port and keepalive are required")
	}
	sites := make(map[string]Site, len(topology.Sites))
	for _, site := range topology.Sites {
		if _, exists := sites[site.Name]; exists || site.Name == "" {
			return Result{}, fmt.Errorf("site names must be unique and non-empty")
		}
		sites[site.Name] = site
	}
	for _, site := range topology.Sites {
		for _, reached := range site.Reaches {
			if _, ok := sites[reached]; !ok {
				return Result{}, fmt.Errorf("site %q reaches unknown site %q", site.Name, reached)
			}
		}
	}

	result := Result{Configs: map[string]NodeConfig{}, Relays: map[string]string{}, Problems: map[string][]string{}}
	nodes := make([]*node, 0, len(members))
	usedAddress, usedKey, usedID := map[netip.Addr]string{}, map[string]string{}, map[string]bool{}
	for _, member := range members {
		problem := func(format string, args ...any) {
			result.Problems[member.ID] = append(result.Problems[member.ID], fmt.Sprintf(format, args...))
		}
		if member.ID == "" || usedID[member.ID] {
			return Result{}, fmt.Errorf("member IDs must be unique and non-empty")
		}
		usedID[member.ID] = true
		site, ok := sites[member.Site]
		if !ok {
			problem("site %q is not declared", member.Site)
			continue
		}
		address, err := netip.ParseAddr(member.Address)
		if err != nil || !pool.Contains(address) {
			problem("address %q is not in pool %s", member.Address, pool)
			continue
		}
		if other, taken := usedAddress[address]; taken {
			problem("address %s is already used by %s", address, other)
			continue
		}
		if member.PublicKey == "" {
			problem("no public key")
			continue
		}
		if other, taken := usedKey[member.PublicKey]; taken {
			problem("public key is already used by %s", other)
			continue
		}
		if member.Role != RoleHub && member.Role != RoleLeaf {
			problem("role must be Hub or Leaf")
			continue
		}
		routable := address
		if member.NodeAddress != "" {
			routable, err = netip.ParseAddr(member.NodeAddress)
			if err != nil || !routable.Is4() || pool.Contains(routable) {
				problem("node address %q must be an IPv4 address outside the pool", member.NodeAddress)
				continue
			}
		}
		valid := true
		for _, endpoint := range member.Endpoints {
			if _, err := netip.ParseAddrPort(endpoint.Address); err != nil || (endpoint.Scope != ScopePublic && endpoint.Scope != ScopeSite) {
				problem("endpoint %q must be ip:port with scope Public or Site", endpoint.Address)
				valid = false
			}
		}
		if member.Role == RoleLeaf && (len(member.Endpoints) > 0 || member.Relay) {
			problem("a leaf has no endpoints and does not relay")
			valid = false
		}
		if member.Role == RoleLeaf && member.ListenPort != 0 {
			problem("a leaf has no listenPort: no one dials it, and it dials from a port of its own choosing")
			valid = false
		}
		if member.Role == RoleHub && len(member.Endpoints) == 0 {
			problem("a hub needs at least one endpoint")
			valid = false
		}
		if !valid {
			continue
		}
		usedAddress[address], usedKey[member.PublicKey] = member.ID, member.ID
		nodes = append(nodes, &node{Member: member, site: site, address: address, routable: routable})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].address.Less(nodes[j].address) })

	// A leaf has at most one relay: a relay in its own site first, then the
	// lowest tunnel address. Two peers cannot own the same prefix.
	for _, leaf := range nodes {
		if leaf.Role != RoleLeaf {
			continue
		}
		for _, hub := range nodes {
			if !hub.Relay || !dialable(leaf, hub) {
				continue
			}
			if leaf.site.Relay == "" && (leaf.relay == nil || hub.Site == leaf.Site && leaf.relay.Site != leaf.Site) || hub.ID == leaf.site.Relay {
				leaf.relay = hub
			}
		}
		if leaf.relay != nil {
			result.Relays[leaf.ID] = leaf.relay.ID
		}
		// A relay the site names and the leaf cannot use is said, not
		// replaced: which hub carries a site's leaves is the declaration's.
		if leaf.site.Relay != "" && leaf.relay == nil {
			result.Problems[leaf.ID] = append(result.Problems[leaf.ID], "the site's relay "+leaf.site.Relay+" is not a relaying hub this leaf can dial")
		}
	}

	for _, self := range nodes {
		// A leaf's packets leave through address translators, and the entry
		// one keeps for a source port can outlive a restart of the leaf and
		// stop carrying the answers. A leaf therefore has no port of its own
		// (0): its gateway takes a new one with every boot and whenever a hub
		// stops answering.
		port := cmp.Or(self.ListenPort, topology.ListenPort)
		if self.Role == RoleLeaf {
			port = 0
		}
		config := NodeConfig{
			Interface: cmp.Or(self.Interface, topology.Interface), Address: self.address.String() + "/32",
			ListenPort: port, MTU: topology.MTU,
			Peers: []Peer{}, Routes: []string{}, Names: append(topology.Names[:len(topology.Names):len(topology.Names)], self.site.Names...), DNS: self.site.DNS,
		}
		if self.Role == RoleHub && self.site.Discovery != nil {
			discovery := *self.site.Discovery
			for _, from := range nodes {
				if from.ID == self.site.DiscoveryFrom {
					discovery.RelayFrom = from.address.String()
				}
			}
			// Without the node it relays for, a hub would relay for anyone or no one: it does not look either.
			if discovery.RelayFrom == "" {
				result.Problems[self.ID] = append(result.Problems[self.ID], "the site's discovery is from "+self.site.DiscoveryFrom+", which is not a member of the network")
			} else {
				config.Discovery = &discovery
			}
		}
		type candidate struct {
			peer     Peer
			prefixes []netip.Prefix
		}
		var candidates []candidate
		var dialed []netip.Addr
		for _, other := range nodes {
			if other == self || !dialable(self, other) {
				continue
			}
			c := candidate{peer: Peer{PublicKey: other.PublicKey}}
			if endpoint, ok := dial(self, other); ok {
				c.peer.Endpoint = endpoint.String()
				c.peer.KeepaliveSeconds = topology.KeepaliveSeconds
				dialed = append(dialed, endpoint.Addr())
			}
			c.prefixes = append(c.prefixes, netip.PrefixFrom(other.address, 32))
			if other.Site != self.Site {
				for _, advertisement := range other.Advertise {
					if prefix, err := netip.ParsePrefix(advertisement.CIDR); err != nil || !prefix.Addr().Is4() || prefix.Bits() == 0 {
						problem := fmt.Sprintf("advertised prefix %q is not a non-default IPv4 prefix", advertisement.CIDR)
						if !slices.Contains(result.Problems[other.ID], problem) {
							result.Problems[other.ID] = append(result.Problems[other.ID], problem)
						}
					}
				}
				c.prefixes = append(c.prefixes, beyond(other, self.Site, pool)...)
			}
			if self.relay == other {
				c.prefixes = append(c.prefixes, pool)
				// What lies behind the relay's other tunnels and is not a
				// tunnel address: the leaf has no tunnel of its own to it.
				for _, far := range nodes {
					if far == self || far == other || far.Site == self.Site || dialable(self, far) || !dialable(other, far) {
						continue
					}
					c.prefixes = append(c.prefixes, beyond(far, self.Site, pool)...)
				}
			}
			if other.Relay && self.Role == RoleHub {
				// A hub that a relayed leaf cannot dial still has to answer it.
				for _, leaf := range nodes {
					if leaf.relay == other && !dialable(self, leaf) {
						c.prefixes = append(c.prefixes, netip.PrefixFrom(leaf.address, 32))
					}
				}
			}
			candidates = append(candidates, c)
		}
		owned := map[netip.Prefix]bool{}
		for _, c := range candidates {
			for _, prefix := range c.prefixes {
				loops := false
				for _, endpoint := range dialed {
					if prefix.Contains(endpoint) {
						loops = true
					}
				}
				if loops {
					result.Problems[self.ID] = append(result.Problems[self.ID], fmt.Sprintf("prefix %s would route a dialled endpoint into the tunnel; dropped", prefix))
					continue
				}
				if owned[prefix] {
					result.Problems[self.ID] = append(result.Problems[self.ID], fmt.Sprintf("prefix %s is offered by more than one peer; kept the first", prefix))
					continue
				}
				owned[prefix] = true
				c.peer.AllowedIPs = append(c.peer.AllowedIPs, prefix.String())
				config.Routes = append(config.Routes, prefix.String())
			}
			config.Peers = append(config.Peers, c.peer)
		}
		uplink := self.Uplink
		if uplink == nil && self.Role == RoleLeaf {
			uplink = self.site.LeafUplink
		}
		if uplink != nil {
			seen := map[netip.Addr]bool{}
			for _, endpoint := range dialed {
				if seen[endpoint] {
					continue
				}
				seen[endpoint] = true
				config.UplinkRoutes = append(config.UplinkRoutes, UplinkRoute{
					Destination: endpoint.String() + "/32", Gateway: uplink.Gateway,
				})
			}
		}
		result.Configs[self.ID] = config
	}

	for _, from := range nodes {
		for _, to := range nodes {
			if from != to && !reaches(result.Configs, nodes, from, to) {
				result.Unreachable = append(result.Unreachable, from.ID+" -> "+to.ID)
			}
		}
	}
	return result, nil
}

// dial returns the endpoint from uses to open the tunnel to to.
func dial(from, to *node) (netip.AddrPort, bool) {
	local := from.Site == to.Site || slices.Contains(from.site.Reaches, to.Site)
	var public netip.AddrPort
	for _, endpoint := range to.Endpoints {
		parsed := netip.MustParseAddrPort(endpoint.Address)
		if endpoint.Scope == ScopeSite && local {
			return parsed, true
		}
		if endpoint.Scope == ScopePublic && !public.IsValid() {
			public = parsed
		}
	}
	return public, public.IsValid()
}

// beyond lists what a member of site reaches through far besides far's tunnel
// address: its node address and the prefixes it advertises to that site.
func beyond(far *node, site string, pool netip.Prefix) []netip.Prefix {
	var prefixes []netip.Prefix
	if far.routable != far.address {
		prefixes = append(prefixes, netip.PrefixFrom(far.routable, 32))
	}
	for _, advertisement := range far.Advertise {
		prefix, err := netip.ParsePrefix(advertisement.CIDR)
		if err == nil && prefix.Addr().Is4() && prefix.Bits() != 0 && (len(advertisement.Sites) == 0 || slices.Contains(advertisement.Sites, site)) {
			prefixes = append(prefixes, prefix.Masked())
		}
	}
	return prefixes
}

// dialable reports whether two members share a tunnel: at least one is
// a hub, one side can dial the other, and hubs of one site link only when it meshes.
func dialable(a, b *node) bool {
	if a.Role == RoleLeaf && b.Role == RoleLeaf {
		return false
	}
	if a.Site == b.Site && a.Role == RoleHub && b.Role == RoleHub && !a.site.Mesh {
		return false
	}
	_, forward := dial(a, b)
	_, backward := dial(b, a)
	return forward || backward
}

// reaches follows the computed configurations from one member to another's
// address: directly, through one relay, or on the site's own network.
func reaches(configs map[string]NodeConfig, nodes []*node, from, to *node) bool {
	if from.Site == to.Site && to.routable != to.address && from.routable != from.address {
		return true
	}
	next := nextHop(configs[from.ID], to.routable)
	if next == "" {
		return false
	}
	if next == to.PublicKey {
		return true
	}
	for _, relay := range nodes {
		if relay.PublicKey == next {
			return nextHop(configs[relay.ID], to.routable) == to.PublicKey
		}
	}
	return false
}

// nextHop is WireGuard's cryptokey routing: the peer with the longest prefix.
func nextHop(config NodeConfig, target netip.Addr) string {
	best, key := -1, ""
	for _, peer := range config.Peers {
		for _, allowed := range peer.AllowedIPs {
			prefix := netip.MustParsePrefix(allowed)
			if prefix.Contains(target) && prefix.Bits() > best {
				best, key = prefix.Bits(), peer.PublicKey
			}
		}
	}
	return key
}

// Allocate returns the lowest free host address of the pool.
func Allocate(pool string, taken []string) (string, error) {
	prefix, err := netip.ParsePrefix(pool)
	if err != nil || !prefix.Addr().Is4() || prefix.Bits() > 30 {
		return "", fmt.Errorf("pool must be an IPv4 prefix of at most /30")
	}
	prefix = prefix.Masked()
	used := make(map[netip.Addr]bool, len(taken))
	for _, value := range taken {
		address, err := netip.ParseAddr(value)
		if err != nil {
			return "", fmt.Errorf("allocated address %q is invalid", value)
		}
		used[address] = true
	}
	for address := prefix.Addr().Next(); prefix.Contains(address.Next()); address = address.Next() {
		if !used[address] {
			return address.String(), nil
		}
	}
	return "", fmt.Errorf("pool %s is exhausted", prefix)
}
