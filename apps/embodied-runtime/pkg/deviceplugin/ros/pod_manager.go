package ros

import (
	_ "embed"

	"k8s.io/client-go/kubernetes"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/podmanager"
)

// ---------------------------------------------------------------------------
// PodManager — manages the ros-controller as a Kubernetes Pod
// ---------------------------------------------------------------------------

// Default images for the ros-controller pod.
const (
	defaultImage     = "rlinf/ros-base:v0.1.0"
	defaultInitImage = "rlinf/embodied-runtime:v0.1.0"
)

// preCommand sources the ROS and catkin workspaces before launching the
// controller binary. Each setup script is sourced only if it exists, so
// a missing workspace (e.g. devel vs. devel_isolated) does not break
// startup. It runs inside a bash shell.
//
//go:embed pre_command.sh
var preCommand string

// PodManager wraps podmanager.PodManager with ros-specific defaults.
type PodManager struct {
	*podmanager.PodManager
}

// NewPodManager creates a PodManager for the ros-controller with
// ros-specific defaults. The controller binary (ros-controller) and CLI
// (rosctr) are deployed via an init container from the embodied-runtime
// image. The main container sources the ROS + catkin workspaces before
// launching the controller with the config and http-addr flags.
func NewPodManager(clientset kubernetes.Interface, opts podmanager.PodOptions) *PodManager {
	cfg := podmanager.Config{
		// Identity
		Namespace:       opts.Namespace,
		PodGenerateName: opts.PodGenerateName,

		// Images
		Image:               defaultImage,
		ImagePullPolicy:     opts.ImagePullPolicy,
		InitImage:           defaultInitImage,
		InitImagePullPolicy: opts.InitImagePullPolicy,

		// Controller binary
		ControllerBin: "ros-controller",
		CLIBin:        "rosctr",

		// Config
		ConfigFileName:  "ros-controller.yaml",
		ConfigMountPath: "/etc/rlinf",

		// Paths
		SocketPath: "/var/run/rlark/ros-ctrl.sock",

		// Scheduling
		NodeName: opts.NodeName,

		// Container command — bash with ROS workspace sourcing.
		Shell:      "bash",
		PreCommand: preCommand,
		ExtraEnv:   opts.ExtraEnv,

		// Pod spec
		HostPID:           true,
		Privileged:        true,
		Hostname:          opts.Hostname,
		Subdomain:         opts.Subdomain,
		Labels:            opts.Labels,
		PriorityClassName: "system-cluster-critical",
		Tolerations:       podmanager.DefaultTolerations(),
		OwnerReferences:   opts.OwnerReferences,
	}

	// Apply user override for tolerations.
	if opts.Tolerations != nil {
		cfg.Tolerations = opts.Tolerations
	}

	// Apply user overrides for images and naming.
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
		cfg.PodGenerateName = "ros-controller"
	} else {
		cfg.PodGenerateName = opts.PodGenerateName
	}

	return &PodManager{PodManager: podmanager.NewPodManager(clientset, cfg)}
}
