package kubernetes

import (
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestObjectComparatorUsesServiceCompatibilityProjection(t *testing.T) {
	current := agentruntime.TypedManifest{Object: []byte(`{"metadata":{"labels":{"owned":"value","external":"kept"}},"spec":{"clusterIP":"10.0.0.1","clusterIPs":["10.0.0.1"],"ports":[{"port":80,"protocol":"TCP","targetPort":80}],"selector":{"app":"task"},"type":"ClusterIP"}}`)}
	desiredManifest := agentruntime.TypedManifest{Object: []byte(`{"metadata":{"labels":{"owned":"value"}},"spec":{"ports":[{"port":80}],"selector":{"app":"task"}}}`)}
	equal, err := (ObjectComparator{}).Equal(current, agentruntime.DesiredObject{Manifest: desiredManifest, ReconcileProfile: kubernetesobjects.ServiceReconcileProfile})
	if err != nil {
		t.Fatal(err)
	}
	if !equal {
		t.Fatal("API-server materialized Service fields caused false drift")
	}
}

func TestObjectComparatorDefaultsToJSONSemanticEquality(t *testing.T) {
	equal, err := (ObjectComparator{}).Equal(agentruntime.TypedManifest{Object: []byte(`{"a":1,"b":2}`)}, agentruntime.DesiredObject{Manifest: agentruntime.TypedManifest{Object: []byte(`{"b":2,"a":1}`)}})
	if err != nil || !equal {
		t.Fatalf("equal=%v error=%v", equal, err)
	}
}
