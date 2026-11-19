package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// stubUserProvider resolves a single caller, so that the tenant scope can be
// exercised without an authentication secret.
type stubUserProvider struct {
	caller *user
	err    error
}

func (p stubUserProvider) List(context.Context) ([]user, error) { return nil, p.err }

func (p stubUserProvider) Get(_ context.Context, id string) (*user, error) {
	if p.err != nil {
		return nil, p.err
	}
	if p.caller == nil || p.caller.ID != id {
		return nil, fmt.Errorf("user %q not found", id)
	}
	return p.caller, nil
}

func (p stubUserProvider) Authenticate(context.Context, string, string) (*user, error) {
	return p.caller, p.err
}

func TestTenantScopeAllowsReadsMissingTenantAsDefault(t *testing.T) {
	member := tenantScope{tenants: []string{tenant.DefaultID}}
	if !member.allows("") {
		t.Fatal("an object without a tenant label must belong to the default tenant")
	}
	if member.allows("other") {
		t.Fatal("allows() accepted a tenant outside the scope")
	}
	if !(tenantScope{admin: true}).allows("other") {
		t.Fatal("an administrator must not be restricted")
	}
	// An empty scope must deny, so that a failure to resolve membership cannot
	// widen access to the default tenant.
	if (tenantScope{}).allows("") {
		t.Fatal("an empty scope must not allow the default tenant")
	}
}

func TestTenantScopeWriteTenantFallsBackToDefault(t *testing.T) {
	if got := (tenantScope{tenants: []string{"team-a", "team-b"}}).writeTenant(); got != "team-a" {
		t.Fatalf("writeTenant() = %q, want %q", got, "team-a")
	}
	if got := (tenantScope{admin: true}).writeTenant(); got != tenant.DefaultID {
		t.Fatalf("writeTenant() = %q, want %q", got, tenant.DefaultID)
	}
}

func TestTenantScopeRestrictListOptions(t *testing.T) {
	opts := db.ListOptions{}
	(tenantScope{admin: true}).restrictListOptions(&opts)
	if len(opts.LabelSelector) != 0 {
		t.Fatalf("an administrator list must stay unfiltered, got %#v", opts.LabelSelector)
	}

	(tenantScope{tenants: []string{"team-a", "team-b"}}).restrictListOptions(&opts)
	if len(opts.LabelSelector) != 1 {
		t.Fatalf("LabelSelector = %#v, want one entry", opts.LabelSelector)
	}
	selector := opts.LabelSelector[0]
	if selector.Key != tenant.IDLabel || selector.Op != "in" || selector.Value != "team-a,team-b" {
		t.Fatalf("unexpected selector: %#v", selector)
	}
}

func TestTenantScopeRestrictKubeListDropsForeignObjects(t *testing.T) {
	list := &rlarkiov1alpha1.JobList{Items: []rlarkiov1alpha1.Job{
		{ObjectMeta: metav1.ObjectMeta{Name: "mine", Labels: map[string]string{tenant.IDLabel: "team-a"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "theirs", Labels: map[string]string{tenant.IDLabel: "team-b"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "legacy"}},
	}}

	scope := tenantScope{tenants: []string{"team-a", tenant.DefaultID}}
	if err := scope.restrictKubeList(list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 2 || list.Items[0].Name != "mine" || list.Items[1].Name != "legacy" {
		t.Fatalf("unexpected items: %#v", list.Items)
	}
}

func TestTenantScopeRestrictKubeListKeepsAdminListIntact(t *testing.T) {
	list := &rlarkiov1alpha1.JobList{Items: []rlarkiov1alpha1.Job{
		{ObjectMeta: metav1.ObjectMeta{Name: "theirs", Labels: map[string]string{tenant.IDLabel: "team-b"}}},
	}}
	if err := (tenantScope{admin: true}).restrictKubeList(list); err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 {
		t.Fatalf("unexpected items: %#v", list.Items)
	}
}

func TestTenantScopeRestrictKubeListRejectsNonList(t *testing.T) {
	if err := (tenantScope{tenants: []string{"team-a"}}).restrictKubeList(map[string]any{}); err == nil {
		t.Fatal("restrictKubeList() accepted a value that is not a list object")
	}
}

func TestIsTenantScopedResourceCoversDerivedPods(t *testing.T) {
	for _, resource := range []string{"workflows", "jobs", "tasks", "pods"} {
		if !isTenantScopedResource(resource) {
			t.Fatalf("%s must be tenant scoped", resource)
		}
	}
	for _, resource := range []string{"nodes", "domains", "clusters"} {
		if isTenantScopedResource(resource) {
			t.Fatalf("%s is infrastructure and must stay outside the tenant scope", resource)
		}
	}
}

func TestRequestedTenantIDDistinguishesAbsentFromExplicit(t *testing.T) {
	if got := requestedTenantID([]byte(`{"metadata":{"labels":{}}}`)); got != "" {
		t.Fatalf("requestedTenantID() = %q, want an empty string for an absent label", got)
	}
	if got := requestedTenantID([]byte(`{"metadata":{"labels":{"rlark.io/tenant-id":"team-a"}}}`)); got != "team-a" {
		t.Fatalf("requestedTenantID() = %q, want %q", got, "team-a")
	}
	if got := requestedTenantID([]byte("not json")); got != "" {
		t.Fatalf("requestedTenantID() = %q, want an empty string for an unreadable body", got)
	}
}

func TestTenantIDOfMapDefaultsForLegacyObject(t *testing.T) {
	if got := tenantIDOfMap(map[string]any{"metadata": map[string]any{"name": "legacy"}}); got != tenant.DefaultID {
		t.Fatalf("tenantIDOfMap() = %q, want %q", got, tenant.DefaultID)
	}
	object := map[string]any{"metadata": map[string]any{"labels": map[string]any{tenant.IDLabel: "team-a"}}}
	if got := tenantIDOfMap(object); got != "team-a" {
		t.Fatalf("tenantIDOfMap() = %q, want %q", got, "team-a")
	}
}

func TestAuthorizeTenantQueryRefusesForeignTenantFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/?tenant=team-b", nil)

	if authorizeTenantQuery(c, tenantScope{tenants: []string{"team-a"}}) {
		t.Fatal("authorizeTenantQuery() accepted a tenant outside the scope")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestAuthorizeTenantObjectReportsForeignObjectAsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	if authorizeTenantObject(c, tenantScope{tenants: []string{"team-a"}}, "team-b") {
		t.Fatal("authorizeTenantObject() accepted an object owned by another tenant")
	}
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestResolveTenantScopeRejectsCallerWithoutTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(authSubjectKey, "user")

	g := &Gateway{users: stubUserProvider{caller: &user{ID: "user"}}}
	if _, ok := g.tenantScopeFor(c, "jobs"); ok {
		t.Fatal("tenantScopeFor() accepted a caller without a tenant assignment")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestResolveTenantScopeCachesResultPerRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(authSubjectKey, "user")

	g := &Gateway{users: stubUserProvider{caller: &user{ID: "user", TenantIDs: []string{"team-a"}}}}
	scope, err := g.resolveTenantScope(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.tenants) != 1 || scope.tenants[0] != "team-a" {
		t.Fatalf("unexpected scope: %#v", scope)
	}

	// A second resolution must not consult the provider again, so a provider
	// that now fails still yields the cached scope.
	g.users = stubUserProvider{err: context.DeadlineExceeded}
	cached, err := g.resolveTenantScope(c)
	if err != nil {
		t.Fatalf("resolveTenantScope() error = %v, want the cached scope", err)
	}
	if len(cached.tenants) != 1 || cached.tenants[0] != "team-a" {
		t.Fatalf("unexpected cached scope: %#v", cached)
	}
}

func TestResolveTenantScopeSkipsProviderForAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c.Set(authRoleKey, roleAdmin)

	g := &Gateway{users: stubUserProvider{err: context.DeadlineExceeded}}
	scope, err := g.resolveTenantScope(c)
	if err != nil {
		t.Fatal(err)
	}
	if !scope.admin {
		t.Fatalf("unexpected scope: %#v", scope)
	}
}

func TestTenantScopeForUnscopedResourceSkipsResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	g := &Gateway{users: stubUserProvider{err: context.DeadlineExceeded}}
	scope, ok := g.tenantScopeFor(c, "nodes")
	if !ok || !scope.admin {
		t.Fatalf("tenantScopeFor(nodes) = %#v, %v; want an unrestricted scope", scope, ok)
	}
}
