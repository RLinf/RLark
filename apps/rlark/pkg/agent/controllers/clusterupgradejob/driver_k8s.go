package clusterupgradejob

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// K8sDriver implements UpgradeDriver for a Kubernetes data plane.
// It runs upgrade/rollback as batchv1.Job and stores configs in ConfigMaps.
type K8sDriver struct {
	client            client.Client
	namespace         string
	managementCluster string
	stateStore        StateStore
}

// NewK8sDriver creates a K8sDriver.
func NewK8sDriver(c client.Client, managementCluster string) *K8sDriver {
	return &K8sDriver{
		client:            c,
		namespace:         StateStoreNamespace,
		managementCluster: managementCluster,
		stateStore:        NewConfigMapStateStore(c),
	}
}

// StateStore returns the ConfigMap-backed StateStore.
func (d *K8sDriver) StateStore() StateStore {
	return d.stateStore
}

// PrepareConfig stores the merged deploy config as a ConfigMap.
// The configRef returned is the ConfigMap name.
func (d *K8sDriver) PrepareConfig(ctx context.Context, job *rlarkv1alpha1.ClusterUpgradeJob, config string) (string, error) {
	cmName := upgradeConfigMapName(job.Name)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: d.namespace,
		},
		Data: map[string]string{
			"deploy-config.yaml": config,
		},
	}
	if err := d.client.Create(ctx, cm); err != nil {
		// Tolerate AlreadyExists in case the agent restarted after creating
		// the ConfigMap but before recording the job status.
		if !apierrors.IsAlreadyExists(err) {
			return "", fmt.Errorf("create upgrade configmap: %w", err)
		}
	}
	return cmName, nil
}

// CleanupConfig deletes the per-upgrade ConfigMap. NotFound is success.
func (d *K8sDriver) CleanupConfig(ctx context.Context, job *rlarkv1alpha1.ClusterUpgradeJob) error {
	err := d.client.Delete(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      upgradeConfigMapName(job.Name),
			Namespace: d.namespace,
		},
	})
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return nil
}

// StartUpgrade creates a batchv1.Job that runs the rlark-upgrade image.
func (d *K8sDriver) StartUpgrade(ctx context.Context, job *rlarkv1alpha1.ClusterUpgradeJob, rollbackImage, configRef string) (string, error) {
	timeout := job.Spec.TimeoutSeconds
	if timeout == 0 {
		timeout = 600
	}

	autoRollback := job.Spec.AutoRollback
	volumeMounts := []corev1.VolumeMount{{
		Name:      "deploy-config",
		MountPath: "/etc/rlark/deploy-config.yaml",
		SubPath:   "deploy-config.yaml",
	}}
	volumes := []corev1.Volume{{
		Name: "deploy-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: configRef},
			},
		},
	}}

	// Mount rollback config when a previous version exists.
	// Using Optional=true so the Pod can be scheduled even if rlark-state is absent.
	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      "rollback-config",
		MountPath: "/etc/rlark/rollback-deploy-config.yaml",
		SubPath:   "rollback-deploy-config.yaml",
	})
	optional := true
	volumes = append(volumes, corev1.Volume{
		Name: "rollback-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: StateStoreName},
				Items: []corev1.KeyToPath{
					{Key: "deploy-config", Path: "rollback-deploy-config.yaml"},
				},
				Optional: &optional,
			},
		},
	})

	// Use a stable job name so that a restarted agent can find the existing job
	// instead of creating a duplicate.
	jobName := "upgrade-" + job.Name
	k8sJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: d.namespace,
			Labels: map[string]string{
				"rlark.io/cluster-upgrade-job": job.Name,
				"rlark.io/cluster":             d.managementCluster,
			},
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: ptr.To(int32(3600)),
			BackoffLimit:            ptr.To(int32(0)),
			// Use 2x the user timeout so that health check failure (which uses
			// half the timeout) still leaves room for rollback before the
			// hard deadline kills the Pod and loses the exit code.
			ActiveDeadlineSeconds: ptr.To(timeout * 2), // 保证 job 不会因 ActiveDeadlineSeconds 而 terminating 拿不到退出码
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy:      corev1.RestartPolicyNever,
					ServiceAccountName: "rlark-agent",
					Containers: []corev1.Container{{
						Name:  "upgrade",
						Image: job.Spec.Image,
						Env: []corev1.EnvVar{
							{Name: "TARGET_IMAGE", Value: job.Spec.Image},
							{Name: "ROLLBACK_IMAGE", Value: rollbackImage},
							{Name: "AUTO_ROLLBACK", Value: fmt.Sprintf("%v", autoRollback)},
							{Name: "HEALTH_CHECK_TIMEOUT", Value: fmt.Sprintf("%ds", timeout/2)},
						},
						VolumeMounts: volumeMounts,
					}},
					Volumes: volumes,
				},
			},
		},
	}

	if err := d.client.Create(ctx, k8sJob); err != nil {
		// If the job already exists (e.g. agent restarted after creating it),
		// return the existing job name so the caller can resume monitoring.
		if apierrors.IsAlreadyExists(err) {
			return jobName, nil
		}
		return "", fmt.Errorf("create upgrade job: %w", err)
	}
	return jobName, nil
}

// CheckStatus reads a batchv1.Job and maps its state to JobStatus.
func (d *K8sDriver) CheckStatus(ctx context.Context, jobName string) (JobStatus, error) {
	result, err := d.CheckJobResult(ctx, jobName)
	if err != nil {
		return "", err
	}
	return result.Status, nil
}

// CheckJobResult reads a batchv1.Job and returns its status plus the container
// exit code, so the caller can distinguish "upgrade failed, rollback succeeded"
// (exit code 2) from "upgrade failed, rollback also failed" (other non-zero).
func (d *K8sDriver) CheckJobResult(ctx context.Context, jobName string) (*JobResult, error) {
	var k8sJob batchv1.Job
	err := d.client.Get(ctx, types.NamespacedName{
		Namespace: d.namespace,
		Name:      jobName,
	}, &k8sJob)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return &JobResult{Status: JobStatusNotFound}, nil
		}
		return nil, err
	}

	if k8sJob.Status.Active > 0 {
		return &JobResult{Status: JobStatusRunning}, nil
	}
	if k8sJob.Status.Succeeded > 0 {
		return &JobResult{Status: JobStatusSucceeded, ExitCode: 0}, nil
	}
	if k8sJob.Status.Failed > 0 {
		// Read the exit code from the failed Pod.
		exitCode := d.readExitCodeFromPods(ctx, &k8sJob)
		return &JobResult{Status: JobStatusFailed, ExitCode: exitCode}, nil
	}
	return &JobResult{Status: JobStatusRunning}, nil
}

// readExitCodeFromPods finds a terminated Pod for the Job and returns its exit code.
func (d *K8sDriver) readExitCodeFromPods(ctx context.Context, k8sJob *batchv1.Job) int32 {
	logger := log.FromContext(ctx)
	selector, err := metav1.LabelSelectorAsSelector(k8sJob.Spec.Selector)
	if err != nil {
		logger.Info("failed to build selector from job", "job", k8sJob.Name, "error", err)
		return -1
	}
	var pods corev1.PodList
	if err := d.client.List(ctx, &pods, client.InNamespace(d.namespace), client.MatchingLabelsSelector{Selector: selector}); err != nil {
		logger.Info("failed to list pods for job", "job", k8sJob.Name, "error", err)
		return -1
	}
	logger.Info("found pods for job", "job", k8sJob.Name, "podCount", len(pods.Items))
	for _, pod := range pods.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Terminated != nil {
				logger.Info("found terminated container", "pod", pod.Name, "container", cs.Name, "exitCode", cs.State.Terminated.ExitCode)
				return cs.State.Terminated.ExitCode
			}
		}
	}
	logger.Info("no terminated container found", "job", k8sJob.Name, "podCount", len(pods.Items))
	return -1
}

// ─── ConfigMap-backed StateStore ──────────────────────────────────────────────

// ConfigMapStateStore stores ClusterState as a ConfigMap in the local K8s cluster.
type ConfigMapStateStore struct {
	client    client.Client
	namespace string
	name      string
}

// NewConfigMapStateStore creates a StateStore backed by a ConfigMap.
func NewConfigMapStateStore(c client.Client) *ConfigMapStateStore {
	return &ConfigMapStateStore{
		client:    c,
		namespace: StateStoreNamespace,
		name:      StateStoreName,
	}
}

// Get reads the ConfigMap and returns the extracted ClusterState.
func (s *ConfigMapStateStore) Get(ctx context.Context) (*ClusterState, error) {
	var cm corev1.ConfigMap
	err := s.client.Get(ctx, types.NamespacedName{
		Namespace: s.namespace,
		Name:      s.name,
	}, &cm)
	if err != nil {
		return nil, err // caller checks apierrors.IsNotFound
	}
	return &ClusterState{
		Image:        cm.Data["image"],
		DeployConfig: cm.Data["deploy-config"],
	}, nil
}

// Save creates or updates the ConfigMap with the given ClusterState.
// An empty DeployConfig means the user did not provide a config patch for this
// upgrade, so the existing deploy-config is left unchanged (it represents the
// last explicitly provided configuration).
func (s *ConfigMapStateStore) Save(ctx context.Context, state *ClusterState) error {
	var cm corev1.ConfigMap
	err := s.client.Get(ctx, types.NamespacedName{
		Namespace: s.namespace,
		Name:      s.name,
	}, &cm)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get state configmap: %w", err)
		}
		cm = corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.name,
				Namespace: s.namespace,
			},
			Data: map[string]string{},
		}
		if state.DeployConfig != "" {
			cm.Data["deploy-config"] = state.DeployConfig
		}
		cm.Data["image"] = state.Image
		if createErr := s.client.Create(ctx, &cm); createErr != nil {
			return fmt.Errorf("create state configmap: %w", createErr)
		}
		return nil
	}

	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data["image"] = state.Image
	if state.DeployConfig != "" {
		cm.Data["deploy-config"] = state.DeployConfig
	}
	return s.client.Update(ctx, &cm)
}
