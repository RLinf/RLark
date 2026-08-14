package common

const (
	// SecretNamespace is the namespace where management cluster secrets live.
	SecretNamespace = "default"

	// SSHUserKeySecretName is the KCP Secret holding user SSH public keys.
	SSHUserKeySecretName = "rlark-ssh-keys"

	// UIAuthSecretName is the KCP Secret holding UI auth credentials.
	UIAuthSecretName = "rlark-ui-auth"

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

	// ImageRegistrySecretLabel is the label on Secrets that hold image registry credentials.
	ImageRegistrySecretLabel = "rlark.io/image-registry"

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
)

// Constants used by the package.
const (
	AgentCertLabelKey   = "rlark.io/agent-cert"
	AgentCertLabelValue = "true"

	DomainSuffix = "rlark-domain"
)
