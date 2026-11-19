package constants

const (
	// ControlPlaneNodeRole labels a cluster's control plane nodes and taints them (kubeadm, the site's installer).
	ControlPlaneNodeRole = "node-role.kubernetes.io/control-plane"
)

// The agent's own ports. A cluster agent may run in the host's network (a site's management
// machine), next to other controllers, so it keeps off the ports controllers commonly default to
// (8080, 8081).
const (
	AgentMetricsPort = 10381
	AgentProbePort   = 10382
)

const (
	// KCPCredentialDir holds a control-plane component's own kcp credential: kubeconfig and token.
	KCPCredentialDir = "/etc/rlark/kcp"
)
