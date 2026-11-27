package release

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestReadSnapshot(t *testing.T) {
	object := func(apiVersion, kind, namespace, name string, status map[string]any) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": apiVersion, "kind": kind,
			"metadata": map[string]any{"namespace": namespace, "name": name},
			"status":   status,
		}}
	}
	cluster := object("rlinf.io/v1alpha1", "Cluster", "", "office", map[string]any{"phase": "Downgraded"})
	node := object("rlinf.io/v1alpha1", "Node", "rlark-office", "s1u-01", map[string]any{"phase": "Online"})
	cluster.Object["spec"] = map[string]any{"agentNamespace": "rlark-office"}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		clusterGVR: "ClusterList", nodeGVR: "NodeList", addonGVR: "AddonList",
		siteNetworkGVR: "SiteNetworkList", enrollmentGVR: "NetworkEnrollmentList",
	},
		cluster,
		node,
		object("rlinf.io/v1alpha1", "Addon", "rlark-office", "station", map[string]any{"phase": "Ready", "version": "0.4.0"}),
		// An object of a namespace no cluster has is nobody's.
		object("rlinf.io/v1alpha1", "Addon", "elsewhere", "station", map[string]any{"phase": "Failed"}),
		object("station.rlark.io/v1alpha1", "SiteNetwork", "station-system", "default", map[string]any{
			"members":     int64(2),
			"unreachable": []any{"b to a", "a to b"},
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
				map[string]any{"type": "FullyConnected", "status": "False"},
			},
		}),
		object("station.rlark.io/v1alpha1", "NetworkEnrollment", "rlark-office", "s1u-01", map[string]any{
			"phase": "Enrolled", "configHash": "abc", "relay": "cloud/gpu-1",
			"problems": []any{"z", "a"},
			"links":    []any{map[string]any{"hub": "cloud/gpu-1", "state": "Connected", "since": "2026-11-13T19:00:00Z"}},
			"observation": map[string]any{
				"reportedAt": "2026-11-13T19:59:00Z", "nodeUID": "u1",
				"gateway": map[string]any{"documentLevel": int64(2), "digest": "aa11"}, "agent": map[string]any{"digest": "bb22"},
				"pressures": []any{"PIDPressure", "DiskPressure"},
			},
		}),
		// Nodes in no tunnel network: one under no pressure, with a gateway and an agent that state nothing; one whose
		// report states no pressures; one that never reported.
		object("station.rlark.io/v1alpha1", "NetworkEnrollment", "rlark-office", "desk-1", map[string]any{"observation": map[string]any{"reportedAt": "2026-11-13T19:59:30Z", "nodeUID": "u3", "pressures": []any{}}}),
		object("station.rlark.io/v1alpha1", "NetworkEnrollment", "rlark-office", "desk-2", map[string]any{"observation": map[string]any{"reportedAt": "2026-11-13T19:57:59Z", "nodeUID": "u2"}}),
		object("station.rlark.io/v1alpha1", "NetworkEnrollment", "rlark-office", "desk-3", map[string]any{"phase": "Pending"}),
	)

	snapshot, err := Read(context.Background(), client, taken.Add(300*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(snapshot)
	want := `{"takenAt":"2026-11-13T20:00:00Z",` +
		`"siteNetwork":{"ready":true,"fullyConnected":false,"members":2,"unreachable":["a to b","b to a"]},` +
		`"clusters":{"office":"Downgraded"},` +
		`"nodes":{"office/s1u-01":"Online"},` +
		`"enrollments":{"office/desk-1":{"phase":""},"office/desk-2":{"phase":""},"office/desk-3":{"phase":"Pending"},"office/s1u-01":{"phase":"Enrolled","configHash":"abc","problems":["a","z"],"links":{"cloud/gpu-1":"Connected"}}},` +
		`"addons":{"office/station":{"phase":"Ready","version":"0.4.0"}},` +
		`"observations":{"office/desk-1":{"reportedAt":"2026-11-13T19:59:30Z","fresh":true,"pressures":[]},` +
		`"office/desk-2":{"reportedAt":"2026-11-13T19:57:59Z","fresh":false,"pressures":null},` +
		`"office/s1u-01":{"reportedAt":"2026-11-13T19:59:00Z","fresh":true,"gatewayLevel":2,"pressures":["DiskPressure","PIDPressure"]}}}`
	var again Snapshot
	if err := json.Unmarshal(got, &again); err != nil || again.Observations["office/desk-1"].Pressures == nil || again.Observations["office/desk-2"].Pressures != nil {
		t.Fatalf("a baseline read back does not tell no pressure from no reading: %v %+v", err, again.Observations)
	}
	if string(got) != want {
		t.Fatalf("snapshot\n got %s\nwant %s", got, want)
	}
}
