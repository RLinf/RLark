package task

import (
	"sort"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func observedRootInstanceNodes(result coordinator.ObserveResult) []string {
	return instanceNodes(result.Instances)
}

func instanceNodes(instances agentruntime.InstanceSnapshot) []string {
	nodes := make(map[string]struct{})
	for _, instance := range instances.Instances {
		for _, condition := range instance.Conditions {
			if condition.Source == "kubernetes" && condition.Scope == "node" && condition.Subject != "" {
				nodes[condition.Subject] = struct{}{}
			}
		}
	}
	resultNodes := make([]string, 0, len(nodes))
	for node := range nodes {
		resultNodes = append(resultNodes, node)
	}
	sort.Strings(resultNodes)
	return resultNodes
}

func kubernetesWorkloadRef(task *rlarkv1alpha1.Task, object client.Object, kind string) agentruntime.ObjectRef {
	return agentruntime.ObjectRef{RuntimeID: kubernetesruntime.DefaultID, ID: "apps/v1, Kind=" + kind, Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(object.GetUID()), LogicalTask: agentruntime.LogicalTaskID{Namespace: task.Namespace, Name: task.Name}}
}
