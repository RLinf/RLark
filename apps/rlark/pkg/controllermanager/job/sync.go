package job

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
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const jobLabel = "rlinf.io/job"

// taskJobField indexes Tasks by the name in their jobLabel label. The builder
// always stamps the label, and adoption backfills it, so a Task that belongs to
// a Job is always indexed under the Job name.
const taskJobField = "rlinf.io/job-owner"

// indexTaskByJob is the field indexer function registered for taskJobField.
func indexTaskByJob(obj client.Object) []string {
	task, ok := obj.(*rlarkv1alpha1.Task)
	if !ok || task.Labels[jobLabel] == "" {
		return nil
	}
	return []string{task.Labels[jobLabel]}
}

// listJobTasks returns the Tasks labelled as belonging to job. The lookup goes
// through taskJobField so the controller does not scan every Task in the
// cluster. Membership is only narrowed here; ownership still has to be proven
// by the caller from authoritative metadata.
func (r *Reconciler) listJobTasks(ctx context.Context, job *rlarkv1alpha1.Job) ([]*rlarkv1alpha1.Task, error) {
	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.MatchingFields{taskJobField: job.Name}); err != nil {
		return nil, fmt.Errorf("list Tasks for Job %s: %w", job.Name, err)
	}
	candidates := make([]*rlarkv1alpha1.Task, 0, len(tasks.Items))
	for i := range tasks.Items {
		candidates = append(candidates, &tasks.Items[i])
	}
	return candidates, nil
}

// findOwnedTasksByTemplate maps each Job task template to the Task that already
// exists for it. Discovery is ownership-driven rather than derived from the
// template's nodeSelector, so observing and stopping a Job keeps working after
// the Nodes a template selected have been removed.
func (r *Reconciler) findOwnedTasksByTemplate(
	ctx context.Context,
	job *rlarkv1alpha1.Job,
) (map[string]*rlarkv1alpha1.Task, error) {
	candidates, err := r.listJobTasks(ctx, job)
	if err != nil {
		return nil, err
	}

	byTemplate := make(map[string]*rlarkv1alpha1.Task, len(job.Spec.Tasks))
	for _, t := range job.Spec.Tasks {
		taskName := utils.ChildName(job.Name, t.Name)
		for _, task := range candidates {
			if task.Annotations[utils.ChildTemplateAnnotation] != t.Name && task.Name != taskName {
				continue
			}
			if _, _, err := utils.ClassifyChild(task, job.UID, rlarkv1alpha1.GroupVersion.String(), "Job", jobLabel, job.Name, t.Name); err != nil {
				return nil, fmt.Errorf("task %s/%s ownership conflict: %w", task.Namespace, task.Name, err)
			}
			byTemplate[t.Name] = task
			break
		}
	}
	return byTemplate, nil
}

func (r *Reconciler) syncTaskStatuses(
	ctx context.Context,
	job *rlarkv1alpha1.Job,
) (bool, error) {
	statusMap := buildTaskStatusMap(job)
	tasksByTemplate, err := r.findOwnedTasksByTemplate(ctx, job)
	if err != nil {
		return false, err
	}
	changed := false

	for _, t := range job.Spec.Tasks {
		ts := statusMap[t.Name]
		if ts == nil || ts.Phase == "" {
			continue
		}

		task := tasksByTemplate[t.Name]
		if task == nil {
			continue
		}
		if utils.SyncStatusEntry(ts, string(task.Status.Phase), task.Status.Message) {
			changed = true
		}
	}

	return changed, nil
}

func (r *Reconciler) dispatchTasks(
	ctx context.Context,
	job *rlarkv1alpha1.Job,
	logger logr.Logger,
) (bool, error) {
	oldTemplates := make([]string, len(job.Status.Tasks))
	for i := range job.Status.Tasks {
		oldTemplates[i] = job.Status.Tasks[i].Name
	}
	return r.dispatchTasksWithTemplates(ctx, job, logger, oldTemplates)
}

func (r *Reconciler) dispatchTasksWithTemplates(
	ctx context.Context,
	job *rlarkv1alpha1.Job,
	logger logr.Logger,
	oldTemplates []string,
) (bool, error) {
	namespaces, err := r.resolveTaskNamespaces(ctx, job.Spec.Tasks)
	if err != nil {
		return false, err
	}
	if waiting, err := r.pruneTasksInNamespaces(ctx, job, namespaces, oldTemplates); err != nil {
		return false, err
	} else if waiting {
		return false, controller.ErrRequeueAfterChildCleanup
	}
	statusMap := buildTaskStatusMap(job)
	changed := false

	for i, t := range job.Spec.Tasks {
		ts := statusMap[t.Name]

		task, err := r.reconcileTaskInNamespace(ctx, job, t, namespaces[i], logger)
		if err != nil {
			return false, err
		}
		if utils.SyncStatusEntry(ts, string(task.Status.Phase), task.Status.Message) {
			changed = true
		}
	}

	return changed, nil
}

func (r *Reconciler) reconcileTask(
	ctx context.Context,
	job *rlarkv1alpha1.Job,
	t rlarkv1alpha1.JobTaskTemplate,
	logger logr.Logger,
) (*rlarkv1alpha1.Task, error) {
	taskNamespace, err := r.resolveTaskNamespace(ctx, &t)
	if err != nil {
		return nil, err
	}
	return r.reconcileTaskInNamespace(ctx, job, t, taskNamespace, logger)
}

func (r *Reconciler) reconcileTaskInNamespace(
	ctx context.Context,
	job *rlarkv1alpha1.Job,
	t rlarkv1alpha1.JobTaskTemplate,
	taskNamespace string,
	logger logr.Logger,
) (*rlarkv1alpha1.Task, error) {
	taskName := utils.ChildName(job.Name, t.Name)
	var task rlarkv1alpha1.Task
	err := r.Get(ctx, types.NamespacedName{Name: taskName, Namespace: taskNamespace}, &task)
	if err == nil {
		if err := r.ensureTaskOwnership(ctx, job, t.Name, &task); err != nil {
			return nil, err
		}
		if !task.DeletionTimestamp.IsZero() {
			return nil, controller.ErrRequeueAfterChildCleanup
		}
		if task.Annotations[RestartedAtAnnotation] != job.Annotations[RestartedAtAnnotation] {
			if err := client.IgnoreNotFound(r.Delete(ctx, &task)); err != nil {
				return nil, fmt.Errorf("delete Task %s/%s for Job restart: %w", taskNamespace, taskName, err)
			}
			return nil, controller.ErrRequeueAfterChildCleanup
		}
		resuming := task.Spec.Suspended && !job.Spec.Suspended
		if (!taskTerminal(&task) || resuming) && !taskEqual(&task, job, t) {
			desired := buildTask(job, t, taskName, taskNamespace)
			task.Spec = desired.Spec
			task.Annotations = utils.MergeAnnotations(task.Annotations, desired.Annotations)
			if err := r.Update(ctx, &task); err != nil {
				return nil, fmt.Errorf("update Task %s/%s: %w", taskNamespace, taskName, err)
			}
			logger.Info("Updated Task for job", "task", taskName, "namespace", taskNamespace)
		}
		return &task, nil
	}
	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("get Task %s/%s: %w", taskNamespace, taskName, err)
	}

	newTask := buildTask(job, t, taskName, taskNamespace)
	if err := ctrl.SetControllerReference(job, newTask, r.Scheme); err != nil {
		return nil, fmt.Errorf("set controller reference on Task %s: %w", taskName, err)
	}
	if err := r.Create(ctx, newTask); err != nil {
		if !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("create Task %s/%s: %w", taskNamespace, taskName, err)
		}
		if err := r.Get(ctx, types.NamespacedName{Name: taskName, Namespace: taskNamespace}, &task); err != nil {
			return nil, fmt.Errorf("reload Task %s/%s after create race: %w", taskNamespace, taskName, err)
		}
		if err := r.ensureTaskOwnership(ctx, job, t.Name, &task); err != nil {
			return nil, err
		}
		return &task, nil
	}

	logger.Info("Created Task for job", "task", taskName, "namespace", taskNamespace)
	newTask.Status.Phase = rlarkv1alpha1.TaskPhasePending
	return newTask, nil
}

func taskTerminal(task *rlarkv1alpha1.Task) bool {
	return task.Status.Phase == rlarkv1alpha1.TaskPhaseSucceeded || task.Status.Phase == rlarkv1alpha1.TaskPhaseFailed
}

func (r *Reconciler) ensureTaskOwnership(ctx context.Context, job *rlarkv1alpha1.Job, template string, task *rlarkv1alpha1.Task) error {
	owned, legacy, err := utils.ClassifyChild(task, job.UID, rlarkv1alpha1.GroupVersion.String(), "Job", jobLabel, job.Name, template)
	if err != nil || !owned {
		return fmt.Errorf("task %s/%s ownership conflict: %w", task.Namespace, task.Name, err)
	}
	if !legacy {
		return nil
	}
	// Backfill the Job label as well, so the adopted Task is discoverable
	// through taskJobField by name and not only by owner reference UID.
	if task.Labels == nil {
		task.Labels = map[string]string{}
	}
	task.Labels[jobLabel] = job.Name
	task.Annotations = utils.MergeAnnotations(task.Annotations, map[string]string{
		utils.ParentUIDAnnotation: string(job.UID), utils.ChildTemplateAnnotation: template,
	})
	if err := ctrl.SetControllerReference(job, task, r.Scheme); err != nil {
		return fmt.Errorf("adopt Task %s/%s: %w", task.Namespace, task.Name, err)
	}
	if err := r.Update(ctx, task); err != nil {
		return fmt.Errorf("adopt Task %s/%s: %w", task.Namespace, task.Name, err)
	}
	return nil
}

func (r *Reconciler) pruneTasks(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	oldTemplates := make([]string, len(job.Status.Tasks))
	for i := range job.Status.Tasks {
		oldTemplates[i] = job.Status.Tasks[i].Name
	}
	namespaces, err := r.resolveTaskNamespaces(ctx, job.Spec.Tasks)
	if err != nil {
		return false, err
	}
	return r.pruneTasksInNamespaces(ctx, job, namespaces, oldTemplates)
}

func (r *Reconciler) resolveTaskNamespaces(ctx context.Context, templates []rlarkv1alpha1.JobTaskTemplate) ([]string, error) {
	namespaces := make([]string, len(templates))
	for i := range templates {
		namespace, err := r.resolveTaskNamespace(ctx, &templates[i])
		if err != nil {
			return nil, err
		}
		namespaces[i] = namespace
	}
	return namespaces, nil
}

func (r *Reconciler) pruneTasksInNamespaces(ctx context.Context, job *rlarkv1alpha1.Job, namespaces, oldTemplates []string) (bool, error) {
	desired := make(map[string]string, len(job.Spec.Tasks))
	for i := range job.Spec.Tasks {
		template := &job.Spec.Tasks[i]
		desired[namespaces[i]+"/"+utils.ChildName(job.Name, template.Name)] = template.Name
	}
	tasks, err := r.listJobTasks(ctx, job)
	if err != nil {
		return false, err
	}
	waiting := false
	for _, task := range tasks {
		template, keep := desired[task.Namespace+"/"+task.Name]
		if keep {
			if _, _, err := utils.ClassifyChild(task, job.UID, rlarkv1alpha1.GroupVersion.String(), "Job", jobLabel, job.Name, template); err != nil {
				return false, fmt.Errorf("task %s/%s ownership conflict: %w", task.Namespace, task.Name, err)
			}
			continue
		}
		owned, _ := utils.ClassifyRemovedChild(task, job.UID, rlarkv1alpha1.GroupVersion.String(), "Job", jobLabel, job.Name, oldTemplates)
		if !owned {
			continue
		}
		waiting = true
		if task.Status.Phase != rlarkv1alpha1.TaskPhaseStopped {
			task.Spec.Suspended = true
			if err := r.Update(ctx, task); err != nil {
				return false, fmt.Errorf("stop pruned Task %s/%s: %w", task.Namespace, task.Name, err)
			}
			continue
		}
		if task.DeletionTimestamp.IsZero() {
			if err := r.Delete(ctx, task); client.IgnoreNotFound(err) != nil {
				return false, fmt.Errorf("prune Task %s/%s: %w", task.Namespace, task.Name, err)
			}
		}
	}
	return waiting, nil
}

func taskEqual(existing *rlarkv1alpha1.Task, job *rlarkv1alpha1.Job, t rlarkv1alpha1.JobTaskTemplate) bool {
	desired := buildTask(job, t, existing.Name, existing.Namespace)
	return reflect.DeepEqual(existing.Spec, desired.Spec) &&
		existing.Annotations[RestartedAtAnnotation] == desired.Annotations[RestartedAtAnnotation]
}

func (r *Reconciler) reconcileWithStateMachine(
	ctx context.Context,
	job *rlarkv1alpha1.Job,
) (bool, error) {
	logger := log.FromContext(ctx).WithValues("job", job.Name)

	if job.Spec.Suspended {
		return r.reconcileStoppedJob(ctx, job)
	}

	if err := validateTaskTemplates(job); err != nil {
		logger.Error(err, "invalid Task template configuration")
		return markJobInvalid(job, err.Error()), nil
	}

	f := newJobStateMachine()
	f.SetState(string(job.Status.Phase))

	changed := syncTaskStatusSnapshot(job)
	if markJobValid(job) {
		changed = true
	}

	if f.Can(EventInit) {
		if err := f.Event(ctx, EventInit, job); err != nil {
			return false, err
		}
		changed = true
	}

	syncChanged, err := r.syncTaskStatuses(ctx, job)
	if err != nil {
		return false, err
	}
	if syncChanged {
		changed = true
	}
	dispatchChanged, err := r.dispatchTasks(ctx, job, logger)
	if err != nil {
		return false, err
	}
	if dispatchChanged {
		changed = true
	}

	event := r.evaluateJobEvent(job)
	if event != "" && f.Can(event) {
		if err := f.Event(ctx, event, job); err != nil {
			return false, err
		}
		changed = true
	}

	return changed, nil
}

func (r *Reconciler) reconcileStoppedJob(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	f := newJobStateMachine()
	f.SetState(string(job.Status.Phase))
	changed := syncTaskStatusSnapshot(job)

	if f.Can(EventInit) {
		if err := f.Event(ctx, EventInit, job); err != nil {
			return false, err
		}
		changed = true
	}
	if syncChanged, err := r.syncTaskStatuses(ctx, job); err != nil {
		return false, err
	} else if syncChanged {
		changed = true
	}
	waiting, err := r.deleteOwnedTasks(ctx, job)
	if err != nil {
		return false, err
	}
	if waiting {
		return changed, controller.ErrRequeueAfterChildCleanup
	}
	if markTaskStatusesStopped(job) {
		changed = true
	}
	if f.Can(EventJobStopped) {
		if err := f.Event(ctx, EventJobStopped, job); err != nil {
			return false, err
		}
		changed = true
	}
	return changed, nil
}

func markTaskStatusesStopped(job *rlarkv1alpha1.Job) bool {
	changed := syncTaskStatusSnapshot(job)
	for i := range job.Status.Tasks {
		switch job.Status.Tasks[i].Phase {
		case rlarkv1alpha1.TaskPhaseSucceeded, rlarkv1alpha1.TaskPhaseFailed, rlarkv1alpha1.TaskPhaseStopped:
			continue
		default:
			job.Status.Tasks[i].Phase = rlarkv1alpha1.TaskPhaseStopped
			changed = true
		}
	}
	return changed
}

// jobConditionValidated is the type of the condition recording spec validation.
const jobConditionValidated = rlarkv1alpha1.ConditionTypeValidated

// Reason values for the Job Validated condition. They are stable, CamelCase
// identifiers consumed by clients.
const (
	jobReasonSpecValidated      = "SpecValidated"
	jobReasonInvalidRayHeadTask = "InvalidRayHeadTask"
)

// markJobValid records that the current spec passed validation, clearing a
// stale Validated=False left behind by a previously invalid spec.
func markJobValid(job *rlarkv1alpha1.Job) bool {
	return apimeta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
		Type:               jobConditionValidated,
		Status:             metav1.ConditionTrue,
		Reason:             jobReasonSpecValidated,
		ObservedGeneration: job.Generation,
	})
}

// markJobInvalid marks the Job as Failed due to an invalid spec and records the
// reason in a "Validated" condition. It returns whether the Job object changed.
func markJobInvalid(job *rlarkv1alpha1.Job, message string) bool {
	changed := false

	if job.Status.Phase != rlarkv1alpha1.JobPhaseFailed {
		job.Status.Phase = rlarkv1alpha1.JobPhaseFailed
		changed = true
	}
	if job.Status.EndTime == nil {
		now := metav1.Now()
		job.Status.EndTime = &now
		changed = true
	}

	if apimeta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
		Type:               jobConditionValidated,
		Status:             metav1.ConditionFalse,
		Reason:             jobReasonInvalidRayHeadTask,
		Message:            message,
		ObservedGeneration: job.Generation,
	}) {
		changed = true
	}

	return changed
}

func (r *Reconciler) evaluateJobEvent(job *rlarkv1alpha1.Job) string {
	phases := make([]string, len(job.Status.Tasks))
	for i, ts := range job.Status.Tasks {
		phases[i] = string(ts.Phase)
	}
	s := utils.SummarizePhases(phases,
		string(rlarkv1alpha1.TaskPhaseSucceeded),
		string(rlarkv1alpha1.TaskPhaseFailed),
		string(rlarkv1alpha1.TaskPhaseRunning),
		string(rlarkv1alpha1.TaskPhaseStopped),
	)

	if job.Spec.Suspended {
		if s.AllStopped && s.HasItems {
			return EventJobStopped
		}
		if s.HasItems && job.Status.Phase != rlarkv1alpha1.JobPhasePending {
			return EventTasksPending
		}
		return ""
	}

	switch job.Status.Phase {
	case rlarkv1alpha1.JobPhaseStopped:
		return EventTasksPending
	case rlarkv1alpha1.JobPhaseSucceeded:
		if !s.AllSucceeded {
			return EventTasksPending
		}
	case rlarkv1alpha1.JobPhaseFailed:
		if !s.AnyFailed {
			return EventTasksPending
		}
	}

	if s.AnyFailed {
		return EventAnyTaskFailed
	}

	if s.AllSucceeded && s.HasItems {
		return EventAllTasksDone
	}

	if allTasksInPhase(phases, string(rlarkv1alpha1.TaskPhaseRunning)) {
		return EventTasksRunning
	}

	if s.HasItems && job.Status.Phase != rlarkv1alpha1.JobPhasePending {
		return EventTasksPending
	}

	return ""
}

func allTasksInPhase(phases []string, phase string) bool {
	if len(phases) == 0 {
		return false
	}
	for _, current := range phases {
		if current != phase {
			return false
		}
	}
	return true
}
