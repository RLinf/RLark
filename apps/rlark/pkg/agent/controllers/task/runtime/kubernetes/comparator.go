package kubernetes

import agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"

// ObjectComparator applies Kubernetes reconcile-profile comparison before the
// Coordinator selects an update operation. It performs no Runtime I/O.
type ObjectComparator struct{ Profiles ProfileRegistry }

// Equal compares only the profile-owned projection when one requires custom
// comparison, and otherwise uses JSON semantic equality.
func (c ObjectComparator) Equal(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
	if desired.ReconcileProfile == (agentruntime.ObjectKind{}) {
		return semanticJSONEqual(current, desired)
	}
	profiles := c.Profiles
	if profiles.executors == nil {
		profiles = BuiltinProfileRegistry()
	}
	return profiles.Equal(current, desired)
}
