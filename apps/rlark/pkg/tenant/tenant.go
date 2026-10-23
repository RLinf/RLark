// Package tenant defines tenant identity and data-plane namespace resolution.
package tenant

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DefaultID is assigned logically to resources created before tenant metadata existed.
	DefaultID = "default"
	// DefaultNamespace preserves the historical data-plane workload namespace.
	DefaultNamespace = "rlark-system"

	IDLabel             = "rlark.io/tenant-id"
	CreatedByAnnotation = "rlark.io/created-by"
)

// Tenant describes a business tenant and its data-plane namespace placement.
type Tenant struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	DefaultNamespace  string            `json:"namespace"`
	ClusterNamespaces map[string]string `json:"clusterNamespaces,omitempty"`
}

// Provider supplies tenants and resolves their data-plane namespaces.
type Provider interface {
	List(context.Context) ([]Tenant, error)
	Get(context.Context, string) (*Tenant, error)
	ResolveNamespace(context.Context, string, string) (string, error)
}

// StaticProvider exposes the backwards-compatible default tenant. It can be
// replaced by a persistent provider without changing callers.
type StaticProvider struct{}

func (StaticProvider) List(context.Context) ([]Tenant, error) {
	return []Tenant{defaultTenant()}, nil
}

func (StaticProvider) Get(_ context.Context, id string) (*Tenant, error) {
	if id == "" {
		id = DefaultID
	}
	if id != DefaultID {
		return nil, fmt.Errorf("tenant %q not found", id)
	}
	t := defaultTenant()
	return &t, nil
}

func (p StaticProvider) ResolveNamespace(ctx context.Context, tenantID, clusterID string) (string, error) {
	t, err := p.Get(ctx, tenantID)
	if err != nil {
		return "", err
	}
	if namespace := t.ClusterNamespaces[clusterID]; namespace != "" {
		return namespace, nil
	}
	if t.DefaultNamespace != "" {
		return t.DefaultNamespace, nil
	}
	return DefaultNamespace, nil
}

// IDOf returns the explicit tenant label or the backwards-compatible default.
func IDOf(object metav1.Object) string {
	if object != nil && object.GetLabels()[IDLabel] != "" {
		return object.GetLabels()[IDLabel]
	}
	return DefaultID
}

// SetIdentity writes normalized tenant and creator metadata to an object.
func SetIdentity(object metav1.Object, tenantID, createdBy string) {
	if tenantID == "" {
		tenantID = DefaultID
	}
	labels := cloneMap(object.GetLabels())
	labels[IDLabel] = tenantID
	object.SetLabels(labels)
	if createdBy != "" {
		annotations := cloneMap(object.GetAnnotations())
		annotations[CreatedByAnnotation] = createdBy
		object.SetAnnotations(annotations)
	}
}

// Inherit copies tenant identity and original creator metadata to a child.
func Inherit(parent, child metav1.Object) {
	createdBy := ""
	if parent != nil {
		createdBy = parent.GetAnnotations()[CreatedByAnnotation]
	}
	SetIdentity(child, IDOf(parent), createdBy)
}

func defaultTenant() Tenant {
	return Tenant{ID: DefaultID, Name: "Default", DefaultNamespace: DefaultNamespace, ClusterNamespaces: map[string]string{}}
}

func cloneMap(source map[string]string) map[string]string {
	result := make(map[string]string, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}
