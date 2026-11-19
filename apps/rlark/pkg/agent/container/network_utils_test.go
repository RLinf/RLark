package container

import (
	"testing"
)

func TestIsUUID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid uuid", "01234567-89ab-cdef-0123-456789abcdef", true},
		{"valid uuid uppercase", "01234567-89AB-CDEF-0123-456789ABCDEF", true},
		{"valid uuid mixed case", "01234567-89Ab-cDef-0123-456789AbCdEf", true},
		{"too short", "01234567-89ab-cdef-0123-456789abcde", false},
		{"too long", "01234567-89ab-cdef-0123-456789abcdef0", false},
		{"missing dashes", "0123456789abcdef0123456789abcdef", false},
		{"wrong dash position", "0123456-789ab-cdef-0123-456789abcdef", false},
		{"invalid char g", "01234567-89ab-cdef-0123-456789abcdefg", false},
		{"invalid char hyphen in wrong spot", "01234567-89ab-cdef-0123-456789abcde-", false},
		{"empty string", "", false},
		{"all zeros valid", "00000000-0000-0000-0000-000000000000", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUUID(tt.input); got != tt.want {
				t.Errorf("isUUID(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestPodUIDFromHostsSource(t *testing.T) {
	validUUID := "01234567-89ab-cdef-0123-456789abcdef"

	tests := []struct {
		name    string
		source  string
		wantUID string
		wantOK  bool
	}{
		{
			name:    "standard kubelet path",
			source:  "/var/lib/kubelet/pods/" + validUUID + "/etc/hosts",
			wantUID: validUUID,
			wantOK:  true,
		},
		{
			name:    "kubelet on custom mount",
			source:  "/data/kubelet/pods/" + validUUID + "/etc/hosts",
			wantUID: validUUID,
			wantOK:  true,
		},
		{
			name:    "kubelet on symlinked path",
			source:  "/mnt/ssd1/kubelet/pods/" + validUUID + "/etc/hosts",
			wantUID: validUUID,
			wantOK:  true,
		},
		{
			name:    "short custom path",
			source:  "/opt/kubelet/pods/" + validUUID + "/etc/hosts",
			wantUID: validUUID,
			wantOK:  true,
		},
		{
			name:    "no trailing components",
			source:  "/pods/" + validUUID + "/etc/hosts",
			wantUID: validUUID,
			wantOK:  true,
		},
		{
			name:    "invalid uuid in path",
			source:  "/var/lib/kubelet/pods/not-a-uuid/etc/hosts",
			wantUID: "",
			wantOK:  false,
		},
		{
			name:    "not a kubelet path",
			source:  "/some/other/mount",
			wantUID: "",
			wantOK:  false,
		},
		{
			name:    "missing /etc/hosts suffix",
			source:  "/var/lib/kubelet/pods/" + validUUID + "/etc/host",
			wantUID: "",
			wantOK:  false,
		},
		{
			name:    "no pods segment",
			source:  "/var/lib/kubelet/containers/" + validUUID + "/hosts",
			wantUID: "",
			wantOK:  false,
		},
		{
			name:    "empty string",
			source:  "",
			wantUID: "",
			wantOK:  false,
		},
		{
			name:    "multiple uuid-like segments but no pods",
			source:  "/var/lib/foo/" + validUUID + "/bar/" + validUUID + "/etc/hosts",
			wantUID: "",
			wantOK:  false,
		},
		{
			name:    "deep path with long prefix",
			source:  "/var/lib/kubelet/plugins/kubernetes.io/csi/pv/pvc-xxx/pods/" + validUUID + "/etc/hosts",
			wantUID: validUUID,
			wantOK:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotUID, gotOK := podUIDFromHostsSource(tt.source)
			if gotOK != tt.wantOK {
				t.Errorf("podUIDFromHostsSource(%q) ok = %v, want %v", tt.source, gotOK, tt.wantOK)
			}
			if gotUID != tt.wantUID {
				t.Errorf("podUIDFromHostsSource(%q) uid = %q, want %q", tt.source, gotUID, tt.wantUID)
			}
		})
	}
}
