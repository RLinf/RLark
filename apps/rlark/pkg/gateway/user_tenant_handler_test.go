package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// userRouter mounts the user routes the way RegisterRoutes does, so that the
// static "/me" route and the "/:id" route are resolved by the real gin router
// instead of a hand built context.
func userRouter(g *Gateway, subject, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(authSubjectKey, subject)
		c.Set(authRoleKey, role)
	})
	users := router.Group("/api/v1/users")
	users.GET("/me", g.handleGetUser)
	users.GET("/:id", g.handleGetUser)
	return router
}

// TestHandleGetUserResolvesMeOnStaticRoute pins the self lookup. "/me" is a
// static route, so the ":id" parameter is empty rather than "me" and a literal
// comparison against "me" leaves the requested identity empty, which used to
// make the caller look like a request for another user.
func TestHandleGetUserResolvesMeOnStaticRoute(t *testing.T) {
	g := &Gateway{users: stubUserProvider{caller: &user{ID: "user", Username: "user", Role: "user", TenantIDs: []string{tenant.DefaultID}}}}

	for _, path := range []string{"/api/v1/users/me", "/api/v1/users/user"} {
		recorder := httptest.NewRecorder()
		userRouter(g, "user", "user").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want %d (body %s)", path, recorder.Code, http.StatusOK, recorder.Body.String())
		}
		var result user
		if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.ID != "user" {
			t.Fatalf("GET %s returned %q, want the caller", path, result.ID)
		}
	}
}

func TestHandleGetUserForbidsAnotherUser(t *testing.T) {
	g := &Gateway{users: stubUserProvider{caller: &user{ID: "user", Username: "user", Role: "user", TenantIDs: []string{tenant.DefaultID}}}}

	recorder := httptest.NewRecorder()
	userRouter(g, "user", "user").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/users/admin", nil))
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestHandleGetUserLetsAdminReadAnotherUser(t *testing.T) {
	g := &Gateway{users: stubUserProvider{caller: &user{ID: "user", Username: "user", Role: "user", TenantIDs: []string{tenant.DefaultID}}}}

	recorder := httptest.NewRecorder()
	userRouter(g, "admin", roleAdmin).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/users/user", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body %s)", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestSecretUserProviderBuildsUsersFromLegacySecret(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: common.UIAuthSecretName, Namespace: "management"},
		Data: map[string][]byte{
			common.UIAuthAdminPasswordKey: []byte("admin-secret"),
			common.UIAuthUserPasswordKey:  []byte("user-secret"),
		},
	})
	provider := secretUserProvider{client: client, namespace: "management"}
	users, err := provider.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 2 || users[0].Username != "admin" || users[1].Username != "user" {
		t.Fatalf("unexpected users: %#v", users)
	}
	if len(users[1].TenantIDs) != 1 || users[1].TenantIDs[0] != tenant.DefaultID {
		t.Fatalf("unexpected user tenants: %#v", users[1].TenantIDs)
	}
	if _, err := provider.Authenticate(context.Background(), "user", "user-secret"); err != nil {
		t.Fatalf("Authenticate() error = %v", err)
	}
	if _, err := provider.Authenticate(context.Background(), "user", "wrong"); err == nil {
		t.Fatal("Authenticate() accepted an invalid password")
	}
}

func TestNormalizeTenantMapDoesNotChangeSource(t *testing.T) {
	object := map[string]any{"metadata": map[string]any{"name": "legacy"}}
	normalizeTenantMap(object)
	metadata := object["metadata"].(map[string]any)
	labels := metadata["labels"].(map[string]any)
	if labels[tenant.IDLabel] != tenant.DefaultID {
		t.Fatalf("tenant = %v, want %q", labels[tenant.IDLabel], tenant.DefaultID)
	}
}

func TestPrepareTenantResourceForCreateOverridesIdentity(t *testing.T) {
	body, err := prepareTenantResourceForCreate([]byte(`{"metadata":{"labels":{"rlark.io/tenant-id":"forged"},"annotations":{"rlark.io/created-by":"forged"}}}`), tenant.DefaultID, "user")
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		t.Fatal(err)
	}
	metadata := object["metadata"].(map[string]any)
	if metadata["labels"].(map[string]any)[tenant.IDLabel] != tenant.DefaultID {
		t.Fatalf("unexpected labels: %v", metadata["labels"])
	}
	if metadata["annotations"].(map[string]any)[tenant.CreatedByAnnotation] != "user" {
		t.Fatalf("unexpected annotations: %v", metadata["annotations"])
	}
}
