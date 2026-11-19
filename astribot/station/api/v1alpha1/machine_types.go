package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Machine is a machine RLark knows of, a node or not: the list an
// administrator picks from to add nodes. A machine comes into it by being
// seen on its site's network (source Discovery) or by registering itself
// (source Registration). Its identity is its serial, which its class says
// where to read; the address it was seen at is an observation. Its name is its site and the hardware address of the
// device it was first seen through.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Class",type=string,JSONPath=`.spec.nodeClass`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Problem",type=string,JSONPath=`.status.problem`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.entry.address`
type Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MachineSpec   `json:"spec"`
	Status            MachineStatus `json:"status,omitempty"`
}

type MachineSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Site string `json:"site"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	NodeClass string `json:"nodeClass"`
	// +kubebuilder:validation:Enum=Discovery;Registration
	Source string `json:"source"`
	// Admit is the administrator's decision that the machine is to be a
	// node. Without it the machine is not joined.
	Admit *MachineAdmit `json:"admit,omitempty"`
}

type MachineAdmit struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ClusterID string `json:"clusterID"`
	// Labels replace the values of labels the machine's class declares, for
	// this machine: the body of a robot that is not the usual one.
	// +kubebuilder:validation:MaxProperties=8
	Labels map[string]string `json:"labels,omitempty"`
	// RequestedBy is the authenticated identity at the Gateway.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	RequestedBy string `json:"requestedBy"`
	// RequestedAt is when; asking again for a machine whose join stopped
	// is a new request, and starts a new join.
	RequestedAt metav1.Time `json:"requestedAt"`
}

// +kubebuilder:validation:Enum=Discovered;Admitted;Joined
type MachinePhase string

const (
	// Discovered: seen on its site's network, not a node.
	MachineDiscovered MachinePhase = "Discovered"
	// Admitted: its name is allocated and its join is under way.
	MachineAdmitted MachinePhase = "Admitted"
	// Joined: it is a node and its entry device has keys of its own.
	MachineJoined MachinePhase = "Joined"
)

// The problems a machine can have; none means it has none.
const (
	// The accounts of the machine's class were refused: somebody has changed
	// a password on this machine.
	MachineCredentialRejected = "CredentialRejected"
	// The entry device no longer shows the key it was recognised by.
	MachineHostKeyChanged = "HostKeyChanged"
	// The entry device answers; the computer behind it does not.
	MachineNodeUnreachable = "NodeUnreachable"
	// The entry device does not answer.
	MachineEntryUnreachable = "EntryUnreachable"
	// Another machine that is on the network now reports the same serial.
	MachineDuplicateSerial = "DuplicateSerial"
	// The machine is a node already, of a cluster or under a name RLark has
	// no admission for.
	MachineAlreadyNode = "AlreadyNode"
	// The machine's class does not say what is needed to admit it.
	MachineClassIncomplete = "ClassIncomplete"
	// A password of the machine's class has not been entered in RLark: nobody
	// logs in to the machine.
	MachineAccessUnconfigured = "AccessUnconfigured"
	// The machine's join stopped; Status.Message says where.
	MachineJoinBlocked = "JoinBlocked"
)

type MachineStatus struct {
	Phase MachinePhase `json:"phase,omitempty"`
	// +kubebuilder:validation:MaxLength=64
	Problem string `json:"problem,omitempty"`
	// +kubebuilder:validation:MaxLength=512
	Message string `json:"message,omitempty"`
	// Serial is the machine's serial, read on its entry device.
	// +kubebuilder:validation:MaxLength=128
	Serial string `json:"serial,omitempty"`
	// InspectedAt is when RLark last logged in to the machine to read what
	// it is. Station API clears it when the machine shows up changed.
	InspectedAt *metav1.Time `json:"inspectedAt,omitempty"`
	// Entry is the device the machine was last seen through.
	Entry MachineEntry `json:"entry,omitempty"`
	// NodeHostKeyFingerprint is the SSH host key of the computer behind the
	// entry device, as it was first read through the verified entry device.
	// +kubebuilder:validation:MaxLength=50
	NodeHostKeyFingerprint string `json:"nodeHostKeyFingerprint,omitempty"`
	// ClusterID and NodeName are the node the machine is or is becoming.
	// +kubebuilder:validation:MaxLength=128
	ClusterID string `json:"clusterID,omitempty"`
	// +kubebuilder:validation:MaxLength=253
	NodeName string `json:"nodeName,omitempty"`
	// Operation is the NodeOperation that joins the machine.
	// +kubebuilder:validation:MaxLength=253
	Operation string `json:"operation,omitempty"`
}

type MachineEntry struct {
	// +kubebuilder:validation:MaxLength=15
	Address string `json:"address,omitempty"`
	// +kubebuilder:validation:MaxLength=17
	MAC string `json:"mac,omitempty"`
	// +kubebuilder:validation:MaxLength=50
	HostKeyFingerprint string `json:"hostKeyFingerprint,omitempty"`
	// Via is the relay of the hub that saw the device, "tunnel-address:port":
	// where the management plane reaches Address from.
	// +kubebuilder:validation:MaxLength=21
	Via      string       `json:"via,omitempty"`
	LastSeen *metav1.Time `json:"lastSeen,omitempty"`
}

// +kubebuilder:object:root=true
type MachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Machine `json:"items"`
}
