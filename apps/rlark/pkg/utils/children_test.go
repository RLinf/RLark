package utils

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClassifyRemovedChildRequiresAuthoritativeIdentity(t *testing.T) {
	controller := true
	tests := []struct {
		name       string
		meta       metav1.ObjectMeta
		old        []string
		wantOwned  bool
		wantLegacy bool
	}{
		{name: "matching owner only", meta: metav1.ObjectMeta{Name: "arbitrary", OwnerReferences: []metav1.OwnerReference{{APIVersion: "rlinf.io/v1alpha1", Kind: "Workflow", Name: "wf", UID: "uid", Controller: &controller}}}, wantOwned: true, wantLegacy: true},
		{name: "matching parent uid", meta: metav1.ObjectMeta{Name: "arbitrary", Annotations: map[string]string{ParentUIDAnnotation: "uid"}}, wantOwned: true, wantLegacy: true},
		{name: "old status proves label only", meta: metav1.ObjectMeta{Name: ChildName("wf", "old"), Labels: map[string]string{"parent": "wf"}}, old: []string{"old"}, wantOwned: true, wantLegacy: true},
		{name: "prefix does not prove label only", meta: metav1.ObjectMeta{Name: "wf-unrelated", Labels: map[string]string{"parent": "wf"}}},
		{name: "conflicting owner", meta: metav1.ObjectMeta{Name: ChildName("wf", "old"), Labels: map[string]string{"parent": "wf"}, OwnerReferences: []metav1.OwnerReference{{APIVersion: "rlinf.io/v1alpha1", Kind: "Workflow", Name: "wf", UID: "other", Controller: &controller}}}, old: []string{"old"}},
		{name: "conflicting uid", meta: metav1.ObjectMeta{Name: ChildName("wf", "old"), Labels: map[string]string{"parent": "wf"}, Annotations: map[string]string{ParentUIDAnnotation: "other"}}, old: []string{"old"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owned, legacy := ClassifyRemovedChild(&tt.meta, "uid", "rlinf.io/v1alpha1", "Workflow", "parent", "wf", tt.old)
			if owned != tt.wantOwned || legacy != tt.wantLegacy {
				t.Fatalf("ClassifyRemovedChild() = %v, %v, want %v, %v", owned, legacy, tt.wantOwned, tt.wantLegacy)
			}
		})
	}
}
