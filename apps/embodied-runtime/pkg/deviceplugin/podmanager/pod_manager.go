// Package podmanager implements a ControllerManager that runs the controller
// inside a Kubernetes Pod. The controller configuration is base64-encoded and
// embedded in the init container's command; the init container decodes it
// onto a shared emptyDir volume that the main container mounts read-only.
//
// This mirrors the structure of the example YAML manifests in /examples,
// but replaces the ConfigMap volume with an emptyDir provisioned by the
// init container, avoiding the need for a separate ConfigMap object.
//
// The PodManager structurally satisfies deviceplugin.ControllerManager.
package podmanager

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	// ConfigHashAnnotation stores an MD5 hash of the controller config +
	// PodConfig on the Pod so that Maintain can detect drift without
	// in-memory state.
	ConfigHashAnnotation = "rlinf.io/config-hash"

	// initContainerName is the name of the binary-init initContainer.
	initContainerName = "binary-init"

	// podIPEnvVar is the downward-API env var injected into the main container.
	podIPEnvVar = "POD_IP"

	// Volume names (shared between init and main containers).
	volConfig  = "config"   // emptyDir — holds the decoded controller YAML
	volCtrlBin = "ctrl-bin" // emptyDir — holds binaries copied by init
	volHostBin = "cli-host-bin"
	volSocket  = "socket-dir"

	// defaultPollInterval is the interval used when waiting for pod state.
	defaultPollInterval = 2 * time.Second
	// deleteTimeout is how long to wait for a pod to terminate after deletion.
	deleteTimeout = 60 * time.Second
	// startTimeout is how long to wait for a pod to reach Running.
	startTimeout = 2 * time.Minute

	// defaultReconcileInterval is how often the background reconciler
	// re-checks the pod for deletion or config drift.
	defaultReconcileInterval = 30 * time.Second
	// defaultNotRunningGrace is how long a not-running pod (whose config still
	// matches) is observed before being recreated — avoids thrashing during
	// transient states like image pulls or container restarts.
	defaultNotRunningGrace = 10 * time.Minute
)

// ---------------------------------------------------------------------------
// PodManager
// ---------------------------------------------------------------------------

// PodManager manages the lifecycle of a controller running inside a
// Kubernetes Pod. It creates a Pod whose init container decodes the
// base64-encoded controller YAML onto a shared emptyDir, then launches the
// controller binary in the main container. If the desired config differs
// from the running pod's config — detected via an annotation hash — the pod
// is deleted and recreated.
//
// PodManager structurally satisfies deviceplugin.ControllerManager.
type PodManager struct {
	mu sync.Mutex

	clientset kubernetes.Interface
	config    Config

	// lastConfig is the latest config applied via ApplyConfig.
	lastConfig []byte
	// lastRestartedConfig is the config that the currently-running pod was
	// started with (kept for parity with LocalManager).
	lastRestartedConfig []byte
	// runningHash is the config-hash annotation of the pod detected during
	// Init or the last successful Maintain/Start. It lets the manager know
	// the current pod's config even before ApplyConfig is called.
	runningHash string

	// --- background reconciliation ---
	// reconcileMu serializes reconcile calls (Maintain + the background
	// loop) and guards the fields below. It is always acquired BEFORE mu
	// when both are held (lock ordering: reconcileMu → mu).
	reconcileMu       sync.Mutex
	reconcileCancel   context.CancelFunc
	notRunningSince   time.Time // first time the pod was seen not-running; zero while running
	reconcileInterval time.Duration
	notRunningGrace   time.Duration
}

// NewPodManager creates a PodManager with the given clientset and config.
// Defaults are applied to any zero-valued fields.
func NewPodManager(clientset kubernetes.Interface, cfg Config) *PodManager {
	cfg.applyDefaults()
	return &PodManager{
		clientset:         clientset,
		config:            cfg,
		reconcileInterval: defaultReconcileInterval,
		notRunningGrace:   defaultNotRunningGrace,
	}
}

// Config returns the effective configuration (after defaults were applied).
func (m *PodManager) Config() Config {
	return m.config
}

// ---------------------------------------------------------------------------
// Init
// ---------------------------------------------------------------------------

// Init detects whether a controller Pod already exists in the cluster and,
// if so, records its config hash and running state. This should be called
// once after NewPodManager, before ApplyConfig/Maintain, so that the
// manager starts with knowledge of the current pod — avoiding an
// unnecessary recreate on device-plugin restart.
//
// After Init:
//   - If a running pod exists with a matching config hash (checked later
//     by Maintain against the desired config), Maintain will be a no-op.
//   - If a pod exists but is not running or has a different hash,
//     Maintain will recreate it.
//   - If no pod exists, Maintain will create one.
func (m *PodManager) Init(ctx context.Context) error {
	pod, err := m.clientset.CoreV1().Pods(m.config.Namespace).Get(ctx, m.config.PodName(), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			m.mu.Lock()
			m.runningHash = ""
			m.mu.Unlock()
			log.Printf("[pod-manager] %s Init: no existing pod found", m.config.PodName())
			return nil
		}
		return fmt.Errorf("get pod: %w", err)
	}

	hash := pod.Annotations[ConfigHashAnnotation]
	m.mu.Lock()
	m.runningHash = hash
	m.mu.Unlock()

	if pod.Status.Phase == corev1.PodRunning {
		log.Printf("[pod-manager] %s Init: existing pod running (phase=%s, hash=%s)",
			m.config.PodName(), pod.Status.Phase, hash)
	} else {
		log.Printf("[pod-manager] %s Init: existing pod not running (phase=%s, hash=%s)",
			m.config.PodName(), pod.Status.Phase, hash)
	}

	// Start the background reconciler that watches for pod deletion or config
	// drift. Idempotent — safe to call Init more than once.
	m.startReconciler()
	return nil
}

// ---------------------------------------------------------------------------
// ApplyConfig
// ---------------------------------------------------------------------------

// ApplyConfig stores the controller YAML as the desired configuration. The
// config is embedded (base64-encoded) in the init container's command when
// the pod is (re)created by Start/Maintain. No Kubernetes objects are
// created here — the config lives only in memory until the pod is built.
func (m *PodManager) ApplyConfig(ctx context.Context, config []byte) error {
	m.mu.Lock()
	m.lastConfig = append([]byte(nil), config...)
	m.mu.Unlock()

	log.Printf("[pod-manager] %s config stored (hash=%s, %d bytes)",
		m.config.PodName(), configHash(config, &m.config), len(config))
	return nil
}

// ---------------------------------------------------------------------------
// Maintain
// ---------------------------------------------------------------------------

// Maintain is the on-demand entry point for reconciliation. It delegates to
// the same reconcile logic the background loop uses, so an explicit call
// (e.g. at device-plugin startup) and the periodic check behave identically.
// Returns true when a (re)create was triggered.
func (m *PodManager) Maintain(ctx context.Context) (bool, error) {
	return m.reconcile(ctx)
}

// reconcile is the single source of truth for drift detection and recovery.
// It is serialized by reconcileMu so the background loop and explicit
// Maintain calls never overlap. Detection covers three cases:
//   - Pod missing (deleted manually or otherwise) → recreate.
//   - Pod's config-hash annotation != desired hash (config or PodConfig
//     changed since the pod was built) → recreate.
//   - Pod exists with matching config but not Running → observe within a
//     grace period before recreating, so transient states (image pull,
//     container restart, node hiccup) don't cause thrash.
//
// The hash covers BOTH the controller YAML (lastConfig) AND the full Config
// (pod spec), so changing e.g. the image via the device-plugin config
// invalidates the hash and triggers a recreate.
func (m *PodManager) reconcile(ctx context.Context) (bool, error) {
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()

	if err := ctx.Err(); err != nil {
		return false, nil // shutting down
	}

	m.mu.Lock()
	config := m.lastConfig
	m.mu.Unlock()
	if len(config) == 0 {
		return false, nil // nothing to apply
	}

	desiredHash := configHash(config, &m.config)
	pod, err := m.clientset.CoreV1().Pods(m.config.Namespace).Get(ctx, m.config.PodName(), metav1.GetOptions{})
	switch {
	case apierrors.IsNotFound(err):
		// Pod was deleted — rebuild.
		log.Printf("[pod-manager] %s reconcile: pod missing, recreating", m.config.PodName())
		m.notRunningSince = time.Time{}
		if err := m.Start(ctx); err != nil {
			return false, fmt.Errorf("recreate missing pod: %w", err)
		}
		return true, nil
	case err != nil:
		return false, fmt.Errorf("get pod: %w", err)
	}

	// Pod exists — detect config/spec drift via the annotation hash.
	if pod.Annotations[ConfigHashAnnotation] != desiredHash {
		log.Printf("[pod-manager] %s reconcile: drift (have=%s want=%s), recreating",
			m.config.PodName(), pod.Annotations[ConfigHashAnnotation], desiredHash)
		m.notRunningSince = time.Time{}
		if err := m.Start(ctx); err != nil {
			return false, fmt.Errorf("recreate drifted pod: %w", err)
		}
		return true, nil
	}

	// Config matches — check running state.
	if pod.Status.Phase == corev1.PodRunning {
		m.notRunningSince = time.Time{}
		m.mu.Lock()
		m.lastRestartedConfig = append([]byte(nil), config...)
		m.runningHash = desiredHash
		m.mu.Unlock()
		return false, nil
	}

	// Not running, correct config — observe within a grace period before
	// recreating, so transient states don't thrash.
	since := m.notRunningSince
	if since.IsZero() {
		since = time.Now()
		m.notRunningSince = since
	}
	elapsed := time.Since(since)
	if elapsed < m.notRunningGrace {
		log.Printf("[pod-manager] %s reconcile: not running (phase=%s), observing %.0fs/%.0fs",
			m.config.PodName(), pod.Status.Phase, elapsed.Seconds(), m.notRunningGrace.Seconds())
		return false, nil
	}

	log.Printf("[pod-manager] %s reconcile: not running for %.0fs, recreating",
		m.config.PodName(), elapsed.Seconds())
	m.notRunningSince = time.Time{}
	if err := m.Start(ctx); err != nil {
		return false, fmt.Errorf("recreate stale not-running pod: %w", err)
	}
	return true, nil
}

// startReconciler launches the background reconcile loop, idempotently. The
// loop uses its own (cancellable) context so it is not tied to any request
// context; Stop cancels it. Tick reconciliation is best-effort — errors are
// logged, not fatal.
func (m *PodManager) startReconciler() {
	m.reconcileMu.Lock()
	defer m.reconcileMu.Unlock()
	if m.reconcileCancel != nil {
		return // already running
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.reconcileCancel = cancel
	go m.reconcileLoop(ctx)
}

// reconcileLoop periodically calls reconcile until its context is cancelled.
func (m *PodManager) reconcileLoop(ctx context.Context) {
	t := time.NewTicker(m.reconcileInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if _, err := m.reconcile(ctx); err != nil {
				log.Printf("[pod-manager] %s reconcile: %v", m.config.PodName(), err)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Start
// ---------------------------------------------------------------------------

// Start creates (or recreates) the controller Pod. Any existing pod with
// the same name is deleted first. The method blocks until the pod reaches
// the Running phase or the context is cancelled.
func (m *PodManager) Start(ctx context.Context) error {
	m.mu.Lock()
	config := m.lastConfig
	m.mu.Unlock()

	if len(config) == 0 {
		return fmt.Errorf("no config applied; call ApplyConfig first")
	}

	// Delete any leftover pod with the same name (e.g. from a failed start
	// or a previous incarnation).
	if err := m.deletePod(ctx); err != nil {
		return fmt.Errorf("delete existing pod: %w", err)
	}

	// Create the pod.
	pod := m.buildPod(config)
	if _, err := m.clientset.CoreV1().Pods(m.config.Namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create pod %s: %w", m.config.PodName(), err)
	}

	// Wait for the pod to reach Running.
	if err := m.waitForPodRunning(ctx); err != nil {
		return fmt.Errorf("wait for pod %s running: %w", m.config.PodName(), err)
	}

	m.mu.Lock()
	m.lastRestartedConfig = append([]byte(nil), config...)
	m.runningHash = configHash(config, &m.config)
	m.mu.Unlock()

	log.Printf("[pod-manager] %s pod started", m.config.PodName())
	return nil
}

// ---------------------------------------------------------------------------
// Stop
// ---------------------------------------------------------------------------

// Stop deletes the controller Pod, waiting up to 60 s for it to terminate.
// The background reconciler is stopped first so it does not recreate the
// pod out from under us.
func (m *PodManager) Stop(ctx context.Context) {
	// Stop the background reconciler and reset reconcile state.
	m.reconcileMu.Lock()
	if m.reconcileCancel != nil {
		m.reconcileCancel()
		m.reconcileCancel = nil
	}
	m.notRunningSince = time.Time{}
	m.reconcileMu.Unlock()

	if err := m.deletePod(ctx); err != nil {
		log.Printf("[pod-manager] %s stop: delete pod: %v", m.config.PodName(), err)
		return
	}
	m.mu.Lock()
	m.lastRestartedConfig = nil
	m.runningHash = ""
	m.mu.Unlock()
	log.Printf("[pod-manager] %s pod stopped", m.config.PodName())
}

// ---------------------------------------------------------------------------
// IsRunning
// ---------------------------------------------------------------------------

// IsRunning returns whether the controller Pod exists, is in the Running
// phase, and reports the Ready condition. Checking Ready (in addition to
// Phase) avoids treating a Pod whose containers are still starting or whose
// readiness probe is failing as available to serve requests.
func (m *PodManager) IsRunning(ctx context.Context) bool {
	pod, err := m.clientset.CoreV1().Pods(m.config.Namespace).Get(ctx, m.config.PodName(), metav1.GetOptions{})
	if err != nil {
		return false
	}
	return pod.Status.Phase == corev1.PodRunning && isPodReady(pod)
}

// isPodReady reports whether the pod's PodReady condition is True.
func isPodReady(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Cluster helpers
// ---------------------------------------------------------------------------

// deletePod deletes the pod named in the config (if it exists) and waits
// for it to be fully removed from the API server so that a subsequent
// Create with the same name succeeds.
func (m *PodManager) deletePod(ctx context.Context) error {
	pods := m.clientset.CoreV1().Pods(m.config.Namespace)

	_, err := pods.Get(ctx, m.config.PodName(), metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("get pod: %w", err)
	}

	// Issue the deletion.
	graceful := int64(10)
	if err := pods.Delete(ctx, m.config.PodName(), metav1.DeleteOptions{
		GracePeriodSeconds: &graceful,
	}); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("delete pod: %w", err)
	}

	// Wait for the pod to disappear.
	return wait.PollUntilContextTimeout(ctx, defaultPollInterval, deleteTimeout, false,
		func(ctx context.Context) (bool, error) {
			_, err := pods.Get(ctx, m.config.PodName(), metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			if err != nil {
				return false, err
			}
			return false, nil
		})
}

// waitForPodRunning polls the pod status until it reaches Running or Failed,
// or the timeout elapses.
func (m *PodManager) waitForPodRunning(ctx context.Context) error {
	pods := m.clientset.CoreV1().Pods(m.config.Namespace)
	var lastPhase corev1.PodPhase

	err := wait.PollUntilContextTimeout(ctx, defaultPollInterval, startTimeout, false,
		func(ctx context.Context) (bool, error) {
			pod, err := pods.Get(ctx, m.config.PodName(), metav1.GetOptions{})
			if err != nil {
				if apierrors.IsNotFound(err) {
					return false, fmt.Errorf("pod disappeared during startup")
				}
				return false, err
			}
			lastPhase = pod.Status.Phase
			switch pod.Status.Phase {
			case corev1.PodRunning:
				return true, nil
			case corev1.PodFailed:
				return false, fmt.Errorf("pod failed to start (phase=%s)", pod.Status.Phase)
			default:
				return false, nil
			}
		})

	if err != nil && lastPhase != "" {
		return fmt.Errorf("%w (last phase=%s)", err, lastPhase)
	}
	return err
}

// ---------------------------------------------------------------------------
// Pod builder
// ---------------------------------------------------------------------------

// buildPod constructs the full Pod spec. The init container:
//  1. Decodes the base64-encoded controller YAML onto the shared config
//     emptyDir at <ConfigMountPath>/<ConfigFileName>.
//  2. Copies the controller and CLI binaries from the init image into the
//     shared bin emptyDir and onto the host.
//
// The main container runs the controller binary with the config flag and
// mounts the config + bin emptyDirs and the socket hostPath.
func (m *PodManager) buildPod(config []byte) *corev1.Pod {
	c := &m.config

	configPath := c.ConfigMountPath + "/" + c.ConfigFileName
	b64Config := base64.StdEncoding.EncodeToString(config)

	// --- init container script ---
	// Decode config, then deploy binaries.
	initScript := fmt.Sprintf(`set -e
printf '%%s' '%[1]s' | base64 -d > %[2]s
cp /usr/local/bin/%[3]s %[4]s/%[3]s
cp /usr/local/bin/%[5]s %[4]s/%[5]s
cp /usr/local/bin/%[5]s /host-bin/%[5]s
echo "init complete"`,
		b64Config,       // %[1]s — base64 config
		configPath,      // %[2]s — config file path
		c.ControllerBin, // %[3]s
		c.BinDir,        // %[4]s
		c.CLIBin)        // %[5]s

	// --- main container command ---
	controllerCmd := fmt.Sprintf("%s/%s --config=%s", c.BinDir, c.ControllerBin, configPath)
	if len(c.ExtraArgs) > 0 {
		controllerCmd += " " + strings.Join(c.ExtraArgs, " ")
	}
	var mainScript string
	if strings.TrimSpace(c.PreCommand) != "" {
		mainScript = c.PreCommand + "\n" + controllerCmd
	} else {
		mainScript = controllerCmd
	}

	// --- probe command ---
	probeCmd := []string{
		c.BinDir + "/" + c.CLIBin,
		"--socket-path=" + c.SocketPath,
		"list",
	}

	privileged := c.Privileged

	// Build env vars: POD_IP downward API + any extra envs.
	envVars := append([]corev1.EnvVar{
		{
			Name: podIPEnvVar,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
	}, c.ExtraEnv...)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            c.PodName(),
			Namespace:       c.Namespace,
			OwnerReferences: c.OwnerReferences,
			Labels:          c.Labels,
			Annotations: map[string]string{
				ConfigHashAnnotation: configHash(config, c),
				"description":        "managed by embodied-runtime device-plugin",
			},
		},
		Spec: corev1.PodSpec{
			HostPID:           c.HostPID,
			Hostname:          c.Hostname,
			Subdomain:         c.Subdomain,
			PriorityClassName: c.PriorityClassName,
			RestartPolicy:     corev1.RestartPolicyAlways,

			InitContainers: []corev1.Container{
				{
					Name:            initContainerName,
					Image:           c.InitImage,
					ImagePullPolicy: c.InitImagePullPolicy,
					Command:         []string{c.InitShell, "-c"},
					Args:            []string{initScript},
					VolumeMounts: []corev1.VolumeMount{
						{Name: volConfig, MountPath: c.ConfigMountPath},
						{Name: volCtrlBin, MountPath: c.BinDir},
						{Name: volHostBin, MountPath: "/host-bin"},
					},
				},
			},

			Containers: []corev1.Container{
				{
					Name:            c.ControllerBin,
					Image:           c.Image,
					ImagePullPolicy: c.ImagePullPolicy,
					Command:         []string{c.Shell, "-c"},
					Args:            []string{mainScript},
					Env:             envVars,
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
					VolumeMounts: []corev1.VolumeMount{
						{Name: volConfig, MountPath: c.ConfigMountPath, ReadOnly: true},
						{Name: volCtrlBin, MountPath: c.BinDir},
						{Name: volSocket, MountPath: c.SocketDir},
					},
					LivenessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{Command: probeCmd},
						},
						InitialDelaySeconds: c.LivenessInitialDelay,
						PeriodSeconds:       c.LivenessPeriod,
					},
					ReadinessProbe: &corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{Command: probeCmd},
						},
						InitialDelaySeconds: c.ReadinessInitialDelay,
						PeriodSeconds:       c.ReadinessPeriod,
					},
				},
			},

			Volumes: []corev1.Volume{
				{
					Name: volConfig,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: volCtrlBin,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
				{
					Name: volHostBin,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: c.HostBinDir,
							Type: dirOrCreatePtr(),
						},
					},
				},
				{
					Name: volSocket,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: c.SocketDir,
							Type: dirOrCreatePtr(),
						},
					},
				},
			},

			Tolerations: c.Tolerations,
		},
	}

	// Schedule onto a specific node via nodeSelector (matching the
	// kubernetes.io/hostname label) when NodeName is configured.
	if c.NodeName != "" {
		pod.Spec.NodeSelector = map[string]string{
			"kubernetes.io/hostname": c.NodeName,
		}
	}

	return pod
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

// configHash returns the hex-encoded MD5 of the controller config bytes AND
// the full PodConfig (Config struct). Including the pod spec means changing
// e.g. the image or env via the device-plugin config invalidates the hash and
// triggers a recreate. MD5 is sufficient here — this is a non-adversarial
// drift-detection checksum, not a cryptographic primitive. The Config is
// JSON-marshaled, which is deterministic for our fields (map keys sorted by
// encoding/json).
func configHash(config []byte, cfg *Config) string {
	h := md5.New()
	h.Write(config)
	if cfg != nil {
		if b, err := json.Marshal(cfg); err == nil {
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// dirOrCreatePtr returns a pointer to HostPathDirectoryOrCreate.
func dirOrCreatePtr() *corev1.HostPathType {
	t := corev1.HostPathDirectoryOrCreate
	return &t
}
