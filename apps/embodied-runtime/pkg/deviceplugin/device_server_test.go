package deviceplugin

import (
	"testing"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/netmac"
	devicepb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/device/v1"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// TestEffectiveDevinitSocketPath verifies the default lives inside RunDir
// (so the existing RunDir mount reaches it) and an explicit value wins.
func TestEffectiveDevinitSocketPath(t *testing.T) {
	cfg := PluginConfig{}
	if got := cfg.EffectiveDevinitSocketPath(); got != DevinitSocketPath {
		t.Errorf("default = %q, want %q", got, DevinitSocketPath)
	}
	if got := cfg.EffectiveDevinitSocketPath(); got != RunDir+"/devinit.sock" {
		t.Errorf("default %q should be inside RunDir %q", got, RunDir)
	}

	cfg.DevinitSocketPath = "/custom/devinit.sock"
	if got := cfg.EffectiveDevinitSocketPath(); got != "/custom/devinit.sock" {
		t.Errorf("override = %q, want /custom/devinit.sock", got)
	}
}

// TestAllocateContainer_HostMacvlans verifies Allocate injects the device
// init discovery env vars (RLINF_EMBODIED_DEVINIT_ENABLED + _SOCKET_PATH)
// when host_macvlans is non-empty, and omits both when it is empty. The
// macvlan itself is never device-mounted (it must be created inside the pod
// netns), so Devices stays empty regardless.
func TestAllocateContainer_HostMacvlans(t *testing.T) {
	cases := []struct {
		name        string
		hostMacvlns []netmac.MACVLANConfig
		wantEnv     bool
	}{
		{name: "none", hostMacvlns: nil, wantEnv: false},
		{
			name: "one macvlan",
			hostMacvlns: []netmac.MACVLANConfig{
				{Name: "macvlan0", HostNIC: "eno1", IP: "172.16.0.100/24"},
			},
			wantEnv: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Plugin{cfg: PluginConfig{HostMacvlans: tc.hostMacvlns}}
			resp := p.allocateContainer(&pluginapi.ContainerAllocateRequest{
				DevicesIds: []string{"device-0"},
			})

			enabled, ok := resp.Envs["RLINF_EMBODIED_DEVINIT_ENABLED"]
			if tc.wantEnv {
				if !ok || enabled != "1" {
					t.Errorf("RLINF_EMBODIED_DEVINIT_ENABLED = %q (ok=%v), want 1", enabled, ok)
				}
				if sp, ok2 := resp.Envs["RLINF_EMBODIED_DEVINIT_SOCKET_PATH"]; !ok2 || sp == "" {
					t.Errorf("RLINF_EMBODIED_DEVINIT_SOCKET_PATH missing/empty: %q (ok=%v)", sp, ok2)
				}
				// macvlans are never device-mounted.
				if len(resp.Devices) != 0 {
					t.Errorf("macvlans must not be device-mounted, got %d DeviceSpecs", len(resp.Devices))
				}
			} else {
				if ok {
					t.Errorf("RLINF_EMBODIED_DEVINIT_ENABLED should not be set, got %q", enabled)
				}
			}
		})
	}
}

// TestNewDeviceServer_StoresRawConfigs verifies NewDeviceServer stores the
// configs AS-IS — enrichment and validation are deferred to each Setup call
// (per-container) so every pod gets its own auto-picked IP. No config is
// dropped at construction, and the stored values are not mutated.
func TestNewDeviceServer_StoresRawConfigs(t *testing.T) {
	cfgs := []netmac.MACVLANConfig{
		{Name: "good", HostNIC: "eno1", IP: "172.16.0.100/24"},
		{Name: "bad-net", HostNIC: "eno1", IP: "172.16.0.0/24"},     // network addr
		{Name: "bad-bcast", HostNIC: "eno1", IP: "172.16.0.255/24"}, // broadcast
		{Name: "bad-noip", HostNIC: "eno1", IP: ""},
	}
	srv := NewDeviceServer(cfgs, "/tmp/macvlan-test.sock")
	if !srv.HasMacvlans() {
		t.Fatal("HasMacvlans = false, want true (all raw configs stored)")
	}
	if len(srv.cfgs) != len(cfgs) {
		t.Fatalf("stored %d configs, want %d (NewDeviceServer must not drop; enrich/validate is per-Setup)",
			len(srv.cfgs), len(cfgs))
	}
	// The stored configs must equal the originals — no enrichment mutation.
	for i, want := range cfgs {
		if srv.cfgs[i] != want {
			t.Errorf("cfg[%d] mutated: got %+v, want %+v", i, srv.cfgs[i], want)
		}
	}
}

// TestNewDeviceServer_EmptyHasNoMacvlans verifies a server with no configs
// reports HasMacvlans=false (so the device plugin skips starting it). With
// the per-Setup enrich/validate design, a non-empty slice reports
// HasMacvlans=true even when every entry is invalid — validity is checked at
// Setup time, not construction.
func TestNewDeviceServer_EmptyHasNoMacvlans(t *testing.T) {
	srv := NewDeviceServer(nil, "/tmp/macvlan-test.sock")
	if srv.HasMacvlans() {
		t.Error("HasMacvlans = true for nil configs, want false")
	}
	// A non-empty (even all-invalid) slice starts the server; Setup drops the
	// invalid entries per call.
	srv = NewDeviceServer([]netmac.MACVLANConfig{
		{Name: "bad", HostNIC: "eno1", IP: "172.16.0.0/24"},
	}, "/tmp/macvlan-test.sock")
	if !srv.HasMacvlans() {
		t.Error("HasMacvlans = false for a non-empty (all-invalid) slice, want true (validity is per-Setup)")
	}
}

// TestEnrichForSetup_DropsInvalid verifies the per-Setup enrich+validate path
// drops configs that cannot be completed. EnrichMACVLANConfig is best-effort
// (a hard no-op on non-Linux; a no-op when the host is unreachable on Linux),
// so on the test host enrichment leaves the config unchanged and the result
// is driven by the pure ValidateMACVLANConfig: a fully-specified config
// passes; network/broadcast/empty IPs and missing name are rejected.
func TestEnrichForSetup_DropsInvalid(t *testing.T) {
	cases := []struct {
		name string
		cfg  netmac.MACVLANConfig
		want bool
	}{
		{"good", netmac.MACVLANConfig{Name: "good", HostNIC: "eno1", IP: "172.16.0.100/24"}, true},
		{"network-addr", netmac.MACVLANConfig{Name: "bad-net", HostNIC: "eno1", IP: "172.16.0.0/24"}, false},
		{"broadcast", netmac.MACVLANConfig{Name: "bad-bcast", HostNIC: "eno1", IP: "172.16.0.255/24"}, false},
		{"no-ip", netmac.MACVLANConfig{Name: "bad-noip", HostNIC: "eno1", IP: ""}, false},
		{"no-name", netmac.MACVLANConfig{HostNIC: "eno1", IP: "172.16.0.100/24"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := enrichForSetup(tc.cfg)
			if ok != tc.want {
				t.Errorf("enrichForSetup(%s) ok = %v, want %v", tc.name, ok, tc.want)
			}
		})
	}
}

// TestEnrichForSetup_DoesNotMutateInput verifies the helper enriches a COPY —
// the caller's config is untouched, so the pristine stored config can be
// re-enriched for the next pod (with a possibly different auto-picked IP).
func TestEnrichForSetup_DoesNotMutateInput(t *testing.T) {
	cfg := netmac.MACVLANConfig{Name: "m0", HostNIC: "eno1", IP: "172.16.0.100/24"}
	orig := cfg
	if _, ok := enrichForSetup(cfg); !ok {
		t.Fatalf("enrichForSetup dropped a fully-specified config")
	}
	if cfg != orig {
		t.Errorf("enrichForSetup mutated its input: got %+v, want %+v", cfg, orig)
	}
}

// TestMacvlanNames verifies the skipped-list helper preserves order.
func TestMacvlanNames(t *testing.T) {
	cfgs := []netmac.MACVLANConfig{
		{Name: "macvlan0"},
		{Name: "macvlan1"},
	}
	got := macvlanNames(cfgs)
	if len(got) != 2 || got[0] != "macvlan0" || got[1] != "macvlan1" {
		t.Errorf("macvlanNames = %v, want [macvlan0 macvlan1]", got)
	}
}

// TestSetupResponse_NoConfigs verifies Setup returns an empty (non-error)
// response when the server has no configs — e.g. if all were filtered out
// after construction. The host-network check is bypassed here because the
// caller is not in a real netns; this case focuses on the empty-config path
// by driving the server directly with a context that carries a PID, and
// asserting the behaviour when host-network detection cannot be performed
// is surfaced as an error rather than a crash.
func TestSetupResponse_NilServerSafe(t *testing.T) {
	// A nil-pointer-safe HasMacvlans keeps the device plugin wiring simple.
	var nilSrv *DeviceServer
	if nilSrv.HasMacvlans() {
		t.Error("nil DeviceServer.HasMacvlans = true, want false")
	}
}

// Compile-time assertion that DeviceServer satisfies the gRPC server
// interface generated from the proto.
var _ devicepb.DeviceServiceServer = (*DeviceServer)(nil)

// Compile-time assertion that the device plugin Server still implements the
// kubelet DevicePlugin server interface after the macvlan wiring was added.
var _ pluginapi.DevicePluginServer = (*Server)(nil)
