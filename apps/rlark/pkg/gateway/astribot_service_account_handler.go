package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// A service account is how a program, not a person, calls this API: Fleet's
// Dashboard submitting and stopping Jobs for its operators. It has a role and
// tenants like a user, and one token that does not expire and is revoked by
// rotating it or deleting the account. Only the token's hash is stored. The
// token can be delivered as a job secret, so the program that uses it reads
// it from a mounted file and nobody copies it by hand.
const (
	serviceAccountLabel       = "rlark.io/service-account"
	serviceAccountTokenPrefix = "rlsa."
	serviceAccountSubject     = "service-account:"
)

func serviceAccountSecretName(name string) string { return "rlark-service-account-" + name }

type serviceAccountRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Role        string   `json:"role"`
	TenantIDs   []string `json:"tenant_ids"`
	// Deliver, when set, keeps a job secret holding the token on these clusters.
	Deliver *struct {
		Clusters   []string `json:"clusters"`
		SecretName string   `json:"secret_name"`
	} `json:"deliver"`
}

type serviceAccountView struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Role            string   `json:"role"`
	TenantIDs       []string `json:"tenantIds"`
	DeliverClusters []string `json:"deliverClusters,omitempty"`
	DeliverSecret   string   `json:"deliverSecret,omitempty"`
	CreatedBy       string   `json:"createdBy,omitempty"`
	CreatedAt       string   `json:"createdAt,omitempty"`
}

func serviceAccountFromSecret(secret *v1.Secret) serviceAccountView {
	list := func(key string) []string {
		if value := string(secret.Data[key]); value != "" {
			return strings.Split(value, ",")
		}
		return nil
	}
	return serviceAccountView{
		Name: string(secret.Data["name"]), Description: string(secret.Data["description"]), Role: string(secret.Data["role"]),
		TenantIDs: list("tenant-ids"), DeliverClusters: list("deliver-clusters"), DeliverSecret: string(secret.Data["deliver-secret"]),
		CreatedBy: secret.Annotations[tenant.CreatedByAnnotation], CreatedAt: secret.CreationTimestamp.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// newServiceAccountToken returns a token and the hash that is stored for it.
func newServiceAccountToken(name string) (string, string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", "", err
	}
	token := serviceAccountTokenPrefix + name + "." + base64.RawURLEncoding.EncodeToString(random)
	digest := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(digest[:]), nil
}

// authenticateServiceAccount accepts a bearer token of a service account. The
// account is read on every request, so a rotated or deleted one stops working
// at once. Its tenants become the request's scope directly: a service account
// is not in the user directory.
func (g *Gateway) authenticateServiceAccount(c *gin.Context, token string) bool {
	name, _, ok := strings.Cut(strings.TrimPrefix(token, serviceAccountTokenPrefix), ".")
	if !ok || len(validation.IsDNS1123Label(name)) != 0 {
		return false
	}
	secret, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(c.Request.Context(), serviceAccountSecretName(name), metav1.GetOptions{})
	if err != nil || secret.Labels[serviceAccountLabel] != "true" {
		return false
	}
	digest := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(digest[:])), secret.Data["token-sha256"]) != 1 {
		return false
	}
	account := serviceAccountFromSecret(secret)
	c.Set(authSubjectKey, serviceAccountSubject+name)
	c.Set(authRoleKey, account.Role)
	if account.Role != roleAdmin {
		c.Set(tenantScopeContextKey, tenantScope{tenants: account.TenantIDs})
	}
	return true
}

// issueServiceAccountToken stores a new token hash and, when the account is
// delivered, replaces the job secret that carries the token.
func (g *Gateway) issueServiceAccountToken(ctx context.Context, subject string, secret *v1.Secret, create bool) (string, error) {
	account := serviceAccountFromSecret(secret)
	token, hash, err := newServiceAccountToken(account.Name)
	if err != nil {
		return "", err
	}
	secret.Data["token-sha256"] = []byte(hash)
	secrets := g.rawClient.CoreV1().Secrets(secret.Namespace)
	if create {
		_, err = secrets.Create(ctx, secret, metav1.CreateOptions{})
	} else {
		_, err = secrets.Update(ctx, secret, metav1.UpdateOptions{})
	}
	if err != nil {
		return "", err
	}
	if account.DeliverSecret != "" {
		err = putJobSecret(ctx, g.rawClient, g.managementNamespace(), subject, jobSecretRequest{
			Name: account.DeliverSecret, Description: "token of service account " + account.Name, Clusters: account.DeliverClusters,
			Data: map[string]string{"token": token, "service-account": account.Name},
		}, true)
	}
	return token, err
}

func (g *Gateway) createServiceAccount(c *gin.Context) {
	var req serviceAccountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Role == "" {
		req.Role = "user"
	}
	if len(req.TenantIDs) == 0 {
		req.TenantIDs = []string{tenant.DefaultID}
	}
	if len(validation.IsDNS1123Label(req.Name)) != 0 || len(req.Name) > 40 || (req.Role != "user" && req.Role != roleOperator && req.Role != roleAdmin) {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "name must be a DNS label of at most 40 characters and role user, operator or admin")
		return
	}
	data := map[string][]byte{
		"name": []byte(req.Name), "description": []byte(req.Description), "role": []byte(req.Role), "tenant-ids": []byte(strings.Join(req.TenantIDs, ",")),
	}
	if req.Deliver != nil {
		// Checked before anything is stored: the account and its delivery are one request.
		if _, err := buildJobSecretReplication(g.managementNamespace(), jobSecretRequest{Name: req.Deliver.SecretName, Clusters: req.Deliver.Clusters, Data: map[string]string{"token": "x"}}); err != nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "deliver: "+err.Error())
			return
		}
		data["deliver-clusters"], data["deliver-secret"] = []byte(strings.Join(req.Deliver.Clusters, ",")), []byte(req.Deliver.SecretName)
	}
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: serviceAccountSecretName(req.Name), Namespace: g.managementNamespace(), Labels: map[string]string{serviceAccountLabel: "true"}},
		Type:       v1.SecretTypeOpaque, Data: data,
	}
	tenant.SetIdentity(secret, tenant.DefaultID, c.GetString(authSubjectKey))
	token, err := g.issueServiceAccountToken(c.Request.Context(), c.GetString(authSubjectKey), secret, true)
	if errors.IsAlreadyExists(err) {
		respondError(c, http.StatusConflict, CodeInvalidRequest, "a service account of this name exists")
		return
	}
	if err != nil {
		respondKubeError(c, fmt.Sprintf("create service account: %v", err), err)
		return
	}
	// The only time the token is shown.
	c.JSON(http.StatusCreated, gin.H{"success": true, "name": req.Name, "token": token})
}

// rotateServiceAccountToken replaces the token; the previous one stops working at once.
func (g *Gateway) rotateServiceAccountToken(c *gin.Context) {
	secret, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(c.Request.Context(), serviceAccountSecretName(c.Param("name")), metav1.GetOptions{})
	if err != nil || secret.Labels[serviceAccountLabel] != "true" {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "service account not found")
		return
	}
	token, err := g.issueServiceAccountToken(c.Request.Context(), c.GetString(authSubjectKey), secret, false)
	if err != nil {
		respondKubeError(c, fmt.Sprintf("rotate service account token: %v", err), err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "name": c.Param("name"), "token": token})
}

func (g *Gateway) listServiceAccounts(c *gin.Context) {
	secrets, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).List(c.Request.Context(), metav1.ListOptions{LabelSelector: serviceAccountLabel + "=true"})
	if err != nil {
		respondKubeError(c, fmt.Sprintf("list service accounts: %v", err), err)
		return
	}
	items := make([]serviceAccountView, 0, len(secrets.Items))
	for i := range secrets.Items {
		items = append(items, serviceAccountFromSecret(&secrets.Items[i]))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	c.JSON(http.StatusOK, gin.H{"success": true, "data": items})
}

// deleteServiceAccount revokes the token and removes its delivered secret.
func (g *Gateway) deleteServiceAccount(c *gin.Context) {
	secrets := g.rawClient.CoreV1().Secrets(g.managementNamespace())
	secret, err := secrets.Get(c.Request.Context(), serviceAccountSecretName(c.Param("name")), metav1.GetOptions{})
	if err != nil || secret.Labels[serviceAccountLabel] != "true" {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "service account not found")
		return
	}
	if err := secrets.Delete(c.Request.Context(), secret.Name, metav1.DeleteOptions{}); err != nil {
		respondKubeError(c, fmt.Sprintf("delete service account: %v", err), err)
		return
	}
	if delivered := string(secret.Data["deliver-secret"]); delivered != "" {
		if err := secrets.Delete(c.Request.Context(), jobSecretReplicationName(delivered), metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			respondKubeError(c, fmt.Sprintf("remove delivered token: %v", err), err)
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
