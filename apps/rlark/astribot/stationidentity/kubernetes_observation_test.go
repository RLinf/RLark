package stationidentity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
)

type observationNamespaceMap map[string]string

func (m observationNamespaceMap) ResolveObservationNamespace(_ context.Context, clusterID string) (string, error) {
	namespace := m[clusterID]
	if namespace == "" {
		return "", ErrClusterNotRegistered
	}
	return namespace, nil
}

func testDynamicClient(objects ...runtime.Object) *fake.FakeDynamicClient {
	return fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		rlarkClusterGVR:      "ClusterList",
		networkEnrollmentGVR: "NetworkEnrollmentList",
		nodeAdmissionGVR:     "NodeAdmissionList",
		siteNetworkGVR:       "SiteNetworkList",
	}, objects...)
}

// A node's first report makes its record; every report replaces the one
// before it in status.observation and touches nothing else of the record,
// whichever Node of the name sends it.
func TestKubernetesObservationSinkWritesOnlyTheObservationOfTheNodesRecord(t *testing.T) {
	client := testDynamicClient()
	receivedAt := time.Date(2026, 11, 11, 1, 2, 3, 4, time.FixedZone("test", 8*60*60))
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"cluster-a": "agent-a"}, Now: func() time.Time { return receivedAt }}
	evidence := validEvidence()
	evidence.ClusterID = "cluster-a"
	evidence.NodeName = "node-a"
	evidence.SystemUUID = "system-a"
	evidence.NodePressures = []string{"DiskPressure"}
	observation := Observation{
		BootTime:  &receivedAt,
		Wireguard: json.RawMessage(`{"interface":"st-wg0","addresses":["10.254.240.10/32"]}`),
		Probes:    []Probe{{Target: "nas:445", OK: true}},
		Gateway:   &GatewayProgram{DocumentLevel: 2, Digest: strings.Repeat("ab", 32)},
		Agent:     AgentProgram{Digest: strings.Repeat("cd", 32)},
	}
	resource := client.Resource(networkEnrollmentGVR).Namespace("agent-a")
	store := func(e Evidence, o Observation) map[string]interface{} {
		t.Helper()
		if err := sink.Store(context.Background(), e, o); err != nil {
			t.Fatal(err)
		}
		stored, err := resource.Get(context.Background(), "node-a", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return stored.Object
	}
	created := store(evidence, observation)
	if spec, _ := json.Marshal(created["spec"]); string(spec) != `{"clusterID":"cluster-a","nodeUID":"n","systemUUID":"system-a"}` {
		t.Fatalf("the record of a first report: %s", spec)
	}
	reported := created["status"].(map[string]interface{})["observation"].(map[string]interface{})
	if reported["reportedAt"] != receivedAt.UTC().Format(time.RFC3339Nano) || reported["nodeUID"] != "n" || reported["wireguard"].(map[string]interface{})["interface"] != "st-wg0" {
		t.Fatalf("the report: %v", reported)
	}
	programs, _ := json.Marshal(map[string]any{"gateway": reported["gateway"], "agent": reported["agent"], "pressures": reported["pressures"]})
	if want := `{"agent":{"digest":"` + strings.Repeat("cd", 32) + `"},"gateway":{"digest":"` + strings.Repeat("ab", 32) + `","documentLevel":2},"pressures":["DiskPressure"]}`; string(programs) != want {
		t.Fatalf("programs and pressures = %s", programs)
	}

	// station-controller answers and judges; the node enrolls.
	answered, _ := resource.Get(context.Background(), "node-a", metav1.GetOptions{})
	_ = unstructured.SetNestedField(answered.Object, "Enrolled", "status", "phase")
	_ = unstructured.SetNestedField(answered.Object, "Bound", "status", "node", "state")
	_ = unstructured.SetNestedField(answered.Object, "key", "spec", "publicKey")
	if _, err := resource.Update(context.Background(), answered, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	// A gateway that stopped reconciling, of a Node under no pressure, reinstalled under the same name.
	evidence.NodeUID, evidence.NodePressures = "reinstalled", []string{}
	again := store(evidence, Observation{Agent: observation.Agent})
	spec, status := again["spec"].(map[string]interface{}), again["status"].(map[string]interface{})
	if spec["nodeUID"] != "n" || spec["publicKey"] != "key" || status["phase"] != "Enrolled" || status["node"].(map[string]interface{})["state"] != "Bound" {
		t.Fatalf("a report changed what is not its own: %v", again)
	}
	reported = status["observation"].(map[string]interface{})
	_, wireguard := reported["wireguard"]
	_, bootTime := reported["bootTime"]
	_, gateway := reported["gateway"]
	if pressures, found := reported["pressures"].([]interface{}); reported["nodeUID"] != "reinstalled" || wireguard || bootTime || gateway || !found || len(pressures) != 0 {
		t.Fatalf("the latest report kept what the one before it said, or its pressures are not an empty list: %v", reported)
	}
}

func TestKubernetesClusterNamespaceResolverRequiresUniqueAuthoritativeScope(t *testing.T) {
	cluster := func(name, namespace string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "rlinf.io/v1alpha1", "kind": "Cluster",
			"metadata": map[string]interface{}{"name": name},
			"spec":     map[string]interface{}{"agentNamespace": namespace},
		}}
	}
	t.Run("resolves by cluster object name", func(t *testing.T) {
		resolver := KubernetesClusterNamespaceResolver{Client: testDynamicClient(cluster("cluster-a", "agent-a"))}
		namespace, err := resolver.ResolveObservationNamespace(context.Background(), "cluster-a")
		if err != nil || namespace != "agent-a" {
			t.Fatalf("namespace=%q err=%v", namespace, err)
		}
	})
	t.Run("rejects duplicate namespace claim", func(t *testing.T) {
		resolver := KubernetesClusterNamespaceResolver{Client: testDynamicClient(cluster("cluster-a", "agent-a"), cluster("cluster-b", "agent-a"))}
		if _, err := resolver.ResolveObservationNamespace(context.Background(), "cluster-a"); err == nil {
			t.Fatal("accepted duplicate authoritative namespace")
		}
	})
	t.Run("rejects unknown cluster", func(t *testing.T) {
		resolver := KubernetesClusterNamespaceResolver{Client: testDynamicClient(cluster("cluster-a", "agent-a"))}
		if _, err := resolver.ResolveObservationNamespace(context.Background(), "missing"); !errors.Is(err, ErrClusterNotRegistered) {
			t.Fatalf("unknown cluster error=%v", err)
		}
	})
}

// A peer is named by the member enrolled with its key, or, before it enrolls, by the admission it redeemed or the
// SiteNetwork's declaration of it: a hub that reports such a key has taken the new node's document.
func TestObservationPeersAreStoredUnderTheMemberEnrolledWithTheirKey(t *testing.T) {
	enrollment := func(namespace, cluster, name, phase, key string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NetworkEnrollment",
			"metadata": map[string]interface{}{"namespace": namespace, "name": name},
			"spec":     map[string]interface{}{"clusterID": cluster},
			"status":   map[string]interface{}{"phase": phase, "publicKey": key},
		}}
	}
	leaf, moved, stranger := "bGVhZi1rZXktbGVhZi1rZXktbGVhZi1rZXktbGVhZi0=", "bW92ZWQta2V5LW1vdmVkLWtleS1tb3ZlZC1rZXktbW8=", "c3RyYW5nZXItc3RyYW5nZXItc3RyYW5nZXItc3RyYW4="
	redeemed, declared := "cmVkZWVtZWQtcmVkZWVtZWQtcmVkZWVtZWQtcmVkZWU=", "ZGVjbGFyZWQtZGVjbGFyZWQtZGVjbGFyZWQtZGVjbGE="
	client := testDynamicClient(
		enrollment("agent-robot", "robot-01", "s1u-01-x86", "Enrolled", leaf),
		// A machine that moved leaves a rejected enrollment with its key behind.
		enrollment("agent-old", "old", "desk", "Rejected", moved),
		enrollment("agent-new", "new", "desk", "Enrolled", moved),
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
			"metadata": map[string]interface{}{"namespace": "agent-baidu", "name": "gpu-2"},
			"spec":     map[string]interface{}{"clusterID": "baidu"},
			"status":   map[string]interface{}{"publicKey": redeemed, "config": "{}"},
		}},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "SiteNetwork",
			"metadata": map[string]interface{}{"namespace": "station-system", "name": "default"},
			"spec": map[string]interface{}{"nodes": []interface{}{
				map[string]interface{}{"clusterID": "baidu", "nodeName": "gpu-1", "publicKey": declared},
				map[string]interface{}{"clusterID": "baidu", "nodeName": "gpu-9"},
			}},
		}},
	)
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"cluster-a": "agent-a"}, Enrolled: EnrolledKeys(client), Admitted: AdmittedKeys(client), Declared: &DeclaredKeys{Client: client}}
	e := validEvidence()
	e.ClusterID = "cluster-a"
	e.NodeName = "hub"
	store := func(wireguard string) []interface{} {
		t.Helper()
		if err := sink.Store(context.Background(), e, Observation{Wireguard: json.RawMessage(wireguard)}); err != nil {
			t.Fatal(err)
		}
		stored, err := client.Resource(networkEnrollmentGVR).Namespace("agent-a").Get(context.Background(), "hub", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		peers, _, _ := unstructured.NestedSlice(stored.Object, "status", "observation", "wireguard", "peers")
		return peers
	}
	peers := store(`{"interface":"st-wg0","peers":[
		{"publicKey":"` + stranger + `","receivedBytes":7},
		{"publicKey":"` + leaf + `","latestHandshake":"2026-11-14T01:02:03Z","receivedBytes":148,"sentBytes":92},
		{"publicKey":"` + moved + `"},
		{"publicKey":"` + redeemed + `"},
		{"publicKey":"` + declared + `","latestHandshake":"2026-11-14T01:02:04Z"}]}`)
	want := []interface{}{
		map[string]interface{}{"name": "robot-01/s1u-01-x86", "latestHandshake": "2026-11-14T01:02:03Z", "receivedBytes": int64(148), "sentBytes": int64(92)},
		map[string]interface{}{"name": "new/desk"},
		map[string]interface{}{"name": "baidu/gpu-2"},
		map[string]interface{}{"name": "baidu/gpu-1", "latestHandshake": "2026-11-14T01:02:04Z"},
	}
	if got, _ := json.Marshal(peers); len(peers) != 4 || string(got) != string(mustJSON(t, want)) {
		t.Fatalf("peers = %s", got)
	}
	if peers := store(`{"interface":"st-wg0"}`); len(peers) != 0 {
		t.Fatalf("a report without peers kept %v", peers)
	}
}

// Reports arrive from every node every 30 s; the enrollments are listed for
// all of them together, and sooner only for a key that is new.
func TestObservationSinkListsEnrollmentsOncePerInterval(t *testing.T) {
	enrollment := func(name, key string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NetworkEnrollment",
			"metadata": map[string]interface{}{"namespace": "agent-robot", "name": name},
			"spec":     map[string]interface{}{"clusterID": "robot-01"},
			"status":   map[string]interface{}{"phase": "Enrolled", "publicKey": key},
		}}
	}
	first, second := "bGVhZi1rZXktbGVhZi1rZXktbGVhZi1rZXktbGVhZi0=", "bW92ZWQta2V5LW1vdmVkLWtleS1tb3ZlZC1rZXktbW8="
	client := testDynamicClient(enrollment("first", first))
	clock := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"cluster-a": "agent-a"}, Now: func() time.Time { return clock }, Enrolled: EnrolledKeys(client), Admitted: AdmittedKeys(client), Declared: &DeclaredKeys{Client: client}}
	sink.Enrolled.Now, sink.Admitted.Now, sink.Declared.Now = sink.Now, sink.Now, sink.Now
	e := validEvidence()
	e.ClusterID = "cluster-a"
	e.NodeName = "hub"
	// lists is how often the enrollments have been listed, names what is stored.
	report := func(after time.Duration, keys ...string) (lists int, names string) {
		t.Helper()
		clock = clock.Add(after)
		peers := make([]string, len(keys))
		for i, key := range keys {
			peers[i] = `{"publicKey":"` + key + `"}`
		}
		if err := sink.Store(context.Background(), e, Observation{Wireguard: json.RawMessage(`{"interface":"st-wg0","peers":[` + strings.Join(peers, ",") + `]}`)}); err != nil {
			t.Fatal(err)
		}
		for _, action := range client.Actions() {
			if action.Matches("list", "networkenrollments") {
				lists++
			}
		}
		stored, err := client.Resource(networkEnrollmentGVR).Namespace("agent-a").Get(context.Background(), "hub", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		entries, _, _ := unstructured.NestedSlice(stored.Object, "status", "observation", "wireguard", "peers")
		for _, entry := range entries {
			names += entry.(map[string]interface{})["name"].(string) + " "
		}
		return lists, names
	}
	report(0, first)
	if lists, names := report(time.Second, first); lists != 1 || names != "robot-01/first " {
		t.Fatalf("first reports: %d lists, %q", lists, names)
	}
	if _, err := client.Resource(networkEnrollmentGVR).Namespace("agent-robot").Create(context.Background(), enrollment("second", second), metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	if lists, names := report(time.Second, first, second); lists != 1 || names != "robot-01/first " {
		t.Fatalf("a new key within five seconds: %d lists, %q", lists, names)
	}
	if lists, names := report(4*time.Second, first, second); lists != 2 || names != "robot-01/first robot-01/second " {
		t.Fatalf("a new key after five seconds: %d lists, %q", lists, names)
	}
	if lists, _ := report(29*time.Second, first, second); lists != 2 {
		t.Fatalf("known keys within thirty seconds: %d lists", lists)
	}
	if lists, _ := report(2*time.Second, first, second); lists != 3 {
		t.Fatalf("after thirty seconds: %d lists", lists)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
