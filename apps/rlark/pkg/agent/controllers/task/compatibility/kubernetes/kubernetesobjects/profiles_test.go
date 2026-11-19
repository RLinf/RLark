package kubernetesobjects

import (
	"testing"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

func TestServiceProjectionEqualIgnoresAPIServerMaterializedFields(t *testing.T) {
	current := agentruntime.TypedManifest{Object: []byte(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"head","namespace":"runtime","labels":{"app":"task"}},"spec":{"clusterIP":"10.0.0.1","clusterIPs":["10.0.0.1"],"ipFamilies":["IPv4"],"ipFamilyPolicy":"SingleStack","internalTrafficPolicy":"Cluster","sessionAffinity":"None","type":"ClusterIP","selector":{"app":"task"},"ports":[{"name":"client","protocol":"TCP","port":10001,"targetPort":10001,"nodePort":30001}]}}`)}
	desired := agentruntime.TypedManifest{Object: []byte(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"head","namespace":"runtime","labels":{"app":"task"}},"spec":{"selector":{"app":"task"},"ports":[{"name":"client","port":10001}]}}`)}
	equal, err := ServiceProjectionEqual(current, desired)
	if err != nil {
		t.Fatal(err)
	}
	if !equal {
		t.Fatal("API Server defaults and allocations must not cause Service drift")
	}
}

func TestServiceProjectionEqualDetectsOwnedFieldDrift(t *testing.T) {
	current := agentruntime.TypedManifest{Object: []byte(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"head","namespace":"runtime"},"spec":{"selector":{"app":"old"},"ports":[{"port":10001}]}}`)}
	desired := agentruntime.TypedManifest{Object: []byte(`{"apiVersion":"v1","kind":"Service","metadata":{"name":"head","namespace":"runtime"},"spec":{"selector":{"app":"task"},"ports":[{"port":10001}]}}`)}
	equal, err := ServiceProjectionEqual(current, desired)
	if err != nil {
		t.Fatal(err)
	}
	if equal {
		t.Fatal("owned selector drift was ignored")
	}
}
