package deviceplugin

import (
	"fmt"
	"os"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cameracontroller"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/netmac"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/ros2controller"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/roscontroller"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ---------------------------------------------------------------------------
// Manager mode
// ---------------------------------------------------------------------------

// ManagerMode selects how a controller is launched.
type ManagerMode string

const (
	// ManagerModeDisabled does not launch any controller. The manager is
	// nil, so no config is generated/applied, no subprocess or pod is
	// started, and device detection for this controller is skipped. This
	// is the default: nothing runs unless a mode is explicitly set.
	ManagerModeDisabled ManagerMode = "disabled"

	// ManagerModeLocal runs the controller as a local subprocess via exec.
	// Requires the controller binary to be present on the host.
	ManagerModeLocal ManagerMode = "local"

	// ManagerModePod runs the controller inside a Kubernetes Pod. Requires
	// the device-plugin to have permissions to create/get/delete pods.
	ManagerModePod ManagerMode = "pod"
)

// ---------------------------------------------------------------------------
// Pod-mode configs
// ---------------------------------------------------------------------------

// PodConfig holds pod-mode configuration for a controller pod (camera or
// ros — the fields are identical, so the two share this one struct). The
// controller-specific defaults (image, binary name, shell, env) are filled
// in by the per-controller NewPodManager constructor; this struct only
// carries the operator-tunable knobs.
type PodConfig struct {
	Namespace       string `yaml:"namespace"`         // default "default"
	NodeName        string `yaml:"node_name"`         // nodeSelector hostname
	Image           string `yaml:"image"`             // main container image
	InitImage       string `yaml:"init_image"`        // init container image
	PodGenerateName string `yaml:"pod_generate_name"` // base pod name; actual name is <PodGenerateName>-<NodeName>
	// PreCommand overrides the shell snippet run before the controller
	// binary. ros-controller sources the ROS + catkin workspaces by default;
	// camera-controller has none. Empty keeps the per-controller default.
	// A non-empty value fully replaces the default — for ros you must
	// include the ROS workspace sourcing yourself if you still need it.
	PreCommand string `yaml:"pre_command,omitempty"`
	// ExtraEnv are additional environment variables for the main container,
	// as a name→value map. Neither controller sets any by default; set this
	// to inject workspace or runtime env vars (e.g. ROS_PACKAGE_PATH).
	ExtraEnv map[string]string `yaml:"extra_env,omitempty"`
	// Hostname sets pod.spec.hostname for headless-service DNS. Empty leaves
	// it at the pod-name default. The headless Service is deployed alongside
	// the device plugin (not created by it).
	Hostname string `yaml:"hostname,omitempty"`
	// Subdomain sets pod.spec.subdomain; must match the name of the headless
	// Service that routes to these pods.
	Subdomain string `yaml:"subdomain,omitempty"`
	// Labels is the sole source of the pod's label set — there are no
	// built-in labels. Set whatever the workload needs here, including the
	// selector a headless Service uses to route to these pods (e.g.
	// app.kubernetes.io/name: <controller>).
	Labels          map[string]string       `yaml:"labels,omitempty"`
	OwnerReferences []metav1.OwnerReference `yaml:"-"` // auto-discovered
}

// ---------------------------------------------------------------------------
// Per-controller configs
// ---------------------------------------------------------------------------

// CameraConfig holds all camera-controller configuration, both local and
// pod mode. The manager_mode field selects which set of fields is used.
//
// The device-specific camera definitions (Cameras, AutoDetectV4L2) are
// inlined directly, replacing the previous separate device-config file.
type CameraConfig struct {
	ManagerMode ManagerMode `yaml:"manager_mode"`

	// Local-mode fields (used when manager_mode == "local").
	CtrlConfigPath string `yaml:"ctrl_config_path"` // where to write the camera-controller YAML
	CtrlBin        string `yaml:"ctrl_bin"`         // camera-controller binary path
	CtrCLI         string `yaml:"ctr_cli"`          // camctr CLI binary path

	// HTTPAddr is the optional TCP address ("host:port" or ":port") for the
	// camera-controller's HTTP/JSON gateway. It is written into the
	// controller's config file (http_addr), so it applies in BOTH local and
	// pod mode — not a pod-only setting. Empty disables HTTP.
	// Mirrors pkg/cameracontroller.ControllerConfig.HTTPAddr.
	HTTPAddr string `yaml:"http_addr,omitempty"`

	// Pod-mode fields (used when manager_mode == "pod").
	Pod PodConfig `yaml:"pod"`

	// Device config — camera definitions inlined from the former
	// device-config file.
	Cameras []cameracontroller.CameraConfig `yaml:"cameras,omitempty"`
	// AutoDetectV4L2 enables auto-detection of V4L2 cameras from
	// /sys/class/video4linux. Defaults to true (nil). Set false to disable.
	AutoDetectV4L2 *bool `yaml:"auto_detect_v4l2,omitempty"`
}

// ROSConfig holds all ros-controller configuration, both local and pod
// mode. The manager_mode field selects which set of fields is used.
//
// The ros-controller device config (roscontroller.ControllerConfig) is
// inlined directly, replacing the previous separate device-config file.
type ROSConfig struct {
	ManagerMode ManagerMode `yaml:"manager_mode"`

	// Local-mode fields (used when manager_mode == "local").
	CtrlConfigPath string `yaml:"ctrl_config_path"`
	CtrlBin        string `yaml:"ctrl_bin"`
	CtrCLI         string `yaml:"ctr_cli"`

	// Pod-mode fields (used when manager_mode == "pod").
	Pod PodConfig `yaml:"pod"`

	// Device config — ros-controller config inlined from the former
	// device-config file.
	roscontroller.ControllerConfig `yaml:",inline"`
}

// ROS2Config holds all ros2-controller configuration, both local and pod
// mode. Mirrors ROSConfig but for the ROS 2 controller.
type ROS2Config struct {
	ManagerMode ManagerMode `yaml:"manager_mode"`

	// Local-mode fields (used when manager_mode == "local").
	CtrlConfigPath string `yaml:"ctrl_config_path"`
	CtrlBin        string `yaml:"ctrl_bin"`
	CtrCLI         string `yaml:"ctr_cli"`

	// Pod-mode fields (used when manager_mode == "pod").
	Pod PodConfig `yaml:"pod"`

	// Device config — ros2-controller config inlined.
	ros2controller.ControllerConfig `yaml:",inline"`
}

// ---------------------------------------------------------------------------
// Host device passthrough config
// ---------------------------------------------------------------------------

// HostDeviceConfig defines a single host device node (/dev/xxx) to mount
// directly into the container during Allocate. Unlike Camera and ROS, no
// controller manager is launched — the device is simply passed through to
// the requesting pod based on this configuration.
type HostDeviceConfig struct {
	// HostPath is the path of the device on the host, e.g.
	// "/dev/video0", "/dev/ttyUSB0", "/dev/snd/controlC0".
	HostPath string `yaml:"host_path"`

	// ContainerPath is the path the device appears at inside the
	// container. Defaults to HostPath when empty.
	ContainerPath string `yaml:"container_path,omitempty"`

	// Permissions is the cgroup-style access for the device: any
	// combination of "r" (read), "w" (write), "m" (mknod). Defaults to
	// "rwm" when empty.
	Permissions string `yaml:"permissions,omitempty"`
}

// ---------------------------------------------------------------------------
// PluginConfig
// ---------------------------------------------------------------------------

// PluginConfig holds configuration for the plugin. It can be loaded from a
// YAML file via LoadConfig; see examples/device-plugin-config.yaml.
type PluginConfig struct {
	// ResourceName fully overrides the Kubernetes resource name advertised
	// to kubelet. When empty, the base "rlinf.io/device" is used, with the
	// model suffix appended if Model is set (e.g. "rlinf.io/device-franka").
	ResourceName string `yaml:"resource_name"`

	// Model optionally identifies the device model exposed by this plugin
	// (e.g. "franka"). When set, and ResourceName/SocketPath are not
	// explicitly configured, the advertised resource becomes
	// "rlinf.io/device-<model>" and the gRPC socket basename becomes
	// "rlinf-device-<model>.sock", so different device types can be
	// distinguished within a cluster (or coexist on one node). An explicit
	// ResourceName or SocketPath always takes priority over the model.
	Model string `yaml:"model"`

	// DeviceCount is the number of devices to advertise when auto-detection
	// is disabled or fails.
	DeviceCount int `yaml:"device_count"`

	// SocketPath fully overrides the Unix socket path for the gRPC server.
	// When empty, the socket is derived from Model
	// ("rlinf-device-<model>.sock") or defaults to "rlinf-device.sock".
	SocketPath string `yaml:"socket_path"`

	// Camera holds all camera-controller configuration (manager_mode,
	// local-mode paths, pod-mode settings).
	Camera CameraConfig `yaml:"camera"`

	// ROS holds all ros-controller configuration (manager_mode, local-mode
	// paths, pod-mode settings).
	ROS ROSConfig `yaml:"ros"`

	// ROS2 holds all ros2-controller configuration (manager_mode,
	// local-mode paths, pod-mode settings).
	ROS2 ROS2Config `yaml:"ros2"`

	// HostDevices lists host /dev/* nodes to mount directly into pods
	// during Allocate. Unlike Camera and ROS, no controller manager is
	// launched — the device paths defined here are simply passed through.
	// Empty (default) means no host device passthrough.
	HostDevices []HostDeviceConfig `yaml:"host_devices,omitempty"`

	// HostMacvlans lists macvlan interfaces to attach to requesting pods'
	// network namespaces at startup. Unlike HostDevices (mounted directly
	// during Allocate), macvlans are NOT device-mounted — they are created
	// on demand via the device init gRPC service: a pod's init container runs
	// the `devinit` CLI, which connects to the init service Unix socket (see
	// DevinitSocketPath). The service reads the caller's PID from the socket
	// peer credentials and creates the macvlans in that PID's network
	// namespace using pkg/netmac. Pods using hostNetwork are skipped (the
	// macvlan would land in the host netns). Empty (default) means the init
	// service is not started.
	HostMacvlans []netmac.MACVLANConfig `yaml:"host_macvlans,omitempty"`

	// DevinitSocketPath overrides the Unix socket path for the device init
	// gRPC service consumed by the `devinit` init-container CLI. When empty,
	// DevinitSocketPath (a socket inside RunDir, which is already mounted
	// into requesting pods) is used.
	DevinitSocketPath string `yaml:"devinit_socket_path,omitempty"`

	// SkipRegister skips kubelet registration. Useful for testing.
	SkipRegister bool `yaml:"skip_register"`
}

// DefaultPluginConfig returns a PluginConfig with sensible defaults.
// ResourceName and SocketPath are left empty so they can be detected as
// "unset"; EffectiveResourceName / EffectiveSocketPath supply the defaults.
func DefaultPluginConfig() PluginConfig {
	return PluginConfig{
		DeviceCount: DefaultDeviceCount,
		Camera: CameraConfig{
			ManagerMode:    ManagerModeDisabled,
			CtrlConfigPath: CameraCtrlConfigPath,
			CtrlBin:        envOrDefault("CAM_CTRL_BIN", "/usr/local/bin/camera-controller"),
			CtrCLI:         envOrDefault("CAM_CTR_BIN", "/opt/rlinf/bin/camctr"),
		},
		ROS: ROSConfig{
			ManagerMode:    ManagerModeDisabled,
			CtrlConfigPath: ROSCtrlConfigPath,
			CtrlBin:        envOrDefault("ROS_CTRL_BIN", "/usr/local/bin/ros-controller"),
			CtrCLI:         envOrDefault("ROS_CTR_BIN", "/opt/rlinf/bin/rosctr"),
		},
		ROS2: ROS2Config{
			ManagerMode:    ManagerModeDisabled,
			CtrlConfigPath: ROS2CtrlConfigPath,
			CtrlBin:        envOrDefault("ROS2_CTRL_BIN", "/usr/local/bin/ros2-controller"),
			CtrCLI:         envOrDefault("ROS2_CTR_BIN", "/opt/rlinf/bin/rosctr"),
		},
	}
}

// EffectiveResourceName returns the resource name this plugin advertises to
// kubelet. An explicit ResourceName always wins. Otherwise the base
// "rlinf.io/device" is used, with the model suffix appended when Model is set
// (e.g. "rlinf.io/device-franka").
func (cfg PluginConfig) EffectiveResourceName() string {
	if cfg.ResourceName != "" {
		return cfg.ResourceName
	}
	name := ResourceName
	if cfg.Model != "" {
		name += "-" + cfg.Model
	}
	return name
}

// EffectiveSocketPath returns the Unix socket path the plugin listens on.
// An explicit SocketPath always wins. Otherwise, when Model is set, the
// basename becomes "rlinf-device-<model>.sock" (allowing multiple plugins to
// coexist on one node); when Model is empty the default "rlinf-device.sock"
// is used.
func (cfg PluginConfig) EffectiveSocketPath() string {
	if cfg.SocketPath != "" {
		return cfg.SocketPath
	}
	if cfg.Model != "" {
		return pluginSocketPathForModel(cfg.Model)
	}
	return PluginSocketPath()
}

// EffectiveDevinitSocketPath returns the Unix socket path the device init
// gRPC service (consumed by the `devinit` init-container CLI) listens on.
// An explicit DevinitSocketPath always wins; otherwise the service reuses
// RunDir (already mounted read-only into requesting pods via Allocate) so
// the init container can reach the socket without an extra mount.
func (cfg PluginConfig) EffectiveDevinitSocketPath() string {
	if cfg.DevinitSocketPath != "" {
		return cfg.DevinitSocketPath
	}
	return DevinitSocketPath
}

// ---------------------------------------------------------------------------
// LoadConfig
// ---------------------------------------------------------------------------

// LoadConfig reads a plugin configuration YAML file and returns a
// PluginConfig with defaults applied. Fields present in the YAML override
// the defaults from DefaultPluginConfig; fields absent in the YAML keep
// their default values.
//
// The YAML uses snake_case keys, e.g.:
//
//	camera:
//	  manager_mode: pod
//	  pod:
//	    node_name: your-node-name
func LoadConfig(path string) (PluginConfig, error) {
	cfg := DefaultPluginConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config file: %w", err)
	}

	return cfg, nil
}
