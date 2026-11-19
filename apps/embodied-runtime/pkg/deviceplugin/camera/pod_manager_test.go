package camera

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/podmanager"
)

// TestNewPodManager_Defaults verifies that the camera PodManager is
// configured with the correct camera-specific defaults.
func TestNewPodManager_Defaults(t *testing.T) {
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName: "node-1",
	})
	cfg := mgr.Config()

	if cfg.PodGenerateName != "camera-controller" {
		t.Errorf("PodGenerateName = %q", cfg.PodGenerateName)
	}
	if cfg.Image != defaultImage {
		t.Errorf("Image = %q, want %q", cfg.Image, defaultImage)
	}
	if cfg.InitImage != defaultInitImage {
		t.Errorf("InitImage = %q, want %q", cfg.InitImage, defaultInitImage)
	}
	if cfg.ControllerBin != "camera-controller" {
		t.Errorf("ControllerBin = %q", cfg.ControllerBin)
	}
	if cfg.CLIBin != "camctr" {
		t.Errorf("CLIBin = %q", cfg.CLIBin)
	}
	if cfg.ConfigFileName != "camera-controller.yaml" {
		t.Errorf("ConfigFileName = %q", cfg.ConfigFileName)
	}
	if cfg.SocketPath != "/var/run/rlark/camera-ctrl.sock" {
		t.Errorf("SocketPath = %q", cfg.SocketPath)
	}
	if cfg.Shell != "sh" {
		t.Errorf("Shell = %q, want sh", cfg.Shell)
	}
	if cfg.PreCommand != "" {
		t.Errorf("PreCommand = %q, want empty", cfg.PreCommand)
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
	if len(cfg.ExtraArgs) != 0 {
		t.Errorf("ExtraArgs = %v, want empty", cfg.ExtraArgs)
	}
	if len(cfg.ExtraEnv) != 0 {
		t.Errorf("ExtraEnv = %v, want empty", cfg.ExtraEnv)
	}
}

// TestNewPodManager_Overrides verifies that user-provided options override
// the defaults.
func TestNewPodManager_Overrides(t *testing.T) {
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		Namespace:       "robot-ns",
		NodeName:        "robot-node",
		Image:           "custom/camera:v2",
		InitImage:       "custom/runtime:v2",
		ImagePullPolicy: corev1.PullIfNotPresent,
		PodGenerateName: "cam-0",
	})
	cfg := mgr.Config()

	if cfg.Namespace != "robot-ns" {
		t.Errorf("Namespace = %q", cfg.Namespace)
	}
	if cfg.NodeName != "robot-node" {
		t.Errorf("NodeName = %q", cfg.NodeName)
	}
	if cfg.Image != "custom/camera:v2" {
		t.Errorf("Image = %q", cfg.Image)
	}
	if cfg.InitImage != "custom/runtime:v2" {
		t.Errorf("InitImage = %q", cfg.InitImage)
	}
	if cfg.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("ImagePullPolicy = %q", cfg.ImagePullPolicy)
	}
	if cfg.PodGenerateName != "cam-0" {
		t.Errorf("PodGenerateName = %q", cfg.PodGenerateName)
	}
}

// TestNewPodManager_HeadlessDNS verifies that Hostname/Subdomain flow through
// to the podmanager Config (and thus to pod.spec.hostname/subdomain).
func TestNewPodManager_HeadlessDNS(t *testing.T) {
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName:  "node-1",
		Hostname:  "camera-controller",
		Subdomain: "camera-controller-headless",
	})
	cfg := mgr.Config()
	if cfg.Hostname != "camera-controller" {
		t.Errorf("Hostname = %q, want camera-controller", cfg.Hostname)
	}
	if cfg.Subdomain != "camera-controller-headless" {
		t.Errorf("Subdomain = %q, want camera-controller-headless", cfg.Subdomain)
	}
}

// TestNewPodManager_ApplyConfig verifies that ApplyConfig stores the config
// without error.
func TestNewPodManager_ApplyConfig(t *testing.T) {
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName: "node-1",
	})
	config := []byte("cameras:\n- id: cam0\n  camera_type: v4l2\n")

	if err := mgr.ApplyConfig(context.Background(), config); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
}

// TestNewPodManager_PreCommandOverride verifies that a user-supplied
// PreCommand is set on the config (camera-controller has no default
// pre-command, so the override is the only way to set one).
func TestNewPodManager_PreCommandOverride(t *testing.T) {
	custom := "source /opt/ros/noetic/setup.bash"
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName:   "node-1",
		PreCommand: custom,
	})
	cfg := mgr.Config()

	if cfg.PreCommand != custom {
		t.Errorf("PreCommand = %q, want %q", cfg.PreCommand, custom)
	}
}

// TestNewPodManager_ExtraEnv verifies that user-supplied ExtraEnv flows
// through to the config (camera-controller has no built-in default).
func TestNewPodManager_ExtraEnv(t *testing.T) {
	env := []corev1.EnvVar{
		{Name: "ROS_PACKAGE_PATH", Value: "/opt/ros/noetic/share"},
	}
	mgr := NewPodManager(fake.NewSimpleClientset(), podmanager.PodOptions{
		NodeName: "node-1",
		ExtraEnv: env,
	})
	cfg := mgr.Config()

	if len(cfg.ExtraEnv) != len(env) {
		t.Fatalf("ExtraEnv = %v, want %v", cfg.ExtraEnv, env)
	}
	if cfg.ExtraEnv[0] != env[0] {
		t.Errorf("ExtraEnv[0] = %+v, want %+v", cfg.ExtraEnv[0], env[0])
	}
}
