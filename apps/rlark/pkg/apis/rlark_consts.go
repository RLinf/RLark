package apis

// Constants used by the package.
const (
	RLarkAgentNamespacePrefix    = "rlark-"
	RLarkAgentServiceAccountName = "rlark-agent"
	Namespace                    = "rlark-system"
)

// Constants used by the package.
const (
	MetaCertRole = "cert-role"

	MetaPermissionPrefix     = "permission:"
	MetaPermissionAdmin      = MetaPermissionPrefix + "admin"       // value: "true"
	MetaPermissionAgentProxy = MetaPermissionPrefix + "agent-proxy" // value: agent1,agent2,agent3

	MetaKubernetesImpersonation            = "kubernetes-impersonation"
	MetaKubernetesImpersonationGroup       = "kubernetes-impersonation-group"
	MetaKubernetesImpersonationUID         = "kubernetes-impersonation-uid"
	MetaKubernetesImpersonationExtraPrefix = "kubernetes-impersonation-extra-"

	MetaRemoteDialerPeerID    = "remote-dialer-peer-id"
	MetaRemoteDialerPeerToken = "remote-dialer-peer-token"
	MetaRemoteDialerClientID  = "remote-dialer-client-id"

	MetaAgentID   = "agent-id"
	MetaNamespace = "namespace"

	MetaDomainID = "domain-id"

	MetaUserID    = "user-id"
	MetaUserKeyID = "user-key-id"
)

// Constants used by the package.
const (
	RemoteDialerRoleHeader = "X-Remote-Dialer-Role"
)
