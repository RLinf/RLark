package node

import (
	"fmt"
	"strings"
)

const (
	nodeRobotLabelPrefix = "rlark.io/robot-"
	// A node's place in a rolling change of its machine (an integer, earlier
	// waves first) and a request that the whole machine restart (an id; the
	// machine reports what became of it in an annotation of its own).
	nodeRolloutWaveLabel    = "rlark.io/rollout-wave"
	nodeRestartRequestLabel = "rlark.io/restart-request"
	// The accelerator a node has (nvidia): the management plane declares it, and the nvidia-gpu Addon selects by it
	// (the Gateway's new cluster labels its GPU machine so; a node never labels itself).
	nodeAcceleratorLabel = "rlark.io/accelerator"
)

// siteOwnedLabels are the labels a site declares on the management Node
// besides RLark's own: SetManagementOwnedLabels.
var siteOwnedLabels []string

// SetManagementOwnedLabels names further labels whose value is the
// management Node's (--management-node-labels): a key, or all keys that begin
// with an entry ending in "/" or "-". A label a node gives itself must not be
// among them: the agent removes from the node what the management Node lacks.
func SetManagementOwnedLabels(entries []string) error {
	for _, entry := range entries {
		domain, _, named := strings.Cut(entry, "/")
		if !named || domain == "" || domain == "kubernetes.io" || domain == "k8s.io" || strings.HasSuffix(domain, ".kubernetes.io") || strings.HasSuffix(domain, ".k8s.io") {
			return fmt.Errorf("management node label %q: a label of a domain other than Kubernetes' own, as domain/name or domain/ for all of them", entry)
		}
	}
	siteOwnedLabels = entries
	return nil
}
