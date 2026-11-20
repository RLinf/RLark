package types

// InstallCredentialName is the pull Secret rlarkadm makes of a credential: the agent delivers the credential itself
// under its own name.
func InstallCredentialName(name string) string { return name + "-install" }

// ImagePullCredential is one image registry credential as a pull Secret holds it.
type ImagePullCredential struct {
	Name             string `json:"name" yaml:"name"`
	DockerConfigJSON string `json:"dockerConfigJSON" yaml:"docker-config-json"`
}

// AstribotKubernetesEnv is what the site adds to KubernetesEnv. The site takes away KubernetesEnv.ContainerdSocket
// as a key (json and yaml "-", so a config naming containerd-socket is refused): it configured only the node agent
// DaemonSet rlarkadm no longer deploys (astribot_component.go); the node agents are the Cluster's declaration
// (spec.nodeAgents) and take the runtime socket from each node's kubeadm.alpha.kubernetes.io/cri-socket annotation.
type AstribotKubernetesEnv struct {
	// ImagePullCredentials are the image registry credentials RLark gives the cluster with its agent's certificate
	// (the Gateway's answer, image_pull_secrets): put in before the agent, whose own image is pulled with them, and
	// kept by the agent from then on.
	ImagePullCredentials []ImagePullCredential `json:"imagePullCredentials,omitempty" yaml:"image-pull-credentials,omitempty"`
}
