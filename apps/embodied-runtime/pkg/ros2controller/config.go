package ros2controller

import (
	"fmt"
	"os"
	"sort"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/netmac"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Type-level configuration
// ---------------------------------------------------------------------------

// RobotTypeConfig defines the configuration for a robot type (e.g. "franka", "ur5").
type RobotTypeConfig struct {
	Type  string                `yaml:"type"`
	Modes map[string]ModeConfig `yaml:"modes"`
}

// ModeConfig defines how to launch a ros2 launch process for a specific mode.
type ModeConfig struct {
	// Package is the ROS 2 package containing the launch file.
	Package string `yaml:"package"`

	// LaunchFile is the name of the launch file (e.g. "impedance.launch.py",
	// "joint.launch.xml", "control.launch.yaml").
	LaunchFile string `yaml:"launch_file"`

	// Args are default key=value arguments passed to ros2 launch.
	// Robot-level params override these when PassthroughRobotArgs is true,
	// or via ArgFrom mapping.
	Args map[string]string `yaml:"args,omitempty"`

	// PassthroughRobotArgs, when true, merges all robot params into
	// launch args with identity mapping (param name = arg name).
	// Mutually exclusive with ArgFrom.
	PassthroughRobotArgs bool `yaml:"passthrough_robot_args,omitempty"`

	// ArgFrom maps launch argument names → robot param names.
	// Only the params listed here are merged from robot params, using
	// the mapped key as the arg name.
	//
	// Mutually exclusive with PassthroughRobotArgs. When neither is set,
	// no robot params are merged.
	ArgFrom map[string]string `yaml:"arg_from,omitempty"`

	// Env are additional environment variables for the launch process,
	// merged with server-level ROS env (e.g. RMW_IMPLEMENTATION,
	// CYCLONEDDS_URI).
	Env map[string]string `yaml:"env,omitempty"`
}

// MACVLANConfig specifies how to create a macvlan interface for the container.
// Re-exported from the shared netmac package so the YAML config structure
// stays self-contained for config-file consumers.
type MACVLANConfig = netmac.MACVLANConfig

// ---------------------------------------------------------------------------
// Instance-level configuration
// ---------------------------------------------------------------------------

// RobotConfig defines the configuration for a specific robot instance.
type RobotConfig struct {
	// ID is the unique identifier for this robot (e.g. "franka-0").
	ID string `yaml:"id"`

	// Type references a registered RobotTypeConfig.Type (e.g. "franka").
	Type string `yaml:"type"`

	// Params are robot-specific parameters that override the type defaults.
	Params map[string]string `yaml:"params,omitempty"`

	// WebService is the base URL of the robot's web UI or API.
	// When set, ros2-controller reverse-proxies /v1/robots/<id>/proxy/* →
	// this URL. Example: "https://172.16.0.2/"
	WebService string `yaml:"web_service,omitempty"`

	// DomainID optionally overrides the auto-assigned ROS_DOMAIN_ID for
	// this robot. When 0 (default), the controller auto-assigns the next
	// available domain ID. Use this when you need a fixed domain ID (e.g.
	// to match a robot's pre-configured DDS domain).
	DomainID int `yaml:"domain_id,omitempty"`
}

// ---------------------------------------------------------------------------
// Config file
// ---------------------------------------------------------------------------

// ControllerConfig is the top-level structure read from a YAML config file.
type ControllerConfig struct {
	MACVLANs              []MACVLANConfig    `yaml:"macvlans"`
	Types                 []*RobotTypeConfig `yaml:"types"`
	Robots                []*RobotConfig     `yaml:"robots"`
	AllowedLaunchPackages []string           `yaml:"allowed_launch_packages,omitempty"`
	// HTTPAddr is the optional TCP address ("host:port" or ":port") for the
	// HTTP/JSON gateway (REST + per-robot proxy). Empty disables HTTP.
	HTTPAddr string `yaml:"http_addr,omitempty"`
}

// LoadConfigFile reads and parses a YAML configuration file.
func LoadConfigFile(path string) (*ControllerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	var cfg ControllerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}

	return &cfg, nil
}

// --------------------------------------------------------------------------
// Methods
// --------------------------------------------------------------------------

// SupportedModes returns the list of mode names supported by this robot type,
// sorted for stable, reproducible output (map iteration is non-deterministic).
func (t *RobotTypeConfig) SupportedModes() []string {
	modes := make([]string, 0, len(t.Modes))
	for m := range t.Modes {
		modes = append(modes, m)
	}
	sort.Strings(modes)
	return modes
}

// ModeConfig returns the ModeConfig for a given mode, and whether it exists.
func (t *RobotTypeConfig) ModeConfig(mode string) (ModeConfig, bool) {
	cfg, ok := t.Modes[mode]
	return cfg, ok
}

// MergeArgs merges the mode-level default args with robot-level params.
// Robot-level params take precedence over mode defaults.
//
// Merge strategy:
//   - PassthroughRobotArgs: identity merge all robot params (legacy behavior).
//   - ArgFrom has entries:     only merge the params listed in the mapping,
//     using the mapped key as the launch arg name.
//   - Neither set:             no robot params are merged.
//
// PassthroughRobotArgs and ArgFrom are mutually exclusive (ArgFrom wins
// if both are set).
func (m ModeConfig) MergeArgs(robotParams map[string]string) []string {
	merged := make(map[string]string, len(m.Args))
	for k, v := range m.Args {
		merged[k] = v
	}

	switch {
	case len(m.ArgFrom) > 0:
		for argName, paramName := range m.ArgFrom {
			if v, ok := robotParams[paramName]; ok {
				merged[argName] = v
			}
		}

	case m.PassthroughRobotArgs:
		for k, v := range robotParams {
			merged[k] = v
		}

	default:
	}

	args := make([]string, 0, len(merged))
	for k, v := range merged {
		args = append(args, k+":="+v)
	}
	return args
}
