package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &NodeJoin{}, &NodeJoinList{})
		return nil
	})
}

// NodeJoinRoute is how a machine reaches Station API for its code.
// +kubebuilder:validation:Enum=direct;tunnel;tunnel-up
type NodeJoinRoute string

const (
	// NodeJoinRouteDirect is a machine that reaches Station API at its bootstrap address.
	NodeJoinRouteDirect NodeJoinRoute = "direct"
	// NodeJoinRouteTunnel is a machine that reaches it only through a tunnel of its own, which the join makes first.
	NodeJoinRouteTunnel NodeJoinRoute = "tunnel"
	// NodeJoinRouteTunnelUp is a machine declared with its key whose tunnel already answers.
	NodeJoinRouteTunnelUp NodeJoinRoute = "tunnel-up"
)

// NodeJoinSpec is the machine the Gateway joins to a cluster over SSH. The
// machine's password is never part of it: it is given with the request that
// starts or retries the join, and lives only while that runs.
type NodeJoinSpec struct {
	// +kubebuilder:validation:MinLength=1
	ClusterID string `json:"clusterID"`
	// NodeName is the node's name: the one asked for, or the machine's host name.
	// +kubebuilder:validation:MinLength=1
	NodeName string          `json:"nodeName"`
	Machine  NodeJoinMachine `json:"machine"`
	// RequestedBy is the user who asked for the join.
	RequestedBy string `json:"requestedBy,omitempty"`
}

// NodeJoinMachine is the SSH address and account of the machine.
type NodeJoinMachine struct {
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`
	// +kubebuilder:default=22
	Port int32 `json:"port,omitempty"`
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`
	// TunnelEndpoint is the IPv4 ip:port other sites dial for the machine's
	// tunnel, when it joins through one: empty, the address at port 51820.
	// +kubebuilder:validation:MaxLength=21
	TunnelEndpoint string `json:"tunnelEndpoint,omitempty"`
}

// NodeJoinTiming is when one step of the join was seen.
type NodeJoinTiming struct {
	Event string      `json:"event"`
	At    metav1.Time `json:"at"`
}

// NodeJoinFacts are what the stages found and chose; later stages, and a
// retry, go on from them.
type NodeJoinFacts struct {
	OS             string                `json:"os,omitempty"`
	Kernel         string                `json:"kernel,omitempty"`
	GPUs           []ClusterProvisionGPU `json:"gpus,omitempty"`
	DiskFree       string                `json:"diskFree,omitempty"`
	PrivateAddress string                `json:"privateAddress,omitempty"`
	Route          NodeJoinRoute         `json:"route,omitempty"`
	// BundleSHA256 is the sha256 of the installer the machine holds.
	BundleSHA256 string `json:"bundleSHA256,omitempty"`
	// PublicKey is the machine's tunnel key, made on the machine.
	PublicKey     string `json:"publicKey,omitempty"`
	TunnelAddress string `json:"tunnelAddress,omitempty"`
	// DeclaredHub says this join declared the machine a hub in the SiteNetwork.
	DeclaredHub bool `json:"declaredHub,omitempty"`
	// DeclaredGeneration is the SiteNetwork's generation the declaration wrote.
	DeclaredGeneration int64 `json:"declaredGeneration,omitempty"`
	// AdmissionIssuedAt is when the join's code was given out.
	AdmissionIssuedAt *metav1.Time `json:"admissionIssuedAt,omitempty"`
	// Timings are the moments of the join's steps, in order.
	Timings []NodeJoinTiming `json:"timings,omitempty"`
}

// NodeJoinStatus is how the join went, stage by stage.
type NodeJoinStatus struct {
	Phase ClusterProvisionPhase `json:"phase,omitempty"`
	// Stage is the stage running, or the one the join stopped at.
	Stage string `json:"stage,omitempty"`
	// HostKey is the SHA256 fingerprint the machine showed the first time;
	// every later connection must show the same.
	HostKey string                  `json:"hostKey,omitempty"`
	Facts   NodeJoinFacts           `json:"facts,omitempty"`
	Stages  []ClusterProvisionStage `json:"stages,omitempty"`
}

// NodeJoin is the joining of a machine to a cluster over SSH: the record the
// console shows while the Gateway joins it and where it stopped. Its name is
// <cluster>.<node>; label rlark.io/cluster holds the cluster.
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=nodejoins,scope=Cluster
// +kubebuilder:printcolumn:name="Cluster",type=string,JSONPath=`.spec.clusterID`
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.spec.nodeName`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Stage",type=string,JSONPath=`.status.stage`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type NodeJoin struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NodeJoinSpec   `json:"spec"`
	Status            NodeJoinStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type NodeJoinList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeJoin `json:"items"`
}
