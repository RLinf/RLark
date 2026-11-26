package task

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	raykubernetes "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/workload/ray/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
)

type kubernetesEligibility struct{ agentType string }

func (e kubernetesEligibility) Matches(task *rlarkv1alpha1.Task) bool {
	return string(task.Spec.Runtime.Kind) == e.agentType
}

func (kubernetesEligibility) ValidationMessage(task *rlarkv1alpha1.Task) string {
	if task.Spec.Workload.Kind != taskapi.WorkloadTypeRay || task.Spec.Workload.Version != taskapi.WorkloadVersionRayV1 {
		return "Task has unsupported Kubernetes workload, skipping"
	}
	return ""
}

type kubernetesLifecycle struct{ controller *Controller }

// BeforeReconcile stops the Task root workload while a Node Drain request that
// the Node still backs is present. The Node Agent writes such a request from
// the Node's current Drain operations and removes it when the operation leaves
// the Node Spec, but that projection can be missed while the Agent is offline
// or after the Node object disappears. The Host therefore re-reads the Node and
// releases requests the Node no longer backs, so a Task drained in the past can
// always start again without waiting for that Agent.
// No dedicated rescheduler exists: the restarted workload is placed by normal
// Kubernetes scheduling.
func (l kubernetesLifecycle) BeforeReconcile(ctx context.Context, input host.LifecycleInput) (host.LifecycleDecision, error) {
	task, err := kubernetesLifecycleTask(input)
	if err != nil {
		return host.LifecycleDecision{}, err
	}
	if err := l.releaseStaleDrainRequests(ctx, task); err != nil {
		return host.LifecycleDecision{}, err
	}
	if request := currentDrainRequest(task); request != nil {
		intent := host.IntentForTask(task, rlarkv1alpha1.TaskPhaseStopped, "Task root workload stopped for Node Drain; the Task resumes when the Drain operation is removed", nil, host.StatusPriorityLifecycle)
		intent.Observed.DrainRequests = host.StoppedTaskDrainStatuses(task)
		return host.LifecycleDecision{
			Action:             host.LifecycleCleanup,
			Reason:             "NodeDrain",
			AfterCleanup:       host.LifecycleWait,
			AfterCleanupStatus: &intent,
		}, nil
	}
	if !task.Spec.Suspended && task.Status.Phase == rlarkv1alpha1.TaskPhaseStopped {
		intent := host.IntentForTask(task, rlarkv1alpha1.TaskPhasePending, "", nil, host.StatusPriorityLifecycle)
		return host.LifecycleDecision{Action: host.LifecycleWait, Reason: "StoppedStatusReset", Status: &intent, Retry: host.RetryDirective{Class: host.RetryOnChange}}, nil
	}
	if task.Spec.Workload.Template == nil {
		return host.LifecycleDecision{}, fmt.Errorf("ray workload requires a Pod template")
	}
	if err := raykubernetes.EnsureImagePullSecrets(ctx, l.controller.LocalKubeClient, task.Spec.Workload.Template, getWorkloadNamespace(task)); err != nil {
		return host.LifecycleDecision{}, fmt.Errorf("ensure image pull secrets: %w", err)
	}
	return host.LifecycleDecision{Action: host.LifecycleProceed, Reason: "Ready"}, nil
}

func (l kubernetesLifecycle) AfterReconcile(_ context.Context, input host.LifecycleInput) (*host.StatusIntent, error) {
	_, err := kubernetesLifecycleTask(input)
	return nil, err
}

// releaseStaleDrainRequests drops Drain requests that no longer correspond to a
// Drain operation on the Node they name. A request is a projection of Node
// state, not user intent, so a Task must not stay stopped once the operation
// that produced it is gone. Publishing the release here also covers Nodes whose
// Agent stopped reconciling, which would otherwise leave every Task that was
// ever drained stopped forever.
func (l kubernetesLifecycle) releaseStaleDrainRequests(ctx context.Context, task *rlarkv1alpha1.Task) error {
	if len(task.Spec.DrainRequests) == 0 {
		return nil
	}
	live := make([]rlarkv1alpha1.TaskDrainRequest, 0, len(task.Spec.DrainRequests))
	released := make([]string, 0, len(task.Spec.DrainRequests))
	for _, request := range task.Spec.DrainRequests {
		backed, err := l.drainRequestBackedByNode(ctx, task.Namespace, request)
		if err != nil {
			return err
		}
		if backed {
			live = append(live, request)
			continue
		}
		released = append(released, request.OperationID)
	}
	if len(released) == 0 {
		return nil
	}
	before := task.DeepCopy()
	if len(live) == 0 {
		task.Spec.DrainRequests = nil
	} else {
		task.Spec.DrainRequests = live
	}
	if err := l.controller.ManagementClient.Patch(ctx, task, client.MergeFromWithOptions(before, client.MergeFromWithOptimisticLock{})); err != nil {
		return fmt.Errorf("release stale Task Drain requests: %w", err)
	}
	log.FromContext(ctx).Info("released stale Task Drain requests",
		"task", client.ObjectKeyFromObject(task), "operations", released)
	return nil
}

// drainRequestBackedByNode reports whether the Management Node named by the
// request still carries a Drain operation matching the request. A missing or
// terminating Node cannot enact the request, so it does not back it. The
// Completed status is intentionally NOT considered here: an operation that
// has finished draining but still remains in node.spec.operations (waiting for
// the operator to undrain) must continue to keep the Task stopped, because
// the Node remains cordoned and the Task must not auto-restart on it.
func (l kubernetesLifecycle) drainRequestBackedByNode(ctx context.Context, namespace string, request rlarkv1alpha1.TaskDrainRequest) (bool, error) {
	if request.OperationID == "" || request.NodeName == "" {
		return false, nil
	}
	var node rlarkv1alpha1.Node
	if err := l.controller.ManagementClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: request.NodeName}, &node); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("get Node %s/%s for Task Drain request: %w", namespace, request.NodeName, err)
	}
	if node.DeletionTimestamp != nil {
		return false, nil
	}
	for _, operation := range node.Spec.Operations {
		if operation.Type == rlarkv1alpha1.NodeOperationDrain && operation.ID == request.OperationID {
			return true, nil
		}
	}
	return false, nil
}

func currentDrainRequest(task *rlarkv1alpha1.Task) *rlarkv1alpha1.TaskDrainRequest {
	if task == nil {
		return nil
	}
	for i := range task.Spec.DrainRequests {
		request := &task.Spec.DrainRequests[i]
		if request.OperationID != "" && request.NodeUID != "" {
			return request
		}
	}
	return nil
}

func kubernetesLifecycleTask(input host.LifecycleInput) (*rlarkv1alpha1.Task, error) {
	task, ok := input.External.(*rlarkv1alpha1.Task)
	if !ok || task == nil {
		return nil, fmt.Errorf("kubernetes lifecycle expected *v1alpha1.Task, got %T", input.External)
	}
	return task, nil
}
