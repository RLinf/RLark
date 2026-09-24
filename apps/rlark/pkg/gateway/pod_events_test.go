package gateway

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestRelevantPodEvents(t *testing.T) {
	currentUID := types.UID("current-pod")
	events := []corev1.Event{
		{Type: corev1.EventTypeNormal, Reason: "Pulling", Message: "Pulling image x", InvolvedObject: corev1.ObjectReference{UID: currentUID}},
		{Type: corev1.EventTypeNormal, Reason: "Created", Message: "Created container", InvolvedObject: corev1.ObjectReference{UID: currentUID}},
		{Type: corev1.EventTypeWarning, Reason: "Failed", Message: "Failed to pull image", InvolvedObject: corev1.ObjectReference{UID: currentUID}},
		{Type: corev1.EventTypeWarning, Reason: "BackOff", Message: "Old pod failed", InvolvedObject: corev1.ObjectReference{UID: "old-pod"}},
	}
	got := relevantPodEvents(events, currentUID)
	if len(got) != 2 || got[0].Reason != "Pulling" || got[1].Reason != "Failed" {
		t.Fatalf("relevantPodEvents() = %+v, want current Pod Pulling and Failed events", got)
	}
}
