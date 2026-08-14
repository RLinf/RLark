package ros2controller

import (
	"fmt"
	"log"
	"sync"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// maxDomainID is the maximum ROS_DOMAIN_ID value (DDS spec: 0–101).
const maxDomainID = 101

// Controller implements the RobotController gRPC service for ROS 2 robots.
// It manages robot nodes by launching ros2 launch processes that run directly
// in the container.
//
// Unlike ROS 1, there is no central master (roscore). Each robot gets its
// own ROS_DOMAIN_ID for DDS-level isolation, so multiple robots in the same
// container do not discover each other's topics/services.
//
// Networking:
//
//	Container
//	┌─────────────────────────────────────────────────────┐
//	│ eth0:       10.1.0.5/24 (pod network)              │
//	│ macvlan0:   172.16.0.100/24 (robot network)        │
//	│                                                     │
//	│ robot-0: ROS_DOMAIN_ID=0                           │
//	│   ros2 launch (pymoveitconfig/impedance.launch.py) │
//	│                                                     │
//	│ robot-1: ROS_DOMAIN_ID=1                           │
//	│   ros2 launch (pymoveitconfig/joint.launch.py)     │
//	└─────────────────────────────────────────────────────┘
type Controller struct {
	pb.UnimplementedRobotControllerServer

	mu     sync.RWMutex
	types  map[string]*RobotTypeConfig
	robots map[string]*robotState

	nextDomainID          int // auto-assigned ROS_DOMAIN_ID for the next robot
	podIP                 string
	allowedLaunchPackages map[string]bool // nil = empty (none allowed); "*" = allow all
}

type robotState struct {
	Config     *RobotConfig
	Mode       string
	State      pb.RobotState
	ActiveMode ModeConfig // current running mode's full config
	domainID   int
	process    *RobotProcess
}

// NewController creates a new Controller with empty registries.
// allowedPackages is a whitelist of launch packages allowed for custom modes.
// Empty list means no packages are allowed (custom modes disabled).
// The special entry "*" allows all packages without restriction.
func NewController(podIP string, allowedPackages []string) *Controller {
	c := &Controller{
		types:        make(map[string]*RobotTypeConfig),
		robots:       make(map[string]*robotState),
		nextDomainID: 0,
		podIP:        podIP,
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
	log.Printf("[ros2-controller] registered robot type: %s (modes: %v)", cfg.Type, cfg.SupportedModes())
	return nil
}

// RegisterRobot registers a robot instance of a previously registered type
// and assigns it a ROS_DOMAIN_ID.
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

	// Assign ROS_DOMAIN_ID. Use the explicit override if set, otherwise
	// auto-assign the next available ID.
	domainID := cfg.DomainID
	if domainID == 0 {
		domainID = c.nextDomainID
		c.nextDomainID++
		if c.nextDomainID > maxDomainID {
			return fmt.Errorf("register robot %q: ROS_DOMAIN_ID exhausted (max %d)", cfg.ID, maxDomainID)
		}
	}

	c.robots[cfg.ID] = &robotState{
		Config:   cfg,
		State:    pb.RobotState_ROBOT_STATE_STOPPED,
		domainID: domainID,
		process: NewRobotProcess(func(err error) {
			c.onRobotExit(cfg.ID, err)
		}),
	}
	log.Printf("[ros2-controller] registered robot: %s (type=%s, domain_id=%d)",
		cfg.ID, cfg.Type, domainID)
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

// --------------------------------------------------------------------------
// onRobotExit — called in a goroutine when the ros2 launch process exits
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
		log.Printf("[ros2-controller] %s process died unexpectedly: %v", robotID, exitErr)
	} else {
		rs.State = pb.RobotState_ROBOT_STATE_STOPPED
		log.Printf("[ros2-controller] %s process exited normally", robotID)
	}
}

// Shutdown stops all launch processes. Called when the server is shutting down.
func (c *Controller) Shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for id, rs := range c.robots {
		if rs.process.IsRunning() {
			if err := rs.process.Stop(); err != nil {
				log.Printf("[ros2-controller] stop process for %s: %v", id, err)
			}
		}
	}
}
