package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeAdmission lets one machine that is not yet a node join: an operator
// writes it and hands the machine the one-time code whose hash it holds. The
// machine redeems the code at Station API with its WireGuard public key and
// receives its tunnel configuration and what it needs to join the cluster.
// The admission stays afterwards as the node's network declaration. Its name
// is the Node name in the namespace mapped to the cluster.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=admit
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Address",type=string,JSONPath=`.status.address`
// +kubebuilder:printcolumn:name="Expires",type=string,JSONPath=`.spec.expiresAt`
type NodeAdmission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NodeAdmissionSpec   `json:"spec"`
	Status            NodeAdmissionStatus `json:"status,omitempty"`
}

type NodeAdmissionSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ClusterID string `json:"clusterID"`
	// Site is the site the machine stands in when it is not its cluster's.
	// A node the SiteNetwork declares with a site is in that one.
	// +kubebuilder:validation:MaxLength=63
	Site string `json:"site,omitempty"`
	// CodeHash is the SHA-256 of the one-time code in hex. The code itself is
	// shown once to the operator and stored nowhere.
	// +kubebuilder:validation:MinLength=64
	// +kubebuilder:validation:MaxLength=64
	CodeHash string `json:"codeHash"`
	// ExpiresAt ends the time in which the code can be redeemed.
	ExpiresAt metav1.Time `json:"expiresAt"`
	// Labels and Taints (key=value:effect) are what the node registers with.
	// +kubebuilder:validation:MaxProperties=32
	Labels map[string]string `json:"labels,omitempty"`
	// +kubebuilder:validation:MaxItems=8
	// +kubebuilder:validation:items:MaxLength=317
	Taints []string `json:"taints,omitempty"`
	// Address pins the tunnel address; empty lets the controller allocate one.
	// +kubebuilder:validation:MaxLength=15
	Address string             `json:"address,omitempty"`
	Uplink  *SiteNetworkUplink `json:"uplink,omitempty"`
	// PublicKey is written by Station API when the code is redeemed; the code
	// then stays valid for this key only.
	// +kubebuilder:validation:MaxLength=44
	PublicKey string `json:"publicKey,omitempty"`
	// RedeemedAt is when Station API accepted the code.
	RedeemedAt *metav1.Time `json:"redeemedAt,omitempty"`
	// IssuedBy and IssuedAt are who gave out the current code and when: the
	// authenticated identity at the Gateway, never a value the request names.
	// +kubebuilder:validation:MaxLength=256
	IssuedBy string       `json:"issuedBy,omitempty"`
	IssuedAt *metav1.Time `json:"issuedAt,omitempty"`
	// NodeClass names the NodeClass of the machine. The node follows what
	// the class declares, and the class says what lets the machine out to
	// its site (uplink).
	// +kubebuilder:validation:MaxLength=63
	NodeClass string `json:"nodeClass,omitempty"`
	// Serial is the serial of the machine the name was given to: the same
	// machine, admitted again, is the same node.
	// +kubebuilder:validation:MaxLength=128
	Serial string `json:"serial,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Redeemed;Joined;Expired
type NodeAdmissionPhase string

const (
	// Pending: the code has not been redeemed.
	NodeAdmissionPending NodeAdmissionPhase = "Pending"
	// Redeemed: the machine has its tunnel configuration and is joining.
	NodeAdmissionRedeemed NodeAdmissionPhase = "Redeemed"
	// Joined: the node has enrolled under its own identity.
	NodeAdmissionJoined NodeAdmissionPhase = "Joined"
	// Expired: the code was not redeemed in time.
	NodeAdmissionExpired NodeAdmissionPhase = "Expired"
)

type NodeAdmissionStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Phase              NodeAdmissionPhase `json:"phase,omitempty"`
	// +kubebuilder:validation:MaxLength=512
	Reason string `json:"reason,omitempty"`
	// Address is the tunnel address; once set it stays with this admission.
	Address string `json:"address,omitempty"`
	// Config is the document station-gateway reconciles until the node has
	// enrolled under its own identity, for PublicKey.
	// +kubebuilder:validation:MaxLength=262144
	Config     string `json:"config,omitempty"`
	ConfigHash string `json:"configHash,omitempty"`
	PublicKey  string `json:"publicKey,omitempty"`
	// RedeemedAt is the redemption (spec.redeemedAt) this status answers.
	RedeemedAt *metav1.Time `json:"redeemedAt,omitempty"`
	// RejoinsNodeUID is set while a machine that already is this node, with
	// the same key, joins again: its cluster got a new control plane. It is
	// the Node UID the machine had when it redeemed the code; the admission
	// is Joined once the node has enrolled under another.
	// +kubebuilder:validation:MaxLength=128
	RejoinsNodeUID string `json:"rejoinsNodeUID,omitempty"`
}

// +kubebuilder:object:root=true
type NodeAdmissionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeAdmission `json:"items"`
}
