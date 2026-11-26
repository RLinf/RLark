package v1alpha1

const (
	WorkflowLabel = "rlinf.io/workflow"

	ClusterStoppedAnnotation = "rlark.io/stopped"
	// ClusterDrainSuspendedAnnotation records that Task.Spec.Suspended was set by a
	// cluster drain, so cancelling the drain only resumes Tasks the drain suspended.
	ClusterDrainSuspendedAnnotation = "rlark.io/drain-suspended"

	// NodeDrainStoppedAnnotation holds the name of the Node whose completed drain
	// stopped this Task. The marker is written before the Drain operation leaves
	// Node.spec.operations and keeps the Task stopped afterwards, so retiring a
	// finished Drain cannot restart the workload on a still-cordoned Node.
	NodeDrainStoppedAnnotation = "rlark.io/node-drain-stopped"
	// NodeDrainSuspendedAnnotation holds the name of the Node whose drain set
	// Task.Spec.Suspended, so cancelling that drain only resumes Tasks the drain
	// suspended instead of overriding a user's own suspension.
	NodeDrainSuspendedAnnotation = "rlark.io/node-drain-suspended"

	RayRoleAnnotation          = "rlark.io/ray-role"
	RayHeadTaskNameAnnotation  = "rlark.io/ray-head-task-name"
	RayTotalNodesAnnotation    = "rlark.io/ray-total-nodes"
	RayNodeRankStartAnnotation = "rlark.io/ray-node-rank-start"
)
