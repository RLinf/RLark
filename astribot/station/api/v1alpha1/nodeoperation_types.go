package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// NodeOperation records a single node lifecycle operation: the join of a
// machine an administrator admitted, or the removal of a node. It holds no
// credential: a join logs in with the accounts of the machine's NodeClass, a
// removal logs in nowhere.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:printcolumn:name="Action",type=string,JSONPath=`.spec.action`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type NodeOperation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NodeOperationSpec   `json:"spec"`
	Status            NodeOperationStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="operation spec is immutable"
// +kubebuilder:validation:XValidation:rule="self.action != 'Join' || (has(self.ssh) && has(self.nodeClass) && has(self.admission))",message="a join redeems an admission and logs in with the accounts of a node class"
// +kubebuilder:validation:XValidation:rule="self.action != 'Remove' || (!has(self.ssh) && !has(self.nodeClass) && !has(self.admission))",message="a removal logs in nowhere: the node is reset from inside its cluster"
type NodeOperationSpec struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	RequestID string `json:"requestID"`
	// +kubebuilder:validation:Enum=Join;Remove
	Action string `json:"action"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	ClusterID string `json:"clusterID"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	NodeName string `json:"nodeName"`
	// NodeUID is the Node instance a removal is of. A removal without it is
	// of whatever is under the name: the Node its cluster has when the
	// removal starts (status.nodeUID), or, when its cluster has none, what
	// RLark keeps of the name (a join that never finished, a node reset by
	// hand).
	// +kubebuilder:validation:MaxLength=128
	NodeUID string             `json:"nodeUID,omitempty"`
	SSH     *NodeSSHConnection `json:"ssh,omitempty"`
	// Admission names the NodeAdmission, in the operation's namespace, this
	// Join redeems: the machine is installed by the installer an operator's
	// typed command runs, and comes out the same node.
	// +kubebuilder:validation:MaxLength=253
	Admission string `json:"admission,omitempty"`
	// NodeClass names the NodeClass whose standing accounts a Join logs in with.
	// +kubebuilder:validation:MaxLength=63
	NodeClass string `json:"nodeClass,omitempty"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=256
	RequestedBy string `json:"requestedBy"`
}

type NodeSSHConnection struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Address string `json:"address"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	User string `json:"user"`
	// HostKeyFingerprint is the administrator-confirmed SHA256 fingerprint.
	// +kubebuilder:validation:Pattern=`^SHA256:[A-Za-z0-9+/]{43}$`
	HostKeyFingerprint string `json:"hostKeyFingerprint"`
	// Jump is dialed first, and Address through it: the device a machine
	// stands behind. Nothing is sent to either to authenticate with
	// before it has shown its key.
	Jump *NodeSSHHop `json:"jump,omitempty"`
}

type NodeSSHHop struct {
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	Address string `json:"address"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=128
	User string `json:"user"`
	// +kubebuilder:validation:Pattern=`^SHA256:[A-Za-z0-9+/]{43}$`
	HostKeyFingerprint string `json:"hostKeyFingerprint"`
	// Via is the relay of the hub of the site Address is in, as
	// "tunnel-address:port": Address is an address of that site's own
	// network, which the management plane reaches through its hub.
	// +kubebuilder:validation:MaxLength=21
	Via string `json:"via,omitempty"`
}

type NodeOperationStatus struct {
	// +kubebuilder:validation:Enum=Pending;Running;Blocked;Succeeded;Failed
	Phase string `json:"phase,omitempty"`
	// Stage names the last verified step, not an instruction to replay an effect.
	// +kubebuilder:validation:MaxLength=128
	Stage string `json:"stage,omitempty"`
	// NodeUID identifies the actual member Node instance established by a join.
	// +kubebuilder:validation:MaxLength=128
	NodeUID            string       `json:"nodeUID,omitempty"`
	ObservedGeneration int64        `json:"observedGeneration,omitempty"`
	CompletedAt        *metav1.Time `json:"completedAt,omitempty"`
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type NodeOperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeOperation `json:"items"`
}
