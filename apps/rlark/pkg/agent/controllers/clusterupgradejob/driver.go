package clusterupgradejob

import (
	"context"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// UpgradeDriver abstracts all platform-specific operations needed to execute
// an upgrade on the local data plane. Implementations exist for Kubernetes
// (batchv1.Job + corev1.ConfigMap) and can be added for Docker / Raw modes.
//
// The reconciler (pullReconciler) is responsible for the state machine:
// start → monitor → success/failure → rollback. The driver provides the
// concrete primitives used at each step.
type UpgradeDriver interface {
	// StateStore returns the StateStore used to persist cluster state
	// (current image + deploy config) across upgrades.
	StateStore() StateStore

	// PrepareConfig persists the deploy config for an upgrade attempt.
	// It returns an opaque configRef that is passed to StartUpgrade.
	// The driver decides how to store it (e.g. ConfigMap in K8s mode).
	PrepareConfig(ctx context.Context, job *rlarkv1alpha1.ClusterUpgradeJob, config string) (configRef string, err error)

	// CleanupConfig removes the config created by PrepareConfig.
	// NotFound / already-gone is not an error.
	CleanupConfig(ctx context.Context, job *rlarkv1alpha1.ClusterUpgradeJob) error

	// StartUpgrade launches the upgrade workload.
	// It returns the driver-specific job name used later in CheckStatus.
	// rollbackImage is the image to fall back to (empty = no rollback possible).
	// configRef is the value returned by PrepareConfig.
	StartUpgrade(ctx context.Context, job *rlarkv1alpha1.ClusterUpgradeJob, rollbackImage, configRef string) (jobName string, err error)

	// CheckStatus returns the current state of a running or finished job.
	// It must return JobStatusNotFound when the job no longer exists (e.g. TTL-cleared).
	CheckStatus(ctx context.Context, jobName string) (JobStatus, error)
}

// JobResult extends JobStatus with the container exit code, so the caller
// can distinguish "upgrade failed, rollback succeeded" (exit code 2) from
// "upgrade failed, rollback also failed" (other non-zero exit codes).
type JobResult struct {
	Status   JobStatus
	ExitCode int32
}

// JobStatus represents the lifecycle state of a driver-managed upgrade/rollback job.
type JobStatus string

const (
	// JobStatusRunning means the job is still active.
	JobStatusRunning JobStatus = "Running"
	// JobStatusSucceeded means the job completed successfully.
	JobStatusSucceeded JobStatus = "Succeeded"
	// JobStatusFailed means the job finished with a failure.
	JobStatusFailed JobStatus = "Failed"
	// JobStatusNotFound means the job no longer exists.
	JobStatusNotFound JobStatus = "NotFound"
)

// ─── StateStore ───────────────────────────────────────────────────────────────

// ClusterState represents the current version and config of a data-plane cluster.
type ClusterState struct {
	// Image is the currently deployed rlark-upgrade image tag.
	Image string
	// DeployConfig is the full deploy-config YAML captured at last upgrade time.
	DeployConfig string
}

// StateStore persists and retrieves the ClusterState for the local data plane.
type StateStore interface {
	// Get returns the current ClusterState.
	// It returns an error matching apierrors.IsNotFound when no state exists yet.
	Get(ctx context.Context) (*ClusterState, error)

	// Save persists the ClusterState, creating the underlying resource if needed.
	Save(ctx context.Context, state *ClusterState) error
}

const (
	StateStoreName      = "rlark-state"
	StateStoreNamespace = "rlark-system"
)

func upgradeConfigMapName(jobName string) string {
	return "upgrade-" + jobName + "-config"
}
