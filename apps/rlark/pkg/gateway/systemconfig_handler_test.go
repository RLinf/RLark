package gateway

import (
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/logquery"
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
