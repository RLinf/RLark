package ros

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/podmanager"
)

// TestNewPodManager_Defaults verifies that the ros PodManager is configured
// with the correct ros-specific defaults.
func TestNewPodManager_Defaults(t *testing.T) {
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName: "node-1",
	})
	cfg := mgr.Config()

	if cfg.PodGenerateName != "ros-controller" {
		t.Errorf("PodGenerateName = %q", cfg.PodGenerateName)
	}
	if cfg.Image != defaultImage {
		t.Errorf("Image = %q, want %q", cfg.Image, defaultImage)
	}
	if cfg.InitImage != defaultInitImage {
		t.Errorf("InitImage = %q, want %q", cfg.InitImage, defaultInitImage)
	}
	if cfg.ControllerBin != "ros-controller" {
		t.Errorf("ControllerBin = %q", cfg.ControllerBin)
	}
	if cfg.CLIBin != "rosctr" {
		t.Errorf("CLIBin = %q", cfg.CLIBin)
	}
	if cfg.ConfigFileName != "ros-controller.yaml" {
		t.Errorf("ConfigFileName = %q", cfg.ConfigFileName)
	}
	if cfg.SocketPath != "/var/run/rlark/ros-ctrl.sock" {
		t.Errorf("SocketPath = %q", cfg.SocketPath)
	}
	if cfg.Shell != "bash" {
		t.Errorf("Shell = %q, want bash", cfg.Shell)
	}
	if cfg.PreCommand == "" {
		t.Error("PreCommand should not be empty (should source ROS)")
	}
	if !cfg.HostPID {
		t.Error("HostPID should be true")
	}
	if !cfg.Privileged {
		t.Error("Privileged should be true")
	}
	if cfg.NodeName != "node-1" {
		t.Errorf("NodeName = %q, want node-1", cfg.NodeName)
	}

	// Extra args: the controller reads http_addr from the mounted config
	// file (same path in local and pod mode), so no --http-addr CLI flag
	// is passed. ExtraArgs should be empty.
	if len(cfg.ExtraArgs) != 0 {
		t.Errorf("ExtraArgs = %v, want empty (http_addr comes from config file)", cfg.ExtraArgs)
	}

	// ExtraEnv has no built-in default anymore (env vars are configured via
	// pod options). It should be empty unless explicitly set.
	if len(cfg.ExtraEnv) != 0 {
		t.Errorf("ExtraEnv = %v, want empty (no built-in default)", cfg.ExtraEnv)
	}
}

// TestNewPodManager_Overrides verifies that user-provided options override
// the defaults.
func TestNewPodManager_Overrides(t *testing.T) {
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		Namespace:       "robot-ns",
		NodeName:        "robot-node",
		Image:           "custom/ros:v2",
		InitImage:       "custom/runtime:v2",
		PodGenerateName: "ros-0",
	})
	cfg := mgr.Config()

	if cfg.Namespace != "robot-ns" {
		t.Errorf("Namespace = %q", cfg.Namespace)
	}
	if cfg.NodeName != "robot-node" {
		t.Errorf("NodeName = %q", cfg.NodeName)
	}
	if cfg.Image != "custom/ros:v2" {
		t.Errorf("Image = %q", cfg.Image)
	}
	if cfg.InitImage != "custom/runtime:v2" {
		t.Errorf("InitImage = %q", cfg.InitImage)
	}
	if cfg.PodGenerateName != "ros-0" {
		t.Errorf("PodGenerateName = %q", cfg.PodGenerateName)
	}
}

// TestNewPodManager_HeadlessDNS verifies that Hostname/Subdomain flow through
// to the podmanager Config (and thus to pod.spec.hostname/subdomain).
func TestNewPodManager_HeadlessDNS(t *testing.T) {
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName:  "node-1",
		Hostname:  "ros-controller",
		Subdomain: "ros-controller-headless",
	})
	cfg := mgr.Config()
	if cfg.Hostname != "ros-controller" {
		t.Errorf("Hostname = %q, want ros-controller", cfg.Hostname)
	}
	if cfg.Subdomain != "ros-controller-headless" {
		t.Errorf("Subdomain = %q, want ros-controller-headless", cfg.Subdomain)
	}
}

// TestNewPodManager_ImagePullPolicy verifies the pull policy override.
func TestNewPodManager_ImagePullPolicy(t *testing.T) {
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName:        "node-1",
		ImagePullPolicy: corev1.PullIfNotPresent,
	})
	cfg := mgr.Config()

	if cfg.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("ImagePullPolicy = %q, want IfNotPresent", cfg.ImagePullPolicy)
	}
}

// TestNewPodManager_PreCommandOverride verifies that a user-supplied
// PreCommand fully replaces the ROS workspace sourcing default.
func TestNewPodManager_PreCommandOverride(t *testing.T) {
	custom := "source /my/ws/setup.bash"
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName:   "node-1",
		PreCommand: custom,
	})
	cfg := mgr.Config()

	if cfg.PreCommand != custom {
		t.Errorf("PreCommand = %q, want %q", cfg.PreCommand, custom)
	}
}

// TestNewPodManager_PreCommandDefault verifies that an empty PreCommand
// falls back to the constructor default (ROS workspace sourcing).
func TestNewPodManager_PreCommandDefault(t *testing.T) {
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName: "node-1",
	})
	cfg := mgr.Config()

	if cfg.PreCommand != preCommand {
		t.Errorf("PreCommand = %q, want default %q", cfg.PreCommand, preCommand)
	}
}

// TestNewPodManager_ExtraEnv verifies that user-supplied ExtraEnv flows
// through to the config (no built-in default anymore).
func TestNewPodManager_ExtraEnv(t *testing.T) {
	env := []corev1.EnvVar{
		{Name: "ROS_PACKAGE_PATH", Value: "/catkin_ws/devel_isolated/share"},
		{Name: "CMAKE_PREFIX_PATH", Value: "/catkin_ws/devel_isolated"},
	}
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName: "node-1",
		ExtraEnv: env,
	})
	cfg := mgr.Config()

	if len(cfg.ExtraEnv) != len(env) {
		t.Fatalf("ExtraEnv = %v, want %v", cfg.ExtraEnv, env)
	}
	for i, e := range env {
		if cfg.ExtraEnv[i] != e {
			t.Errorf("ExtraEnv[%d] = %+v, want %+v", i, cfg.ExtraEnv[i], e)
		}
	}
}
