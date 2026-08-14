package roscontroller

import (
	"fmt"
	"log"
	"sync"
	"time"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Controller implements the RobotController gRPC service. It manages robot
// nodes by launching roslaunch processes that run directly in the container.
//
// Each robot gets its own roscore on a unique port to avoid node name /
// topic conflicts between multiple robots.
//
// Networking:
//
//	Container
//	┌─────────────────────────────────────────────────────┐
//	│ eth0:       10.1.0.5/24 (pod network)              │
//	│ macvlan0:   172.16.0.100/24 (robot network)        │
//	│                                                     │
//	│ roscore:11311 (robot-0)                             │
//	│   roslaunch                                         │
//	│     ROS_MASTER_URI=http://10.1.0.5:11311           │
//	│                                                     │
//	│ roscore:11312 (robot-1)                             │
//	│   roslaunch                                         │
//	│     ROS_MASTER_URI=http://10.1.0.5:11312           │
//	└─────────────────────────────────────────────────────┘
type Controller struct {
	pb.UnimplementedRobotControllerServer

	mu     sync.RWMutex
	types  map[string]*RobotTypeConfig
	robots map[string]*robotState

	nextROSPort           int // auto-assigned port for the next robot's roscore
	podIP                 string
	allowedLaunchPackages map[string]bool // nil = empty (none allowed); "*" = allow all
}

type robotState struct {
	Config     *RobotConfig
	Mode       string
	State      pb.RobotState
	ActiveMode ModeConfig // current running mode's full config
	roscore    *ROSCore   // per-robot roscore
	process    *RobotProcess
}

// NewController creates a new Controller with empty registries.
// allowedPackages is a whitelist of launch packages allowed for custom modes.
// Empty list means no packages are allowed (custom modes disabled).
// The special entry "*" allows all packages without restriction.
func NewController(podIP string, allowedPackages []string) *Controller {
	c := &Controller{
		types:       make(map[string]*RobotTypeConfig),
		robots:      make(map[string]*robotState),
		nextROSPort: 11311,
		podIP:       podIP,
	}
	if len(allowedPackages) > 0 {
		c.allowedLaunchPackages = make(map[string]bool, len(allowedPackages))
		for _, pkg := range allowedPackages {
			c.allowedLaunchPackages[pkg] = true
		}
	}
	return c
}

// --------------------------------------------------------------------------
// Configuration API
// --------------------------------------------------------------------------

// RegisterType registers a robot type with its supported modes.
func (c *Controller) RegisterType(cfg *RobotTypeConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cfg.Type == "" {
		return fmt.Errorf("robot type config: Type is required")
	}
	if len(cfg.Modes) == 0 {
		return fmt.Errorf("robot type config: at least one mode is required")
	}

	c.types[cfg.Type] = cfg
	log.Printf("[ros-controller] registered robot type: %s (modes: %v)", cfg.Type, cfg.SupportedModes())
	return nil
}

// RegisterRobot registers a robot instance of a previously registered type
// and starts its dedicated roscore.
func (c *Controller) RegisterRobot(cfg *RobotConfig) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cfg.ID == "" {
		return fmt.Errorf("robot config: ID is required")
	}
	if cfg.Type == "" {
		return fmt.Errorf("robot config: Type is required")
	}
	if _, ok := c.types[cfg.Type]; !ok {
		return fmt.Errorf("unknown robot type %q: register the type first", cfg.Type)
	}
	if _, exists := c.robots[cfg.ID]; exists {
		return fmt.Errorf("robot %q already registered", cfg.ID)
	}

	// Assign roscore port and start it immediately so it's ready
	// before any robot operation. Eager startup avoids the race
	// where roslaunch is launched before roscore finishes initializing.
	roscore := assignROSCore(&c.nextROSPort, c.podIP)
	if err := roscore.Start(); err != nil {
		return fmt.Errorf("register robot %q: start roscore: %w", cfg.ID, err)
	}

	// Wait for roscore to be responsive (HTTP server accepting requests).
	// If it doesn't come up in time, clean up and mark the robot as ERROR.
	if err := roscore.WaitReady(5 * time.Second); err != nil {
		_ = roscore.Stop()
		rs := &robotState{
			Config:  cfg,
			State:   pb.RobotState_ROBOT_STATE_ERROR,
			roscore: roscore,
			process: NewRobotProcess(func(err error) {
				c.onRobotExit(cfg.ID, err)
			}),
		}
		c.robots[cfg.ID] = rs
		return fmt.Errorf("register robot %q: roscore not ready: %w", cfg.ID, err)
	}

	c.robots[cfg.ID] = &robotState{
		Config:  cfg,
		State:   pb.RobotState_ROBOT_STATE_STOPPED,
		roscore: roscore,
		process: NewRobotProcess(func(err error) {
			c.onRobotExit(cfg.ID, err)
		}),
	}
	log.Printf("[ros-controller] registered robot: %s (type=%s)", cfg.ID, cfg.Type)
	return nil
}

// --------------------------------------------------------------------------
// Internal helpers
// --------------------------------------------------------------------------

func (c *Controller) lookupRobot(robotID string) (*robotState, *RobotTypeConfig, error) {
	rs, ok := c.robots[robotID]
	if !ok {
		return nil, nil, status.Errorf(codes.NotFound, "robot %q not registered", robotID)
	}
	typeCfg, ok := c.types[rs.Config.Type]
	if !ok {
		return nil, nil, status.Errorf(codes.FailedPrecondition,
			"robot type %q not registered (robot: %s)", rs.Config.Type, robotID)
	}
	return rs, typeCfg, nil
}

// assignROSCore creates a new ROSCore for the given robot config with an
// auto-assigned unique port.
func assignROSCore(nextPort *int, podIP string) *ROSCore {
	port := *nextPort
	*nextPort++
	return NewROSCore(port, podIP)
}

// rosURI returns the ROS_MASTER_URI for a robot state, or empty string
// if the roscore is not initialized.
func rosURI(rs *robotState) string {
	if rs.roscore != nil {
		return rs.roscore.URI()
	}
	return ""
}

// checkROSCoreHealth checks if the robot's roscore is responsive via HTTP.
// If the roscore is not healthy, the robot is marked as ERROR and a log
// message is emitted. Caller must hold c.mu (read or write).
func (c *Controller) checkROSCoreHealth(rs *robotState) {
	if rs.roscore == nil {
		return
	}
	if rs.State == pb.RobotState_ROBOT_STATE_ERROR {
		return // already in error state
	}
	if !rs.roscore.Healthy() {
		rs.State = pb.RobotState_ROBOT_STATE_ERROR
		log.Printf("[ros-controller] %s: roscore is not responding (port %d)", rs.Config.ID, rs.roscore.Port())
	}
}

// --------------------------------------------------------------------------
// onRobotExit — called in a goroutine when the roslaunch process exits
// --------------------------------------------------------------------------

func (c *Controller) onRobotExit(robotID string, exitErr error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rs, ok := c.robots[robotID]
	if !ok {
		return
	}

	if exitErr != nil {
		rs.State = pb.RobotState_ROBOT_STATE_ERROR
		log.Printf("[ros-controller] %s process died unexpectedly: %v", robotID, exitErr)
	} else {
		rs.State = pb.RobotState_ROBOT_STATE_STOPPED
		log.Printf("[ros-controller] %s process exited normally", robotID)
	}
}

// Shutdown stops all roscore processes. Called when the server is shutting down.
func (c *Controller) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for id, rs := range c.robots {
		if rs.roscore != nil && rs.roscore.IsRunning() {
			if err := rs.roscore.Stop(); err != nil {
				log.Printf("[ros-controller] stop roscore for %s: %v", id, err)
			}
		}
	}
}
