package gateway

import (
	"encoding/json"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/logquery"
	rlarkadmtypes "github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm/types"
	corev1 "k8s.io/api/core/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
)

func TestValidateSSHConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  sshConfig
		wantErr bool
	}{
		{name: "empty", config: sshConfig{}},
		{name: "hostname and port", config: sshConfig{JumpHost: "jump.example.com", JumpPort: "2222"}},
		{name: "port without host", config: sshConfig{JumpPort: "22"}, wantErr: true},
		{name: "host with scheme", config: sshConfig{JumpHost: "ssh://jump.example.com"}, wantErr: true},
		{name: "host with port", config: sshConfig{JumpHost: "jump.example.com:22"}, wantErr: true},
		{name: "invalid port", config: sshConfig{JumpHost: "jump.example.com", JumpPort: "65536"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSSHConfig(&test.config)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateSSHConfig() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateDeploymentConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  rlarkadmtypes.DeployConfig
		wantErr bool
	}{
		{name: "valid", config: rlarkadmtypes.DeployConfig{ControlPlaneAddress: "https://rlark.example.com:8443", Kubernetes: &rlarkadmtypes.KubernetesEnv{Kubeconfig: "/etc/kubernetes/admin.conf", ImagePullPolicy: "IfNotPresent", ContainerdSocket: "/run/containerd/containerd.sock"}}},
		{name: "address omitted for signing fallback", config: rlarkadmtypes.DeployConfig{Kubernetes: &rlarkadmtypes.KubernetesEnv{AgentImage: "rlark:latest"}}},
		{name: "missing kubernetes", config: rlarkadmtypes.DeployConfig{ControlPlaneAddress: "https://rlark.example.com:8443"}, wantErr: true},
		{name: "invalid pull policy", config: rlarkadmtypes.DeployConfig{ControlPlaneAddress: "https://rlark.example.com:8443", Kubernetes: &rlarkadmtypes.KubernetesEnv{ImagePullPolicy: "Sometimes"}}, wantErr: true},
		{name: "control plane field", config: rlarkadmtypes.DeployConfig{ControlPlaneAddress: "https://rlark.example.com:8443", Kubernetes: &rlarkadmtypes.KubernetesEnv{ServerImage: "server:v1"}}, wantErr: true},
		{name: "certificate field", config: rlarkadmtypes.DeployConfig{ControlPlaneAddress: "https://rlark.example.com:8443", Kubernetes: &rlarkadmtypes.KubernetesEnv{}, Cert: &rlarkadmtypes.CertConfig{CACert: "unexpected"}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDeploymentConfig(&test.config)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateDeploymentConfig() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestValidateDeploymentConfigPreservesEmptyAddress(t *testing.T) {
	config := &rlarkadmtypes.DeployConfig{Kubernetes: &rlarkadmtypes.KubernetesEnv{AgentImage: "rlark:latest"}}
	if err := validateDeploymentConfig(config); err != nil {
		t.Fatalf("validateDeploymentConfig() error = %v", err)
	}
	if config.ControlPlaneAddress != "" {
		t.Fatalf("ControlPlaneAddress = %q, want empty", config.ControlPlaneAddress)
	}
}

func TestDeploymentConfigRoundTrip(t *testing.T) {
	want := &rlarkadmtypes.DeployConfig{ControlPlaneAddress: "https://rlark.example.com:8443", SSHAddress: "client@rlark.example.com:2222", Kubernetes: &rlarkadmtypes.KubernetesEnv{Kubeconfig: "~/.kube/config", AgentImage: "rlark:v1", Image: "rlark:v1", ImagePullPolicy: "IfNotPresent", ImagePullSecrets: []string{"registry-secret"}, ContainerdSocket: "/run/containerd/containerd.sock"}}
	secret := &corev1.Secret{Data: map[string][]byte{}}
	if err := writeSystemConfigToSecret(secret, &systemConfig{Deployment: want}); err != nil {
		t.Fatalf("writeSystemConfigToSecret() error = %v", err)
	}
	if !json.Valid(secret.Data[common.SystemConfigKeyDeployment]) {
		t.Fatal("deployment config was not stored as JSON")
	}
	got := readSystemConfig(secret).Deployment
	if got == nil || got.ControlPlaneAddress != want.ControlPlaneAddress || got.SSHAddress != want.SSHAddress || got.Kubernetes == nil || got.Kubernetes.Kubeconfig != want.Kubernetes.Kubeconfig || got.Kubernetes.AgentImage != want.Kubernetes.AgentImage || got.Kubernetes.Image != want.Kubernetes.Image || got.Kubernetes.ImagePullPolicy != want.Kubernetes.ImagePullPolicy || len(got.Kubernetes.ImagePullSecrets) != 1 || got.Kubernetes.ContainerdSocket != want.Kubernetes.ContainerdSocket {
		t.Fatalf("deployment config = %#v, want %#v", got, want)
	}
}

func TestDeploymentConfigJSONUsesAPIFieldNames(t *testing.T) {
	raw := []byte(`{"controlPlaneAddress":"https://rlark.example.com:8443","sshAddress":"client@rlark.example.com:2222","kubernetes":{"kubeconfig":"~/.kube/config","agentImage":"agent:v1","imagePullPolicy":"Never","containerdSocket":"/run/k3s/containerd/containerd.sock"}}`)
	var config rlarkadmtypes.DeployConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if config.ControlPlaneAddress == "" || config.SSHAddress == "" || config.Kubernetes == nil || config.Kubernetes.AgentImage != "agent:v1" || config.Kubernetes.ImagePullPolicy != "Never" {
		t.Fatalf("deployment config was not decoded from API field names: %#v", config)
	}
}

func TestPreserveMaskedLogFields(t *testing.T) {
	incoming := &logquery.Config{Backend: "sls", Config: map[string]interface{}{
		"endpoint": "new", "accessKeyId": "****", "accessKeySecret": "****",
	}}
	existing := &logquery.Config{Backend: "sls", Config: map[string]interface{}{
		"accessKeyId": "real-id", "accessKeySecret": "real-secret",
	}}

	preserveMaskedLogFields(incoming, existing)

	if incoming.Config["accessKeyId"] != "real-id" || incoming.Config["accessKeySecret"] != "real-secret" {
		t.Fatalf("masked credentials were not preserved: %#v", incoming.Config)
	}
}
