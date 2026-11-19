package release

import (
	"bytes"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/yaml"
)

// stationDaemonSet is the DaemonSet name of the tree's astribot/station/deploy/station-agent.yaml (Station's
// DaemonSets as every member cluster runs all three; which nodes run which is their node selectors') as admit-member.sh
// install renders it, with
// the cluster's images and the site's pull account. Its Station API and cluster id are the ConfigMap station-agent's,
// which every member cluster has.
func stationDaemonSet(file, name string, want ClusterState) (*appsv1.DaemonSet, error) {
	rendered := strings.NewReplacer("REPLACE_RLARK_IMAGE", want.StationAgent, "REPLACE_PRIVATE_WIREGUARD_IMAGE", want.StationWireguard,
		"REPLACE_IMAGE_PULL_SECRET", want.StationPullSecret).Replace(file)
	for _, document := range bytes.Split([]byte(rendered), []byte("\n---")) {
		var daemonSet appsv1.DaemonSet
		if err := yaml.Unmarshal(document, &daemonSet); err != nil {
			return nil, fmt.Errorf("station-agent.yaml: %w", err)
		}
		if daemonSet.Kind == "DaemonSet" && daemonSet.Name == name {
			return &daemonSet, nil
		}
	}
	return nil, fmt.Errorf("station-agent.yaml has no DaemonSet %s", name)
}
