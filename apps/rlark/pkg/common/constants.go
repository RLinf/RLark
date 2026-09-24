package common

const (
	// SecretNamespace is the namespace where management cluster secrets live.
	SecretNamespace = "default"

	// SSHUserKeySecretName is the KCP Secret holding user SSH public keys.
	SSHUserKeySecretName = "rlark-ssh-keys"

	// UIAuthSecretName is the KCP Secret holding UI auth credentials.
	UIAuthSecretName       = "rlark-ui-auth"
	UIAuthAdminPasswordKey = "admin-password"
	UIAuthUserPasswordKey  = "user-password"
	UIAuthJWTSigningKey    = "jwt-signing-key"

	// AdminCertSecretName is the KCP Secret holding the admin signing cert.
	AdminCertSecretName = "rlark-admin-cert"

	// TLSCASecretName is the KCP Secret holding the TLS CA.
	TLSCASecretName = "rlark-tls-ca"

	// TLSSecretName is the KCP Secret holding the TLS cert/key pair.
	TLSSecretName = "rlark-tls"

	// ClientCASecretName is the KCP Secret holding the client CA.
	ClientCASecretName = "rlark-client-ca"

	// AgentCertSecretPrefix is the prefix for per-cluster agent cert secrets.
	AgentCertSecretPrefix = "rlark-agent-cert-"

	ImageRegistryReplicationLabel  = "rlark.io/image-registry-replication"
	ImageRegistryDeliveryLabel     = "rlark.io/image-registry-delivery"
	ImageRegistryCredentialLabel   = "rlark.io/image-registry-credential"
	ImageRegistryCredentialDataKey = "credential.json"

	// ImageRegistryAnnotationRegistry is the annotation storing the registry URL.
	ImageRegistryAnnotationRegistry = "rlark.io/registry"
	// ImageRegistryAnnotationUsername is the annotation storing the registry username.
	ImageRegistryAnnotationUsername = "rlark.io/username"

	// SystemConfigSecretName is the KCP Secret holding platform system configuration.
	SystemConfigSecretName = "rlark-system-config"
)

// SystemConfig keys stored in the SystemConfigSecret data field.
const (
	SystemConfigKeySSHJumpHost = "sshJumpHost"
	SystemConfigKeySSHJumpPort = "sshJumpPort"

	// SystemConfigKeySSH stores SSH config as a JSON object.
	SystemConfigKeySSH = "ssh"
	// SystemConfigKeyLog stores log backend config as a JSON object.
	SystemConfigKeyLog = "log"
	// SystemConfigKeyDeployment stores data-plane deployment YAML defaults as a JSON object.
	SystemConfigKeyDeployment = "deployment"
)

// Constants used by the package.
const (
	AgentCertLabelKey   = "rlark.io/agent-cert"
	AgentCertLabelValue = "true"

	DomainSuffix = "rlark-domain"
)
