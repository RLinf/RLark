package podmanager

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ---------------------------------------------------------------------------
// buildPod tests
// ---------------------------------------------------------------------------

// TestBuildPod_CameraLike verifies the pod structure for a camera-like
// config: correct init container, main container, volumes, probes, and
// node scheduling.
func TestBuildPod_CameraLike(t *testing.T) {
	cfg := Config{
		Namespace:       "default",
		PodGenerateName: "camera-controller",
		Image:           "rlinf/camera-base:v0.1.0",
		InitImage:       "rlinf/embodied-runtime:v0.1.0",
		ControllerBin:   "camera-controller",
		CLIBin:          "camctr",
		ConfigFileName:  "camera-controller.yaml",
		ConfigMountPath: "/etc/rlinf",
		SocketPath:      "/var/run/rlark/camera-ctrl.sock",
		NodeName:        "your-node-name",
		Shell:           "sh",
		HostPID:         true,
		Privileged:      true,
		Tolerations:     DefaultTolerations(),
	}
	cfg.applyDefaults()

	mgr := NewPodManager(fake.NewSimpleClientset(), cfg)
	config := []byte("cameras:\n- id: cam0\n  camera_type: v4l2\n")
	pod := mgr.buildPod(config)

	// --- metadata ---
	if pod.Name != "camera-controller-your-node-name" {
		t.Errorf("pod name = %q, want camera-controller-your-node-name", pod.Name)
	}
	if pod.Namespace != "default" {
		t.Errorf("namespace = %q", pod.Namespace)
	}
	if pod.Annotations[ConfigHashAnnotation] == "" {
		t.Error("config hash annotation missing")
	}
	if pod.Spec.NodeSelector["kubernetes.io/hostname"] != "your-node-name" {
		t.Errorf("nodeSelector hostname = %q, want your-node-name",
			pod.Spec.NodeSelector["kubernetes.io/hostname"])
	}
	if !pod.Spec.HostPID {
		t.Error("hostPID should be true")
	}
	if pod.Spec.PriorityClassName != "system-cluster-critical" {
		t.Errorf("priorityClassName = %q", pod.Spec.PriorityClassName)
	}

	// --- init container ---
	if len(pod.Spec.InitContainers) != 1 {
		t.Fatalf("init containers = %d, want 1", len(pod.Spec.InitContainers))
	}
	initC := pod.Spec.InitContainers[0]
	if initC.Name != "binary-init" {
		t.Errorf("init container name = %q", initC.Name)
	}
	if initC.Image != "rlinf/embodied-runtime:v0.1.0" {
		t.Errorf("init image = %q", initC.Image)
	}
	// init container should mount config, ctrl-bin, cli-host-bin
	volNames := volumeMountNames(initC.VolumeMounts)
	for _, want := range []string{volConfig, volCtrlBin, volHostBin} {
		if !volNames[want] {
			t.Errorf("init container missing volume mount %q (have %v)", want, volNames)
		}
	}

	// --- main container ---
	if len(pod.Spec.Containers) != 1 {
		t.Fatalf("containers = %d, want 1", len(pod.Spec.Containers))
	}
	mainC := pod.Spec.Containers[0]
	if mainC.Name != "camera-controller" {
		t.Errorf("main container name = %q", mainC.Name)
	}
	if mainC.Image != "rlinf/camera-base:v0.1.0" {
		t.Errorf("main image = %q", mainC.Image)
	}
	if mainC.SecurityContext == nil || mainC.SecurityContext.Privileged == nil || !*mainC.SecurityContext.Privileged {
		t.Error("main container should be privileged")
	}
	// config mount should be readOnly
	for _, vm := range mainC.VolumeMounts {
		if vm.Name == volConfig && !vm.ReadOnly {
			t.Error("config volume should be readOnly in main container")
		}
	}
	// probes
	if mainC.LivenessProbe == nil || mainC.LivenessProbe.Exec == nil {
		t.Error("liveness probe missing")
	}
	if mainC.ReadinessProbe == nil || mainC.ReadinessProbe.Exec == nil {
		t.Error("readiness probe missing")
	}
	probeCmd := mainC.LivenessProbe.Exec.Command
	wantProbe := []string{"/opt/rlinf/bin/camctr", "--socket-path=/var/run/rlark/camera-ctrl.sock", "list"}
	if len(probeCmd) != 3 || probeCmd[0] != wantProbe[0] || probeCmd[1] != wantProbe[1] || probeCmd[2] != wantProbe[2] {
		t.Errorf("liveness probe cmd = %v, want %v", probeCmd, wantProbe)
	}

	// --- volumes ---
	volMap := volumeMap(pod.Spec.Volumes)
	for _, want := range []string{volConfig, volCtrlBin, volHostBin, volSocket} {
		if _, ok := volMap[want]; !ok {
			t.Errorf("missing volume %q", want)
		}
	}
	if volMap[volConfig].EmptyDir == nil {
		t.Error("config volume should be emptyDir")
	}
	if volMap[volCtrlBin].EmptyDir == nil {
		t.Error("ctrl-bin volume should be emptyDir")
	}
	if volMap[volHostBin].HostPath == nil {
		t.Error("cli-host-bin volume should be hostPath")
	}
	if volMap[volSocket].HostPath == nil {
		t.Error("socket-dir volume should be hostPath")
	}

	// --- tolerations ---
	if len(pod.Spec.Tolerations) != 1 {
		t.Fatalf("tolerations = %d, want 1", len(pod.Spec.Tolerations))
	}
	tol := pod.Spec.Tolerations[0]
	if tol.Key != "rlinf.io/robot" || string(tol.Operator) != "Exists" || string(tol.Effect) != "NoSchedule" {
		t.Errorf("toleration = %+v", tol)
	}
}

// TestBuildPod_RosLike verifies the pod structure for a ros-like config,
// including the bash shell, pre-command sourcing, extra args, and extra env.
func TestBuildPod_RosLike(t *testing.T) {
	cfg := Config{
		Namespace:       "default",
		PodGenerateName: "ros-controller",
		Image:           "rlinf/serl_franka_controllers:v0.1.0",
		InitImage:       "rlinf/embodied-runtime:v0.1.0",
		ControllerBin:   "ros-controller",
		CLIBin:          "rosctr",
		ConfigFileName:  "ros-controller.yaml",
		ConfigMountPath: "/etc/rlinf",
		SocketPath:      "/var/run/rlark/ros-ctrl.sock",
		NodeName:        "your-node-name",
		Shell:           "bash",
		PreCommand:      "source /opt/ros/noetic/setup.bash\nsource /catkin_ws/devel_isolated/setup.bash",
		ExtraArgs:       []string{"--http-addr=:8080"},
		ExtraEnv: []corev1.EnvVar{
			{Name: "ROS_PACKAGE_PATH", Value: "/catkin_ws/devel_isolated/share"},
			{Name: "CMAKE_PREFIX_PATH", Value: "/catkin_ws/devel_isolated"},
		},
		HostPID:     true,
		Privileged:  true,
		Tolerations: DefaultTolerations(),
	}
	cfg.applyDefaults()

	mgr := NewPodManager(fake.NewSimpleClientset(), cfg)
	config := []byte("robots:\n- id: franka-1\n  type: franka\n")
	pod := mgr.buildPod(config)

	mainC := pod.Spec.Containers[0]

	// Command should use bash.
	if len(mainC.Command) < 1 || mainC.Command[0] != "bash" {
		t.Errorf("shell = %v, want bash", mainC.Command)
	}

	// Init container must use sh (the init image is Alpine, no bash) even
	// though the main container uses bash.
	initC := pod.Spec.InitContainers[0]
	if len(initC.Command) < 1 || initC.Command[0] != "sh" {
		t.Errorf("init shell = %v, want sh", initC.Command)
	}

	// Main script should contain the pre-command and the controller invocation.
	mainScript := mainC.Args[0]
	if !strings.Contains(mainScript, "source /opt/ros/noetic/setup.bash") {
		t.Errorf("main script missing ROS source: %q", mainScript)
	}
	if !strings.Contains(mainScript, "/opt/rlinf/bin/ros-controller --config=/etc/rlinf/ros-controller.yaml") {
		t.Errorf("main script missing controller invocation: %q", mainScript)
	}
	if !strings.Contains(mainScript, "--http-addr=:8080") {
		t.Errorf("main script missing http-addr: %q", mainScript)
	}

	// Extra env vars should be present.
	envMap := envVarMap(mainC.Env)
	if envMap["ROS_PACKAGE_PATH"] != "/catkin_ws/devel_isolated/share" {
		t.Errorf("ROS_PACKAGE_PATH = %q", envMap["ROS_PACKAGE_PATH"])
	}
	if envMap["CMAKE_PREFIX_PATH"] != "/catkin_ws/devel_isolated" {
		t.Errorf("CMAKE_PREFIX_PATH = %q", envMap["CMAKE_PREFIX_PATH"])
	}
	// POD_IP should always be injected.
	if _, ok := envMap[podIPEnvVar]; !ok {
		t.Error("POD_IP env var missing")
	}

	// Socket path in probe should be ros-specific.
	probeCmd := mainC.LivenessProbe.Exec.Command
	if probeCmd[1] != "--socket-path=/var/run/rlark/ros-ctrl.sock" {
		t.Errorf("probe socket = %q, want ros-ctrl.sock", probeCmd[1])
	}
}

// TestBuildPod_InitShell verifies that InitShell is used for the init
// container and Shell for the main container, and that InitShell defaults
// to "sh" independently of Shell.
func TestBuildPod_InitShell(t *testing.T) {
	// Default: InitShell unset, Shell=bash → init uses sh, main uses bash.
	cfg := Config{
		PodGenerateName: "ros-controller",
		Image:           "img:v1",
		InitImage:       "runtime:v1",
		ControllerBin:   "ros-controller",
		CLIBin:          "rosctr",
		ConfigFileName:  "ros-controller.yaml",
		Shell:           "bash",
	}
	cfg.applyDefaults()
	if cfg.InitShell != "sh" {
		t.Errorf("default InitShell = %q, want sh", cfg.InitShell)
	}
	pod := NewPodManager(fake.NewSimpleClientset(), cfg).buildPod([]byte("x"))
	if got := pod.Spec.InitContainers[0].Command[0]; got != "sh" {
		t.Errorf("init shell = %q, want sh", got)
	}
	if got := pod.Spec.Containers[0].Command[0]; got != "bash" {
		t.Errorf("main shell = %q, want bash", got)
	}

	// Explicit override: InitShell=bash is honoured.
	cfg.InitShell = "bash"
	cfg.applyDefaults()
	pod = NewPodManager(fake.NewSimpleClientset(), cfg).buildPod([]byte("x"))
	if got := pod.Spec.InitContainers[0].Command[0]; got != "bash" {
		t.Errorf("init shell = %q, want bash", got)
	}
}

// ---------------------------------------------------------------------------
// Init container config delivery test
// ---------------------------------------------------------------------------

// TestInitContainerConfigDelivery verifies that the base64-encoded config in
// the init container's command decodes back to the original config bytes.
func TestInitContainerConfigDelivery(t *testing.T) {
	cfg := Config{
		PodGenerateName: "test",
		Image:           "test:v1",
		InitImage:       "runtime:v1",
		ControllerBin:   "controller",
		CLIBin:          "cli",
		ConfigFileName:  "config.yaml",
		NodeName:        "node-1",
		Shell:           "sh",
		HostPID:         true,
		Privileged:      true,
	}
	cfg.applyDefaults()

	mgr := NewPodManager(fake.NewSimpleClientset(), cfg)

	config := []byte("key: value\nlist:\n  - a\n  - b\n")
	pod := mgr.buildPod(config)

	initC := pod.Spec.InitContainers[0]
	script := initC.Args[0]

	// Extract the base64 string from the printf command.
	// The script looks like: printf '%s' '<BASE64>' | base64 -d > <path>
	idx := strings.Index(script, "printf '%s' '")
	if idx < 0 {
		t.Fatalf("printf not found in init script: %q", script)
	}
	start := idx + len("printf '%s' '")
	end := strings.Index(script[start:], "'")
	if end < 0 {
		t.Fatalf("closing quote not found in init script")
	}
	b64Str := script[start : start+end]

	decoded, err := base64.StdEncoding.DecodeString(b64Str)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	if string(decoded) != string(config) {
		t.Errorf("decoded config = %q, want %q", decoded, config)
	}

	// The decoded config path should be correct.
	if !strings.Contains(script, "> /etc/rlinf/config.yaml") {
		t.Errorf("config path not found in script: %q", script)
	}

	// Binary copy commands should be present.
	if !strings.Contains(script, "cp /usr/local/bin/controller /opt/rlinf/bin/controller") {
		t.Errorf("controller copy missing: %q", script)
	}
	if !strings.Contains(script, "cp /usr/local/bin/cli /opt/rlinf/bin/cli") {
		t.Errorf("cli copy missing: %q", script)
	}
	if !strings.Contains(script, "cp /usr/local/bin/cli /host-bin/cli") {
		t.Errorf("host-bin copy missing: %q", script)
	}
}

// ---------------------------------------------------------------------------
// ApplyConfig + Maintain tests (with fake clientset)
// ---------------------------------------------------------------------------

// TestApplyConfig_StoresConfig verifies that ApplyConfig stores the config in
// memory without creating any Kubernetes objects.
func TestApplyConfig_StoresConfig(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{PodGenerateName: "test", ControllerBin: "ctrl", CLIBin: "cli"}
	cfg.applyDefaults()

	mgr := NewPodManager(clientset, cfg)
	config := []byte("test: config\n")

	if err := mgr.ApplyConfig(context.Background(), config); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	// No pods should have been created.
	pods, err := clientset.CoreV1().Pods("default").List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(pods.Items) != 0 {
		t.Errorf("pods created = %d, want 0", len(pods.Items))
	}
}

// TestMaintain_CreatesPod verifies that Maintain creates a pod when none
// exists. Since the fake clientset doesn't run controllers, the pod will
// stay Pending — so we expect Maintain to return an error (timeout waiting
// for Running).
func TestMaintain_CreatesPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{
		PodGenerateName: "test-ctrl",
		Image:           "test:v1",
		InitImage:       "runtime:v1",
		ControllerBin:   "ctrl",
		CLIBin:          "cli",
		ConfigFileName:  "config.yaml",
		NodeName:        "node-1",
		Shell:           "sh",
		HostPID:         true,
		Privileged:      true,
	}
	cfg.applyDefaults()

	mgr := NewPodManager(clientset, cfg)
	config := []byte("test: config\n")

	if err := mgr.ApplyConfig(context.Background(), config); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	// Maintain should try to create the pod. It will time out waiting for
	// Running (fake clientset doesn't update phases), but the pod should
	// be created in the cluster.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := mgr.Maintain(ctx)
	// Error is expected (pod never reaches Running).
	if err == nil {
		// If the fake clientset somehow worked, that's fine too.
		t.Log("Maintain succeeded unexpectedly")
	}

	// Verify the pod was created.
	pod, err := clientset.CoreV1().Pods("default").Get(ctx, "test-ctrl-node-1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pod: %v", err)
	}
	if pod.Spec.NodeSelector["kubernetes.io/hostname"] != "node-1" {
		t.Errorf("nodeSelector = %v, want hostname=node-1", pod.Spec.NodeSelector)
	}
}

// TestMaintain_NoChangeWhenRunning verifies that Maintain returns (false, nil)
// when the pod is already running with the matching config hash.
func TestMaintain_NoChangeWhenRunning(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{
		PodGenerateName: "test-ctrl",
		Image:           "test:v1",
		InitImage:       "runtime:v1",
		ControllerBin:   "ctrl",
		CLIBin:          "cli",
		ConfigFileName:  "config.yaml",
		Shell:           "sh",
		HostPID:         true,
		Privileged:      true,
	}
	cfg.applyDefaults()

	mgr := NewPodManager(clientset, cfg)
	config := []byte("test: config\n")

	// Pre-create a running pod with the correct hash annotation.
	desiredPod := mgr.buildPod(config)
	desiredPod.Status.Phase = corev1.PodRunning
	_, err := clientset.CoreV1().Pods("default").Create(context.Background(), desiredPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create pod: %v", err)
	}

	// ApplyConfig + Maintain should be a no-op.
	if err := mgr.ApplyConfig(context.Background(), config); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
	restarted, err := mgr.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain: %v", err)
	}
	if restarted {
		t.Error("Maintain should not have restarted")
	}
}

// TestMaintain_RestartsOnConfigChange verifies that Maintain recreates the
// pod when the config hash differs.
func TestMaintain_RestartsOnConfigChange(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{
		PodGenerateName: "test-ctrl",
		Image:           "test:v1",
		InitImage:       "runtime:v1",
		ControllerBin:   "ctrl",
		CLIBin:          "cli",
		ConfigFileName:  "config.yaml",
		Shell:           "sh",
		HostPID:         true,
		Privileged:      true,
	}
	cfg.applyDefaults()

	mgr := NewPodManager(clientset, cfg)

	// Pre-create a running pod with an old config hash.
	oldConfig := []byte("old: config\n")
	oldPod := mgr.buildPod(oldConfig)
	oldPod.Status.Phase = corev1.PodRunning
	// Give it a stale hash annotation.
	oldPod.Annotations[ConfigHashAnnotation] = "stale-hash"
	_, err := clientset.CoreV1().Pods("default").Create(context.Background(), oldPod, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create old pod: %v", err)
	}

	// Apply new config.
	newConfig := []byte("new: config\n")
	if err := mgr.ApplyConfig(context.Background(), newConfig); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	// Maintain should delete the old pod and create a new one.
	// It will time out waiting for Running, but the new pod should exist.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = mgr.Maintain(ctx)
	// Error expected (timeout), but the old pod should be gone and new one created.
	_ = err

	newPod, err := clientset.CoreV1().Pods("default").Get(ctx, "test-ctrl", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get new pod: %v", err)
	}
	// The new pod should have the correct hash annotation.
	if newPod.Annotations[ConfigHashAnnotation] != configHash(newConfig, &cfg) {
		t.Errorf("hash = %q, want %q",
			newPod.Annotations[ConfigHashAnnotation], configHash(newConfig, &cfg))
	}
}

// ---------------------------------------------------------------------------
// reconcile — grace / drift / background-loop tests
// ---------------------------------------------------------------------------

// sentinelLabel is stamped onto pre-created pods so the test can tell whether
// a recreate happened: a pod freshly built by buildPod does not carry it (the
// test configs leave Config.Labels empty), so its presence proves the pod was
// NOT recreated, and its absence proves it was.
const sentinelLabel = "test-sentinel"

// notRunningPodConfig is a Config used by the reconcile tests; labels are
// intentionally empty so buildPod produces pods without the sentinel label.
func notRunningPodConfig() Config {
	cfg := Config{
		PodGenerateName: "test-ctrl",
		Image:           "test:v1",
		InitImage:       "runtime:v1",
		ControllerBin:   "ctrl",
		CLIBin:          "cli",
		ConfigFileName:  "config.yaml",
		Shell:           "sh",
		HostPID:         true,
		Privileged:      true,
	}
	cfg.applyDefaults()
	return cfg
}

// TestReconcile_NotRunningObserveGrace verifies that a pod which exists with
// the correct config but is not yet Running is NOT immediately recreated —
// reconcile observes it within the grace period (transient states like image
// pulls or container restarts should not cause thrash).
func TestReconcile_NotRunningObserveGrace(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := notRunningPodConfig()
	mgr := NewPodManager(clientset, cfg)

	config := []byte("test: config\n")
	oldPod := mgr.buildPod(config)
	oldPod.Status.Phase = corev1.PodPending
	oldPod.Labels = map[string]string{sentinelLabel: "old"} // prove no recreate
	if _, err := clientset.CoreV1().Pods("default").Create(context.Background(), oldPod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	if err := mgr.ApplyConfig(context.Background(), config); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	restarted, err := mgr.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain: %v", err)
	}
	if restarted {
		t.Error("Maintain should not have recreated (within grace)")
	}

	got, err := clientset.CoreV1().Pods("default").Get(context.Background(), "test-ctrl", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pod: %v", err)
	}
	if got.Labels[sentinelLabel] != "old" {
		t.Error("pod was recreated (sentinel lost); expected observation within grace")
	}
}

// TestReconcile_NotRunningExceedsGrace verifies that once a not-running pod
// (correct config) has been observed past the grace period, reconcile
// recreates it.
func TestReconcile_NotRunningExceedsGrace(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := notRunningPodConfig()
	mgr := NewPodManager(clientset, cfg)

	config := []byte("test: config\n")
	oldPod := mgr.buildPod(config)
	oldPod.Status.Phase = corev1.PodPending
	oldPod.Labels = map[string]string{sentinelLabel: "old"}
	if _, err := clientset.CoreV1().Pods("default").Create(context.Background(), oldPod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	if err := mgr.ApplyConfig(context.Background(), config); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	// Pretend the pod has been not-running well past the grace period.
	mgr.notRunningGrace = 1 * time.Minute
	mgr.notRunningSince = time.Now().Add(-3 * time.Minute)

	// Start will time out waiting for Running (fake clientset doesn't advance
	// phases), but the recreate (delete + create) still happens.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, _ = mgr.Maintain(ctx) // error expected (timeout), recreate still happens

	got, err := clientset.CoreV1().Pods("default").Get(ctx, "test-ctrl", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pod: %v", err)
	}
	if got.Labels[sentinelLabel] == "old" {
		t.Error("pod was not recreated; expected recreate after grace exceeded")
	}
	if got.Annotations[ConfigHashAnnotation] != configHash(config, &cfg) {
		t.Errorf("hash = %q, want %q",
			got.Annotations[ConfigHashAnnotation], configHash(config, &cfg))
	}
}

// TestReconcile_PodConfigDrift verifies that changing the PodConfig (here,
// the image) invalidates the hash and triggers a recreate — even though the
// controller YAML config bytes are unchanged.
func TestReconcile_PodConfigDrift(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := notRunningPodConfig()
	mgr := NewPodManager(clientset, cfg)

	config := []byte("test: config\n")
	// Pre-create a running pod carrying the CURRENT hash.
	runningPod := mgr.buildPod(config)
	runningPod.Status.Phase = corev1.PodRunning
	if _, err := clientset.CoreV1().Pods("default").Create(context.Background(), runningPod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pod: %v", err)
	}
	if err := mgr.ApplyConfig(context.Background(), config); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	// Sanity: with the same config + PodConfig, Maintain is a no-op.
	if restarted, err := mgr.Maintain(context.Background()); err != nil || restarted {
		t.Fatalf("Maintain should be a no-op initially, got restarted=%v err=%v", restarted, err)
	}

	// Now change the PodConfig (image) — the hash changes even though the
	// controller YAML (lastConfig) is identical.
	mgr.config.Image = "test:v2-changed"

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, _ = mgr.Maintain(ctx) // recreate times out waiting for Running

	got, err := clientset.CoreV1().Pods("default").Get(ctx, "test-ctrl", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pod: %v", err)
	}
	// The recreated pod's hash now reflects the changed image, and the pod
	// spec carries the new image.
	if got.Annotations[ConfigHashAnnotation] != configHash(config, &mgr.config) {
		t.Errorf("hash = %q, want %q (PodConfig drift not detected)",
			got.Annotations[ConfigHashAnnotation], configHash(config, &mgr.config))
	}
	if got.Spec.Containers[0].Image != "test:v2-changed" {
		t.Errorf("image = %q, want test:v2-changed", got.Spec.Containers[0].Image)
	}
}

// TestReconcile_LoopNoThrash verifies the background reconciler does not
// recreate a healthy (Running, matching-hash) pod across several ticks.
func TestReconcile_LoopNoThrash(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := notRunningPodConfig()
	mgr := NewPodManager(clientset, cfg)
	mgr.reconcileInterval = 50 * time.Millisecond // fast ticks

	config := []byte("test: config\n")
	// Pre-create a running pod with the matching hash before Init.
	runningPod := mgr.buildPod(config)
	runningPod.Status.Phase = corev1.PodRunning
	runningPod.Labels = map[string]string{sentinelLabel: "old"}
	if _, err := clientset.CoreV1().Pods("default").Create(context.Background(), runningPod, metav1.CreateOptions{}); err != nil {
		t.Fatalf("create pod: %v", err)
	}

	if err := mgr.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := mgr.ApplyConfig(context.Background(), config); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}

	// Let the background loop tick several times.
	time.Sleep(500 * time.Millisecond)

	got, err := clientset.CoreV1().Pods("default").Get(context.Background(), "test-ctrl", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get pod: %v", err)
	}
	if got.Labels[sentinelLabel] != "old" {
		t.Error("background loop recreated a healthy pod (sentinel lost); expected no thrash")
	}
	if got.Status.Phase != corev1.PodRunning {
		t.Errorf("phase = %s, want Running", got.Status.Phase)
	}

	mgr.Stop(context.Background())
}

// ---------------------------------------------------------------------------
// IsRunning / Stop tests
// ---------------------------------------------------------------------------

// TestIsRunning_NoPod verifies that IsRunning returns false when no pod exists.
func TestIsRunning_NoPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{PodGenerateName: "nope", ControllerBin: "ctrl", CLIBin: "cli"}
	cfg.applyDefaults()
	mgr := NewPodManager(clientset, cfg)

	if mgr.IsRunning(context.Background()) {
		t.Error("IsRunning should be false when no pod exists")
	}
}

// TestIsRunning_Running verifies that IsRunning returns true when the pod is
// in the Running phase and reports the Ready condition.
func TestIsRunning_Running(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{PodGenerateName: "test", ControllerBin: "ctrl", CLIBin: "cli"}
	cfg.applyDefaults()
	mgr := NewPodManager(clientset, cfg)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionTrue,
			}},
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	if !mgr.IsRunning(context.Background()) {
		t.Error("IsRunning should be true for Running+Ready pod")
	}
}

// TestIsRunning_NotReady verifies that IsRunning returns false when the pod is
// in the Running phase but its PodReady condition is not True (e.g. the
// readiness probe has not passed yet).
func TestIsRunning_NotReady(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{PodGenerateName: "test", ControllerBin: "ctrl", CLIBin: "cli"}
	cfg.applyDefaults()
	mgr := NewPodManager(clientset, cfg)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{{
				Type:   corev1.PodReady,
				Status: corev1.ConditionFalse,
			}},
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	if mgr.IsRunning(context.Background()) {
		t.Error("IsRunning should be false when pod is Running but not Ready")
	}
}

// TestStop_DeletesPod verifies that Stop deletes the pod.
func TestStop_DeletesPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{PodGenerateName: "test", ControllerBin: "ctrl", CLIBin: "cli"}
	cfg.applyDefaults()
	mgr := NewPodManager(clientset, cfg)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	mgr.Stop(context.Background())

	_, err := clientset.CoreV1().Pods("default").Get(context.Background(), "test", metav1.GetOptions{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("pod should be deleted, got err: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Init tests
// ---------------------------------------------------------------------------

// TestInit_NoPod verifies that Init succeeds and sets runningHash to empty
// when no pod exists.
func TestInit_NoPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{PodGenerateName: "test", ControllerBin: "ctrl", CLIBin: "cli"}
	cfg.applyDefaults()
	mgr := NewPodManager(clientset, cfg)

	if err := mgr.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if mgr.runningHash != "" {
		t.Errorf("runningHash = %q, want empty", mgr.runningHash)
	}
}

// TestInit_RunningPod verifies that Init detects an existing running pod and
// records its config hash annotation.
func TestInit_RunningPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{PodGenerateName: "test", ControllerBin: "ctrl", CLIBin: "cli"}
	cfg.applyDefaults()
	mgr := NewPodManager(clientset, cfg)

	// Pre-create a running pod with a known config hash.
	config := []byte("test: config\n")
	hash := configHash(config, &cfg)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				ConfigHashAnnotation: hash,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	if err := mgr.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if mgr.runningHash != hash {
		t.Errorf("runningHash = %q, want %q", mgr.runningHash, hash)
	}
}

// TestInit_PendingPod verifies that Init records the config hash even when
// the pod is not yet running.
func TestInit_PendingPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{PodGenerateName: "test", ControllerBin: "ctrl", CLIBin: "cli"}
	cfg.applyDefaults()
	mgr := NewPodManager(clientset, cfg)

	config := []byte("pending: config\n")
	hash := configHash(config, &cfg)
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				ConfigHashAnnotation: hash,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	if err := mgr.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if mgr.runningHash != hash {
		t.Errorf("runningHash = %q, want %q", mgr.runningHash, hash)
	}
}

// TestInit_ThenMaintain_NoRecreate verifies the full flow: Init detects a
// running pod with matching config, then Maintain is a no-op.
func TestInit_ThenMaintain_NoRecreate(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	cfg := Config{PodGenerateName: "test", ControllerBin: "ctrl", CLIBin: "cli"}
	cfg.applyDefaults()
	mgr := NewPodManager(clientset, cfg)

	config := []byte("test: config\n")
	hash := configHash(config, &cfg)

	// Pre-create a running pod with the correct hash.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
			Annotations: map[string]string{
				ConfigHashAnnotation: hash,
			},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	// Init should detect the running pod.
	if err := mgr.Init(context.Background()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if mgr.runningHash != hash {
		t.Fatalf("runningHash = %q, want %q", mgr.runningHash, hash)
	}

	// ApplyConfig with the same config, then Maintain should be a no-op.
	if err := mgr.ApplyConfig(context.Background(), config); err != nil {
		t.Fatalf("ApplyConfig: %v", err)
	}
	restarted, err := mgr.Maintain(context.Background())
	if err != nil {
		t.Fatalf("Maintain: %v", err)
	}
	if restarted {
		t.Error("Maintain should not have restarted")
	}
}

// ---------------------------------------------------------------------------
// OwnerReferences tests
// ---------------------------------------------------------------------------

// TestBuildPod_OwnerReferences verifies that OwnerReferences from Config
// are applied to the pod's ObjectMeta.
func TestBuildPod_OwnerReferences(t *testing.T) {
	controller := true
	ownerRef := metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "DaemonSet",
		Name:       "device-plugin",
		UID:        "abc-123",
		Controller: &controller,
	}

	cfg := Config{
		PodGenerateName: "test",
		Image:           "test:v1",
		InitImage:       "runtime:v1",
		ControllerBin:   "ctrl",
		CLIBin:          "cli",
		ConfigFileName:  "config.yaml",
		OwnerReferences: []metav1.OwnerReference{ownerRef},
	}
	cfg.applyDefaults()

	mgr := NewPodManager(fake.NewSimpleClientset(), cfg)
	pod := mgr.buildPod([]byte("test: config\n"))

	if len(pod.OwnerReferences) != 1 {
		t.Fatalf("ownerReferences = %d, want 1", len(pod.OwnerReferences))
	}
	got := pod.OwnerReferences[0]
	if got.APIVersion != "apps/v1" || got.Kind != "DaemonSet" || got.Name != "device-plugin" {
		t.Errorf("ownerRef = %+v, want {apps/v1 DaemonSet device-plugin}", got)
	}
	if got.UID != "abc-123" {
		t.Errorf("UID = %q, want abc-123", got.UID)
	}
	if got.Controller == nil || !*got.Controller {
		t.Error("Controller should be true")
	}
}

// TestBuildPod_NoOwnerReferences verifies that pods with no OwnerReferences
// in Config have an empty (nil) OwnerReferences field.
func TestBuildPod_NoOwnerReferences(t *testing.T) {
	cfg := Config{
		PodGenerateName: "test",
		Image:           "test:v1",
		InitImage:       "runtime:v1",
		ControllerBin:   "ctrl",
		CLIBin:          "cli",
		ConfigFileName:  "config.yaml",
	}
	cfg.applyDefaults()

	mgr := NewPodManager(fake.NewSimpleClientset(), cfg)
	pod := mgr.buildPod([]byte("test: config\n"))

	if len(pod.OwnerReferences) != 0 {
		t.Errorf("ownerReferences = %d, want 0", len(pod.OwnerReferences))
	}
}

// TestDiscoverOwnerRefForPod_IgnoresController verifies that
// DiscoverOwnerRefForPod always returns the pod itself as owner, even
// when the pod has a controlling owner (e.g. a DaemonSet).
func TestDiscoverOwnerRefForPod_IgnoresController(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	controller := true
	ownerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "device-plugin-abc",
			Namespace: "default",
			UID:       "pod-uid-123",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "apps/v1",
					Kind:       "DaemonSet",
					Name:       "device-plugin-ds",
					UID:        "ds-uid-456",
					Controller: &controller,
				},
			},
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), ownerPod, metav1.CreateOptions{})

	ref, err := DiscoverOwnerRefForPod(context.Background(), clientset, "device-plugin-abc", "default")
	if err != nil {
		t.Fatalf("DiscoverOwnerRefForPod: %v", err)
	}
	if ref.Kind != "Pod" {
		t.Errorf("Kind = %q, want Pod", ref.Kind)
	}
	if ref.Name != "device-plugin-abc" {
		t.Errorf("Name = %q, want device-plugin-abc", ref.Name)
	}
	if ref.UID != "pod-uid-123" {
		t.Errorf("UID = %q, want pod-uid-123", ref.UID)
	}
	if ref.Controller == nil || !*ref.Controller {
		t.Error("Controller should be true")
	}
}

// TestDiscoverOwnerRefForPod_StandalonePod verifies that the function
// returns the pod itself as owner.
func TestDiscoverOwnerRefForPod_StandalonePod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	ownerPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "standalone-pod",
			Namespace: "default",
			UID:       "standalone-uid",
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), ownerPod, metav1.CreateOptions{})

	ref, err := DiscoverOwnerRefForPod(context.Background(), clientset, "standalone-pod", "default")
	if err != nil {
		t.Fatalf("DiscoverOwnerRefForPod: %v", err)
	}
	if ref.Kind != "Pod" {
		t.Errorf("Kind = %q, want Pod", ref.Kind)
	}
	if ref.Name != "standalone-pod" {
		t.Errorf("Name = %q, want standalone-pod", ref.Name)
	}
	if ref.UID != "standalone-uid" {
		t.Errorf("UID = %q, want standalone-uid", ref.UID)
	}
}

// TestDiscoverOwnerRefForPod_NotFound verifies that a non-existent pod
// returns an error.
func TestDiscoverOwnerRefForPod_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	_, err := DiscoverOwnerRefForPod(context.Background(), clientset, "nope", "default")
	if err == nil {
		t.Fatal("expected error for non-existent pod")
	}
}

// TestDiscoverOwnerRef_NoEnv verifies that DiscoverOwnerRef returns an error
// when POD_NAME/POD_NAMESPACE are unset.
func TestDiscoverOwnerRef_NoEnv(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("POD_NAMESPACE", "")
	_, err := DiscoverOwnerRef(context.Background(), fake.NewSimpleClientset())
	if err == nil {
		t.Fatal("expected error when env vars are unset")
	}
}

// ---------------------------------------------------------------------------
// Tolerations discovery tests
// ---------------------------------------------------------------------------

// TestDiscoverTolerationsForPod verifies that DiscoverTolerationsForPod
// returns the tolerations from the specified pod.
func TestDiscoverTolerationsForPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	tolerations := []corev1.Toleration{
		{
			Key:      "dedicated",
			Operator: corev1.TolerationOpEqual,
			Value:    "robot",
			Effect:   corev1.TaintEffectNoSchedule,
		},
		{
			Key:      "rlinf.io/robot",
			Operator: corev1.TolerationOpExists,
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "device-plugin-abc",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Tolerations: tolerations,
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	got, err := DiscoverTolerationsForPod(context.Background(), clientset, "device-plugin-abc", "default")
	if err != nil {
		t.Fatalf("DiscoverTolerationsForPod: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("tolerations = %d, want 2", len(got))
	}
	if got[0].Key != "dedicated" || got[1].Key != "rlinf.io/robot" {
		t.Errorf("tolerations = %+v", got)
	}
}

// TestDiscoverTolerationsForPod_None verifies that a pod with no tolerations
// returns an empty slice without error.
func TestDiscoverTolerationsForPod_None(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "plain-pod",
			Namespace: "default",
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	got, err := DiscoverTolerationsForPod(context.Background(), clientset, "plain-pod", "default")
	if err != nil {
		t.Fatalf("DiscoverTolerationsForPod: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("tolerations = %d, want 0", len(got))
	}
}

// TestDiscoverTolerationsForPod_NotFound verifies that a non-existent pod
// returns an error.
func TestDiscoverTolerationsForPod_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	_, err := DiscoverTolerationsForPod(context.Background(), clientset, "nope", "default")
	if err == nil {
		t.Fatal("expected error for non-existent pod")
	}
}

// TestDiscoverTolerations_NoEnv verifies that DiscoverTolerations returns an
// error when POD_NAME/POD_NAMESPACE are unset.
func TestDiscoverTolerations_NoEnv(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("POD_NAMESPACE", "")
	_, err := DiscoverTolerations(context.Background(), fake.NewSimpleClientset())
	if err == nil {
		t.Fatal("expected error when env vars are unset")
	}
}

// TestDiscoverTolerations_WithEnv verifies that DiscoverTolerations reads the
// pod identity from env vars and returns the pod's tolerations.
func TestDiscoverTolerations_WithEnv(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp-pod",
			Namespace: "dp-ns",
		},
		Spec: corev1.PodSpec{
			Tolerations: DefaultTolerations(),
		},
	}
	_, _ = clientset.CoreV1().Pods("dp-ns").Create(context.Background(), pod, metav1.CreateOptions{})

	t.Setenv("POD_NAME", "dp-pod")
	t.Setenv("POD_NAMESPACE", "dp-ns")

	got, err := DiscoverTolerations(context.Background(), clientset)
	if err != nil {
		t.Fatalf("DiscoverTolerations: %v", err)
	}
	if len(got) != 1 || got[0].Key != "rlinf.io/robot" {
		t.Errorf("tolerations = %+v, want DefaultTolerations", got)
	}
}

// ---------------------------------------------------------------------------
// Image discovery tests
// ---------------------------------------------------------------------------

// TestDiscoverImageForPod verifies that DiscoverImageForPod returns the image
// of the pod's first container.
func TestDiscoverImageForPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "device-plugin-abc",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "device-plugin", Image: "rlinf/embodied-runtime:v0.2.0"},
				{Name: "sidecar", Image: "other:v1"},
			},
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	got, err := DiscoverImageForPod(context.Background(), clientset, "device-plugin-abc", "default")
	if err != nil {
		t.Fatalf("DiscoverImageForPod: %v", err)
	}
	if got != "rlinf/embodied-runtime:v0.2.0" {
		t.Errorf("image = %q, want rlinf/embodied-runtime:v0.2.0", got)
	}
}

// TestDiscoverImageForPod_NoContainers verifies that a pod with no containers
// returns an error.
func TestDiscoverImageForPod_NoContainers(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-pod",
			Namespace: "default",
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	_, err := DiscoverImageForPod(context.Background(), clientset, "empty-pod", "default")
	if err == nil {
		t.Fatal("expected error for pod with no containers")
	}
}

// TestDiscoverImageForPod_NotFound verifies that a non-existent pod returns
// an error.
func TestDiscoverImageForPod_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	_, err := DiscoverImageForPod(context.Background(), clientset, "nope", "default")
	if err == nil {
		t.Fatal("expected error for non-existent pod")
	}
}

// TestDiscoverImage_WithEnv verifies that DiscoverImage reads the pod identity
// from env vars and returns the pod's first container image.
func TestDiscoverImage_WithEnv(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp-pod",
			Namespace: "dp-ns",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "device-plugin", Image: "rlinf/embodied-runtime:v0.3.0"},
			},
		},
	}
	_, _ = clientset.CoreV1().Pods("dp-ns").Create(context.Background(), pod, metav1.CreateOptions{})

	t.Setenv("POD_NAME", "dp-pod")
	t.Setenv("POD_NAMESPACE", "dp-ns")

	got, err := DiscoverImage(context.Background(), clientset)
	if err != nil {
		t.Fatalf("DiscoverImage: %v", err)
	}
	if got != "rlinf/embodied-runtime:v0.3.0" {
		t.Errorf("image = %q, want rlinf/embodied-runtime:v0.3.0", got)
	}
}

// TestDiscoverImage_NoEnv verifies that DiscoverImage returns an error when
// POD_NAME/POD_NAMESPACE are unset.
func TestDiscoverImage_NoEnv(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("POD_NAMESPACE", "")
	_, err := DiscoverImage(context.Background(), fake.NewSimpleClientset())
	if err == nil {
		t.Fatal("expected error when env vars are unset")
	}
}

// ---------------------------------------------------------------------------
// NodeName discovery tests
// ---------------------------------------------------------------------------

// TestDiscoverNodeNameForPod verifies that DiscoverNodeNameForPod returns the
// node name the pod is scheduled on.
func TestDiscoverNodeNameForPod(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "device-plugin-abc",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "robot-node-1",
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	got, err := DiscoverNodeNameForPod(context.Background(), clientset, "device-plugin-abc", "default")
	if err != nil {
		t.Fatalf("DiscoverNodeNameForPod: %v", err)
	}
	if got != "robot-node-1" {
		t.Errorf("nodeName = %q, want robot-node-1", got)
	}
}

// TestDiscoverNodeNameForPod_Unscheduled verifies that a pod not yet
// scheduled to a node returns an error.
func TestDiscoverNodeNameForPod_Unscheduled(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-pod",
			Namespace: "default",
		},
	}
	_, _ = clientset.CoreV1().Pods("default").Create(context.Background(), pod, metav1.CreateOptions{})

	_, err := DiscoverNodeNameForPod(context.Background(), clientset, "pending-pod", "default")
	if err == nil {
		t.Fatal("expected error for unscheduled pod")
	}
}

// TestDiscoverNodeNameForPod_NotFound verifies that a non-existent pod
// returns an error.
func TestDiscoverNodeNameForPod_NotFound(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	_, err := DiscoverNodeNameForPod(context.Background(), clientset, "nope", "default")
	if err == nil {
		t.Fatal("expected error for non-existent pod")
	}
}

// TestDiscoverNodeName_WithEnv verifies that DiscoverNodeName reads the pod
// identity from env vars and returns the node name.
func TestDiscoverNodeName_WithEnv(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dp-pod",
			Namespace: "dp-ns",
		},
		Spec: corev1.PodSpec{
			NodeName: "robot-node-2",
		},
	}
	_, _ = clientset.CoreV1().Pods("dp-ns").Create(context.Background(), pod, metav1.CreateOptions{})

	t.Setenv("POD_NAME", "dp-pod")
	t.Setenv("POD_NAMESPACE", "dp-ns")

	got, err := DiscoverNodeName(context.Background(), clientset)
	if err != nil {
		t.Fatalf("DiscoverNodeName: %v", err)
	}
	if got != "robot-node-2" {
		t.Errorf("nodeName = %q, want robot-node-2", got)
	}
}

// TestDiscoverNodeName_NoEnv verifies that DiscoverNodeName returns an error
// when POD_NAME/POD_NAMESPACE are unset.
func TestDiscoverNodeName_NoEnv(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("POD_NAMESPACE", "")
	_, err := DiscoverNodeName(context.Background(), fake.NewSimpleClientset())
	if err == nil {
		t.Fatal("expected error when env vars are unset")
	}
}

// ---------------------------------------------------------------------------
// CurrentPod tests
// ---------------------------------------------------------------------------

// TestCurrentPod_WithEnv verifies that CurrentPod reads the pod identity
// from env vars and returns the pod.
func TestCurrentPod_WithEnv(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "dp-pod", Namespace: "dp-ns", UID: "uid-1"},
	}
	_, _ = clientset.CoreV1().Pods("dp-ns").Create(context.Background(), pod, metav1.CreateOptions{})

	t.Setenv("POD_NAME", "dp-pod")
	t.Setenv("POD_NAMESPACE", "dp-ns")

	got, err := CurrentPod(context.Background(), clientset)
	if err != nil {
		t.Fatalf("CurrentPod: %v", err)
	}
	if got.Name != "dp-pod" || got.UID != "uid-1" {
		t.Errorf("got pod = %s/%s, want dp-pod/uid-1", got.Name, got.UID)
	}
}

// TestCurrentPod_NoEnv verifies that CurrentPod returns an error when env
// vars are unset.
func TestCurrentPod_NoEnv(t *testing.T) {
	t.Setenv("POD_NAME", "")
	t.Setenv("POD_NAMESPACE", "")
	_, err := CurrentPod(context.Background(), fake.NewSimpleClientset())
	if err == nil {
		t.Fatal("expected error when env vars are unset")
	}
}

// TestCurrentPodFor verifies that CurrentPodFor returns the pod for the
// given name/namespace.
func TestCurrentPodFor(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "abc", Namespace: "ns-1"},
	}
	_, _ = clientset.CoreV1().Pods("ns-1").Create(context.Background(), pod, metav1.CreateOptions{})

	got, err := CurrentPodFor(context.Background(), clientset, "abc", "ns-1")
	if err != nil {
		t.Fatalf("CurrentPodFor: %v", err)
	}
	if got.Name != "abc" {
		t.Errorf("pod name = %q, want abc", got.Name)
	}
}

// TestCurrentPodFor_NotFound verifies that a non-existent pod returns an
// error.
func TestCurrentPodFor_NotFound(t *testing.T) {
	_, err := CurrentPodFor(context.Background(), fake.NewSimpleClientset(), "nope", "default")
	if err == nil {
		t.Fatal("expected error for non-existent pod")
	}
}

// ---------------------------------------------------------------------------
// From-pod extractor tests
// ---------------------------------------------------------------------------

// TestOwnerRefFromPod_IgnoresController verifies that OwnerRefFromPod
// always returns the pod itself as owner, even when the pod has a
// controlling owner (e.g. a DaemonSet).
func TestOwnerRefFromPod_IgnoresController(t *testing.T) {
	controller := true
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dp",
			UID:  "pod-uid",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: "apps/v1", Kind: "DaemonSet", Name: "ds", UID: "ds-uid", Controller: &controller},
			},
		},
	}
	ref := OwnerRefFromPod(pod)
	if ref.Kind != "Pod" || ref.Name != "dp" || ref.UID != "pod-uid" {
		t.Errorf("ownerRef = %+v, want Pod/dp/pod-uid", ref)
	}
	if ref.Controller == nil || !*ref.Controller {
		t.Error("Controller should be true")
	}
}

// TestOwnerRefFromPod_Standalone verifies that OwnerRefFromPod returns the
// pod itself as owner.
func TestOwnerRefFromPod_Standalone(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "standalone", UID: "s-uid"},
	}
	ref := OwnerRefFromPod(pod)
	if ref.Kind != "Pod" || ref.Name != "standalone" || ref.UID != "s-uid" {
		t.Errorf("ownerRef = %+v, want Pod/standalone/s-uid", ref)
	}
}

// TestTolerationsFromPod verifies that TolerationsFromPod returns the pod's
// tolerations.
func TestTolerationsFromPod(t *testing.T) {
	tols := []corev1.Toleration{{Key: "k", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec:       corev1.PodSpec{Tolerations: tols},
	}
	got := TolerationsFromPod(pod)
	if len(got) != 1 || got[0].Key != "k" {
		t.Errorf("tolerations = %+v, want one with key k", got)
	}
}

// TestImageFromPod verifies that ImageFromPod returns the first container's
// image.
func TestImageFromPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "c0", Image: "img:v1"},
			{Name: "c1", Image: "other:v2"},
		}},
	}
	got, err := ImageFromPod(pod)
	if err != nil {
		t.Fatalf("ImageFromPod: %v", err)
	}
	if got != "img:v1" {
		t.Errorf("image = %q, want img:v1", got)
	}
}

// TestImageFromPod_NoContainers verifies that ImageFromPod returns an error
// when the pod has no containers.
func TestImageFromPod_NoContainers(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
	}
	_, err := ImageFromPod(pod)
	if err == nil {
		t.Fatal("expected error for pod with no containers")
	}
}

// TestNodeNameFromPod verifies that NodeNameFromPod returns the scheduled
// node name.
func TestNodeNameFromPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
		Spec:       corev1.PodSpec{NodeName: "node-1"},
	}
	got, err := NodeNameFromPod(pod)
	if err != nil {
		t.Fatalf("NodeNameFromPod: %v", err)
	}
	if got != "node-1" {
		t.Errorf("nodeName = %q, want node-1", got)
	}
}

// TestNodeNameFromPod_Unscheduled verifies that NodeNameFromPod returns an
// error when the pod is not yet scheduled.
func TestNodeNameFromPod_Unscheduled(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p", Namespace: "ns"},
	}
	_, err := NodeNameFromPod(pod)
	if err == nil {
		t.Fatal("expected error for unscheduled pod")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func volumeMountNames(mounts []corev1.VolumeMount) map[string]bool {
	m := make(map[string]bool, len(mounts))
	for _, vm := range mounts {
		m[vm.Name] = true
	}
	return m
}

func volumeMap(vols []corev1.Volume) map[string]corev1.Volume {
	m := make(map[string]corev1.Volume, len(vols))
	for _, v := range vols {
		m[v.Name] = v
	}
	return m
}

func envVarMap(envs []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(envs))
	for _, e := range envs {
		m[e.Name] = e.Value
	}
	return m
}

// TestBuildPod_HeadlessDNS verifies that Config.Hostname / Config.Subdomain
// are projected onto pod.spec.hostname / pod.spec.subdomain so the pod is
// reachable at <hostname>.<subdomain>.<ns>.svc.cluster.local when a headless
// Service named <subdomain> exists. The Service itself is not created here —
// only the pod side is configured.
func TestBuildPod_HeadlessDNS(t *testing.T) {
	cfg := Config{
		Namespace:       "robot-ns",
		PodGenerateName: "ros-controller",
		Image:           "img:v1",
		InitImage:       "init:v1",
		ControllerBin:   "ros-controller",
		CLIBin:          "rosctr",
		ConfigFileName:  "ros-controller.yaml",
		NodeName:        "node-1",
		Hostname:        "ros-controller",
		Subdomain:       "ros-controller-headless",
	}
	cfg.applyDefaults()

	pod := NewPodManager(fake.NewSimpleClientset(), cfg).buildPod([]byte("x"))

	if pod.Spec.Hostname != "ros-controller" {
		t.Errorf("hostname = %q, want ros-controller", pod.Spec.Hostname)
	}
	if pod.Spec.Subdomain != "ros-controller-headless" {
		t.Errorf("subdomain = %q, want ros-controller-headless", pod.Spec.Subdomain)
	}
}

// TestBuildPod_HeadlessDNS_DeriveHostname verifies that when Subdomain is set
// but Hostname is left empty, applyDefaults derives Hostname from the pod's
// deterministic name (PodName()) so a stable per-pod A record
// <pod-name>.<subdomain>.<ns>.svc.cluster.local is generated even without an
// explicit hostname. The readable per-pod record needs pod.spec.hostname to
// be set; this makes it explicit from the (deterministic) pod name.
func TestBuildPod_HeadlessDNS_DeriveHostname(t *testing.T) {
	cfg := Config{
		PodGenerateName: "camera-controller",
		Image:           "img:v1",
		InitImage:       "init:v1",
		ControllerBin:   "camera-controller",
		CLIBin:          "camctr",
		ConfigFileName:  "camera-controller.yaml",
		NodeName:        "node-1",
		Subdomain:       "camera-controller-headless",
		// Hostname intentionally left empty.
	}
	cfg.applyDefaults()

	if cfg.Hostname != "camera-controller-node-1" {
		t.Errorf("derived Hostname = %q, want camera-controller-node-1", cfg.Hostname)
	}

	pod := NewPodManager(fake.NewSimpleClientset(), cfg).buildPod([]byte("x"))
	if pod.Spec.Hostname != "camera-controller-node-1" {
		t.Errorf("pod hostname = %q, want camera-controller-node-1 (derived from pod name)", pod.Spec.Hostname)
	}
	if pod.Spec.Subdomain != "camera-controller-headless" {
		t.Errorf("subdomain = %q, want camera-controller-headless", pod.Spec.Subdomain)
	}
}

// TestBuildPod_HeadlessDNS_Empty verifies that leaving Hostname AND Subdomain
// empty leaves them unset on the pod (no stable headless FQDN, and no
// derivation since subdomain is the trigger).
func TestBuildPod_HeadlessDNS_Empty(t *testing.T) {
	cfg := Config{
		PodGenerateName: "ctrl",
		ControllerBin:   "ctrl",
		CLIBin:          "cli",
		Image:           "img:v1",
		InitImage:       "init:v1",
		ConfigFileName:  "ctrl.yaml",
	}
	cfg.applyDefaults()

	pod := NewPodManager(fake.NewSimpleClientset(), cfg).buildPod([]byte("x"))
	if pod.Spec.Hostname != "" {
		t.Errorf("hostname = %q, want empty (unset)", pod.Spec.Hostname)
	}
	if pod.Spec.Subdomain != "" {
		t.Errorf("subdomain = %q, want empty (unset)", pod.Spec.Subdomain)
	}
}

// TestBuildPod_NoDefaultLabels verifies the pod carries NO built-in labels
// when the operator doesn't set any — AppLabel and its derived
// app/app.kubernetes.io/* labels were removed, so Labels is the sole source.
func TestBuildPod_NoDefaultLabels(t *testing.T) {
	cfg := Config{
		PodGenerateName: "ros-controller",
		Image:           "img:v1",
		InitImage:       "init:v1",
		ControllerBin:   "ros-controller",
		CLIBin:          "rosctr",
		ConfigFileName:  "ros-controller.yaml",
		NodeName:        "node-1",
	}
	cfg.applyDefaults()
	pod := NewPodManager(fake.NewSimpleClientset(), cfg).buildPod([]byte("x"))

	if len(pod.Labels) != 0 {
		t.Errorf("labels = %v, want empty (no built-in labels)", pod.Labels)
	}
}

// TestBuildPod_Labels verifies the pod's labels are exactly the operator-
// supplied Labels map (the sole source). A headless Service selects on
// whatever key the operator puts here, e.g. app.kubernetes.io/name.
func TestBuildPod_Labels(t *testing.T) {
	cfg := Config{
		PodGenerateName: "ctrl",
		ControllerBin:   "ctrl",
		CLIBin:          "cli",
		Image:           "img:v1",
		InitImage:       "init:v1",
		ConfigFileName:  "ctrl.yaml",
		Labels: map[string]string{
			"app.kubernetes.io/name": "ros-controller",
			"team":                   "robotics",
		},
	}
	cfg.applyDefaults()
	pod := NewPodManager(fake.NewSimpleClientset(), cfg).buildPod([]byte("x"))

	if len(pod.Labels) != 2 {
		t.Fatalf("labels = %v, want exactly the 2 operator labels", pod.Labels)
	}
	if pod.Labels["app.kubernetes.io/name"] != "ros-controller" {
		t.Errorf("name label = %q", pod.Labels["app.kubernetes.io/name"])
	}
	if pod.Labels["team"] != "robotics" {
		t.Errorf("team label = %q", pod.Labels["team"])
	}
}
