package utils

import (
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ParentUIDAnnotation     = "rlark.io/parent-uid"
	ChildTemplateAnnotation = "rlark.io/child-template"
)

// ChildOwnership classifies one candidate child object against one parent.
type ChildOwnership string

const (
	// ChildOwned means ownership is proven by the canonical metadata.
	ChildOwned ChildOwnership = "Owned"
	// ChildAdoptable means ownership is proven, but only through a legacy form
	// that the caller must migrate to the canonical one.
	ChildAdoptable ChildOwnership = "Adoptable"
	// ChildForeign means the object provably does not belong to this parent, so
	// skipping it is safe.
	ChildForeign ChildOwnership = "Foreign"
	// ChildUnproven means ownership can neither be proven nor disproven. The
	// caller must refuse to act on the object instead of guessing.
	ChildUnproven ChildOwnership = "Unproven"
)

// ChildReason records the evidence a ChildOwnership was derived from, so
// callers can surface a stable cause instead of an opaque boolean.
type ChildReason string

const (
	// ReasonAuthoritativeMetadata is the canonical form: controller reference
	// plus parent UID and child template annotations.
	ReasonAuthoritativeMetadata ChildReason = "AuthoritativeMetadata"
	// ReasonLegacyOwnerReference is proven parent identity in an incomplete
	// canonical form, for example a controller reference without annotations.
	ReasonLegacyOwnerReference ChildReason = "LegacyOwnerReference"
	// ReasonLegacyLabel is the label-only form emitted by older controllers.
	ReasonLegacyLabel ChildReason = "LegacyLabel"
	// ReasonOwnedByOtherParent means the object records a different parent.
	ReasonOwnedByOtherParent ChildReason = "OwnedByOtherParent"
	// ReasonNoParentReference means nothing on the object refers to this parent.
	ReasonNoParentReference ChildReason = "NoParentReference"
	// ReasonConflictingMetadata means the ownership metadata contradicts itself
	// or the requested child template identity.
	ReasonConflictingMetadata ChildReason = "ConflictingOwnershipMetadata"
	// ReasonUnprovenOwnership means the available evidence is insufficient to
	// attribute the object to this parent.
	ReasonUnprovenOwnership ChildReason = "OwnershipUnproven"
)

// ParentRef identifies the parent an ownership decision is made against. It
// owns the only controller-reference comparison in the control plane; callers
// must not re-implement owner matching.
type ParentRef struct {
	UID        types.UID
	APIVersion string
	Kind       string
	Name       string
	// Label is the stable label legacy children are selected by. It may be
	// empty for parents that never emitted a label-only form.
	Label string
}

// Controls reports whether this parent is the controller recorded on obj.
func (p ParentRef) Controls(obj metav1.Object) bool {
	owner := metav1.GetControllerOf(obj)
	return owner != nil && owner.UID == p.UID && owner.APIVersion == p.APIVersion &&
		owner.Kind == p.Kind && owner.Name == p.Name
}

// ChildIndexKeys returns the parent names under which obj must be indexed so a
// parent of the given apiVersion and kind can find it with a field selector.
//
// Narrowing by index has to cover every form of ownership evidence that
// ClassifyChild accepts as proof. A child that carries only a controller
// OwnerReference is still Adoptable, so indexing by label alone would hide it
// from its own parent and silently leak it.
func ChildIndexKeys(obj metav1.Object, apiVersion, kind, label string) []string {
	keys := make([]string, 0, 2)
	if owner := metav1.GetControllerOf(obj); owner != nil &&
		owner.APIVersion == apiVersion && owner.Kind == kind && owner.Name != "" {
		keys = append(keys, owner.Name)
	}
	if label != "" {
		if name := obj.GetLabels()[label]; name != "" && !slices.Contains(keys, name) {
			keys = append(keys, name)
		}
	}
	return keys
}

// ChildDecision is the evidence-carrying result of a classification.
type ChildDecision struct {
	Ownership ChildOwnership
	Reason    ChildReason
}

// Owned reports whether the caller may act on the child.
func (d ChildDecision) Owned() bool {
	return d.Ownership == ChildOwned || d.Ownership == ChildAdoptable
}

// Legacy reports whether the child still carries a legacy ownership form and
// has to be migrated to the canonical one.
func (d ChildDecision) Legacy() bool { return d.Ownership == ChildAdoptable }

func (d ChildDecision) err(obj metav1.Object, parent ParentRef) error {
	return fmt.Errorf("child %s is %s for %s %s: %s",
		obj.GetName(), d.Ownership, parent.Kind, parent.Name, d.Reason)
}

// ClassifyChild decides whether the caller may adopt or update obj as a child
// of parent. templateName is the child template identity, or "" for singleton
// children that have no template dimension.
//
// Every decision the caller must not act on is returned together with an
// error, including ChildUnproven: ownership that cannot be proven is refused
// rather than ignored, so the caller reports a blocked reconcile instead of
// mutating an object it may not own.
func ClassifyChild(obj metav1.Object, parent ParentRef, templateName string) (ChildDecision, error) {
	annotations := obj.GetAnnotations()
	uid, hasUID := annotations[ParentUIDAnnotation]
	template, hasTemplate := annotations[ChildTemplateAnnotation]
	owner := metav1.GetControllerOf(obj)
	controlled := parent.Controls(obj)
	uidMatches := hasUID && uid == string(parent.UID)
	templateMatches := hasTemplate == (templateName != "") && template == templateName

	decide := func(ownership ChildOwnership, reason ChildReason) (ChildDecision, error) {
		decision := ChildDecision{Ownership: ownership, Reason: reason}
		if decision.Owned() {
			return decision, nil
		}
		return decision, decision.err(obj, parent)
	}

	switch {
	case hasUID && !uidMatches:
		return decide(ChildForeign, ReasonOwnedByOtherParent)
	case owner != nil && !controlled:
		return decide(ChildForeign, ReasonOwnedByOtherParent)
	case controlled && uidMatches && templateMatches:
		return decide(ChildOwned, ReasonAuthoritativeMetadata)
	case controlled && !hasUID && !hasTemplate:
		// Written before the canonical annotations existed.
		return decide(ChildAdoptable, ReasonLegacyOwnerReference)
	case hasUID || hasTemplate || owner != nil:
		// Parent identity is present but incomplete or contradicts the
		// requested template, which means the name collides with a different
		// child of the same parent.
		return decide(ChildUnproven, ReasonConflictingMetadata)
	case parent.Label != "" && obj.GetLabels()[parent.Label] == parent.Name:
		return decide(ChildAdoptable, ReasonLegacyLabel)
	default:
		return decide(ChildUnproven, ReasonUnprovenOwnership)
	}
}

// ClassifyRemovedChild decides whether the caller may stop or delete obj as a
// child of parent that is no longer desired. legacyNames are the names the
// parent could have given its children before the canonical metadata existed;
// child names are never assumed to be reversible.
//
// ChildForeign is returned without an error because skipping another parent's
// object is the correct outcome. ChildUnproven is returned with an error: a
// label-only object that cannot be attributed to this parent must block the
// reconcile rather than be deleted on a guess.
func ClassifyRemovedChild(obj metav1.Object, parent ParentRef, legacyNames []string) (ChildDecision, error) {
	annotations := obj.GetAnnotations()
	uid, hasUID := annotations[ParentUIDAnnotation]
	_, hasTemplate := annotations[ChildTemplateAnnotation]
	owner := metav1.GetControllerOf(obj)
	controlled := parent.Controls(obj)
	uidMatches := hasUID && uid == string(parent.UID)

	switch {
	case hasUID && !uidMatches:
		return ChildDecision{ChildForeign, ReasonOwnedByOtherParent}, nil
	case owner != nil && !controlled:
		return ChildDecision{ChildForeign, ReasonOwnedByOtherParent}, nil
	case controlled && uidMatches && hasTemplate:
		return ChildDecision{ChildOwned, ReasonAuthoritativeMetadata}, nil
	case controlled || uidMatches:
		return ChildDecision{ChildAdoptable, ReasonLegacyOwnerReference}, nil
	case hasTemplate:
		// Template identity without parent identity cannot be attributed.
		decision := ChildDecision{ChildUnproven, ReasonConflictingMetadata}
		return decision, decision.err(obj, parent)
	case parent.Label == "" || obj.GetLabels()[parent.Label] != parent.Name:
		return ChildDecision{ChildForeign, ReasonNoParentReference}, nil
	}

	for _, name := range legacyNames {
		if name == obj.GetName() {
			return ChildDecision{ChildAdoptable, ReasonLegacyLabel}, nil
		}
	}
	decision := ChildDecision{ChildUnproven, ReasonUnprovenOwnership}
	return decision, decision.err(obj, parent)
}

func MergeAnnotations(existing, owned map[string]string) map[string]string {
	merged := make(map[string]string, len(existing)+len(owned))
	for key, value := range existing {
		merged[key] = value
	}
	for key, value := range owned {
		if value == "" {
			delete(merged, key)
		} else {
			merged[key] = value
		}
	}
	return merged
}
