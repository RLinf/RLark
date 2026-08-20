package types

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDeployConfigRejectsCamelCaseUnknownField(t *testing.T) {
	path := filepath.Join(t.TempDir(), "deploy.yaml")
	data := []byte(`apiVersion: rlinf.io/v1alpha1
kind: DeployConfig
plane: data
controlPlaneAddress: https://rlark.example.com
cert:
  ca-cert: ca
  agent-cert: cert
  agent-key: key
kubernetes:
  agent-image: rlark:latest
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write deploy config: %v", err)
	}

	_, err := LoadDeployConfig(path)
	if err == nil {
		t.Fatal("LoadDeployConfig() expected an unknown field error")
	}
	if !strings.Contains(err.Error(), "field controlPlaneAddress not found") {
		t.Fatalf("LoadDeployConfig() error = %q, want unknown controlPlaneAddress field", err)
	}
}

func TestDeployConfigValidateRequiresDataPlaneCertificates(t *testing.T) {
	tests := []struct {
		name    string
		cert    *CertConfig
		wantErr string
	}{
		{
			name:    "missing cert config",
			wantErr: "cert is required for data plane",
		},
		{
			name:    "missing CA certificate",
			cert:    &CertConfig{AgentCert: "cert", AgentKey: "key"},
			wantErr: "cert.ca-cert is required for data plane",
		},
		{
			name:    "missing agent certificate",
			cert:    &CertConfig{CACert: "ca", AgentKey: "key"},
			wantErr: "cert.agent-cert is required for data plane",
		},
		{
			name:    "missing agent key",
			cert:    &CertConfig{CACert: "ca", AgentCert: "cert"},
			wantErr: "cert.agent-key is required for data plane",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DeployConfig{
				APIVersion:          "rlinf.io/v1alpha1",
				Kind:                "DeployConfig",
				Plane:               PlaneData,
				ControlPlaneAddress: "https://rlark.example.com",
				Kubernetes:          &KubernetesEnv{},
				Cert:                tt.cert,
			}

			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate() expected %q", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("Validate() error = %q, want %q", err, tt.wantErr)
			}
		})
	}
}
