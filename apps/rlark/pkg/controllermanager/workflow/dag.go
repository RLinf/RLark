package workflow

import (
	"fmt"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

type dag struct {
	templates  map[string]rlarkv1alpha1.WorkflowJobTemplate
	inDegree   map[string]int
	dependents map[string][]string
	resolved   map[string]bool
}

func newDAG(templates []rlarkv1alpha1.WorkflowJobTemplate) (*dag, error) {
	d := &dag{
		templates:  make(map[string]rlarkv1alpha1.WorkflowJobTemplate, len(templates)),
		inDegree:   make(map[string]int, len(templates)),
		dependents: make(map[string][]string),
		resolved:   make(map[string]bool, len(templates)),
	}

	for _, jt := range templates {
		d.templates[jt.Name] = jt
		d.inDegree[jt.Name] = len(jt.Dependencies)
		for _, dep := range jt.Dependencies {
			if _, ok := d.templates[dep]; !ok {
				return nil, fmt.Errorf("job %q depends on unknown job %q", jt.Name, dep)
			}
			d.dependents[dep] = append(d.dependents[dep], jt.Name)
		}
	}

	return d, nil
}

func (d *dag) resolve(name string) {
	if d.resolved[name] {
		return
	}
	d.resolved[name] = true
	for _, dep := range d.dependents[name] {
		d.inDegree[dep]--
	}
}

func (d *dag) dispatchReady(jobStatusMap map[string]*rlarkv1alpha1.WorkflowJobStatus) []string {
	var ready []string
	for name := range d.templates {
		if d.resolved[name] || d.inDegree[name] != 0 {
			continue
		}
		if js := jobStatusMap[name]; js != nil && js.Phase != "" {
			continue
		}
		ready = append(ready, name)
	}
	return ready
}
