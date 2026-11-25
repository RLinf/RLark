package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:validation:Enum=Bound;Offline;Conflict;Retired
type NodeState string

const (
	NodeStateBound    NodeState = "Bound"
	NodeStateOffline  NodeState = "Offline"
	NodeStateConflict NodeState = "Conflict"
	NodeStateRetired  NodeState = "Retired"
)

// NetworkEnrollmentNode is what station-controller makes of the node's
// reports (status.node).
type NetworkEnrollmentNode struct {
	State NodeState `json:"state,omitempty"`
	// BootTime is the node's latest reported boot time.
	BootTime     *metav1.Time `json:"bootTime,omitempty"`
	LastSeenTime *metav1.Time `json:"lastSeenTime,omitempty"`
	// Reboots counts the boots seen since the record was made or taken over by another Node.
	// +kubebuilder:validation:Minimum=0
	Reboots int64 `json:"reboots,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// NodeObservation is the latest report of a node's station-agent, as
// station-api verified and stored it (status.observation).
type NodeObservation struct {
	// ReportedAt is server receipt time, never a client-provided timestamp.
	ReportedAt metav1.Time `json:"reportedAt"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	NodeUID string `json:"nodeUID"`
	// SystemUUID is evidence only and must not be used to authorize a binding.
	SystemUUID string `json:"systemUUID,omitempty"`
	// Identity is derived by Station API from the Node it authenticated, not
	// accepted as an arbitrary client assertion.
	Identity  *NodeIdentityObservation `json:"identity,omitempty"`
	Hardware  *HardwareObservation     `json:"hardware,omitempty"`
	BootTime  *metav1.Time             `json:"bootTime,omitempty"`
	Wireguard *WireguardObservation    `json:"wireguard,omitempty"`
	// +kubebuilder:validation:MaxItems=256
	Probes []StationProbe `json:"probes,omitempty"`
	// Gateway is the station-gateway of the node, as that program states it
	// with every good pass. A node without it runs a gateway that is not
	// reconciling.
	Gateway *GatewayProgram `json:"gateway,omitempty"`
	// Agent is the station-agent that sent this report.
	Agent AgentProgram `json:"agent"`
	// Pressures are the DiskPressure, MemoryPressure and PIDPressure
	// conditions that are True on the member Node, read by station-api when
	// it verified the report. The list is written with every report, so an
	// empty one says the Node is under none and a missing one that the report
	// was stored by a station-api from before the field.
	// +kubebuilder:validation:MaxItems=3
	// +kubebuilder:validation:items:Enum=DiskPressure;MemoryPressure;PIDPressure
	// +listType=set
	Pressures []string `json:"pressures,omitempty"`
}

type NodeIdentityObservation struct {
	// +kubebuilder:validation:Enum=Robot;Server
	Category string `json:"category,omitempty"`
	// +kubebuilder:validation:MaxLength=63
	Model string `json:"model,omitempty"`
	// +kubebuilder:validation:Enum=node-labels;node-info
	Source string `json:"source,omitempty"`
	// ReportedAt is Station API receipt time.
	ReportedAt metav1.Time `json:"reportedAt"`
}

type HardwareObservation struct {
	// +kubebuilder:validation:MaxLength=64
	Architecture string `json:"architecture,omitempty"`
	// +kubebuilder:validation:MaxLength=256
	OSImage string `json:"osImage,omitempty"`
	// +kubebuilder:validation:MaxLength=256
	KernelVersion string `json:"kernelVersion,omitempty"`
	// +kubebuilder:validation:MaxLength=256
	SystemVendor string `json:"systemVendor,omitempty"`
	// +kubebuilder:validation:MaxLength=256
	ProductName string `json:"productName,omitempty"`
}

type GatewayProgram struct {
	// DocumentLevel is the newest configuration document the program
	// understands; it understands every one before it.
	// +kubebuilder:validation:Minimum=1
	DocumentLevel int32 `json:"documentLevel"`
	// Digest is the sha256 of the program's file.
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{64}$`
	Digest string `json:"digest"`
}

type AgentProgram struct {
	// Digest is the sha256 of the program's file.
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{64}$`
	Digest string `json:"digest"`
}

type WireguardObservation struct {
	// Interface is the observed interface name, not a desired configuration.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=15
	Interface string `json:"interface"`
	// PublicKey is the public key of the observed interface; private keys are never reported.
	// +kubebuilder:validation:MaxLength=64
	PublicKey string `json:"publicKey,omitempty"`
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:MaxLength=64
	// +listType=set
	Addresses []string `json:"addresses,omitempty"`
	// Peers are the node's WireGuard peers that are enrolled members, as the
	// node's kernel counts them. A Hub's entry for a Leaf is the only account
	// of a Leaf that has dropped off.
	// +kubebuilder:validation:MaxItems=1024
	// +listType=map
	// +listMapKey=name
	Peers []WireguardPeerObservation `json:"peers,omitempty"`
}

type WireguardPeerObservation struct {
	// Name is the peer's member id, "<clusterID>/<nodeName>". station-api
	// assigns it from the public key the node reported: it is that of the
	// Enrolled NetworkEnrollment with this key.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	Name            string       `json:"name"`
	LatestHandshake *metav1.Time `json:"latestHandshake,omitempty"`
	// +kubebuilder:validation:Minimum=0
	ReceivedBytes int64 `json:"receivedBytes,omitempty"`
	// +kubebuilder:validation:Minimum=0
	SentBytes int64 `json:"sentBytes,omitempty"`
}

type StationProbe struct {
	// Target is a configured probe label or host:port, bounded by API policy.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	Target string `json:"target"`
	// Via identifies the observed route label.
	// +kubebuilder:validation:MaxLength=256
	Via string `json:"via,omitempty"`
	OK  bool   `json:"ok"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=600000
	RTTMillis *int64 `json:"rttMs,omitempty"`
}
