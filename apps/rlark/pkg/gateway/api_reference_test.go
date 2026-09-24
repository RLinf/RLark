package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAPIReferenceReturnsGroupedRegisteredEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/v1/api-reference", (&Gateway{}).handleAPIReference)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/api-reference", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}

	var body apiReferenceResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Title.ZH == "" || body.Title.EN == "" || len(body.Sections) == 0 {
		t.Fatalf("incomplete API reference: %#v", body)
	}

	registered := map[string]struct{}{}
	registeredRouter := gin.New()
	(&Gateway{}).RegisterRoutes(registeredRouter)
	for _, route := range registeredRouter.Routes() {
		registered[route.Method+" "+openAPIPath(route.Path)] = struct{}{}
	}
	for _, section := range body.Sections {
		for _, endpoint := range section.Endpoints {
			path := endpoint.Path
			for i, char := range path {
				if char == '?' {
					path = path[:i]
					break
				}
			}
			if _, ok := registered[endpoint.Method+" "+path]; !ok {
				t.Errorf("API reference endpoint is not registered: %s %s", endpoint.Method, path)
			}
		}
	}
}
