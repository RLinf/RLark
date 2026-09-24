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

func TestDeployConfigSSHServerAddress(t *testing.T) {
	tests := []struct {
		name         string
		sshAddress   string
		controlPlane string
		want         string
	}{
		{
			name:       "explicit ssh-address takes precedence",
			sshAddress: "operator@ssh.example.com:2200",
			// control-plane-address is ignored when ssh-address is set.
			controlPlane: "https://rlark-server:8443",
			want:         "operator@ssh.example.com:2200",
		},
		{
			name:         "derive from https url with port",
			controlPlane: "https://rlark-server.rlark-system.svc:8443",
			want:         "client@rlark-server.rlark-system.svc:2222",
		},
		{
			name:         "derive from https url without port",
			controlPlane: "https://rlark.example.com",
			want:         "client@rlark.example.com:2222",
		},
		{
			name:         "derive from host:port without scheme",
			controlPlane: "rlark-server:8443",
			want:         "client@rlark-server:2222",
		},
		{
			name:         "derive from bare host",
			controlPlane: "rlark-server",
			want:         "client@rlark-server:2222",
		},
		{
			name:         "empty when nothing configured",
			controlPlane: "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DeployConfig{
				ControlPlaneAddress: tt.controlPlane,
				SSHAddress:          tt.sshAddress,
			}
			if got := cfg.SSHServerAddress(); got != tt.want {
				t.Fatalf("SSHServerAddress() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDeployConfigManagementAPI(t *testing.T) {
	tests := []struct {
		name          string
		plane         Plane
		managementAPI string
		wantNative    bool
		wantErr       string
	}{
		{name: "default kcp", plane: PlaneControl},
		{name: "explicit kcp", plane: PlaneControl, managementAPI: "kcp"},
		{name: "current kubernetes", plane: PlaneControl, managementAPI: "kubernetes", wantNative: true},
		{name: "unknown", plane: PlaneControl, managementAPI: "other", wantErr: "kubernetes.management-api must be"},
		{name: "data plane", plane: PlaneData, managementAPI: "kubernetes", wantErr: "is only supported for the control plane"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DeployConfig{
				APIVersion: "rlark.io/v1alpha1",
				Kind:       "DeployConfig",
				Plane:      tt.plane,
				Kubernetes: &KubernetesEnv{ManagementAPI: tt.managementAPI},
			}
			if tt.plane == PlaneData {
				cfg.ControlPlaneAddress = "https://rlark.example.com"
				cfg.Cert = &CertConfig{CACert: "ca", AgentCert: "cert", AgentKey: "key"}
			}
			err := cfg.Validate()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if got := cfg.UsesKubernetesManagementAPI(); got != tt.wantNative {
				t.Fatalf("UsesKubernetesManagementAPI() = %v, want %v", got, tt.wantNative)
			}
		})
	}
}

func TestDeployConfigRejectsMultipleKCPReplicas(t *testing.T) {
	cfg := DeployConfig{
		APIVersion: "rlark.io/v1alpha1",
		Kind:       "DeployConfig",
		Plane:      PlaneControl,
		Kubernetes: &KubernetesEnv{
			KCP: &ComponentConfig{Replicas: 2},
		},
	}
	if err := cfg.Validate(); err == nil || err.Error() != "kubernetes.kcp.replicas must be 1" {
		t.Fatalf("Validate() error = %v, want kcp replica limit", err)
	}
}
