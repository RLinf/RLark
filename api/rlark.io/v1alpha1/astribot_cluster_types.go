package v1alpha1

// ClusterNodeAgents declares the node Agents of a cluster: which Kubernetes Nodes run one, and of which image.
// They connect to the Server with the cluster Agent's credentials.
type ClusterNodeAgents struct {
	// Enabled makes the cluster Agent keep the node Agent DaemonSet; false makes it remove the DaemonSet.
	Enabled bool `json:"enabled"`
	// Image is the node Agents' image. It is the node Agents' own: neither the cluster Agent's image nor the
	// image of the tools the cluster Agent gives Task Pods (its --image), so that each of the three is replaced
	// by itself. Required while Enabled; without it the cluster Agent leaves the DaemonSet as it is.
	// +optional
	Image string `json:"image,omitempty"`
	// NodeSelector selects the Kubernetes Nodes that run a node Agent. It defaults to rlark.io/enabled=true,
	// the label that makes RLark manage a Kubernetes Node.
	// +kubebuilder:default={"rlark.io/enabled":"true"}
	// +kubebuilder:validation:MinProperties=1
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Rollout is how the node Agents are replaced when their DaemonSet changes. Absent, the defaults of
	// RolloutPolicy: one node first, then a tenth of the ready nodes at a time.
	// +optional
	Rollout *RolloutPolicy `json:"rollout,omitempty"`
}
