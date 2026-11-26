package task

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
)

func TestKubernetesAdmissionPrecedesLifecycleAndRuntimeWrites(t *testing.T) {
	for _, missing := range []string{"inventory", "environment"} {
		t.Run(missing, func(t *testing.T) {
			scheme := executionScheme(t)
			task := claimedExecutionTask(rlarkv1alpha1.TaskPhasePending)
			task.Finalizers, task.Annotations = nil, nil
			task.Spec.Workload.Template.Spec.Containers = []corev1.Container{{Name: "main", Image: "example:v1"}}
			if missing == "environment" {
				task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{Type: "unknown", Version: "v99"}}
			}
			management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
			local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
			execution, err := newTestKubernetesExecutionCoordinator(local, "rlark:test")
			if err != nil {
				t.Fatal(err)
			}
			if missing == "inventory" {
				execution.Runtimes[0].Info.Capabilities = nil
			}
			controller := newHostedTestController(t, base.Controller{AgentType: "Kubernetes", ManagementNamespace: task.Namespace, ManagementClient: management, LocalKubeClient: local}, execution)
			result, err := controller.host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
			if err != nil || result.Outcome != host.OutcomeBlocked || result.Status == nil || result.Retry.Class != host.RetryOnChange {
				t.Fatalf("admission=%s err=%v", result.Outcome, err)
			}
			if local.creates+local.updates+local.deletes != 0 {
				t.Fatal("invalid contract caused local lifecycle/Runtime writes")
			}
			var current rlarkv1alpha1.Task
			if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
				t.Fatal(err)
			}
			if len(current.Finalizers) != 0 {
				t.Fatal("invalid contract was claimed")
			}
			condition := apimeta.FindStatusCondition(current.Status.Conditions, host.ConditionExecutionAdmitted)
			if condition == nil || condition.Status != metav1.ConditionFalse || condition.Reason != "ExecutionAdmissionFailed" || condition.ObservedGeneration != current.Generation {
				t.Fatalf("invisible admission rejection: %#v", condition)
			}
			pull := &pullReconciler{c: controller}
			requeue, err := pull.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(task)})
			if err != nil || requeue.RequeueAfter != 0 {
				t.Fatalf("permanent rejection entered error/delayed retry: %#v %v", requeue, err)
			}
		})
	}
}

func TestKubernetesAdmissionPreservesDrainCleanupAndStoppedReset(t *testing.T) {
	for _, drain := range []bool{false, true} {
		scheme := executionScheme(t)
		task := claimedExecutionTask(rlarkv1alpha1.TaskPhaseStopped)
		task.Annotations[ManagementTaskClaimantAnnotation] = task.Namespace + "/Kubernetes"
		task.Spec.Environments = []rlarkv1alpha1.TaskEnvironment{{Type: "unknown", Version: "v99"}}
		task.Status.Conditions = []metav1.Condition{{Type: host.ConditionExecutionAdmitted, Status: metav1.ConditionFalse, Reason: "ExecutionAdmissionFailed", Message: "old rejection", ObservedGeneration: task.Generation, LastTransitionTime: metav1.Now()}}
		if drain {
			task.Status.Phase = rlarkv1alpha1.TaskPhaseRunning
			task.Spec.DrainRequests = []rlarkv1alpha1.TaskDrainRequest{{OperationID: "drain", NodeUID: "node", NodeName: "node"}}
		}
		management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
		if drain {
			node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Namespace: task.Namespace, Name: "node"}, Spec: rlarkv1alpha1.NodeSpec{Operations: []rlarkv1alpha1.NodeOperationRequest{{ID: "drain", Type: rlarkv1alpha1.NodeOperationDrain}}}}
			if err := management.Create(context.Background(), node); err != nil {
				t.Fatal(err)
			}
		}
		local := &executionCountingClient{Client: fake.NewClientBuilder().WithScheme(scheme).Build()}
		execution, err := newTestKubernetesExecutionCoordinator(local, "rlark:test")
		if err != nil {
			t.Fatal(err)
		}
		controller := newHostedTestController(t, base.Controller{AgentType: "Kubernetes", ManagementNamespace: task.Namespace, ManagementClient: management, LocalKubeClient: local}, execution)
		result, err := controller.host.Reconcile(context.Background(), client.ObjectKeyFromObject(task))
		if err != nil || result.Outcome != host.OutcomeWaitingForChange {
			t.Fatalf("cleanup/reset=%s err=%v", result.Outcome, err)
		}
		if local.creates+local.updates != 0 {
			t.Fatal("cleanup/reset created workload")
		}
		var current rlarkv1alpha1.Task
		if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &current); err != nil {
			t.Fatal(err)
		}
		if apimeta.FindStatusCondition(current.Status.Conditions, host.ConditionExecutionAdmitted) != nil {
			t.Fatalf("cleanup/reset reported skipped compilation as admission: drain=%t phase=%s conditions=%#v", drain, current.Status.Phase, current.Status.Conditions)
		}
	}
}
