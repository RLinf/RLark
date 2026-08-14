package ros2controller

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMergeArgs verifies the three merge strategies for launch arguments.
func TestMergeArgs(t *testing.T) {
	robotParams := map[string]string{
		"robot_ip":   "172.16.0.2",
		"robot_name": "franka",
		"rate":       "200",
	}

	t.Run("passthrough_all", func(t *testing.T) {
		m := ModeConfig{
			PassthroughRobotArgs: true,
			Args:                 map[string]string{"rate": "100"},
		}
		args := m.MergeArgs(robotParams)
		mapped := argsToMap(args)
		if mapped["robot_ip"] != "172.16.0.2" {
			t.Errorf("robot_ip not passed through: %v", mapped)
		}
		if mapped["rate"] != "200" {
			t.Errorf("robot param should override default: got %v", mapped["rate"])
		}
	})

	t.Run("arg_from_mapping", func(t *testing.T) {
		m := ModeConfig{
			Args:    map[string]string{"rate": "100"},
			ArgFrom: map[string]string{"robot_uri": "robot_ip"},
		}
		args := m.MergeArgs(robotParams)
		mapped := argsToMap(args)
		if mapped["robot_uri"] != "172.16.0.2" {
			t.Errorf("arg_from mapping failed: %v", mapped)
		}
		if mapped["rate"] != "100" {
			t.Errorf("default should be kept: %v", mapped["rate"])
		}
		if _, exists := mapped["robot_name"]; exists {
			t.Errorf("robot_name should not be merged with arg_from: %v", mapped)
		}
	})

	t.Run("neither_set", func(t *testing.T) {
		m := ModeConfig{
			Args: map[string]string{"rate": "100"},
		}
		args := m.MergeArgs(robotParams)
		mapped := argsToMap(args)
		if len(mapped) != 1 || mapped["rate"] != "100" {
			t.Errorf("only defaults should be present: %v", mapped)
		}
	})

	t.Run("arg_from_wins_over_passthrough", func(t *testing.T) {
		m := ModeConfig{
			PassthroughRobotArgs: true,
			ArgFrom:              map[string]string{"robot_uri": "robot_ip"},
		}
		args := m.MergeArgs(robotParams)
		mapped := argsToMap(args)
		if _, exists := mapped["robot_name"]; exists {
			t.Errorf("passthrough should be ignored when arg_from is set: %v", mapped)
		}
		if mapped["robot_uri"] != "172.16.0.2" {
			t.Errorf("arg_from mapping should work: %v", mapped)
		}
	})
}

// TestSupportedModes verifies sorted output.
func TestSupportedModes(t *testing.T) {
	typeCfg := &RobotTypeConfig{
		Type: "franka",
		Modes: map[string]ModeConfig{
			"impedance": {},
			"joint":     {},
			"position":  {},
		},
	}
	modes := typeCfg.SupportedModes()
	want := []string{"impedance", "joint", "position"}
	if len(modes) != len(want) {
		t.Fatalf("got %v, want %v", modes, want)
	}
	for i, m := range modes {
		if m != want[i] {
			t.Errorf("modes[%d] = %q, want %q", i, m, want[i])
		}
	}
}

// TestLoadConfigFile verifies YAML parsing of the controller config.
func TestLoadConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `macvlans:
  - host_nic: eno1
    name: macvlan0
    ip: 172.16.0.100/24
types:
  - type: franka
    modes:
      impedance:
        package: moveit_servo
        launch_file: servo.launch.py
        passthrough_robot_args: true
robots:
  - id: franka-0
    type: franka
    params:
      robot_ip: 172.16.0.2
    web_service: https://172.16.0.2/
    domain_id: 5
allowed_launch_packages:
  - moveit_servo
http_addr: :8080
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfigFile(path)
	if err != nil {
		t.Fatalf("LoadConfigFile: %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Errorf("http_addr = %q, want :8080", cfg.HTTPAddr)
	}
	if len(cfg.MACVLANs) != 1 {
		t.Fatalf("macvlans = %d, want 1", len(cfg.MACVLANs))
	}
	if cfg.MACVLANs[0].HostNIC != "eno1" {
		t.Errorf("host_nic = %q", cfg.MACVLANs[0].HostNIC)
	}
	if len(cfg.Types) != 1 || cfg.Types[0].Type != "franka" {
		t.Errorf("types = %+v", cfg.Types)
	}
	if len(cfg.Robots) != 1 || cfg.Robots[0].ID != "franka-0" {
		t.Errorf("robots = %+v", cfg.Robots)
	}
	if cfg.Robots[0].DomainID != 5 {
		t.Errorf("domain_id = %d, want 5", cfg.Robots[0].DomainID)
	}
	if len(cfg.AllowedLaunchPackages) != 1 || cfg.AllowedLaunchPackages[0] != "moveit_servo" {
		t.Errorf("allowed_launch_packages = %v", cfg.AllowedLaunchPackages)
	}
}

// argsToMap converts a slice of "key:=value" strings into a map.
func argsToMap(args []string) map[string]string {
	m := make(map[string]string, len(args))
	for _, a := range args {
		// a is "key:=value"
		for i := 0; i < len(a); i++ {
			if i+2 <= len(a) && a[i:i+2] == ":=" {
				m[a[:i]] = a[i+2:]
				break
			}
		}
	}
	return m
}
