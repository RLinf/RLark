package workflow

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

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
		return &job, nil
	}
	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("get Job %s: %w", jobName, err)
	}

	newJob := buildJob(wf, jt, jobName)
	if err := ctrl.SetControllerReference(wf, newJob, r.Scheme); err != nil {
		return nil, fmt.Errorf("set controller reference on Job %s: %w", jobName, err)
	}
	if err := r.Create(ctx, newJob); err != nil && !errors.IsAlreadyExists(err) {
		return nil, fmt.Errorf("create Job %s: %w", jobName, err)
	}

	logger.Info("Created Job for workflow", "job", jobName, "workflowJob", jt.Name)
	newJob.Status.Phase = rlarkv1alpha1.JobPhasePending
	return newJob, nil
}

func (r *Reconciler) reconcileWithStateMachine(
	ctx context.Context,
	wf *rlarkv1alpha1.Workflow,
) (bool, error) {
	f := newWorkflowStateMachine()
	f.SetState(string(wf.Status.Phase))

	changed := false

	if f.Can(EventInit) {
		if err := f.Event(ctx, EventInit, wf); err != nil {
			return false, err
		}
		changed = true
	}

	if f.Can(EventStart) {
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

	dagChanged, err := r.reconcileDAG(ctx, wf, log.FromContext(ctx))
	if err != nil {
		return false, err
	}
	if dagChanged {
		changed = true
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
	if s.AnyFailed {
		return EventAnyJobFailed
	}
	if s.AllSucceeded && s.HasItems {
		return EventAllJobsSucceeded
	}
	return ""
}
