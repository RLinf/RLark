package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/persistence"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

func listOptionsForQuery(t *testing.T, query url.Values) (db.ListOptions, error) {
	t.Helper()

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/probe?"+query.Encode(), nil)
	g := &Gateway{}
	return g.parseListOptions(ctx)
}

func TestParseListOptionsCanonicalizesOrderTerms(t *testing.T) {
	// The store only recognizes "field" or "field asc|desc". Forwarding the raw
	// text would let "name  desc" reach the store with an unparsed direction and
	// sort the other way round.
	cases := map[string][]string{
		"name":                  {"name"},
		"name desc":             {"name desc"},
		"name  desc":            {"name desc"},
		"name DESC":             {"name desc"},
		"created_at DESC,name":  {"created_at desc", "name"},
		" name ,  uid   asc   ": {"name", "uid asc"},
	}
	for order, want := range cases {
		opts, err := listOptionsForQuery(t, url.Values{"order": {order}})
		if err != nil {
			t.Fatalf("order=%q: %v", order, err)
		}
		if len(opts.OrderBy) != len(want) {
			t.Fatalf("order=%q produced %v, want %v", order, opts.OrderBy, want)
		}
		for i := range want {
			if opts.OrderBy[i] != want[i] {
				t.Fatalf("order=%q produced %v, want %v", order, opts.OrderBy, want)
			}
		}
	}
}

func TestParseListOptionsRejectsUnusableOrder(t *testing.T) {
	for _, order := range []string{
		"name;drop",
		"name sideways",
		"raw #>> '{a}'",
	} {
		if _, err := listOptionsForQuery(t, url.Values{"order": {order}}); err == nil {
			t.Fatalf("order=%q was accepted, want rejection", order)
		}
	}
}

func TestParseListOptionsRejectsUnusableSelectorPaths(t *testing.T) {
	// The store binds selector paths as arguments, so a hostile path can no
	// longer change the statement. It is still refused at the edge so that a
	// caller gets 400 instead of a result set it cannot explain.
	payload := `x}' IS NULL OR 'a` + "=v"
	for _, param := range []string{"fieldSelector", "labelSelector"} {
		_, err := listOptionsForQuery(t, url.Values{param: {payload}})
		var invalid *invalidQueryParam
		if !errors.As(err, &invalid) {
			t.Fatalf("%s=%q error = %v, want an invalid query parameter", param, payload, err)
		}
		if invalid.Param != param {
			t.Fatalf("%s=%q rejected parameter %q, want %q", param, payload, invalid.Param, param)
		}
	}
}

func TestParseListOptionsKeepsValidSelectorPaths(t *testing.T) {
	opts, err := listOptionsForQuery(t, url.Values{
		"fieldSelector": {"status.phase=Running"},
		"labelSelector": {"rlark.io/node-category=cloud"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(opts.FieldSelector) != 1 || opts.FieldSelector[0].Path != "status.phase" {
		t.Fatalf("field selectors = %#v, want status.phase", opts.FieldSelector)
	}
	if len(opts.LabelSelector) != 1 || opts.LabelSelector[0].Key != "rlark.io/node-category" {
		t.Fatalf("label selectors = %#v, want rlark.io/node-category", opts.LabelSelector)
	}
}

func TestJobTagListOptionsNarrowsVocabularyLikeAJobList(t *testing.T) {
	// The aggregation does not go through parseListOptions, so the "tenant"
	// filter and the caller scope have to be applied here as well. Without
	// them the reported vocabulary describes a wider object set than the Job
	// list it is meant to filter.
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/probe?namespace=ns&tenant=team-a", nil)

	opts := jobTagListOptions(ctx, tenantScope{tenants: []string{"team-a", "team-b"}})
	if opts.Namespace != "ns" {
		t.Fatalf("namespace = %q, want ns", opts.Namespace)
	}
	if len(opts.LabelSelector) != 2 {
		t.Fatalf("label selectors = %#v, want the tenant filter and the scope restriction", opts.LabelSelector)
	}
	if got := opts.LabelSelector[0]; got.Key != tenant.IDLabel || got.Op != "=" || got.Value != "team-a" {
		t.Fatalf("tenant filter = %#v, want rlark.io/tenant-id=team-a", got)
	}
	if got := opts.LabelSelector[1]; got.Key != tenant.IDLabel || got.Op != "in" || got.Value != "team-a,team-b" {
		t.Fatalf("scope restriction = %#v, want an in-operator over the caller tenants", got)
	}
}

func TestJobTagListOptionsLeavesAdminAggregationUnfiltered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodGet, "/probe", nil)

	if opts := jobTagListOptions(ctx, tenantScope{admin: true}); len(opts.LabelSelector) != 0 {
		t.Fatalf("label selectors = %#v, want an unfiltered aggregation", opts.LabelSelector)
	}
}

func TestHandleListJobTagsFromPersistenceRefusesForeignTenantFilter(t *testing.T) {
	// The backend is left unusable on purpose: a refused filter must be
	// answered before any query reaches the persistence backend.
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/probe?tenant=team-b", nil)
	ctx.Set(authSubjectKey, "user")

	g := &Gateway{
		persistenceBackend: unqueriedBackend{t: t},
		users:              stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{"team-a"}}},
	}
	g.handleListJobTagsFromPersistence(ctx)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
}

// The start endpoint carries no tenant identity of its own, so it has to decide
// ownership from the stored object. Both of its branches read that object, so
// both are asserted: a caller outside the owning tenant must be answered like a
// missing object and must not change anything.
func TestHandleStartJobRefusesAnActiveJobOfAnotherTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/probe", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "theirs"}}
	ctx.Set(authSubjectKey, "user")

	client := clientset.NewSimpleClientset(&rlarkiov1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "theirs", Labels: map[string]string{tenant.IDLabel: "team-b"}},
		Spec:       rlarkiov1alpha1.JobSpec{Suspended: true},
	})
	g := &Gateway{
		kubeClient:         client,
		accessors:          registerAccessors(client),
		persistenceBackend: unqueriedBackend{t: t},
		users:              stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{"team-a"}}},
	}
	g.handleStartJob(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	job, err := client.RlinfV1alpha1().Jobs().Get(context.Background(), "theirs", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !job.Spec.Suspended {
		t.Fatal("the refused request still started the job")
	}
}

func TestHandleStartJobRefusesAnArchivedJobOfAnotherTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/probe", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "theirs"}}
	ctx.Set(authSubjectKey, "user")

	// The management API no longer holds the Job, so the handler falls through
	// to the archived snapshot, which is where the tenant label survives.
	client := clientset.NewSimpleClientset()
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		persistenceBackend: archivedJobBackend{t: t, raw: []byte(`{
			"apiVersion":"rlinf.io/v1alpha1","kind":"Job",
			"metadata":{"name":"theirs","labels":{"` + tenant.IDLabel + `":"team-b"}},
			"spec":{"domain":"dom","stopped":true}
		}`)},
		users: stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{"team-a"}}},
	}
	g.handleStartJob(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	jobs, err := client.RlinfV1alpha1().Jobs().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("restored jobs = %#v, want the refused request to restore nothing", jobs.Items)
	}
}

// blockedJobFixture builds a suspended Job whose only task template selects a
// single Node, together with the Cluster that owns that Node. The Node has a
// Schedulable=True condition and reports Kubernetes runtime capability so that
// it passes the same filters the Job controller applies.
// nodeFixture builds a Clientset with a single online Cluster and a single Node
// whose Schedulable condition is controlled by the caller. The Node also carries
// Kubernetes runtime capability. Used by tests that need to drive the node-level
// eligibility filter in validateJobPlacement.
func newSimpleClientsetWithSchedulableNode(t *testing.T, schedulable metav1.ConditionStatus) *clientset.Clientset {
	t.Helper()
	return clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
			Spec:       rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"},
			Status:     rlarkiov1alpha1.ClusterStatus{Phase: rlarkiov1alpha1.ClusterOnline},
		},
		&rlarkiov1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gpu-a",
				Namespace: "agent-a",
			},
			Status: rlarkiov1alpha1.NodeStatus{
				ObservedNodeLabels: map[string]string{"kubernetes.io/hostname": "gpu-a"},
				Conditions: []metav1.Condition{{
					Type:   rlarkiov1alpha1.ConditionTypeSchedulable,
					Status: schedulable,
				}},
				CapabilitySnapshot: rlarkiov1alpha1.NodeCapabilitySnapshot{
					Runtime: rlarkiov1alpha1.NodeRuntimeCapability{
						Type:          rlarkiov1alpha1.AgentTypeKubernetes,
						WorkloadKinds: []string{"rlinf.io/ray"},
					},
				},
			},
		},
	)
}

func blockedJobFixture(t *testing.T, phase rlarkiov1alpha1.ClusterPhase, delisted bool) (*clientset.Clientset, *Gateway) {
	t.Helper()
	schedulableTrue := metav1.ConditionTrue
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
			Spec:       rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a", Delisted: delisted},
			Status:     rlarkiov1alpha1.ClusterStatus{Phase: phase},
		},
		&rlarkiov1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gpu-a",
				Namespace: "agent-a",
			},
			Status: rlarkiov1alpha1.NodeStatus{
				ObservedNodeLabels: map[string]string{"kubernetes.io/hostname": "gpu-a"},
				Conditions: []metav1.Condition{{
					Type:   rlarkiov1alpha1.ConditionTypeSchedulable,
					Status: schedulableTrue,
				}},
				CapabilitySnapshot: rlarkiov1alpha1.NodeCapabilitySnapshot{
					Runtime: rlarkiov1alpha1.NodeRuntimeCapability{
						Type:          rlarkiov1alpha1.AgentTypeKubernetes,
						WorkloadKinds: []string{"rlinf.io/ray"},
					},
				},
			},
		},
		&rlarkiov1alpha1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "job-a", Labels: map[string]string{tenant.IDLabel: "team-a"}},
			Spec: rlarkiov1alpha1.JobSpec{
				Suspended: true,
				Tasks: []rlarkiov1alpha1.JobTaskTemplate{{
					Name: "worker",
					TaskSpec: rlarkiov1alpha1.TaskSpec{
						Runtime:  rlarkiov1alpha1.TaskRuntimeSpec{Kind: rlarkiov1alpha1.RuntimeKindKubernetes},
						Workload: rlarkiov1alpha1.TaskWorkloadSpec{Kind: "rlinf.io/ray"},
						Placement: rlarkiov1alpha1.TaskPlacementSpec{
							NodeSelector: map[string]string{"kubernetes.io/hostname": "gpu-a"},
						},
					},
				}},
			},
		},
	)
	return client, &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		users:      stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{"team-a"}}},
	}
}

// Starting a Job whose Node lives in an Offline or Delisted Cluster must fail
// immediately instead of leaving the Job Pending forever. Both reasons are
// asserted because the console exposes them as distinct states. The caller asks
// for English, so the response must carry the English wording only.
func TestHandleStartJobRejectsNodeInUnavailableOrDelistedCluster(t *testing.T) {
	cases := map[string]struct {
		phase    rlarkiov1alpha1.ClusterPhase
		delisted bool
	}{
		"offline":  {phase: rlarkiov1alpha1.ClusterOffline},
		"delisted": {phase: rlarkiov1alpha1.ClusterDelisted, delisted: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/probe", nil)
			ctx.Request.Header.Set("Accept-Language", "en")
			ctx.Params = gin.Params{{Key: "name", Value: "job-a"}}
			ctx.Set(authSubjectKey, "user")

			client, g := blockedJobFixture(t, tc.phase, tc.delisted)
			g.handleStartJob(ctx)

			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusConflict, recorder.Body.String())
			}
			body := recorder.Body.String()
			if !strings.Contains(body, "unavailable or delisted") {
				t.Fatalf("body %q does not contain the English wording", body)
			}
			if strings.Contains(body, "不可用或已下架") {
				t.Fatalf("body %q mixes Chinese into an English response", body)
			}
			job, err := client.RlinfV1alpha1().Jobs().Get(context.Background(), "job-a", metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !job.Spec.Suspended {
				t.Fatal("the rejected start still unsuspended the job")
			}
		})
	}
}

func TestHandleStartJobAllowsNodeInOnlineCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/probe", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "job-a"}}
	ctx.Set(authSubjectKey, "user")

	client, g := blockedJobFixture(t, rlarkiov1alpha1.ClusterOnline, false)
	g.handleStartJob(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	job, err := client.RlinfV1alpha1().Jobs().Get(context.Background(), "job-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if job.Spec.Suspended {
		t.Fatal("the accepted start did not unsuspend the job")
	}
}

func TestHandleStartJobClearsNodeDrainStops(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/probe", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "job-a"}}
	ctx.Set(authSubjectKey, "user")

	client, g := blockedJobFixture(t, rlarkiov1alpha1.ClusterOnline, false)
	job, err := client.RlinfV1alpha1().Jobs().Get(context.Background(), "job-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	job.UID = "job-uid"
	if _, err := client.RlinfV1alpha1().Jobs().Update(context.Background(), job, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RlinfV1alpha1().Tasks("agent-a").Create(context.Background(), &rlarkiov1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "job-a-worker",
			Namespace: "agent-a",
			Labels:    map[string]string{"rlinf.io/job": job.Name},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: rlarkiov1alpha1.GroupVersion.String(),
				Kind:       "Job",
				Name:       job.Name,
				UID:        job.UID,
				Controller: ptrBool(true),
			}},
			Annotations: map[string]string{
				rlarkiov1alpha1.NodeDrainStoppedAnnotation:   "gpu-a",
				rlarkiov1alpha1.NodeDrainSuspendedAnnotation: "gpu-a",
			},
		},
		Spec: rlarkiov1alpha1.TaskSpec{Suspended: true},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	g.handleStartJob(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	task, err := client.RlinfV1alpha1().Tasks("agent-a").Get(context.Background(), "job-a-worker", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if task.Spec.Suspended {
		t.Fatal("the accepted start did not resume the Task the node drain stopped")
	}
	if _, ok := task.Annotations[rlarkiov1alpha1.NodeDrainStoppedAnnotation]; ok {
		t.Fatalf("node drain marker left behind: %#v", task.Annotations)
	}
}

func TestHandleStartJobRejectsRestoredJobInUnavailableCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/probe", nil)
	ctx.Params = gin.Params{{Key: "name", Value: "job-a"}}
	ctx.Set(authSubjectKey, "user")

	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
			Spec:       rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"},
			Status:     rlarkiov1alpha1.ClusterStatus{Phase: rlarkiov1alpha1.ClusterOffline},
		},
		&rlarkiov1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gpu-a",
				Namespace: "agent-a",
			},
			Status: rlarkiov1alpha1.NodeStatus{
				ObservedNodeLabels: map[string]string{"kubernetes.io/hostname": "gpu-a"},
			},
		},
	)
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		persistenceBackend: archivedJobBackend{t: t, raw: []byte(`{
			"apiVersion":"rlinf.io/v1alpha1","kind":"Job",
			"metadata":{"name":"job-a","labels":{"` + tenant.IDLabel + `":"team-a"}},
			"spec":{"tasks":[{"name":"worker","placement":{"nodeSelector":{"kubernetes.io/hostname":"gpu-a"}}}]}
		}`)},
		users: stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{"team-a"}}},
	}
	g.handleStartJob(ctx)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	jobs, err := client.RlinfV1alpha1().Jobs().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs.Items) != 0 {
		t.Fatalf("restored jobs = %#v, want the rejected start to restore nothing", jobs.Items)
	}
}

// Restart reaches the server as a patch that unsuspends the Job, so it has to be
// guarded on the same boundary as POST /jobs/:name/start.
func TestHandleKubePatchJobRejectsStartOnUnavailableCluster(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/probe", strings.NewReader(`{"spec":{"suspended":false}}`))
	ctx.Params = gin.Params{{Key: "name", Value: "job-a"}}
	ctx.Set(authSubjectKey, "user")

	client, g := blockedJobFixture(t, rlarkiov1alpha1.ClusterOffline, false)
	g.handleKubePatch("jobs")(ctx)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	job, err := client.RlinfV1alpha1().Jobs().Get(context.Background(), "job-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !job.Spec.Suspended {
		t.Fatal("the rejected patch still unsuspended the job")
	}
}

// A restart patch may move the Job to a healthy Node in the same request that
// unsuspends it. The guard must validate the placement the patch produces, not
// the stale one stored on the Job, or the move cannot ever start.
func TestHandleKubePatchJobValidatesPatchedPlacement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPatch, "/probe", strings.NewReader(`{"spec":{"suspended":false,"tasks":[{"name":"worker","runtime":{"kind":"Kubernetes"},"workload":{"kind":"rlinf.io/ray"},"placement":{"nodeSelector":{"kubernetes.io/hostname":"gpu-b"}}}]}}`))
	ctx.Params = gin.Params{{Key: "name", Value: "job-a"}}
	ctx.Set(authSubjectKey, "user")

	node := func(name, namespace string) *rlarkiov1alpha1.Node {
		return &rlarkiov1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: rlarkiov1alpha1.NodeStatus{
				ObservedNodeLabels: map[string]string{"kubernetes.io/hostname": name},
				Conditions: []metav1.Condition{{
					Type:   rlarkiov1alpha1.ConditionTypeSchedulable,
					Status: metav1.ConditionTrue,
				}},
				CapabilitySnapshot: rlarkiov1alpha1.NodeCapabilitySnapshot{
					Runtime: rlarkiov1alpha1.NodeRuntimeCapability{
						Type:          rlarkiov1alpha1.AgentTypeKubernetes,
						WorkloadKinds: []string{"rlinf.io/ray"},
					},
				},
			},
		}
	}
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
			Spec:       rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"},
			Status:     rlarkiov1alpha1.ClusterStatus{Phase: rlarkiov1alpha1.ClusterOffline},
		},
		&rlarkiov1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-b"},
			Spec:       rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-b"},
			Status:     rlarkiov1alpha1.ClusterStatus{Phase: rlarkiov1alpha1.ClusterOnline},
		},
		node("gpu-a", "agent-a"),
		node("gpu-b", "agent-b"),
		&rlarkiov1alpha1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "job-a", Labels: map[string]string{tenant.IDLabel: "team-a"}},
			Spec: rlarkiov1alpha1.JobSpec{
				Suspended: true,
				Tasks: []rlarkiov1alpha1.JobTaskTemplate{{
					Name: "worker",
					TaskSpec: rlarkiov1alpha1.TaskSpec{
						Runtime:  rlarkiov1alpha1.TaskRuntimeSpec{Kind: rlarkiov1alpha1.RuntimeKindKubernetes},
						Workload: rlarkiov1alpha1.TaskWorkloadSpec{Kind: "rlinf.io/ray"},
						Placement: rlarkiov1alpha1.TaskPlacementSpec{
							NodeSelector: map[string]string{"kubernetes.io/hostname": "gpu-a"},
						},
					},
				}},
			},
		},
	)
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		users:      stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{"team-a"}}},
	}
	g.handleKubePatch("jobs")(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	job, err := client.RlinfV1alpha1().Jobs().Get(context.Background(), "job-a", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if job.Spec.Suspended {
		t.Fatal("the accepted patch did not unsuspend the job")
	}
}

// A Job created running is dispatched immediately, so an unusable placement has
// to fail the create instead of producing a Job that waits forever. Delisted and
// Offline Clusters are both covered because the console reports them separately.
func TestHandleKubeCreateJobRejectsNodeInUnavailableOrDelistedCluster(t *testing.T) {
	cases := map[string]struct {
		phase    rlarkiov1alpha1.ClusterPhase
		delisted bool
	}{
		"offline":  {phase: rlarkiov1alpha1.ClusterOffline},
		"delisted": {phase: rlarkiov1alpha1.ClusterDelisted, delisted: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			client, _ := blockedJobFixture(t, tc.phase, tc.delisted)
			g := &Gateway{
				kubeClient: client,
				accessors:  registerAccessors(client),
				users:      stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
				tenants:    tenant.StaticProvider{},
			}

			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Set(authSubjectKey, "user")
			ctx.Request = httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(`{
				"metadata":{"name":"job-a"},
				"spec":{"tasks":[{"name":"worker","runtime":{"kind":"Kubernetes"},"workload":{"kind":"rlinf.io/ray","version":"v1"},"placement":{"nodeSelector":{"kubernetes.io/hostname":"gpu-a"}}}]}
			}`))
			g.handleKubeCreate("jobs")(ctx)

			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusConflict, recorder.Body.String())
			}
			body := recorder.Body.String()
			if !strings.Contains(body, "不可用或已下架") {
				t.Fatalf("body %q does not contain the Chinese wording", body)
			}
			if strings.Contains(body, "unavailable or delisted") {
				t.Fatalf("body %q mixes English into a Chinese response", body)
			}
		})
	}
}

// A suspended Job is only a template; it must stay creatable even when the
// selected Cluster is offline, because it does not run until started.
func TestHandleKubeCreateJobAllowsSuspendedJobOnUnavailableCluster(t *testing.T) {
	client, _ := blockedJobFixture(t, rlarkiov1alpha1.ClusterOffline, false)
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		users:      stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:    tenant.StaticProvider{},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(authSubjectKey, "user")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(`{
		"metadata":{"name":"job-a"},
		"spec":{
			"suspended":true,
			"tasks":[{"name":"worker","placement":{"nodeSelector":{"kubernetes.io/hostname":"gpu-a"}}}]
		}
	}`))
	g.handleKubeCreate("jobs")(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
}

func TestHandleKubeCreateJobAllowsNodeInOnlineCluster(t *testing.T) {
	client, _ := blockedJobFixture(t, rlarkiov1alpha1.ClusterOnline, false)
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		users:      stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:    tenant.StaticProvider{},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(authSubjectKey, "user")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(`{
		"metadata":{"name":"job-a"},
		"spec":{"tasks":[{"name":"worker","runtime":{"kind":"Kubernetes"},"workload":{"kind":"rlinf.io/ray","version":"v1"},"placement":{"nodeSelector":{"kubernetes.io/hostname":"gpu-a"}}}]}
	}`))
	g.handleKubeCreate("jobs")(ctx)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
}

// TestHandleKubeCreateJobRejectsUnschedulableNode verifies that creating a
// running Job fails with 409 when the only selected Node is Schedulable=False,
// mirroring what the Job controller would never dispatch. This prevents the
// silent "Pending forever" state we saw with jo-9a8186d8ef8e4eca where both
// data-plane nodes became cordoned.
func TestHandleKubeCreateJobRejectsUnschedulableNode(t *testing.T) {
	client := newSimpleClientsetWithSchedulableNode(t, metav1.ConditionFalse)
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		users:      stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:    tenant.StaticProvider{},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(authSubjectKey, "user")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(`{
		"metadata":{"name":"job-a"},
		"spec":{"tasks":[{"name":"worker","runtime":{"kind":"Kubernetes"},"workload":{"kind":"rlinf.io/ray","version":"v1"},"placement":{"nodeSelector":{"kubernetes.io/hostname":"gpu-a"}}}]}
	}`))
	g.handleKubeCreate("jobs")(ctx)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "不可调度") {
		t.Fatalf("body %q does not contain the Chinese wording", body)
	}
	if strings.Contains(body, "unschedulable") {
		t.Fatalf("body %q mixes English into a Chinese response", body)
	}
}

// TestHandleKubeCreateJobRejectsCapabilityMismatch verifies that creating a
// running Job fails with 409 when the Node's runtime capability cannot run
// the requested workload kind (e.g. Docker node cannot run Ray).
func TestHandleKubeCreateJobRejectsCapabilityMismatch(t *testing.T) {
	schedulableTrue := metav1.ConditionTrue
	client := clientset.NewSimpleClientset(
		&rlarkiov1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-a"},
			Spec:       rlarkiov1alpha1.ClusterSpec{AgentNamespace: "agent-a"},
			Status:     rlarkiov1alpha1.ClusterStatus{Phase: rlarkiov1alpha1.ClusterOnline},
		},
		&rlarkiov1alpha1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "gpu-a",
				Namespace: "agent-a",
			},
			Status: rlarkiov1alpha1.NodeStatus{
				ObservedNodeLabels: map[string]string{"kubernetes.io/hostname": "gpu-a"},
				Conditions: []metav1.Condition{{
					Type:   rlarkiov1alpha1.ConditionTypeSchedulable,
					Status: schedulableTrue,
				}},
				CapabilitySnapshot: rlarkiov1alpha1.NodeCapabilitySnapshot{
					Runtime: rlarkiov1alpha1.NodeRuntimeCapability{
						Type:          rlarkiov1alpha1.AgentType("Docker"),
						WorkloadKinds: []string{"docker.io/basic"},
					},
				},
			},
		},
	)
	g := &Gateway{
		kubeClient: client,
		accessors:  registerAccessors(client),
		users:      stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{tenant.DefaultID}}},
		tenants:    tenant.StaticProvider{},
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set(authSubjectKey, "user")
	ctx.Request = httptest.NewRequest(http.MethodPost, "/jobs", strings.NewReader(`{
		"metadata":{"name":"job-a"},
		"spec":{"tasks":[{"name":"worker","runtime":{"kind":"Kubernetes"},"workload":{"kind":"rlinf.io/ray","version":"v1"},"placement":{"nodeSelector":{"kubernetes.io/hostname":"gpu-a"}}}]}
	}`))
	g.handleKubeCreate("jobs")(ctx)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusConflict, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "运行时能力不匹配") {
		t.Fatalf("body %q does not contain the Chinese wording", body)
	}
	if strings.Contains(body, "is not supported by node") {
		t.Fatalf("body %q mixes English into a Chinese response", body)
	}
}

// archivedJobBackend serves one archived snapshot and fails the test on every
// other backend call, so a refused restore cannot be mistaken for a successful
// one.
type archivedJobBackend struct {
	t   *testing.T
	raw []byte
}

func (b archivedJobBackend) Save(context.Context, db.ResourceModel) error {
	b.t.Fatalf("the start endpoint reached Save")
	return nil
}

func (b archivedJobBackend) Load(_ context.Context, kind, _, _ string) (*persistence.Resource, error) {
	if kind != "jobs" {
		b.t.Fatalf("Load kind = %q, want jobs", kind)
	}
	return &persistence.Resource{Raw: b.raw}, nil
}

func (b archivedJobBackend) List(context.Context, string, db.ListOptions) (*persistence.ListResult, error) {
	b.t.Fatalf("the start endpoint reached List")
	return nil, nil
}

func (b archivedJobBackend) Delete(context.Context, string, string, string) error {
	b.t.Fatalf("the start endpoint reached Delete")
	return nil
}

// unqueriedBackend fails the test when a request that must be refused still
// reaches the persistence backend.
type unqueriedBackend struct {
	t *testing.T
}

func (b unqueriedBackend) Save(context.Context, db.ResourceModel) error {
	b.t.Fatalf("a refused request reached Save")
	return nil
}

func (b unqueriedBackend) Load(context.Context, string, string, string) (*persistence.Resource, error) {
	b.t.Fatalf("a refused request reached Load")
	return nil, nil
}

func (b unqueriedBackend) List(context.Context, string, db.ListOptions) (*persistence.ListResult, error) {
	b.t.Fatalf("a refused request reached List")
	return nil, nil
}

func (b unqueriedBackend) Delete(context.Context, string, string, string) error {
	b.t.Fatalf("a refused request reached Delete")
	return nil
}
