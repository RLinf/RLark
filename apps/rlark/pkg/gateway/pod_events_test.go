package gateway

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestRelevantPodEvents(t *testing.T) {
	events := []corev1.Event{
		{Type: corev1.EventTypeNormal, Reason: "Pulling", Message: "Pulling image x"},
		{Type: corev1.EventTypeNormal, Reason: "Created", Message: "Created container"},
		{Type: corev1.EventTypeWarning, Reason: "Failed", Message: "Failed to pull image"},
	}
	got := relevantPodEvents(events)
	if len(got) != 2 || got[0].Reason != "Pulling" || got[1].Reason != "Failed" {
		t.Fatalf("relevantPodEvents() = %+v, want Pulling and Failed", got)
	}
}
