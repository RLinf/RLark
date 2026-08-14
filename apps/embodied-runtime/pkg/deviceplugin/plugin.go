// Package deviceplugin implements a Kubernetes Device Plugin that exposes
// shared robot and camera devices to pods. It follows the Kubernetes Device
// Plugin API (v1beta1) and is designed to run as part of a DaemonSet.
//
// The plugin registers with kubelet and advertises "rlinf.io/device"
// resources, or "rlinf.io/device-<model>" when a model is configured. Pods
// requesting this resource will receive the necessary environment variables
// and Unix socket mounts to access the node-local ROS Core, Robot Node
// Controller, and Camera Controller.
package deviceplugin

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/camera"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/podmanager"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/ros"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/ros2"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/mutatingwebhook"
	camerapb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	rospb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// ResourceName is the Kubernetes resource name exposed by this plugin.
	ResourceName = "rlinf.io/device"

	// PluginSocketName is the basename of the Unix socket the plugin listens on.
	PluginSocketName = "rlinf-device.sock"

	// DefaultDeviceCount is the number of ROS devices advertised when
	// auto-detection is not yet implemented.
	DefaultDeviceCount = 1

	// ROSCtrlSocketPath is the path to the ros-controller gRPC socket.
	ROSCtrlSocketPath = "/var/run/rlark/ros-ctrl.sock"

	// ROS2CtrlSocketPath is the path to the ros2-controller gRPC socket.
	ROS2CtrlSocketPath = "/var/run/rlark/ros2-ctrl.sock"

	// CameraCtrlSocketPath is the path to the camera-controller gRPC socket.
	CameraCtrlSocketPath = "/var/run/rlark/camera-ctrl.sock"

	// RunDir is the parent directory of the controller sockets (ros-ctrl.sock,
	// camera-ctrl.sock). Mounted as a directory so that socket recreation is
	// reflected immediately in the container.
	RunDir = "/var/run/rlark"

	// DevinitSocketPath is the default Unix socket path for the device
	// init gRPC service (consumed by the `devinit` init-container CLI).
	// It lives inside RunDir so the existing RunDir read-only mount (added
	// in Allocate) makes it reachable from a pod's init container without an
	// extra mount. Override per-config with PluginConfig.DevinitSocketPath.
	DevinitSocketPath = RunDir + "/devinit.sock"

	// ROSCtrlConfigPath is the path to the ros-controller YAML config file.
	ROSCtrlConfigPath = "/etc/rlinf/ros-controller.yaml"

	// ROS2CtrlConfigPath is the path to the ros2-controller YAML config file.
	ROS2CtrlConfigPath = "/etc/rlinf/ros2-controller.yaml"

	// CameraCtrlConfigPath is the path to the camera-controller YAML config file.
	CameraCtrlConfigPath = "/etc/rlinf/camera-controller.yaml"

	// BinDir is the parent directory of the CLI binaries (rosctr, camctr).
	// Mounted as a directory so that binary updates are reflected immediately.
	BinDir = "/opt/rlinf/bin"
)

// envOrDefault returns the value of the named environment variable, or the
// given default if the variable is empty or unset.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// PluginSocketPath returns the full path to the plugin's gRPC socket.
func PluginSocketPath() string {
	return pluginapi.DevicePluginPath + PluginSocketName
}

// pluginSocketPathForModel returns the device-plugin gRPC socket path for the
// given model. The basename becomes "rlinf-device-<model>.sock", so plugins
// exposing different models can coexist on the same node.
func pluginSocketPathForModel(model string) string {
	return pluginapi.DevicePluginPath + "rlinf-device-" + model + ".sock"
}

// ---------------------------------------------------------------------------
// Plugin — core device management logic
// ---------------------------------------------------------------------------

// Plugin manages the device inventory, handles ListAndWatch streaming to
// kubelet, and processes allocation requests from the scheduler.
type Plugin struct {
	cfg PluginConfig

	// clientset is the Kubernetes client used for pod-mode controllers and
	// (optionally) the mutating webhook. nil when no pod-mode controller and
	// no webhook is configured.
	clientset kubernetes.Interface
	// disc holds attributes of the device-plugin's own pod, auto-discovered
	// via the downward API. Used to default the devinit init image and to own
	// the controller pods / CA Secret.
	disc discoveredPod
	// webhookCfg holds the mutating webhook options (CLI-derived). Only used
	// when Enabled is true and host_macvlans is configured.
	webhookCfg WebhookConfig

	devices       []*pluginapi.Device
	rosManager    ControllerManager
	ros2Manager   ControllerManager
	cameraManager ControllerManager

	mu         sync.Mutex
	robots     []*rospb.RobotInfo
	ros2Robots []*rospb.RobotInfo
	cameras    []*camerapb.CameraDescriptor
}

// ResourceName returns the resource name this plugin advertises to kubelet,
// including any configured model suffix.
func (p *Plugin) ResourceName() string {
	return p.cfg.EffectiveResourceName()
}

// ---------------------------------------------------------------------------
// Manager factory helpers
// ---------------------------------------------------------------------------

// discoveredPod holds attributes of the device-plugin's own pod, fetched
// once and reused to fill in blank fields of the controller pods it
// creates.
type discoveredPod struct {
	ownerRefs   []metav1.OwnerReference
	tolerations []corev1.Toleration
	initImage   string // device-plugin image, reused as init image
	nodeName    string // node the device-plugin runs on
}

// envOrDiscovered returns the config value if set, otherwise the discovered
// value. This lets the user override auto-discovered attributes via config
// while still getting sensible defaults.
func envOrDiscovered(configVal, discoveredVal string) string {
	if configVal != "" {
		return configVal
	}
	return discoveredVal
}

// coreV1EnvVars converts the YAML-friendly name→value map from PodConfig
// into the []corev1.EnvVar the podmanager expects. The result is sorted by
// name so the generated pod spec is stable across runs (Go map iteration
// order is non-deterministic). A nil/empty map yields nil so no empty
// slice is passed through.
func coreV1EnvVars(envs map[string]string) []corev1.EnvVar {
	if len(envs) == 0 {
		return nil
	}
	out := make([]corev1.EnvVar, 0, len(envs))
	for name, value := range envs {
		out = append(out, corev1.EnvVar{Name: name, Value: value})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// newCameraManager creates the camera-controller manager based on the
// configured mode. If pod mode is selected but the clientset is nil
// (creation failed), it falls back to local mode. Fields left empty in the
// config are auto-filled from the discovered device-plugin pod attributes.
// Returns nil when manager_mode is "disabled" — no controller is launched,
// no config is generated, and V4L2 auto-detection is skipped.
func newCameraManager(cfg CameraConfig, clientset kubernetes.Interface, disc discoveredPod) ControllerManager {
	if cfg.ManagerMode == ManagerModeDisabled {
		return nil
	}
	if cfg.ManagerMode == ManagerModePod && clientset != nil {
		mgr := camera.NewPodManager(clientset, podmanager.PodOptions{
			Namespace:       cfg.Pod.Namespace,
			NodeName:        envOrDiscovered(cfg.Pod.NodeName, disc.nodeName),
			Image:           cfg.Pod.Image,
			InitImage:       envOrDiscovered(cfg.Pod.InitImage, disc.initImage),
			PodGenerateName: cfg.Pod.PodGenerateName,
			PreCommand:      cfg.Pod.PreCommand,
			ExtraEnv:        coreV1EnvVars(cfg.Pod.ExtraEnv),
			Hostname:        cfg.Pod.Hostname,
			Subdomain:       cfg.Pod.Subdomain,
			Labels:          cfg.Pod.Labels,
			Tolerations:     disc.tolerations,
			OwnerReferences: disc.ownerRefs,
		})
		if initErr := mgr.Init(context.Background()); initErr != nil {
			log.Printf("[device-plugin] WARNING: init camera pod manager: %v", initErr)
		}
		return mgr
	}
	if cfg.ManagerMode == ManagerModePod && clientset == nil {
		log.Printf("[device-plugin] WARNING: camera pod mode requested but no clientset — falling back to local")
	}
	return camera.NewLocalManager(cfg.CtrlBin, cfg.CtrlConfigPath, cfg.CtrCLI)
}

// newROSManager creates the ros-controller manager based on the configured
// mode. If pod mode is selected but the clientset is nil (creation failed),
// it falls back to local mode. Fields left empty in the config are
// auto-filled from the discovered device-plugin pod attributes.
// Returns nil when manager_mode is "disabled" — no controller is launched
// and no config is generated.
func newROSManager(cfg ROSConfig, clientset kubernetes.Interface, disc discoveredPod) ControllerManager {
	if cfg.ManagerMode == ManagerModeDisabled {
		return nil
	}
	if cfg.ManagerMode == ManagerModePod && clientset != nil {
		mgr := ros.NewPodManager(clientset, podmanager.PodOptions{
			Namespace:       cfg.Pod.Namespace,
			NodeName:        envOrDiscovered(cfg.Pod.NodeName, disc.nodeName),
			Image:           cfg.Pod.Image,
			InitImage:       envOrDiscovered(cfg.Pod.InitImage, disc.initImage),
			PodGenerateName: cfg.Pod.PodGenerateName,
			PreCommand:      cfg.Pod.PreCommand,
			ExtraEnv:        coreV1EnvVars(cfg.Pod.ExtraEnv),
			Hostname:        cfg.Pod.Hostname,
			Subdomain:       cfg.Pod.Subdomain,
			Labels:          cfg.Pod.Labels,
			Tolerations:     disc.tolerations,
			OwnerReferences: disc.ownerRefs,
		})
		if initErr := mgr.Init(context.Background()); initErr != nil {
			log.Printf("[device-plugin] WARNING: init ros pod manager: %v", initErr)
		}
		return mgr
	}
	if cfg.ManagerMode == ManagerModePod && clientset == nil {
		log.Printf("[device-plugin] WARNING: ros pod mode requested but no clientset — falling back to local")
	}
	return ros.NewLocalManager(cfg.CtrlBin, cfg.CtrlConfigPath, cfg.CtrCLI)
}

// newROS2Manager creates the ros2-controller manager based on the configured
// mode. Mirrors newROSManager but for ROS 2. Returns nil when manager_mode
// is "disabled".
func newROS2Manager(cfg ROS2Config, clientset kubernetes.Interface, disc discoveredPod) ControllerManager {
	if cfg.ManagerMode == ManagerModeDisabled {
		return nil
	}
	if cfg.ManagerMode == ManagerModePod && clientset != nil {
		mgr := ros2.NewPodManager(clientset, podmanager.PodOptions{
			Namespace:       cfg.Pod.Namespace,
			NodeName:        envOrDiscovered(cfg.Pod.NodeName, disc.nodeName),
			Image:           cfg.Pod.Image,
			InitImage:       envOrDiscovered(cfg.Pod.InitImage, disc.initImage),
			PodGenerateName: cfg.Pod.PodGenerateName,
			PreCommand:      cfg.Pod.PreCommand,
			ExtraEnv:        coreV1EnvVars(cfg.Pod.ExtraEnv),
			Hostname:        cfg.Pod.Hostname,
			Subdomain:       cfg.Pod.Subdomain,
			Labels:          cfg.Pod.Labels,
			Tolerations:     disc.tolerations,
			OwnerReferences: disc.ownerRefs,
		})
		if initErr := mgr.Init(context.Background()); initErr != nil {
			log.Printf("[device-plugin] WARNING: init ros2 pod manager: %v", initErr)
		}
		return mgr
	}
	if cfg.ManagerMode == ManagerModePod && clientset == nil {
		log.Printf("[device-plugin] WARNING: ros2 pod mode requested but no clientset — falling back to local")
	}
	return ros2.NewLocalManager(cfg.CtrlBin, cfg.CtrlConfigPath, cfg.CtrCLI)
}

// ---------------------------------------------------------------------------
// NewPlugin
// ---------------------------------------------------------------------------

// NewPlugin creates a new device plugin, detects devices, and starts the
// ros-controller, ros2-controller, and camera-controller. Each controller
// can run as a local subprocess or as a Kubernetes Pod, configured
// independently via cfg.ROS.ManagerMode, cfg.ROS2.ManagerMode, and
// cfg.Camera.ManagerMode.
//
// whcfg carries the mutating webhook options (CLI-derived). When Enabled and
// host_macvlans is configured, NewServer will start a webhook that injects
// the devinit init container into pods requesting this plugin's resource.
// It also causes a Kubernetes clientset to be created (even with no pod-mode
// controller) so the webhook can manage its caBundle.
func NewPlugin(cfg PluginConfig, whcfg WebhookConfig) *Plugin {
	// Create a Kubernetes clientset if any controller is in pod mode, or if
	// the mutating webhook is enabled (it needs API access for caBundle
	// management and to read the device-plugin's own image via the downward
	// API).
	var clientset kubernetes.Interface
	if cfg.Camera.ManagerMode == ManagerModePod || cfg.ROS.ManagerMode == ManagerModePod || cfg.ROS2.ManagerMode == ManagerModePod || whcfg.Enabled {
		cs, err := podmanager.NewClientset()
		if err != nil {
			log.Printf("[device-plugin] WARNING: create kubernetes clientset: %v — pod-mode controllers and webhook will be disabled", err)
		} else {
			clientset = cs
		}
	}

	// Discover attributes of the current (device-plugin) pod in a single
	// API call and reuse them to fill in any fields the caller left blank:
	//   - ownerRefs   → garbage-collect controller pods with the device-plugin pod
	//   - tolerations → schedule onto the same tainted nodes
	//   - initImage   → reuse the device-plugin image as the init image and
	//                   as the default devinit image for the injected init
	//                   container (the device-plugin image ships devinit)
	//   - nodeName    → pin the controller to the same node
	var disc discoveredPod
	if clientset != nil {
		pod, err := podmanager.CurrentPod(context.Background(), clientset)
		if err != nil {
			log.Printf("[device-plugin] WARNING: discover current pod: %v", err)
		} else {
			disc.ownerRefs = []metav1.OwnerReference{podmanager.OwnerRefFromPod(pod)}
			disc.tolerations = podmanager.TolerationsFromPod(pod)
			if img, err := podmanager.ImageFromPod(pod); err == nil {
				disc.initImage = img
			}
			if node, err := podmanager.NodeNameFromPod(pod); err == nil {
				disc.nodeName = node
			}
		}
	}

	cameraManager := newCameraManager(cfg.Camera, clientset, disc)
	rosManager := newROSManager(cfg.ROS, clientset, disc)
	ros2Manager := newROS2Manager(cfg.ROS2, clientset, disc)

	p := &Plugin{
		cfg:           cfg,
		clientset:     clientset,
		disc:          disc,
		webhookCfg:    whcfg,
		rosManager:    rosManager,
		ros2Manager:   ros2Manager,
		cameraManager: cameraManager,
	}

	// Apply and start ros-controller. Skipped when manager_mode is
	// "disabled" (rosManager == nil) — no config is generated, applied,
	// or started.
	if rosManager != nil {
		if data := generateROSConfig(cfg.ROS); data != nil {
			if err := rosManager.ApplyConfig(context.Background(), data); err != nil {
				log.Printf("[device-plugin] WARNING: apply ros-controller config: %v", err)
			}
			if _, err := rosManager.Maintain(context.Background()); err != nil {
				log.Printf("[device-plugin] WARNING: maintain ros-controller: %v", err)
			}
		}
	} else {
		log.Println("[device-plugin] ros-controller disabled — skipping config and startup")
	}

	// Apply and start ros2-controller. Skipped when manager_mode is
	// "disabled" (ros2Manager == nil).
	if ros2Manager != nil {
		if data := generateROS2Config(cfg.ROS2); data != nil {
			if err := ros2Manager.ApplyConfig(context.Background(), data); err != nil {
				log.Printf("[device-plugin] WARNING: apply ros2-controller config: %v", err)
			}
			if _, err := ros2Manager.Maintain(context.Background()); err != nil {
				log.Printf("[device-plugin] WARNING: maintain ros2-controller: %v", err)
			}
		}
	} else {
		log.Println("[device-plugin] ros2-controller disabled — skipping config and startup")
	}

	// Apply and start camera-controller. Skipped when manager_mode is
	// "disabled" (cameraManager == nil) — no config is generated, so
	// V4L2 auto-detection is also skipped.
	if cameraManager != nil {
		if data := generateCameraConfig(cfg.Camera); data != nil {
			if err := cameraManager.ApplyConfig(context.Background(), data); err != nil {
				log.Printf("[device-plugin] WARNING: apply camera-controller config: %v", err)
			}
			if _, err := cameraManager.Maintain(context.Background()); err != nil {
				log.Printf("[device-plugin] WARNING: maintain camera-controller: %v", err)
			}
		}
	} else {
		log.Println("[device-plugin] camera-controller disabled — skipping config and startup")
	}

	// Detect devices (after managers are started).
	p.devices = p.detectDevices()

	return p
}

// ---------------------------------------------------------------------------
// ListAndWatch — stream device updates to kubelet
// ---------------------------------------------------------------------------

// ListAndWatch sends the current device list to kubelet, then streams updates
// whenever the device inventory changes.
func (p *Plugin) ListAndWatch(stream pluginapi.DevicePlugin_ListAndWatchServer) error {
	// Send the initial device list.
	if err := stream.Send(&pluginapi.ListAndWatchResponse{
		Devices: p.devices,
	}); err != nil {
		return fmt.Errorf("send initial device list: %w", err)
	}

	// Periodically re-scan devices and update the inventory.
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			newDevices := p.detectDevices()
			if !deviceListsEqual(p.devices, newDevices) {
				p.devices = newDevices
				if err := stream.Send(&pluginapi.ListAndWatchResponse{
					Devices: p.devices,
				}); err != nil {
					return fmt.Errorf("send device update: %w", err)
				}
				log.Printf("[device-plugin] device inventory updated: %d devices", len(p.devices))
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Allocate — called by kubelet when a pod requests ROS devices
// ---------------------------------------------------------------------------

// Allocate processes a device allocation request from kubelet.
//
// For each container requesting devices, the response includes:
//   - Mount of the controller socket directory
//   - Mount of the CLI binary directory
func (p *Plugin) Allocate(req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	resp := &pluginapi.AllocateResponse{
		ContainerResponses: make([]*pluginapi.ContainerAllocateResponse, len(req.ContainerRequests)),
	}

	for i := range req.ContainerRequests {
		resp.ContainerResponses[i] = p.allocateContainer(req.ContainerRequests[i])
	}

	return resp, nil
}

// allocateContainer processes allocation for a single container.
//
// The allocation injects:
//   - Environment variables signaling which runtimes are available:
//     RLINF_EMBODIED_RUNTIME_ENABLED is always set; RLINF_EMBODIED_ROS_ENABLED,
//     RLINF_EMBODIED_ROS2_ENABLED, and RLINF_EMBODIED_CAMERA_ENABLED are set
//     only when the corresponding controller manager is enabled (i.e. not in
//     "disabled" mode). RLINF_EMBODIED_{ROS,ROS2,CAMERA}_SOCKET_PATH carry the
//     controller Unix socket paths so CLIs and SDKs can connect without a
//     hard-coded --socket-path. RLINF_EMBODIED_HOST_DEVICES_ENABLED is set
//     when cfg.HostDevices is non-empty, signaling that host /dev/* nodes
//     were mounted.
//   - Socket dir mount — controller sockets for robot/camera lifecycle
//   - Binary dir mount — rosctr/camctr CLI for debugging/probes
//   - Host device specs — /dev/* nodes declared in cfg.HostDevices are
//     passed through directly (no manager involved).
//
// Directories are mounted instead of individual files so that inode changes
// (socket recreation, binary updates) are reflected in the container
// immediately.
func (p *Plugin) allocateContainer(req *pluginapi.ContainerAllocateRequest) *pluginapi.ContainerAllocateResponse {
	log.Printf("[device-plugin] allocate devices: %v", req.DevicesIds)

	envs := map[string]string{
		"RLINF_EMBODIED_RUNTIME_ENABLED": "1",
		"RLINF_EMBODIED_PATH":            BinDir,
	}
	if p.rosManager != nil {
		envs["RLINF_EMBODIED_ROS_ENABLED"] = "1"
		envs["RLINF_EMBODIED_ROS_SOCKET_PATH"] = ROSCtrlSocketPath
		// When exactly one robot is cached with a non-empty ROS_MASTER_URI,
		// inject it directly so ROS tools inside the container can talk to
		// the robot without first querying the controller. With multiple
		// robots the caller must disambiguate via the CLI/socket.
		if robots := p.Robots(); len(robots) == 1 && robots[0].GetRosMasterUri() != "" {
			envs["ROS_MASTER_URI"] = robots[0].GetRosMasterUri()
		}
	}
	if p.ros2Manager != nil {
		envs["RLINF_EMBODIED_ROS2_ENABLED"] = "1"
		envs["RLINF_EMBODIED_ROS2_SOCKET_PATH"] = ROS2CtrlSocketPath
		// When exactly one robot is cached with a non-zero ROS_DOMAIN_ID,
		// inject it directly so ROS 2 tools inside the container talk to
		// the correct DDS domain.
		if robots := p.ROS2Robots(); len(robots) == 1 && robots[0].GetRosDomainId() != 0 {
			envs["ROS_DOMAIN_ID"] = strconv.Itoa(int(robots[0].GetRosDomainId()))
		}
	}
	if p.cameraManager != nil {
		envs["RLINF_EMBODIED_CAMERA_ENABLED"] = "1"
		envs["RLINF_EMBODIED_CAMERA_SOCKET_PATH"] = CameraCtrlSocketPath
	}

	// Host device passthrough — /dev/* nodes declared in cfg.HostDevices
	// are mounted directly. No manager is involved; the device specs are
	// built from the config at allocate time. The env var lets pods detect
	// that host devices were injected.
	hostDeviceSpecs := hostDeviceSpecs(p.cfg.HostDevices)
	if len(hostDeviceSpecs) > 0 {
		envs["RLINF_EMBODIED_HOST_DEVICES_ENABLED"] = "1"
	}

	// Host macvlans are NOT device-mounted (a macvlan must be created
	// inside the pod netns). Instead the device plugin runs an init
	// gRPC service on a Unix socket; a pod's init container runs the
	// `devinit` CLI to request setup. The RunDir mount above already
	// makes the socket reachable, so only the env vars are injected here
	// for the init container / CLI to discover it.
	if len(p.cfg.HostMacvlans) > 0 {
		envs["RLINF_EMBODIED_DEVINIT_ENABLED"] = "1"
		envs["RLINF_EMBODIED_DEVINIT_SOCKET_PATH"] = p.cfg.EffectiveDevinitSocketPath()
	}

	mounts := []*pluginapi.Mount{
		{
			HostPath:      RunDir,
			ContainerPath: RunDir,
			ReadOnly:      true,
		},
		{
			HostPath:      BinDir,
			ContainerPath: BinDir,
			ReadOnly:      true,
		},
	}

	return &pluginapi.ContainerAllocateResponse{
		Envs:    envs,
		Mounts:  mounts,
		Devices: hostDeviceSpecs,
		Annotations: map[string]string{
			"rlinf.io/devices": fmt.Sprintf("%v", req.DevicesIds),
		},
	}
}

// ---------------------------------------------------------------------------
// PreStartContainer (stub)
// ---------------------------------------------------------------------------

// PreStartContainer is called before a container starts.
func (p *Plugin) PreStartContainer(req *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	log.Printf("[device-plugin] PreStartContainer: devices=%v", req.DevicesIds)
	return &pluginapi.PreStartContainerResponse{}, nil
}

// ---------------------------------------------------------------------------
// GetPreferredAllocation (stub)
// ---------------------------------------------------------------------------

// GetPreferredAllocation returns a preferred device allocation for the given
// containers.
func (p *Plugin) GetPreferredAllocation(req *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	log.Printf("[device-plugin] GetPreferredAllocation: %d containers", len(req.ContainerRequests))
	return &pluginapi.PreferredAllocationResponse{
		ContainerResponses: make([]*pluginapi.ContainerPreferredAllocationResponse, len(req.ContainerRequests)),
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// deviceListsEqual compares two device lists for equality.
func deviceListsEqual(a, b []*pluginapi.Device) bool {
	if len(a) != len(b) {
		return false
	}
	byID := make(map[string]*pluginapi.Device, len(a))
	for _, d := range a {
		byID[d.ID] = d
	}
	for _, d := range b {
		existing, ok := byID[d.ID]
		if !ok || existing.Health != d.Health {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Server — gRPC server that implements the DevicePlugin service
// ---------------------------------------------------------------------------

// Server wraps the gRPC server and implements pluginapi.DevicePluginServer.
type Server struct {
	pluginapi.UnimplementedDevicePluginServer

	plugin *Plugin
	srv    *grpc.Server
	socket string

	// deviceSrv serves the on-demand device gRPC service (currently the
	// macvlan setup RPC). nil when the plugin config has no host_macvlans.
	deviceSrv *DeviceServer

	// webhookSrv serves the mutating admission webhook that auto-injects the
	// devinit init container. nil when the webhook is disabled or not
	// configured.
	webhookSrv *mutatingwebhook.Server

	stopCh chan struct{}
}

// NewServer creates a gRPC server for the device plugin. If socketPath is
// empty, PluginSocketPath() is used as the default. When the plugin config
// declares host_macvlans, a DeviceServer is also constructed and started
// alongside the device-plugin gRPC server. When the webhook is enabled (and
// host_macvlans is configured), a mutating webhook server is also constructed
// so it can be started alongside the gRPC server.
func NewServer(plugin *Plugin, socketPath string) *Server {
	if socketPath == "" {
		socketPath = PluginSocketPath()
	}
	s := &Server{
		plugin: plugin,
		socket: socketPath,
		stopCh: make(chan struct{}),
	}
	if deviceSrv := NewDeviceServer(plugin.cfg.HostMacvlans, plugin.cfg.EffectiveDevinitSocketPath()); deviceSrv.HasMacvlans() {
		s.deviceSrv = deviceSrv
	}
	// The webhook is best-effort: a misconfiguration (missing image, no API
	// access) logs a warning and leaves the device plugin running without
	// auto-injection rather than failing the whole process.
	if ws, err := newWebhookServer(plugin); err != nil {
		log.Printf("[device-plugin/webhook] WARNING: webhook disabled: %v", err)
	} else if ws != nil {
		s.webhookSrv = ws
	}
	return s
}

// Start initialises the gRPC server and begins listening on the Unix socket.
func (s *Server) Start() error {
	if err := os.MkdirAll(filepath.Dir(s.socket), 0755); err != nil {
		return fmt.Errorf("create socket dir: %w", err)
	}

	if err := os.Remove(s.socket); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", s.socket)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.socket, err)
	}

	s.srv = grpc.NewServer()
	pluginapi.RegisterDevicePluginServer(s.srv, s)

	go func() {
		<-s.stopCh
		s.srv.GracefulStop()
	}()

	// Start the on-demand macvlan gRPC service alongside the device-plugin
	// server. Runs in its own goroutine on a separate Unix socket; nil
	// when no host_macvlans are configured.
	if s.deviceSrv != nil {
		go func() {
			if err := s.deviceSrv.Start(); err != nil {
				log.Printf("[device-plugin] macvlan service exited: %v", err)
			}
		}()
	}

	// Start the mutating webhook (non-blocking). It serves admission reviews
	// over HTTPS in its own goroutine; Stop shuts it down. nil when the
	// webhook is disabled or not configured.
	if s.webhookSrv != nil {
		if err := s.webhookSrv.Start(); err != nil {
			log.Printf("[device-plugin/webhook] WARNING: start: %v", err)
			s.webhookSrv = nil
		}
	}

	return s.srv.Serve(listener)
}

// Stop gracefully shuts down the gRPC server, the ros-controller,
// ros2-controller, and camera-controller.
func (s *Server) Stop() {
	if s.plugin.rosManager != nil {
		s.plugin.rosManager.Stop(context.Background())
	}
	if s.plugin.ros2Manager != nil {
		s.plugin.ros2Manager.Stop(context.Background())
	}
	if s.plugin.cameraManager != nil {
		s.plugin.cameraManager.Stop(context.Background())
	}
	if s.deviceSrv != nil {
		s.deviceSrv.Stop()
	}
	if s.webhookSrv != nil {
		s.webhookSrv.Stop()
	}
	close(s.stopCh)
}

// ---------------------------------------------------------------------------
// gRPC service methods — delegate to Plugin
// ---------------------------------------------------------------------------

// GetDevicePluginOptions returns device plugin options indicating that
// PreStartContainer and GetPreferredAllocation are not required.
func (s *Server) GetDevicePluginOptions(ctx context.Context, _ *pluginapi.Empty) (*pluginapi.DevicePluginOptions, error) {
	return &pluginapi.DevicePluginOptions{
		PreStartRequired:                false,
		GetPreferredAllocationAvailable: false,
	}, nil
}

// ListAndWatch streams the current device list and subsequent health updates
// to the kubelet.
func (s *Server) ListAndWatch(_ *pluginapi.Empty, stream pluginapi.DevicePlugin_ListAndWatchServer) error {
	return s.plugin.ListAndWatch(stream)
}

// Allocate returns the runtime environment for the requested devices.
func (s *Server) Allocate(ctx context.Context, req *pluginapi.AllocateRequest) (*pluginapi.AllocateResponse, error) {
	return s.plugin.Allocate(req)
}

// PreStartContainer performs any pre-start setup for the requested devices.
func (s *Server) PreStartContainer(ctx context.Context, req *pluginapi.PreStartContainerRequest) (*pluginapi.PreStartContainerResponse, error) {
	return s.plugin.PreStartContainer(req)
}

// GetPreferredAllocation returns the preferred device allocation from the
// available devices.
func (s *Server) GetPreferredAllocation(ctx context.Context, req *pluginapi.PreferredAllocationRequest) (*pluginapi.PreferredAllocationResponse, error) {
	return s.plugin.GetPreferredAllocation(req)
}

// ---------------------------------------------------------------------------
// Kubelet registration
// ---------------------------------------------------------------------------

// RegisterWithKubelet connects to the kubelet device plugin registration
// endpoint and registers this plugin. It retries with exponential backoff
// until kubelet is reachable.
func (s *Server) RegisterWithKubelet() error {
	conn, err := dialKubelet()
	if err != nil {
		return fmt.Errorf("dial kubelet: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := pluginapi.NewRegistrationClient(conn)

	resourceName := s.plugin.ResourceName()

	req := &pluginapi.RegisterRequest{
		Version:      pluginapi.Version,
		Endpoint:     filepath.Base(s.socket),
		ResourceName: resourceName,
		Options: &pluginapi.DevicePluginOptions{
			PreStartRequired:                false,
			GetPreferredAllocationAvailable: false,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.Register(ctx, req)
	if err != nil {
		return fmt.Errorf("register with kubelet: %w", err)
	}

	log.Printf("[device-plugin] registered with kubelet: resource=%s endpoint=%s",
		resourceName, req.Endpoint)
	return nil
}

// dialKubelet connects to the kubelet registration Unix socket with retries.
func dialKubelet() (*grpc.ClientConn, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	backoff := 1 * time.Second
	const maxBackoff = 32 * time.Second

	for {
		//nolint:staticcheck
		conn, err := grpc.DialContext(ctx,
			pluginapi.KubeletSocket,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock(),
			grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", addr)
			}),
		)
		if err == nil {
			return conn, nil
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("kubelet socket not available after 30s: %w", ctx.Err())
		case <-time.After(backoff):
			log.Printf("[device-plugin] kubelet not ready, retrying in %v...", backoff)
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// EnsureSocketDir creates the directory for the plugin socket with correct
// permissions.
func EnsureSocketDir() error {
	return os.MkdirAll(pluginapi.DevicePluginPath, 0755)
}
