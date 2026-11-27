package node

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
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
	// The robot Station a node stands at (S01, S02, …): given at its admission, changed through the Gateway.
	nodeStationLabel = "rlark.io/station"
)

// downwardNodeAnnotations are the management Node's annotations the node itself acts on: the restart its claim
// holder asked for (the Gateway's POST nodes/{name}/robot-restart), which the robot's power service runs, and its
// Station's mark with its reason (the Gateway's station mark), which the robot's power service serves to its device
// layer for its lamp. Both are the management plane's (isSiteOwnedNodeAnnotation), so neither goes back up.
var downwardNodeAnnotations = []string{"rlark.io/robot-restart", "rlark.io/station-mark-set"}

// pullManagementAnnotations puts downwardNodeAnnotations of the management Node on the local Node, and answers
// whether it changed it.
func pullManagementAnnotations(mgmtNode *rlarkv1alpha1.Node, localNode *corev1.Node) bool {
	changed := false
	for _, key := range downwardNodeAnnotations {
		value, declared := mgmtNode.Annotations[key]
		if current, present := localNode.Annotations[key]; present == declared && current == value {
			continue
		}
		if !declared {
			delete(localNode.Annotations, key)
		} else {
			if localNode.Annotations == nil {
				localNode.Annotations = map[string]string{}
			}
			localNode.Annotations[key] = value
		}
		changed = true
	}
	return changed
}

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

// isSiteOwnedNodeLabel: a robot's identity (rlark.io/robot-model, -body, -id) is declared on its management Node
// like the node's category, and so are what people ask of a node through the management plane (its rollout wave
// and a restart), its accelerator, and the labels the site names.
func isSiteOwnedNodeLabel(key string) bool {
	if strings.HasPrefix(key, nodeRobotLabelPrefix) || key == nodeRolloutWaveLabel || key == nodeRestartRequestLabel || key == nodeAcceleratorLabel || key == "rlark.io/station-mark" || key == nodeStationLabel {
		return true
	}
	for _, entry := range siteOwnedLabels {
		if key == entry || (strings.HasSuffix(entry, "/") || strings.HasSuffix(entry, "-")) && strings.HasPrefix(key, entry) {
			return true
		}
	}
	return false
}

// isSiteOwnedNodeAnnotation: what the management plane says of a robot beside its labels (who set its desired power).
func isSiteOwnedNodeAnnotation(key string) bool {
	return strings.HasPrefix(key, nodeRobotLabelPrefix) || key == "rlark.io/station-mark-set"
}

// nodeReport is one report of a node onto its management Node, written as a merge patch of only the fields the
// report owns: a full Update from an agent built against an older NodeSpec would drop the fields it does not know,
// such as the spec.displayName an administrator set.
type nodeReport struct {
	before client.Patch
	tenant string
}

func beginNodeReport(mgmtNode *rlarkv1alpha1.Node) nodeReport {
	return nodeReport{before: client.MergeFrom(mgmtNode.DeepCopy()), tenant: mgmtNode.Labels[tenant.IDLabel]}
}

// patch writes the report. The gateway writes the tenant onto every object it stores; the node has no such label,
// and the report must not take it off the management Node.
func (r nodeReport) patch(ctx context.Context, management client.Client, mgmtNode *rlarkv1alpha1.Node) error {
	if _, reported := mgmtNode.Labels[tenant.IDLabel]; !reported && r.tenant != "" {
		mgmtNode.Labels[tenant.IDLabel] = r.tenant
	}
	return management.Patch(ctx, mgmtNode, r.before)
}
