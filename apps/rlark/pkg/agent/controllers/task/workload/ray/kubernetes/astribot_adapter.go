package kubernetes

import (
	"encoding/json"

	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// daemonSetRollout decodes a per-node Task's DaemonSet carrier parameters, its rollout policy; the other carriers
// have none.
func daemonSetRollout(kind string, carrier coordination.VersionedConfiguration) (rlarkv1alpha1.RolloutPolicy, error) {
	if kind != CarrierDaemonSet {
		return rlarkv1alpha1.RolloutPolicy{}, nil
	}
	decoded, err := taskapi.DecodeKubernetesDaemonSetCarrierParameters(apiextensionsv1.JSON{Raw: carrier.Parameters})
	if err != nil {
		return rlarkv1alpha1.RolloutPolicy{}, fault.Wrap(fault.InvalidArgument, "decode DaemonSet Kubernetes carrier parameters", err)
	}
	return decoded.RolloutPolicy, nil
}

// launcherArgs: a Task without a Ray role (its Job has no head) still runs through the launcher, which then runs
// only its prepare and run scripts.
func launcherArgs(role string) []string {
	if role == "" {
		return []string{"ray-launcher"}
	}
	return []string{"ray-launcher", "--role=" + role}
}

// observeDaemonSet projects a per-node Task's DaemonSet. A failing instance holds the Task in Pending but does not
// fail it as it would a replicated carrier.
func observeDaemonSet(manifest []byte, instances agentruntime.InstanceSnapshot) (rlarkv1alpha1.TaskPhase, string, error) {
	var object appsv1.DaemonSet
	if err := json.Unmarshal(manifest, &object); err != nil {
		return "", "", err
	}
	phase, message := DaemonSetStatusPhase(&object, instances)
	return phase, message, nil
}
