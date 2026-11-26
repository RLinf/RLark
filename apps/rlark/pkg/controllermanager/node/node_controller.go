package node

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerconfig "sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

const (
	nodeHeartbeatLeasePrefix = "node-heartbeat-"
	nodeDrainRequeueInterval = 5 * time.Second
	// nodeDrainCleanupFinalizer keeps a deleted Node around until the control
	// plane has released every Task DrainRequest that targets it. This is the
	// last-resort path when the Node Agent is offline and the deletion bypasses
	// the Gateway, for example kubectl delete.
	nodeDrainCleanupFinalizer = "rlark.io/node-drain-request-cleanup"
)

// Reason codes this controller attaches to a classified pass. Closed set: they
// are used as a metric label.
const (
	reasonLeaseLookupFailed    = "LeaseLookupFailed"
	reasonDrainLookupFailed    = "DrainOperationLookupFailed"
	reasonDrainRequestReleased = "DrainRequestsReleased"
	reasonDrainRetired         = "DrainOperationsRetired"
	reasonDrainRetireFailed    = "DrainOperationRetireFailed"
)

func nodeHeartbeatLeaseName(nodeName string) string {
	return nodeHeartbeatLeasePrefix + nodeName
}

// Reconciler reconciles Node resources.
type Reconciler struct {
	client.Client
	Scheme                  *runtime.Scheme
	MaxConcurrentReconciles int
	Now                     func() time.Time
	LeaseTimeout            time.Duration
	LeaseGracePeriod        time.Duration
	// Shell classifies every pass, turns a retry class into a jittered interval
	// and records the result. The zero value is silent, so a test may leave it
	// unset.
	Shell controller.Shell
}

// +kubebuilder:rbac:groups=rlinf.io,resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rlinf.io,resources=nodes/finalizers,verbs=update
// +kubebuilder:rbac:groups=rlinf.io,resources=tasks,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=coordination.k8s.io,resources=leases,verbs=get;list;watch

// Reconcile derives lifecycle conclusions from management intent and Agent facts.
// The controller owns only Phase and Conditions; Agents own raw observations.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.Shell.Run(ctx, "node", func(ctx context.Context) (controller.Result, error) {
		return r.reconcile(ctx, req)
	})
}

func (r *Reconciler) reconcile(ctx context.Context, req ctrl.Request) (controller.Result, error) {
	logger := log.FromContext(ctx).WithValues("node", req.NamespacedName)

	var node rlarkv1alpha1.Node
	if err := r.Get(ctx, req.NamespacedName, &node); err != nil {
		if apierrors.IsNotFound(err) {
			return controller.Result{
				Outcome: sharedreconcile.OutcomeDropped,
				Reason:  controller.ReasonObjectGone,
				Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
			}, nil
		}
		return controller.Result{Reason: controller.ReasonReconcileFailed}, err
	}

	// A terminating Node can no longer be undrained. Release the requests it
	// projected onto Tasks before allowing the object to disappear.
	if node.DeletionTimestamp != nil {
		if err := r.reconcileNodeDeletion(ctx, &node); err != nil {
			return controller.Result{Reason: controller.ReasonReconcileFailed}, err
		}
		return controller.Result{
			Outcome: sharedreconcile.OutcomeSucceeded,
			Reason:  reasonDrainRequestReleased,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryNone},
		}, nil
	}
	if err := r.ensureDrainCleanupFinalizer(ctx, &node); err != nil {
		return controller.Result{Reason: controller.ReasonReconcileFailed}, err
	}

	now := r.now()
	lease, err := r.getHeartbeatLease(ctx, node.Namespace, node.Name)
	if err != nil {
		logger.Error(err, "get node heartbeat lease")
		return controller.Result{Reason: reasonLeaseLookupFailed}, err
	}

	// The operator-configured timeout overrides whatever the Lease advertises
	// and the configured grace period extends it; a Lease without a
	// leaseDurationSeconds falls back to the shared default instead of failing
	// the pass.
	heartbeat := controller.ObserveLeaseHeartbeat(lease, node.Status.LastHeartbeatTime, now, sharedreconcile.HeartbeatTimeouts{
		Override: r.LeaseTimeout,
		Grace:    r.LeaseGracePeriod,
	})
	if heartbeat.ClockSkew {
		logger.Info("node heartbeat Lease is dated in the future; honouring it but the agent clock runs ahead",
			"lastRenewal", heartbeat.LastRenewal)
	}
	connected := !heartbeat.Expired()
	connectionReason := nodeConnectionReason(heartbeat.State)
	drainOperations, err := r.drainOperations(ctx, &node)
	if err != nil {
		return controller.Result{Reason: reasonDrainLookupFailed}, err
	}
	before := node.DeepCopy()
	node.Status.DrainOperations = drainOperations
	// This controller owns LastHeartbeatTime: it is the renewal the Lease
	// recorded, read through one clock, not a timestamp each agent stamps with
	// its own.
	node.Status.LastHeartbeatTime = controller.HeartbeatTime(heartbeat)
	conditionsChanged := setNodeConditions(&node, connected, connectionReason, now)
	phase := nodePhase(&node, connected)
	node.Status.Phase = phase
	lifecycleChanged := phase != before.Status.Phase || conditionsChanged || !slices.EqualFunc(node.Status.DrainOperations, before.Status.DrainOperations, func(a, b rlarkv1alpha1.NodeDrainOperationStatus) bool { return a == b })
	if lifecycleChanged {
		node.Status.LastStatusChangeTime = &metav1.Time{Time: now}
	}
	// A renewal that moved forward is written even when nothing else changed,
	// so an observer can tell a live Node from one whose state happens to be
	// unchanged since the last pass.
	if lifecycleChanged || !sameTime(node.Status.LastHeartbeatTime, before.Status.LastHeartbeatTime) {
		if err := r.Status().Patch(ctx, &node, client.MergeFrom(before)); err != nil {
			if apierrors.IsConflict(err) {
				return controller.Result{Reason: controller.ReasonStatusWriteConflict}, err
			}
			return controller.Result{Reason: controller.ReasonStatusWriteFailed}, fmt.Errorf("patch node lifecycle status: %w", err)
		}
		logger.Info("updated node lifecycle status", "phase", phase, "reason", connectionReason)
	}

	// Re-check when this Lease actually expires rather than a full timeout
	// later: a renewal seen halfway through the window must not buy another
	// whole window of silence.
	requeueAfter := controller.NextHeartbeatCheck(now, heartbeat)
	if retired, err := r.retireCompletedDrains(ctx, &node, drainOperations); err != nil {
		return controller.Result{Reason: reasonDrainRetireFailed}, err
	} else if retired {
		return controller.Result{
			Outcome: sharedreconcile.OutcomeSucceeded,
			Reason:  reasonDrainRetired,
			Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange, After: nodeDrainRequeueInterval},
		}, nil
	}
	if hasOperation(&node, rlarkv1alpha1.NodeOperationDrain) && !nodeDrainCompleted(&node) && requeueAfter > nodeDrainRequeueInterval {
		requeueAfter = nodeDrainRequeueInterval
	}
	// A Lease that stops being renewed produces no event, so expiry can only be
	// noticed by looking again. The shell jitters the interval so that every
	// Node does not re-check on the same tick.
	return controller.Result{
		Outcome: sharedreconcile.OutcomeSucceeded,
		Reason:  connectionReason,
		Retry:   sharedreconcile.RetryDirective{Class: sharedreconcile.RetryOnChange, After: requeueAfter},
	}, nil
}

func (r *Reconciler) ensureDrainCleanupFinalizer(ctx context.Context, node *rlarkv1alpha1.Node) error {
	if slices.Contains(node.Finalizers, nodeDrainCleanupFinalizer) {
		return nil
	}
	before := node.DeepCopy()
	node.Finalizers = append(node.Finalizers, nodeDrainCleanupFinalizer)
	if err := r.Patch(ctx, node, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
		return fmt.Errorf("add node drain cleanup finalizer: %w", err)
	}
	return nil
}

// reconcileNodeDeletion releases every Task DrainRequest that targets the
// terminating Node, clears the stop markers its completed drains left behind and
// then removes the cleanup finalizer. The Gateway usually performs the same
// release, but this finalizer also covers deletions that bypass the Gateway and
// Agents that are offline during the deletion.
func (r *Reconciler) reconcileNodeDeletion(ctx context.Context, node *rlarkv1alpha1.Node) error {
	if !slices.Contains(node.Finalizers, nodeDrainCleanupFinalizer) {
		return nil
	}
	if err := r.releaseNodeDrainRequests(ctx, node); err != nil {
		return err
	}
	if err := r.clearNodeDrainStops(ctx, node); err != nil {
		return err
	}
	before := node.DeepCopy()
	node.Finalizers = slices.DeleteFunc(slices.Clone(node.Finalizers), func(finalizer string) bool {
		return finalizer == nodeDrainCleanupFinalizer
	})
	if err := r.Patch(ctx, node, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
		return fmt.Errorf("remove node drain cleanup finalizer: %w", err)
	}
	return nil
}

// retireCompletedDrains removes Drain operations whose eviction finished.
//
// Removing the operation releases the DrainRequests the Node Agent dispatched to
// the drained Tasks, which would restart them on the still-cordoned Node. The
// Tasks are therefore marked as stopped by this Node's drain first: the marker
// stays after the operation is gone, so only an explicit cancellation (uncordon,
// undrain, recover or Node deletion) resumes them.
func (r *Reconciler) retireCompletedDrains(
	ctx context.Context,
	node *rlarkv1alpha1.Node,
	drainOperations []rlarkv1alpha1.NodeDrainOperationStatus,
) (bool, error) {
	completed := make(map[string]struct{}, len(drainOperations))
	for _, status := range drainOperations {
		if status.Completed {
			completed[status.OperationID] = struct{}{}
		}
	}
	if len(completed) == 0 {
		return false, nil
	}

	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.InNamespace(node.Namespace)); err != nil {
		return false, fmt.Errorf("list Tasks to retire node drain: %w", err)
	}

	marked := true
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if !holdsDrainRequestFor(task, completed) {
			continue
		}
		if task.Annotations[rlarkv1alpha1.NodeDrainStoppedAnnotation] == node.Name && task.Spec.Suspended {
			continue
		}
		before := task.DeepCopy()
		if task.Annotations == nil {
			task.Annotations = map[string]string{}
		}
		task.Annotations[rlarkv1alpha1.NodeDrainStoppedAnnotation] = node.Name
		if !task.Spec.Suspended {
			task.Annotations[rlarkv1alpha1.NodeDrainSuspendedAnnotation] = node.Name
			task.Spec.Suspended = true
		}
		if err := r.Patch(ctx, task, client.MergeFrom(before)); err != nil {
			return false, fmt.Errorf("mark Task %s/%s stopped by node drain: %w", task.Namespace, task.Name, err)
		}
		marked = false
	}
	if !marked {
		// Reconcile again after the markers are visible before retiring the
		// operation, so no Task can restart in the gap.
		return true, nil
	}

	before := node.DeepCopy()
	kept := make([]rlarkv1alpha1.NodeOperationRequest, 0, len(node.Spec.Operations))
	for _, operation := range node.Spec.Operations {
		if operation.Type == rlarkv1alpha1.NodeOperationDrain {
			if _, done := completed[operation.ID]; done {
				continue
			}
		}
		kept = append(kept, operation)
	}
	if len(kept) == len(node.Spec.Operations) {
		return false, nil
	}
	node.Spec.Operations = kept
	if err := r.Patch(ctx, node, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
		return false, fmt.Errorf("retire completed node drain operations: %w", err)
	}

	// The Tasks are durably stopped, so releasing the now-dangling requests
	// cannot restart them.
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if !holdsDrainRequestFor(task, completed) {
			continue
		}
		keptRequests := make([]rlarkv1alpha1.TaskDrainRequest, 0, len(task.Spec.DrainRequests))
		for _, request := range task.Spec.DrainRequests {
			if _, done := completed[request.OperationID]; !done {
				keptRequests = append(keptRequests, request)
			}
		}
		taskBefore := task.DeepCopy()
		if len(keptRequests) == 0 {
			task.Spec.DrainRequests = nil
		} else {
			task.Spec.DrainRequests = keptRequests
		}
		if err := r.Patch(ctx, task, client.MergeFrom(taskBefore)); err != nil {
			return false, fmt.Errorf("release retired drain requests from Task %s/%s: %w", task.Namespace, task.Name, err)
		}
	}
	return true, nil
}

func holdsDrainRequestFor(task *rlarkv1alpha1.Task, operationIDs map[string]struct{}) bool {
	for _, request := range task.Spec.DrainRequests {
		if _, ok := operationIDs[request.OperationID]; ok {
			return true
		}
	}
	return false
}

func (r *Reconciler) releaseNodeDrainRequests(ctx context.Context, node *rlarkv1alpha1.Node) error {
	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.InNamespace(node.Namespace)); err != nil {
		return fmt.Errorf("list Tasks for node drain cleanup: %w", err)
	}
	for i := range tasks.Items {
		task := &tasks.Items[i]
		kept := make([]rlarkv1alpha1.TaskDrainRequest, 0, len(task.Spec.DrainRequests))
		for _, request := range task.Spec.DrainRequests {
			if request.NodeName != node.Name {
				kept = append(kept, request)
			}
		}
		if len(kept) == len(task.Spec.DrainRequests) {
			continue
		}
		before := task.DeepCopy()
		if len(kept) == 0 {
			task.Spec.DrainRequests = nil
		} else {
			task.Spec.DrainRequests = kept
		}
		if err := r.Patch(ctx, task, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
			return fmt.Errorf("release drain requests from Task %s/%s: %w", task.Namespace, task.Name, err)
		}
	}
	return nil
}

// clearNodeDrainStops removes the drain stop markers a deleted Node left on the
// Tasks it drained. The Node is gone, so nothing can cancel that drain any more;
// keeping the markers would leave the Tasks suspended forever.
func (r *Reconciler) clearNodeDrainStops(ctx context.Context, node *rlarkv1alpha1.Node) error {
	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.InNamespace(node.Namespace)); err != nil {
		return fmt.Errorf("list Tasks for node drain cleanup: %w", err)
	}
	for i := range tasks.Items {
		task := &tasks.Items[i]
		if task.Annotations[rlarkv1alpha1.NodeDrainStoppedAnnotation] != node.Name {
			continue
		}
		before := task.DeepCopy()
		delete(task.Annotations, rlarkv1alpha1.NodeDrainStoppedAnnotation)
		if task.Annotations[rlarkv1alpha1.NodeDrainSuspendedAnnotation] == node.Name {
			delete(task.Annotations, rlarkv1alpha1.NodeDrainSuspendedAnnotation)
			task.Spec.Suspended = false
		}
		if err := r.Patch(ctx, task, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
			return fmt.Errorf("clear node drain markers from Task %s/%s: %w", task.Namespace, task.Name, err)
		}
	}
	return nil
}

func (r *Reconciler) drainOperations(ctx context.Context, node *rlarkv1alpha1.Node) ([]rlarkv1alpha1.NodeDrainOperationStatus, error) {
	operations := make(map[string]rlarkv1alpha1.NodeDrainOperationStatus)
	for _, operation := range node.Spec.Operations {
		if operation.Type == rlarkv1alpha1.NodeOperationDrain && operation.ID != "" {
			operations[operation.ID] = rlarkv1alpha1.NodeDrainOperationStatus{OperationID: operation.ID}
		}
	}
	if len(operations) == 0 {
		return nil, nil
	}
	// Completion is terminal for as long as the operation stays in Spec. A Drain
	// operation the Node already reported completed must not be recomputed from
	// scratch: while it is still present, recomputing would report the finished
	// operation as pending and re-enter Draining. retireCompletedDrains removes
	// the operation once the Tasks it stopped are durably suspended.
	for _, status := range node.Status.DrainOperations {
		if !status.Completed {
			continue
		}
		operation, ok := operations[status.OperationID]
		if !ok {
			continue
		}
		operation.Completed = true
		operations[status.OperationID] = operation
	}
	if node.Status.KubernetesNodeUID == "" {
		result := make([]rlarkv1alpha1.NodeDrainOperationStatus, 0, len(operations))
		for _, status := range operations {
			result = append(result, status)
		}
		slices.SortFunc(result, func(a, b rlarkv1alpha1.NodeDrainOperationStatus) int {
			return strings.Compare(a.OperationID, b.OperationID)
		})
		return result, nil
	}

	// Track how many Tasks have a DrainRequest dispatched for each operation.
	// This lets us distinguish "Agent hasn't started dispatching yet" from
	// "Agent scanned and found zero RLark-managed workloads".
	dispatchedCount := make(map[string]int32)

	var tasks rlarkv1alpha1.TaskList
	if err := r.List(ctx, &tasks, client.InNamespace(node.Namespace)); err != nil {
		return nil, fmt.Errorf("list Tasks for node drain: %w", err)
	}
	for _, task := range tasks.Items {
		for _, request := range task.Spec.DrainRequests {
			status, found := operations[request.OperationID]
			if !found || request.NodeUID != string(node.Status.KubernetesNodeUID) {
				continue
			}
			// This Task has a DrainRequest dispatched for this operation.
			dispatchedCount[request.OperationID]++

			matched := false
			for _, observed := range task.Status.DrainRequests {
				if observed.OperationID != request.OperationID || observed.NodeUID != request.NodeUID || observed.ObservedGeneration != task.Generation {
					continue
				}
				matched = true
				if observed.State == rlarkv1alpha1.TaskDrainStateWaitingForReschedule {
					status.Waiting++
				} else {
					status.Blocked++
				}
				break
			}
			if !matched {
				status.Blocked++
			}
			operations[request.OperationID] = status
		}
	}

	result := make([]rlarkv1alpha1.NodeDrainOperationStatus, 0, len(operations))
	for opID, status := range operations {
		if !status.Completed {
			status.Completed = isDrainOperationCompleted(node, opID, status, dispatchedCount[opID])
		}
		result = append(result, status)
	}
	slices.SortFunc(result, func(a, b rlarkv1alpha1.NodeDrainOperationStatus) int {
		return strings.Compare(a.OperationID, b.OperationID)
	})
	return result, nil
}

// isDrainOperationCompleted decides whether a Node Drain operation can be marked
// as Completed. It must not conflate "the Agent hasn't scanned the node yet"
// with "the node genuinely has zero RLark-managed workloads".
//
// Completion rules:
//  1. If at least one Task has received a DrainRequest for this operation, the
//     operation completes only when Blocked==0 (all dispatched Tasks have moved
//     out of Blocked state).
//  2. If no Task has received a DrainRequest yet, we consult the Agent's
//     workload observation:
//     - Agent hasn't reported its workload snapshot yet → not completed (Agent
//     may not have scanned the node).
//     - Agent reported RLarkManaged==0 → completed (genuinely zero RLark work).
//     - Agent reported RLarkManaged>0 but nothing dispatched → not completed
//     (Agent hasn't dispatched DrainRequests yet, e.g. just after Drain was
//     requested).
func isDrainOperationCompleted(node *rlarkv1alpha1.Node, opID string, status rlarkv1alpha1.NodeDrainOperationStatus, dispatchedCount int32) bool {
	if dispatchedCount > 0 {
		// At least one DrainRequest was dispatched. Complete only when nothing
		// is Blocked; WaitingForReschedule does not block completion because the
		// workload has already stopped running on this Node.
		return status.Blocked == 0
	}

	// No DrainRequest dispatched for this operation on any Task.
	// Check whether the Agent has scanned the node's RLark workloads.
	summary := node.Status.ActiveWorkloads
	if summary.Total == 0 && summary.RLarkManaged == 0 && node.Status.KubernetesNodeUID == "" {
		// "Agent has scanned" must be read from a field the Agent owns.
		// KubernetesNodeUID is written on every push and on nothing else, so an
		// empty value means no observation has arrived yet and an all-zero
		// summary cannot be distinguished from a genuinely empty Node. Be
		// conservative and wait for the first report.
		return false
	}

	// Agent has reported its workload observation. Use RLarkManaged to decide:
	if summary.RLarkManaged == 0 {
		// Agent scanned the node and found no RLark-managed Pods.
		// Nothing to drain → operation completes immediately.
		return true
	}

	// Agent found RLark-managed Pods but no DrainRequest was dispatched yet.
	// The Agent's pull controller hasn't acted on this Drain operation yet.
	return false
}

func nodeDrainCompleted(node *rlarkv1alpha1.Node) bool {
	found := false
	for _, operation := range node.Status.DrainOperations {
		found = true
		if !operation.Completed {
			return false
		}
	}
	return found
}

// nodeConnectionReason maps a heartbeat verdict to the stable Reason the
// AgentConnected condition carries.
//
// A Lease that exists but was never renewed is reported the same way as a
// missing Lease: neither of them proves that an agent ever ran.
func nodeConnectionReason(state sharedreconcile.HeartbeatState) string {
	switch state {
	case sharedreconcile.HeartbeatActive:
		return rlarkv1alpha1.ConditionReasonLeaseValid
	case sharedreconcile.HeartbeatExpired:
		return rlarkv1alpha1.ConditionReasonLeaseExpired
	default:
		return rlarkv1alpha1.ConditionReasonHeartbeatNeverReceived
	}
}

// setNodeConditions renders every condition this controller owns and reports
// whether the recorded condition list changed.
//
// None of these conditions declares an ObservedGeneration. They all answer
// "what was observed" - from the heartbeat Lease and from status fields the
// node agent reported - rather than "was this Spec acted on", so a
// per-condition generation would claim a contract this controller cannot
// honour: a Spec change alone does not make any of them fresh. A consumer that
// needs freshness compares status.observedGeneration instead.
//
// The mutations are applied in three passes because the later conditions are
// roll-ups of the earlier ones: Ready and Degraded read the capability
// conditions, and Schedulable reads AgentConnected, Ready, KubernetesCordoned,
// and ResourcePressure.
func setNodeConditions(node *rlarkv1alpha1.Node, connected bool, connectionReason string, now time.Time) bool {
	transition := metav1.NewTime(now)
	kubernetesStatus, _ := kubernetesReadyCondition(node.Status.KubernetesReady)
	runtimeReadyReason := runtimeReason(kubernetesStatus)
	networkReadyReason := availabilityReason(kubernetesStatus, rlarkv1alpha1.ConditionReasonNetworkAvailable, rlarkv1alpha1.ConditionReasonNetworkUnavailable)
	storageReadyStatus := storageConditionStatus(node.Status.Storage, node.Status.DiskPressure)
	storageReadyReason := storageReason(node.Status.Storage, node.Status.DiskPressure)
	deviceReadyReason := availabilityReason(kubernetesStatus, rlarkv1alpha1.ConditionReasonDeviceAvailable, rlarkv1alpha1.ConditionReasonDeviceUnavailable)
	resourcePressureReadyStatus := resourcePressureStatus(node.Status.DiskPressure)
	resourcePressureReadyReason := resourcePressureReason(node.Status.DiskPressure)
	kubernetesCordonedStatus, kubernetesCordonedReason := kubernetesCordonedCondition(node.Status.KubernetesCordoned)

	draining := hasOperation(node, rlarkv1alpha1.NodeOperationDrain)
	drainingStatus, drainingReason := drainingCondition(node, draining)

	// Pass one: the directly observed conditions.
	changed := sharedreconcile.ApplyConditionMutations(&node.Status.Conditions, 0, []sharedreconcile.ConditionMutation{
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeAgentConnected,
			Status:             boolConditionStatus(connected),
			Reason:             connectionReason,
			Message:            "Agent connectivity is derived from the node heartbeat Lease.",
			LastTransitionTime: transition,
		}},
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeRuntimeReady,
			Status:             kubernetesStatus,
			Reason:             runtimeReadyReason,
			Message:            "Runtime availability is derived from the Agent-reported Kubernetes Node readiness.",
			LastTransitionTime: transition,
		}},
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeNetworkReady,
			Status:             kubernetesStatus,
			Reason:             networkReadyReason,
			Message:            "Network availability is derived from the Agent-reported Kubernetes Node readiness.",
			LastTransitionTime: transition,
		}},
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeStorageReady,
			Status:             storageReadyStatus,
			Reason:             storageReadyReason,
			Message:            "Storage availability is derived from the Agent-reported storage snapshot and disk pressure.",
			LastTransitionTime: transition,
		}},
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeDeviceReady,
			Status:             kubernetesStatus,
			Reason:             deviceReadyReason,
			Message:            "Device availability is derived from the Agent-reported Kubernetes Node readiness.",
			LastTransitionTime: transition,
		}},
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeResourcePressure,
			Status:             resourcePressureReadyStatus,
			Reason:             resourcePressureReadyReason,
			Message:            "Resource pressure is derived from the Agent-reported Kubernetes disk pressure.",
			LastTransitionTime: transition,
		}},
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeKubernetesCordoned,
			Status:             kubernetesCordonedStatus,
			Reason:             kubernetesCordonedReason,
			Message:            "Cordon state is the Agent-observed Kubernetes Node spec.unschedulable. RLark does not manage that field, but a cordoned Node blocks every Pod on it, so it makes the Node unschedulable for RLark placement as well.",
			LastTransitionTime: transition,
		}},
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeDraining,
			Status:             drainingStatus,
			Reason:             drainingReason,
			Message:            "Draining reflects the current Node drain operation request and Task observations.",
			LastTransitionTime: transition,
		}},
	})

	// Pass two: the roll-ups that read the conditions written above. A
	// completed drain no longer holds readiness; the Node remains cordoned.
	readyStatus, readyReason := nodeReadyCondition(connected, connectionReason, kubernetesStatus, node.Status.Conditions, draining && !nodeDrainCompleted(node))
	degradedStatus, degradedReason := nodeDegradedCondition(node.Status.Conditions)
	if sharedreconcile.ApplyConditionMutations(&node.Status.Conditions, 0, []sharedreconcile.ConditionMutation{
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeReady,
			Status:             readyStatus,
			Reason:             readyReason,
			Message:            "Readiness is derived from Agent connectivity, Kubernetes Node readiness, drain state, and critical runtime, network, and device availability; resource pressure does not change readiness.",
			LastTransitionTime: transition,
		}},
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeDegraded,
			Status:             degradedStatus,
			Reason:             degradedReason,
			Message:            "Degraded reports resource pressure or failed capability observations for diagnosis; it does not change readiness, and resource pressure separately excludes the Node from new placement.",
			LastTransitionTime: transition,
		}},
	}) {
		changed = true
	}

	// Pass three: the placement decision, which reads Ready.
	schedulableStatus, schedulableReason := nodeSchedulableCondition(node)
	if sharedreconcile.ApplyConditionMutations(&node.Status.Conditions, 0, []sharedreconcile.ConditionMutation{
		{Condition: metav1.Condition{
			Type:               rlarkv1alpha1.ConditionTypeSchedulable,
			Status:             schedulableStatus,
			Reason:             schedulableReason,
			Message:            "Schedulability is the single placement decision; it combines spec.unschedulable with Agent connectivity, readiness, an external Kubernetes cordon, and scheduler-relevant resource pressure.",
			LastTransitionTime: transition,
		}},
	}) {
		changed = true
	}
	return changed
}

// nodeSchedulableCondition derives the single placement decision shared by the
// Job scheduler and every API consumer. It must run after AgentConnected,
// Ready, KubernetesCordoned, and ResourcePressure have been written into
// node.Status.Conditions.
func nodeSchedulableCondition(node *rlarkv1alpha1.Node) (metav1.ConditionStatus, string) {
	if node.Spec.Unschedulable {
		return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonUnschedulable
	}
	for _, conditionType := range []string{
		rlarkv1alpha1.ConditionTypeAgentConnected,
		rlarkv1alpha1.ConditionTypeReady,
	} {
		condition := apimeta.FindStatusCondition(node.Status.Conditions, conditionType)
		if condition == nil {
			return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonNotSchedulable
		}
		if condition.Status != metav1.ConditionTrue {
			return metav1.ConditionFalse, conditionReason(condition, rlarkv1alpha1.ConditionReasonNotSchedulable)
		}
	}
	// A Kubernetes Node cordoned outside RLark refuses every Pod, so placing new
	// Tasks on it would leave them Pending. Unknown or missing state keeps the
	// previous behavior: only an explicit True excludes the Node.
	if condition := apimeta.FindStatusCondition(node.Status.Conditions, rlarkv1alpha1.ConditionTypeKubernetesCordoned); condition != nil && condition.Status == metav1.ConditionTrue {
		return metav1.ConditionFalse, conditionReason(condition, rlarkv1alpha1.ConditionReasonKubernetesCordoned)
	}
	// Unknown or missing ResourcePressure keeps the previous scheduler behavior:
	// only an explicit True excludes the Node.
	if condition := apimeta.FindStatusCondition(node.Status.Conditions, rlarkv1alpha1.ConditionTypeResourcePressure); condition != nil && condition.Status == metav1.ConditionTrue {
		return metav1.ConditionFalse, conditionReason(condition, rlarkv1alpha1.ConditionReasonResourcePressureDetected)
	}
	return metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonSchedulable
}

// sameTime reports whether two optional timestamps denote the same instant.
// Comparing the pointers would report a change on every pass.
func sameTime(left, right *metav1.Time) bool {
	switch {
	case left == nil && right == nil:
		return true
	case left == nil || right == nil:
		return false
	default:
		return left.Time.Equal(right.Time)
	}
}

func boolConditionStatus(value bool) metav1.ConditionStatus {
	if value {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func kubernetesReadyCondition(ready *bool) (metav1.ConditionStatus, string) {
	if ready == nil {
		return metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonKubernetesNodeUnknown
	}
	if *ready {
		return metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonKubernetesNodeReady
	}
	return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesNodeNotReady
}

// kubernetesCordonedCondition reports the Agent-observed Kubernetes Node cordon
// state. RLark does not manage spec.unschedulable, but an explicit cordon makes
// the Node unschedulable for RLark placement too, because the Kubernetes
// scheduler refuses every Pod on it.
func kubernetesCordonedCondition(cordoned *bool) (metav1.ConditionStatus, string) {
	if cordoned == nil {
		return metav1.ConditionUnknown, rlarkv1alpha1.ConditionReasonKubernetesCordonUnobserved
	}
	if *cordoned {
		return metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonKubernetesCordoned
	}
	return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesNotCordoned
}

func runtimeReason(status metav1.ConditionStatus) string {
	return availabilityReason(status, rlarkv1alpha1.ConditionReasonRuntimeAvailable, rlarkv1alpha1.ConditionReasonRuntimeUnavailable)
}

func availabilityReason(status metav1.ConditionStatus, available, unavailable string) string {
	if status == metav1.ConditionTrue {
		return available
	}
	return unavailable
}

func storageConditionStatus(storage *rlarkv1alpha1.NodeStorageStatus, diskPressure *bool) metav1.ConditionStatus {
	if diskPressure != nil && *diskPressure {
		return metav1.ConditionFalse
	}
	if storage == nil {
		return metav1.ConditionUnknown
	}
	return metav1.ConditionTrue
}

func storageReason(storage *rlarkv1alpha1.NodeStorageStatus, diskPressure *bool) string {
	if diskPressure != nil && *diskPressure {
		return rlarkv1alpha1.ConditionReasonStorageUnavailable
	}
	if storage == nil {
		return rlarkv1alpha1.ConditionReasonStorageUnavailable
	}
	return rlarkv1alpha1.ConditionReasonStorageAvailable
}

func resourcePressureStatus(diskPressure *bool) metav1.ConditionStatus {
	if diskPressure == nil {
		return metav1.ConditionUnknown
	}
	return boolConditionStatus(*diskPressure)
}

func resourcePressureReason(diskPressure *bool) string {
	if diskPressure == nil {
		return rlarkv1alpha1.ConditionReasonResourcePressureUnknown
	}
	if *diskPressure {
		return rlarkv1alpha1.ConditionReasonResourcePressureDetected
	}
	return rlarkv1alpha1.ConditionReasonNoResourcePressure
}

func drainingCondition(node *rlarkv1alpha1.Node, draining bool) (metav1.ConditionStatus, string) {
	if !draining {
		return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonNoDrainRequest
	}
	if nodeDrainCompleted(node) {
		return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonDrainComplete
	}
	for _, operation := range node.Status.DrainOperations {
		if operation.Blocked > 0 {
			return metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonDrainBlocked
		}
	}
	return metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonDrainRequested
}

// nodeReadyCondition reports whether the Node can keep running workloads.
// Scheduler-relevant resource pressure is reported through ResourcePressure and
// Degraded instead of failing readiness: the scheduler excludes pressure
// explicitly, so existing workloads stay Ready while new placement is blocked.
func nodeReadyCondition(connected bool, connectionReason string, kubernetesStatus metav1.ConditionStatus, conditions []metav1.Condition, draining bool) (metav1.ConditionStatus, string) {
	if !connected {
		return metav1.ConditionFalse, connectionReason
	}
	if draining {
		return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonDrainRequested
	}
	if kubernetesStatus != metav1.ConditionTrue {
		// Readiness is two-state. An unobserved Kubernetes Node Ready condition
		// must not be treated as ready, so it fails readiness instead of surfacing
		// an Unknown state.
		if kubernetesStatus == metav1.ConditionUnknown {
			return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesNodeReadyUnobserved
		}
		return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonKubernetesNodeNotReady
	}
	for _, conditionType := range []string{
		rlarkv1alpha1.ConditionTypeRuntimeReady,
		rlarkv1alpha1.ConditionTypeNetworkReady,
		rlarkv1alpha1.ConditionTypeDeviceReady,
	} {
		condition := apimeta.FindStatusCondition(conditions, conditionType)
		if condition != nil && condition.Status == metav1.ConditionFalse {
			return metav1.ConditionFalse, conditionReason(condition, rlarkv1alpha1.ConditionReasonRuntimeUnavailable)
		}
	}
	return metav1.ConditionTrue, rlarkv1alpha1.ConditionReasonKubernetesNodeReady
}

func conditionReason(condition *metav1.Condition, fallback string) string {
	if condition != nil && condition.Reason != "" {
		return condition.Reason
	}
	return fallback
}

func nodeDegradedCondition(conditions []metav1.Condition) (metav1.ConditionStatus, string) {
	if condition := apimeta.FindStatusCondition(conditions, rlarkv1alpha1.ConditionTypeResourcePressure); condition != nil && condition.Status == metav1.ConditionTrue {
		return metav1.ConditionTrue, condition.Reason
	}
	for _, conditionType := range []string{
		rlarkv1alpha1.ConditionTypeRuntimeReady,
		rlarkv1alpha1.ConditionTypeNetworkReady,
		rlarkv1alpha1.ConditionTypeStorageReady,
		rlarkv1alpha1.ConditionTypeDeviceReady,
	} {
		condition := apimeta.FindStatusCondition(conditions, conditionType)
		if condition != nil && condition.Status == metav1.ConditionFalse {
			return metav1.ConditionTrue, condition.Reason
		}
	}
	return metav1.ConditionFalse, rlarkv1alpha1.ConditionReasonNoResourcePressure
}

func nodePhase(node *rlarkv1alpha1.Node, connected bool) rlarkv1alpha1.NodePhase {
	// A completed Drain operation stays in spec.operations until an operator
	// removes it, so it must not keep the Node in Draining. Continue with the
	// durable scheduling intent, which reports the cordoned Node as Cordoned.
	if hasOperation(node, rlarkv1alpha1.NodeOperationDrain) && !nodeDrainCompleted(node) {
		return rlarkv1alpha1.NodeDraining
	}
	if hasOperation(node, rlarkv1alpha1.NodeOperationEnterMaintenance) {
		return rlarkv1alpha1.NodeCordoned
	}
	if !connected {
		return rlarkv1alpha1.NodeOffline
	}
	if node.Status.KubernetesReady == nil {
		return rlarkv1alpha1.NodeRegistering
	}
	if node.Spec.Unschedulable {
		return rlarkv1alpha1.NodeCordoned
	}
	if !*node.Status.KubernetesReady {
		return rlarkv1alpha1.NodeDegraded
	}
	if hasDegradedObservation(node.Status.Conditions) {
		return rlarkv1alpha1.NodeDegraded
	}
	return rlarkv1alpha1.NodeOnline
}

func hasOperation(node *rlarkv1alpha1.Node, operationType rlarkv1alpha1.NodeOperationType) bool {
	for _, operation := range node.Spec.Operations {
		if operation.Type == operationType {
			return true
		}
	}
	return false
}

func hasDegradedObservation(conditions []metav1.Condition) bool {
	condition := apimeta.FindStatusCondition(conditions, rlarkv1alpha1.ConditionTypeDegraded)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

func (r *Reconciler) getHeartbeatLease(ctx context.Context, namespace, nodeName string) (*coordinationv1.Lease, error) {
	var lease coordinationv1.Lease
	if err := r.Get(ctx, client.ObjectKey{Namespace: namespace, Name: nodeHeartbeatLeaseName(nodeName)}, &lease); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get agent heartbeat lease: %w", err)
	}
	return &lease, nil
}

func (r *Reconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// SetupWithManager registers the controller with Node and Agent Lease watches.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&rlarkv1alpha1.Node{}).
		Watches(
			&coordinationv1.Lease{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
				nodeName, ok := strings.CutPrefix(object.GetName(), nodeHeartbeatLeasePrefix)
				if !ok || nodeName == "" {
					return nil
				}
				return []reconcile.Request{{NamespacedName: client.ObjectKey{Namespace: object.GetNamespace(), Name: nodeName}}}
			}),
		).
		Named("node").
		WithOptions(controllerconfig.Options{MaxConcurrentReconciles: r.MaxConcurrentReconciles}).
		Complete(r)
}
