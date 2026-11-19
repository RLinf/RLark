package v1alpha1

const (
	WorkflowLabel               = "rlinf.io/workflow"
	WorkflowStoppedByAnnotation = "rlinf.io/workflow-stopped-by"

	ClusterStoppedAnnotation = "rlark.io/stopped"
	// ClusterDrainSuspendedAnnotation records that Task.Spec.Suspended was set by a
	// cluster drain, so cancelling the drain only resumes Tasks the drain suspended.
	ClusterDrainSuspendedAnnotation = "rlark.io/drain-suspended"

	RayRoleAnnotation          = "rlark.io/ray-role"
	RayHeadTaskNameAnnotation  = "rlark.io/ray-head-task-name"
	RayTotalNodesAnnotation    = "rlark.io/ray-total-nodes"
	RayNodeRankStartAnnotation = "rlark.io/ray-node-rank-start"
)
