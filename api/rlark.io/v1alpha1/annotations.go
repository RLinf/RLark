package v1alpha1

const (
	WorkflowLabel               = "rlinf.io/workflow"
	WorkflowStoppedByAnnotation = "rlinf.io/workflow-stopped-by"

	RayRoleAnnotation          = "rlark.io/ray-role"
	RayHeadTaskNameAnnotation  = "rlark.io/ray-head-task-name"
	RayTotalNodesAnnotation    = "rlark.io/ray-total-nodes"
	RayNodeRankStartAnnotation = "rlark.io/ray-node-rank-start"

	RayRoleHead   = "head"
	RayRoleWorker = "worker"
)
