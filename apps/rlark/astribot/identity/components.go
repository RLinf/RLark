package identity

import (
	"fmt"
	"net/url"

	rbacv1 "k8s.io/api/rbac/v1"
)

// RootContext is the context of kcp's root workspace in kcp's administrator kubeconfig. The tokens
// are signed for that workspace and good in it only.
const RootContext = "root"

// Namespace holds the RLark components' accounts in kcp.
const Namespace = "rlark-system"

// StationNamespace holds Station's accounts in kcp and its Secrets in the cluster it runs in.
const StationNamespace = "station-system"

// Server is kcp's address for the components: the administrator's, or override's scheme and host with
// the administrator's path (the root workspace's).
func Server(admin, override string) (string, error) {
	if override == "" {
		return admin, nil
	}
	a, err := url.Parse(admin)
	if err != nil {
		return "", err
	}
	o, err := url.Parse(override)
	if err != nil || o.Scheme == "" || o.Host == "" {
		return "", fmt.Errorf("kcp server %q: want scheme://host:port", override)
	}
	a.Scheme, a.Host = o.Scheme, o.Host
	return a.String(), nil
}

// RLark are RLark's own clients of the management plane.
func RLark() []Component {
	return []Component{
		{
			// The server forwards the requests of administrators' certificates as itself and those of
			// agents as the agents' ServiceAccounts: it holds what an administrator holds in the workspace.
			Name: "rlark-server", Namespace: Namespace, ServiceAccount: "rlark-server",
			ClusterRole: "cluster-admin",
		},
		{
			Name: "rlark-controller-manager", Namespace: Namespace, ServiceAccount: "rlark-controller-manager",
			Rules: controllerManagerRules,
		},
		{
			Name: "rlark-gateway", Namespace: Namespace, ServiceAccount: "rlark-gateway",
			Rules: gatewayRules,
		},
		{
			// The release command (rlarkctl release apply) on the management machine: it writes the
			// release's definitions and reads what a release is checked against.
			Name: "rlark-release", Namespace: Namespace, ServiceAccount: "rlark-release",
			Rules: releaseRules,
		},
	}
}

// RLarkSecretName is the Secret holding an RLark component's credential in rlarkadm's Kubernetes mode.
func RLarkSecretName(c Component) string { return c.Name + "-kcp" }

// Station are Station's clients of the management plane. Their accounts and permissions are
// Station's own (astribot/station/deploy/management-rbac.yaml); only their tokens are issued here.
func Station() []Component {
	return []Component{
		{Name: "station-api", Namespace: StationNamespace, ServiceAccount: "station-api"},
		{Name: "station-controller", Namespace: StationNamespace, ServiceAccount: "station-controller"},
	}
}

// StationSecretName is the Secret Station's Deployments mount their management kubeconfig from.
func StationSecretName(c Component) string {
	if c.Name == "station-controller" {
		return "station-management-kubeconfig-controller"
	}
	return "station-management-kubeconfig"
}

var (
	read = []string{"get", "list", "watch"}
	rw   = []string{"get", "list", "watch", "create", "update", "patch", "delete"}
)

// The controller-manager's permissions, from what it reads and writes (11-16 audit). Its replication
// applies whatever a declaration names, which today is Secrets and ConfigMaps.
var controllerManagerRules = []rbacv1.PolicyRule{
	{APIGroups: []string{"rlinf.io"}, Resources: []string{"jobs", "workflows", "tasks", "nodes", "clusters", "domains", "domainpeers"}, Verbs: rw},
	{APIGroups: []string{"rlinf.io"}, Resources: []string{"pods"}, Verbs: read},
	{APIGroups: []string{"rlinf.io"}, Resources: []string{"jobs/status", "workflows/status", "tasks/status", "nodes/status", "clusters/status", "domains/status", "jobs/finalizers", "workflows/finalizers"}, Verbs: []string{"update", "patch"}},
	{APIGroups: []string{""}, Resources: []string{"secrets", "configmaps"}, Verbs: rw},
	{APIGroups: []string{""}, Resources: []string{"namespaces"}, Verbs: read},
	{APIGroups: []string{"coordination.k8s.io"}, Resources: []string{"leases"}, Verbs: rw},
	{APIGroups: []string{"", "events.k8s.io"}, Resources: []string{"events"}, Verbs: []string{"create", "patch"}},
}

// The release command's permissions (apps/rlark/astribot/release): the definitions of a release's tree (CRDs and
// Station's management RBAC, which it can only write holding what they grant: escalate and bind); the objects a
// release writes as the Gateway writes them (an Addon and its Package, a cluster's node agents, a standing Job);
// what its checks read; the record of the release the site runs; its credentials for the member clusters; and its
// lease.
var releaseRules = []rbacv1.PolicyRule{
	{APIGroups: []string{"apiextensions.k8s.io"}, Resources: []string{"customresourcedefinitions"}, Verbs: []string{"get", "list", "create", "update", "patch"}},
	{APIGroups: []string{"rbac.authorization.k8s.io"}, Resources: []string{"clusterroles", "clusterrolebindings", "roles", "rolebindings"}, Verbs: []string{"get", "list", "create", "update", "patch", "escalate", "bind"}},
	{APIGroups: []string{""}, Resources: []string{"serviceaccounts", "namespaces"}, Verbs: []string{"get", "list", "create", "update", "patch"}},
	{APIGroups: []string{"rlinf.io"}, Resources: []string{"clusters", "addons", "jobs"}, Verbs: []string{"get", "list", "watch", "create", "update"}},
	{APIGroups: []string{"rlinf.io"}, Resources: []string{"nodes"}, Verbs: read},
	{APIGroups: []string{"station.rlark.io"}, Resources: []string{"sitenetworks", "networkenrollments", "stationobservations"}, Verbs: read},
	// Addon Packages and the record of the release the site runs (rlark-system/rlark-release-current).
	{APIGroups: []string{""}, Resources: []string{"configmaps"}, Verbs: []string{"get", "create", "update"}},
	// The release's credentials for the member clusters (rlark-release-member-credentials).
	{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get", "list", "create", "update"}},
	{APIGroups: []string{"coordination.k8s.io"}, Resources: []string{"leases"}, Verbs: rw},
}

// The gateway's permissions: the API it serves over RLark's and Station's objects (11-16 audit).
var gatewayRules = []rbacv1.PolicyRule{
	{APIGroups: []string{"rlinf.io"}, Resources: []string{"jobs", "workflows", "domains", "nodes", "tasks", "pods", "clusters", "addons"}, Verbs: append(rw, "deletecollection")},
	// A new cluster is made, and a machine joined over SSH, by the Gateway, which records how it goes (ClusterProvision, NodeJoin).
	{APIGroups: []string{"rlinf.io"}, Resources: []string{"clusterprovisions", "clusterprovisions/status", "nodejoins", "nodejoins/status"}, Verbs: rw},
	{APIGroups: []string{""}, Resources: []string{"secrets", "configmaps"}, Verbs: append(rw, "deletecollection")},
	// The namespaces of the member clusters' credentials a new cluster's are written into.
	{APIGroups: []string{""}, Resources: []string{"namespaces"}, Verbs: []string{"get", "create"}},
	{APIGroups: []string{"station.rlark.io"}, Resources: []string{"stations", "stationobservations", "networkenrollments", "stationrebinds", "nodeoperations", "nodeadmissions", "nodeclasses", "machines", "sitenetworks"}, Verbs: rw},
}
