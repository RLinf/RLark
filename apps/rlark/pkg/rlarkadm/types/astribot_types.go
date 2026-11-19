package types

// InstallCredentialName is the pull Secret rlarkadm makes of a credential: the agent delivers the credential itself
// under its own name.
func InstallCredentialName(name string) string { return name + "-install" }

// ImagePullCredential is one image registry credential as a pull Secret holds it.
type ImagePullCredential struct {
	Name             string `json:"name" yaml:"name"`
	DockerConfigJSON string `json:"dockerConfigJSON" yaml:"docker-config-json"`
}
