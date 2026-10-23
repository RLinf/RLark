package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

func TestPrepareJobForCreateGeneratesResourceName(t *testing.T) {
	body := []byte(`{"apiVersion":"rlinf.io/v1alpha1","kind":"Job","metadata":{"name":"Readable Job"}}`)

	prepared, err := prepareJobForCreate(body)
	if err != nil {
		t.Fatal(err)
	}

	var job rlarkiov1alpha1.Job
	if err := json.Unmarshal(prepared, &job); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^jo-[0-9a-f]{16}$`).MatchString(job.Name) {
		t.Fatalf("generated job name %q does not match expected format", job.Name)
	}
	if got := job.Annotations[jobDisplayNameAnnotation]; got != "Readable Job" {
		t.Fatalf("display name = %q, want %q", got, "Readable Job")
	}
}

func TestSetSuspendedFalseResumesJob(t *testing.T) {
	job := &rlarkiov1alpha1.Job{
		Spec: rlarkiov1alpha1.JobSpec{
			Suspended: true,
		},
	}

	setSuspendedFalse(job)

	if job.Spec.Suspended {
		t.Fatal("Job remained suspended")
	}
}

func TestPrepareJobForCreatePreservesDisplayNameAnnotation(t *testing.T) {
	body := []byte(`{"metadata":{"name":"client-name","annotations":{"rlark.io/display-name":"Training Run"}}}`)

	prepared, err := prepareJobForCreate(body)
	if err != nil {
		t.Fatal(err)
	}

	var job rlarkiov1alpha1.Job
	if err := json.Unmarshal(prepared, &job); err != nil {
		t.Fatal(err)
	}
	if got := job.Annotations[jobDisplayNameAnnotation]; got != "Training Run" {
		t.Fatalf("display name = %q, want %q", got, "Training Run")
	}
}

func TestPrepareJobForCreateRejectsLongDisplayName(t *testing.T) {
	body := []byte(`{"metadata":{"name":"client-name","annotations":{"rlark.io/display-name":"123456789012345678901234567890123456789012345678901"}}}`)

	if _, err := prepareJobForCreate(body); err == nil {
		t.Fatal("expected error for a job name longer than 50 characters")
	}
}

func TestPrepareJobForCreatePreservesFrontendJobRequest(t *testing.T) {
	body := []byte(`{
		"apiVersion":"rlinf.io/v1alpha1",
		"kind":"Job",
		"metadata":{"name":"jo-7bdbef36d2624b03","annotations":{"rlark.io/display-name":"tagtest"}},
		"spec":{
			"domain":"lky-test-domain",
			"tags":[{"key":"user","values":["tang","yanhan"]},{"key":"usage","values":["test","tag"]}],
			"tasks":[{
				"name":"actor","head":true,"role":"Actor",
				"placement":{"nodeSelector":{"kubernetes.io/hostname":"hgx-049"}},
				"runtime":{"kind":"Kubernetes"},
				"workload":{"kind":"rlinf.io/ray","version":"v1","parameters":{"role":"head","prepare":{"inline":"sleep 600"},"run":{"inline":"python train.py"}},"carrier":{"type":"rlinf.io/kubernetes-statefulset","version":"v1","parameters":{"replicas":1}},"template":{"spec":{"containers":[{"name":"main","image":"example.com/rlark:latest","env":[{"name":"RLARK_TASK_ROLE","value":"Actor"}],"resources":{"requests":{},"limits":{}}}],"volumes":[]}}}
			}]
		}
	}`)

	prepared, err := prepareJobForCreate(body)
	if err != nil {
		t.Fatal(err)
	}

	var job rlarkiov1alpha1.Job
	if err := json.Unmarshal(prepared, &job); err != nil {
		t.Fatal(err)
	}
	if job.Spec.Domain != "lky-test-domain" {
		t.Fatalf("domain = %q, want lky-test-domain", job.Spec.Domain)
	}
	if len(job.Spec.Tags) != 2 || job.Spec.Tags[0].Key != "user" || len(job.Spec.Tags[0].Values) != 2 {
		t.Fatalf("tags = %#v, want frontend tags preserved", job.Spec.Tags)
	}
	if len(job.Spec.Tasks) != 1 {
		t.Fatalf("tasks = %#v, want one task", job.Spec.Tasks)
	}
	task := job.Spec.Tasks[0]
	if task.Runtime.Kind != rlarkiov1alpha1.RuntimeKindKubernetes || task.Role != rlarkiov1alpha1.TaskRoleActor {
		t.Fatalf("task = %#v, want Kubernetes Actor task", task)
	}
	if task.Workload.Kind != "rlinf.io/ray" || task.Workload.Carrier.Type != "rlinf.io/kubernetes-statefulset" {
		t.Fatalf("workload = %#v, want Ray on StatefulSet", task.Workload)
	}
	var carrier struct {
		Replicas int32 `json:"replicas"`
	}
	if err := json.Unmarshal(task.Workload.Carrier.Parameters.Raw, &carrier); err != nil || carrier.Replicas != 1 {
		t.Fatalf("carrier parameters = %s, error = %v", task.Workload.Carrier.Parameters.Raw, err)
	}
	if got := job.Annotations[jobDisplayNameAnnotation]; got != "tagtest" {
		t.Fatalf("display name = %q, want tagtest", got)
	}
}

func kubeListOptionsForQuery(t *testing.T, query string) (metav1.ListOptions, error) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/probe?"+query, nil)
	return parseKubeListOptions(ctx)
}

func TestHandleListNodesFiltersNamespacesNotOwnedByClusters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
			Spec:       rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-cluster-a"},
		},
		&rlarkiov1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "owned-node", Namespace: "agent-cluster-a"}},
		&rlarkiov1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "orphan-node", Namespace: "rlark-poc"}},
	)
	g := &Gateway{kubeClient: client, accessors: registerAccessors(client)}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/nodes", nil)
	g.handleListNodes(ctx)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result rlarkiov1alpha1.NodeList
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "owned-node" {
		t.Fatalf("items = %#v, want only owned-node", result.Items)
	}
}

func TestHandleListNodesExcludesAnExplicitOrphanNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "orphan-node", Namespace: "rlark-poc"}},
	)
	g := &Gateway{kubeClient: client, accessors: registerAccessors(client)}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/nodes?namespace=rlark-poc", nil)
	g.handleListNodes(ctx)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result rlarkiov1alpha1.NodeList
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Items == nil || len(result.Items) != 0 {
		t.Fatalf("items = %#v, want a non-nil empty Node list", result.Items)
	}
	var envelope map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if _, ok := envelope["metadata"]; ok {
		t.Fatalf("response metadata = %#v, want no envelope metadata", envelope["metadata"])
	}
}

func TestHandleListNodesPassesBoundedLimitToNamespaces(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}, Spec: rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"}},
		&rlarkiov1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "agent-a"}},
	)
	var gotLimit int64
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		nodeLister: nodeListerFunc(func(_ context.Context, _ string, opts metav1.ListOptions) (*rlarkiov1alpha1.NodeList, error) {
			gotLimit = opts.Limit
			return &rlarkiov1alpha1.NodeList{Items: []rlarkiov1alpha1.Node{{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "agent-a"}}}}, nil
		}),
	}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/nodes?limit=1", nil)
	g.handleListNodes(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if gotLimit != 2 {
		t.Fatalf("downstream limit = %d, want 2", gotLimit)
	}
}

func TestHandleListNodesFiltersBeforePagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}, Spec: rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"}},
		&rlarkiov1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "orphan"}},
		&rlarkiov1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "agent-a"}},
		&rlarkiov1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "agent-a"}},
	)
	g := &Gateway{kubeClient: client, accessors: registerAccessors(client)}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/nodes?limit=1", nil)
	g.handleListNodes(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("first page status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var first rlarkiov1alpha1.NodeList
	if err := json.Unmarshal(response.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].Name != "a" || first.Continue == "" {
		t.Fatalf("first page = %#v, want Node a and continue", first)
	}

	response = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/nodes?limit=1&continue="+first.Continue, nil)
	g.handleListNodes(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("second page status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var second rlarkiov1alpha1.NodeList
	if err := json.Unmarshal(response.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].Name != "b" || second.Continue != "" {
		t.Fatalf("second page = %#v, want final Node b", second)
	}
}

func TestHandleListNodesPaginatesAcrossLimitedNamespaceLists(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}, Spec: rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"}},
		&rlarkiov1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-b"}, Spec: rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-b"}},
	)
	nodesByNamespace := map[string][]rlarkiov1alpha1.Node{
		"agent-a": {
			{ObjectMeta: metav1.ObjectMeta{Name: "a1", Namespace: "agent-a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "a2", Namespace: "agent-a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "a3", Namespace: "agent-a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "a4", Namespace: "agent-a"}},
			{ObjectMeta: metav1.ObjectMeta{Name: "a5", Namespace: "agent-a"}},
		},
		"agent-b": {{ObjectMeta: metav1.ObjectMeta{Name: "b1", Namespace: "agent-b"}}},
	}
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		nodeLister: nodeListerFunc(func(_ context.Context, namespace string, opts metav1.ListOptions) (*rlarkiov1alpha1.NodeList, error) {
			start := 0
			if opts.Continue != "" {
				var err error
				start, err = strconv.Atoi(opts.Continue)
				if err != nil {
					return nil, err
				}
			}
			end := min(start+int(opts.Limit), len(nodesByNamespace[namespace]))
			result := &rlarkiov1alpha1.NodeList{Items: nodesByNamespace[namespace][start:end]}
			if end < len(nodesByNamespace[namespace]) {
				result.Continue = strconv.Itoa(end)
			}
			return result, nil
		}),
	}

	var names []string
	continueToken := ""
	for {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/nodes?limit=2&continue="+continueToken, nil)
		g.handleListNodes(ctx)
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
		}
		var page rlarkiov1alpha1.NodeList
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Items {
			names = append(names, item.Name)
		}
		if page.Continue == "" {
			break
		}
		continueToken = page.Continue
	}
	if want := []string{"a1", "a2", "a3", "a4", "a5", "b1"}; !slices.Equal(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

// A single page must not read every Node of every selected namespace: the
// aggregation stops as soon as it holds one item more than the requested limit.
func TestHandleListNodesStopsReadingOnceLimitIsReached(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}, Spec: rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"}},
		&rlarkiov1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-b"}, Spec: rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-b"}},
	)
	nodesByNamespace := map[string][]rlarkiov1alpha1.Node{"agent-a": {}, "agent-b": {}}
	for i := range 100 {
		nodesByNamespace["agent-a"] = append(nodesByNamespace["agent-a"], rlarkiov1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("a%03d", i), Namespace: "agent-a"},
		})
		nodesByNamespace["agent-b"] = append(nodesByNamespace["agent-b"], rlarkiov1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("b%03d", i), Namespace: "agent-b"},
		})
	}
	var listCalls int
	queried := map[string]int{}
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		nodeLister: nodeListerFunc(func(_ context.Context, namespace string, opts metav1.ListOptions) (*rlarkiov1alpha1.NodeList, error) {
			listCalls++
			queried[namespace]++
			start := 0
			if opts.Continue != "" {
				var err error
				start, err = strconv.Atoi(opts.Continue)
				if err != nil {
					return nil, err
				}
			}
			end := min(start+int(opts.Limit), len(nodesByNamespace[namespace]))
			result := &rlarkiov1alpha1.NodeList{Items: nodesByNamespace[namespace][start:end]}
			if end < len(nodesByNamespace[namespace]) {
				result.Continue = strconv.Itoa(end)
			}
			return result, nil
		}),
	}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/nodes?limit=5", nil)
	g.handleListNodes(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var page rlarkiov1alpha1.NodeList
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 5 || page.Items[0].Name != "a000" || page.Items[4].Name != "a004" {
		t.Fatalf("items = %v, want a000..a004", page.Items)
	}
	if page.Continue == "" {
		t.Fatal("continue token is empty, want more pages")
	}
	if listCalls != 1 {
		t.Fatalf("list calls = %d, want a single bounded request", listCalls)
	}
	if queried["agent-b"] != 0 {
		t.Fatalf("agent-b was listed %d times, want 0 before the limit is exhausted", queried["agent-b"])
	}
}

func TestHandleListNodesDeduplicatesDuplicateAgentNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"}, Spec: rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"}},
		&rlarkiov1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster-b"}, Spec: rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"}},
		&rlarkiov1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Name: "owned-node", Namespace: "agent-a"}},
	)
	g := &Gateway{kubeClient: client, accessors: registerAccessors(client)}

	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/nodes", nil)
	g.handleListNodes(ctx)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	var result rlarkiov1alpha1.NodeList
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "owned-node" {
		t.Fatalf("items = %#v, want owned-node", result.Items)
	}
}

func TestParseKubeListOptionsBoundsAnOmittedLimit(t *testing.T) {
	opts, err := kubeListOptionsForQuery(t, "")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Limit != maxListLimit {
		t.Fatalf("limit = %d, want the bounded default %d", opts.Limit, maxListLimit)
	}
}

func TestParseKubeListOptionsKeepsAnExplicitLimit(t *testing.T) {
	opts, err := kubeListOptionsForQuery(t, "limit=2")
	if err != nil {
		t.Fatal(err)
	}
	if opts.Limit != 2 {
		t.Fatalf("limit = %d, want 2", opts.Limit)
	}
}

func TestParseKubeListOptionsRejectsUnusableLimit(t *testing.T) {
	for _, query := range []string{"limit=abc", "limit=0", "limit=-1", "limit=501"} {
		if _, err := kubeListOptionsForQuery(t, query); err == nil {
			t.Fatalf("query %q was accepted, want rejection", query)
		}
	}
}

func TestParseKubeListOptionsRejectsOffset(t *testing.T) {
	// The management API has no offset. Silently ignoring it would return the
	// first page while the caller believes it skipped ahead.
	for _, query := range []string{"offset=1", "limit=1&offset=1", "offset=0"} {
		_, err := kubeListOptionsForQuery(t, query)
		var invalid *invalidQueryParam
		if !errors.As(err, &invalid) {
			t.Fatalf("query %q error = %v, want an invalid query parameter", query, err)
		}
		if invalid.Param != "offset" {
			t.Fatalf("query %q rejected parameter %q, want offset", query, invalid.Param)
		}
	}
}

func TestHandleListJobTagsKubeRefusesForeignTenantFilter(t *testing.T) {
	// The Kubernetes fallback aggregates the same vocabulary as the database
	// path, so it has to refuse a filter pointing outside the caller scope the
	// same way a Job list does.
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/probe?tenant=team-b", nil)
	ctx.Set(authSubjectKey, "user")

	g := &Gateway{
		accessors: registerAccessors(clientset.NewSimpleClientset()),
		users:     stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{"team-a"}}},
	}
	g.handleListJobTagsKube(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
}

func TestHandleListJobTagsKubeRestrictsVocabularyToCallerScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/probe", nil)
	ctx.Set(authSubjectKey, "user")

	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "mine", Namespace: "ns", Labels: map[string]string{tenant.IDLabel: "team-a"}},
			Spec:       rlarkiov1alpha1.JobSpec{Tags: []rlarkiov1alpha1.JobTag{{Key: "owner", Values: []string{"team-a"}}}},
		},
		&rlarkiov1alpha1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "theirs", Namespace: "ns", Labels: map[string]string{tenant.IDLabel: "team-b"}},
			Spec:       rlarkiov1alpha1.JobSpec{Tags: []rlarkiov1alpha1.JobTag{{Key: "secret", Values: []string{"team-b"}}}},
		},
	)
	g := &Gateway{
		accessors: registerAccessors(client),
		users:     stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{"team-a"}}},
	}
	g.handleListJobTagsKube(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var payload struct {
		Items []jobTag `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Items) != 1 || payload.Items[0].Key != "owner" {
		t.Fatalf("tags = %#v, want only the tags of the caller tenant", payload.Items)
	}
}

func TestWithListEnvelopeReportsTheAppliedWindow(t *testing.T) {
	normalized := map[string]any{
		"items":    []any{map[string]any{"metadata": map[string]any{"name": "a"}}},
		"metadata": map[string]any{"continue": "token", "remainingItemCount": float64(4)},
	}

	root, ok := withListEnvelope(normalized, maxListLimit).(map[string]any)
	if !ok {
		t.Fatalf("envelope type = %T, want map", withListEnvelope(normalized, maxListLimit))
	}
	if root["limit"] != maxListLimit {
		t.Fatalf("limit = %v, want %d", root["limit"], maxListLimit)
	}
	if root["hasMore"] != true {
		t.Fatalf("hasMore = %v, want true", root["hasMore"])
	}
	if root["total"] != 5 {
		t.Fatalf("total = %v, want 5", root["total"])
	}
}

// The typed client clears TypeMeta, so a Kubernetes-backed single-object read
// would answer without apiVersion and kind while the database-backed read keeps
// them. Both must report the same shape.
func TestWithRestoredTypeMetaStampsApiVersionAndKind(t *testing.T) {
	domain := &rlarkiov1alpha1.Domain{ObjectMeta: metav1.ObjectMeta{Name: "dom"}}

	restored, ok := withRestoredTypeMeta(domain).(*rlarkiov1alpha1.Domain)
	if !ok {
		t.Fatalf("restored value = %T, want *Domain", withRestoredTypeMeta(domain))
	}
	if restored.APIVersion != "rlinf.io/v1alpha1" {
		t.Errorf("apiVersion = %q, want rlinf.io/v1alpha1", restored.APIVersion)
	}
	if restored.Kind != "Domain" {
		t.Errorf("kind = %q, want Domain", restored.Kind)
	}

	// The accessor may hand back a pointer other readers share, so the original
	// object must not be stamped in place.
	if domain.Kind != "" || domain.APIVersion != "" {
		t.Errorf("source object was mutated: %+v", domain.TypeMeta)
	}

	body, err := json.Marshal(normalizedTenantObject(restored))
	if err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatal(err)
	}
	if response["kind"] != "Domain" || response["apiVersion"] != "rlinf.io/v1alpha1" {
		t.Errorf("response = %s, want apiVersion and kind present", body)
	}
}

// handleGetKube has to restore TypeMeta inside the tenant normalization:
// normalizedTenantObject turns its argument into a plain map and
// withRestoredTypeMeta only recognizes a runtime.Object, so a swapped nesting
// order would silently answer without a kind again. Asserting both the kind and
// the normalized tenant label on the handler response is what pins the order
// down; the unit tests above pass either way.
func TestHandleGetKubeReportsKindAndTheNormalizedTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/probe", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "legacy"}}
	ctx.Set(authSubjectKey, "user")

	// A Job created before tenant labelling, read by a non-admin caller: the
	// scope has to admit it as the default tenant, so the response passes
	// through the tenant authorization branch and not an admin shortcut.
	client := clientset.NewSimpleClientset(&rlarkiov1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "legacy"},
	})
	g := &Gateway{
		accessors: registerAccessors(client),
		users:     stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
	}
	g.handleGetKube(ctx, "jobs")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	var response struct {
		APIVersion string `json:"apiVersion"`
		Kind       string `json:"kind"`
		Metadata   struct {
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if want := rlarkiov1alpha1.GroupVersion.String(); response.APIVersion != want || response.Kind != "Job" {
		t.Errorf("apiVersion/kind = %q/%q, want %q/Job", response.APIVersion, response.Kind, want)
	}
	if got := response.Metadata.Labels[tenant.IDLabel]; got != tenant.DefaultID {
		t.Errorf("tenant label = %q, want %q", got, tenant.DefaultID)
	}
}

func TestWithRestoredTypeMetaKeepsExistingKindAndUnknownValues(t *testing.T) {
	pod := &rlarkiov1alpha1.Pod{
		TypeMeta:   metav1.TypeMeta{APIVersion: "rlinf.io/v1alpha1", Kind: "Pod"},
		ObjectMeta: metav1.ObjectMeta{Name: "p1"},
	}
	if restored := withRestoredTypeMeta(pod); restored != any(pod) {
		t.Error("an object that already carries its kind must be returned untouched")
	}

	value := map[string]any{"kind": "Domain"}
	if restored := withRestoredTypeMeta(value); !reflect.DeepEqual(restored, value) {
		t.Errorf("restored = %#v, want the non-Kubernetes value unchanged", restored)
	}
}
