package agent

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

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
//
// It stands where the official Agent has an atomic.Bool (Agent.ready), and
// Store and Load keep the official calls as they are: there the one who
// speaks is the node server.
type readiness struct {
	mu      sync.Mutex
	parts   map[string]bool
	changed chan struct{}
}

// Store is the node server saying whether it serves the node's workloads.
func (r *readiness) Store(ready bool) { r.set(partNodeServer, ready) }

// Load is whether the agent is ready.
func (r *readiness) Load() bool {
	ready, _ := r.state()
	return ready
}

// lock locks r. The Agent holds a readiness as its zero value, so its parts are made on first use.
func (r *readiness) lock() {
	r.mu.Lock()
	if r.parts == nil {
		r.parts, r.changed = map[string]bool{}, make(chan struct{})
	}
}

// set records what a part says.
func (r *readiness) set(part string, ready bool) {
	r.lock()
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
	r.lock()
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

// announce puts the agent's readiness on the header its tunnel connects with.
func (r *readiness) announce(header http.Header) (bool, <-chan struct{}) {
	ready, changed := r.state()
	header.Set(apis.RemoteDialerReadyHeader, strconv.FormatBool(ready))
	return ready, changed
}
