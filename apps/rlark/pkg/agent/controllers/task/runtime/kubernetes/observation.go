package kubernetes

import (
	"context"
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ObservationDirectControllerV1 follows a Pod's direct controller reference.
	ObservationDirectControllerV1 = "rlinf.io/kubernetes-direct-controller/v1"
	// ObservationReplicaSetControllerV1 follows Pod -> ReplicaSet -> root.
	ObservationReplicaSetControllerV1 = "rlinf.io/kubernetes-replicaset-controller/v1"
)

// ObservationProfile describes a finite controller-owner chain from Pod to root.
// Intermediates are ordered from the Pod's immediate owner toward the root.
// Root may be empty for a generic direct-controller profile.
type ObservationProfile struct {
	ID            string
	Root          schema.GroupVersionKind
	Intermediates []schema.GroupVersionKind
}

// ObservationProfiles is an immutable registry of trusted instance-watch paths.
type ObservationProfiles struct{ profiles map[string]ObservationProfile }

// NewObservationProfiles validates and copies the supplied paths. It starts no I/O.
func NewObservationProfiles(profiles ...ObservationProfile) (ObservationProfiles, error) {
	r := ObservationProfiles{profiles: make(map[string]ObservationProfile, len(profiles))}
	for _, p := range profiles {
		if p.ID == "" {
			return ObservationProfiles{}, fmt.Errorf("observation profile ID is required")
		}
		if _, exists := r.profiles[p.ID]; exists {
			return ObservationProfiles{}, fmt.Errorf("duplicate observation profile %q", p.ID)
		}
		if !p.Root.Empty() && (p.Root.Version == "" || p.Root.Kind == "") {
			return ObservationProfiles{}, fmt.Errorf("incomplete root in profile %q", p.ID)
		}
		seen := map[schema.GroupVersionKind]bool{p.Root: true}
		for _, gvk := range p.Intermediates {
			if gvk.Version == "" || gvk.Kind == "" || seen[gvk] {
				return ObservationProfiles{}, fmt.Errorf("invalid owner chain in profile %q", p.ID)
			}
			seen[gvk] = true
		}
		p.Intermediates = slices.Clone(p.Intermediates)
		r.profiles[p.ID] = p
	}
	return r, nil
}

// BuiltinObservationProfiles returns Kubernetes-owned paths, not Workload kinds.
func BuiltinObservationProfiles() ObservationProfiles {
	r, err := NewObservationProfiles(
		ObservationProfile{ID: ObservationDirectControllerV1},
		ObservationProfile{ID: ObservationReplicaSetControllerV1, Root: appsv1.SchemeGroupVersion.WithKind("Deployment"), Intermediates: []schema.GroupVersionKind{appsv1.SchemeGroupVersion.WithKind("ReplicaSet")}},
	)
	if err != nil {
		panic(err)
	}
	return r
}

// Resolve selects a profile for an explicit root GVK. Unknown IDs fail closed.
func (r ObservationProfiles) Resolve(id string, root schema.GroupVersionKind) (ObservationProfile, error) {
	p, found := r.profiles[id]
	if !found || (!p.Root.Empty() && p.Root != root) {
		return ObservationProfile{}, fmt.Errorf("unsupported observation profile %q for %s", id, root)
	}
	p.Root = root
	p.Intermediates = slices.Clone(p.Intermediates)
	return p, nil
}

// OwningRoot verifies every API version and object-instance UID in the declared
// chain. All reads stay in the instance namespace. Errors are retryable by the
// caller; a stale, incomplete, or foreign chain yields no root.
func (p ObservationProfile) OwningRoot(ctx context.Context, rootReader, intermediaryReader client.Reader, instance, root client.Object) (client.Object, error) {
	current := instance
	for i, gvk := range append(slices.Clone(p.Intermediates), p.Root) {
		owner := metav1.GetControllerOf(current)
		if owner == nil || owner.UID == "" || owner.Name == "" || owner.Kind != gvk.Kind || owner.APIVersion != gvk.GroupVersion().String() {
			return nil, nil
		}
		// Intermediaries have get-only RBAC and no informer. Only the root
		// uses the observed reader, with the same typed/unstructured prototype
		// as its watch (avoids a second informer for a different object form).
		var obj client.Object = &unstructured.Unstructured{}
		obj.GetObjectKind().SetGroupVersionKind(gvk)
		reader := intermediaryReader
		if i == len(p.Intermediates) {
			obj = root
			reader = rootReader
		}
		if err := reader.Get(ctx, client.ObjectKey{Namespace: instance.GetNamespace(), Name: owner.Name}, obj); err != nil {
			return nil, client.IgnoreNotFound(err)
		}
		if obj.GetUID() != owner.UID {
			return nil, nil
		}
		current = obj
	}
	return current, nil
}
