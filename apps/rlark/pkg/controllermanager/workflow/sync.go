package workflow

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const workflowLabel = "rlinf.io/workflow"

func (r *Reconciler) syncJobStatuses(
	ctx context.Context,
	wf *rlarkv1alpha1.Workflow,
) (bool, error) {
	statusMap := buildJobStatusMap(wf)
	changed := false

	for _, jt := range wf.Spec.JobTemplates {
		js := statusMap[jt.Name]
		if js == nil || js.Phase == "" {
			continue
		}

		var job rlarkv1alpha1.Job
		err := r.Get(ctx, types.NamespacedName{Name: utils.ChildName(wf.Name, jt.Name)}, &job)
		if err != nil {
			if errors.IsNotFound(err) {
				continue
			}
			return false, fmt.Errorf("get Job %s: %w", jt.Name, err)
		}
		if _, _, err := utils.ClassifyChild(&job, wf.UID, rlarkv1alpha1.GroupVersion.String(), "Workflow", workflowLabel, wf.Name, jt.Name); err != nil {
			return false, fmt.Errorf("job %s ownership conflict: %w", job.Name, err)
		}
		if utils.SyncStatusEntry(js, string(job.Status.Phase), "") {
			changed = true
		}
	}

	return changed, nil
}

func (r *Reconciler) reconcileDAG(
	ctx context.Context,
	wf *rlarkv1alpha1.Workflow,
	logger logr.Logger,
) (bool, error) {
	d, err := newDAG(wf.Spec.JobTemplates)
	if err != nil {
		return false, fmt.Errorf("invalid DAG: %w", err)
	}

	jobStatusMap := buildJobStatusMap(wf)

	for _, jt := range wf.Spec.JobTemplates {
		if js := jobStatusMap[jt.Name]; js != nil && js.Phase == rlarkv1alpha1.JobPhaseSucceeded {
			d.resolve(jt.Name)
		}
	}

	statusChanged := false
	for _, name := range d.dispatchReady(jobStatusMap) {
		jt := d.templates[name]
		js := jobStatusMap[name]

		job, err := r.reconcileJob(ctx, wf, jt, logger)
		if err != nil {
			return false, err
		}
		if utils.SyncStatusEntry(js, string(job.Status.Phase), "") {
			statusChanged = true
		}
	}

	return statusChanged, nil
}

func (r *Reconciler) reconcileJob(
	ctx context.Context,
	wf *rlarkv1alpha1.Workflow,
	jt rlarkv1alpha1.WorkflowJobTemplate,
	logger logr.Logger,
) (*rlarkv1alpha1.Job, error) {
	jobName := utils.ChildName(wf.Name, jt.Name)

	var job rlarkv1alpha1.Job
	err := r.Get(ctx, types.NamespacedName{Name: jobName}, &job)
	if err == nil {
		if err := r.ensureJobOwnership(ctx, wf, jt.Name, &job); err != nil {
			return nil, err
		}
		if !job.DeletionTimestamp.IsZero() {
			return nil, controller.ErrRequeueAfterChildCleanup
		}
		if !jobTerminal(&job) {
			desired := buildJob(wf, jt, jobName)
			marker := string(wf.UID)
			workflowStopped := wf.Spec.Stopped || job.Annotations[rlarkv1alpha1.WorkflowStoppedByAnnotation] == marker
			if workflowStopped {
				desired.Spec.Stopped = true
				desired.Annotations[rlarkv1alpha1.WorkflowStoppedByAnnotation] = marker
			}
			if !reflect.DeepEqual(job.Spec, desired.Spec) ||
				(workflowStopped && job.Annotations[rlarkv1alpha1.WorkflowStoppedByAnnotation] != marker) {
				job.Spec = desired.Spec
				job.Annotations = utils.MergeAnnotations(job.Annotations, desired.Annotations)
				if err := r.Update(ctx, &job); err != nil {
					return nil, fmt.Errorf("update Job %s: %w", jobName, err)
				}
				logger.Info("Updated Job for workflow", "job", jobName, "workflowJob", jt.Name)
			}
		}
		return &job, nil
	}
	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("get Job %s: %w", jobName, err)
	}

	newJob := buildJob(wf, jt, jobName)
	if err := ctrl.SetControllerReference(wf, newJob, r.Scheme); err != nil {
		return nil, fmt.Errorf("set controller reference on Job %s: %w", jobName, err)
	}
	if err := r.Create(ctx, newJob); err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("create Job %s: %w", jobName, err)
		}
		if err := r.Get(ctx, types.NamespacedName{Name: jobName}, &job); err != nil {
			return nil, fmt.Errorf("reload Job %s after create race: %w", jobName, err)
		}
		if err := r.ensureJobOwnership(ctx, wf, jt.Name, &job); err != nil {
			return nil, err
		}
		return &job, nil
	}

	logger.Info("Created Job for workflow", "job", jobName, "workflowJob", jt.Name)
	newJob.Status.Phase = rlarkv1alpha1.JobPhasePending
	return newJob, nil
}

func jobTerminal(job *rlarkv1alpha1.Job) bool {
	return job.Status.Phase == rlarkv1alpha1.JobPhaseSucceeded || job.Status.Phase == rlarkv1alpha1.JobPhaseFailed
}

func (r *Reconciler) ensureJobOwnership(ctx context.Context, wf *rlarkv1alpha1.Workflow, template string, job *rlarkv1alpha1.Job) error {
	owned, legacy, err := utils.ClassifyChild(job, wf.UID, rlarkv1alpha1.GroupVersion.String(), "Workflow", workflowLabel, wf.Name, template)
	if err != nil || !owned {
		return fmt.Errorf("job %s ownership conflict: %w", job.Name, err)
	}
	if !legacy {
		return nil
	}
	job.Annotations = utils.MergeAnnotations(job.Annotations, map[string]string{
		utils.ParentUIDAnnotation: string(wf.UID), utils.ChildTemplateAnnotation: template,
	})
	if err := ctrl.SetControllerReference(wf, job, r.Scheme); err != nil {
		return fmt.Errorf("adopt Job %s: %w", job.Name, err)
	}
	if err := r.Update(ctx, job); err != nil {
		return fmt.Errorf("adopt Job %s: %w", job.Name, err)
	}
	return nil
}

func (r *Reconciler) reconcileWithStateMachine(
	ctx context.Context,
	wf *rlarkv1alpha1.Workflow,
) (bool, error) {
	f := newWorkflowStateMachine()
	f.SetState(string(wf.Status.Phase))

	oldTemplates := make([]string, len(wf.Status.Jobs))
	for i := range wf.Status.Jobs {
		oldTemplates[i] = wf.Status.Jobs[i].Name
	}
	changed := syncJobStatusSnapshot(wf)
	waiting, err := r.pruneJobsWithTemplates(ctx, wf, oldTemplates)
	if err != nil {
		return false, err
	}
	if waiting {
		return changed, controller.ErrRequeueAfterChildCleanup
	}

	if f.Can(EventInit) {
		if err := f.Event(ctx, EventInit, wf); err != nil {
			return false, err
		}
		changed = true
	}

	if wf.Spec.Stopped && f.Can(EventStop) {
		if err := f.Event(ctx, EventStop, wf); err != nil {
			return false, err
		}
		changed = true
	}

	resuming := !wf.Spec.Stopped && f.Can(EventResume)
	if resuming {
		if err := f.Event(ctx, EventResume, wf); err != nil {
			return false, err
		}
		wf.Status.Jobs = nil
		changed = true
		return changed, nil
	}

	if !wf.Spec.Stopped && f.Can(EventStart) {
		if err := f.Event(ctx, EventStart, wf); err != nil {
			return false, err
		}
		changed = true
	}

	syncChanged, err := r.syncJobStatuses(ctx, wf)
	if err != nil {
		return false, err
	}
	if syncChanged {
		changed = true
	}
	if wf.Spec.Stopped {
		waiting, err := r.deleteOwnedJobs(ctx, wf)
		if err != nil {
			return false, err
		}
		if waiting {
			return changed, controller.ErrRequeueAfterChildCleanup
		}
		if markJobStatusesStopped(wf) {
			changed = true
		}
		if f.Can(EventAllJobsStopped) {
			if err := f.Event(ctx, EventAllJobsStopped, wf); err != nil {
				return false, err
			}
			changed = true
		}
		return changed, nil
	}

	if !wf.Spec.Stopped && wf.Status.Phase == rlarkv1alpha1.WorkflowPhaseRunning {
		dagChanged, err := r.reconcileDAG(ctx, wf, log.FromContext(ctx))
		if err != nil {
			return false, err
		}
		if dagChanged {
			changed = true
		}
	}

	event := r.evaluateWorkflowEvent(wf)
	if event != "" && f.Can(event) {
		if err := f.Event(ctx, event, wf); err != nil {
			return false, err
		}
		changed = true
	}

	return changed, nil
}

func markJobStatusesStopped(wf *rlarkv1alpha1.Workflow) bool {
	changed := syncJobStatusSnapshot(wf)
	for i := range wf.Status.Jobs {
		switch wf.Status.Jobs[i].Phase {
		case rlarkv1alpha1.JobPhaseSucceeded, rlarkv1alpha1.JobPhaseFailed, rlarkv1alpha1.JobPhaseStopped:
			continue
		default:
			wf.Status.Jobs[i].Phase = rlarkv1alpha1.JobPhaseStopped
			changed = true
		}
	}
	return changed
}

func (r *Reconciler) pruneJobs(ctx context.Context, wf *rlarkv1alpha1.Workflow) (bool, error) {
	oldTemplates := make([]string, len(wf.Status.Jobs))
	for i := range wf.Status.Jobs {
		oldTemplates[i] = wf.Status.Jobs[i].Name
	}
	return r.pruneJobsWithTemplates(ctx, wf, oldTemplates)
}

func (r *Reconciler) pruneJobsWithTemplates(ctx context.Context, wf *rlarkv1alpha1.Workflow, oldTemplates []string) (bool, error) {
	desired := make(map[string]string, len(wf.Spec.JobTemplates))
	for _, template := range wf.Spec.JobTemplates {
		desired[utils.ChildName(wf.Name, template.Name)] = template.Name
	}
	var jobs rlarkv1alpha1.JobList
	if err := r.List(ctx, &jobs, client.MatchingLabels{workflowLabel: wf.Name}); err != nil {
		return false, fmt.Errorf("list Jobs for Workflow %s: %w", wf.Name, err)
	}
	waiting := false
	for i := range jobs.Items {
		job := &jobs.Items[i]
		template, keep := desired[job.Name]
		if keep {
			if _, _, err := utils.ClassifyChild(job, wf.UID, rlarkv1alpha1.GroupVersion.String(), "Workflow", workflowLabel, wf.Name, template); err != nil {
				return false, fmt.Errorf("job %s ownership conflict: %w", job.Name, err)
			}
			continue
		}
		owned, _ := utils.ClassifyRemovedChild(job, wf.UID, rlarkv1alpha1.GroupVersion.String(), "Workflow", workflowLabel, wf.Name, oldTemplates)
		if owned {
			waiting = true
			if err := r.Delete(ctx, job); client.IgnoreNotFound(err) != nil {
				return false, fmt.Errorf("prune Job %s: %w", job.Name, err)
			}
		}
	}
	return waiting, nil
}

func (r *Reconciler) evaluateWorkflowEvent(wf *rlarkv1alpha1.Workflow) string {
	phases := make([]string, len(wf.Status.Jobs))
	for i, js := range wf.Status.Jobs {
		phases[i] = string(js.Phase)
	}
	s := utils.SummarizePhases(phases,
		string(rlarkv1alpha1.JobPhaseSucceeded),
		string(rlarkv1alpha1.JobPhaseFailed),
		string(rlarkv1alpha1.JobPhaseRunning),
		string(rlarkv1alpha1.JobPhaseStopped),
	)
	if wf.Spec.Stopped && (!s.HasItems || s.AllStopped) {
		return EventAllJobsStopped
	}
	if s.AnyFailed {
		return EventAnyJobFailed
	}
	if s.AllSucceeded && s.HasItems {
		return EventAllJobsSucceeded
	}
	return ""
}
