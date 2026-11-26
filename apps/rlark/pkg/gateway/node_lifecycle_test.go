package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	v1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func newAdminNodeContext(t *testing.T, method, target string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, nil)
	c.Set(authRoleKey, roleAdmin)
	return c, recorder
}

func performDrainNode(t *testing.T, g *Gateway) *httptest.ResponseRecorder {
	t.Helper()
	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/node-a/drain?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}
	g.drainNode(c)
	return recorder
}

func decodeDrainNodeResponse(t *testing.T, recorder *httptest.ResponseRecorder) (string, v1alpha1.Node) {
	t.Helper()
	var body struct {
		OperationID string        `json:"operationId"`
		Data        v1alpha1.Node `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.OperationID, body.Data
}

func TestUncordonNodeClearsUnschedulable(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec:       v1alpha1.NodeSpec{Unschedulable: true},
	})
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/node-a/uncordon?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.uncordonNode(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}

	got, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Unschedulable {
		t.Fatalf("Unschedulable = true, want false")
	}
}

func TestUncordonNodeCancelsDrainAndKeepsDrainedTasksStopped(t *testing.T) {
	node := &v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec: v1alpha1.NodeSpec{
			Unschedulable: true,
			Operations: []v1alpha1.NodeOperationRequest{
				{ID: "drain-1", Type: v1alpha1.NodeOperationDrain},
				{ID: "maint-1", Type: v1alpha1.NodeOperationEnterMaintenance},
			},
		},
	}
	drained := &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task-drained", Namespace: "ns-1", Annotations: map[string]string{
			v1alpha1.NodeDrainStoppedAnnotation:   "node-a",
			v1alpha1.NodeDrainSuspendedAnnotation: "node-a",
		}},
		Spec: v1alpha1.TaskSpec{
			Suspended:     true,
			DrainRequests: []v1alpha1.TaskDrainRequest{{OperationID: "drain-1", NodeName: "node-a"}},
		},
	}
	// A Task the user suspended before the drain keeps its own desired state.
	userSuspended := &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task-user-suspended", Namespace: "ns-1", Annotations: map[string]string{
			v1alpha1.NodeDrainStoppedAnnotation: "node-a",
		}},
		Spec: v1alpha1.TaskSpec{Suspended: true},
	}
	client := clientset.NewSimpleClientset(node, drained, userSuspended)
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/node-a/uncordon?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.uncordonNode(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}

	gotNode, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if gotNode.Spec.Unschedulable {
		t.Fatal("Unschedulable = true, want false")
	}
	if len(gotNode.Spec.Operations) != 1 || gotNode.Spec.Operations[0].ID != "maint-1" {
		t.Fatalf("operations = %#v, want only the EnterMaintenance operation", gotNode.Spec.Operations)
	}

	gotDrained, err := client.RlinfV1alpha1().Tasks("ns-1").Get(context.Background(), "task-drained", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !gotDrained.Spec.Suspended {
		t.Fatal("drained Task must stay suspended until its Job is started")
	}
	if len(gotDrained.Spec.DrainRequests) != 0 {
		t.Fatalf("drain requests = %#v, want released", gotDrained.Spec.DrainRequests)
	}
	if gotDrained.Annotations[v1alpha1.NodeDrainStoppedAnnotation] != "node-a" {
		t.Fatalf("node drain marker must stay until the Job is started: %#v", gotDrained.Annotations)
	}
	if gotDrained.Annotations[v1alpha1.NodeDrainSuspendedAnnotation] != "node-a" {
		t.Fatalf("node drain suspension marker must stay until the Job is started: %#v", gotDrained.Annotations)
	}

	gotUserSuspended, err := client.RlinfV1alpha1().Tasks("ns-1").Get(context.Background(), "task-user-suspended", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !gotUserSuspended.Spec.Suspended {
		t.Fatal("uncordon must not resume a Task the user suspended")
	}
	if gotUserSuspended.Annotations[v1alpha1.NodeDrainStoppedAnnotation] != "node-a" {
		t.Fatalf("node drain marker must stay until the Job is started: %#v", gotUserSuspended.Annotations)
	}
}

func TestUncordonNodeShortCircuitsWhenAlreadySchedulable(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec:       v1alpha1.NodeSpec{Unschedulable: false},
	})
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/node-a/uncordon?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.uncordonNode(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUncordonNodeMissingName(t *testing.T) {
	client := clientset.NewSimpleClientset()
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes//uncordon?namespace=ns-1")

	g.uncordonNode(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
}

func TestUncordonNodeMissingNamespace(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
	})
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/node-a/uncordon")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.uncordonNode(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "namespace") {
		t.Fatalf("body = %s, want namespace hint", recorder.Body.String())
	}
}

func TestUncordonNodeNotFound(t *testing.T) {
	client := clientset.NewSimpleClientset()
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/missing/uncordon?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "missing"}}

	g.uncordonNode(c)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", recorder.Code, recorder.Body.String())
	}
}

// --- drainNode ---

func TestDrainNodeReusesUnfinishedOperationAtLimit(t *testing.T) {
	operations := []v1alpha1.NodeOperationRequest{{ID: "drain-existing", Type: v1alpha1.NodeOperationDrain}}
	for len(operations) < maxNodeOperations {
		operations = append(operations, v1alpha1.NodeOperationRequest{
			ID:   fmt.Sprintf("maintenance-%d", len(operations)),
			Type: v1alpha1.NodeOperationEnterMaintenance,
		})
	}
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec:       v1alpha1.NodeSpec{Unschedulable: true, Operations: operations},
	})
	g := &Gateway{kubeClient: client}

	for i := 0; i < 2; i++ {
		recorder := performDrainNode(t, g)
		if recorder.Code != http.StatusOK {
			t.Fatalf("request %d status = %d, want 200: %s", i+1, recorder.Code, recorder.Body.String())
		}
		opID, _ := decodeDrainNodeResponse(t, recorder)
		if opID != "drain-existing" {
			t.Fatalf("request %d operationId = %q, want existing operation", i+1, opID)
		}
	}

	got, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Operations) != maxNodeOperations {
		t.Fatalf("operations length = %d, want unchanged %d", len(got.Spec.Operations), maxNodeOperations)
	}
	drainCount := 0
	for _, operation := range got.Spec.Operations {
		if operation.Type == v1alpha1.NodeOperationDrain {
			drainCount++
		}
	}
	if drainCount != 1 {
		t.Fatalf("Drain operation count = %d, want 1", drainCount)
	}
}

func TestDrainNodeCreatesNewOperationAfterCompletion(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec: v1alpha1.NodeSpec{
			Unschedulable: true,
			Operations:    []v1alpha1.NodeOperationRequest{{ID: "drain-complete", Type: v1alpha1.NodeOperationDrain}},
		},
		Status: v1alpha1.NodeStatus{DrainOperations: []v1alpha1.NodeDrainOperationStatus{{
			OperationID: "drain-complete",
			Completed:   true,
		}}},
	})
	g := &Gateway{kubeClient: client}

	recorder := performDrainNode(t, g)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	opID, _ := decodeDrainNodeResponse(t, recorder)
	if opID == "" || opID == "drain-complete" {
		t.Fatalf("operationId = %q, want a new operation", opID)
	}
	_, responseNode := decodeDrainNodeResponse(t, recorder)
	if len(responseNode.Spec.Operations) != 2 || responseNode.Spec.Operations[1].ID != opID {
		t.Fatalf("response operations = %#v, want completed and new operations", responseNode.Spec.Operations)
	}
}

func TestDrainNodeRejectsOperationLimit(t *testing.T) {
	operations := make([]v1alpha1.NodeOperationRequest, 0, maxNodeOperations)
	for len(operations) < maxNodeOperations {
		operations = append(operations, v1alpha1.NodeOperationRequest{
			ID:   fmt.Sprintf("maintenance-%d", len(operations)),
			Type: v1alpha1.NodeOperationEnterMaintenance,
		})
	}
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec:       v1alpha1.NodeSpec{Operations: operations},
	})
	g := &Gateway{kubeClient: client}

	recorder := performDrainNode(t, g)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	var body ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != CodeResourceConflict || body.Details["field"] != "spec.operations" || body.Details["limit"] != float64(maxNodeOperations) {
		t.Fatalf("error response = %#v, want operation limit details", body)
	}
	got, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Operations) != maxNodeOperations || got.Spec.Unschedulable {
		t.Fatalf("Node mutated after rejected drain: operations=%d unschedulable=%t", len(got.Spec.Operations), got.Spec.Unschedulable)
	}
}

func TestDrainNodeRetriesConflictWithoutDuplicateOperation(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
	})
	patchAttempts := 0
	client.PrependReactor("patch", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patchAttempts++
		if patchAttempts == 1 {
			return true, nil, apierrors.NewConflict(v1alpha1.Resource("nodes"), "node-a", fmt.Errorf("injected conflict"))
		}
		return false, nil, nil
	})
	g := &Gateway{kubeClient: client}

	recorder := performDrainNode(t, g)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if patchAttempts != 2 {
		t.Fatalf("patch attempts = %d, want 2", patchAttempts)
	}
	opID, _ := decodeDrainNodeResponse(t, recorder)
	got, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Spec.Operations) != 1 || got.Spec.Operations[0].ID != opID || !got.Spec.Unschedulable {
		t.Fatalf("Node after conflict retry = %#v, operationId = %q", got.Spec, opID)
	}
}

func TestMergeNodePatchCarriesResourceVersion(t *testing.T) {
	before := &v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1", ResourceVersion: "42"},
	}
	after := before.DeepCopy()
	after.Spec.Unschedulable = true

	patch, err := mergeNodePatch(before, after)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(patch, &document); err != nil {
		t.Fatal(err)
	}
	metadata, _ := document["metadata"].(map[string]any)
	if metadata["resourceVersion"] != "42" {
		t.Fatalf("patch metadata = %#v, want resourceVersion precondition 42", metadata)
	}
	spec, _ := document["spec"].(map[string]any)
	if spec["unschedulable"] != true {
		t.Fatalf("patch spec = %#v, want unschedulable change", spec)
	}
}

func TestDrainNodePreservesConcurrentOperationOnConflict(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
	})
	gvr := v1alpha1.GroupVersion.WithResource("nodes")
	patchAttempts := 0
	client.PrependReactor("patch", "nodes", func(action k8stesting.Action) (bool, runtime.Object, error) {
		patchAttempts++
		if patchAttempts > 1 {
			return false, nil, nil
		}
		// Simulate a concurrent enter-maintenance request that committed after
		// this drain read the Node, then reject the stale patch as the API
		// server would with a resourceVersion precondition.
		obj, err := client.Tracker().Get(gvr, "ns-1", "node-a")
		if err != nil {
			t.Fatal(err)
		}
		concurrent := obj.(*v1alpha1.Node).DeepCopy()
		concurrent.Spec.Operations = append(concurrent.Spec.Operations, v1alpha1.NodeOperationRequest{
			ID:   "maintenance-concurrent",
			Type: v1alpha1.NodeOperationEnterMaintenance,
		})
		if err := client.Tracker().Update(gvr, concurrent, "ns-1"); err != nil {
			t.Fatal(err)
		}
		return true, nil, apierrors.NewConflict(v1alpha1.Resource("nodes"), "node-a", fmt.Errorf("injected conflict"))
	})
	g := &Gateway{kubeClient: client}

	recorder := performDrainNode(t, g)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if patchAttempts != 2 {
		t.Fatalf("patch attempts = %d, want 2", patchAttempts)
	}
	opID, _ := decodeDrainNodeResponse(t, recorder)
	got, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]bool, len(got.Spec.Operations))
	for _, operation := range got.Spec.Operations {
		ids[operation.ID] = true
	}
	if !ids["maintenance-concurrent"] || !ids[opID] || len(got.Spec.Operations) != 2 {
		t.Fatalf("Operations = %#v, want concurrent maintenance and drain %q", got.Spec.Operations, opID)
	}
}

// --- rlinfv1alpha1DeleteNode (取消纳管) ---

func TestDeleteNodeRejectsWhenTasksObserved(t *testing.T) {
	client := clientset.NewSimpleClientset(
		&v1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
			Spec:       v1alpha1.NodeSpec{Unschedulable: true},
		},
		&v1alpha1.Task{
			ObjectMeta: metav1.ObjectMeta{Name: "task-1", Namespace: "ns-1"},
			Status:     v1alpha1.TaskStatus{ObservedNodes: []string{"node-a"}},
		},
	)
	g := &Gateway{kubeClient: client, accessors: registerAccessors(client)}

	c, recorder := newAdminNodeContext(t, http.MethodDelete, "/rlinf.io/v1alpha1/nodes/node-a?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.rlinfv1alpha1DeleteNode(c)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "task-1") {
		t.Fatalf("body = %s, want task name in message", recorder.Body.String())
	}
	if _, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{}); err != nil {
		t.Fatalf("node must survive rejected delete: %v", err)
	}
}

func TestDeleteNodeAllowsWhenNoTasksOnNode(t *testing.T) {
	client := clientset.NewSimpleClientset(
		&v1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
			Spec:       v1alpha1.NodeSpec{Unschedulable: true},
		},
		&v1alpha1.Task{
			ObjectMeta: metav1.ObjectMeta{Name: "task-1", Namespace: "ns-1"},
			Status:     v1alpha1.TaskStatus{ObservedNodes: []string{"node-b"}},
		},
		&v1alpha1.Task{
			ObjectMeta: metav1.ObjectMeta{Name: "task-2", Namespace: "other-ns"},
			Status:     v1alpha1.TaskStatus{ObservedNodes: []string{"node-a"}},
		},
	)
	g := &Gateway{kubeClient: client, accessors: registerAccessors(client)}

	c, recorder := newAdminNodeContext(t, http.MethodDelete, "/rlinf.io/v1alpha1/nodes/node-a?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.rlinfv1alpha1DeleteNode(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if _, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{}); err == nil {
		t.Fatal("node must be deleted when no tasks observe it")
	}
}

func TestDeleteNodeReleasesDrainRequestsWhenAgentOffline(t *testing.T) {
	client := clientset.NewSimpleClientset(
		&v1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
			Spec:       v1alpha1.NodeSpec{Unschedulable: true},
		},
		&v1alpha1.Task{
			ObjectMeta: metav1.ObjectMeta{Name: "task-1", Namespace: "ns-1"},
			Spec: v1alpha1.TaskSpec{DrainRequests: []v1alpha1.TaskDrainRequest{
				{OperationID: "drain-1", NodeUID: "node-uid", NodeName: "node-a"},
				{OperationID: "drain-2", NodeUID: "other-uid", NodeName: "node-b"},
			}},
		},
	)
	g := &Gateway{kubeClient: client, accessors: registerAccessors(client)}

	c, recorder := newAdminNodeContext(t, http.MethodDelete, "/rlinf.io/v1alpha1/nodes/node-a?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.rlinfv1alpha1DeleteNode(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if _, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{}); err == nil {
		t.Fatal("node must be deleted")
	}
	task, err := client.RlinfV1alpha1().Tasks("ns-1").Get(context.Background(), "task-1", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Spec.DrainRequests) != 1 || task.Spec.DrainRequests[0].OperationID != "drain-2" {
		t.Fatalf("drain requests = %#v, want only the request for node-b", task.Spec.DrainRequests)
	}
}

func TestDeleteNodeRejectsWhenNotCordoned(t *testing.T) {
	// Agent 仍连接的 Node 若 unschedulable=false,应在 tasks 检查之前就被 cordon 检查拦截。
	client := clientset.NewSimpleClientset(
		&v1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
			Spec:       v1alpha1.NodeSpec{Unschedulable: false},
			Status: v1alpha1.NodeStatus{Conditions: []metav1.Condition{
				{Type: v1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionTrue},
			}},
		},
	)
	g := &Gateway{kubeClient: client, accessors: registerAccessors(client)}

	c, recorder := newAdminNodeContext(t, http.MethodDelete, "/rlinf.io/v1alpha1/nodes/node-a?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.rlinfv1alpha1DeleteNode(c)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", recorder.Code, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "not cordoned") {
		t.Fatalf("body = %s, want 'not cordoned' in message", body)
	}
	if _, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{}); err != nil {
		t.Fatalf("node must survive rejected delete: %v", err)
	}
}

func TestDeleteNodeAllowsUnconnectedNode(t *testing.T) {
	// Agent 未连接的 Node(AgentConnected 非 True)会被控制器判定为 Offline 且不可调度,
	// 取消纳管不再要求先 cordon,否则离线节点将永远无法取消纳管。
	client := clientset.NewSimpleClientset(
		&v1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
			Spec:       v1alpha1.NodeSpec{Unschedulable: false},
			Status: v1alpha1.NodeStatus{
				Phase: v1alpha1.NodeOffline,
				Conditions: []metav1.Condition{
					{Type: v1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionFalse},
				},
			},
		},
	)
	g := &Gateway{kubeClient: client, accessors: registerAccessors(client)}

	c, recorder := newAdminNodeContext(t, http.MethodDelete, "/rlinf.io/v1alpha1/nodes/node-a?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.rlinfv1alpha1DeleteNode(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	if _, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{}); err == nil {
		t.Fatal("node must be deleted")
	}
}

// --- recoverNode ---

func TestRecoverNodeClearsDrainAndUnschedulable(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec: v1alpha1.NodeSpec{
			Unschedulable: true,
			Operations: []v1alpha1.NodeOperationRequest{
				{ID: "drain-1", Type: v1alpha1.NodeOperationDrain},
			},
		},
	})
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/node-a/recover?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.recoverNode(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}

	got, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Unschedulable {
		t.Fatalf("Unschedulable = true, want false")
	}
	for _, op := range got.Spec.Operations {
		if op.Type == v1alpha1.NodeOperationDrain {
			t.Fatalf("Drain operation %q still present after recover", op.ID)
		}
	}
}

func TestRecoverNodeWithOnlyUnschedulable(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec:       v1alpha1.NodeSpec{Unschedulable: true},
	})
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/node-a/recover?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.recoverNode(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}

	got, err := client.RlinfV1alpha1().Nodes("ns-1").Get(context.Background(), "node-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Unschedulable {
		t.Fatalf("Unschedulable = true, want false")
	}
	if len(got.Spec.Operations) != 0 {
		t.Fatalf("Operations = %#v, want empty", got.Spec.Operations)
	}
}

func TestRecoverNodeShortCircuitsWhenHealthy(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec:       v1alpha1.NodeSpec{Unschedulable: false},
	})
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/node-a/recover?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.recoverNode(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
}

func TestRecoverNodeResponseShape(t *testing.T) {
	client := clientset.NewSimpleClientset(&v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-a", Namespace: "ns-1"},
		Spec: v1alpha1.NodeSpec{
			Unschedulable: true,
			Operations: []v1alpha1.NodeOperationRequest{
				{ID: "drain-1", Type: v1alpha1.NodeOperationDrain},
			},
		},
	})
	g := &Gateway{kubeClient: client}

	c, recorder := newAdminNodeContext(t, http.MethodPost, "/rlinf.io/v1alpha1/nodes/node-a/recover?namespace=ns-1")
	c.Params = gin.Params{{Key: "name", Value: "node-a"}}

	g.recoverNode(c)

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["success"] != true {
		t.Fatalf("success = %v, want true", body["success"])
	}
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("data type = %T, want object", body["data"])
	}
	spec, _ := data["spec"].(map[string]any)
	if spec == nil {
		t.Fatalf("response lacks spec: %s", recorder.Body.String())
	}
	if spec["unschedulable"] == true {
		t.Fatalf("spec.unschedulable = true, want false")
	}
}

func TestClearNodeDrainStopsForJobReleasesOnlyOwnedTasks(t *testing.T) {
	job := &v1alpha1.Job{ObjectMeta: metav1.ObjectMeta{Name: "job-a", UID: "job-uid"}}
	owned := &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-a-worker",
			Namespace: "agent-a",
			Labels:    map[string]string{taskJobLabel: job.Name},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "Job",
				Name:       job.Name,
				UID:        job.UID,
				Controller: ptrBool(true),
			}},
			Annotations: map[string]string{
				v1alpha1.NodeDrainStoppedAnnotation:   "node-a",
				v1alpha1.NodeDrainSuspendedAnnotation: "node-a",
			},
		},
		Spec: v1alpha1.TaskSpec{Suspended: true},
	}
	otherJobTask := &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-b-worker",
			Namespace: "agent-a",
			Labels:    map[string]string{taskJobLabel: "job-b"},
			Annotations: map[string]string{
				v1alpha1.NodeDrainStoppedAnnotation:   "node-a",
				v1alpha1.NodeDrainSuspendedAnnotation: "node-a",
			},
		},
		Spec: v1alpha1.TaskSpec{Suspended: true},
	}
	client := clientset.NewSimpleClientset(owned, otherJobTask)
	g := &Gateway{kubeClient: client}

	stopped, err := g.jobHasNodeDrainStops(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !stopped {
		t.Fatal("jobHasNodeDrainStops = false, want true")
	}

	if err := g.clearNodeDrainStopsForJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	gotOwned, err := client.RlinfV1alpha1().Tasks("agent-a").Get(context.Background(), "job-a-worker", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if gotOwned.Spec.Suspended {
		t.Fatal("owned Task still suspended, want resumed by the Job start")
	}
	if _, ok := gotOwned.Annotations[v1alpha1.NodeDrainStoppedAnnotation]; ok {
		t.Fatalf("node drain marker left behind: %#v", gotOwned.Annotations)
	}
	if _, ok := gotOwned.Annotations[v1alpha1.NodeDrainSuspendedAnnotation]; ok {
		t.Fatalf("node drain suspension marker left behind: %#v", gotOwned.Annotations)
	}

	gotOther, err := client.RlinfV1alpha1().Tasks("agent-a").Get(context.Background(), "job-b-worker", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !gotOther.Spec.Suspended {
		t.Fatal("a Task owned by another Job must stay suspended")
	}
	if gotOther.Annotations[v1alpha1.NodeDrainStoppedAnnotation] != "node-a" {
		t.Fatalf("a Task owned by another Job lost its node drain marker: %#v", gotOther.Annotations)
	}
}
