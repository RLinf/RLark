package roscontroller

import (
	"context"
	"fmt"
	"log"
	"time"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --------------------------------------------------------------------------
// StartRobot
// --------------------------------------------------------------------------

// StartRobot launches the roslaunch process for the requested robot in the
// specified mode.
func (c *Controller) StartRobot(ctx context.Context, req *pb.StartRobotRequest) (*pb.StartRobotResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rs, typeCfg, err := c.lookupRobot(req.RobotId)
	if err != nil {
		return nil, err
	}

	modeCfg, displayName, err := c.resolveModeConfig(typeCfg, req.Mode, req.ModeConfig)
	if err != nil {
		return nil, err
	}

	// Clean up any previous run, then launch the new mode.
	c.stopRobot(rs)
	if err := c.launchMode(rs, displayName, modeCfg); err != nil {
		return nil, fmt.Errorf("start robot %s: %w", req.RobotId, err)
	}

	log.Printf("[ros-controller] StartRobot: %s mode=%s package=%s launch=%s",
		req.RobotId, displayName, modeCfg.Package, modeCfg.LaunchFile)

	return &pb.StartRobotResponse{
		RobotId:      rs.Config.ID,
		State:        rs.State,
		CurrentMode:  modeConfigToInfo(rs.Mode, modeCfg),
		RosMasterUri: rosURI(rs),
		Params:       rs.Config.Params,
	}, nil
}

// --------------------------------------------------------------------------
// StopRobot
// --------------------------------------------------------------------------

// StopRobot stops the running roslaunch process for the requested robot.
func (c *Controller) StopRobot(ctx context.Context, req *pb.StopRobotRequest) (*pb.StopRobotResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rs, _, err := c.lookupRobot(req.RobotId)
	if err != nil {
		return nil, err
	}

	if rs.State != pb.RobotState_ROBOT_STATE_RUNNING {
		rs.State = pb.RobotState_ROBOT_STATE_STOPPED
		return &pb.StopRobotResponse{RobotId: req.RobotId}, nil
	}

	c.stopRobot(rs)
	rs.State = pb.RobotState_ROBOT_STATE_STOPPED
	log.Printf("[ros-controller] StopRobot: %s stopped", req.RobotId)

	return &pb.StopRobotResponse{RobotId: req.RobotId}, nil
}

// --------------------------------------------------------------------------
// SwitchMode
// --------------------------------------------------------------------------

// SwitchMode stops the current mode and launches the requested mode for the
// robot.
func (c *Controller) SwitchMode(ctx context.Context, req *pb.SwitchModeRequest) (*pb.SwitchModeResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rs, typeCfg, err := c.lookupRobot(req.RobotId)
	if err != nil {
		return nil, err
	}

	modeCfg, displayName, err := c.resolveModeConfig(typeCfg, req.Mode, req.ModeConfig)
	if err != nil {
		return nil, err
	}

	// Stop current mode, then launch the new one.
	if rs.State == pb.RobotState_ROBOT_STATE_RUNNING {
		log.Printf("[ros-controller] SwitchMode: %s stopping current mode %s", req.RobotId, rs.Mode)
		c.stopRobot(rs)
	}

	if err := c.launchMode(rs, displayName, modeCfg); err != nil {
		return nil, fmt.Errorf("switch mode for %s: %w", req.RobotId, err)
	}

	log.Printf("[ros-controller] SwitchMode: %s → %s (package=%s launch=%s)",
		req.RobotId, displayName, modeCfg.Package, modeCfg.LaunchFile)

	return &pb.SwitchModeResponse{
		RobotId:      rs.Config.ID,
		Mode:         rs.Mode,
		State:        rs.State,
		CurrentMode:  modeConfigToInfo(rs.Mode, rs.ActiveMode),
		RosMasterUri: rosURI(rs),
		Params:       rs.Config.Params,
	}, nil
}

// --------------------------------------------------------------------------
// ResetRobot
// --------------------------------------------------------------------------

// ResetRobot stops the roslaunch process and restarts roscore to return the
// robot to a clean state.
func (c *Controller) ResetRobot(ctx context.Context, req *pb.ResetRobotRequest) (*pb.ResetRobotResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rs, _, err := c.lookupRobot(req.RobotId)
	if err != nil {
		return nil, err
	}

	// Stop the roslaunch process if running.
	c.stopRobot(rs)

	// Stop and restart roscore to get a clean ROS master.
	if rs.roscore != nil {
		if rs.roscore.IsRunning() {
			if err := rs.roscore.Stop(); err != nil {
				log.Printf("[ros-controller] ResetRobot: %s stop roscore: %v", req.RobotId, err)
			}
		}
		if err := rs.roscore.Start(); err != nil {
			rs.State = pb.RobotState_ROBOT_STATE_ERROR
			return nil, fmt.Errorf("reset robot %s: restart roscore: %w", req.RobotId, err)
		}
		// Wait for the new roscore to be ready.
		if err := rs.roscore.WaitReady(5 * time.Second); err != nil {
			rs.State = pb.RobotState_ROBOT_STATE_ERROR
			return nil, fmt.Errorf("reset robot %s: roscore not ready: %w", req.RobotId, err)
		}
	}

	// Reset state.
	rs.Mode = ""
	rs.ActiveMode = ModeConfig{}
	rs.State = pb.RobotState_ROBOT_STATE_STOPPED

	log.Printf("[ros-controller] ResetRobot: %s reset complete", req.RobotId)

	return &pb.ResetRobotResponse{
		RobotId:      req.RobotId,
		State:        rs.State,
		RosMasterUri: rosURI(rs),
	}, nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

func (c *Controller) buildLaunchConfig(modeCfg ModeConfig, rs *robotState) LaunchConfig {
	env := make([]string, 0, 4+len(modeCfg.Env))

	// Use the per-robot roscore URI.
	if rs.roscore != nil {
		env = append(env, "ROS_MASTER_URI="+rs.roscore.URI())
	}
	if c.podIP != "" {
		env = append(env, "ROS_IP="+c.podIP)
	}
	for k, v := range modeCfg.Env {
		env = append(env, k+"="+v)
	}

	return LaunchConfig{
		Package:    modeCfg.Package,
		LaunchFile: modeCfg.LaunchFile,
		Args:       modeCfg.MergeArgs(rs.Config.Params),
		Env:        env,
	}
}

// resolveModeConfig returns the ModeConfig for the given mode name.
// It also generates a display name for the mode when modeName is empty
// (custom mode), using the format "pkg/launch_file".
//
//   - modeName != "":  preset lookup — mode must exist in the type config.
//     Only extra args and env from pbCfg are merged into the preset.
//
//   - modeName == "":  custom mode — pbCfg must be non-nil with non-empty
//     package and launch_file. The display name is auto-generated.
func (c *Controller) resolveModeConfig(typeCfg *RobotTypeConfig, modeName string, pbCfg *pb.ModeConfig) (ModeConfig, string, error) {
	if modeName != "" {
		// Preset mode: must be registered.
		cfg, ok := typeCfg.ModeConfig(modeName)
		if !ok {
			return ModeConfig{}, "", status.Errorf(codes.InvalidArgument,
				"unsupported mode %q for robot type %q (supported: %v)",
				modeName, typeCfg.Type, typeCfg.SupportedModes())
		}

		// Merge extra args and env from the request (if provided).
		// Other fields (package, launch_file, arg_from, passthrough) are
		// ignored when using a preset.
		if pbCfg != nil {
			if len(pbCfg.Args) > 0 {
				if cfg.Args == nil {
					cfg.Args = make(map[string]string, len(pbCfg.Args))
				}
				for k, v := range pbCfg.Args {
					cfg.Args[k] = v
				}
			}
			if len(pbCfg.Env) > 0 {
				if cfg.Env == nil {
					cfg.Env = make(map[string]string, len(pbCfg.Env))
				}
				for k, v := range pbCfg.Env {
					cfg.Env[k] = v
				}
			}
		}

		return cfg, modeName, nil
	}

	// Custom mode: no preset name given — must use pbCfg.
	if pbCfg == nil {
		return ModeConfig{}, "", status.Error(codes.InvalidArgument,
			"custom mode requires mode_config with package and launch_file")
	}
	if pbCfg.Package == "" {
		return ModeConfig{}, "", status.Error(codes.InvalidArgument,
			"custom mode: package is required")
	}
	if pbCfg.LaunchFile == "" {
		return ModeConfig{}, "", status.Error(codes.InvalidArgument,
			"custom mode: launch_file is required")
	}

	// Check launch package whitelist.
	// Empty whitelist = no packages allowed. "*" = allow all.
	if c.allowedLaunchPackages != nil {
		if !c.allowedLaunchPackages["*"] && !c.allowedLaunchPackages[pbCfg.Package] {
			return ModeConfig{}, "", status.Errorf(codes.PermissionDenied,
				"custom mode: package %q is not in the allowed list", pbCfg.Package)
		}
	} else {
		return ModeConfig{}, "", status.Error(codes.PermissionDenied,
			"custom mode is disabled (no allowed launch packages configured)")
	}

	// Generate a temporary mode name from package/launch_file.
	displayName := modeCfgDisplayName(pbCfg.Package, pbCfg.LaunchFile)
	return modeConfigFromProto(pbCfg), displayName, nil
}

// modeCfgDisplayName generates a human-readable mode name from a package
// and launch file, used as the display name for custom (ad-hoc) modes.
func modeCfgDisplayName(pkg, launchFile string) string {
	return pkg + "/" + launchFile
}

// modeConfigToInfo converts a ModeConfig + name to a protobuf ModeInfo.
func modeConfigToInfo(name string, cfg ModeConfig) *pb.ModeInfo {
	if cfg.Package == "" && cfg.LaunchFile == "" {
		return nil
	}
	return &pb.ModeInfo{
		Name:                 name,
		Package:              cfg.Package,
		LaunchFile:           cfg.LaunchFile,
		Args:                 cfg.Args,
		Env:                  cfg.Env,
		ArgFrom:              cfg.ArgFrom,
		PassthroughRobotArgs: cfg.PassthroughRobotArgs,
	}
}

// modeConfigFromProto converts a protobuf ModeConfig to the Go struct.
func modeConfigFromProto(pb *pb.ModeConfig) ModeConfig {
	return ModeConfig{
		Package:              pb.Package,
		LaunchFile:           pb.LaunchFile,
		Args:                 pb.Args,
		PassthroughRobotArgs: pb.PassthroughRobotArgs,
		ArgFrom:              pb.ArgFrom,
		Env:                  pb.Env,
	}
}

// launchMode is the common path shared by StartRobot and SwitchMode.
// It:
//  1. Starts the per-robot roscore if not already running
//  2. Launches roslaunch
//  3. Sets rs.Mode and rs.State on success
//
// Caller must hold c.mu (write lock) and ensure the robot is in a clean
// state (i.e. stopRobot has been called if a process was running).
func (c *Controller) launchMode(rs *robotState, modeName string, modeCfg ModeConfig) error {
	// roscore is started eagerly during RegisterRobot. Verify it's still
	// healthy via HTTP check — fail fast if it died unexpectedly.
	if rs.roscore != nil && !rs.roscore.Healthy() {
		rs.State = pb.RobotState_ROBOT_STATE_ERROR
		return fmt.Errorf("roscore is not responding (port %d)", rs.roscore.Port())
	}

	// Launch roslaunch (runs directly in the container).
	launchCfg := c.buildLaunchConfig(modeCfg, rs)
	if err := rs.process.Start(launchCfg); err != nil {
		rs.State = pb.RobotState_ROBOT_STATE_ERROR
		return err
	}

	rs.Mode = modeName
	rs.ActiveMode = modeCfg
	rs.State = pb.RobotState_ROBOT_STATE_RUNNING
	return nil
}

// stopRobot stops the process.
func (c *Controller) stopRobot(rs *robotState) {
	if rs.process.IsRunning() {
		if err := rs.process.Stop(); err != nil {
			log.Printf("[ros-controller] stop process: %v", err)
		}
	}
}
