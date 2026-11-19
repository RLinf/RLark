package camera

import (
	"k8s.io/client-go/kubernetes"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin/podmanager"
)

// ---------------------------------------------------------------------------
// PodManager — manages the camera-controller as a Kubernetes Pod.
// ---------------------------------------------------------------------------

// Default images and names for the camera-controller pod.
const (
	defaultImage     = "rlinf/camera-base:v0.1.0"
	defaultInitImage = "rlinf/embodied-runtime:v0.1.0"
)

// PodManager wraps podmanager.PodManager with camera-specific defaults.
type PodManager struct {
	*podmanager.PodManager
}

// NewPodManager creates a PodManager for the camera-controller with
// camera-specific defaults. The controller binary (camera-controller) and
// CLI (camctr) are deployed via an init container from the embodied-runtime
// image, and the camera-controller config is base64-decoded onto a shared
// emptyDir.
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
		ControllerBin: "camera-controller",
		CLIBin:        "camctr",

		// Config
		ConfigFileName:  "camera-controller.yaml",
		ConfigMountPath: "/etc/rlinf",

		// Paths
		SocketPath: "/var/run/rlark/camera-ctrl.sock",

		// Scheduling
		NodeName: opts.NodeName,

		// Container command
		ExtraEnv: opts.ExtraEnv,

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
		cfg.PodGenerateName = "camera-controller"
	} else {
		cfg.PodGenerateName = opts.PodGenerateName
	}

	return &PodManager{PodManager: podmanager.NewPodManager(clientset, cfg)}
}
