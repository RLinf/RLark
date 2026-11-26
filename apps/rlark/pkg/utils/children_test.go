package utils

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	testAPIVersion = "rlinf.io/v1alpha1"
	testKind       = "Workflow"
	testLabel      = "parent"
)

func testParent() ParentRef {
	return ParentRef{
		UID:        "uid",
		APIVersion: testAPIVersion,
		Kind:       testKind,
		Name:       "wf",
		Label:      testLabel,
	}
}

func controllerRef(uid string) []metav1.OwnerReference {
	controller := true
	return []metav1.OwnerReference{{
		APIVersion: testAPIVersion,
		Kind:       testKind,
		Name:       "wf",
		UID:        types.UID(uid),
		Controller: &controller,
	}}
}

// TestClassifyChild pins the contract that an attribution the controller
// cannot prove is reported as an error, so the caller cannot mutate the child
// by accident.
func TestClassifyChild(t *testing.T) {
	tests := []struct {
		name          string
		meta          metav1.ObjectMeta
		template      string
		wantOwnership ChildOwnership
		wantReason    ChildReason
		wantErr       bool
	}{
		{
			name: "canonical metadata",
			meta: metav1.ObjectMeta{
				Name:            ChildName("wf", "step"),
				Labels:          map[string]string{testLabel: "wf"},
				Annotations:     map[string]string{ParentUIDAnnotation: "uid", ChildTemplateAnnotation: "step"},
				OwnerReferences: controllerRef("uid"),
			},
			template:      "step",
			wantOwnership: ChildOwned,
			wantReason:    ReasonAuthoritativeMetadata,
		},
		{
			name: "owner reference without canonical annotations is adoptable",
			meta: metav1.ObjectMeta{
				Name:            ChildName("wf", "step"),
				OwnerReferences: controllerRef("uid"),
			},
			template:      "step",
			wantOwnership: ChildAdoptable,
			wantReason:    ReasonLegacyOwnerReference,
		},
		{
			name: "label alone is adoptable",
			meta: metav1.ObjectMeta{
				Name:   ChildName("wf", "step"),
				Labels: map[string]string{testLabel: "wf"},
			},
			template:      "step",
			wantOwnership: ChildAdoptable,
			wantReason:    ReasonLegacyLabel,
		},
		{
			name:          "no ownership evidence is rejected",
			meta:          metav1.ObjectMeta{Name: ChildName("wf", "step")},
			template:      "step",
			wantOwnership: ChildUnproven,
			wantReason:    ReasonUnprovenOwnership,
			wantErr:       true,
		},
		{
			name: "another parent uid is foreign",
			meta: metav1.ObjectMeta{
				Name:        ChildName("wf", "step"),
				Labels:      map[string]string{testLabel: "wf"},
				Annotations: map[string]string{ParentUIDAnnotation: "other"},
			},
			template:      "step",
			wantOwnership: ChildForeign,
			wantReason:    ReasonOwnedByOtherParent,
			wantErr:       true,
		},
		{
			name: "another parent owner reference is foreign",
			meta: metav1.ObjectMeta{
				Name:            ChildName("wf", "step"),
				Labels:          map[string]string{testLabel: "wf"},
				OwnerReferences: controllerRef("other"),
			},
			template:      "step",
			wantOwnership: ChildForeign,
			wantReason:    ReasonOwnedByOtherParent,
			wantErr:       true,
		},
		{
			name: "template mismatch is rejected",
			meta: metav1.ObjectMeta{
				Name:            ChildName("wf", "step"),
				Annotations:     map[string]string{ParentUIDAnnotation: "uid", ChildTemplateAnnotation: "other"},
				OwnerReferences: controllerRef("uid"),
			},
			template:      "step",
			wantOwnership: ChildUnproven,
			wantReason:    ReasonConflictingMetadata,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := ClassifyChild(&tt.meta, testParent(), tt.template)
			assertDecision(t, "ClassifyChild", decision, err, tt.wantOwnership, tt.wantReason, tt.wantErr)
		})
	}
}

// TestClassifyRemovedChildRequiresAuthoritativeIdentity pins the asymmetry of
// the removal path: another parent's object is skipped silently, while an
// object that cannot be attributed at all blocks the reconcile instead of
// being deleted on a guess.
func TestClassifyRemovedChildRequiresAuthoritativeIdentity(t *testing.T) {
	tests := []struct {
		name          string
		meta          metav1.ObjectMeta
		legacyNames   []string
		wantOwnership ChildOwnership
		wantReason    ChildReason
		wantErr       bool
	}{
		{
			name: "canonical metadata",
			meta: metav1.ObjectMeta{
				Name:            ChildName("wf", "old"),
				Annotations:     map[string]string{ParentUIDAnnotation: "uid", ChildTemplateAnnotation: "old"},
				OwnerReferences: controllerRef("uid"),
			},
			wantOwnership: ChildOwned,
			wantReason:    ReasonAuthoritativeMetadata,
		},
		{
			name:          "matching owner only",
			meta:          metav1.ObjectMeta{Name: "arbitrary", OwnerReferences: controllerRef("uid")},
			wantOwnership: ChildAdoptable,
			wantReason:    ReasonLegacyOwnerReference,
		},
		{
			name:          "matching parent uid",
			meta:          metav1.ObjectMeta{Name: "arbitrary", Annotations: map[string]string{ParentUIDAnnotation: "uid"}},
			wantOwnership: ChildAdoptable,
			wantReason:    ReasonLegacyOwnerReference,
		},
		{
			name: "recorded legacy name proves a label-only child",
			meta: metav1.ObjectMeta{
				Name:   ChildName("wf", "old"),
				Labels: map[string]string{testLabel: "wf"},
			},
			legacyNames:   []string{ChildName("wf", "old")},
			wantOwnership: ChildAdoptable,
			wantReason:    ReasonLegacyLabel,
		},
		{
			name: "unrecorded label-only child is rejected",
			meta: metav1.ObjectMeta{
				Name:   "wf-unrelated",
				Labels: map[string]string{testLabel: "wf"},
			},
			legacyNames:   []string{ChildName("wf", "old")},
			wantOwnership: ChildUnproven,
			wantReason:    ReasonUnprovenOwnership,
			wantErr:       true,
		},
		{
			name:          "no parent reference is skipped",
			meta:          metav1.ObjectMeta{Name: ChildName("wf", "old")},
			legacyNames:   []string{ChildName("wf", "old")},
			wantOwnership: ChildForeign,
			wantReason:    ReasonNoParentReference,
		},
		{
			name: "conflicting owner",
			meta: metav1.ObjectMeta{
				Name:            ChildName("wf", "old"),
				Labels:          map[string]string{testLabel: "wf"},
				OwnerReferences: controllerRef("other"),
			},
			legacyNames:   []string{ChildName("wf", "old")},
			wantOwnership: ChildForeign,
			wantReason:    ReasonOwnedByOtherParent,
		},
		{
			name: "conflicting uid",
			meta: metav1.ObjectMeta{
				Name:        ChildName("wf", "old"),
				Labels:      map[string]string{testLabel: "wf"},
				Annotations: map[string]string{ParentUIDAnnotation: "other"},
			},
			legacyNames:   []string{ChildName("wf", "old")},
			wantOwnership: ChildForeign,
			wantReason:    ReasonOwnedByOtherParent,
		},
		{
			name: "template without parent identity is rejected",
			meta: metav1.ObjectMeta{
				Name:        ChildName("wf", "old"),
				Labels:      map[string]string{testLabel: "wf"},
				Annotations: map[string]string{ChildTemplateAnnotation: "old"},
			},
			legacyNames:   []string{ChildName("wf", "old")},
			wantOwnership: ChildUnproven,
			wantReason:    ReasonConflictingMetadata,
			wantErr:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision, err := ClassifyRemovedChild(&tt.meta, testParent(), tt.legacyNames)
			assertDecision(t, "ClassifyRemovedChild", decision, err, tt.wantOwnership, tt.wantReason, tt.wantErr)
		})
	}
}

func assertDecision(
	t *testing.T, fn string, decision ChildDecision, err error,
	wantOwnership ChildOwnership, wantReason ChildReason, wantErr bool,
) {
	t.Helper()
	if decision.Ownership != wantOwnership || decision.Reason != wantReason {
		t.Fatalf("%s() = %v/%v, want %v/%v", fn,
			decision.Ownership, decision.Reason, wantOwnership, wantReason)
	}
	if wantErr && err == nil {
		t.Fatalf("%s() returned no error for %v/%v", fn, wantOwnership, wantReason)
	}
	if !wantErr && err != nil {
		t.Fatalf("%s() returned unexpected error: %v", fn, err)
	}
}
