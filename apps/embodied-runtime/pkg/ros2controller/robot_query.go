package ros2controller

import (
	"context"
	"log"
	"sort"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
)

// --------------------------------------------------------------------------
// GetRobotStatus
// --------------------------------------------------------------------------

// GetRobotStatus returns the current state and mode of the requested robot.
func (c *Controller) GetRobotStatus(ctx context.Context, req *pb.GetRobotStatusRequest) (*pb.GetRobotStatusResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rs, _, err := c.lookupRobot(req.RobotId)
	if err != nil {
		return nil, err
	}

	// Check launch process health. Unlike ROS 1, there is no roscore HTTP
	// endpoint to probe — the process being alive is the health signal.
	if rs.State == pb.RobotState_ROBOT_STATE_RUNNING && !rs.process.IsRunning() {
		rs.State = pb.RobotState_ROBOT_STATE_ERROR
		log.Printf("[ros2-controller] GetRobotStatus: %s process died unexpectedly", req.RobotId)
	}

	return &pb.GetRobotStatusResponse{
		RobotId:     rs.Config.ID,
		Mode:        rs.Mode,
		State:       rs.State,
		CurrentMode: modeConfigToInfo(rs.Mode, rs.ActiveMode),
		RosDomainId: int32(rs.domainID),
		Params:      rs.Config.Params,
	}, nil
}

// --------------------------------------------------------------------------
// ListRobots / ListModes
// --------------------------------------------------------------------------

// ListRobots returns information for all registered robots.
func (c *Controller) ListRobots(ctx context.Context, req *pb.ListRobotsRequest) (*pb.ListRobotsResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	robots := make([]*pb.RobotInfo, 0, len(c.robots))
	for _, rs := range c.robots {
		robots = append(robots, &pb.RobotInfo{
			RobotId:     rs.Config.ID,
			Mode:        rs.Mode,
			State:       rs.State,
			CurrentMode: modeConfigToInfo(rs.Mode, rs.ActiveMode),
			RosDomainId: int32(rs.domainID),
			Params:      rs.Config.Params,
		})
	}

	sort.Slice(robots, func(i, j int) bool {
		return robots[i].GetRobotId() < robots[j].GetRobotId()
	})

	return &pb.ListRobotsResponse{Robots: robots}, nil
}

// ListModes returns the modes supported by the requested robot's type.
func (c *Controller) ListModes(ctx context.Context, req *pb.ListModesRequest) (*pb.ListModesResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rs, typeCfg, err := c.lookupRobot(req.RobotId)
	if err != nil {
		return nil, err
	}

	modes := make([]*pb.ModeInfo, 0, len(typeCfg.Modes))
	for _, name := range typeCfg.SupportedModes() {
		cfg := typeCfg.Modes[name]
		modes = append(modes, &pb.ModeInfo{
			Name:                 name,
			Package:              cfg.Package,
			LaunchFile:           cfg.LaunchFile,
			Args:                 cfg.Args,
			Env:                  cfg.Env,
			ArgFrom:              cfg.ArgFrom,
			PassthroughRobotArgs: cfg.PassthroughRobotArgs,
		})
	}

	return &pb.ListModesResponse{
		RobotId: rs.Config.ID,
		Modes:   modes,
	}, nil
}

// --------------------------------------------------------------------------
// GetRobotLogs
// --------------------------------------------------------------------------

// GetRobotLogs returns the trailing log lines from the robot's launch
// process.
func (c *Controller) GetRobotLogs(ctx context.Context, req *pb.GetRobotLogsRequest) (*pb.GetRobotLogsResponse, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	rs, _, err := c.lookupRobot(req.RobotId)
	if err != nil {
		return nil, err
	}

	lines := rs.process.Logs(int(req.Tail))
	return &pb.GetRobotLogsResponse{
		RobotId: req.RobotId,
		Lines:   lines,
	}, nil
}
