package workflow

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const workflowLabel = "rlinf.io/workflow"

// jobWorkflowField indexes Jobs by the Workflow they carry evidence for: the
// canonical workflowLabel and, for Jobs that predate it, the controller
// OwnerReference. Both forms are proof of ownership, so both must be indexed.
const jobWorkflowField = "rlinf.io/workflow-owner"

// workflowParentRef is the single ownership identity Jobs are classified
// against.
func workflowParentRef(wf *rlarkv1alpha1.Workflow) utils.ParentRef {
	return utils.ParentRef{
		UID:        wf.UID,
		APIVersion: rlarkv1alpha1.GroupVersion.String(),
		Kind:       "Workflow",
		Name:       wf.Name,
		Label:      workflowLabel,
	}
}

// indexJobByWorkflow is the field indexer function registered for
// jobWorkflowField.
func indexJobByWorkflow(obj client.Object) []string {
	job, ok := obj.(*rlarkv1alpha1.Job)
	if !ok {
		return nil
	}
	return utils.ChildIndexKeys(job, rlarkv1alpha1.GroupVersion.String(), "Workflow", workflowLabel)
}

// listWorkflowJobs returns the Jobs that carry ownership evidence for wf. The
// lookup goes through jobWorkflowField so the controller does not scan every
// Job in the cluster. Membership is only narrowed here; ownership still has to
// be proven by the caller from authoritative metadata.
func (r *Reconciler) listWorkflowJobs(ctx context.Context, wf *rlarkv1alpha1.Workflow) (rlarkv1alpha1.JobList, error) {
	var jobs rlarkv1alpha1.JobList
	if err := r.List(ctx, &jobs, client.MatchingFields{jobWorkflowField: wf.Name}); err != nil {
		return jobs, fmt.Errorf("list Jobs for Workflow %s: %w", wf.Name, err)
	}
	return jobs, nil
}

// legacyJobNames are the Job names this Workflow could have created before the
// canonical ownership annotations existed. Child names are derived here rather
// than reversed from the object, so a Job name is never treated as proof of
// ownership on its own.
func legacyJobNames(wf *rlarkv1alpha1.Workflow) []string {
	names := make([]string, len(wf.Status.Jobs))
	for i := range wf.Status.Jobs {
		names[i] = utils.ChildName(wf.Name, wf.Status.Jobs[i].Name)
	}
	return names
}

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
		if _, err := utils.ClassifyChild(&job, workflowParentRef(wf), jt.Name); err != nil {
			return false, fmt.Errorf("job %s ownership conflict: %w", job.Name, err)
		}
		if utils.SyncStatusEntry(js, job.Status.Phase, "") {
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
		if utils.SyncStatusEntry(js, job.Status.Phase, "") {
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
			// Only a running Workflow dispatches Jobs, so the desired Spec here
			// always describes a Job that should run: a Job that an earlier stop
			// suspended is resumed by the same comparison that reconciles every
			// other field.
			desired := buildJob(wf, jt, jobName)
			if !reflect.DeepEqual(job.Spec, desired.Spec) {
				// Patch instead of Update so only the fields this controller
				// decided to change are submitted. A full replacement would
				// also resubmit every field the local copy happens to hold,
				// silently reverting a disjoint concurrent write.
				before := job.DeepCopy()
				job.Spec = desired.Spec
				job.Annotations = utils.MergeAnnotations(job.Annotations, desired.Annotations)
				if err := r.Patch(ctx, &job, client.MergeFrom(before)); err != nil {
					return nil, fmt.Errorf("patch Job %s: %w", jobName, err)
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

// jobStopped reports whether a Job needs no further stopping, either because it
// reached a terminal phase on its own or because it reports that it stopped.
func jobStopped(job *rlarkv1alpha1.Job) bool {
	return jobTerminal(job) || job.Status.Phase == rlarkv1alpha1.JobPhaseStopped
}

// stopOwnedJobs propagates a suspended Workflow's stop intent to the Jobs it
// owns and reports whether all of them have come to rest.
//
// Suspending a Workflow is a durable desired-state switch, not a request to
// delete its children: the Jobs are kept so a later resume can pick the run up
// again, and each one that is still active is asked to suspend itself instead.
// Because nothing is deleted, this path never waits for child cleanup, so a Job
// whose Tasks the Agent has not reported as stopped yet cannot stop the
// Workflow from recording the Spec it observed.
func (r *Reconciler) stopOwnedJobs(ctx context.Context, wf *rlarkv1alpha1.Workflow) (bool, error) {
	jobs, err := r.listWorkflowJobs(ctx, wf)
	if err != nil {
		return false, err
	}

	parent := workflowParentRef(wf)
	legacyNames := legacyJobNames(wf)
	allStopped := true
	for i := range jobs.Items {
		job := &jobs.Items[i]
		decision, err := utils.ClassifyRemovedChild(job, parent, legacyNames)
		if err != nil {
			return false, fmt.Errorf("job %s ownership conflict: %w", job.Name, err)
		}
		if !decision.Owned() {
			continue
		}
		if jobStopped(job) {
			// Already at rest. A Job that reached Succeeded or Failed on its
			// own keeps that outcome: asking it to suspend afterwards would
			// only overwrite a finished run's result with a stop.
			continue
		}
		allStopped = false
		if !job.DeletionTimestamp.IsZero() || job.Spec.Suspended {
			continue
		}
		// Stopping changes exactly one Spec field, so patch it instead of
		// resubmitting every field this copy happens to hold.
		before := job.DeepCopy()
		job.Spec.Suspended = true
		if err := r.Patch(ctx, job, client.MergeFrom(before)); err != nil {
			return false, fmt.Errorf("suspend Job %s: %w", job.Name, err)
		}
	}
	return allStopped, nil
}

func (r *Reconciler) ensureJobOwnership(ctx context.Context, wf *rlarkv1alpha1.Workflow, template string, job *rlarkv1alpha1.Job) error {
	decision, err := utils.ClassifyChild(job, workflowParentRef(wf), template)
	if err != nil {
		return fmt.Errorf("job %s ownership conflict: %w", job.Name, err)
	}
	if !decision.Legacy() {
		return nil
	}
	before := job.DeepCopy()
	// Backfill the Workflow label as well, so the adopted Job stays
	// discoverable by the label selector the prune path lists with.
	if job.Labels == nil {
		job.Labels = map[string]string{}
	}
	job.Labels[workflowLabel] = wf.Name
	job.Annotations = utils.MergeAnnotations(job.Annotations, map[string]string{
		utils.ParentUIDAnnotation: string(wf.UID), utils.ChildTemplateAnnotation: template,
	})
	if err := ctrl.SetControllerReference(wf, job, r.Scheme); err != nil {
		return fmt.Errorf("adopt Job %s: %w", job.Name, err)
	}
	if err := r.Patch(ctx, job, client.MergeFrom(before)); err != nil {
		return fmt.Errorf("adopt Job %s: %w", job.Name, err)
	}
	return nil
}

func (r *Reconciler) reconcileWithStateMachine(
	ctx context.Context,
	wf *rlarkv1alpha1.Workflow,
) (bool, error) {
	logger := log.FromContext(ctx).WithValues("workflow", wf.Name)

	// An invalid Spec cannot be fixed by retrying: record the conclusion in the
	// Validated condition and enter Failed instead of requeueing forever.
	if !wf.Spec.Suspended {
		if _, err := newDAG(wf.Spec.JobTemplates); err != nil {
			logger.Error(err, "invalid Workflow DAG")
			changed := markWorkflowInvalid(wf, err.Error())
			if markWorkflowProgress(wf) {
				changed = true
			}
			return changed, nil
		}
	}

	f := newWorkflowStateMachine()
	f.SetState(string(wf.Status.Phase))

	legacyNames := legacyJobNames(wf)
	changed := syncJobStatusSnapshot(wf)
	if !wf.Spec.Suspended && markWorkflowValid(wf) {
		changed = true
	}
	waiting, err := r.pruneJobsWithTemplates(ctx, wf, legacyNames)
	if err != nil {
		return false, err
	}
	if waiting {
		if markWorkflowProgress(wf) {
			changed = true
		}
		return changed, controller.ErrRequeueAfterChildCleanup
	}

	if f.Can(EventInit) {
		if err := f.Event(ctx, EventInit, wf); err != nil {
			return false, err
		}
		changed = true
	}

	if wf.Spec.Suspended && f.Can(EventStop) {
		if err := f.Event(ctx, EventStop, wf); err != nil {
			return false, err
		}
		changed = true
	}

	resuming := !wf.Spec.Suspended && f.Can(EventResume)
	if resuming {
		if err := f.Event(ctx, EventResume, wf); err != nil {
			return false, err
		}
		wf.Status.Jobs = nil
		changed = true
		if markWorkflowProgress(wf) {
			changed = true
		}
		return changed, nil
	}

	if !wf.Spec.Suspended && f.Can(EventStart) {
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
	if wf.Spec.Suspended {
		allStopped, err := r.stopOwnedJobs(ctx, wf)
		if err != nil {
			return false, err
		}
		if allStopped {
			// Every remaining Job status without a terminal phase belongs to a
			// template that never produced a Job, so it has nothing left to run.
			if markJobStatusesStopped(wf) {
				changed = true
			}
			if f.Can(EventAllJobsStopped) {
				if err := f.Event(ctx, EventAllJobsStopped, wf); err != nil {
					return false, err
				}
				changed = true
			}
		}
		if markWorkflowProgress(wf) {
			changed = true
		}
		return changed, nil
	}

	if wf.Status.Phase == rlarkv1alpha1.WorkflowPhaseRunning {
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

	if markWorkflowProgress(wf) {
		changed = true
	}

	return changed, nil
}

// workflowConditionValidated is the type of the condition recording spec validation.
const workflowConditionValidated = rlarkv1alpha1.ConditionTypeValidated

// Reason values for the Workflow Validated condition. They are stable,
// CamelCase identifiers consumed by clients.
const (
	workflowReasonSpecValidated = "SpecValidated"
	workflowReasonInvalidDAG    = "InvalidJobDAG"
)

// markWorkflowInvalid marks the Workflow as Failed due to an invalid spec and
// records the reason in a "Validated" condition. It returns whether the
// Workflow object changed.
func markWorkflowInvalid(wf *rlarkv1alpha1.Workflow, message string) bool {
	changed := false

	if wf.Status.Phase != rlarkv1alpha1.WorkflowPhaseFailed {
		wf.Status.Phase = rlarkv1alpha1.WorkflowPhaseFailed
		changed = true
	}
	if wf.Status.EndTime == nil {
		now := metav1.Now()
		wf.Status.EndTime = &now
		changed = true
	}
	if apimeta.SetStatusCondition(&wf.Status.Conditions, metav1.Condition{
		Type:               workflowConditionValidated,
		Status:             metav1.ConditionFalse,
		Reason:             workflowReasonInvalidDAG,
		Message:            message,
		ObservedGeneration: wf.Generation,
	}) {
		changed = true
	}

	return changed
}

// markWorkflowValid records that the current spec passed validation, clearing a
// stale Validated=False left behind by a previously invalid spec.
func markWorkflowValid(wf *rlarkv1alpha1.Workflow) bool {
	return apimeta.SetStatusCondition(&wf.Status.Conditions, metav1.Condition{
		Type:               workflowConditionValidated,
		Status:             metav1.ConditionTrue,
		Reason:             workflowReasonSpecValidated,
		ObservedGeneration: wf.Generation,
	})
}

// workflowConditionProgressing is the type of the condition recording that the
// controller is still driving the Workflow towards its spec.
const workflowConditionProgressing = rlarkv1alpha1.ConditionTypeProgressing

// Reason values for the Workflow Progressing condition. Each names the action
// the controller is currently taking, not the state it is in.
const (
	workflowReasonAwaitingJobs = "AwaitingJobs"
	workflowReasonJobsRunning  = "JobsRunning"
	workflowReasonStoppingJobs = "StoppingJobs"
)

// workflowProgress returns the reason and message describing what the
// controller is currently driving, or two empty strings once the Workflow has
// settled. "Settled" means the Workflow reached a terminal phase or completed
// the stop its spec asked for; in both cases there is no action left to report.
func workflowProgress(wf *rlarkv1alpha1.Workflow) (string, string) {
	if wf.Spec.Suspended {
		if wf.Status.Phase == rlarkv1alpha1.WorkflowPhaseStopped {
			return "", ""
		}
		return workflowReasonStoppingJobs, "The Workflow is suspended and its Jobs are being stopped."
	}

	switch wf.Status.Phase {
	case rlarkv1alpha1.WorkflowPhaseSucceeded, rlarkv1alpha1.WorkflowPhaseFailed:
		return "", ""
	case rlarkv1alpha1.WorkflowPhaseStopping:
		return workflowReasonStoppingJobs, "The Workflow is stopping the Jobs left over from its previous run."
	case rlarkv1alpha1.WorkflowPhaseRunning:
		return workflowReasonJobsRunning, "The Workflow is walking its Job DAG and its Jobs are running."
	default:
		return workflowReasonAwaitingJobs, "The Workflow is waiting for its first Jobs to be created and start."
	}
}

// markWorkflowProgress publishes the Progressing condition, or withdraws it
// once the Workflow has settled. Withdrawing rather than setting Status=False
// keeps "not progressing" distinguishable from "progress failed": the latter
// is reported by Validated=False or by the terminal phase, and collapsing both
// into one condition value would lose that distinction.
func markWorkflowProgress(wf *rlarkv1alpha1.Workflow) bool {
	reason, message := workflowProgress(wf)
	if reason == "" {
		return sharedreconcile.ApplyConditionMutations(&wf.Status.Conditions, wf.Generation,
			[]sharedreconcile.ConditionMutation{{
				Condition: metav1.Condition{Type: workflowConditionProgressing},
				Remove:    true,
			}})
	}
	return sharedreconcile.ApplyConditionMutations(&wf.Status.Conditions, wf.Generation,
		[]sharedreconcile.ConditionMutation{{
			Condition: metav1.Condition{
				Type:               workflowConditionProgressing,
				Status:             metav1.ConditionTrue,
				Reason:             reason,
				Message:            message,
				ObservedGeneration: wf.Generation,
			},
		}})
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
	return r.pruneJobsWithTemplates(ctx, wf, legacyJobNames(wf))
}

func (r *Reconciler) pruneJobsWithTemplates(ctx context.Context, wf *rlarkv1alpha1.Workflow, legacyNames []string) (bool, error) {
	desired := make(map[string]string, len(wf.Spec.JobTemplates))
	for _, template := range wf.Spec.JobTemplates {
		desired[utils.ChildName(wf.Name, template.Name)] = template.Name
	}
	jobs, err := r.listWorkflowJobs(ctx, wf)
	if err != nil {
		return false, err
	}
	parent := workflowParentRef(wf)
	waiting := false
	for i := range jobs.Items {
		job := &jobs.Items[i]
		template, keep := desired[job.Name]
		if keep {
			if _, err := utils.ClassifyChild(job, parent, template); err != nil {
				return false, fmt.Errorf("job %s ownership conflict: %w", job.Name, err)
			}
			continue
		}
		decision, err := utils.ClassifyRemovedChild(job, parent, legacyNames)
		if err != nil {
			return false, fmt.Errorf("job %s ownership conflict: %w", job.Name, err)
		}
		if decision.Owned() {
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
	if wf.Spec.Suspended && (!s.HasItems || s.AllStopped) {
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
