package ros2

import (
	_ "embed"

	"k8s.io/client-go/kubernetes"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/podmanager"
)

// ---------------------------------------------------------------------------
// PodManager — manages the ros2-controller as a Kubernetes Pod.
// ---------------------------------------------------------------------------

// Default images for the ros2-controller pod.
const (
	defaultImage     = "rlinf/ros2-base:v0.1.0"
	defaultInitImage = "rlinf/embodied-runtime:v0.1.0"
)

// preCommand sources the ROS 2 and colcon workspaces before launching the
// controller binary. Each setup script is sourced only if it exists.
//
//go:embed pre_command.sh
var preCommand string

// PodManager wraps podmanager.PodManager with ros2-specific defaults.
type PodManager struct {
	*podmanager.PodManager
}

// NewPodManager creates a PodManager for the ros2-controller with
// ros2-specific defaults. The controller binary (ros2-controller) and CLI
// (rosctr, shared with ROS 1) are deployed via an init container from the
// embodied-runtime image. The main container sources the ROS 2 + colcon
// workspaces before launching the controller with the config and http-addr
// flags.
func NewPodManager(clientset kubernetes.Interface, opts podmanager.PodOptions) *PodManager {
	cfg := podmanager.Config{
		Namespace:       opts.Namespace,
		PodGenerateName: opts.PodGenerateName,

		Image:               defaultImage,
		ImagePullPolicy:     opts.ImagePullPolicy,
		InitImage:           defaultInitImage,
		InitImagePullPolicy: opts.InitImagePullPolicy,

		ControllerBin: "ros2-controller",
		CLIBin:        "rosctr",

		ConfigFileName:  "ros2-controller.yaml",
		ConfigMountPath: "/etc/rlinf",

		SocketPath: "/var/run/rlark/ros2-ctrl.sock",

		NodeName: opts.NodeName,

		Shell:      "bash",
		PreCommand: preCommand,
		ExtraEnv:   opts.ExtraEnv,

		HostPID:           true,
		Privileged:        true,
		Hostname:          opts.Hostname,
		Subdomain:         opts.Subdomain,
		Labels:            opts.Labels,
		PriorityClassName: "system-cluster-critical",
		Tolerations:       podmanager.DefaultTolerations(),
		OwnerReferences:   opts.OwnerReferences,
	}

	if opts.Tolerations != nil {
		cfg.Tolerations = opts.Tolerations
	}

	if opts.Image != "" {
		cfg.Image = opts.Image
	}
	if opts.InitImage != "" {
		cfg.InitImage = opts.InitImage
	}
	if opts.PreCommand != "" {
		cfg.PreCommand = opts.PreCommand
	}
	if opts.PodGenerateName == "" {
		cfg.PodGenerateName = "ros2-controller"
	} else {
		cfg.PodGenerateName = opts.PodGenerateName
	}

	return &PodManager{PodManager: podmanager.NewPodManager(clientset, cfg)}
}
