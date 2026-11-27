package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeClass is everything RLark knows of one kind of machine: a robot model,
// a GPU server. How one is recognised before it is a node, how it is reached,
// where the accounts of the kind are kept, what its nodes are named and
// register with. It is declared once for the whole management plane, so that
// the hundredth machine of a kind comes in the way the first did.
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
type NodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NodeClassSpec `json:"spec"`
}

type NodeClassSpec struct {
	// Kind is the onboarding category. Empty preserves existing class behavior
	// and is inferred from the legacy robot labels.
	// +kubebuilder:validation:Enum=Robot;Server
	Kind NodeClassKind `json:"kind,omitempty"`
	// Model references RobotModel.metadata.name. Legacy classes may omit it
	// and infer a model from rlark.io/robot-model.
	// +kubebuilder:validation:MaxLength=63
	Model string `json:"model,omitempty"`
	// Labels and Taints (key=value:effect) are what a node of the class
	// registers with. A change of Labels reaches the nodes that have joined
	// as far as the cluster agent carries management labels to the cluster
	// (rlark.io/node-category, rlark.io/robot-* and the prefixes the agent
	// is started with); a change of Taints reaches a node when it is
	// installed anew.
	// +kubebuilder:validation:MaxProperties=32
	Labels map[string]string `json:"labels,omitempty"`
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:validation:items:MaxLength=317
	Taints []string `json:"taints,omitempty"`
	// Naming is how a machine of the class that is admitted from the list of
	// machines gets its node name. Without it such a machine cannot be.
	Naming *NodeClassNaming `json:"naming,omitempty"`
	// Discovery is how a machine of the class is recognised on a site's
	// network before anybody has logged in to it.
	Discovery *NodeClassDiscovery `json:"discovery,omitempty"`
	// Access is how RLark logs in to a machine of the class.
	Access *NodeClassAccess `json:"access,omitempty"`
	// Uplink is what lets a machine of the class out to its site.
	Uplink *NodeClassUplink `json:"uplink,omitempty"`
	// Addons are the Addons, by the names of their Addon objects in the
	// node's cluster, that run a pod on every node of the class (DaemonSets).
	// A machine's join is done when each has a ready pod on its node.
	// +kubebuilder:validation:MaxItems=16
	// +listType=set
	Addons []string `json:"addons,omitempty"`
}

// +kubebuilder:validation:Enum=Robot;Server
type NodeClassKind string

const (
	NodeClassKindRobot  NodeClassKind = "Robot"
	NodeClassKindServer NodeClassKind = "Server"
)

// NodeClassNaming gives every machine of the class the next number its
// cluster has not used. The number stays with the machine's serial: a
// machine installed anew is the node it was.
type NodeClassNaming struct {
	// NodeName is the node's name with one decimal verb for the number, as
	// "s1u-%02d-x86".
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9][-a-z0-9.]*%(0[1-9])?d([-a-z0-9.]*[a-z0-9])?$|^%(0[1-9])?d[-a-z0-9.]*[a-z0-9]$`
	NodeName string `json:"nodeName"`
	// Labels are labels whose values carry the same number, written the same way.
	// +kubebuilder:validation:MaxProperties=8
	Labels map[string]string `json:"labels,omitempty"`
}

type NodeClassDiscovery struct {
	// HostKeyFingerprints are the SHA256 fingerprints of the ED25519 SSH host
	// keys a machine of the class shows as it leaves the factory, one for every
	// image the factory has used. A host that shows one is a machine of the
	// class that has not joined; nothing else on the network is asked to
	// authenticate anybody.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=16
	// +kubebuilder:validation:items:Pattern=`^SHA256:[A-Za-z0-9+/]{43}$`
	// +listType=set
	HostKeyFingerprints []string `json:"hostKeyFingerprints"`
}

type NodeClassAccess struct {
	// Entry is the device of the machine that stands on the site's network
	// (a robot's Orin): the one a scan of the site sees.
	Entry NodeClassEntry `json:"entry"`
	// Node is the computer that becomes the node (a robot's x86), dialed
	// through Entry at the address it has on every machine of the class.
	Node NodeClassHost `json:"node"`
}

// SecretValue names one value of a secret kept in RLark (the secrets Jobs
// refer to by name): the passwords every machine of a class has are entered
// there, by an administrator, and read where a login needs them. Nothing of a
// value is written to any other object or log.
type SecretValue struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	Secret string `json:"secret"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Key string `json:"key"`
}

type NodeClassEntry struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	User string `json:"user"`
	// SerialFile is the file on the device that holds the machine's serial,
	// its identity (a Jetson module's /proc/device-tree/serial-number). It is
	// read on the device itself: a machine whose computer does not answer
	// has its identity all the same.
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:Pattern=`^/[A-Za-z0-9._/-]+$`
	SerialFile string `json:"serialFile"`
	// RotateHostKey gives the device SSH host keys of its own when its
	// machine joins. From then on the factory's key is shown only by
	// machines that have not joined.
	RotateHostKey bool `json:"rotateHostKey,omitempty"`
	// Password is the password of User on the device.
	Password SecretValue `json:"password"`
}

type NodeClassHost struct {
	// +kubebuilder:validation:Format=ipv4
	Address string `json:"address"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	User string `json:"user"`
	// Password is the password of User on the computer. Until it has been
	// entered, the machines of the class are listed and nobody logs in to them.
	Password SecretValue `json:"password"`
	// SudoPassword is what the account's sudo asks for where that is another
	// password than Password.
	SudoPassword *SecretValue `json:"sudoPassword,omitempty"`
}

type NodeClassUplink struct {
	// Setup is what the entry device of a new machine runs once so that the
	// machine's node can reach the site at all.
	Setup NodeClassUplinkSetup `json:"setup"`
}

// NodeClassUplinkSetup names a file of an Addon's package in the catalog.
// RLark runs it on the entry device, or Station API hands it out for an
// admission's one-time code, and knows nothing of what it does; whoever
// publishes the Addon keeps the device as the file leaves it.
type NodeClassUplinkSetup struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Addon string `json:"addon"`
	// Path is the file's path inside the Addon's package.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=255
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9_][A-Za-z0-9._-]*(/[A-Za-z0-9_][A-Za-z0-9._-]*)*$`
	Path string `json:"path"`
}

// +kubebuilder:object:root=true
type NodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeClass `json:"items"`
}
