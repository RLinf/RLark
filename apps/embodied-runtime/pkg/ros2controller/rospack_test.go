package ros2controller

import (
	"testing"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
)

// TestParseShowArgsOutput verifies parsing of "ros2 launch --show-args"
// output: argument names, descriptions, defaults, and required detection.
func TestParseShowArgsOutput(t *testing.T) {
	output := `Arguments (pass arguments as "<name>:=<value>"):

'robot_ip':
    Robot IP address.

'rate' (default: '100'):
    Control rate in Hz.

'verbose' (default: 'false'):
    Verbose logging.

'robot_name':
    Name of the robot.
`
	args := parseShowArgsOutput(output)

	byName := make(map[string]*pb.LaunchArg, len(args))
	for _, a := range args {
		byName[a.Name] = a
	}

	// robot_ip: no default → required.
	rip, ok := byName["robot_ip"]
	if !ok {
		t.Fatal("robot_ip arg missing")
	}
	if !rip.Required {
		t.Error("robot_ip should be required (no default)")
	}
	if rip.Default != "" {
		t.Errorf("robot_ip default = %q, want empty", rip.Default)
	}
	if rip.Description != "Robot IP address." {
		t.Errorf("robot_ip desc = %q", rip.Description)
	}

	// rate: has default '100' → not required.
	rate, ok := byName["rate"]
	if !ok {
		t.Fatal("rate arg missing")
	}
	if rate.Required {
		t.Error("rate should not be required (has default)")
	}
	if rate.Default != "100" {
		t.Errorf("rate default = %q, want 100", rate.Default)
	}

	// verbose: default 'false'.
	verb, ok := byName["verbose"]
	if !ok {
		t.Fatal("verbose arg missing")
	}
	if verb.Default != "false" {
		t.Errorf("verbose default = %q, want false", verb.Default)
	}

	// robot_name: required.
	rn, ok := byName["robot_name"]
	if !ok {
		t.Fatal("robot_name arg missing")
	}
	if !rn.Required {
		t.Error("robot_name should be required")
	}
}

// TestParseShowArgsOutput_Empty verifies that empty input returns no args.
func TestParseShowArgsOutput_Empty(t *testing.T) {
	if got := parseShowArgsOutput(""); len(got) != 0 {
		t.Errorf("empty input = %v, want none", got)
	}
}

// TestExtractDefault verifies the "(default: 'value')" extraction.
func TestExtractDefault(t *testing.T) {
	cases := []struct {
		desc     string
		wantDef  string
		wantDesc string
	}{
		{"Control rate in Hz. (default: '100')", "100", "Control rate in Hz."},
		{"Robot IP address.", "", "Robot IP address."},
		{"No default here", "", "No default here"},
	}
	for _, c := range cases {
		def, desc := extractDefault(c.desc)
		if def != c.wantDef {
			t.Errorf("extractDefault(%q) def = %q, want %q", c.desc, def, c.wantDef)
		}
		if desc != c.wantDesc {
			t.Errorf("extractDefault(%q) desc = %q, want %q", c.desc, desc, c.wantDesc)
		}
	}
}

// TestIsLaunchFile verifies the launch file extension check.
func TestIsLaunchFile(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"impedance.launch.py", true},
		{"joint.launch.xml", true},
		{"control.launch.yaml", true},
		{"legacy.launch", true},
		{"README.md", false},
		{"launch", false},
		{"script.py", false},
	}
	for _, c := range cases {
		if got := isLaunchFile(c.name); got != c.want {
			t.Errorf("isLaunchFile(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestIsPackageAllowed verifies the whitelist logic.
func TestIsPackageAllowed(t *testing.T) {
	t.Run("nil_whitelist", func(t *testing.T) {
		c := NewController("10.0.0.1", nil)
		if c.isPackageAllowed("any") {
			t.Error("nil whitelist should disallow all")
		}
	})
	t.Run("empty_whitelist", func(t *testing.T) {
		c := NewController("10.0.0.1", []string{})
		if c.isPackageAllowed("any") {
			t.Error("empty whitelist should disallow all")
		}
	})
	t.Run("star_allows_all", func(t *testing.T) {
		c := NewController("10.0.0.1", []string{"*"})
		if !c.isPackageAllowed("any_pkg") {
			t.Error("\"*\" should allow all packages")
		}
	})
	t.Run("exact_match", func(t *testing.T) {
		c := NewController("10.0.0.1", []string{"pkg_a", "pkg_b"})
		if !c.isPackageAllowed("pkg_a") {
			t.Error("pkg_a should be allowed")
		}
		if c.isPackageAllowed("pkg_c") {
			t.Error("pkg_c should not be allowed")
		}
	})
}

// TestModeCfgDisplayName verifies the "pkg/launch_file" display name.
func TestModeCfgDisplayName(t *testing.T) {
	got := modeCfgDisplayName("my_pkg", "custom.launch.py")
	want := "my_pkg/custom.launch.py"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// TestModeConfigToInfo verifies the protobuf conversion, including the nil
// case for empty configs.
func TestModeConfigToInfo(t *testing.T) {
	t.Run("empty_returns_nil", func(t *testing.T) {
		if info := modeConfigToInfo("x", ModeConfig{}); info != nil {
			t.Errorf("empty config should return nil, got %+v", info)
		}
	})
	t.Run("populated", func(t *testing.T) {
		mc := ModeConfig{
			Package:    "pkg",
			LaunchFile: "f.launch.py",
			Args:       map[string]string{"a": "1"},
			Env:        map[string]string{"E": "v"},
			ArgFrom:    map[string]string{"arg": "param"},
		}
		info := modeConfigToInfo("joint", mc)
		if info == nil {
			t.Fatal("expected non-nil")
		}
		if info.Name != "joint" || info.Package != "pkg" || info.LaunchFile != "f.launch.py" {
			t.Errorf("info = %+v", info)
		}
		if info.Args["a"] != "1" || info.Env["E"] != "v" || info.ArgFrom["arg"] != "param" {
			t.Errorf("maps not copied: %+v", info)
		}
	})
}

// TestModeConfigFromProto verifies the proto-to-struct conversion preserves all
// fields.
func TestModeConfigFromProto(t *testing.T) {
	pbCfg := &pb.ModeConfig{
		Package:              "pkg",
		LaunchFile:           "f.launch.py",
		Args:                 map[string]string{"a": "1"},
		PassthroughRobotArgs: true,
		ArgFrom:              map[string]string{"arg": "param"},
		Env:                  map[string]string{"E": "v"},
	}
	mc := modeConfigFromProto(pbCfg)
	if mc.Package != "pkg" || mc.LaunchFile != "f.launch.py" {
		t.Errorf("pkg/launch: %+v", mc)
	}
	if !mc.PassthroughRobotArgs || mc.Args["a"] != "1" || mc.ArgFrom["arg"] != "param" || mc.Env["E"] != "v" {
		t.Errorf("fields not copied: %+v", mc)
	}
}
