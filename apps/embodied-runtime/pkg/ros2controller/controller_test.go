package ros2controller

import (
	"testing"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
)

// TestRegisterType verifies robot type registration and validation.
func TestRegisterType(t *testing.T) {
	c := NewController("10.0.0.1", nil)

	t.Run("valid", func(t *testing.T) {
		cfg := &RobotTypeConfig{
			Type: "franka",
			Modes: map[string]ModeConfig{
				"impedance": {Package: "pkg", LaunchFile: "impedance.launch.py"},
			},
		}
		if err := c.RegisterType(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("missing_type", func(t *testing.T) {
		cfg := &RobotTypeConfig{
			Modes: map[string]ModeConfig{
				"impedance": {Package: "pkg", LaunchFile: "impedance.launch.py"},
			},
		}
		if err := c.RegisterType(cfg); err == nil {
			t.Error("expected error for empty type")
		}
	})

	t.Run("no_modes", func(t *testing.T) {
		cfg := &RobotTypeConfig{Type: "empty"}
		if err := c.RegisterType(cfg); err == nil {
			t.Error("expected error for no modes")
		}
	})
}

// TestRegisterRobot_DomainID verifies ROS_DOMAIN_ID assignment: auto-increment
// by default, explicit override when set.
func TestRegisterRobot_DomainID(t *testing.T) {
	c := NewController("10.0.0.1", nil)
	if err := c.RegisterType(&RobotTypeConfig{
		Type: "franka",
		Modes: map[string]ModeConfig{
			"impedance": {Package: "pkg", LaunchFile: "impedance.launch.py"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("auto_increment", func(t *testing.T) {
		if err := c.RegisterRobot(&RobotConfig{ID: "r0", Type: "franka"}); err != nil {
			t.Fatalf("register r0: %v", err)
		}
		if err := c.RegisterRobot(&RobotConfig{ID: "r1", Type: "franka"}); err != nil {
			t.Fatalf("register r1: %v", err)
		}
		c.mu.RLock()
		defer c.mu.RUnlock()
		if c.robots["r0"].domainID != 0 {
			t.Errorf("r0 domain_id = %d, want 0", c.robots["r0"].domainID)
		}
		if c.robots["r1"].domainID != 1 {
			t.Errorf("r1 domain_id = %d, want 1", c.robots["r1"].domainID)
		}
	})

	t.Run("explicit_override", func(t *testing.T) {
		c := NewController("10.0.0.1", nil)
		if err := c.RegisterType(&RobotTypeConfig{
			Type: "franka",
			Modes: map[string]ModeConfig{
				"impedance": {Package: "pkg", LaunchFile: "impedance.launch.py"},
			},
		}); err != nil {
			t.Fatal(err)
		}
		if err := c.RegisterRobot(&RobotConfig{ID: "r0", Type: "franka", DomainID: 42}); err != nil {
			t.Fatalf("register r0: %v", err)
		}
		c.mu.RLock()
		defer c.mu.RUnlock()
		if c.robots["r0"].domainID != 42 {
			t.Errorf("r0 domain_id = %d, want 42", c.robots["r0"].domainID)
		}
		// nextDomainID should NOT advance when an explicit ID is used.
		if c.nextDomainID != 0 {
			t.Errorf("nextDomainID = %d, want 0 (should not advance for explicit)", c.nextDomainID)
		}
	})
}

// TestRegisterRobot_Errors verifies validation failures during registration.
func TestRegisterRobot_Errors(t *testing.T) {
	c := NewController("10.0.0.1", nil)
	if err := c.RegisterType(&RobotTypeConfig{
		Type: "franka",
		Modes: map[string]ModeConfig{
			"impedance": {Package: "pkg", LaunchFile: "impedance.launch.py"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("missing_id", func(t *testing.T) {
		if err := c.RegisterRobot(&RobotConfig{Type: "franka"}); err == nil {
			t.Error("expected error for missing ID")
		}
	})
	t.Run("missing_type", func(t *testing.T) {
		if err := c.RegisterRobot(&RobotConfig{ID: "r0"}); err == nil {
			t.Error("expected error for missing type")
		}
	})
	t.Run("unknown_type", func(t *testing.T) {
		if err := c.RegisterRobot(&RobotConfig{ID: "r0", Type: "unknown"}); err == nil {
			t.Error("expected error for unknown type")
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		_ = c.RegisterRobot(&RobotConfig{ID: "r0", Type: "franka"})
		if err := c.RegisterRobot(&RobotConfig{ID: "r0", Type: "franka"}); err == nil {
			t.Error("expected error for duplicate robot")
		}
	})
}

// TestRegisterRobot_InitialState verifies a newly registered robot is STOPPED.
func TestRegisterRobot_InitialState(t *testing.T) {
	c := NewController("10.0.0.1", nil)
	if err := c.RegisterType(&RobotTypeConfig{
		Type: "franka",
		Modes: map[string]ModeConfig{
			"impedance": {Package: "pkg", LaunchFile: "impedance.launch.py"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.RegisterRobot(&RobotConfig{ID: "r0", Type: "franka"}); err != nil {
		t.Fatalf("register: %v", err)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	rs := c.robots["r0"]
	if rs.State != pb.RobotState_ROBOT_STATE_STOPPED {
		t.Errorf("initial state = %v, want STOPPED", rs.State)
	}
	if rs.Mode != "" {
		t.Errorf("initial mode = %q, want empty", rs.Mode)
	}
}
