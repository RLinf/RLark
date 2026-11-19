package utils

import (
	"fmt"
	"strings"
)

// Topo performs topological sorting.
type Topo interface {
	GetName() string
	GetDependencies() []string
}

// TopologicalSort returns components in deployment order (dependencies first).
// Uses Kahn's algorithm. Returns error on cyclic dependencies.
func TopologicalSort(comps []Topo) ([]Topo, error) {
	compMap := make(map[string]*Topo, len(comps))
	for i := range comps {
		compMap[comps[i].GetName()] = &comps[i]
	}

	inDegree := make(map[string]int, len(comps))
	dependents := make(map[string][]string)
	for _, c := range comps {
		inDegree[c.GetName()] = len(c.GetDependencies())
		for _, dep := range c.GetDependencies() {
			if _, ok := compMap[dep]; ok {
				dependents[dep] = append(dependents[dep], c.GetName())
			} else {
				inDegree[c.GetName()]--
			}
		}
	}

	var queue []string
	for _, c := range comps {
		if inDegree[c.GetName()] == 0 {
			queue = append(queue, c.GetName())
		}
	}

	var sorted []Topo
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		sorted = append(sorted, *compMap[name])

		for _, dependent := range dependents[name] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	if len(sorted) != len(comps) {
		var cyclic []string
		for _, c := range comps {
			if inDegree[c.GetName()] > 0 {
				cyclic = append(cyclic, c.GetName())
			}
		}
		return nil, fmt.Errorf("cyclic dependency detected among: %s", strings.Join(cyclic, ", "))
	}

	return sorted, nil
}
