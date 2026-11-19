package ros2controller

import (
	"context"
	"fmt"
	"log"
	"strconv"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --------------------------------------------------------------------------
// StartRobot
// --------------------------------------------------------------------------

// StartRobot launches the ROS 2 launch process for the requested robot in
// the specified mode.
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

	c.stopRobot(rs)
	if err := c.launchMode(rs, displayName, modeCfg); err != nil {
		return nil, fmt.Errorf("start robot %s: %w", req.RobotId, err)
	}

	log.Printf("[ros2-controller] StartRobot: %s mode=%s package=%s launch=%s",
		req.RobotId, displayName, modeCfg.Package, modeCfg.LaunchFile)

	return &pb.StartRobotResponse{
		RobotId:     rs.Config.ID,
		State:       rs.State,
		CurrentMode: modeConfigToInfo(rs.Mode, modeCfg),
		RosDomainId: int32(rs.domainID),
		Params:      rs.Config.Params,
	}, nil
}

// --------------------------------------------------------------------------
// StopRobot
// --------------------------------------------------------------------------

// StopRobot stops the running launch process for the requested robot.
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
	log.Printf("[ros2-controller] StopRobot: %s stopped", req.RobotId)

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

	if rs.State == pb.RobotState_ROBOT_STATE_RUNNING {
		log.Printf("[ros2-controller] SwitchMode: %s stopping current mode %s", req.RobotId, rs.Mode)
		c.stopRobot(rs)
	}

	if err := c.launchMode(rs, displayName, modeCfg); err != nil {
		return nil, fmt.Errorf("switch mode for %s: %w", req.RobotId, err)
	}

	log.Printf("[ros2-controller] SwitchMode: %s → %s (package=%s launch=%s)",
		req.RobotId, displayName, modeCfg.Package, modeCfg.LaunchFile)

	return &pb.SwitchModeResponse{
		RobotId:     rs.Config.ID,
		Mode:        rs.Mode,
		State:       rs.State,
		CurrentMode: modeConfigToInfo(rs.Mode, rs.ActiveMode),
		RosDomainId: int32(rs.domainID),
		Params:      rs.Config.Params,
	}, nil
}

// --------------------------------------------------------------------------
// ResetRobot
// --------------------------------------------------------------------------

// ResetRobot stops the launch process and resets the robot to a clean
// stopped state.
func (c *Controller) ResetRobot(ctx context.Context, req *pb.ResetRobotRequest) (*pb.ResetRobotResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rs, _, err := c.lookupRobot(req.RobotId)
	if err != nil {
		return nil, err
	}

	// Stop the launch process if running. Unlike ROS 1, there is no
	// master (roscore) to restart — the DDS layer self-heals once a new
	// launch process starts.
	c.stopRobot(rs)

	// Reset state.
	rs.Mode = ""
	rs.ActiveMode = ModeConfig{}
	rs.State = pb.RobotState_ROBOT_STATE_STOPPED

	log.Printf("[ros2-controller] ResetRobot: %s reset complete (domain_id=%d)",
		req.RobotId, rs.domainID)

	return &pb.ResetRobotResponse{
		RobotId:     req.RobotId,
		State:       rs.State,
		RosDomainId: int32(rs.domainID),
	}, nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

func (c *Controller) buildLaunchConfig(modeCfg ModeConfig, rs *robotState) LaunchConfig {
	env := make([]string, 0, 4+len(modeCfg.Env))

	// ROS_DOMAIN_ID — the key isolation mechanism for ROS 2 DDS.
	env = append(env, "ROS_DOMAIN_ID="+strconv.Itoa(rs.domainID))

	// RMW implementation and DDS config can be passed via mode-level env.
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
// See the equivalent method in pkg/roscontroller for full semantics.
func (c *Controller) resolveModeConfig(typeCfg *RobotTypeConfig, modeName string, pbCfg *pb.ModeConfig) (ModeConfig, string, error) {
	if modeName != "" {
		cfg, ok := typeCfg.ModeConfig(modeName)
		if !ok {
			return ModeConfig{}, "", status.Errorf(codes.InvalidArgument,
				"unsupported mode %q for robot type %q (supported: %v)",
				modeName, typeCfg.Type, typeCfg.SupportedModes())
		}

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

	if c.allowedLaunchPackages != nil {
		if !c.allowedLaunchPackages["*"] && !c.allowedLaunchPackages[pbCfg.Package] {
			return ModeConfig{}, "", status.Errorf(codes.PermissionDenied,
				"custom mode: package %q is not in the allowed list", pbCfg.Package)
		}
	} else {
		return ModeConfig{}, "", status.Error(codes.PermissionDenied,
			"custom mode is disabled (no allowed launch packages configured)")
	}

	displayName := modeCfgDisplayName(pbCfg.Package, pbCfg.LaunchFile)
	return modeConfigFromProto(pbCfg), displayName, nil
}

func modeCfgDisplayName(pkg, launchFile string) string {
	return pkg + "/" + launchFile
}

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
// It launches ros2 launch and sets rs.Mode and rs.State on success.
//
// Caller must hold c.mu (write lock) and ensure the robot is in a clean
// state (i.e. stopRobot has been called if a process was running).
func (c *Controller) launchMode(rs *robotState, modeName string, modeCfg ModeConfig) error {
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

// stopRobot stops the launch process.
func (c *Controller) stopRobot(rs *robotState) {
	if rs.process.IsRunning() {
		if err := rs.process.Stop(); err != nil {
			log.Printf("[ros2-controller] stop process: %v", err)
		}
	}
}
