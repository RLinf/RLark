package main

import (
	"fmt"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

// modeFlags carries all the optional mode configuration flags shared by
// start and switch commands.
type modeFlags struct {
	Package     string
	LaunchFile  string
	Args        []string
	ArgFrom     []string
	Passthrough bool
	Env         []string
}

// resolveModeConfig checks the mode name and flags, returning a ModeConfig
// suitable for the request. The logic mirrors the server's resolveModeConfig:
//
//   - mode != "" (preset):  only --arg and --env are accepted; --package,
//     --launch-file, --arg-from, --passthrough-robot-args are rejected.
//   - mode == "" (custom):  --package + --launch-file are required; all flags
//     are allowed.
//
// Returns nil ModeConfig when no override flags are provided (preset with no
// extras). The caller should not set ModeConfig on the request when nil.
func resolveModeConfig(cmd *cobra.Command, mode string, flags *modeFlags) (*pb.ModeConfig, error) {
	if mode != "" {
		return resolvePresetModeConfig(cmd, flags)
	}
	return resolveCustomModeConfig(flags)
}

func resolvePresetModeConfig(cmd *cobra.Command, flags *modeFlags) (*pb.ModeConfig, error) {
	// Preset mode: only --arg and --env are accepted.
	if cmd.Flags().Changed("package") || cmd.Flags().Changed("launch-file") {
		return nil, fmt.Errorf("--package and --launch-file are only valid without a mode name (custom mode)")
	}
	if cmd.Flags().Changed("arg-from") {
		return nil, fmt.Errorf("--arg-from is only valid for custom mode (no mode name)")
	}
	if cmd.Flags().Changed("passthrough-robot-args") {
		return nil, fmt.Errorf("--passthrough-robot-args is only valid for custom mode (no mode name)")
	}

	if len(flags.Args) == 0 && len(flags.Env) == 0 {
		return nil, nil
	}

	return buildModeOverride(flags.Args, flags.Env)
}

func resolveCustomModeConfig(flags *modeFlags) (*pb.ModeConfig, error) {
	// Custom mode: require --package + --launch-file.
	if flags.Package == "" {
		return nil, fmt.Errorf("--package is required for custom mode (no mode name given)")
	}
	if flags.LaunchFile == "" {
		return nil, fmt.Errorf("--launch-file is required for custom mode")
	}

	return buildModeConfig(flags.Package, flags.LaunchFile, flags.Args, flags.ArgFrom, flags.Passthrough, flags.Env)
}

// addModeFlags registers the shared mode configuration flags on a command.
func addModeFlags(cmd *cobra.Command, flags *modeFlags) {
	cmd.Flags().StringVar(&flags.Package, "package", "", "ROS package (custom mode only)")
	cmd.Flags().StringVar(&flags.LaunchFile, "launch-file", "", "Launch file (custom mode only)")
	cmd.Flags().StringArrayVar(&flags.Args, "arg", nil, "roslaunch args (key=val, repeatable)")
	cmd.Flags().StringArrayVar(&flags.ArgFrom, "arg-from", nil, "arg→param mapping (arg_name=param_name, repeatable; custom mode only)")
	cmd.Flags().BoolVar(&flags.Passthrough, "passthrough-robot-args", false, "Passthrough all robot params (identity merge; custom mode only)")
	cmd.Flags().StringArrayVar(&flags.Env, "env", nil, "Extra env vars (key=val, repeatable)")
}

// buildModeOverride builds a ModeConfig with only args and env overrides
// for a preset mode. The server merges these into the preset.
func buildModeOverride(args, env []string) (*pb.ModeConfig, error) {
	cfg := &pb.ModeConfig{
		Args: make(map[string]string),
		Env:  make(map[string]string),
	}

	for _, a := range args {
		k, v, ok := stringsCut(a, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --arg format %q, expected key=val", a)
		}
		cfg.Args[k] = v
	}

	for _, e := range env {
		k, v, ok := stringsCut(e, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --env format %q, expected key=val", e)
		}
		cfg.Env[k] = v
	}

	return cfg, nil
}

// buildModeConfig parses CLI flags into a protobuf ModeConfig for custom mode.
func buildModeConfig(pkg, launch string, args, argFrom []string, passthrough bool, env []string) (*pb.ModeConfig, error) {
	cfg := &pb.ModeConfig{
		Package:              pkg,
		LaunchFile:           launch,
		Args:                 make(map[string]string),
		PassthroughRobotArgs: passthrough,
		Env:                  make(map[string]string),
	}

	for _, a := range args {
		k, v, ok := stringsCut(a, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --arg format %q, expected key=val", a)
		}
		cfg.Args[k] = v
	}

	if len(argFrom) > 0 {
		cfg.ArgFrom = make(map[string]string, len(argFrom))
		for _, m := range argFrom {
			argName, paramName, ok := stringsCut(m, "=")
			if !ok {
				return nil, fmt.Errorf("invalid --arg-from format %q, expected arg_name=param_name", m)
			}
			cfg.ArgFrom[argName] = paramName
		}
	}

	for _, e := range env {
		k, v, ok := stringsCut(e, "=")
		if !ok {
			return nil, fmt.Errorf("invalid --env format %q, expected key=val", e)
		}
		cfg.Env[k] = v
	}

	return cfg, nil
}

// stringsCut is like strings.Cut (go1.18+).
func stringsCut(s, sep string) (before, after string, found bool) {
	n := len(sep)
	if n == 0 {
		return "", s, false
	}
	for i := 0; i <= len(s)-n; i++ {
		if s[i:i+n] == sep {
			return s[:i], s[i+n:], true
		}
	}
	return s, "", false
}
