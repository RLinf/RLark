package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ClusterPhase summarizes the current lifecycle state of a data-plane cluster.
// The set is closed in the CRD schema: adding a value requires updating the
// enum marker, regenerating the CRD manifests, and updating the documentation.
// +kubebuilder:validation:Enum=Online;Offline;Downgraded;Cordoned;Draining
type ClusterPhase string

const (
	ClusterOnline     ClusterPhase = "Online"
	ClusterOffline    ClusterPhase = "Offline"
	ClusterDowngraded ClusterPhase = "Downgraded"
	ClusterCordoned   ClusterPhase = "Cordoned"
	ClusterDraining   ClusterPhase = "Draining"
)

// ClusterSpec declares the durable operational state of a data-plane cluster.
type ClusterSpec struct {
	// AgentNamespace is the management-plane namespace that contains the agent heartbeat Lease and Node CRs.
	AgentNamespace string `json:"agentNamespace"`
	Cordoned       bool   `json:"cordoned,omitempty"`
	Draining       bool   `json:"draining,omitempty"`
}

// ClusterStatus reports the controller's current observation of a data-plane cluster.
type ClusterStatus struct {
	Phase             ClusterPhase `json:"phase,omitempty"`
	LastHeartbeatTime *metav1.Time `json:"lastHeartbeatTime,omitempty"`
	TotalNodes        int32        `json:"totalNodes,omitempty"`
	OnlineNodes       int32        `json:"onlineNodes,omitempty"`
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
