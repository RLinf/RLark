package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ClusterPhase summarizes the current lifecycle state of a data-plane cluster.
// The set is closed in the CRD schema: adding a value requires updating the
// enum marker, regenerating the CRD manifests, and updating the documentation.
// +kubebuilder:validation:Enum=Online;Offline;Downgraded;Cordoned;Draining;Delisted
type ClusterPhase string

const (
	ClusterOnline     ClusterPhase = "Online"
	ClusterOffline    ClusterPhase = "Offline"
	ClusterDowngraded ClusterPhase = "Downgraded"
	ClusterCordoned   ClusterPhase = "Cordoned"
	ClusterDraining   ClusterPhase = "Draining"
	ClusterDelisted   ClusterPhase = "Delisted"
)

// ClusterSpec declares the durable operational state of a data-plane cluster.
type ClusterSpec struct {
	// AgentNamespace is the management-plane namespace that contains the agent heartbeat Lease and Node CRs.
	// +kubebuilder:validation:MinLength=1
	AgentNamespace        string `json:"agentNamespace"`
	Cordoned              bool   `json:"cordoned,omitempty"`
	Draining              bool   `json:"draining,omitempty"`
	Delisted              bool   `json:"delisted,omitempty"`
	DrainPreviousCordoned bool   `json:"drainPreviousCordoned,omitempty"`
}

// ClusterStatus reports the controller's current observation of a data-plane cluster.
type ClusterStatus struct {
	Phase ClusterPhase `json:"phase,omitempty"`
	// ObservedGeneration is the metadata.generation observed by the Cluster
	// controller the last time it finished a reconciliation. ClusterSpec's
	// cordoned / draining are Spec-driven switches, so consumers must compare
	// this field with metadata.generation to tell whether phase and conditions
	// already reflect the latest Spec. It is a controller contract, not an API
	// server guarantee.
	ObservedGeneration int64        `json:"observedGeneration,omitempty"`
	LastHeartbeatTime  *metav1.Time `json:"lastHeartbeatTime,omitempty"`
	// LastStatusChangeTime is the time Phase last changed. It carries the same
	// semantics as the identically named field on NodeStatus and distinguishes
	// "has stayed in this phase for a long time" from "just flipped".
	LastStatusChangeTime *metav1.Time `json:"lastStatusChangeTime,omitempty"`
	TotalNodes           int32        `json:"totalNodes,omitempty"`
	OnlineNodes          int32        `json:"onlineNodes,omitempty"`
	// ReadyNodes is the number of connected Nodes with Ready=True that are not
	// cordoned by spec.unschedulable. A ready Node under resource pressure is
	// still counted here even though it is excluded from new placement.
	ReadyNodes int32 `json:"readyNodes,omitempty"`
	// DegradedNodes is the number of connected Nodes that are not ready or report Degraded=True.
	DegradedNodes int32 `json:"degradedNodes,omitempty"`
	// DisconnectedNodes is the number of Nodes whose AgentConnected condition is not True.
	DisconnectedNodes int32 `json:"disconnectedNodes,omitempty"`
	// UnschedulableNodes is the number of Nodes with spec.unschedulable=true.
	UnschedulableNodes int32 `json:"unschedulableNodes,omitempty"`
	// DominantFailureReason is the most frequent reason among unavailable candidate Nodes.
	DominantFailureReason string `json:"dominantFailureReason,omitempty"`
	// Conditions records the structured conclusions of the Cluster controller,
	// which is their only writer. Ready and Degraded are independent dimensions:
	// a cluster that lost part of its capacity reports Ready=True together with
	// Degraded=True. They are observation-driven and therefore carry no
	// per-condition ObservedGeneration; use the status-level ObservedGeneration
	// above to tell whether they already reflect the latest Spec.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusters,scope=Cluster,shortName=rlcluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Online",type=integer,JSONPath=`.status.onlineNodes`
// +kubebuilder:printcolumn:name="Total",type=integer,JSONPath=`.status.totalNodes`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterSpec   `json:"spec,omitempty"`
	Status            ClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}
