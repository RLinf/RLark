package gateway

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.yaml.in/yaml/v3"
)

// loadSwaggerData reads the hand written OpenAPI document. The tests below check
// cross-references in it, which the Go compiler cannot see.
func loadSwaggerData(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../docs/api/swagger.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSwaggerOperationsAreRegisteredRoutes(t *testing.T) {
	data := loadSwaggerData(t)

	var document struct {
		Paths map[string]map[string]struct {
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}

	registered := registeredRoutes(t)
	// /metrics is served by the metrics listener, not by RegisterRoutes.
	registered["GET /metrics"] = struct{}{}

	for path, pathItem := range document.Paths {
		for method, operation := range pathItem {
			method = strings.ToUpper(method)
			if _, ok := registered[method+" "+path]; !ok {
				t.Errorf("OpenAPI operation is not registered by Gateway.RegisterRoutes: %s %s", method, path)
			}
			if _, ok := operation.Responses["401"]; ok {
				t.Errorf("OpenAPI operation declares unsupported 401 response: %s %s", method, path)
			}
		}
	}
}

// loadSwaggerPaths returns the "METHOD /path" keys described by the OpenAPI
// document, with paths left in OpenAPI form ("{name}" placeholders).
func loadSwaggerPaths(t *testing.T) map[string]struct{} {
	t.Helper()

	var document struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(loadSwaggerData(t), &document); err != nil {
		t.Fatal(err)
	}

	operations := map[string]struct{}{}
	for path, pathItem := range document.Paths {
		for method := range pathItem {
			operations[strings.ToUpper(method)+" "+path] = struct{}{}
		}
	}
	return operations
}

// registeredRoutes returns the "METHOD /path" keys that Gateway.RegisterRoutes
// installs, with gin placeholders rewritten to OpenAPI form.
func registeredRoutes(t *testing.T) map[string]struct{} {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&Gateway{}).RegisterRoutes(router)

	routes := map[string]struct{}{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+openAPIPath(route.Path)] = struct{}{}
	}
	return routes
}

// routesUndocumentedInSwagger lists the routes that the hand written OpenAPI
// document does not describe yet. It is a shrink-only baseline: a newly added
// route must either be documented or be added here deliberately, and
// TestRegisteredRoutesAreDocumented fails when an entry becomes obsolete, so
// the list cannot silently drift away from the router.
//
// Do not add entries without documenting why. Removing entries by documenting
// the route in docs/api/swagger.yaml is the intended direction of travel.
var routesUndocumentedInSwagger = map[string]struct{}{
	"POST /api/v1/auth/login":                           {},
	"GET /api/v1/api-reference":                         {},
	"GET /api/v1/users/me":                              {},
	"GET /api/v1/users/{id}":                            {},
	"GET /api/v1/users":                                 {},
	"GET /api/v1/tenants":                               {},
	"GET /api/v1/tenants/{id}":                          {},
	"GET /api/v1/rlinf.io/v1alpha1/workflows":           {},
	"POST /api/v1/rlinf.io/v1alpha1/workflows":          {},
	"GET /api/v1/rlinf.io/v1alpha1/workflows/{name}":    {},
	"PUT /api/v1/rlinf.io/v1alpha1/workflows/{name}":    {},
	"PATCH /api/v1/rlinf.io/v1alpha1/workflows/{name}":  {},
	"DELETE /api/v1/rlinf.io/v1alpha1/workflows/{name}": {},
	"GET /api/v1/rlinf.io/v1alpha1/jobs/tags":           {},
	"GET /api/v1/rlinf.io/v1alpha1/pods":                {},
	"GET /api/v1/rlinf.io/v1alpha1/pods/{name}":         {},
	"PATCH /api/v1/rlinf.io/v1alpha1/pods/{name}":       {},
	"GET /api/v1/rlinf.io/v1alpha1/pods/{name}/events":  {},
	// Websocket upgrade, not expressible as an OpenAPI 3.0 operation.
	"GET /api/v1/rlinf.io/v1alpha1/pods/{name}/terminal": {},
	"GET /api/v1/rlinf.io/v1alpha1/domains":              {},
	"POST /api/v1/rlinf.io/v1alpha1/domains":             {},
	"GET /api/v1/rlinf.io/v1alpha1/domains/{name}":       {},
	"PUT /api/v1/rlinf.io/v1alpha1/domains/{name}":       {},
	"PATCH /api/v1/rlinf.io/v1alpha1/domains/{name}":     {},
	"DELETE /api/v1/rlinf.io/v1alpha1/domains/{name}":    {},
	// Opaque reverse proxy to the TensorBoard sidecar, registered with Any().
	"GET /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}":     {},
	"POST /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}":    {},
	"PUT /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}":     {},
	"PATCH /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}":   {},
	"DELETE /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}":  {},
	"HEAD /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}":    {},
	"OPTIONS /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}": {},
	"CONNECT /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}": {},
	"TRACE /api/v1/rlinf.io/v1alpha1/tasks/{name}/tensorboard/{path}":   {},
	"GET /api/v1/certificates/agent":                                    {},
	"GET /api/v1/certificates/agent/{cluster_id}":                       {},
	"POST /api/v1/certificates/agent":                                   {},
	"POST /api/v1/certificates/revoke":                                  {},
	"GET /api/v1/ssh-user-keys":                                         {},
	"POST /api/v1/ssh-user-keys":                                        {},
	"DELETE /api/v1/ssh-user-keys/{id}":                                 {},
	"GET /api/v1/image-registries":                                      {},
	"POST /api/v1/image-registries":                                     {},
	"GET /api/v1/image-registries/{id}":                                 {},
	"PUT /api/v1/image-registries/{id}":                                 {},
	"DELETE /api/v1/image-registries/{id}":                              {},
	"GET /api/v1/system-config":                                         {},
	"PUT /api/v1/system-config":                                         {},
	"GET /api/v1/storage/storageclass":                                  {},
	"POST /api/v1/storage/storageclass":                                 {},
	"PUT /api/v1/storage/storageclass/{name}":                           {},
	"DELETE /api/v1/storage/storageclass/{name}":                        {},
	"GET /api/v1/storage/storageclass/provider":                         {},
	"GET /api/v1/storage/storageclass/{name}/{cluster}/list":            {},
	"POST /api/v1/storage/storageclass/{name}/{cluster}/upload":         {},
	"GET /api/v1/storage/storageclass/{name}/{cluster}/object/{key}":    {},
	"DELETE /api/v1/storage/storageclass/{name}/{cluster}/object/{key}": {},
	"GET /api/v1/addons":                                                {},
	"GET /api/v1/addons/{name}":                                         {},
	"GET /api/v1/installed-addons":                                      {},
	"GET /api/v1/clusters/{cluster_id}/addons":                          {},
	"POST /api/v1/clusters/{cluster_id}/addons":                         {},
	"GET /api/v1/clusters/{cluster_id}/addons/{name}":                   {},
	"PUT /api/v1/clusters/{cluster_id}/addons/{name}":                   {},
	"DELETE /api/v1/clusters/{cluster_id}/addons/{name}":                {},
}

// TestRegisteredRoutesAreDocumented is the route -> spec direction of the
// OpenAPI consistency check. TestSwaggerOperationsAreRegisteredRoutes only
// catches operations that describe a route which does not exist; without this
// test a new endpoint could ship with no documented request, response or error
// contract at all.
func TestRegisteredRoutesAreDocumented(t *testing.T) {
	documented := loadSwaggerPaths(t)
	routes := registeredRoutes(t)

	unused := make(map[string]struct{}, len(routesUndocumentedInSwagger))
	for route := range routesUndocumentedInSwagger {
		unused[route] = struct{}{}
	}

	undocumented := make([]string, 0)
	for route := range routes {
		_, exempt := routesUndocumentedInSwagger[route]
		delete(unused, route)
		if _, ok := documented[route]; ok {
			if exempt {
				t.Errorf("route %s is documented now; remove it from routesUndocumentedInSwagger", route)
			}
			continue
		}
		if !exempt {
			undocumented = append(undocumented, route)
		}
	}
	sort.Strings(undocumented)
	for _, route := range undocumented {
		t.Errorf("route is not described by docs/api/swagger.yaml: %s", route)
	}

	obsolete := make([]string, 0, len(unused))
	for route := range unused {
		obsolete = append(obsolete, route)
	}
	sort.Strings(obsolete)
	for _, route := range obsolete {
		t.Errorf("routesUndocumentedInSwagger lists %s, which Gateway.RegisterRoutes no longer installs", route)
	}
}

var swaggerSchemaRefPattern = regexp.MustCompile(`#/components/schemas/([A-Za-z0-9_.-]+)`)

// TestSwaggerSchemaReferencesResolve guards the hand written spec against a
// dangling $ref. A broken reference is invisible to the Go compiler and to the
// route check, so it would otherwise only surface when a client generates code
// from the document.
func TestSwaggerSchemaReferencesResolve(t *testing.T) {
	data := loadSwaggerData(t)

	var document struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}

	missing := map[string]struct{}{}
	for _, match := range swaggerSchemaRefPattern.FindAllStringSubmatch(string(data), -1) {
		name := match[1]
		if _, ok := document.Components.Schemas[name]; !ok {
			missing[name] = struct{}{}
		}
	}
	if len(missing) == 0 {
		return
	}
	names := make([]string, 0, len(missing))
	for name := range missing {
		names = append(names, name)
	}
	sort.Strings(names)
	t.Errorf("swagger.yaml references undefined schemas: %s", strings.Join(names, ", "))
}

// TestSwaggerErrorCodeEnumMatchesConstants keeps the documented error-code enum
// in step with the constants that are actually returned, so that a new code
// cannot ship undocumented and a removed one cannot linger in the spec.
func TestSwaggerErrorCodeEnumMatchesConstants(t *testing.T) {
	data := loadSwaggerData(t)

	var document struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `yaml:"enum"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}

	errorResponse, ok := document.Components.Schemas["ErrorResponse"]
	if !ok {
		t.Fatal("swagger.yaml does not define the ErrorResponse schema")
	}
	codeProperty, ok := errorResponse.Properties["code"]
	if !ok {
		t.Fatal("ErrorResponse schema does not define the code property")
	}

	documented := map[string]struct{}{}
	for _, code := range codeProperty.Enum {
		documented[code] = struct{}{}
	}
	declared := []string{
		CodeInvalidQueryParameter,
		CodeInvalidRequestBody,
		CodeInvalidRequest,
		CodeUnauthorized,
		CodeForbidden,
		CodeResourceNotFound,
		CodeResourceAlreadyExists,
		CodeResourceConflict,
		CodeResourceInvalid,
		CodeUpstreamUnavailable,
		CodeInternalError,
		CodeNotConfigured,
	}

	for _, code := range declared {
		if _, ok := documented[code]; !ok {
			t.Errorf("error code %s is not documented in the ErrorResponse schema enum", code)
		}
		delete(documented, code)
	}
	for code := range documented {
		t.Errorf("ErrorResponse schema documents error code %s, which no gateway constant produces", code)
	}
}

func openAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}
