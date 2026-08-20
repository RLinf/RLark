package gateway

import (
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.yaml.in/yaml/v3"
)

func TestSwaggerOperationsAreRegisteredRoutes(t *testing.T) {
	data, err := os.ReadFile("../../docs/api/swagger.yaml")
	if err != nil {
		t.Fatal(err)
	}

	var document struct {
		Paths map[string]map[string]struct {
			Responses map[string]any `yaml:"responses"`
		} `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	(&Gateway{}).RegisterRoutes(router)

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+openAPIPath(route.Path)] = struct{}{}
	}

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

func openAPIPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if strings.HasPrefix(part, ":") || strings.HasPrefix(part, "*") {
			parts[i] = "{" + part[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}
