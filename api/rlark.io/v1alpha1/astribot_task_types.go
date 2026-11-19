package v1alpha1

// TaskInstancePhase is the state of one instance of a per-node Task.
// +kubebuilder:validation:Enum=Pending;Running;Failed
type TaskInstancePhase string

const (
	// TaskInstancePending is an instance that is placed but not yet ready,
	// including one on a node that stopped reporting.
	TaskInstancePending TaskInstancePhase = "Pending"
	// TaskInstanceRunning is an instance whose containers run and are ready.
	TaskInstanceRunning TaskInstancePhase = "Running"
	// TaskInstanceFailed is an instance whose container cannot start or keeps
	// crashing. The platform keeps restarting it; the Task does not fail.
	TaskInstanceFailed TaskInstancePhase = "Failed"
)

// TaskInstanceStatus is one instance of a per-node Task as the data-plane Agent
// observed it.
type TaskInstanceStatus struct {
	// Node is the name of the node the instance runs on, empty until the
	// instance is placed.
	Node string `json:"node,omitempty"`
	// Name is the instance's name in its runtime (the Pod name on Kubernetes).
	Name  string            `json:"name"`
	Phase TaskInstancePhase `json:"phase"`
	// Image is the image of the instance's main container: the version the
	// instance runs, which differs between nodes while an update rolls out.
	Image   string `json:"image,omitempty"`
	Message string `json:"message,omitempty"`
}
