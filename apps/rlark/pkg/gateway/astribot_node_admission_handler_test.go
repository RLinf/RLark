package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

type fakeStationBootstrap struct{ paths []string }

func (f *fakeStationBootstrap) Address() string { return "10.11.14.128:18443" }

func (f *fakeStationBootstrap) TunnelAddress() string { return "10.254.240.1:18443" }

func (f *fakeStationBootstrap) Digest(_ context.Context, path string) (string, error) {
	f.paths = append(f.paths, path)
	if path == "/v1/bootstrap/ca" {
		return "cadigest", nil
	}
	return "uplinkdigest", nil
}

func (f *fakeStationBootstrap) Open(_ context.Context, path string) (io.ReadCloser, int64, error) {
	f.paths = append(f.paths, path)
	return io.NopCloser(strings.NewReader(fakeInstaller)), int64(len(fakeInstaller)), nil
}

func (f *fakeStationBootstrap) BundleDigest(context.Context) (string, error) {
	return sha256Hex(fakeInstaller), nil
}

// fakeInstaller is what the fake Station API serves as the installer.
const fakeInstaller = "installer bytes"

func nodeAdmissionGateway(objects ...runtime.Object) (*Gateway, *fakeStationBootstrap) {
	network := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "SiteNetwork",
		"metadata": map[string]interface{}{"name": "default", "namespace": "station-system"},
		"spec": map[string]interface{}{"pool": "10.254.240.0/24", "sites": []interface{}{
			map[string]interface{}{"name": "office", "clusters": []interface{}{"cluster-a"}},
			map[string]interface{}{"name": "farm", "leafUplink": map[string]interface{}{"gateway": "192.168.0.11"}},
		}},
	}}
	class := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeClass", "metadata": map[string]interface{}{"name": "s1u"},
		"spec": map[string]interface{}{
			"access": map[string]interface{}{"entry": map[string]interface{}{"port": int64(22), "user": "nvidia"}},
			"uplink": map[string]interface{}{"setup": map[string]interface{}{"addon": "s1u-robot", "path": "uplink.sh"}}},
	}}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		stationClusterGVR: "ClusterList", nodeAdmissionGVR: "NodeAdmissionList", siteNetworkGVR: "SiteNetworkList",
	}, append([]runtime.Object{stationHandlerCluster("cluster-a", "agent-a"), network, class}, objects...)...)
	bootstrap := &fakeStationBootstrap{}
	return &Gateway{astribotState: astribotState{stationClient: client, stationBootstrap: bootstrap}, kubeClient: clientset.NewSimpleClientset()}, bootstrap
}

func nodeAdmissionCall(t *testing.T, handler gin.HandlerFunc, method, path, name, body string) (int, []byte) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	ctx.Params = gin.Params{{Key: "name", Value: name}}
	ctx.Set(authSubjectKey, "alice")
	handler(ctx)
	return recorder.Code, recorder.Body.Bytes()
}

func storedAdmission(t *testing.T, g *Gateway, name string) *unstructured.Unstructured {
	t.Helper()
	object, err := g.stationClient.Resource(nodeAdmissionGVR).Namespace("agent-a").Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return object
}

func TestIssueNodeAdmissionStoresOnlyTheHashAndAnswersTheCommandOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, bootstrap := nodeAdmissionGateway()
	status, body := nodeAdmissionCall(t, g.handleIssueNodeAdmission, "POST", "/api/v1/station/node-admissions", "",
		`{"cluster_id":"cluster-a","node_name":"s1u-09-x86","labels":{"rlark.io/robot-id":"s1u-09"},"taints":["rlark.io/node-category=robot:NoSchedule"],"ttl_seconds":600}`)
	if status != 201 {
		t.Fatalf("status %d: %s", status, body)
	}
	var issued issuedNodeAdmission
	if err := json.Unmarshal(body, &issued); err != nil {
		t.Fatal(err)
	}
	if len(issued.Code) != joinCodeLength || strings.Trim(issued.Code, "abcdefghijklmnopqrstuvwxyz0123456789") != "" {
		t.Fatalf("code %q", issued.Code)
	}
	want := `c=$(mktemp) && curl --connect-timeout 15 -fsSk https://10.11.14.128:18443/v1/bootstrap/ca -o $c && echo "cadigest  $c" | sha256sum -c - && curl -fsS --cacert $c https://10.11.14.128:18443/b/` + issued.Code + ` | sudo sh`
	if issued.Command != want {
		t.Fatalf("command\n%s\nwant\n%s", issued.Command, want)
	}
	if issued.Admission.State != "Pending" || issued.Admission.IssuedBy != "alice" || issued.Admission.Labels["rlark.io/robot-id"] != "s1u-09" {
		t.Fatalf("admission %+v", issued.Admission)
	}
	stored := storedAdmission(t, g, "s1u-09-x86")
	digest := sha256.Sum256([]byte(issued.Code))
	if hash, _, _ := unstructured.NestedString(stored.Object, "spec", "codeHash"); hash != hex.EncodeToString(digest[:]) {
		t.Fatalf("stored hash %q", hash)
	}
	raw, _ := json.Marshal(stored.Object)
	if strings.Contains(string(raw), issued.Code) {
		t.Fatal("the code itself is stored")
	}
	expires, _ := time.Parse(time.RFC3339, issued.Admission.ExpiresAt)
	if d := time.Until(expires); d < 9*time.Minute || d > 11*time.Minute {
		t.Fatalf("expires in %s", d)
	}
	if strings.Join(bootstrap.paths, ",") != "/v1/bootstrap/ca" {
		t.Fatalf("Station API reads %v", bootstrap.paths)
	}

	// The list never carries the code or its hash.
	status, body = nodeAdmissionCall(t, g.handleListNodeAdmissions, "GET", "/api/v1/station/node-admissions?cluster_id=cluster-a", "", "")
	if status != 200 || strings.Contains(string(body), issued.Code) || strings.Contains(string(body), hex.EncodeToString(digest[:])) || !strings.Contains(string(body), `"state":"Pending"`) {
		t.Fatalf("list %d: %s", status, body)
	}
}

func TestIssueNodeAdmissionBehindADeviceLetsTheMachineOutFirst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, bootstrap := nodeAdmissionGateway()
	status, body := nodeAdmissionCall(t, g.handleIssueNodeAdmission, "POST", "/api/v1/station/node-admissions", "",
		`{"cluster_id":"cluster-a","node_name":"s1u-10-x86","site":"farm","node_class":"s1u"}`)
	var issued issuedNodeAdmission
	_ = json.Unmarshal(body, &issued)
	if status != 201 || !strings.HasPrefix(issued.Command, `ssh -t nvidia@192.168.0.11 'f=$(mktemp) c=$(mktemp) && `) ||
		!strings.Contains(issued.Command, `/b/`+issued.Code+`/uplink -o $f && echo "uplinkdigest  $f" | sha256sum -c - && sudo sh $f' && c=$(mktemp)`) {
		t.Fatalf("status %d command %s", status, issued.Command)
	}
	if strings.Join(bootstrap.paths, ",") != "/v1/bootstrap/ca,/b/"+issued.Code+"/uplink" {
		t.Fatalf("Station API reads %v", bootstrap.paths)
	}
}

func TestIssueNodeAdmissionRejectsWhatTheManagementPlaneWouldNot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, _ := nodeAdmissionGateway()
	for _, body := range []string{
		`{"cluster_id":"cluster-a","node_name":"S1U"}`,
		`{"cluster_id":"cluster-a","node_name":"n","taints":["a=b"]}`,
		`{"cluster_id":"cluster-a","node_name":"n","address":"fd00::1"}`,
		`{"cluster_id":"cluster-a","node_name":"n","site":"moon"}`,
		`{"cluster_id":"cluster-a","node_name":"n","ttl_seconds":60}`,
		`{"cluster_id":"cluster-a","node_name":"n","code":"abcdefgh"}`,
		// Where the cluster's API is, Station API reads from the cluster (cluster-info): nobody types it.
		`{"cluster_id":"cluster-a","node_name":"n","api_server":"10.11.14.128:6443"}`,
	} {
		if status, answer := nodeAdmissionCall(t, g.handleIssueNodeAdmission, "POST", "/api/v1/station/node-admissions", "", body); status != 400 {
			t.Errorf("%s: %d %s", body, status, answer)
		}
	}
}

func redeemedAdmission() *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
		"metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "agent-a", "generation": int64(3)},
		"spec": map[string]interface{}{"clusterID": "cluster-a", "codeHash": strings.Repeat("a", 64), "expiresAt": "2026-11-14T10:00:00Z",
			"address": "10.254.240.9", "labels": map[string]interface{}{"old": "label"},
			"publicKey": "pk", "redeemedAt": "2026-11-14T09:00:00Z", "issuedBy": "bob", "issuedAt": "2026-11-14T08:00:00Z"},
		"status": map[string]interface{}{"observedGeneration": int64(3), "phase": "Joined", "address": "10.254.240.9"},
	}}
}

func TestNewCodeForAnAdmittedNodeOpensItAgain(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, _ := nodeAdmissionGateway(redeemedAdmission())
	status, body := nodeAdmissionCall(t, g.handleListNodeAdmissions, "GET", "/api/v1/station/node-admissions?cluster_id=cluster-a", "", "")
	if status != 200 || !strings.Contains(string(body), `"state":"Joined"`) || !strings.Contains(string(body), `"public_key":"pk"`) {
		t.Fatalf("list %d: %s", status, body)
	}
	// Revoking a code a machine has redeemed is refused.
	if status, _ := nodeAdmissionCall(t, g.handleRevokeNodeAdmission, "POST", "/api/v1/station/node-admissions/s1u-01-x86/revoke?cluster_id=cluster-a", "s1u-01-x86", ""); status != 409 {
		t.Fatalf("revoke of a redeemed code: %d", status)
	}
	// Reissue keeps what the admission declares and clears the redemption.
	status, body = nodeAdmissionCall(t, g.handleReissueNodeAdmission, "POST", "/api/v1/station/node-admissions/s1u-01-x86/reissue?cluster_id=cluster-a", "s1u-01-x86", "")
	var issued issuedNodeAdmission
	_ = json.Unmarshal(body, &issued)
	if status != 200 || issued.Admission.State != "Pending" || issued.Admission.IssuedBy != "alice" || issued.Admission.Address != "10.254.240.9" || issued.Admission.Labels["old"] != "label" || issued.Admission.PublicKey != "" {
		t.Fatalf("reissue %d: %s", status, body)
	}
	// A new admission for the node replaces what it declares, labels as one value.
	status, body = nodeAdmissionCall(t, g.handleIssueNodeAdmission, "POST", "/api/v1/station/node-admissions", "",
		`{"cluster_id":"cluster-a","node_name":"s1u-01-x86","labels":{"new":"label"}}`)
	if status != 200 {
		t.Fatalf("readmit %d: %s", status, body)
	}
	labels, _, _ := unstructured.NestedStringMap(storedAdmission(t, g, "s1u-01-x86").Object, "spec", "labels")
	if len(labels) != 1 || labels["new"] != "label" {
		t.Fatalf("labels %v", labels)
	}
}

func TestRevokeEndsAnOpenCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, _ := nodeAdmissionGateway()
	if status, body := nodeAdmissionCall(t, g.handleIssueNodeAdmission, "POST", "/api/v1/station/node-admissions", "",
		`{"cluster_id":"cluster-a","node_name":"n1"}`); status != 201 {
		t.Fatalf("issue %d: %s", status, body)
	}
	status, body := nodeAdmissionCall(t, g.handleRevokeNodeAdmission, "POST", "/api/v1/station/node-admissions/n1/revoke?cluster_id=cluster-a", "n1", "")
	if status != 200 || !strings.Contains(string(body), `"state":"Expired"`) {
		t.Fatalf("revoke %d: %s", status, body)
	}
	expires, _, _ := unstructured.NestedString(storedAdmission(t, g, "n1").Object, "spec", "expiresAt")
	if at, _ := time.Parse(time.RFC3339, expires); time.Now().Before(at) {
		t.Fatalf("still open until %s", expires)
	}
	if status, _ := nodeAdmissionCall(t, g.handleRevokeNodeAdmission, "POST", "/api/v1/station/node-admissions/n1/revoke?cluster_id=cluster-a", "n1", ""); status != 200 {
		t.Fatalf("second revoke %d", status)
	}
	if status, _ := nodeAdmissionCall(t, g.handleRevokeNodeAdmission, "POST", "/api/v1/station/node-admissions/none/revoke?cluster_id=cluster-a", "none", ""); status != 404 {
		t.Fatalf("revoke of nothing %d", status)
	}
}

// A machine the SiteNetwork declares with its key (a new site's first machines, which reach Station API only
// through their tunnel) is given a command that redeems through the tunnel.
func TestIssueNodeAdmissionForADeclaredKeyRedeemsThroughTheTunnel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, _ := nodeAdmissionGateway()
	network, err := g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Get(context.Background(), "default", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_ = unstructured.SetNestedSlice(network.Object, []interface{}{map[string]interface{}{
		"clusterID": "cluster-a", "nodeName": "gpu-2", "role": "Hub", "address": "10.254.240.40", "publicKey": strings.Repeat("A", 43) + "="}}, "spec", "nodes")
	if _, err := g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Update(context.Background(), network, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	for node, address := range map[string]string{"gpu-2": "10.254.240.1:18443", "gpu-3": "10.11.14.128:18443"} {
		status, body := nodeAdmissionCall(t, g.handleIssueNodeAdmission, "POST", "/api/v1/station/node-admissions", "",
			`{"cluster_id":"cluster-a","node_name":"`+node+`"}`)
		var issued issuedNodeAdmission
		if err := json.Unmarshal(body, &issued); status != 201 || err != nil {
			t.Fatalf("status %d: %s", status, body)
		}
		if strings.Count(issued.Command, "https://"+address+"/") != 2 {
			t.Fatalf("%s: command %s, want Station API at %s", node, issued.Command, address)
		}
	}
}

func TestIssueServerNodeAdmissionWithoutNodeClass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	g, bootstrap := nodeAdmissionGateway()
	status, body := nodeAdmissionCall(t, g.handleIssueNodeAdmission, "POST", "/api/v1/station/node-admissions", "",
		`{"cluster_id":"cluster-a","node_name":"office-server-01","machine":{"category":"Server"}}`)
	var issued issuedNodeAdmission
	if status != 201 || json.Unmarshal(body, &issued) != nil ||
		!strings.HasPrefix(issued.Command, "c=$(mktemp)") || issued.Admission.State != "Pending" ||
		strings.Join(bootstrap.paths, ",") != "/v1/bootstrap/ca" {
		t.Fatalf("server admission did not return a direct bootstrap command: HTTP %d", status)
	}
	stored := storedAdmission(t, g, "office-server-01")
	category, _, _ := unstructured.NestedString(stored.Object, "spec", "machine", "category")
	class, _, _ := unstructured.NestedString(stored.Object, "spec", "nodeClass")
	if category != "Server" || class != "" {
		t.Fatalf("admission records category %q and NodeClass %q", category, class)
	}
}

func TestIndexNodeJoinsUsesLastMatch(t *testing.T) {
	joins := []rlarkv1alpha1.NodeJoin{{Spec: rlarkv1alpha1.NodeJoinSpec{NodeName: "node-a"}}, {Spec: rlarkv1alpha1.NodeJoinSpec{NodeName: "node-b"}}, {Spec: rlarkv1alpha1.NodeJoinSpec{NodeName: "node-a"}}}
	got := indexNodeJoins(joins)
	if len(got) != 2 || got["node-a"].Status.Stage != joins[2].Status.Stage {
		t.Fatalf("join index = %#v", got)
	}
}

func TestListNodeAdmissionsPaginatesAndUsesIndexedJoin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	makeAdmission := func(name, issued string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
			"metadata": map[string]interface{}{"name": name, "namespace": "agent-a"},
			"spec":     map[string]interface{}{"clusterID": "cluster-a", "issuedAt": issued, "expiresAt": "2099-01-01T00:00:00Z"},
		}}
	}
	g, _ := nodeAdmissionGateway(makeAdmission("node-old", "2026-11-01T00:00:00Z"), makeAdmission("node-new", "2026-11-02T00:00:00Z"))
	g.kubeClient = clientset.NewSimpleClientset(
		&rlarkv1alpha1.NodeJoin{ObjectMeta: metav1.ObjectMeta{Name: "join-new", Labels: map[string]string{nodeJoinClusterLabel: "cluster-a"}}, Spec: rlarkv1alpha1.NodeJoinSpec{ClusterID: "cluster-a", NodeName: "node-new"}, Status: rlarkv1alpha1.NodeJoinStatus{Phase: rlarkv1alpha1.ClusterProvisionRunning, Stage: "configure"}},
	)
	call := func(path string) (int, []byte) {
		return nodeAdmissionCall(t, g.handleListNodeAdmissions, "GET", path, "", "")
	}
	status, body := call("/api/v1/station/node-admissions?cluster_id=cluster-a&limit=1")
	var first struct {
		Items    []nodeAdmission `json:"items"`
		Continue string          `json:"continue"`
		Total    int             `json:"totalMatched"`
	}
	if err := json.Unmarshal(body, &first); status != 200 || err != nil || len(first.Items) != 1 || first.Items[0].NodeName != "node-new" || first.Items[0].JoinPhase != string(rlarkv1alpha1.ClusterProvisionRunning) || first.Items[0].JoinStage != "configure" || first.Continue != "1" || first.Total != 2 {
		t.Fatalf("page one status=%d body=%s decoded=%+v", status, body, first)
	}
	status, body = call("/api/v1/station/node-admissions?cluster_id=cluster-a&limit=1&continue=" + first.Continue)
	var second struct {
		Items    []nodeAdmission `json:"items"`
		Continue string          `json:"continue"`
		Total    int             `json:"totalMatched"`
	}
	if err := json.Unmarshal(body, &second); status != 200 || err != nil || len(second.Items) != 1 || second.Items[0].NodeName != "node-old" || second.Continue != "" || second.Total != 2 {
		t.Fatalf("page two status=%d body=%s decoded=%+v", status, body, second)
	}
}

func BenchmarkListNodeAdmissionsJoin(b *testing.B) {
	joins := make([]rlarkv1alpha1.NodeJoin, 500)
	for i := range joins {
		joins[i].Spec.NodeName = fmt.Sprintf("node-%d", i)
	}
	admissions := make([]string, 500)
	for i := range admissions {
		admissions[i] = fmt.Sprintf("node-%d", i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		byNode := indexNodeJoins(joins)
		for _, node := range admissions {
			_ = byNode[node]
		}
	}
}
