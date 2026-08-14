package deviceplugin

import (
	"testing"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// TestHostDeviceSpecs_Defaults verifies that empty ContainerPath and
// Permissions are filled in with sensible defaults (host_path and "rwm"
// respectively).
func TestHostDeviceSpecs_Defaults(t *testing.T) {
	in := []HostDeviceConfig{
		{HostPath: "/dev/video0"},
		{HostPath: "/dev/ttyUSB0", ContainerPath: "/dev/usb0", Permissions: "rw"},
	}
	out := hostDeviceSpecs(in)
	if len(out) != 2 {
		t.Fatalf("got %d specs, want 2", len(out))
	}
	if out[0].HostPath != "/dev/video0" {
		t.Errorf("out[0].HostPath = %q", out[0].HostPath)
	}
	if out[0].ContainerPath != "/dev/video0" {
		t.Errorf("out[0].ContainerPath = %q, want /dev/video0", out[0].ContainerPath)
	}
	if out[0].Permissions != "rwm" {
		t.Errorf("out[0].Permissions = %q, want rwm", out[0].Permissions)
	}
	if out[1].ContainerPath != "/dev/usb0" {
		t.Errorf("out[1].ContainerPath = %q, want /dev/usb0", out[1].ContainerPath)
	}
	if out[1].Permissions != "rw" {
		t.Errorf("out[1].Permissions = %q, want rw", out[1].Permissions)
	}
}

// TestHostDeviceSpecs_SkipsEmptyHostPath verifies that entries with an empty
// HostPath are dropped instead of producing an invalid DeviceSpec.
func TestHostDeviceSpecs_SkipsEmptyHostPath(t *testing.T) {
	in := []HostDeviceConfig{
		{HostPath: ""},
		{HostPath: "/dev/video0"},
		{ContainerPath: "/dev/usb0", Permissions: "rw"}, // no host_path
	}
	out := hostDeviceSpecs(in)
	if len(out) != 1 {
		t.Fatalf("got %d specs, want 1 (empty host_path entries skipped)", len(out))
	}
	if out[0].HostPath != "/dev/video0" {
		t.Errorf("out[0].HostPath = %q", out[0].HostPath)
	}
}

// TestHostDeviceSpecs_EmptyInput verifies that a nil/empty input yields an
// empty (non-nil) slice.
func TestHostDeviceSpecs_EmptyInput(t *testing.T) {
	out := hostDeviceSpecs(nil)
	if len(out) != 0 {
		t.Fatalf("got %d specs, want 0", len(out))
	}
	out = hostDeviceSpecs([]HostDeviceConfig{})
	if len(out) != 0 {
		t.Fatalf("got %d specs, want 0", len(out))
	}
}

// TestAllocateContainer_HostDevices verifies that Allocate injects the
// configured host device specs and the RLINF_EMBODIED_HOST_DEVICES_ENABLED env
// var when host_devices is non-empty, and omits both when it is empty.
func TestAllocateContainer_HostDevices(t *testing.T) {
	cases := []struct {
		name     string
		hostDevs []HostDeviceConfig
		wantDevs int
		wantEnv  bool
	}{
		{
			name:     "none",
			hostDevs: nil,
			wantDevs: 0,
			wantEnv:  false,
		},
		{
			name: "two devices",
			hostDevs: []HostDeviceConfig{
				{HostPath: "/dev/video0"},
				{HostPath: "/dev/ttyUSB0", Permissions: "rw"},
			},
			wantDevs: 2,
			wantEnv:  true,
		},
		{
			name: "empty host_path skipped",
			hostDevs: []HostDeviceConfig{
				{HostPath: ""},
				{HostPath: "/dev/video0"},
			},
			wantDevs: 1,
			wantEnv:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Plugin{cfg: PluginConfig{HostDevices: tc.hostDevs}}
			resp := p.allocateContainer(&pluginapi.ContainerAllocateRequest{
				DevicesIds: []string{"device-0"},
			})
			if len(resp.Devices) != tc.wantDevs {
				t.Errorf("got %d device specs, want %d", len(resp.Devices), tc.wantDevs)
			}
			v, ok := resp.Envs["RLINF_EMBODIED_HOST_DEVICES_ENABLED"]
			if tc.wantEnv {
				if !ok {
					t.Error("missing RLINF_EMBODIED_HOST_DEVICES_ENABLED env")
				} else if v != "1" {
					t.Errorf("RLINF_EMBODIED_HOST_DEVICES_ENABLED = %q, want 1", v)
				}
			} else {
				if ok {
					t.Errorf("RLINF_EMBODIED_HOST_DEVICES_ENABLED should not be set, got %q", v)
				}
			}
			// Runtime-enabled env is always present regardless of host devices.
			if v, ok := resp.Envs["RLINF_EMBODIED_RUNTIME_ENABLED"]; !ok || v != "1" {
				t.Errorf("RLINF_EMBODIED_RUNTIME_ENABLED = %q, want 1", v)
			}
		})
	}
}
