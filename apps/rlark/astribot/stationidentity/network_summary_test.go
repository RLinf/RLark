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
	"k8s.io/client-go/kubernetes/fake"
)

type summaryReaderStub struct {
	nodeUID string
	err     error
}

func (s summaryReaderStub) Authenticate(context.Context, string) (Evidence, error) {
	return Evidence{}, errors.New("not used")
}

func (s summaryReaderStub) CurrentNodeUID(context.Context, string) (string, error) {
	return s.nodeUID, s.err
}

type summaryRegistryStub struct{ reader IdentityReader }

func (s summaryRegistryStub) Reader(context.Context, string) (IdentityReader, error) {
	return s.reader, nil
}

func TestNetworkSummaryEndpointRequiresDedicatedCredential(t *testing.T) {
	registry := summaryRegistryStub{summaryReaderStub{nodeUID: "node-uid"}}
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

func TestCurrentNodeUIDIsOfANodeThatIsNotBeingDeleted(t *testing.T) {
	deleting := metav1.Now()
	client := fake.NewSimpleClientset(&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node", UID: "node-uid"}}, &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "going", UID: "going-uid", DeletionTimestamp: &deleting}})
	reader := registeredReader{cluster: RegisteredCluster{Registration: Registration{ClusterID: "cluster"}, Client: client}}
	if uid, err := reader.CurrentNodeUID(context.Background(), "node"); err != nil || uid != "node-uid" {
		t.Fatalf("current Node: %q %v", uid, err)
	}
	if _, err := reader.CurrentNodeUID(context.Background(), "going"); !errors.Is(err, ErrIdentityDenied) {
		t.Fatalf("a Node that is being deleted: %v", err)
	}
}

// The summary is of the Node in the cluster now: a record of another Node UID,
// of a removed node, or with another Node's report, says nothing of it.
func TestNodeNetworkSummaryIsOfTheCurrentNodesOwnFreshReport(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	record := func(recorded, reported string, retired bool, received time.Time) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NetworkEnrollment",
			"metadata": map[string]interface{}{"namespace": "ns", "name": "node"},
			"spec":     map[string]interface{}{"clusterID": "cluster", "nodeUID": recorded, "retired": retired},
			"status": map[string]interface{}{"observation": map[string]interface{}{"nodeUID": reported, "reportedAt": received.Format(time.RFC3339), "wireguard": map[string]interface{}{"interface": "st-wg0", "addresses": []interface{}{"10.254.240.1/32"},
				"peers": []interface{}{map[string]interface{}{"name": "robot-01/s1u-01-x86", "receivedBytes": int64(148)}}}}},
		}}
	}
	for _, tc := range []struct {
		name     string
		objects  []runtime.Object
		expected string
		fail     bool
	}{
		{"no record", nil, "unreported", false},
		{"fresh", []runtime.Object{record("node-uid", "node-uid", false, now)}, "current", false},
		{"stale", []runtime.Object{record("node-uid", "node-uid", false, now.Add(-91*time.Second))}, "stale", false},
		{"record of another Node", []runtime.Object{record("old", "old", false, now)}, "unreported", false},
		{"report of another Node", []runtime.Object{record("node-uid", "old", false, now)}, "unreported", false},
		{"removed", []runtime.Object{record("node-uid", "node-uid", true, now)}, "unreported", false},
		{"future", []runtime.Object{record("node-uid", "node-uid", false, now.Add(time.Minute))}, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NodeNetworkSummaryReader{Registry: summaryRegistryStub{summaryReaderStub{nodeUID: "node-uid"}}, Client: testDynamicClient(tc.objects...), Namespaces: observationNamespaceMap{"cluster": "ns"}, Now: func() time.Time { return now }}
			got, err := s.Read(context.Background(), "cluster", "node")
			if (err != nil) != tc.fail || (!tc.fail && got.State != tc.expected) {
				t.Fatalf("result = %+v, err = %v", got, err)
			}
			if tc.expected == "current" && (got.Interface != "st-wg0" || len(got.Addresses) != 1) {
				t.Fatalf("summary = %+v", got)
			}
		})
	}
	denied := NodeNetworkSummaryReader{Registry: summaryRegistryStub{summaryReaderStub{err: ErrIdentityDenied}}, Client: testDynamicClient(), Namespaces: observationNamespaceMap{"cluster": "ns"}}
	if _, err := denied.Read(context.Background(), "cluster", "node"); !errors.Is(err, ErrIdentityDenied) {
		t.Fatalf("a Node that is not there: %v", err)
	}
}
