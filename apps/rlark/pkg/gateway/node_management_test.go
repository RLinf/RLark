package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1fake "k8s.io/client-go/kubernetes/fake"

	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	v1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// newGatewayWithMockTransport 构造 Gateway 并搭建模拟的代理链路:
//
//	Gateway -> httptest.Server ("server", handleProxy) -> httptest.Server ("agent", /api/kubernetes/*) -> data-plane fake K8s
//
// 这样可以验证 Gateway 的 proxyKubeRequestWithHeaders 是否正确地把 PATCH body 和 Content-Type 透传。
func newGatewayWithMockTransport(
	t *testing.T,
	clusters []v1alpha1.Cluster,
	agentNodes []corev1.Node,
) (*Gateway, *corev1fake.Clientset) {
	t.Helper()
	ctx := context.Background()

	// --- data-plane fake K8s 提供原生 K8s Node ---
	dataPlane := corev1fake.NewSimpleClientset()
	for _, n := range agentNodes {
		cp := n
		if _, err := dataPlane.CoreV1().Nodes().Create(ctx, &cp, metav1.CreateOptions{}); err != nil {
			t.Fatalf("seed data-plane node %s: %v", n.Name, err)
		}
	}

	// --- 模拟 data-plane agent HTTP server (/api/kubernetes/*) ---
	agentMux := http.NewServeMux()
	agentMux.HandleFunc("/api/kubernetes/", func(w http.ResponseWriter, r *http.Request) {
		kubePath := strings.TrimPrefix(r.URL.Path, "/api/kubernetes")
		t.Logf("[agent] %s %s", r.Method, kubePath)

		switch {
		case r.Method == http.MethodGet && kubePath == "/api/v1/nodes":
			list, _ := dataPlane.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
			_ = json.NewEncoder(w).Encode(list)
		case r.Method == http.MethodPatch && strings.HasPrefix(kubePath, "/api/v1/nodes/"):
			name := strings.TrimPrefix(kubePath, "/api/v1/nodes/")
			body, _ := io.ReadAll(r.Body)
			pt := types.PatchType(r.Header.Get("Content-Type"))
			switch pt {
			case "application/merge-patch+json":
				pt = types.MergePatchType
			case "application/strategic-merge-patch+json":
				pt = types.StrategicMergePatchType
			}
			t.Logf("[agent] PATCH node=%s content-type=%s body=%s", name, r.Header.Get("Content-Type"), string(body))
			node, err := dataPlane.CoreV1().Nodes().Patch(ctx, name, pt, body, metav1.PatchOptions{})
			if err != nil {
				http.Error(w, fmt.Sprintf("PATCH failed: %v", err), http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(node)
		default:
			http.Error(w, fmt.Sprintf("unhandled: %s %s", r.Method, kubePath), http.StatusNotFound)
		}
	})
	agentSrv := httptest.NewTLSServer(agentMux)
	t.Cleanup(agentSrv.Close)

	// --- 模拟 rlark-server (/api/proxy/{agentID}/api/kubernetes/*) ---
	serverMux := http.NewServeMux()
	serverMux.HandleFunc("/api/proxy/", func(w http.ResponseWriter, r *http.Request) {
		// /api/proxy/{agentID}/api/kubernetes/{kubePath}
		rest := strings.TrimPrefix(r.URL.Path, "/api/proxy/")
		idx := strings.Index(rest, "/")
		if idx < 0 {
			http.Error(w, "bad path", http.StatusBadRequest)
			return
		}
		kubePath := rest[idx:] // /api/kubernetes/...
		caPool := x509.NewCertPool()
		caPool.AddCert(agentSrv.Certificate())
		out, _ := http.NewRequestWithContext(r.Context(), r.Method, agentSrv.URL+kubePath, r.Body)
		for k, vv := range r.Header {
			for _, v := range vv {
				out.Header.Add(k, v)
			}
		}
		resp, err := (&http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: caPool}}}).Do(out)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
	})
	serverSrv := httptest.NewTLSServer(serverMux)
	t.Cleanup(serverSrv.Close)

	// --- RLark control plane fake k8s (提供 Cluster 列表) ---
	rlarkCs := clientset.NewSimpleClientset()
	for _, c := range clusters {
		cp := c
		if _, err := rlarkCs.RlinfV1alpha1().Clusters().Create(ctx, &cp, metav1.CreateOptions{}); err != nil {
			t.Fatalf("seed cluster %s: %v", c.Name, err)
		}
	}

	// --- 构造 Gateway ---
	caPool := x509.NewCertPool()
	caPool.AddCert(serverSrv.Certificate())
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: caPool}}

	g := &Gateway{
		config:          Config{ServerAddress: serverSrv.URL},
		kubeClient:      rlarkCs,
		serverTransport: transport,
	}
	return g, dataPlane
}

func newAdminGinContext(method, url string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, url, nil)
	c.Set(authRoleKey, "admin")
	return c, w
}

// TestUnmanageRemovesLabelEndToEnd 验证完整的 Gateway -> 代理 -> data-plane K8s 链路:
//  1. unmanageDataPlaneNode 通过 proxyKubeRequestWithHeaders 发出 PATCH body 和 Content-Type
//  2. 模拟的 server + agent 正确转发
//  3. data-plane fake K8s 删除 label
//  4. fetchDataPlaneNodes 再次 list 时 managed 状态反映为 false
func TestUnmanageRemovesLabelEndToEnd(t *testing.T) {
	ctx := context.Background()
	agentID := "cluster-a"

	clusters := []v1alpha1.Cluster{{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       v1alpha1.ClusterSpec{AgentNamespace: "rlark-" + agentID},
	}}
	agentNodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{
			Name:   "node-managed",
			Labels: map[string]string{"rlark.io/enabled": "true", "hostname": "node-managed"},
		}},
		{ObjectMeta: metav1.ObjectMeta{
			Name:   "node-raw",
			Labels: map[string]string{"hostname": "node-raw"},
		}},
	}

	g, dataPlane := newGatewayWithMockTransport(t, clusters, agentNodes)

	// 初始 list
	c, w := newAdminGinContext(http.MethodGet, "/api/v1/data-plane-nodes")
	c.Request = c.Request.WithContext(ctx)
	g.listDataPlaneNodes(c)
	if w.Code != http.StatusOK {
		t.Fatalf("initial list: status=%d body=%s", w.Code, w.Body.String())
	}
	var initResp struct{ Clusters []clusterDataPlaneNodes }
	if err := json.Unmarshal(w.Body.Bytes(), &initResp); err != nil {
		t.Fatalf("decode initial response: %v", err)
	}
	t.Logf("initial: %+v", initResp.Clusters)

	var initManaged int
	for _, cl := range initResp.Clusters {
		for _, n := range cl.Nodes {
			if n.Managed {
				initManaged++
			}
		}
	}
	if initManaged != 1 {
		t.Fatalf("initial managed count = %d, want 1", initManaged)
	}

	// 注销
	u, uw := newAdminGinContext(http.MethodPost,
		fmt.Sprintf("/api/v1/data-plane-nodes/%s/nodes/node-managed/unmanage", agentID))
	u.Request = u.Request.WithContext(ctx)
	u.Params = gin.Params{{Key: "agent_id", Value: agentID}, {Key: "node_name", Value: "node-managed"}}
	g.unmanageDataPlaneNode(u)
	t.Logf("unmanage: status=%d body=%s", uw.Code, uw.Body.String())
	if uw.Code != http.StatusOK {
		t.Fatalf("unmanage failed: status=%d body=%s", uw.Code, uw.Body.String())
	}

	// 再次 list
	c2, w2 := newAdminGinContext(http.MethodGet, "/api/v1/data-plane-nodes")
	c2.Request = c2.Request.WithContext(ctx)
	g.listDataPlaneNodes(c2)
	if w2.Code != http.StatusOK {
		t.Fatalf("after list: status=%d body=%s", w2.Code, w2.Body.String())
	}
	var afterResp struct{ Clusters []clusterDataPlaneNodes }
	if err := json.Unmarshal(w2.Body.Bytes(), &afterResp); err != nil {
		t.Fatalf("decode after response: %v", err)
	}
	t.Logf("after unmanage: %+v", afterResp.Clusters)

	var managedAfter int
	for _, cl := range afterResp.Clusters {
		for _, n := range cl.Nodes {
			if n.Managed {
				managedAfter++
				t.Errorf("node %s still shows managed=true after unmanage (labels=%v)", n.Name, n.Labels)
			}
		}
	}
	if managedAfter != 0 {
		t.Fatalf("after unmanage managed count = %d, want 0", managedAfter)
	}

	// 直接查 data-plane fake K8s 确认 label 被删
	node, err := dataPlane.CoreV1().Nodes().Get(ctx, "node-managed", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get data-plane node: %v", err)
	}
	t.Logf("direct check data-plane labels: %v", node.Labels)
	if _, ok := node.Labels["rlark.io/enabled"]; ok {
		t.Fatal("rlark.io/enabled label still present on data-plane node after unmanage - PATCH did not reach K8s")
	}
}

// TestUnmanageRejectsWhenTasksObserved 验证数据面 unmanage 在节点仍观测到任务时返回 409,
// 且不会向 data-plane K8s 发出删除 label 的 PATCH。
func TestUnmanageRejectsWhenTasksObserved(t *testing.T) {
	ctx := context.Background()
	agentID := "cluster-a"

	clusters := []v1alpha1.Cluster{{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       v1alpha1.ClusterSpec{AgentNamespace: "rlark-" + agentID},
	}}
	agentNodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{
			Name:   "node-managed",
			Labels: map[string]string{"rlark.io/enabled": "true", "hostname": "node-managed"},
		}},
	}

	g, dataPlane := newGatewayWithMockTransport(t, clusters, agentNodes)

	// 管理面 agent 命名空间中存在一个观测到该节点的 Task
	if _, err := g.kubeClient.RlinfV1alpha1().Tasks("rlark-"+agentID).Create(ctx, &v1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task-1", Namespace: "rlark-" + agentID},
		Status:     v1alpha1.TaskStatus{ObservedNodes: []string{"node-managed"}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed task: %v", err)
	}

	u, uw := newAdminGinContext(http.MethodPost,
		fmt.Sprintf("/api/v1/data-plane-nodes/%s/nodes/node-managed/unmanage", agentID))
	u.Request = u.Request.WithContext(ctx)
	u.Params = gin.Params{{Key: "agent_id", Value: agentID}, {Key: "node_name", Value: "node-managed"}}
	g.unmanageDataPlaneNode(u)
	t.Logf("unmanage: status=%d body=%s", uw.Code, uw.Body.String())

	if uw.Code != http.StatusConflict {
		t.Fatalf("unmanage: status=%d, want 409: %s", uw.Code, uw.Body.String())
	}
	if !strings.Contains(uw.Body.String(), "task-1") {
		t.Fatalf("body = %s, want task name in message", uw.Body.String())
	}

	// data-plane label 必须仍然存在
	node, err := dataPlane.CoreV1().Nodes().Get(ctx, "node-managed", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get data-plane node: %v", err)
	}
	if node.Labels["rlark.io/enabled"] != "true" {
		t.Fatalf("label removed despite rejection: labels=%v", node.Labels)
	}
}

// TestUnmanageRejectsWhenNotCordoned 验证 data-plane unmanage 在管理面 Node CR 存在、
// Agent 仍连接但 unschedulable=false 时返回 409。
func TestUnmanageRejectsWhenNotCordoned(t *testing.T) {
	ctx := context.Background()
	agentID := "cluster-a"

	clusters := []v1alpha1.Cluster{{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       v1alpha1.ClusterSpec{AgentNamespace: "rlark-" + agentID},
	}}
	agentNodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{
			Name:   "node-managed",
			Labels: map[string]string{"rlark.io/enabled": "true", "hostname": "node-managed"},
		}},
	}

	g, dataPlane := newGatewayWithMockTransport(t, clusters, agentNodes)

	// 创建管理面 Node CR, Agent 已连接且 unschedulable=false
	if _, err := g.kubeClient.RlinfV1alpha1().Nodes("rlark-"+agentID).Create(ctx, &v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-managed", Namespace: "rlark-" + agentID},
		Spec:       v1alpha1.NodeSpec{Unschedulable: false},
		Status: v1alpha1.NodeStatus{Conditions: []metav1.Condition{
			{Type: v1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionTrue},
		}},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed management node: %v", err)
	}

	u, uw := newAdminGinContext(http.MethodPost,
		fmt.Sprintf("/api/v1/data-plane-nodes/%s/nodes/node-managed/unmanage", agentID))
	u.Request = u.Request.WithContext(ctx)
	u.Params = gin.Params{{Key: "agent_id", Value: agentID}, {Key: "node_name", Value: "node-managed"}}
	g.unmanageDataPlaneNode(u)
	t.Logf("unmanage: status=%d body=%s", uw.Code, uw.Body.String())

	if uw.Code != http.StatusConflict {
		t.Fatalf("unmanage: status=%d, want 409: %s", uw.Code, uw.Body.String())
	}
	if !strings.Contains(uw.Body.String(), "not cordoned") {
		t.Fatalf("body = %s, want 'not cordoned' in message", uw.Body.String())
	}

	// data-plane label 必须仍然存在
	node, err := dataPlane.CoreV1().Nodes().Get(ctx, "node-managed", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get data-plane node: %v", err)
	}
	if node.Labels["rlark.io/enabled"] != "true" {
		t.Fatalf("label removed despite rejection: labels=%v", node.Labels)
	}
}

// TestUnmanageAllowsUnconnectedNode 验证 Agent 离线(AgentConnected 非 True)的节点
// 无需先 cordon 即可取消纳管;这类节点已被控制器判定为 Offline 且不可调度。
func TestUnmanageAllowsUnconnectedNode(t *testing.T) {
	ctx := context.Background()
	agentID := "cluster-a"

	clusters := []v1alpha1.Cluster{{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
		Spec:       v1alpha1.ClusterSpec{AgentNamespace: "rlark-" + agentID},
	}}
	agentNodes := []corev1.Node{
		{ObjectMeta: metav1.ObjectMeta{
			Name:   "node-offline",
			Labels: map[string]string{"rlark.io/enabled": "true", "hostname": "node-offline"},
		}},
	}

	g, dataPlane := newGatewayWithMockTransport(t, clusters, agentNodes)

	if _, err := g.kubeClient.RlinfV1alpha1().Nodes("rlark-"+agentID).Create(ctx, &v1alpha1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-offline", Namespace: "rlark-" + agentID},
		Spec:       v1alpha1.NodeSpec{Unschedulable: false},
		Status: v1alpha1.NodeStatus{
			Phase: v1alpha1.NodeOffline,
			Conditions: []metav1.Condition{
				{Type: v1alpha1.ConditionTypeAgentConnected, Status: metav1.ConditionFalse},
			},
		},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed management node: %v", err)
	}

	u, uw := newAdminGinContext(http.MethodPost,
		fmt.Sprintf("/api/v1/data-plane-nodes/%s/nodes/node-offline/unmanage", agentID))
	u.Request = u.Request.WithContext(ctx)
	u.Params = gin.Params{{Key: "agent_id", Value: agentID}, {Key: "node_name", Value: "node-offline"}}
	g.unmanageDataPlaneNode(u)
	t.Logf("unmanage: status=%d body=%s", uw.Code, uw.Body.String())

	if uw.Code != http.StatusOK {
		t.Fatalf("unmanage: status=%d, want 200: %s", uw.Code, uw.Body.String())
	}

	node, err := dataPlane.CoreV1().Nodes().Get(ctx, "node-offline", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get data-plane node: %v", err)
	}
	if _, ok := node.Labels["rlark.io/enabled"]; ok {
		t.Fatalf("label must be removed: labels=%v", node.Labels)
	}
}
