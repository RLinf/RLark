package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// roleAdmin is the privileged role carried by the authentication token. It is
// compared against the value the JWT middleware stored under authRoleKey.
const roleAdmin = "admin"

// tenantScopeContextKey caches the resolved scope for the current request so
// that a handler touching several code paths resolves the caller identity once.
const tenantScopeContextKey = "rlark.tenantScope"

// errNoTenantAssignment reports an authenticated caller that is not a member of
// any tenant. Such a caller is refused instead of being silently treated as a
// member of the default tenant.
var errNoTenantAssignment = errors.New("user has no tenant assignment")

// errUserProviderUnavailable reports that the server cannot resolve the caller
// identity. The request is refused because an unresolvable scope cannot be
// distinguished from an empty scope.
var errUserProviderUnavailable = errors.New("user provider is not configured")

// tenantScope is the server side answer to "which tenants may this caller read
// and write". It is derived from the authenticated identity only: a tenant
// carried by the request body or the query string is never trusted, because
// client supplied filtering is not an authorization boundary.
type tenantScope struct {
	// admin callers are not restricted to a tenant subset.
	admin bool
	// tenants lists the tenant identifiers a non-admin caller may access. It
	// always holds at least one entry for a successfully resolved scope.
	tenants []string
}

// allows reports whether the caller may access an object owned by tenantID. An
// empty tenantID means the object predates tenant labelling and belongs to the
// default tenant, matching tenant.IDOf.
func (s tenantScope) allows(tenantID string) bool {
	if s.admin {
		return true
	}
	if tenantID == "" {
		tenantID = tenant.DefaultID
	}
	for _, candidate := range s.tenants {
		if candidate == tenantID {
			return true
		}
	}
	return false
}

// writeTenant returns the tenant that owns objects created by this caller.
func (s tenantScope) writeTenant() string {
	if len(s.tenants) > 0 {
		return s.tenants[0]
	}
	return tenant.DefaultID
}

// restrictListOptions narrows a database list query to the caller scope. The
// restriction is applied as an additional selector so that it is combined with
// any client supplied selector instead of replacing it, and so that paging
// counters stay consistent with the rows the caller is allowed to see.
func (s tenantScope) restrictListOptions(opts *db.ListOptions) {
	if s.admin || opts == nil {
		return
	}
	opts.LabelSelector = append(opts.LabelSelector, db.LabelSelector{
		Key: tenant.IDLabel, Op: "in", Value: strings.Join(s.tenants, ","),
	})
}

// restrictKubeList removes objects outside the caller scope from a management
// API list. The management API cannot express the scope as a label selector,
// because an object without the tenant label belongs to the default tenant and
// a label selector cannot match a missing label and a value set at once.
//
// Filtering here means the reported paging window no longer describes the
// returned set, exactly as for the resource ID filter: "limit" and the continue
// token still refer to the window requested from the management API.
func (s tenantScope) restrictKubeList(list any) error {
	if s.admin {
		return nil
	}
	object, ok := list.(runtime.Object)
	if !ok {
		return fmt.Errorf("tenant scope expects a runtime.Object, got %T", list)
	}
	items, err := meta.ExtractList(object)
	if err != nil {
		return err
	}
	kept := make([]runtime.Object, 0, len(items))
	for _, item := range items {
		accessor, err := meta.Accessor(item)
		if err != nil {
			return err
		}
		if s.allows(tenant.IDOf(accessor)) {
			kept = append(kept, item)
		}
	}
	return meta.SetList(object, kept)
}

// tenantIDOfObject reports the tenant that owns a management API object.
func tenantIDOfObject(object any) (string, error) {
	accessor, err := meta.Accessor(object)
	if err != nil {
		return "", err
	}
	return tenant.IDOf(accessor), nil
}

// isTenantScopedResource reports whether reads and deletes of a resource must
// be restricted to the caller tenant.
//
// It is deliberately wider than isTenantResource: the gateway stamps tenant
// identity only on the resources a user creates through it, while a Pod is
// stamped by the agent that derives it from a Task. Node and Domain are
// infrastructure objects that carry no tenant identity and whose write paths
// are already restricted to administrators, so they stay outside the tenant
// scope.
func isTenantScopedResource(resource string) bool {
	switch resource {
	case "workflows", "jobs", "tasks", "pods":
		return true
	}
	return false
}

// resolveTenantScope determines the caller scope from the authenticated
// identity. The result is cached on the request context.
func (g *Gateway) resolveTenantScope(c *gin.Context) (tenantScope, error) {
	if cached, ok := c.Get(tenantScopeContextKey); ok {
		if scope, ok := cached.(tenantScope); ok {
			return scope, nil
		}
	}

	if c.GetString(authRoleKey) == roleAdmin {
		scope := tenantScope{admin: true}
		c.Set(tenantScopeContextKey, scope)
		return scope, nil
	}

	// The scope is resolved per request rather than carried in the token, so
	// that a tenant membership change takes effect immediately instead of
	// after the token expires.
	if g.users == nil && g.rawClient == nil {
		return tenantScope{}, errUserProviderUnavailable
	}
	subject := c.GetString(authSubjectKey)
	caller, err := g.userProvider().Get(c.Request.Context(), subject)
	if err != nil {
		return tenantScope{}, fmt.Errorf("resolve tenant scope for %q: %w", subject, err)
	}

	scope := tenantScope{}
	for _, id := range caller.TenantIDs {
		if id = strings.TrimSpace(id); id != "" {
			scope.tenants = append(scope.tenants, id)
		}
	}
	if len(scope.tenants) == 0 {
		return tenantScope{}, errNoTenantAssignment
	}
	c.Set(tenantScopeContextKey, scope)
	return scope, nil
}

// tenantScopeFor resolves the scope for a resource and writes the error
// response itself. It reports false when the request must not continue.
//
// Resources outside the tenant scope yield an unrestricted scope so that
// callers can apply this helper uniformly.
func (g *Gateway) tenantScopeFor(c *gin.Context, resource string) (tenantScope, bool) {
	if !isTenantScopedResource(resource) {
		return tenantScope{admin: true}, true
	}
	scope, err := g.resolveTenantScope(c)
	if err != nil {
		respondTenantScopeError(c, err)
		return tenantScope{}, false
	}
	return scope, true
}

// respondTenantScopeError maps a scope resolution failure onto the error
// contract. A caller without a tenant is forbidden; an unresolvable identity is
// a server side configuration problem and must not degrade into an
// unrestricted read.
func respondTenantScopeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errNoTenantAssignment):
		respondError(c, http.StatusForbidden, CodeForbidden, "user has no tenant assignment")
	case errors.Is(err, errUserProviderUnavailable):
		respondError(c, http.StatusInternalServerError, CodeNotConfigured, "user provider is not configured")
	default:
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to resolve tenant scope")
	}
}

// authorizeTenantQuery rejects an explicit "tenant" filter pointing outside the
// caller scope and writes the error response itself.
//
// The request is refused rather than narrowed, so that a client can tell a
// refused filter apart from a filter that matched nothing.
func authorizeTenantQuery(c *gin.Context, scope tenantScope) bool {
	requested := strings.TrimSpace(c.Query("tenant"))
	if requested == "" || scope.allows(requested) {
		return true
	}
	respondErrorWithDetails(c, http.StatusForbidden, CodeForbidden, "tenant is outside the caller scope", map[string]any{
		"parameter": "tenant",
		"tenant":    requested,
	})
	return false
}

// authorizeTenantObject checks that the caller may act on an object owned by
// tenantID and writes the error response itself.
//
// An object owned by another tenant is reported as not found rather than
// forbidden: a 403 would confirm that a name exists in a tenant the caller
// cannot see.
func authorizeTenantObject(c *gin.Context, scope tenantScope, tenantID string) bool {
	if scope.allows(tenantID) {
		return true
	}
	respondError(c, http.StatusNotFound, CodeResourceNotFound, "not found")
	return false
}

// authorizeTenantObjectByName checks that the caller may act on the existing
// object named by the request, by reading the stored object instead of trusting
// the request payload: a patch, a delete, a log query or a proxied connection
// carries no tenant identity of its own.
//
// It writes the error response itself and reports false when the request must
// not continue. An administrator is allowed without the extra read.
func (g *Gateway) authorizeTenantObjectByName(c *gin.Context, resource, namespace, name string) bool {
	if !isTenantScopedResource(resource) {
		return true
	}
	scope, ok := g.tenantScopeFor(c, resource)
	if !ok {
		return false
	}
	if scope.admin {
		return true
	}
	a, ok := g.accessors[resource]
	if !ok {
		respondError(c, http.StatusInternalServerError, CodeNotConfigured, "resource not configured")
		return false
	}
	existing, err := a.get(c.Request.Context(), namespace, name, metav1.GetOptions{})
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return false
	}
	tenantID, err := tenantIDOfObject(existing)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to inspect object metadata")
		return false
	}
	return authorizeTenantObject(c, scope, tenantID)
}

// requestedTenantID reads the tenant a create request asks for. An absent label
// yields an empty string, which must stay distinguishable from an explicit
// "default" so that the caller scope can supply the tenant instead.
func requestedTenantID(body []byte) string {
	var object map[string]any
	if json.Unmarshal(body, &object) != nil {
		return ""
	}
	metadata, _ := object["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	value, _ := labels[tenant.IDLabel].(string)
	return strings.TrimSpace(value)
}

// tenantIDOfMap reads the tenant identity from a decoded JSON object, applying
// the same default as tenant.IDOf for objects created before tenant labelling.
func tenantIDOfMap(object map[string]any) string {
	metadata, _ := object["metadata"].(map[string]any)
	labels, _ := metadata["labels"].(map[string]any)
	if value, _ := labels[tenant.IDLabel].(string); value != "" {
		return value
	}
	return tenant.DefaultID
}
