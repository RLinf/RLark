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
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const jobLabel = "rlinf.io/job"

// taskJobField indexes Tasks by the Job they carry evidence for: the canonical
// jobLabel and, for Tasks that predate it, the controller OwnerReference. Both
// forms are proof of ownership, so both must be indexed.
const taskJobField = "rlinf.io/job-owner"

// jobParentRef is the single ownership identity Tasks are classified against.
func jobParentRef(job *rlarkv1alpha1.Job) utils.ParentRef {
	return utils.ParentRef{
		UID:        job.UID,
		APIVersion: rlarkv1alpha1.GroupVersion.String(),
		Kind:       "Job",
		Name:       job.Name,
		Label:      jobLabel,
	}
}

// indexTaskByJob is the field indexer function registered for taskJobField.
func indexTaskByJob(obj client.Object) []string {
	task, ok := obj.(*rlarkv1alpha1.Task)
	if !ok {
		return nil
	}
	return utils.ChildIndexKeys(task, rlarkv1alpha1.GroupVersion.String(), "Job", jobLabel)
}

// listJobTasks returns the Tasks that carry ownership evidence for job. The
// lookup goes through taskJobField so the controller does not scan every Task
// in the cluster. Membership is only narrowed here; ownership still has to be
// proven by the caller from authoritative metadata.
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
			if _, err := utils.ClassifyChild(task, jobParentRef(job), t.Name); err != nil {
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
		if utils.SyncStatusEntry(ts, task.Status.Phase, task.Status.Message) {
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
	return r.dispatchTasksWithTemplates(ctx, job, logger, legacyTaskNames(job))
}

// legacyTaskNames are the Task names this Job could have created before the
// canonical ownership annotations existed. Child names are derived here rather
// than reversed from the object, so a Task name is never treated as proof of
// ownership on its own.
func legacyTaskNames(job *rlarkv1alpha1.Job) []string {
	names := make([]string, len(job.Status.Tasks))
	for i := range job.Status.Tasks {
		names[i] = utils.ChildName(job.Name, job.Status.Tasks[i].Name)
	}
	return names
}

func (r *Reconciler) dispatchTasksWithTemplates(
	ctx context.Context,
	job *rlarkv1alpha1.Job,
	logger logr.Logger,
	legacyNames []string,
) (bool, error) {
	namespaces, err := r.resolveTaskNamespaces(ctx, job.Spec.Tasks)
	if err != nil {
		// Draining cordons the target Cluster or Node, so placement resolution
		// can fail. Honour drain-owned Task stops before surfacing that error;
		// otherwise the drain can deadlock, or a completed Node Drain can leave
		// the Job reporting Running after all its Tasks have stopped.
		honouring, stopErr := r.honourDrainStops(ctx, job)
		if stopErr != nil {
			return false, stopErr
		}
		if honouring {
			return false, nil
		}
		return false, err
	}
	if waiting, err := r.pruneTasksInNamespaces(ctx, job, namespaces, legacyNames); err != nil {
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
		if utils.SyncStatusEntry(ts, task.Status.Phase, task.Status.Message) {
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
		stoppedByDrain := taskStoppedByDrain(&task)
		resuming := task.Spec.Suspended && !job.Spec.Suspended && !stoppedByDrain
		desired, err := desiredTask(job, t, taskName, taskNamespace, stoppedByDrain)
		if err != nil {
			return nil, err
		}
		if (!taskTerminal(&task) || resuming) && !taskUpToDate(&task, desired) {
			drainRequests := append([]rlarkv1alpha1.TaskDrainRequest(nil), task.Spec.DrainRequests...)
			// Patch instead of Update so only the fields this controller
			// decided to change are submitted. A full replacement would also
			// resubmit every field the local copy happens to hold, silently
			// reverting a disjoint concurrent write.
			before := task.DeepCopy()
			task.Spec = desired.Spec
			// DrainRequests is owned by the Node Agent, not the Job controller. Preserve
			// the observed requests across controller-driven Spec updates.
			task.Spec.DrainRequests = drainRequests
			task.Annotations = utils.MergeAnnotations(task.Annotations, desired.Annotations)
			if err := r.Patch(ctx, &task, client.MergeFrom(before)); err != nil {
				return nil, fmt.Errorf("patch Task %s/%s: %w", taskNamespace, taskName, err)
			}
			logger.Info("Updated Task for job", "task", taskName, "namespace", taskNamespace)
		}
		return &task, nil
	}
	if !errors.IsNotFound(err) {
		return nil, fmt.Errorf("get Task %s/%s: %w", taskNamespace, taskName, err)
	}

	newTask, err := desiredTask(job, t, taskName, taskNamespace, false)
	if err != nil {
		return nil, err
	}
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

// taskStoppedByDrain reports whether a drain, not the Job or the user, owns this
// Task's stopped state. Such a Task stays suspended after the drain operation is
// retired: the stop is only released when the user starts the Job or unsuspends
// the Task, whether the drain was cluster-scoped or node-scoped.
func taskStoppedByDrain(task *rlarkv1alpha1.Task) bool {
	return task.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] == "true" ||
		task.Annotations[rlarkv1alpha1.NodeDrainStoppedAnnotation] != ""
}

func (r *Reconciler) ensureTaskOwnership(ctx context.Context, job *rlarkv1alpha1.Job, template string, task *rlarkv1alpha1.Task) error {
	decision, err := utils.ClassifyChild(task, jobParentRef(job), template)
	if err != nil {
		return fmt.Errorf("task %s/%s ownership conflict: %w", task.Namespace, task.Name, err)
	}
	if !decision.Legacy() {
		return nil
	}
	before := task.DeepCopy()
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
	if err := r.Patch(ctx, task, client.MergeFrom(before)); err != nil {
		return fmt.Errorf("adopt Task %s/%s: %w", task.Namespace, task.Name, err)
	}
	return nil
}

func (r *Reconciler) pruneTasks(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	namespaces, err := r.resolveTaskNamespaces(ctx, job.Spec.Tasks)
	if err != nil {
		return false, err
	}
	return r.pruneTasksInNamespaces(ctx, job, namespaces, legacyTaskNames(job))
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

// honourDrainStops applies cluster drain stop requests and recognizes Tasks
// already stopped by a completed Node Drain. Cordoned Clusters and Nodes make
// placement resolution fail, but that must not prevent the Job from stopping
// its Tasks or recording their stopped status. It reports whether any owned
// Task carries a drain stop.
func (r *Reconciler) honourDrainStops(ctx context.Context, job *rlarkv1alpha1.Job) (bool, error) {
	tasksByTemplate, err := r.findOwnedTasksByTemplate(ctx, job)
	if err != nil {
		return false, err
	}
	honouring := false
	for _, task := range tasksByTemplate {
		if !taskStoppedByDrain(task) {
			continue
		}
		honouring = true
		if task.Annotations[rlarkv1alpha1.ClusterStoppedAnnotation] != "true" || task.Spec.Suspended || !task.DeletionTimestamp.IsZero() {
			continue
		}
		before := task.DeepCopy()
		task.Spec.Suspended = true
		if err := r.Patch(ctx, task, client.MergeFrom(before)); err != nil {
			return false, fmt.Errorf("apply cluster drain stop to Task %s/%s: %w", task.Namespace, task.Name, err)
		}
	}
	return honouring, nil
}

func (r *Reconciler) pruneTasksInNamespaces(ctx context.Context, job *rlarkv1alpha1.Job, namespaces, legacyNames []string) (bool, error) {
	desired := make(map[string]string, len(job.Spec.Tasks))
	for i := range job.Spec.Tasks {
		template := &job.Spec.Tasks[i]
		desired[namespaces[i]+"/"+utils.ChildName(job.Name, template.Name)] = template.Name
	}
	tasks, err := r.listJobTasks(ctx, job)
	if err != nil {
		return false, err
	}
	parent := jobParentRef(job)
	waiting := false
	for _, task := range tasks {
		template, keep := desired[task.Namespace+"/"+task.Name]
		if keep {
			if _, err := utils.ClassifyChild(task, parent, template); err != nil {
				return false, fmt.Errorf("task %s/%s ownership conflict: %w", task.Namespace, task.Name, err)
			}
			continue
		}
		decision, err := utils.ClassifyRemovedChild(task, parent, legacyNames)
		if err != nil {
			return false, fmt.Errorf("task %s/%s ownership conflict: %w", task.Namespace, task.Name, err)
		}
		if !decision.Owned() {
			continue
		}
		waiting = true
		if task.Status.Phase != rlarkv1alpha1.TaskPhaseStopped {
			// Suspending a pruned Task touches exactly one Spec field, so
			// patch it rather than resubmitting the whole object.
			//
			// The digest has to go with it. It records the state this
			// controller last compiled, and suspending here is not a compiled
			// state: leaving it in place would make the Task look up to date
			// if the template is ever added back, and the suspend would never
			// be undone. Dropping it falls the next comparison back to the
			// structural one, which sees the suspend and resumes the Task.
			before := task.DeepCopy()
			task.Spec.Suspended = true
			task.Annotations = utils.MergeAnnotations(task.Annotations,
				map[string]string{DesiredDigestAnnotation: ""})
			if err := r.Patch(ctx, task, client.MergeFrom(before)); err != nil {
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

// taskUpToDate reports whether existing already carries the desired state.
//
// The comparison is between two decisions of this controller: the digest
// recorded on the live Task and the digest of the state being compiled now.
// Tasks written before DesiredDigestAnnotation existed carry no digest, so
// they fall back to the structural comparison and are treated as up to date
// when it matches. That keeps an upgrade from rewriting every existing Task,
// bumping its generation and forcing the Agent to recompute it; the digest is
// stamped on the first real spec change.
func taskUpToDate(existing, desired *rlarkv1alpha1.Task) bool {
	recorded := existing.Annotations[DesiredDigestAnnotation]
	if recorded == "" {
		// DrainRequests is written by the Node Agent, so the structural fallback
		// compares only the fields this controller owns. Including it would make
		// an Agent-appended drain request look like a spec change and rewrite the
		// Task on every pass. The digest path has the same property by
		// construction: it is taken over the spec this controller compiled, which
		// never carries a drain request.
		current := existing.Spec
		current.DrainRequests = nil
		desiredSpec := desired.Spec
		desiredSpec.DrainRequests = nil
		return reflect.DeepEqual(current, desiredSpec) &&
			existing.Annotations[RestartedAtAnnotation] == desired.Annotations[RestartedAtAnnotation]
	}
	// The digest only describes the state this controller compiled. Suspension is
	// also written by actors outside that compiled state — a cluster or node drain
	// suspends a running Task without restamping the digest — so a matching digest
	// does not prove the Task still matches the decision being made now. An
	// operator Starting the Job after the drain is released compiles the same
	// non-suspended state that the pre-drain digest recorded; without this check
	// the Task would be reported up to date and stay suspended forever.
	return recorded == desired.Annotations[DesiredDigestAnnotation] &&
		existing.Spec.Suspended == desired.Spec.Suspended
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
		changed := markJobInvalid(job, err.Error())
		if markJobProgress(job) {
			changed = true
		}
		if markJobEvicted(job) {
			changed = true
		}
		return changed, nil
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

	if markJobProgress(job) {
		changed = true
	}
	if markJobEvicted(job) {
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
		if markJobProgress(job) {
			changed = true
		}
		if markJobEvicted(job) {
			changed = true
		}
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
	if markJobProgress(job) {
		changed = true
	}
	if markJobEvicted(job) {
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

// jobConditionProgressing is the type of the condition recording that the
// controller is still driving the Job towards its spec.
const jobConditionProgressing = rlarkv1alpha1.ConditionTypeProgressing

// Reason values for the Job Progressing condition. Each names the action the
// controller is currently taking, not the state it is in.
const (
	jobReasonAwaitingTasks = "AwaitingTasks"
	jobReasonTasksRunning  = "TasksRunning"
	jobReasonStoppingTasks = "StoppingTasks"
)

// jobConditionEvicted is the type of the condition recording that every Worker
// of a Job was stopped by an eviction while the Job itself was not suspended.
const jobConditionEvicted = rlarkv1alpha1.ConditionTypeEvicted

// jobReasonEvicted is the stable reason reported when all Workers were evicted.
const jobReasonEvicted = "Evicted"

// jobEvictedMessage is shown to operators. It carries both languages because
// the Condition is exposed verbatim by the API and the console does not
// translate reasons.
const jobEvictedMessage = "所有 Worker 已被驱逐 / All workers were evicted"

// jobProgress returns the reason and message describing what the controller is
// currently driving, or two empty strings once the Job has settled. "Settled"
// means the Job reached a terminal phase or completed the stop its spec asked
// for; in both cases there is no action left to report.
func jobProgress(job *rlarkv1alpha1.Job) (string, string) {
	if job.Spec.Suspended {
		if job.Status.Phase == rlarkv1alpha1.JobPhaseStopped {
			return "", ""
		}
		return jobReasonStoppingTasks, "The Job is suspended and its Tasks are being stopped."
	}

	switch job.Status.Phase {
	case rlarkv1alpha1.JobPhaseSucceeded, rlarkv1alpha1.JobPhaseFailed:
		return "", ""
	case rlarkv1alpha1.JobPhaseRunning:
		return jobReasonTasksRunning, "The Job's Tasks have been dispatched and are running."
	default:
		return jobReasonAwaitingTasks, "The Job is waiting for its Tasks to be dispatched and start."
	}
}

// markJobProgress publishes the Progressing condition, or withdraws it once
// the Job has settled. Withdrawing rather than setting Status=False keeps
// "not progressing" distinguishable from "progress failed": the latter is
// reported by Validated=False or by the terminal phase, and collapsing both
// into one condition value would lose that distinction.
func markJobProgress(job *rlarkv1alpha1.Job) bool {
	reason, message := jobProgress(job)
	if reason == "" {
		return sharedreconcile.ApplyConditionMutations(&job.Status.Conditions, job.Generation,
			[]sharedreconcile.ConditionMutation{{
				Condition: metav1.Condition{Type: jobConditionProgressing},
				Remove:    true,
			}})
	}
	return sharedreconcile.ApplyConditionMutations(&job.Status.Conditions, job.Generation,
		[]sharedreconcile.ConditionMutation{{
			Condition: metav1.Condition{
				Type:               jobConditionProgressing,
				Status:             metav1.ConditionTrue,
				Reason:             reason,
				Message:            message,
				ObservedGeneration: job.Generation,
			},
		}})
}

// allTaskStatusesStopped reports whether the Job's Task snapshot is non-empty
// and every Task is in the Stopped phase.
func allTaskStatusesStopped(job *rlarkv1alpha1.Job) bool {
	if len(job.Status.Tasks) == 0 {
		return false
	}
	for _, ts := range job.Status.Tasks {
		if ts.Phase != rlarkv1alpha1.TaskPhaseStopped {
			return false
		}
	}
	return true
}

// markJobEvicted records that every Worker of a Job was stopped by an eviction
// while the Job itself was not suspended. A Job reaches this state when a Node
// or Cluster Drain stops all of its Tasks; the Job must then report Stopped
// instead of looking like it is merely waiting for its Workers. The condition
// is withdrawn as soon as the Job is no longer in that state.
func markJobEvicted(job *rlarkv1alpha1.Job) bool {
	if job.Spec.Suspended || job.Status.Phase != rlarkv1alpha1.JobPhaseStopped || !allTaskStatusesStopped(job) {
		return sharedreconcile.ApplyConditionMutations(&job.Status.Conditions, 0,
			[]sharedreconcile.ConditionMutation{{
				Condition: metav1.Condition{Type: jobConditionEvicted},
				Remove:    true,
			}})
	}
	return sharedreconcile.ApplyConditionMutations(&job.Status.Conditions, 0,
		[]sharedreconcile.ConditionMutation{{
			Condition: metav1.Condition{
				Type:    jobConditionEvicted,
				Status:  metav1.ConditionTrue,
				Reason:  jobReasonEvicted,
				Message: jobEvictedMessage,
			},
		}})
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
		// Stay stopped while every Worker is still gone. This is the state a
		// Node or Cluster Drain leaves behind: the Job was not suspended, but
		// all of its Tasks were stopped by the eviction.
		if s.AllStopped && s.HasItems {
			return ""
		}
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

	if s.AllStopped && s.HasItems {
		return EventJobStopped
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
