package agent

import "sync"

// The parts of an agent that say when they are ready.
const (
	// partClusterWatches: the cluster agent's first watches of the management
	// plane and of its own cluster have synced.
	partClusterWatches = "cluster-watches"
	// partNodeWatches: the node agent's first watches of the management plane have synced.
	partNodeWatches = "node-watches"
	// partNodeServer: the node server is serving the node's workloads.
	partNodeServer = "node-server"
)

// readiness is what the agent's parts say of themselves. The agent is ready
// when every part it runs has said so. It answers /readyz, and it is what
// the agent tells the Server on its tunnel, which keeps the heartbeat of a
// ready agent only: a cluster or node reads Online when its agent is
// connected and doing its work, not when it is merely connected.
type readiness struct {
	mu      sync.Mutex
	parts   map[string]bool
	changed chan struct{}
}

func newReadiness() *readiness {
	return &readiness{parts: map[string]bool{}, changed: make(chan struct{})}
}

// set records what a part says.
func (r *readiness) set(part string, ready bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.all()
	r.parts[part] = ready
	if r.all() != before {
		close(r.changed)
		r.changed = make(chan struct{})
	}
}

// state returns whether the agent is ready, and a channel that is closed when that changes.
func (r *readiness) state() (bool, <-chan struct{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.all(), r.changed
}

func (r *readiness) all() bool {
	for _, ready := range r.parts {
		if !ready {
			return false
		}
	}
	return len(r.parts) > 0
}
