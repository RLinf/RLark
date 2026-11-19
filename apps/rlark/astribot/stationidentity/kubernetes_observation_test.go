package stationidentity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
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
		stationObservationGVR: "StationObservationList",
		stationGVR:            "StationList",
		rlarkClusterGVR:       "ClusterList",
		networkEnrollmentGVR:  "NetworkEnrollmentList",
		nodeAdmissionGVR:      "NodeAdmissionList",
		siteNetworkGVR:        "SiteNetworkList",
	}, objects...)
}

func TestKubernetesObservationSinkCreatesAndRefreshesOwnedSnapshot(t *testing.T) {
	client := testDynamicClient()
	emptyBeforeStatusPatch := false
	client.PrependReactor("patch", stationObservationResource, func(ktesting.Action) (bool, runtime.Object, error) {
		obj, err := client.Tracker().Get(stationObservationGVR, "agent-a", "node-a")
		if err != nil {
			return true, nil, err
		}
		unstructuredObject := obj.(*unstructured.Unstructured)
		_, found, _ := unstructured.NestedMap(unstructuredObject.Object, "status")
		if !found {
			emptyBeforeStatusPatch = true
		}
		return false, nil, nil
	})
	receivedAt := time.Date(2026, 11, 11, 1, 2, 3, 4, time.FixedZone("test", 8*60*60))
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"cluster-a": "agent-a"}, Now: func() time.Time { return receivedAt }}
	evidence := validEvidence()
	evidence.ClusterID = "cluster-a"
	evidence.RegisteredClusterID = "cluster-a"
	evidence.NodeName = "node-a"
	evidence.PodNodeName = "node-a"
	evidence.StationUID = "station-uid-a"
	evidence.RegisteredStationUID = "station-uid-a"
	observation := Observation{
		SystemUUID: "system-a",
		BootTime:   &receivedAt,
		Roles:      []string{"worker"},
		Wireguard:  json.RawMessage(`{"interface":"st-wg0","addresses":["10.254.240.10/32"]}`),
		Probes:     []Probe{{Target: "nas:445", OK: true}},
		Gateway:    &GatewayProgram{DocumentLevel: 2, Digest: strings.Repeat("ab", 32)},
		Agent:      AgentProgram{Digest: strings.Repeat("cd", 32)},
	}
	evidence.NodePressures = []string{"DiskPressure"}
	if err := sink.Store(context.Background(), evidence, observation); err != nil {
		t.Fatal(err)
	}
	resource := client.Resource(stationObservationGVR).Namespace("agent-a")
	created, err := resource.Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := created.GetAnnotations()[observationClusterIDKey]; got != "cluster-a" {
		t.Fatalf("cluster ownership annotation = %q", got)
	}
	if got := created.GetAnnotations()[observationStationUIDKey]; got != "station-uid-a" {
		t.Fatalf("Station ownership annotation = %q", got)
	}
	if err := sink.Store(context.Background(), evidence, observation); err != nil {
		t.Fatal(err)
	}
	if !emptyBeforeStatusPatch {
		t.Fatal("new observation was not created empty before status write")
	}
	stored, err := resource.Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	reportedAt, _, _ := unstructured.NestedString(stored.Object, "status", "reportedAt")
	if reportedAt != receivedAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("reportedAt = %q", reportedAt)
	}
	if got, _, _ := unstructured.NestedString(stored.Object, "status", "nodeUID"); got != evidence.NodeUID {
		t.Fatalf("nodeUID = %q", got)
	}
	if got, _, _ := unstructured.NestedString(stored.Object, "status", "wireguard", "interface"); got != "st-wg0" {
		t.Fatalf("wireguard interface = %q", got)
	}
	programs, _ := json.Marshal(map[string]any{"gateway": stored.Object["status"].(map[string]any)["gateway"], "agent": stored.Object["status"].(map[string]any)["agent"], "pressures": stored.Object["status"].(map[string]any)["pressures"]})
	if want := `{"agent":{"digest":"` + strings.Repeat("cd", 32) + `"},"gateway":{"digest":"` + strings.Repeat("ab", 32) + `","documentLevel":2},"pressures":["DiskPressure"]}`; string(programs) != want {
		t.Fatalf("programs and pressures = %s", programs)
	}
	// A gateway that stopped reconciling, of a Node under no pressure.
	evidence.NodePressures = []string{}
	if err := sink.Store(context.Background(), evidence, Observation{Agent: observation.Agent}); err != nil {
		t.Fatal(err)
	}
	stored, err = resource.Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(stored.Object, "status", "wireguard"); found {
		t.Fatal("empty latest snapshot retained old WireGuard data")
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(stored.Object, "status", "bootTime"); found {
		t.Fatal("empty latest snapshot retained old boot time")
	}
	_, gateway, _ := unstructured.NestedFieldNoCopy(stored.Object, "status", "gateway")
	if pressures, found, _ := unstructured.NestedSlice(stored.Object, "status", "pressures"); gateway || !found || len(pressures) != 0 {
		t.Fatalf("a report without a gateway kept the old one, or its pressures are not an empty list: %v", stored.Object["status"])
	}
}

func TestObservationBindingVersionReplacesPreviousSnapshot(t *testing.T) {
	client := testDynamicClient()
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"cluster-a": "agent-a"}}
	e := validEvidence()
	e.ClusterID, e.RegisteredClusterID = "cluster-a", "cluster-a"
	e.NodeName, e.PodNodeName = "node-a", "node-a"
	e.StationUID, e.RegisteredStationUID = "station-a", "station-a"
	wg := Observation{Wireguard: json.RawMessage(`{"interface":"st-wg0","addresses":["10.254.240.1/32"]}`)}
	if err := sink.Store(context.Background(), e, wg); err != nil {
		t.Fatal(err)
	}
	e.Generation++
	e.RegisteredGeneration = e.Generation
	if err := sink.Store(context.Background(), e, Observation{}); err != nil {
		t.Fatal(err)
	}
	got, err := client.Resource(stationObservationGVR).Namespace("agent-a").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetAnnotations()[observationBindingVersionKey] != "2" {
		t.Fatalf("binding version = %q", got.GetAnnotations()[observationBindingVersionKey])
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(got.Object, "status", "wireguard"); found {
		t.Fatal("old WireGuard observation survived new binding")
	}
}

func TestKubernetesObservationSinkTransfersObservationAfterVerifiedStationRebind(t *testing.T) {
	previous := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "StationObservation",
		"metadata": map[string]interface{}{
			"name": "node-a", "namespace": "agent-a", "resourceVersion": "1",
			"annotations": map[string]interface{}{
				observationClusterIDKey: "cluster-a", observationNodeUIDKey: "old-node-uid",
				observationStationUIDKey: "station-uid-a", observationBindingVersionKey: "1",
			},
		},
		"spec": map[string]interface{}{},
		"status": map[string]interface{}{
			"reportedAt": "2026-11-12T10:00:00Z", "nodeUID": "old-node-uid",
			"wireguard": map[string]interface{}{"interface": "st-wg0"},
			"agent":     map[string]interface{}{"digest": "old"},
		},
	}}
	client := testDynamicClient(previous)
	client.PrependReactor("patch", stationObservationResource, func(action ktesting.Action) (bool, runtime.Object, error) {
		patchAction, ok := action.(ktesting.PatchAction)
		if !ok || patchAction.GetSubresource() != "status" {
			return false, nil, nil
		}
		var body struct {
			Status map[string]interface{} `json:"status"`
		}
		if err := json.Unmarshal(patchAction.GetPatch(), &body); err != nil {
			return true, nil, err
		}
		// Mirror the live CRD's required status fields.
		for _, field := range []string{"nodeUID", "reportedAt", "agent"} {
			if value, present := body.Status[field]; present && value == nil {
				return true, nil, errors.New("StationObservation required status field cannot be cleared")
			}
		}
		return false, nil, nil
	})
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"cluster-a": "agent-a"}}
	evidence := validEvidence()
	evidence.ClusterID, evidence.RegisteredClusterID = "cluster-a", "cluster-a"
	evidence.NodeName, evidence.PodNodeName = "node-a", "node-a"
	evidence.NodeUID = "new-node-uid"
	evidence.RegisteredNodeUID = evidence.NodeUID
	evidence.StationUID, evidence.RegisteredStationUID = "station-uid-a", "station-uid-a"
	evidence.Generation, evidence.RegisteredGeneration = 2, 2
	if err := sink.Store(context.Background(), evidence, Observation{Roles: []string{"worker"}}); err != nil {
		t.Fatalf("store after verified rebind: %v", err)
	}
	stored, err := client.Resource(stationObservationGVR).Namespace("agent-a").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if stored.GetAnnotations()[observationNodeUIDKey] != "new-node-uid" || stored.GetAnnotations()[observationBindingVersionKey] != "2" {
		t.Fatalf("ownership annotations did not follow rebind: %#v", stored.GetAnnotations())
	}
	if got, _, _ := unstructured.NestedString(stored.Object, "status", "nodeUID"); got != "new-node-uid" {
		t.Fatalf("observation status node UID = %q", got)
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(stored.Object, "status", "wireguard"); found {
		t.Fatal("old Node WireGuard data survived identity transfer")
	}
}

func TestKubernetesObservationSinkRejectsNodeUIDChangeWithoutStationRebind(t *testing.T) {
	previous := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "StationObservation",
		"metadata": map[string]interface{}{"name": "node", "namespace": "agent", "annotations": map[string]interface{}{
			observationClusterIDKey: "cluster", observationNodeUIDKey: "old", observationStationUIDKey: "station", observationBindingVersionKey: "1",
		}}, "spec": map[string]interface{}{},
	}}
	sink := &KubernetesObservationSink{Client: testDynamicClient(previous), Namespaces: observationNamespaceMap{"cluster": "agent"}}
	evidence := validEvidence()
	evidence.ClusterID, evidence.RegisteredClusterID = "cluster", "cluster"
	evidence.NodeUID, evidence.RegisteredNodeUID = "new", "new"
	evidence.StationUID, evidence.RegisteredStationUID = "station", "station"
	evidence.Generation, evidence.RegisteredGeneration = 1, 1
	if err := sink.Store(context.Background(), evidence, Observation{}); !errors.Is(err, ErrObservationOwnershipConflict) {
		t.Fatalf("unapproved Node UID change error = %v", err)
	}
}

func TestKubernetesObservationSinkRejectsForeignOwnershipAndUnverifiedEvidence(t *testing.T) {
	foreign := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "StationObservation",
		"metadata": map[string]interface{}{
			"name": "node", "namespace": "agent",
			"annotations": map[string]interface{}{observationClusterIDKey: "other", observationStationUIDKey: "station"},
		},
		"spec": map[string]interface{}{},
	}}
	client := testDynamicClient(foreign)
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"c": "agent"}}
	evidence := validEvidence()
	if err := sink.Store(context.Background(), evidence, Observation{}); !errors.Is(err, ErrObservationOwnershipConflict) {
		t.Fatalf("foreign observation ownership error = %v", err)
	}
	evidence.TokenPodUID = "replaced"
	if err := sink.Store(context.Background(), evidence, Observation{}); err == nil {
		t.Fatal("unverified evidence was accepted")
	}
}

func TestKubernetesObservationSinkSupportsUnboundBootstrapAndBindsOnce(t *testing.T) {
	client := testDynamicClient()
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"c": "agent"}}
	evidence := validEvidence()
	evidence.StationUID = ""
	evidence.RegisteredStationUID = ""
	evidence.Generation = 0
	evidence.RegisteredGeneration = 0
	if err := sink.Store(context.Background(), evidence, Observation{}); err != nil {
		t.Fatalf("bootstrap observation rejected: %v", err)
	}
	resource := client.Resource(stationObservationGVR).Namespace("agent")
	current, err := resource.Get(context.Background(), "node", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if current.GetAnnotations()[observationNodeUIDKey] != evidence.NodeUID || current.GetAnnotations()[observationStationUIDKey] != "" {
		t.Fatalf("bootstrap ownership annotations = %#v", current.GetAnnotations())
	}
	evidence.StationUID = "station-object-uid"
	evidence.RegisteredStationUID = evidence.StationUID
	evidence.Generation = 1
	evidence.RegisteredGeneration = 1
	if err := sink.Store(context.Background(), evidence, Observation{}); err != nil {
		t.Fatalf("Station binding did not adopt its verified Observation: %v", err)
	}
	current, err = resource.Get(context.Background(), "node", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if current.GetAnnotations()[observationStationUIDKey] != evidence.StationUID {
		t.Fatalf("Station ownership annotation = %#v", current.GetAnnotations())
	}
	evidence.StationUID = "replacement-station-uid"
	evidence.RegisteredStationUID = evidence.StationUID
	if err := sink.Store(context.Background(), evidence, Observation{}); !errors.Is(err, ErrObservationOwnershipConflict) {
		t.Fatalf("replaced Station identity error = %v", err)
	}
}

func TestKubernetesObservationSinkRetriesCreateAndStatusConflicts(t *testing.T) {
	client := testDynamicClient()
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"c": "agent"}}
	evidence := validEvidence()
	created := false
	client.PrependReactor("create", stationObservationResource, func(action ktesting.Action) (bool, runtime.Object, error) {
		if created {
			return false, nil, nil
		}
		created = true
		obj := action.(ktesting.CreateAction).GetObject().(*unstructured.Unstructured)
		if err := client.Tracker().Create(stationObservationGVR, obj, "agent"); err != nil {
			return true, nil, err
		}
		return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: stationObservationGroup, Resource: stationObservationResource}, "node")
	})
	conflicted := false
	client.PrependReactor("patch", stationObservationResource, func(action ktesting.Action) (bool, runtime.Object, error) {
		if conflicted {
			return false, nil, nil
		}
		conflicted = true
		return true, nil, apierrors.NewConflict(schema.GroupResource{Group: stationObservationGroup, Resource: stationObservationResource}, "node", errors.New("test conflict"))
	})
	if err := sink.Store(context.Background(), evidence, Observation{Roles: []string{"worker"}}); err != nil {
		t.Fatal(err)
	}
	if !created || !conflicted {
		t.Fatalf("create race=%t patch conflict=%t", created, conflicted)
	}
	stored, err := client.Resource(stationObservationGVR).Namespace("agent").Get(context.Background(), "node", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	roles, found, err := unstructured.NestedStringSlice(stored.Object, "status", "roles")
	if err != nil || !found || len(roles) != 1 || roles[0] != "worker" {
		t.Fatalf("status after retry roles=%v found=%t err=%v", roles, found, err)
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

func TestKubernetesObservationSinkPreservesExistingObjectUID(t *testing.T) {
	existing := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "StationObservation",
		"metadata": map[string]interface{}{
			"name": "node", "namespace": "agent", "uid": "object-uid", "resourceVersion": "1",
			"annotations": map[string]interface{}{observationClusterIDKey: "c", observationNodeUIDKey: "n", observationStationUIDKey: "s"},
		},
		"spec": map[string]interface{}{},
	}}
	client := testDynamicClient(existing)
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"c": "agent"}}
	if err := sink.Store(context.Background(), validEvidence(), Observation{}); err != nil {
		t.Fatal(err)
	}
	stored, err := client.Resource(stationObservationGVR).Namespace("agent").Get(context.Background(), "node", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if types.UID(stored.GetUID()) != "object-uid" {
		t.Fatalf("observation UID changed to %q", stored.GetUID())
	}
}

// A machine reinstalled under the same name has a new Node UID. It must be
// able to report, or the Station never shows the conflict an administrator
// rebinds; it gets a fresh snapshot and nothing of the previous Node's.
func TestReinstalledNodeReplacesThePreviousNodesSnapshot(t *testing.T) {
	client := testDynamicClient()
	sink := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"c": "agent"}}
	old := validEvidence()
	boot := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	if err := sink.Store(context.Background(), old, Observation{BootTime: &boot, Wireguard: json.RawMessage(`{"interface":"st-wg0","addresses":["10.254.240.5/32"]}`)}); err != nil {
		t.Fatal(err)
	}
	rebuilt := validEvidence()
	rebuilt.NodeUID, rebuilt.RegisteredNodeUID = "n-rebuilt", "n-rebuilt"
	rebuilt.StationUID, rebuilt.RegisteredStationUID, rebuilt.Generation, rebuilt.RegisteredGeneration = "", "", 0, 0
	if err := sink.Store(context.Background(), rebuilt, Observation{}); err != nil {
		t.Fatalf("reinstalled node could not report: %v", err)
	}
	stored, err := client.Resource(stationObservationGVR).Namespace("agent").Get(context.Background(), "node", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	annotations := stored.GetAnnotations()
	uid, _, _ := unstructured.NestedString(stored.Object, "status", "nodeUID")
	_, hasTunnel, _ := unstructured.NestedMap(stored.Object, "status", "wireguard")
	if uid != "n-rebuilt" || annotations[observationNodeUIDKey] != "n-rebuilt" || annotations[observationStationUIDKey] != "" || hasTunnel {
		t.Fatalf("snapshot still carries the previous Node: %v %v", annotations, stored.Object["status"])
	}
	// Another cluster's node of the same name still cannot take the object.
	foreign := validEvidence()
	foreign.ClusterID, foreign.RegisteredClusterID = "d", "d"
	other := &KubernetesObservationSink{Client: client, Namespaces: observationNamespaceMap{"d": "agent"}}
	if err := other.Store(context.Background(), foreign, Observation{}); !errors.Is(err, ErrObservationOwnershipConflict) {
		t.Fatalf("foreign cluster error = %v", err)
	}
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
	e.ClusterID, e.RegisteredClusterID = "cluster-a", "cluster-a"
	e.NodeName, e.PodNodeName = "hub", "hub"
	store := func(wireguard string) []interface{} {
		t.Helper()
		// The first write of a node only creates its object.
		for range 2 {
			if err := sink.Store(context.Background(), e, Observation{Wireguard: json.RawMessage(wireguard)}); err != nil {
				t.Fatal(err)
			}
		}
		stored, err := client.Resource(stationObservationGVR).Namespace("agent-a").Get(context.Background(), "hub", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		peers, _, _ := unstructured.NestedSlice(stored.Object, "status", "wireguard", "peers")
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
	e.ClusterID, e.RegisteredClusterID = "cluster-a", "cluster-a"
	e.NodeName, e.PodNodeName = "hub", "hub"
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
		stored, err := client.Resource(stationObservationGVR).Namespace("agent-a").Get(context.Background(), "hub", metav1.GetOptions{})
		if err != nil {
			t.Fatal(err)
		}
		entries, _, _ := unstructured.NestedSlice(stored.Object, "status", "wireguard", "peers")
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
