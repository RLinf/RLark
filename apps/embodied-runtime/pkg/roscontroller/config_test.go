package roscontroller

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestSupportedModes verifies that SupportedModes returns every mode name in
// the type config's map, in sorted order (stable, reproducible output).
func TestSupportedModes(t *testing.T) {
	tc := &RobotTypeConfig{
		Type: "franka",
		Modes: map[string]ModeConfig{
			"joint":     {Package: "p", LaunchFile: "j.launch"},
			"impedance": {Package: "p", LaunchFile: "i.launch"},
			"cartesian": {Package: "p", LaunchFile: "c.launch"},
		},
	}
	modes := tc.SupportedModes()
	want := []string{"cartesian", "impedance", "joint"}
	if !reflect.DeepEqual(modes, want) {
		t.Errorf("SupportedModes = %v, want %v (sorted)", modes, want)
	}

	empty := (&RobotTypeConfig{Type: "x"}).SupportedModes()
	if len(empty) != 0 {
		t.Errorf("empty type should have no modes, got %v", empty)
	}
}

// TestModeConfig_Lookup verifies that ModeConfig returns the config for an
// existing mode and ok=false for a missing one.
func TestModeConfig_Lookup(t *testing.T) {
	tc := &RobotTypeConfig{
		Type: "franka",
		Modes: map[string]ModeConfig{
			"joint": {Package: "pkg", LaunchFile: "j.launch", Args: map[string]string{"k": "v"}},
		},
	}
	cfg, ok := tc.ModeConfig("joint")
	if !ok || cfg.Package != "pkg" || cfg.LaunchFile != "j.launch" {
		t.Errorf("ModeConfig(joint) = %+v ok=%v", cfg, ok)
	}
	if _, ok := tc.ModeConfig("nonexistent"); ok {
		t.Error("ModeConfig should return ok=false for missing mode")
	}
}

// TestMergeArgs verifies the three merge strategies (ArgFrom, Passthrough,
// neither) and the base-args precedence.
func TestMergeArgs(t *testing.T) {
	t.Run("neither_set_no_merge", func(t *testing.T) {
		m := ModeConfig{
			Package:    "p",
			LaunchFile: "f.launch",
			Args:       map[string]string{"base": "1"},
		}
		robotParams := map[string]string{"ip": "10.0.0.1"}
		got := m.MergeArgs(robotParams)
		if len(got) != 1 || got[0] != "base:=1" {
			t.Errorf("MergeArgs = %v, want [base:=1]", got)
		}
	})
	t.Run("neither_set_empty_robot_params", func(t *testing.T) {
		m := ModeConfig{Args: map[string]string{"a": "1"}}
		got := m.MergeArgs(nil)
		if len(got) != 1 {
			t.Errorf("MergeArgs(nil) = %v, want 1 arg", got)
		}
	})
	t.Run("passthrough_identity_merge", func(t *testing.T) {
		m := ModeConfig{
			PassthroughRobotArgs: true,
			Args:                 map[string]string{"base": "1"},
		}
		robotParams := map[string]string{"ip": "10.0.0.1", "port": "11311"}
		got := m.MergeArgs(robotParams)
		set := sliceToSet(got)
		if !set["base:=1"] || !set["ip:=10.0.0.1"] || !set["port:=11311"] {
			t.Errorf("MergeArgs(passthrough) = %v, missing expected args", got)
		}
	})
	t.Run("passthrough_robot_overrides_base", func(t *testing.T) {
		m := ModeConfig{
			PassthroughRobotArgs: true,
			Args:                 map[string]string{"ip": "default"},
		}
		robotParams := map[string]string{"ip": "10.0.0.1"}
		got := m.MergeArgs(robotParams)
		if len(got) != 1 || got[0] != "ip:=10.0.0.1" {
			t.Errorf("robot param should override base: %v", got)
		}
	})
	t.Run("arg_from_explicit_mapping", func(t *testing.T) {
		m := ModeConfig{
			Args: map[string]string{"base": "1"},
			ArgFrom: map[string]string{
				"robot_uri": "robot_ip",
			},
		}
		robotParams := map[string]string{
			"robot_ip": "192.168.1.1",
			"unused":   "x",
		}
		got := m.MergeArgs(robotParams)
		set := sliceToSet(got)
		if !set["base:=1"] {
			t.Errorf("base arg lost: %v", got)
		}
		if !set["robot_uri:=192.168.1.1"] {
			t.Errorf("mapped arg lost: %v", got)
		}
		if set["unused:=x"] {
			t.Errorf("unmapped robot param should not appear: %v", got)
		}
	})
	t.Run("arg_from_missing_param_skipped", func(t *testing.T) {
		m := ModeConfig{
			Args:    map[string]string{"base": "1"},
			ArgFrom: map[string]string{"robot_uri": "robot_ip"},
		}
		got := m.MergeArgs(map[string]string{})
		if len(got) != 1 || got[0] != "base:=1" {
			t.Errorf("missing mapped param should leave base: %v", got)
		}
	})
	t.Run("arg_from_takes_precedence_over_passthrough", func(t *testing.T) {
		m := ModeConfig{
			PassthroughRobotArgs: true,
			Args:                 map[string]string{"base": "1"},
			ArgFrom:              map[string]string{"robot_uri": "robot_ip"},
		}
		robotParams := map[string]string{"robot_ip": "10.0.0.1", "extra": "9"}
		got := m.MergeArgs(robotParams)
		set := sliceToSet(got)
		if !set["robot_uri:=10.0.0.1"] {
			t.Errorf("ArgFrom mapping lost: %v", got)
		}
		if set["extra:=9"] {
			t.Errorf("Passthrough should be ignored when ArgFrom is set: %v", got)
		}
	})
	t.Run("empty_args_no_robot_params", func(t *testing.T) {
		got := (ModeConfig{}).MergeArgs(nil)
		if len(got) != 0 {
			t.Errorf("empty MergeArgs = %v, want []", got)
		}
	})
}

// TestLoadConfigFile verifies YAML parsing of a full controller config file,
// covering types, robots, macvlans, and the allowed-packages whitelist.
func TestLoadConfigFile(t *testing.T) {
	content := `
macvlans:
  - host_nic: eth0
    name: macvlan0
    ip: 172.16.0.100/24
    gateway: 172.16.0.1
types:
  - type: franka
    modes:
      joint:
        package: franka_pkg
        launch_file: joint.launch
        args:
          rate: "100"
        arg_from:
          robot_uri: robot_ip
      impedance:
        package: franka_pkg
        launch_file: impedance.launch
        passthrough_robot_args: true
        env:
          ROS_LOG_DIR: /tmp/ros
robots:
  - id: franka-0
    type: franka
    params:
      robot_ip: "192.168.1.1"
    web_service: "https://172.16.0.2/"
allowed_launch_packages:
  - franka_pkg
  - custom_pkg
http_addr: :8080
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}

	if len(cfg.MACVLANs) != 1 {
		t.Fatalf("MACVLANs = %d, want 1", len(cfg.MACVLANs))
	}
	if cfg.MACVLANs[0].HostNIC != "eth0" || cfg.MACVLANs[0].Name != "macvlan0" ||
		cfg.MACVLANs[0].IP != "172.16.0.100/24" || cfg.MACVLANs[0].Gateway != "172.16.0.1" {
		t.Errorf("MACVLAN mismatch: %+v", cfg.MACVLANs[0])
	}

	if len(cfg.Types) != 1 || cfg.Types[0].Type != "franka" {
		t.Fatalf("Types = %+v", cfg.Types)
	}
	joint, ok := cfg.Types[0].ModeConfig("joint")
	if !ok || joint.Package != "franka_pkg" || joint.LaunchFile != "joint.launch" {
		t.Errorf("joint mode: %+v ok=%v", joint, ok)
	}
	if joint.Args["rate"] != "100" || joint.ArgFrom["robot_uri"] != "robot_ip" {
		t.Errorf("joint args/arg_from: %+v", joint)
	}
	imp, ok := cfg.Types[0].ModeConfig("impedance")
	if !ok || !imp.PassthroughRobotArgs || imp.Env["ROS_LOG_DIR"] != "/tmp/ros" {
		t.Errorf("impedance mode: %+v ok=%v", imp, ok)
	}

	if len(cfg.Robots) != 1 || cfg.Robots[0].ID != "franka-0" || cfg.Robots[0].Type != "franka" {
		t.Fatalf("Robots = %+v", cfg.Robots)
	}
	if cfg.Robots[0].Params["robot_ip"] != "192.168.1.1" ||
		cfg.Robots[0].WebService != "https://172.16.0.2/" {
		t.Errorf("Robot params/ws mismatch: %+v", cfg.Robots[0])
	}

	if len(cfg.AllowedLaunchPackages) != 2 {
		t.Errorf("AllowedLaunchPackages = %v", cfg.AllowedLaunchPackages)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
}

// TestLoadConfigFile_MissingFile verifies that a non-existent path returns
// an error.
func TestLoadConfigFile_MissingFile(t *testing.T) {
	_, err := LoadConfigFile(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestLoadConfigFile_InvalidYAML verifies that malformed YAML returns an
// error.
func TestLoadConfigFile_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte(":\n  - [invalid: yaml: ]"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfigFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// sliceToSet converts a slice of "k:=v" strings into a set for order-
// independent comparison.
func sliceToSet(args []string) map[string]bool {
	m := make(map[string]bool, len(args))
	for _, a := range args {
		m[a] = true
	}
	return m
}
