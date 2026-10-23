package workload

import (
	"errors"
	"testing"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
)

func TestSelectKeepsKubernetesWorkloadsIndependent(t *testing.T) {
	candidates := []Candidate[string]{
		{Type: "Deployment", Version: "v1alpha1", Value: "deployment"},
		{Type: "DaemonSet", Version: "v1alpha1", Value: "daemonset"},
		{Type: "StatefulSet", Version: "v1alpha1", Value: "statefulset"},
	}
	for _, candidate := range candidates {
		got, err := Select(coordination.WorkloadSelector{Type: string(candidate.Type), Version: candidate.Version}, candidates)
		if err != nil || got != candidate.Value {
			t.Fatalf("Select(%s) = %q, %v", candidate.Type, got, err)
		}
	}
}

func TestSelectRejectsMissingAndAmbiguousAdapters(t *testing.T) {
	selector := coordination.WorkloadSelector{Type: "Deployment", Version: "v1alpha1"}
	if _, err := Select[string](selector, nil); !errors.Is(err, ErrAdapterNotFound) {
		t.Fatalf("missing error = %v", err)
	}
	duplicate := []Candidate[string]{
		{Type: "Deployment", Version: "v1alpha1"},
		{Type: "Deployment", Version: "v1alpha1"},
	}
	if _, err := Select(selector, duplicate); !errors.Is(err, ErrAmbiguousAdapter) {
		t.Fatalf("ambiguous error = %v", err)
	}
}
