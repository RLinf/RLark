package controller

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	controllerfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const nodeWGUID = "12345678-1234-1234-1234-123456789abc"

type nodeWireGuardEvidenceStub struct{ evidence NodeWireGuardEvidence }

func (s nodeWireGuardEvidenceStub) Evidence(context.Context, string, string, string) (NodeWireGuardEvidence, error) {
	return s.evidence, nil
}

type nodeOperationClusterListerStub []string

func (s nodeOperationClusterListerStub) ClusterIDs(context.Context) ([]string, error) {
	return append([]string(nil), s...), nil
}

type nodeMemberClients map[string]kubernetes.Interface

func (s nodeMemberClients) Client(_ context.Context, cluster string) (kubernetes.Interface, error) {
	client, exists := s[cluster]
	if !exists {
		return nil, errors.New("member cluster is unavailable")
	}
	return client, nil
}

func TestConfigMapNodeWireGuardPeerCleanerRemovesExactUIDAndPreservesForeignPeers(t *testing.T) {
	ctx := context.Background()
	target := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a", UID: nodeWGUID, Labels: map[string]string{stationNetworkLabel: "enabled"}},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
	networkPod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "station-agent-worker-a", Namespace: stationWireGuardNamespace, Labels: map[string]string{"app": "station-agent-network"}}, Spec: corev1.PodSpec{NodeName: "worker-a"}}
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: stationWireGuardConfigMap, Namespace: stationWireGuardNamespace, ResourceVersion: "1"}, Data: map[string]string{
		"worker-a.json": `{"interface":"st-wg0","nodeUID":"` + nodeWGUID + `","publicKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","address":"10.254.240.9/32","listenPort":51820,"mtu":1380,"peers":[],"routes":[]}`,
		"hub.json":      `{"interface":"st-wg0","nodeUID":"hub-uid","address":"10.254.240.1/32","listenPort":51820,"mtu":1380,"peers":[{"publicKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","nodeUID":"` + nodeWGUID + `","allowedIPs":["10.254.240.9/32","172.29.0.0/24"]},{"publicKey":"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=","nodeUID":"other-uid","allowedIPs":["10.254.240.10/32","172.29.1.0/24"]}],"routes":["10.254.240.9/32","172.29.0.0/24","10.254.240.10/32","172.29.1.0/24"]}`,
	}}
	member := fake.NewSimpleClientset(target, configMap, networkPod)
	cleaner := ConfigMapNodeWireGuardPeerCleaner{Members: &memberClientStub{client: member}, Clusters: nodeOperationClusterListerStub{"cluster-a"}}
	op := &stationv1alpha1.NodeOperation{Status: stationv1alpha1.NodeOperationStatus{Stage: "Drain"}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "worker-a", NodeUID: nodeWGUID}}
	if err := cleaner.Check(ctx, op); err != nil {
		t.Fatalf("preflight ownership check: %v", err)
	}
	if done, err := cleaner.Remove(ctx, op); err != nil || done {
		t.Fatalf("remove: done=%v err=%v", done, err)
	}
	updatedNode, err := member.CoreV1().Nodes().Get(ctx, target.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, enabled := updatedNode.Labels[stationNetworkLabel]; enabled {
		t.Fatal("network DaemonSet label remains enabled")
	}
	updated, err := member.CoreV1().ConfigMaps(stationWireGuardNamespace).Get(ctx, stationWireGuardConfigMap, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := updated.Data["worker-a.json"]; exists {
		t.Fatal("target node config remains")
	}
	var hub map[string]json.RawMessage
	if err := json.Unmarshal([]byte(updated.Data["hub.json"]), &hub); err != nil {
		t.Fatal(err)
	}
	var peers []map[string]json.RawMessage
	if err := json.Unmarshal(hub["peers"], &peers); err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 {
		t.Fatalf("remaining peers=%d, want only foreign peer", len(peers))
	}
	var remaining map[string]any
	if err := json.Unmarshal(mustMarshal(t, peers[0]), &remaining); err != nil {
		t.Fatal(err)
	}
	if remaining["nodeUID"] != "other-uid" {
		t.Fatalf("foreign peer changed: %#v", remaining)
	}
	var routes []string
	if err := json.Unmarshal(hub["routes"], &routes); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(routes, ",")
	for _, removed := range []string{"10.254.240.9/32", "172.29.0.0/24"} {
		if strings.Contains(joined, removed) {
			t.Fatalf("target route %s remains in %v", removed, routes)
		}
	}
	for _, preserved := range []string{"10.254.240.10/32", "172.29.1.0/24"} {
		if !strings.Contains(joined, preserved) {
			t.Fatalf("foreign route %s was removed from %v", preserved, routes)
		}
	}
	if err := member.CoreV1().Pods(stationWireGuardNamespace).Delete(ctx, networkPod.Name, metav1.DeleteOptions{}); err != nil {
		t.Fatal(err)
	}
	if done, err := cleaner.Remove(ctx, op); err != nil || !done {
		t.Fatalf("idempotent retry: done=%v err=%v", done, err)
	}
}

func TestConfigMapNodeWireGuardPeerCleanerBlocksUnownedOrChangedIdentity(t *testing.T) {
	ctx := context.Background()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a", UID: nodeWGUID, Labels: map[string]string{stationNetworkLabel: "enabled"}}}
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: stationWireGuardConfigMap, Namespace: stationWireGuardNamespace}, Data: map[string]string{
		"worker-a.json": `{"interface":"st-wg0","address":"10.254.240.9/32","listenPort":51820,"mtu":1380,"peers":[],"routes":[]}`,
	}}
	member := fake.NewSimpleClientset(node, configMap)
	cleaner := ConfigMapNodeWireGuardPeerCleaner{Members: &memberClientStub{client: member}, Clusters: nodeOperationClusterListerStub{"cluster-a"}}
	op := &stationv1alpha1.NodeOperation{Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "worker-a", NodeUID: nodeWGUID}}
	if err := cleaner.Check(ctx, op); err == nil {
		t.Fatal("unowned legacy config passed removal preflight")
	}
	op.Spec.NodeUID = "different-node-uid"
	if _, err := cleaner.Remove(ctx, op); err == nil {
		t.Fatal("peer cleanup accepted a changed Node UID")
	}
}

func TestConfigMapNodeWireGuardPeerCleanerUsesVerifiedObservationForLegacyConfig(t *testing.T) {
	ctx := context.Background()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a", UID: nodeWGUID, Labels: map[string]string{stationNetworkLabel: "enabled"}}}
	configMap := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: stationWireGuardConfigMap, Namespace: stationWireGuardNamespace}, Data: map[string]string{
		"worker-a.json": `{"interface":"st-wg0","address":"10.254.240.9/32","listenPort":51820,"mtu":1380,"peers":[],"routes":[]}`,
		"hub.json":      `{"interface":"st-wg0","address":"10.254.240.1/32","listenPort":51820,"mtu":1380,"peers":[{"publicKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","allowedIPs":["10.254.240.9/32"]}],"routes":["10.254.240.9/32"]}`,
	}}
	member := fake.NewSimpleClientset(node, configMap)
	cleaner := ConfigMapNodeWireGuardPeerCleaner{
		Members:      &memberClientStub{client: member},
		Clusters:     nodeOperationClusterListerStub{"cluster-a"},
		Observations: nodeWireGuardEvidenceStub{evidence: NodeWireGuardEvidence{PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", Addresses: []string{"10.254.240.9/32"}}},
	}
	op := &stationv1alpha1.NodeOperation{Status: stationv1alpha1.NodeOperationStatus{Stage: "Drain"}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "worker-a", NodeUID: nodeWGUID}}
	if err := cleaner.Check(ctx, op); err != nil {
		t.Fatalf("verified legacy config failed preflight: %v", err)
	}
	if done, err := cleaner.Remove(ctx, op); err != nil || !done {
		t.Fatalf("legacy config cleanup: done=%v err=%v", done, err)
	}
	updated, err := member.CoreV1().ConfigMaps(stationWireGuardNamespace).Get(ctx, stationWireGuardConfigMap, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(updated.Data["hub.json"], "10.254.240.9/32") {
		t.Fatalf("legacy peer route remains: %s", updated.Data["hub.json"])
	}
}

func TestManagementNodeWireGuardEvidenceBindsFreshObservationToNodeUID(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	observation := &stationv1alpha1.StationObservation{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-a", Namespace: "agent-a"},
		Status: stationv1alpha1.StationObservationStatus{
			NodeUID: nodeWGUID, ReportedAt: metav1.NewTime(now.Add(-10 * time.Second)),
			Wireguard: &stationv1alpha1.WireguardObservation{Interface: "st-wg0", PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", Addresses: []string{"10.254.240.9/32"}},
		},
	}
	reader := controllerfake.NewClientBuilder().WithScheme(scheme).WithObjects(observation).Build()
	source := ManagementNodeWireGuardEvidence{Reader: reader, Now: func() time.Time { return now }}
	evidence, err := source.Evidence(t.Context(), "agent-a", "worker-a", nodeWGUID)
	if err != nil || evidence.PublicKey != observation.Status.Wireguard.PublicKey || !slices.Contains(evidence.Addresses, "10.254.240.9/32") {
		t.Fatalf("fresh verified evidence=%+v err=%v", evidence, err)
	}
	evidence, err = source.Evidence(t.Context(), "agent-a", "worker-a", "replacement-uid")
	if err != nil || evidence.PublicKey != "" {
		t.Fatalf("observation was reused across Node UID: %+v err=%v", evidence, err)
	}
}

func TestConfigMapNodeWireGuardPeerCleanerRemovesPeersAcrossRegisteredClusters(t *testing.T) {
	ctx := context.Background()
	targetNode := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "worker-a", UID: nodeWGUID, Labels: map[string]string{stationNetworkLabel: "enabled"}}}
	targetCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: stationWireGuardConfigMap, Namespace: stationWireGuardNamespace}, Data: map[string]string{
		"worker-a.json": `{"interface":"st-wg0","nodeUID":"` + nodeWGUID + `","publicKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","address":"10.254.240.9/32","listenPort":51820,"mtu":1380,"peers":[],"routes":[]}`,
	}}
	hubCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: stationWireGuardConfigMap, Namespace: stationWireGuardNamespace}, Data: map[string]string{
		"hub.json":      `{"interface":"st-wg0","nodeUID":"hub-uid","address":"10.254.240.1/32","listenPort":51820,"mtu":1380,"peers":[{"publicKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","allowedIPs":["10.254.240.9/32"]}],"routes":["10.254.240.9/32"]}`,
		"worker-a.json": `{"interface":"st-wg0","nodeUID":"other-cluster-node-uid","publicKey":"BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB=","address":"10.254.240.10/32","listenPort":51820,"mtu":1380,"peers":[],"routes":[]}`,
	}}
	local := fake.NewSimpleClientset(targetNode, targetCM)
	hub := fake.NewSimpleClientset(hubCM)
	cleaner := ConfigMapNodeWireGuardPeerCleaner{
		Members:  nodeMemberClients{"cluster-a": local, "cluster-b": hub},
		Clusters: nodeOperationClusterListerStub{"cluster-a", "cluster-b"},
		Observations: nodeWireGuardEvidenceStub{evidence: NodeWireGuardEvidence{
			PublicKey: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", Addresses: []string{"10.254.240.9/32"},
		}},
	}
	op := &stationv1alpha1.NodeOperation{Status: stationv1alpha1.NodeOperationStatus{Stage: "Drain"}, Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "worker-a", NodeUID: nodeWGUID}}
	if err := cleaner.Check(ctx, op); err != nil {
		t.Fatal(err)
	}
	if done, err := cleaner.Remove(ctx, op); err != nil || !done {
		t.Fatalf("global peer cleanup: done=%v err=%v", done, err)
	}
	got, err := hub.CoreV1().ConfigMaps(stationWireGuardNamespace).Get(ctx, stationWireGuardConfigMap, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.Data["hub.json"], nodeWGUID) || strings.Contains(got.Data["hub.json"], "10.254.240.9/32") {
		t.Fatalf("remote hub config still references target peer: %s", got.Data["hub.json"])
	}
	if !strings.Contains(got.Data["worker-a.json"], "other-cluster-node-uid") {
		t.Fatalf("same-named foreign node config was modified: %s", got.Data["worker-a.json"])
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestANodeWhoseTunnelIsAHostServiceHasNoNetworkDaemonSetToStop(t *testing.T) {
	ctx := context.Background()
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "s1u-03-x86", UID: nodeWGUID, Labels: map[string]string{stationNetworkLabel: "host"}},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}}}}
	member := fake.NewSimpleClientset(node)
	cleaner := ConfigMapNodeWireGuardPeerCleaner{Members: &memberClientStub{client: member}, Clusters: nodeOperationClusterListerStub{"cluster-a"}}
	op := &stationv1alpha1.NodeOperation{Spec: stationv1alpha1.NodeOperationSpec{Action: "Remove", ClusterID: "cluster-a", NodeName: "s1u-03-x86", NodeUID: nodeWGUID}}
	if done, err := cleaner.Remove(ctx, op); err != nil || !done {
		t.Fatalf("remove: done=%v err=%v", done, err)
	}
	if stored, _ := member.CoreV1().Nodes().Get(ctx, "s1u-03-x86", metav1.GetOptions{}); stored.Labels[stationNetworkLabel] != "host" {
		t.Fatalf("the host label was changed: %v", stored.Labels)
	}
}
