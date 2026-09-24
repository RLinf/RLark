package distribution

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ReplicationSecretType corev1.SecretType = "rlinf.io/resource-replication"
	DeliverySecretType    corev1.SecretType = "rlinf.io/resource-delivery"

	InventoryAnnotation = "distribution.rlinf.io/inventory"
)

type Mode struct {
	SecretType         corev1.SecretType
	AllowClusterScoped bool
	FieldManagerPrefix string
	OwnershipPrefix    string
	CleanupFinalizer   string
}

var ReplicationMode = Mode{
	SecretType:         ReplicationSecretType,
	FieldManagerPrefix: "rlark-replication-",
	OwnershipPrefix:    "replication.rlinf.io/",
	CleanupFinalizer:   "replication.rlinf.io/control-plane-cleanup",
}

var DeliveryMode = Mode{
	SecretType:         DeliverySecretType,
	AllowClusterScoped: true,
	FieldManagerPrefix: "rlark-delivery-",
	OwnershipPrefix:    "delivery.rlinf.io/",
	CleanupFinalizer:   "delivery.rlinf.io/agent-cleanup",
}

type GlobSelector struct {
	Include []string `json:"include" yaml:"include"`
	Exclude []string `json:"exclude,omitempty" yaml:"exclude,omitempty"`
}

type ApplyPolicy struct {
	Mode           string `json:"mode" yaml:"mode"`
	ConflictPolicy string `json:"conflictPolicy" yaml:"conflictPolicy"`
	DeletionPolicy string `json:"deletionPolicy" yaml:"deletionPolicy"`
	AdoptExisting  bool   `json:"adoptExisting,omitempty" yaml:"adoptExisting,omitempty"`
}

type Config struct {
	Targets struct {
		Namespaces *GlobSelector `json:"namespaces,omitempty" yaml:"namespaces,omitempty"`
	} `json:"targets,omitempty" yaml:"targets,omitempty"`
	IncludeDefinitionNamespace bool              `json:"includeDefinitionNamespace,omitempty" yaml:"includeDefinitionNamespace,omitempty"`
	RequireTargetLabels        map[string]string `json:"requireTargetNamespaceLabels,omitempty" yaml:"requireTargetNamespaceLabels,omitempty"`
	Security                   struct {
		AllowClusterScoped bool `json:"allowClusterScoped,omitempty" yaml:"allowClusterScoped,omitempty"`
	} `json:"security,omitempty" yaml:"security,omitempty"`
	Apply ApplyPolicy `json:"apply" yaml:"apply"`
}

type Definition struct {
	Config  Config
	Objects []*unstructured.Unstructured
}

type NamespaceIdentity struct {
	Name   string
	UID    types.UID
	Labels map[string]string
	Phase  corev1.NamespacePhase
}

type NamespaceResolver interface {
	List(context.Context) ([]NamespaceIdentity, error)
}

type Policy interface {
	Validate(context.Context, *corev1.Secret, *meta.RESTMapping, NamespaceIdentity, *unstructured.Unstructured, ApplyPolicy) error
}

type PolicyFunc func(context.Context, *corev1.Secret, *meta.RESTMapping, NamespaceIdentity, *unstructured.Unstructured, ApplyPolicy) error

func (f PolicyFunc) Validate(ctx context.Context, declaration *corev1.Secret, mapping *meta.RESTMapping, target NamespaceIdentity, object *unstructured.Unstructured, apply ApplyPolicy) error {
	return f(ctx, declaration, mapping, target, object, apply)
}

type Inventory struct {
	ObservedDeclarationResourceVersion string          `json:"observedDeclarationResourceVersion,omitempty"`
	ObservedRevision                   string          `json:"observedRevision,omitempty"`
	Phase                              string          `json:"phase,omitempty"`
	Message                            string          `json:"message,omitempty"`
	Items                              []InventoryItem `json:"items,omitempty"`
}

type InventoryItem struct {
	Group        string    `json:"group,omitempty"`
	Version      string    `json:"version"`
	Resource     string    `json:"resource"`
	Namespace    string    `json:"namespace,omitempty"`
	NamespaceUID types.UID `json:"namespaceUID,omitempty"`
	Name         string    `json:"name"`
	UID          types.UID `json:"uid,omitempty"`
}

type InventoryStore interface {
	Load(context.Context, *corev1.Secret) (Inventory, error)
	Save(context.Context, *corev1.Secret, Inventory) error
	Delete(context.Context, *corev1.Secret) error
}

type NoopPolicy struct{}

func (NoopPolicy) Validate(context.Context, *corev1.Secret, *meta.RESTMapping, NamespaceIdentity, *unstructured.Unstructured, ApplyPolicy) error {
	return nil
}

type Result struct {
	Inventory Inventory
	Requeue   bool
}

func ownerReference(secret *corev1.Secret) metav1.OwnerReference {
	return *metav1.NewControllerRef(secret, corev1.SchemeGroupVersion.WithKind("Secret"))
}
