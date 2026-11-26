package job

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/sweep"
)

func TestOrphanTaskRuleIsUsableBySweeper(t *testing.T) {
	if _, err := sweep.New(nil, nil, controller.Shell{}, sweep.DefaultOptions(), OrphanTaskRule()); err != nil {
		t.Fatalf("sweeper rejected the Task rule: %v", err)
	}
}

func TestOrphanTaskRuleParentNames(t *testing.T) {
	owned := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "ns"}}
	owned.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: rlarkv1alpha1.GroupVersion.String(),
		Kind:       "Job",
		Name:       "owner-job",
		UID:        types.UID("job-uid"),
		Controller: ptrTo(true),
	}}

	labelled := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name:      "t",
		Namespace: "ns",
		Labels:    map[string]string{jobLabel: "legacy-job"},
	}}

	// A Task created by hand carries neither reference, so the sweeper has no
	// candidate parent to check and must leave it alone.
	bare := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "t", Namespace: "ns"}}

	cases := map[string]struct {
		child client.Object
		want  []string
	}{
		"owner reference": {child: owned, want: []string{"owner-job"}},
		"legacy label":    {child: labelled, want: []string{"legacy-job"}},
		"no reference":    {child: bare, want: nil},
	}

	rule := OrphanTaskRule()
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got := rule.ParentNames(tc.child)
			if len(got) != len(tc.want) {
				t.Fatalf("parent names = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parent names = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func ptrTo[T any](v T) *T { return &v }
