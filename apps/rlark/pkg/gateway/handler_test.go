package gateway

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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
