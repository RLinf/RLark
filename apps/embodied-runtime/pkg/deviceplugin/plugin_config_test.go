package deviceplugin

import (
	"os"
	"path/filepath"
	"testing"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// TestLoadConfig verifies that a YAML config file is parsed into the
// expected PluginConfig values, with defaults applied for omitted fields.
func TestLoadConfig(t *testing.T) {
	content := `
resource_name: rlinf.io/device
device_count: 2
socket_path: /tmp/test.sock
skip_register: true

host_devices:
  - host_path: /dev/video0
    container_path: /dev/video0
    permissions: rwm
  - host_path: /dev/ttyUSB0
    permissions: rw
  - host_path: /dev/snd/controlC0

camera:
  manager_mode: pod
  ctrl_config_path: /etc/rlinf/cam.yaml
  ctrl_bin: /usr/local/bin/camera-controller
  ctr_cli: /opt/rlinf/bin/camctr
  cameras:
    - id: video0
      name: USB Camera
      camera_type: v4l2
      width: 1280
      height: 720
      fps: 30
  auto_detect_v4l2: false
  http_addr: :8080
  pod:
    namespace: robot-ns
    node_name: robot-node
    image: custom/camera:v2
    init_image: custom/runtime:v2
    pod_generate_name: cam-0
    hostname: camera-controller
    subdomain: camera-controller-headless
    pre_command: |
      source /opt/ros/noetic/setup.bash
    extra_env:
      ROS_PACKAGE_PATH: /catkin_ws/devel_isolated/share
    labels:
      app.kubernetes.io/name: camera-controller
      app.kubernetes.io/managed-by: embodied-runtime-device-plugin

ros:
  manager_mode: local
  ctrl_config_path: /etc/rlinf/ros.yaml
  ctrl_bin: /usr/local/bin/ros-controller
  ctr_cli: /opt/rlinf/bin/rosctr
  macvlans:
    - host_nic: eth0
      name: macvlan0
      ip: 172.16.0.100/24
  types:
    - type: franka
      modes:
        joint:
          package: franka_pkg
          launch_file: joint.launch
  robots:
    - id: franka-0
      type: franka
  http_addr: :8080
  allowed_launch_packages:
    - franka_pkg
  pod:
    namespace: default
    node_name: your-node-name
    image: rlinf/serl_franka_controllers:v0.1.0
    init_image: rlinf/embodied-runtime:v0.1.0
    pod_generate_name: ros-controller
    hostname: ros-controller
    subdomain: ros-controller-headless
    pre_command: |
      source /opt/ros/noetic/setup.bash
      source /catkin_ws/devel_isolated/setup.bash
    extra_env:
      ROS_PACKAGE_PATH: /catkin_ws/devel_isolated/share
      CMAKE_PREFIX_PATH: /catkin_ws/devel_isolated
`
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Common fields.
	if cfg.ResourceName != "rlinf.io/device" {
		t.Errorf("ResourceName = %q", cfg.ResourceName)
	}
	if cfg.DeviceCount != 2 {
		t.Errorf("DeviceCount = %d, want 2", cfg.DeviceCount)
	}
	if cfg.SocketPath != "/tmp/test.sock" {
		t.Errorf("SocketPath = %q", cfg.SocketPath)
	}
	if !cfg.SkipRegister {
		t.Error("SkipRegister should be true")
	}

	// Host devices passthrough.
	if len(cfg.HostDevices) != 3 {
		t.Fatalf("HostDevices = %+v, want 3 entries", cfg.HostDevices)
	}
	if cfg.HostDevices[0].HostPath != "/dev/video0" {
		t.Errorf("HostDevices[0].HostPath = %q", cfg.HostDevices[0].HostPath)
	}
	if cfg.HostDevices[0].ContainerPath != "/dev/video0" {
		t.Errorf("HostDevices[0].ContainerPath = %q", cfg.HostDevices[0].ContainerPath)
	}
	if cfg.HostDevices[0].Permissions != "rwm" {
		t.Errorf("HostDevices[0].Permissions = %q", cfg.HostDevices[0].Permissions)
	}
	// container_path defaults to host_path when omitted; handled in
	// hostDeviceSpecs, but the config field itself stays empty.
	if cfg.HostDevices[1].HostPath != "/dev/ttyUSB0" {
		t.Errorf("HostDevices[1].HostPath = %q", cfg.HostDevices[1].HostPath)
	}
	if cfg.HostDevices[1].ContainerPath != "" {
		t.Errorf("HostDevices[1].ContainerPath = %q, want empty", cfg.HostDevices[1].ContainerPath)
	}
	if cfg.HostDevices[1].Permissions != "rw" {
		t.Errorf("HostDevices[1].Permissions = %q", cfg.HostDevices[1].Permissions)
	}
	if cfg.HostDevices[2].HostPath != "/dev/snd/controlC0" {
		t.Errorf("HostDevices[2].HostPath = %q", cfg.HostDevices[2].HostPath)
	}

	// Camera inlined device config.
	if len(cfg.Camera.Cameras) != 1 || cfg.Camera.Cameras[0].ID != "video0" {
		t.Errorf("Camera.Cameras = %+v", cfg.Camera.Cameras)
	}
	if cfg.Camera.AutoDetectV4L2 == nil || *cfg.Camera.AutoDetectV4L2 {
		t.Error("Camera.AutoDetectV4L2 should be false")
	}

	// Camera config.
	if cfg.Camera.ManagerMode != ManagerModePod {
		t.Errorf("Camera.ManagerMode = %q, want pod", cfg.Camera.ManagerMode)
	}
	if cfg.Camera.CtrlConfigPath != "/etc/rlinf/cam.yaml" {
		t.Errorf("Camera.CtrlConfigPath = %q", cfg.Camera.CtrlConfigPath)
	}
	if cfg.Camera.CtrlBin != "/usr/local/bin/camera-controller" {
		t.Errorf("Camera.CtrlBin = %q", cfg.Camera.CtrlBin)
	}
	if cfg.Camera.CtrCLI != "/opt/rlinf/bin/camctr" {
		t.Errorf("Camera.CtrCLI = %q", cfg.Camera.CtrCLI)
	}
	if cfg.Camera.HTTPAddr != ":8080" {
		t.Errorf("Camera.HTTPAddr = %q, want :8080", cfg.Camera.HTTPAddr)
	}
	if cfg.Camera.Pod.Namespace != "robot-ns" {
		t.Errorf("Camera.Pod.Namespace = %q", cfg.Camera.Pod.Namespace)
	}
	if cfg.Camera.Pod.NodeName != "robot-node" {
		t.Errorf("Camera.Pod.NodeName = %q", cfg.Camera.Pod.NodeName)
	}
	if cfg.Camera.Pod.Image != "custom/camera:v2" {
		t.Errorf("Camera.Pod.Image = %q", cfg.Camera.Pod.Image)
	}
	if cfg.Camera.Pod.InitImage != "custom/runtime:v2" {
		t.Errorf("Camera.Pod.InitImage = %q", cfg.Camera.Pod.InitImage)
	}
	if cfg.Camera.Pod.PodGenerateName != "cam-0" {
		t.Errorf("Camera.Pod.PodGenerateName = %q", cfg.Camera.Pod.PodGenerateName)
	}
	if cfg.Camera.Pod.Hostname != "camera-controller" {
		t.Errorf("Camera.Pod.Hostname = %q, want camera-controller", cfg.Camera.Pod.Hostname)
	}
	if cfg.Camera.Pod.Subdomain != "camera-controller-headless" {
		t.Errorf("Camera.Pod.Subdomain = %q, want camera-controller-headless", cfg.Camera.Pod.Subdomain)
	}
	wantCamPreCmd := "source /opt/ros/noetic/setup.bash\n"
	if cfg.Camera.Pod.PreCommand != wantCamPreCmd {
		t.Errorf("Camera.Pod.PreCommand = %q, want %q", cfg.Camera.Pod.PreCommand, wantCamPreCmd)
	}
	if cfg.Camera.Pod.ExtraEnv["ROS_PACKAGE_PATH"] != "/catkin_ws/devel_isolated/share" {
		t.Errorf("Camera.Pod.ExtraEnv = %+v", cfg.Camera.Pod.ExtraEnv)
	}

	// ROS config.
	if cfg.ROS.ManagerMode != ManagerModeLocal {
		t.Errorf("ROS.ManagerMode = %q, want local", cfg.ROS.ManagerMode)
	}
	if cfg.ROS.CtrlConfigPath != "/etc/rlinf/ros.yaml" {
		t.Errorf("ROS.CtrlConfigPath = %q", cfg.ROS.CtrlConfigPath)
	}
	// ROS inlined device config (ControllerConfig).
	if len(cfg.ROS.MACVLANs) != 1 || cfg.ROS.MACVLANs[0].HostNIC != "eth0" {
		t.Errorf("ROS.MACVLANs = %+v", cfg.ROS.MACVLANs)
	}
	if len(cfg.ROS.Types) != 1 || cfg.ROS.Types[0].Type != "franka" {
		t.Errorf("ROS.Types = %+v", cfg.ROS.Types)
	}
	if len(cfg.ROS.Robots) != 1 || cfg.ROS.Robots[0].ID != "franka-0" {
		t.Errorf("ROS.Robots = %+v", cfg.ROS.Robots)
	}
	if cfg.ROS.HTTPAddr != ":8080" {
		t.Errorf("ROS.HTTPAddr = %q, want :8080", cfg.ROS.HTTPAddr)
	}
	if len(cfg.ROS.AllowedLaunchPackages) != 1 || cfg.ROS.AllowedLaunchPackages[0] != "franka_pkg" {
		t.Errorf("ROS.AllowedLaunchPackages = %v", cfg.ROS.AllowedLaunchPackages)
	}
	if cfg.ROS.Pod.Hostname != "ros-controller" {
		t.Errorf("ROS.Pod.Hostname = %q, want ros-controller", cfg.ROS.Pod.Hostname)
	}
	if cfg.ROS.Pod.Subdomain != "ros-controller-headless" {
		t.Errorf("ROS.Pod.Subdomain = %q, want ros-controller-headless", cfg.ROS.Pod.Subdomain)
	}
	wantROSPreCmd := "source /opt/ros/noetic/setup.bash\nsource /catkin_ws/devel_isolated/setup.bash\n"
	if cfg.ROS.Pod.PreCommand != wantROSPreCmd {
		t.Errorf("ROS.Pod.PreCommand = %q, want %q", cfg.ROS.Pod.PreCommand, wantROSPreCmd)
	}
	if len(cfg.ROS.Pod.ExtraEnv) != 2 {
		t.Fatalf("ROS.Pod.ExtraEnv = %+v, want 2 entries", cfg.ROS.Pod.ExtraEnv)
	}
	if cfg.ROS.Pod.ExtraEnv["ROS_PACKAGE_PATH"] != "/catkin_ws/devel_isolated/share" {
		t.Errorf("ROS.Pod.ExtraEnv ROS_PACKAGE_PATH = %q", cfg.ROS.Pod.ExtraEnv["ROS_PACKAGE_PATH"])
	}
	if cfg.ROS.Pod.ExtraEnv["CMAKE_PREFIX_PATH"] != "/catkin_ws/devel_isolated" {
		t.Errorf("ROS.Pod.ExtraEnv CMAKE_PREFIX_PATH = %q", cfg.ROS.Pod.ExtraEnv["CMAKE_PREFIX_PATH"])
	}
}

// TestLoadConfig_Defaults verifies that omitted fields get defaults.
func TestLoadConfig_Defaults(t *testing.T) {
	content := `# empty config — everything should default`
	path := filepath.Join(t.TempDir(), "empty.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// ResourceName/SocketPath default to empty (unset); the effective
	// values come through EffectiveResourceName/EffectiveSocketPath.
	if cfg.ResourceName != "" {
		t.Errorf("ResourceName = %q, want empty", cfg.ResourceName)
	}
	if cfg.SocketPath != "" {
		t.Errorf("SocketPath = %q, want empty", cfg.SocketPath)
	}
	if got := cfg.EffectiveResourceName(); got != ResourceName {
		t.Errorf("EffectiveResourceName() = %q, want %q", got, ResourceName)
	}
	if got := cfg.EffectiveSocketPath(); got != PluginSocketPath() {
		t.Errorf("EffectiveSocketPath() = %q, want %q", got, PluginSocketPath())
	}
	if cfg.DeviceCount != DefaultDeviceCount {
		t.Errorf("DeviceCount = %d, want %d", cfg.DeviceCount, DefaultDeviceCount)
	}
	if cfg.Camera.ManagerMode != ManagerModeDisabled {
		t.Errorf("Camera.ManagerMode = %q, want disabled", cfg.Camera.ManagerMode)
	}
	if cfg.ROS.ManagerMode != ManagerModeDisabled {
		t.Errorf("ROS.ManagerMode = %q, want disabled", cfg.ROS.ManagerMode)
	}
	if cfg.Camera.CtrlConfigPath != CameraCtrlConfigPath {
		t.Errorf("Camera.CtrlConfigPath = %q, want %q", cfg.Camera.CtrlConfigPath, CameraCtrlConfigPath)
	}
	if cfg.ROS.CtrlConfigPath != ROSCtrlConfigPath {
		t.Errorf("ROS.CtrlConfigPath = %q, want %q", cfg.ROS.CtrlConfigPath, ROSCtrlConfigPath)
	}
	if len(cfg.HostDevices) != 0 {
		t.Errorf("HostDevices = %+v, want empty by default", cfg.HostDevices)
	}
}

// TestLoadConfig_MissingFile verifies that a non-existent file returns an error.
func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// TestLoadConfig_PartialCameraPod verifies that partial pod config keeps
// the defaults for the fields not present.
func TestLoadConfig_PartialCameraPod(t *testing.T) {
	content := `
camera:
  manager_mode: pod
  pod:
    node_name: my-node
`
	path := filepath.Join(t.TempDir(), "partial.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Camera.ManagerMode != ManagerModePod {
		t.Errorf("Camera.ManagerMode = %q, want pod", cfg.Camera.ManagerMode)
	}
	if cfg.Camera.Pod.NodeName != "my-node" {
		t.Errorf("Camera.Pod.NodeName = %q, want my-node", cfg.Camera.Pod.NodeName)
	}
	// Namespace should be empty (podmanager applies "default" later).
	if cfg.Camera.Pod.Namespace != "" {
		t.Errorf("Camera.Pod.Namespace = %q, want empty", cfg.Camera.Pod.Namespace)
	}
}

// TestEffectiveNames verifies the resource name and socket path derivation,
// in particular that explicit resource_name / socket_path take priority over
// the model-derived values.
func TestEffectiveNames(t *testing.T) {
	cases := []struct {
		name         string
		cfg          PluginConfig
		wantResource string
		wantSocket   string
	}{
		{
			name:         "defaults",
			cfg:          DefaultPluginConfig(),
			wantResource: "rlinf.io/device",
			wantSocket:   PluginSocketPath(),
		},
		{
			name: "model only",
			cfg: PluginConfig{
				Model:       "franka",
				DeviceCount: 1,
			},
			wantResource: "rlinf.io/device-franka",
			wantSocket:   pluginapi.DevicePluginPath + "rlinf-device-franka.sock",
		},
		{
			name: "explicit resource_name wins over model",
			cfg: PluginConfig{
				ResourceName: "rlinf.io/custom",
				Model:        "franka",
			},
			wantResource: "rlinf.io/custom",
			wantSocket:   pluginapi.DevicePluginPath + "rlinf-device-franka.sock",
		},
		{
			name: "explicit socket_path wins over model",
			cfg: PluginConfig{
				Model:      "franka",
				SocketPath: "/tmp/custom.sock",
			},
			wantResource: "rlinf.io/device-franka",
			wantSocket:   "/tmp/custom.sock",
		},
		{
			name: "explicit both win over model",
			cfg: PluginConfig{
				ResourceName: "rlinf.io/custom",
				Model:        "franka",
				SocketPath:   "/tmp/custom.sock",
			},
			wantResource: "rlinf.io/custom",
			wantSocket:   "/tmp/custom.sock",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.EffectiveResourceName(); got != tc.wantResource {
				t.Errorf("EffectiveResourceName() = %q, want %q", got, tc.wantResource)
			}
			if got := tc.cfg.EffectiveSocketPath(); got != tc.wantSocket {
				t.Errorf("EffectiveSocketPath() = %q, want %q", got, tc.wantSocket)
			}
		})
	}
}
