package task

import (
	"context"
	"fmt"

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

func (l kubernetesLifecycle) BeforeReconcile(ctx context.Context, input host.LifecycleInput) (host.LifecycleDecision, error) {
	task, err := kubernetesLifecycleTask(input)
	if err != nil {
		return host.LifecycleDecision{}, err
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

func kubernetesLifecycleTask(input host.LifecycleInput) (*rlarkv1alpha1.Task, error) {
	task, ok := input.External.(*rlarkv1alpha1.Task)
	if !ok || task == nil {
		return nil, fmt.Errorf("kubernetes lifecycle expected *v1alpha1.Task, got %T", input.External)
	}
	return task, nil
}
