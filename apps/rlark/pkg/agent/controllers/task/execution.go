package task

import (
	"sort"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
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
