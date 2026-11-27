package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(GroupVersion, &ClusterProvision{}, &ClusterProvisionList{})
		return nil
	})
}

// ClusterProvisionPhase is where the making of a cluster stands.
// +kubebuilder:validation:Enum=Running;Succeeded;Failed
type ClusterProvisionPhase string

const (
	ClusterProvisionRunning   ClusterProvisionPhase = "Running"
	ClusterProvisionSucceeded ClusterProvisionPhase = "Succeeded"
	ClusterProvisionFailed    ClusterProvisionPhase = "Failed"
)

// ClusterProvisionStageState is where one stage stands.
// +kubebuilder:validation:Enum=Pending;Running;Done;Skipped;Failed
type ClusterProvisionStageState string

const (
	ClusterProvisionStagePending ClusterProvisionStageState = "Pending"
	ClusterProvisionStageRunning ClusterProvisionStageState = "Running"
	ClusterProvisionStageDone    ClusterProvisionStageState = "Done"
	// ClusterProvisionStageSkipped is a stage that had nothing to do; its readback says why.
	ClusterProvisionStageSkipped ClusterProvisionStageState = "Skipped"
	ClusterProvisionStageFailed  ClusterProvisionStageState = "Failed"
)

// ClusterProvisionSpec is what the cluster is made of: a new site, and its
// first machine, which becomes the site's first hub and the cluster's control
// plane. The machine's password is never part of it: it is given with the
// request that starts or retries the making, and lives only while that runs.
type ClusterProvisionSpec struct {
	// Site is the new site's name in the SiteNetwork.
	// +kubebuilder:validation:MinLength=1
	Site string `json:"site"`
	// Machine is how the Gateway reaches the first machine over SSH.
	Machine ClusterProvisionMachine `json:"machine"`
	// RequestedBy is the user who asked for the cluster.
	RequestedBy string `json:"requestedBy,omitempty"`
}

// ClusterProvisionMachine is the SSH address and account of the first machine.
type ClusterProvisionMachine struct {
	// Address is the machine's address as the management plane reaches it.
	// +kubebuilder:validation:MinLength=1
	Address string `json:"address"`
	// +kubebuilder:default=22
	Port int32 `json:"port,omitempty"`
	// +kubebuilder:validation:MinLength=1
	User string `json:"user"`
	// TunnelEndpoint is the IPv4 ip:port other sites dial for the machine's
	// tunnel, and the machine listens on its port: Address, port 51820, for a
	// machine with an address of its own; the address and port its cloud
	// forwards to it for machines behind one address.
	// +kubebuilder:validation:MinLength=9
	// +kubebuilder:validation:MaxLength=21
	TunnelEndpoint string `json:"tunnelEndpoint"`
}

// ClusterProvisionFacts are what the stages found and chose; later stages,
// and a retry, go on from them.
type ClusterProvisionFacts struct {
	NodeName       string                `json:"nodeName,omitempty"`
	OS             string                `json:"os,omitempty"`
	Kernel         string                `json:"kernel,omitempty"`
	GPUs           []ClusterProvisionGPU `json:"gpus,omitempty"`
	DiskFree       string                `json:"diskFree,omitempty"`
	PrivateAddress string                `json:"privateAddress,omitempty"`
	Routes         []string              `json:"routes,omitempty"`
	CacheVolume    bool                  `json:"cacheVolume,omitempty"`
	BundleSHA256   string                `json:"bundleSHA256,omitempty"`
	// RegistryCA is the CA of the site registry the machine's installer trusts (PEM).
	RegistryCA       string `json:"registryCA,omitempty"`
	PublicKey        string `json:"publicKey,omitempty"`
	TunnelAddress    string `json:"tunnelAddress,omitempty"`
	ControlPlaneName string `json:"controlPlaneName,omitempty"`
	PodSubnet        string `json:"podSubnet,omitempty"`
	ServiceSubnet    string `json:"serviceSubnet,omitempty"`
	Release          string `json:"release,omitempty"`
}

// ClusterProvisionGPU is one model of GPU on the machine.
type ClusterProvisionGPU struct {
	Model string `json:"model"`
	Count int32  `json:"count"`
}

// ClusterProvisionStage is one stage, as it went.
type ClusterProvisionStage struct {
	Name  string                     `json:"name"`
	State ClusterProvisionStageState `json:"state"`
	// Readback is what was read back once the stage had done its work.
	Readback string `json:"readback,omitempty"`
	// Message is why the stage stopped, or the last line of its progress.
	Message    string       `json:"message,omitempty"`
	StartedAt  *metav1.Time `json:"startedAt,omitempty"`
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`
}

// ClusterProvisionStatus is how the making went, stage by stage.
type ClusterProvisionStatus struct {
	Phase ClusterProvisionPhase `json:"phase,omitempty"`
	// Stage is the stage running, or the one the making stopped at.
	Stage string `json:"stage,omitempty"`
	// HostKey is the SHA256 fingerprint the machine showed the first time;
	// every later connection must show the same.
	HostKey string                  `json:"hostKey,omitempty"`
	Facts   ClusterProvisionFacts   `json:"facts,omitempty"`
	Stages  []ClusterProvisionStage `json:"stages,omitempty"`
}

// ClusterProvision is the making of a cluster from a machine: the record the
// console shows while the Gateway makes it and where it stopped. Its name is
// the cluster's.
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=clusterprovisions,scope=Cluster
// +kubebuilder:printcolumn:name="Site",type=string,JSONPath=`.spec.site`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Stage",type=string,JSONPath=`.status.stage`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type ClusterProvision struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClusterProvisionSpec   `json:"spec"`
	Status            ClusterProvisionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type ClusterProvisionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProvision `json:"items"`
}
