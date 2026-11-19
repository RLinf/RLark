package podmanager

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ---------------------------------------------------------------------------
// Config — all knobs for building a controller Pod.
// ---------------------------------------------------------------------------

// Config holds every configurable aspect of the controller pod. The
// camera and ros sub-packages each provide a constructor that fills in
// controller-specific defaults (binary names, images, socket paths, …);
// callers can then override fields such as NodeName or Image.
type Config struct {
	// --- Identity ---
	Namespace       string // Kubernetes namespace (default "default")
	PodGenerateName string // base name of the controller pod; the actual pod name is derived as "<PodGenerateName>-<NodeName>" via PodName()

	// --- Images ---
	Image               string            // main container image
	ImagePullPolicy     corev1.PullPolicy // pull policy for main container
	InitImage           string            // init container image (embodied-runtime)
	InitImagePullPolicy corev1.PullPolicy // pull policy for init container

	// --- Controller binary ---
	// ControllerBin is the controller binary name as it exists in the init
	// image at /usr/local/bin/<ControllerBin> (e.g. "camera-controller").
	ControllerBin string
	// CLIBin is the CLI binary name (e.g. "camctr"), used for probes and
	// copied to the host for operator use.
	CLIBin string

	// --- Config mounting ---
	// ConfigFileName is the file name written onto the shared config
	// emptyDir by the init container (e.g. "camera-controller.yaml").
	ConfigFileName string
	// ConfigMountPath is where the config emptyDir is mounted in the
	// container (e.g. "/etc/rlinf").
	ConfigMountPath string

	// --- Paths ---
	// SocketPath is the Unix socket path used by the CLI for health probes
	// (e.g. "/var/run/rlark/camera-ctrl.sock").
	SocketPath string
	// SocketDir is the hostPath directory for sockets (default "/var/run/rlark").
	SocketDir string
	// BinDir is the in-container shared directory for binaries, mounted from
	// an emptyDir that the initContainer populates (default "/opt/rlinf/bin").
	BinDir string
	// HostBinDir is the hostPath directory for CLI binaries that operators
	// can use on the host (default "/opt/rlinf/bin").
	HostBinDir string

	// --- Scheduling ---
	// NodeName is the value used for spec.nodeSelector's
	// kubernetes.io/hostname key — the pod is scheduled onto the node
	// with this hostname. If empty, no nodeSelector is set and the
	// scheduler decides.
	NodeName string

	// --- Container command ---
	// Shell is the shell used to run the main container's command
	// ("sh" or "bash"). The ros-controller and ros2-controller set this
	// to "bash" so they can source ROS workspace scripts before launching
	// the binary.
	Shell string
	// InitShell is the shell used to run the init container's command.
	// Defaults to "sh" since the init image (embodied-runtime, Alpine)
	// only ships sh and the init script needs no bash features. Kept
	// separate from Shell so a bash main container does not force bash on
	// the init container.
	InitShell string
	// PreCommand is a shell snippet executed before the controller binary
	// (used by ros-controller / ros2-controller to source ROS environment).
	PreCommand string
	// ExtraArgs are additional command-line arguments passed to the
	// controller binary (e.g. ["--http-addr=:8080"]).
	ExtraArgs []string
	// ExtraEnv are additional environment variables for the main container.
	ExtraEnv []corev1.EnvVar

	// --- Pod spec ---
	HostPID           bool                // share host PID namespace (default true)
	Privileged        bool                // run main container privileged (default true)
	PriorityClassName string              // (default "system-cluster-critical")
	Tolerations       []corev1.Toleration // tolerations applied to the pod

	// --- Headless-service DNS ---
	// Hostname sets pod.spec.hostname. When Subdomain is also set and a
	// headless Service named <Subdomain> exists in the namespace, the pod
	// is reachable at "<Hostname>.<Subdomain>.<namespace>.svc.cluster.local".
	// When Subdomain is set but Hostname is left empty, applyDefaults
	// derives Hostname from the pod's deterministic name (PodName()) so a
	// stable per-pod A record is still generated. Empty Subdomain leaves
	// Hostname unset (no stable FQDN via a headless Service).
	// The headless Service itself is NOT created by the device plugin — it
	// is expected to be deployed alongside the device plugin (see the
	// deploy manifests); these fields only configure the pod side.
	Hostname string
	// Subdomain sets pod.spec.subdomain. MUST equal the name of the
	// headless Service that routes to these pods. Empty leaves subdomain
	// unset (no stable FQDN via a headless Service).
	Subdomain string

	// --- Labels & Annotations ---
	// Labels is the sole source of the pod's label set — there are no
	// built-in labels anymore (AppLabel and its derived app/app.kubernetes.io
	// labels were removed). The operator sets whatever labels the workload
	// needs here, including the selector a headless Service uses to route to
	// these pods (e.g. app.kubernetes.io/name: camera-controller). Empty/nil
	// means the pod has no labels.
	Labels map[string]string

	// OwnerReferences, if set, are applied to the created Pod's ObjectMeta.
	// This allows the pod to be garbage-collected when its owner (typically
	// the device-plugin's DaemonSet) is deleted. Use DiscoverOwnerRef to
	// automatically discover the controlling owner from the current pod.
	OwnerReferences []metav1.OwnerReference

	// --- Probe tuning ---
	LivenessInitialDelay  int32 // seconds before liveness probe starts (default 10)
	LivenessPeriod        int32 // liveness probe interval (default 15)
	ReadinessInitialDelay int32 // seconds before readiness probe starts (default 5)
	ReadinessPeriod       int32 // readiness probe interval (default 10)
}

// PodOptions is the user-configurable subset of a controller pod's spec —
// the fields an operator (or the device plugin) supplies when asking for a
// controller pod to be created. It is shared by every controller type
// (camera, ros, …); controller-specific defaults (image, binary name, shell,
// pre-command, env) are filled in by the per-controller NewPodManager
// constructors and are NOT part of this struct. Fields left zero fall back
// to those constructor defaults.
type PodOptions struct {
	// Namespace defaults to "default".
	Namespace string
	// NodeName pins the pod to a specific node (spec.nodeName).
	NodeName string
	// Image overrides the main container image.
	Image string
	// InitImage overrides the binary-init image.
	InitImage string
	// ImagePullPolicy for the main container (default Always).
	ImagePullPolicy corev1.PullPolicy
	// PodGenerateName overrides the pod generate name. The actual pod name
	// is derived as "<PodGenerateName>-<NodeName>".
	PodGenerateName string
	// InitImagePullPolicy for the init container (default Always).
	InitImagePullPolicy corev1.PullPolicy
	// PreCommand overrides the shell snippet run before the controller
	// binary. Each controller constructor supplies its own default
	// (ros-controller sources the ROS 1 + catkin workspaces; ros2-controller
	// sources the ROS 2 + colcon workspaces; camera-controller has none).
	// Empty keeps that constructor default; a non-empty value fully
	// replaces it, so for ros/ros2 you must include the ROS workspace
	// sourcing yourself if you still need it.
	PreCommand string
	// ExtraEnv are additional environment variables for the main container.
	// Neither controller sets any by default; set this to inject workspace
	// or runtime env vars (e.g. ROS_PACKAGE_PATH).
	ExtraEnv []corev1.EnvVar
	// Hostname sets pod.spec.hostname for headless-service DNS. See
	// Config.Hostname.
	Hostname string
	// Subdomain sets pod.spec.subdomain (must match a headless Service
	// name). See Config.Subdomain.
	Subdomain string
	// Labels is the sole source of the pod's label set. See Config.Labels.
	Labels map[string]string
	// Tolerations, if set, override the default tolerations. Use
	// DiscoverTolerations to auto-discover the device-plugin pod's
	// tolerations so the controller lands on the same tainted nodes.
	Tolerations []corev1.Toleration
	// OwnerReferences, if set, are applied to the created pod. Use
	// DiscoverOwnerRef to auto-discover the controlling owner (e.g. the
	// device-plugin's DaemonSet) so the pod is garbage-collected when the
	// owner is deleted.
	OwnerReferences []metav1.OwnerReference
}

// PodName returns the actual pod name, derived from PodGenerateName and
// NodeName as "<PodGenerateName>-<NodeName>". If NodeName is empty,
// PodGenerateName is used as-is.
func (c Config) PodName() string {
	if c.NodeName != "" {
		return c.PodGenerateName + "-" + c.NodeName
	}
	return c.PodGenerateName
}

// applyDefaults fills in zero-valued fields with sensible defaults. Fields
// that are already set (including by the camera/ros constructors) are left
// untouched.
func (c *Config) applyDefaults() {
	if c.Namespace == "" {
		c.Namespace = "default"
	}
	if c.ImagePullPolicy == "" {
		c.ImagePullPolicy = corev1.PullAlways
	}
	if c.InitImagePullPolicy == "" {
		c.InitImagePullPolicy = corev1.PullAlways
	}
	if c.ConfigMountPath == "" {
		c.ConfigMountPath = "/etc/rlinf"
	}
	if c.SocketDir == "" {
		c.SocketDir = "/var/run/rlark"
	}
	if c.BinDir == "" {
		c.BinDir = "/opt/rlinf/bin"
	}
	if c.HostBinDir == "" {
		c.HostBinDir = "/opt/rlinf/bin"
	}
	if c.Shell == "" {
		c.Shell = "sh"
	}
	if c.InitShell == "" {
		c.InitShell = "sh"
	}
	if c.PriorityClassName == "" {
		c.PriorityClassName = "system-cluster-critical"
	}
	if c.LivenessInitialDelay == 0 {
		c.LivenessInitialDelay = 10
	}
	if c.LivenessPeriod == 0 {
		c.LivenessPeriod = 15
	}
	if c.ReadinessInitialDelay == 0 {
		c.ReadinessInitialDelay = 5
	}
	if c.ReadinessPeriod == 0 {
		c.ReadinessPeriod = 10
	}
	// HeadPID and Privileged default to false in Go's zero value, but the
	// examples set them to true. Callers (camera/ros constructors) set them
	// explicitly, so we only set them here if the caller left them at zero.
	// We can't distinguish "false" from "unset" for bools, so we leave them
	// as-is and let constructors set them.

	// Headless-service DNS: when subdomain is set (so the pod is meant to be
	// reachable via a headless Service), default hostname to the pod's
	// deterministic name if the operator left it empty. This guarantees a
	// stable per-pod A record <hostname>.<subdomain>.<ns>.svc.cluster.local
	// even without an explicit hostname — the readable per-pod record needs
	// pod.spec.hostname to be set, so we make it explicit from PodName().
	// Without subdomain there is no per-pod record regardless, so hostname
	// stays empty (the pod falls back to its name as the in-container
	// hostname, which is the same value).
	if c.Subdomain != "" && c.Hostname == "" {
		c.Hostname = c.PodName()
	}
}

// DefaultTolerations returns the toleration set used in the example YAMLs
// (rlinf.io/robot:Exists:NoSchedule). Callers can use it directly or
// append their own.
func DefaultTolerations() []corev1.Toleration {
	return []corev1.Toleration{
		{
			Key:      "rlinf.io/robot",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}
}
