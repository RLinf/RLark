package stationidentity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

type summaryReaderStub struct {
	binding Binding
	err     error
}

func TestNetworkSummaryEndpointRequiresDedicatedCredential(t *testing.T) {
	registry := summaryRegistryStub{summaryReaderStub{binding: Binding{NodeUID: "node-uid", StationUID: "station-uid", Generation: 1}}}
	api := &API{Registry: registry, NetworkSummary: &NodeNetworkSummaryReader{Registry: registry, Client: testDynamicClient(), Namespaces: observationNamespaceMap{"cluster": "ns"}}, GatewayCredential: func() ([]byte, error) { return []byte(strings.Repeat("a", 32)), nil }}
	for _, tc := range []struct {
		name, token string
		status      int
	}{
		{"wrong", strings.Repeat("b", 32), http.StatusUnauthorized},
		{"correct", strings.Repeat("a", 32), http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/internal/node-network", strings.NewReader(`{"cluster_id":"cluster","node_name":"node"}`))
			req.Header.Set("Authorization", "Bearer "+tc.token)
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			api.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
		})
	}
}

type summaryBindingResolver struct{ binding Binding }

func (s summaryBindingResolver) Resolve(_ context.Context, _, _ string, uid string) (Binding, error) {
	if uid != s.binding.NodeUID {
		return Binding{}, errors.New("UID mismatch")
	}
	return s.binding, nil
}

func TestCurrentNodeBindingRejectsReplacementAndInactiveBinding(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", UID: types.UID("new-uid")}})
	reader := registeredReader{cluster: RegisteredCluster{Registration: Registration{ClusterID: "cluster"}, Client: client, Bindings: summaryBindingResolver{binding: Binding{NodeUID: "old-uid", StationUID: "station", Generation: 1}}}}
	if _, err := reader.CurrentNodeBinding(context.Background(), "node"); err == nil {
		t.Fatal("accepted replaced node")
	}
	reader.cluster.Bindings = summaryBindingResolver{binding: Binding{NodeUID: "new-uid", StationUID: "station", Generation: 2, Inactive: true}}
	if _, err := reader.CurrentNodeBinding(context.Background(), "node"); err == nil {
		t.Fatal("accepted inactive binding")
	}
	reader.cluster.Bindings = summaryBindingResolver{binding: Binding{NodeUID: "new-uid", StationUID: "station", Generation: 2}}
	if _, err := reader.CurrentNodeBinding(context.Background(), "node"); err != nil {
		t.Fatal(err)
	}
}

func (s summaryReaderStub) Authenticate(context.Context, string) (Evidence, error) {
	return Evidence{}, errors.New("not used")
}
func (s summaryReaderStub) CurrentNodeBinding(context.Context, string) (Binding, error) {
	return s.binding, s.err
}

type summaryRegistryStub struct{ reader IdentityReader }

func (s summaryRegistryStub) Reader(context.Context, string) (IdentityReader, error) {
	return s.reader, nil
}

func TestNodeNetworkSummaryUnboundAndIdentityErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		err        error
		unreported bool
	}{
		{"unbound", ErrStationBindingNotFound, true},
		{"identity unavailable", ErrIdentityUnavailable, false},
		{"identity conflict", ErrIdentityDenied, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NodeNetworkSummaryReader{Registry: summaryRegistryStub{summaryReaderStub{err: tc.err}}, Client: testDynamicClient(), Namespaces: observationNamespaceMap{"cluster": "ns"}}
			got, err := s.Read(context.Background(), "cluster", "node")
			if tc.unreported {
				if err != nil || got.State != "unreported" {
					t.Fatalf("got %+v, %v", got, err)
				}
			} else if !errors.Is(err, tc.err) {
				t.Fatalf("got error %v, want %v", err, tc.err)
			}
		})
	}
}

func TestNodeNetworkSummaryChecksIdentityOwnershipAndFreshness(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	makeObservation := func(uid, station, received string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "StationObservation",
			"metadata": map[string]interface{}{"namespace": "ns", "name": "node", "annotations": map[string]interface{}{
				observationClusterIDKey: "cluster", observationNodeUIDKey: uid, observationStationUIDKey: station, observationBindingVersionKey: "1",
			}},
			"status": map[string]interface{}{"nodeUID": uid, "reportedAt": received, "wireguard": map[string]interface{}{"interface": "st-wg0", "addresses": []interface{}{"10.254.240.1/32"},
				"peers": []interface{}{map[string]interface{}{"name": "robot-01/s1u-01-x86", "receivedBytes": int64(148)}}}},
		}}
	}
	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		binding  Binding
		expected string
		fail     bool
	}{
		{"fresh", makeObservation("node-uid", "station-uid", now.Format(time.RFC3339)), Binding{NodeUID: "node-uid", StationUID: "station-uid", Generation: 1}, "current", false},
		{"stale", makeObservation("node-uid", "station-uid", now.Add(-91*time.Second).Format(time.RFC3339)), Binding{NodeUID: "node-uid", StationUID: "station-uid", Generation: 1}, "stale", false},
		{"node replaced", makeObservation("old", "station-uid", now.Format(time.RFC3339)), Binding{NodeUID: "new", StationUID: "station-uid", Generation: 2}, "", true},
		{"station replaced", makeObservation("node-uid", "old", now.Format(time.RFC3339)), Binding{NodeUID: "node-uid", StationUID: "new", Generation: 1}, "", true},
		{"binding changed", makeObservation("node-uid", "station-uid", now.Format(time.RFC3339)), Binding{NodeUID: "node-uid", StationUID: "station-uid", Generation: 2}, "", true},
		{"future", makeObservation("node-uid", "station-uid", now.Add(time.Minute).Format(time.RFC3339)), Binding{NodeUID: "node-uid", StationUID: "station-uid", Generation: 1}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := testDynamicClient([]runtime.Object{tc.obj}...)
			s := NodeNetworkSummaryReader{Registry: summaryRegistryStub{summaryReaderStub{binding: tc.binding}}, Client: client, Namespaces: observationNamespaceMap{"cluster": "ns"}, Now: func() time.Time { return now }}
			got, err := s.Read(context.Background(), "cluster", "node")
			if (err != nil) != tc.fail || (!tc.fail && got.State != tc.expected) {
				t.Fatalf("result = %+v, err = %v", got, err)
			}
		})
	}
}
