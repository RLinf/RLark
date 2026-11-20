package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// SiteNetworkMode says how the nodes of the network reach each other.
// +kubebuilder:validation:Enum=Tunnel
type SiteNetworkMode string

// SiteNetworkModeTunnel gives every node a WireGuard address and peers.
const SiteNetworkModeTunnel SiteNetworkMode = "Tunnel"

// SiteNetwork declares one network of nodes across sites: the address pool,
// the sites, and the nodes that are more than a leaf. An administrator owns
// spec; station-controller owns status. Nodes enroll themselves with a
// NetworkEnrollment and never appear here unless they need a role.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=sitenet
// +kubebuilder:printcolumn:name="Mode",type=string,JSONPath=`.spec.mode`
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.spec.pool`
// +kubebuilder:printcolumn:name="Members",type=integer,JSONPath=`.status.members`
type SiteNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SiteNetworkSpec   `json:"spec"`
	Status            SiteNetworkStatus `json:"status,omitempty"`
}

type SiteNetworkSpec struct {
	// +kubebuilder:default=Tunnel
	Mode SiteNetworkMode `json:"mode,omitempty"`
	// Pool is the IPv4 prefix tunnel addresses are allocated from.
	// +kubebuilder:validation:MinLength=9
	// +kubebuilder:validation:MaxLength=18
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="pool is immutable"
	Pool string `json:"pool"`
	// +kubebuilder:default=1380
	// +kubebuilder:validation:Minimum=576
	// +kubebuilder:validation:Maximum=9000
	MTU int32 `json:"mtu,omitempty"`
	// +kubebuilder:default=st-wg0
	// +kubebuilder:validation:MaxLength=15
	Interface string `json:"interface,omitempty"`
	// +kubebuilder:default=51820
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	ListenPort int32 `json:"listenPort,omitempty"`
	// +kubebuilder:default=25
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=3600
	KeepaliveSeconds int32 `json:"keepaliveSeconds,omitempty"`
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=64
	// +listType=map
	// +listMapKey=name
	Sites []SiteNetworkSite `json:"sites"`
	// Names are what every node resolves at the same address wherever it is:
	// what is reached through the tunnel, as a cluster's control plane and
	// the management plane are. A site's own names come after them. Every
	// node keeps its names in its /etc/hosts (station-gateway), so that
	// nothing on a node holds the address of a machine that may move.
	// +kubebuilder:validation:MaxItems=64
	// +listType=map
	// +listMapKey=name
	Names []SiteNetworkName `json:"names,omitempty"`
	// Nodes lists only the nodes that need more than the leaf default.
	// +kubebuilder:validation:MaxItems=256
	Nodes []SiteNetworkNode `json:"nodes,omitempty"`
}

type SiteNetworkSite struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Name string `json:"name"`
	// Clusters are the registered RLark Cluster IDs whose nodes are in this
	// site, except the nodes that name a site of their own.
	// +kubebuilder:validation:MaxItems=256
	// +listType=set
	Clusters []string `json:"clusters,omitempty"`
	// Mesh links the hubs of this site to each other over the tunnel.
	Mesh bool `json:"mesh,omitempty"`
	// Reaches lists the sites whose Site-scoped endpoints this site can dial.
	// +kubebuilder:validation:MaxItems=64
	// +listType=set
	Reaches []string `json:"reaches,omitempty"`
	// LeavesViaRelay makes the leaves of this site peer with their relay only
	// and reach everything else through it. Without it a leaf also dials
	// every other hub it can.
	LeavesViaRelay bool `json:"leavesViaRelay,omitempty"`
	// Relay is the hub that relays for the leaves of this site, as
	// "<clusterID>/<nodeName>": a hub with relay set that the site can dial.
	// Without it a leaf takes a relay of its own site, or else the first one
	// it can dial. Changing it moves each leaf when it next fetches its
	// configuration; two leaves reach each other again once both have.
	// +kubebuilder:validation:MaxLength=382
	// +kubebuilder:validation:Pattern=`^[^/]+/[^/]+$`
	Relay string `json:"relay,omitempty"`
	// LeafUplink is the uplink of every leaf of this site that declares none
	// of its own: machines of one build, each behind a device of its own
	// that has the same address on every one of them (a robot's computer
	// behind the robot's router), are declared once.
	LeafUplink *SiteNetworkLeafUplink `json:"leafUplink,omitempty"`
	// Discovery finds the machines on this site's network that are not nodes yet.
	Discovery *SiteNetworkDiscovery `json:"discovery,omitempty"`
	// DNS are the site's name servers. A node's kubelet gives them to its
	// pods; they are kept in /etc/rlark/station/resolv.conf on every node of
	// the site (station-gateway). Without them a node's pods use the name
	// servers the machine itself has.
	// +kubebuilder:validation:MaxItems=3
	// +kubebuilder:validation:items:Format=ipv4
	DNS []string `json:"dns,omitempty"`
	// Names are what the nodes of this site resolve at an address of the
	// site's own, as its registry is.
	// +kubebuilder:validation:MaxItems=64
	// +listType=map
	// +listMapKey=name
	Names []SiteNetworkName `json:"names,omitempty"`
}

// SiteNetworkName resolves a name to an address.
type SiteNetworkName struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`
	Name string `json:"name"`
	// +kubebuilder:validation:Format=ipv4
	Address string `json:"address"`
}

type SiteNetworkNode struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ClusterID string `json:"clusterID"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	NodeName string `json:"nodeName"`
	// Site is the site the node is in when it is not its cluster's: a
	// cluster's nodes may stand in several sites, as robots of one cluster
	// do whose control plane is somewhere else.
	// +kubebuilder:validation:MaxLength=63
	Site string `json:"site,omitempty"`
	// +kubebuilder:default=Leaf
	// +kubebuilder:validation:Enum=Leaf;Hub
	Role string `json:"role,omitempty"`
	// Relay makes a hub carry traffic between the leaves that dial it.
	Relay bool `json:"relay,omitempty"`
	// Address pins the tunnel address; empty lets the controller allocate one.
	// +kubebuilder:validation:MaxLength=15
	Address string `json:"address,omitempty"`
	// NodeAddress is the address other nodes use for this node when it is not
	// the tunnel address, for example a cloud VPC address.
	// +kubebuilder:validation:MaxLength=15
	NodeAddress string `json:"nodeAddress,omitempty"`
	// +kubebuilder:validation:MaxLength=15
	Interface string `json:"interface,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=65535
	ListenPort int32 `json:"listenPort,omitempty"`
	// PublicKey admits the node before it has enrolled. It is for a node that
	// cannot reach Station API until its tunnel is up; it needs Address too.
	// Once the node enrolls, the key it enrolled with is used.
	// +kubebuilder:validation:MaxLength=44
	PublicKey string `json:"publicKey,omitempty"`
	// +kubebuilder:validation:MaxItems=4
	Endpoints []SiteNetworkEndpoint `json:"endpoints,omitempty"`
	// +kubebuilder:validation:MaxItems=16
	Advertise []SiteNetworkAdvertisement `json:"advertise,omitempty"`
	Uplink    *SiteNetworkUplink         `json:"uplink,omitempty"`
}

type SiteNetworkEndpoint struct {
	// Address is ip:port. Empty means the address the node reported at
	// enrollment and its listen port, for a hub whose address is assigned by DHCP.
	// +kubebuilder:validation:MaxLength=21
	Address string `json:"address,omitempty"`
	// +kubebuilder:validation:Enum=Public;Site
	Scope string `json:"scope"`
}

type SiteNetworkAdvertisement struct {
	// +kubebuilder:validation:MinLength=9
	// +kubebuilder:validation:MaxLength=18
	CIDR string `json:"cidr"`
	// Sites limits the receivers; empty means every other site.
	// +kubebuilder:validation:MaxItems=64
	// +listType=set
	Sites []string `json:"sites,omitempty"`
}

// SiteNetworkUplink makes the node reach the endpoints it dials through a
// given gateway instead of its default route. The node dials from its own
// address in the gateway's network, on the interface that has it: an address
// of another network would be one nothing has given the node, and whatever
// is given that address later takes the node's answers.
type SiteNetworkUplink struct {
	// +kubebuilder:validation:Format=ipv4
	Gateway string `json:"gateway"`
}

// SiteNetworkLeafUplink is the uplink every leaf of a site has. What makes
// the device at Gateway let a new leaf out belongs to the leaf's kind of
// machine (NodeClass, uplink).
type SiteNetworkLeafUplink struct {
	SiteNetworkUplink `json:",inline"`
}

// SiteNetworkDiscovery has the hubs of a site look, on the site's own
// network, for machines that are not nodes yet: hosts whose SSH host key is
// one a NodeClass declares as its factory's. A hub reads the key and asks for
// nothing else. RLark logs in to what was found from the management plane,
// where the accounts are kept: each hub relays, on its tunnel address,
// connections to the SSH ports of Ranges and to nothing else. Two sites may
// use the same prefix.
type SiteNetworkDiscovery struct {
	Enabled bool `json:"enabled"`
	// From is the node the management plane's station-controller dials
	// from. The hubs relay for its tunnel address and for no other peer.
	From SiteNetworkDiscoverySource `json:"from"`
	// Ranges are the IPv4 prefixes of the site's network to look in, each
	// /22 or smaller.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=8
	// +listType=set
	Ranges []string `json:"ranges"`
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default=30
	IntervalSeconds int32 `json:"intervalSeconds,omitempty"`
}

type SiteNetworkDiscoverySource struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ClusterID string `json:"clusterID"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	NodeName string `json:"nodeName"`
}

type SiteNetworkStatus struct {
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	Members            int32 `json:"members,omitempty"`
	// Unreachable lists the first ordered pairs of members with no path.
	// +kubebuilder:validation:MaxItems=32
	Unreachable []string `json:"unreachable,omitempty"`
	// Declared holds, for every node declared with a public key that has not
	// enrolled yet, the document its tunnel starts on: what an administrator
	// puts on the machine (/etc/rlark/station/bootstrap.json) before it can
	// reach Station API. Once the node has enrolled its enrollment answers it.
	// +kubebuilder:validation:MaxItems=64
	// +listType=map
	// +listMapKey=clusterID
	// +listMapKey=nodeName
	Declared []SiteNetworkDeclaredConfig `json:"declared,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type SiteNetworkDeclaredConfig struct {
	ClusterID string `json:"clusterID"`
	NodeName  string `json:"nodeName"`
	// Config is the document station-gateway reconciles, for the declared key.
	// +kubebuilder:validation:MaxLength=262144
	Config     string `json:"config"`
	ConfigHash string `json:"configHash"`
}

// +kubebuilder:object:root=true
type SiteNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SiteNetwork `json:"items"`
}

// NetworkEnrollment is the record of one node: its identity, its latest
// report and its place in the SiteNetwork. Its name is the Node name in the
// namespace mapped to the cluster. station-api writes spec and
// status.observation; station-controller writes the rest of status, and
// spec.nodeUID, systemUUID and retired when the name passes to another Node.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=netenroll
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Site",type=string,JSONPath=`.status.site`
// +kubebuilder:printcolumn:name="Role",type=string,JSONPath=`.status.role`
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.address`
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.node.state`
// +kubebuilder:printcolumn:name="Reported",type=date,JSONPath=`.status.observation.reportedAt`
type NetworkEnrollment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NetworkEnrollmentSpec   `json:"spec"`
	Status            NetworkEnrollmentStatus `json:"status,omitempty"`
}

type NetworkEnrollmentSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ClusterID string `json:"clusterID"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	NodeUID string `json:"nodeUID"`
	// SystemUUID is evidence only: it tells a reinstalled machine from another one.
	// +kubebuilder:validation:MaxLength=128
	SystemUUID string `json:"systemUUID,omitempty"`
	// Retired marks the record of a node that was removed. It has no place
	// in the tunnel until a machine is admitted under the name again.
	Retired bool `json:"retired,omitempty"`
	// PublicKey is the node's WireGuard public key; the private key never
	// leaves the node. Empty for a node that is not in the tunnel.
	// +kubebuilder:validation:MaxLength=44
	PublicKey string `json:"publicKey,omitempty"`
	// Addresses are the node's own addresses as it reported them, the first
	// being the source it uses towards station-api.
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:validation:items:MaxLength=15
	Addresses []string `json:"addresses,omitempty"`
	// ObservedSource is the address station-api saw the request arrive from.
	// It differs from Addresses when the node is behind NAT.
	// +kubebuilder:validation:MaxLength=45
	ObservedSource string `json:"observedSource,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Enrolled;Rejected
type NetworkEnrollmentPhase string

const (
	NetworkEnrollmentPending  NetworkEnrollmentPhase = "Pending"
	NetworkEnrollmentEnrolled NetworkEnrollmentPhase = "Enrolled"
	NetworkEnrollmentRejected NetworkEnrollmentPhase = "Rejected"
)

type NetworkEnrollmentStatus struct {
	ObservedGeneration int64                  `json:"observedGeneration,omitempty"`
	Phase              NetworkEnrollmentPhase `json:"phase,omitempty"`
	// Reason explains Pending and Rejected in one line.
	// +kubebuilder:validation:MaxLength=512
	Reason string `json:"reason,omitempty"`
	Site   string `json:"site,omitempty"`
	Role   string `json:"role,omitempty"`
	// Address is the one address every other member reaches this node at.
	Address string `json:"address,omitempty"`
	// TunnelAddress is the node's address inside the tunnel, when it has one.
	TunnelAddress string `json:"tunnelAddress,omitempty"`
	// Config is the document station-gateway reconciles, for PublicKey.
	// +kubebuilder:validation:MaxLength=262144
	Config string `json:"config,omitempty"`
	// ConfigHash is the SHA-256 of Config in hex.
	ConfigHash string `json:"configHash,omitempty"`
	// PublicKey is the key Config was computed for.
	PublicKey string `json:"publicKey,omitempty"`
	// +kubebuilder:validation:MaxItems=16
	Problems []string `json:"problems,omitempty"`
	// Relay is the hub that relays for this Leaf, "<clusterID>/<nodeName>".
	Relay string `json:"relay,omitempty"`
	// Links says, for a Leaf, what each Hub it dials sees of it. A Leaf that
	// has dropped off cannot report anything, so this is the Hubs' account.
	// +kubebuilder:validation:MaxItems=64
	// +listType=map
	// +listMapKey=hub
	Links []NetworkEnrollmentLink `json:"links,omitempty"`
	// LinkSummary is Links in one line a page shows as it is. It is empty
	// while every link is Connected.
	// +kubebuilder:validation:MaxLength=1024
	LinkSummary string `json:"linkSummary,omitempty"`
	// Observation is station-api's alone: it patches this block and nothing else of status.
	Observation *NodeObservation `json:"observation,omitempty"`
	// Node is the node reconciler's alone.
	Node *NetworkEnrollmentNode `json:"node,omitempty"`
}

// +kubebuilder:validation:Enum=Connected;Unanswered;Silent;Unknown
type NetworkLinkState string

const (
	// NetworkLinkConnected: the Hub completed a handshake with the Leaf in
	// the last three minutes.
	NetworkLinkConnected NetworkLinkState = "Connected"
	// NetworkLinkUnanswered: the Leaf's requests arrive at the Hub and no
	// session forms: the Hub's answers do not get back to it.
	NetworkLinkUnanswered NetworkLinkState = "Unanswered"
	// NetworkLinkSilent: nothing arrives from the Leaf at the Hub.
	NetworkLinkSilent NetworkLinkState = "Silent"
	// NetworkLinkUnknown: the Hub has no current report of the Leaf, so
	// nothing is claimed about it.
	NetworkLinkUnknown NetworkLinkState = "Unknown"
)

type NetworkEnrollmentLink struct {
	// Hub is the member the Leaf dials, "<clusterID>/<nodeName>".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Hub   string           `json:"hub"`
	State NetworkLinkState `json:"state"`
	// Since is when State last changed.
	Since metav1.Time `json:"since"`
	// LatestHandshake is the last handshake the Hub completed with the Leaf,
	// kept while the link is Unanswered or Silent. A Connected link has a new
	// one every two minutes, in the Hub's status.observation.
	LatestHandshake *metav1.Time `json:"latestHandshake,omitempty"`
	// ReceivedBytes is what the Hub had received from the Leaf at ObservedAt,
	// the Hub's report the next one is compared with: a counter that grows
	// without a handshake is Unanswered. Both are kept only while the
	// handshake is stale, and here so that they outlive the controller.
	// +kubebuilder:validation:Minimum=0
	ReceivedBytes int64        `json:"receivedBytes,omitempty"`
	ObservedAt    *metav1.Time `json:"observedAt,omitempty"`
}

// +kubebuilder:object:root=true
type NetworkEnrollmentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NetworkEnrollment `json:"items"`
}
