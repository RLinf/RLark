package addon

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

func TestAddonReadiness(t *testing.T) {
	tests := []struct {
		phase      rlarkv1alpha1.AddonPhase
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{rlarkv1alpha1.AddonPhaseReady, metav1.ConditionTrue, addonReasonReady},
		{rlarkv1alpha1.AddonPhaseFailed, metav1.ConditionFalse, addonReasonFailed},
		{rlarkv1alpha1.AddonPhaseInstalling, metav1.ConditionFalse, addonReasonInstalling},
		{rlarkv1alpha1.AddonPhaseUpgrading, metav1.ConditionFalse, addonReasonUpgrading},
		{rlarkv1alpha1.AddonPhasePending, metav1.ConditionUnknown, addonReasonUnknown},
		{rlarkv1alpha1.AddonPhase("Bogus"), metav1.ConditionUnknown, addonReasonUnknown},
	}

	for _, tt := range tests {
		t.Run(string(tt.phase), func(t *testing.T) {
			status, reason := addonReadiness(tt.phase)
			if status != tt.wantStatus || reason != tt.wantReason {
				t.Fatalf("addonReadiness(%q) = %s/%s, want %s/%s", tt.phase, status, reason, tt.wantStatus, tt.wantReason)
			}
		})
	}
}

func newAddonTestReconciler(t *testing.T, addon *rlarkv1alpha1.Addon) *pullReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := rlarkv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add scheme: %v", err)
	}
	cl := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(addon).
		WithStatusSubresource(&rlarkv1alpha1.Addon{}).
		Build()
	return &pullReconciler{c: &Controller{Controller: base.Controller{ManagementClient: cl}}}
}

func TestUpdateStatusAdvancesObservedGenerationAndWritesReady(t *testing.T) {
	addon := &rlarkv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Generation: 2},
		Spec:       rlarkv1alpha1.AddonSpec{Version: "v1"},
		Status: rlarkv1alpha1.AddonStatus{
			Phase:   rlarkv1alpha1.AddonPhaseReady,
			Message: "ready",
			Version: "v1",
			// Phase, message and version already converged, but the generation moved
			// on: ObservedGeneration must still advance or consumers would treat the
			// status as stale forever.
			ObservedGeneration: 1,
		},
	}
	r := newAddonTestReconciler(t, addon)

	r.updateStatus(context.Background(), addon, rlarkv1alpha1.AddonPhaseReady, "ready")

	var got rlarkv1alpha1.Addon
	if err := r.c.ManagementClient.Get(context.Background(), types.NamespacedName{Name: "a"}, &got); err != nil {
		t.Fatalf("failed to get addon: %v", err)
	}
	if got.Status.ObservedGeneration != 2 {
		t.Fatalf("observedGeneration = %d, want 2", got.Status.ObservedGeneration)
	}
	cond := findAddonCondition(got.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
	if cond == nil || cond.Status != metav1.ConditionTrue || cond.Reason != addonReasonReady {
		t.Fatalf("unexpected Ready condition: %#v", got.Status.Conditions)
	}
}

func TestUpdateStatusRecordsFailure(t *testing.T) {
	addon := &rlarkv1alpha1.Addon{
		ObjectMeta: metav1.ObjectMeta{Name: "a", Generation: 1},
		Spec:       rlarkv1alpha1.AddonSpec{Version: "v1"},
	}
	r := newAddonTestReconciler(t, addon)

	r.updateStatus(context.Background(), addon, rlarkv1alpha1.AddonPhaseFailed, "render error")

	var got rlarkv1alpha1.Addon
	if err := r.c.ManagementClient.Get(context.Background(), types.NamespacedName{Name: "a"}, &got); err != nil {
		t.Fatalf("failed to get addon: %v", err)
	}
	if got.Status.Phase != rlarkv1alpha1.AddonPhaseFailed || got.Status.Message != "render error" {
		t.Fatalf("unexpected status: %#v", got.Status)
	}
	// A failed install must not publish the desired version as the observed one.
	if got.Status.Version != "" {
		t.Fatalf("failed addon reported version %q", got.Status.Version)
	}
	cond := findAddonCondition(got.Status.Conditions, rlarkv1alpha1.ConditionTypeReady)
	if cond == nil || cond.Status != metav1.ConditionFalse || cond.Reason != addonReasonFailed {
		t.Fatalf("unexpected Ready condition: %#v", got.Status.Conditions)
	}
	if cond.Message != "render error" {
		t.Fatalf("Ready condition message = %q, want %q", cond.Message, "render error")
	}
}

func findAddonCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
