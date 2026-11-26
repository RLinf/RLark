// Package upgrade provides the control plane controller for UpgradeJob.
// It creates ClusterUpgradeJob for each target cluster and aggregates status.
package upgrade

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const (
	// FinalizerName is the finalizer for UpgradeJob cleanup.
	FinalizerName = "upgradejobs.rlinf.io/cleanup"
)

// Reconciler reconciles UpgradeJob resources.
// It creates ClusterUpgradeJob for each target cluster and aggregates status.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=rlinf.io,resources=upgradejobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=upgradejobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=upgradejobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=rlinf.io,resources=clusterupgradejobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rlinf.io,resources=clusterupgradejobs/status,verbs=get
// +kubebuilder:rbac:groups=rlinf.io,resources=clusters,verbs=get;list;watch
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var upgradeJob rlarkv1alpha1.UpgradeJob
	if err := r.Get(ctx, req.NamespacedName, &upgradeJob); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion
	if !upgradeJob.DeletionTimestamp.IsZero() {
		return r.reconcileDeletion(ctx, &upgradeJob)
	}

	// Add finalizer
	if !controllerutil.ContainsFinalizer(&upgradeJob, FinalizerName) {
		controllerutil.AddFinalizer(&upgradeJob, FinalizerName)
		if err := r.Update(ctx, &upgradeJob); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
		return ctrl.Result{}, nil
	}

	// Initialize StartTime on first reconcile
	if upgradeJob.Status.StartTime == nil {
		upgradeJob.Status.StartTime = &metav1.Time{Time: time.Now()}
		if err := r.Status().Update(ctx, &upgradeJob); err != nil {
			return ctrl.Result{}, fmt.Errorf("initialize start time: %w", err)
		}
	}

	// Aggregate status first so that reconcileClusterUpgradeJobs sees the latest
	// failure counts before deciding whether to create more jobs.
	if result, err := r.aggregateStatus(ctx, &upgradeJob); err != nil {
		return result, err
	}

	// Create ClusterUpgradeJob for target clusters based on the latest status.
	if err := r.reconcileClusterUpgradeJobs(ctx, &upgradeJob); err != nil {
		return ctrl.Result{}, err
	}

	// Re-aggregate after creating new jobs so the status reflects the latest state.
	return r.aggregateStatus(ctx, &upgradeJob)
}

// reconcileDeletion handles UpgradeJob deletion.
func (r *Reconciler) reconcileDeletion(ctx context.Context, job *rlarkv1alpha1.UpgradeJob) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(job, FinalizerName) {
		// Delete all ClusterUpgradeJobs owned by this UpgradeJob
		if err := r.deleteClusterUpgradeJobs(ctx, job.Name); err != nil {
			return ctrl.Result{}, err
		}

		controllerutil.RemoveFinalizer(job, FinalizerName)
		if err := r.Update(ctx, job); err != nil {
			return ctrl.Result{}, fmt.Errorf("remove finalizer: %w", err)
		}
	}
	return ctrl.Result{}, nil
}

// reconcileClusterUpgradeJobs creates ClusterUpgradeJob for target clusters,
// respecting the concurrency limit in the upgrade strategy.
func (r *Reconciler) reconcileClusterUpgradeJobs(ctx context.Context, job *rlarkv1alpha1.UpgradeJob) error {
	// If any cluster failed and failure tolerance is 0 (default), stop creating.
	// If failure tolerance is > 0, check whether the rate exceeds the threshold.
	if job.Status.FailedClusters > 0 {
		if job.Spec.Strategy.FailureTolerate == 0 {
			return nil
		}
		failureRate := float64(job.Status.FailedClusters) / float64(job.Status.TotalClusters)
		if failureRate > job.Spec.Strategy.FailureTolerate {
			return nil
		}
	}

	clusters, err := r.getTargetClusters(ctx, job)
	if err != nil {
		return fmt.Errorf("get target clusters: %w", err)
	}

	var existingJobs rlarkv1alpha1.ClusterUpgradeJobList
	if err := r.List(ctx, &existingJobs, client.MatchingLabels{"rlark.io/upgrade-job": job.Name}); err != nil {
		return fmt.Errorf("list cluster upgrade jobs: %w", err)
	}

	existingByCluster := make(map[string]struct{}, len(existingJobs.Items))
	active := 0
	for _, cuj := range existingJobs.Items {
		existingByCluster[cuj.Labels["rlark.io/cluster"]] = struct{}{}
		if !isTerminalPhase(cuj.Status.Phase) {
			active++
		}
	}

	remaining := max(0, int(job.Spec.Strategy.Concurrency)-active)

	for _, clusterName := range clusters {
		if remaining == 0 {
			break
		}

		if _, exists := existingByCluster[clusterName]; exists {
			continue
		}

		clusterUpgradeJobName := fmt.Sprintf("%s-%s", job.Name, clusterName)
		clusterNamespace := clusterName

		clusterUpgradeJob := &rlarkv1alpha1.ClusterUpgradeJob{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: clusterNamespace,
				Name:      clusterUpgradeJobName,
				Labels: map[string]string{
					"rlark.io/upgrade-job": job.Name,
					"rlark.io/cluster":     clusterName,
				},
			},
			Spec: rlarkv1alpha1.ClusterUpgradeJobSpec{
				UpgradeJobName: job.Name,
				Image:          job.Spec.Image,
				ConfigPatch:    job.Spec.ConfigPatch,
				TimeoutSeconds: job.Spec.Strategy.TimeoutSeconds,
				AutoRollback:   job.Spec.Strategy.AutoRollback,
			},
		}

		if err := controllerutil.SetControllerReference(job, clusterUpgradeJob, r.Scheme); err != nil {
			return fmt.Errorf("set owner reference: %w", err)
		}

		if err := r.Create(ctx, clusterUpgradeJob); err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			return fmt.Errorf("create cluster upgrade job: %w", err)
		}
		remaining--
	}

	return nil
}

// isTerminalPhase returns true if the ClusterUpgradeJob phase is a terminal state.
func isTerminalPhase(phase rlarkv1alpha1.ClusterUpgradeJobPhase) bool {
	return phase == rlarkv1alpha1.ClusterUpgradeJobSucceeded ||
		phase == rlarkv1alpha1.ClusterUpgradeJobFailed ||
		phase == rlarkv1alpha1.ClusterUpgradeJobRolledBack
}

func (r *Reconciler) aggregateStatus(ctx context.Context, job *rlarkv1alpha1.UpgradeJob) (ctrl.Result, error) {
	var clusterUpgradeJobs rlarkv1alpha1.ClusterUpgradeJobList
	if err := r.List(ctx, &clusterUpgradeJobs, client.MatchingLabels{
		"rlark.io/upgrade-job": job.Name,
	}); err != nil {
		return ctrl.Result{}, fmt.Errorf("list cluster upgrade jobs: %w", err)
	}

	var upgraded, failed, rolledBack, upgrading int32
	conditions := make([]rlarkv1alpha1.UpgradeJobCondition, 0, len(clusterUpgradeJobs.Items))
	for _, cuj := range clusterUpgradeJobs.Items {
		clusterName := cuj.Labels["rlark.io/cluster"]
		conditions = append(conditions, rlarkv1alpha1.UpgradeJobCondition{
			Type:        "ClusterUpgrade",
			Cluster:     clusterName,
			Status:      string(cuj.Status.Phase),
			Reason:      cuj.Status.Message,
			TargetImage: cuj.Spec.Image,
			UpgradeTime: cuj.Status.CompletionTime,
		})

		switch cuj.Status.Phase {
		case rlarkv1alpha1.ClusterUpgradeJobSucceeded:
			upgraded++
		case rlarkv1alpha1.ClusterUpgradeJobFailed:
			failed++
		case rlarkv1alpha1.ClusterUpgradeJobRolledBack:
			rolledBack++
		case rlarkv1alpha1.ClusterUpgradeJobUpgrading:
			upgrading++
		}
	}

	targetClusters, err := r.getTargetClusters(ctx, job)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("get target clusters: %w", err)
	}
	job.Status.TotalClusters = int32(len(targetClusters))
	job.Status.UpgradingClusters = upgrading
	job.Status.UpgradedClusters = upgraded
	job.Status.FailedClusters = failed
	job.Status.RollbackClusters = rolledBack
	job.Status.Conditions = conditions

	r.updatePhase(job)

	return ctrl.Result{}, r.Status().Update(ctx, job)
}

// getTargetClusters returns the list of target cluster names.
func (r *Reconciler) getTargetClusters(ctx context.Context, job *rlarkv1alpha1.UpgradeJob) ([]string, error) {
	// If ClusterNames is specified, use that
	if len(job.Spec.ClusterNames) > 0 {
		return job.Spec.ClusterNames, nil
	}

	// Otherwise, get clusters matching the selector
	var clusters rlarkv1alpha1.ClusterList
	if job.Spec.ClusterSelector != nil {
		selector, err := metav1.LabelSelectorAsSelector(job.Spec.ClusterSelector)
		if err != nil {
			return nil, fmt.Errorf("parse cluster selector: %w", err)
		}

		if err := r.List(ctx, &clusters, client.MatchingLabelsSelector{Selector: selector}); err != nil {
			return nil, fmt.Errorf("list clusters: %w", err)
		}
	} else {
		// No selector, get all clusters
		if err := r.List(ctx, &clusters); err != nil {
			return nil, fmt.Errorf("list clusters: %w", err)
		}
	}

	clusterNames := make([]string, 0, len(clusters.Items))
	for _, cluster := range clusters.Items {
		clusterNames = append(clusterNames, cluster.Name)
	}

	return clusterNames, nil
}

// deleteClusterUpgradeJobs deletes all ClusterUpgradeJobs for an UpgradeJob.
func (r *Reconciler) deleteClusterUpgradeJobs(ctx context.Context, upgradeJobName string) error {
	var clusterUpgradeJobs rlarkv1alpha1.ClusterUpgradeJobList
	if err := r.List(ctx, &clusterUpgradeJobs, client.MatchingLabels{
		"rlark.io/upgrade-job": upgradeJobName,
	}); err != nil {
		return err
	}

	for i := range clusterUpgradeJobs.Items {
		if err := r.Delete(ctx, &clusterUpgradeJobs.Items[i]); err != nil {
			if !apierrors.IsNotFound(err) {
				return err
			}
		}
	}

	return nil
}

// updatePhase updates the overall phase based on aggregated status.
func (r *Reconciler) updatePhase(job *rlarkv1alpha1.UpgradeJob) {
	// Count completed clusters
	completed := job.Status.UpgradedClusters + job.Status.FailedClusters + job.Status.RollbackClusters

	// If any cluster is still upgrading, show Upgrading regardless of failures.
	if job.Status.UpgradingClusters > 0 {
		job.Status.Phase = rlarkv1alpha1.UpgradeJobUpgrading
		return
	}

	// Check if all clusters completed
	if completed >= job.Status.TotalClusters && job.Status.TotalClusters > 0 {
		if job.Status.FailedClusters > 0 {
			job.Status.Phase = rlarkv1alpha1.UpgradeJobFailed
		} else if job.Status.RollbackClusters > 0 {
			job.Status.Phase = rlarkv1alpha1.UpgradeJobRollingBack
		} else {
			job.Status.Phase = rlarkv1alpha1.UpgradeJobSucceeded
		}
		job.Status.CompletionTime = &metav1.Time{Time: time.Now()}
		return
	}

	// Check failure tolerance
	if job.Status.FailedClusters > 0 && job.Status.TotalClusters > 0 {
		failureRate := float64(job.Status.FailedClusters) / float64(job.Status.TotalClusters)
		if failureRate > job.Spec.Strategy.FailureTolerate {
			job.Status.Phase = rlarkv1alpha1.UpgradeJobFailed
			job.Status.CompletionTime = &metav1.Time{Time: time.Now()}
			return
		}
	}

	// Some clusters completed but none are actively upgrading
	if completed > 0 {
		job.Status.Phase = rlarkv1alpha1.UpgradeJobUpgrading
	} else {
		job.Status.Phase = rlarkv1alpha1.UpgradeJobPending
	}
}

// SetupWithManager registers the controller with the manager.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.UpgradeJob{}).
		Watches(&rlarkv1alpha1.ClusterUpgradeJob{}, handler.EnqueueRequestsFromMapFunc(r.upgradeJobForClusterUpgradeJob)).
		Named("upgrade").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}

// upgradeJobForClusterUpgradeJob returns the UpgradeJob for a ClusterUpgradeJob.
func (r *Reconciler) upgradeJobForClusterUpgradeJob(ctx context.Context, object client.Object) []reconcile.Request {
	clusterUpgradeJob, ok := object.(*rlarkv1alpha1.ClusterUpgradeJob)
	if !ok {
		return nil
	}

	upgradeJobName := clusterUpgradeJob.Labels["rlark.io/upgrade-job"]
	if upgradeJobName == "" {
		return nil
	}

	return []reconcile.Request{
		{NamespacedName: client.ObjectKey{Name: upgradeJobName}},
	}
}
