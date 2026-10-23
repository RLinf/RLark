package gateway

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

type user struct {
	ID          string   `json:"id"`
	Username    string   `json:"username"`
	DisplayName string   `json:"displayName"`
	Role        string   `json:"role"`
	TenantIDs   []string `json:"tenantIDs"`
}

type authenticatedUser struct {
	user
	password string
}

type userProvider interface {
	List(context.Context) ([]user, error)
	Get(context.Context, string) (*user, error)
	Authenticate(context.Context, string, string) (*user, error)
}

type secretUserProvider struct {
	client    kubernetes.Interface
	namespace string
}

func (p secretUserProvider) users(ctx context.Context) ([]authenticatedUser, error) {
	secret, err := p.client.CoreV1().Secrets(p.namespace).Get(ctx, common.UIAuthSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return []authenticatedUser{
		{user: user{ID: "admin", Username: "admin", DisplayName: "Administrator", Role: "admin", TenantIDs: []string{tenant.DefaultID}}, password: strings.TrimSpace(string(secret.Data[common.UIAuthAdminPasswordKey]))},
		{user: user{ID: "user", Username: "user", DisplayName: "User", Role: "user", TenantIDs: []string{tenant.DefaultID}}, password: strings.TrimSpace(string(secret.Data[common.UIAuthUserPasswordKey]))},
	}, nil
}

func (p secretUserProvider) List(ctx context.Context) ([]user, error) {
	stored, err := p.users(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]user, 0, len(stored))
	for _, entry := range stored {
		result = append(result, entry.user)
	}
	return result, nil
}

func (p secretUserProvider) Get(ctx context.Context, id string) (*user, error) {
	users, err := p.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range users {
		if users[i].ID == id {
			return &users[i], nil
		}
	}
	return nil, fmt.Errorf("user %q not found", id)
}

func (p secretUserProvider) Authenticate(ctx context.Context, username, password string) (*user, error) {
	users, err := p.users(ctx)
	if err != nil {
		return nil, err
	}
	for _, entry := range users {
		if entry.Username == username && subtle.ConstantTimeCompare([]byte(password), []byte(entry.password)) == 1 {
			result := entry.user
			return &result, nil
		}
	}
	return nil, fmt.Errorf("invalid credentials")
}

func (g *Gateway) handleListUsers(c *gin.Context) {
	users, err := g.userProvider().List(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to read users")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": users})
}

func (g *Gateway) handleGetUser(c *gin.Context) {
	subject := c.GetString(authSubjectKey)
	// "/me" is served by a static route registered next to "/:id", so the path
	// parameter is empty on that route rather than holding the literal "me".
	// Both forms resolve to the authenticated caller.
	requested := c.Param("id")
	if requested == "" || requested == "me" {
		requested = subject
	}
	if c.GetString(authRoleKey) != roleAdmin && requested != subject {
		respondError(c, http.StatusForbidden, CodeForbidden, "access to another user is forbidden")
		return
	}
	result, err := g.userProvider().Get(c.Request.Context(), requested)
	if err != nil {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "user not found")
		return
	}
	c.JSON(http.StatusOK, result)
}

func (g *Gateway) userProvider() userProvider {
	if g.users != nil {
		return g.users
	}
	return secretUserProvider{client: g.rawClient, namespace: g.managementNamespace()}
}

func (g *Gateway) handleListTenants(c *gin.Context) {
	items, err := g.tenants.List(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to read tenants")
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (g *Gateway) handleGetTenant(c *gin.Context) {
	result, err := g.tenants.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "tenant not found")
		return
	}
	c.JSON(http.StatusOK, result)
}
