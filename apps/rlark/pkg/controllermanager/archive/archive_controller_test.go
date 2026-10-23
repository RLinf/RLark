package archive

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestIsTerminalPhase(t *testing.T) {
	r := &JobResource{}
	tests := []struct {
		phase string
		want  bool
	}{
		{"Pending", false},
		{"Running", false},
		{"Stopped", true},
		{"Succeeded", false}, // 仅 Stopped 需要归档，其他状态先忽略
		{"Failed", false},
		{"", false},
	}
	for _, tt := range tests {
		job := &rlarkv1alpha1.Job{Status: rlarkv1alpha1.JobStatus{Phase: rlarkv1alpha1.JobPhase(tt.phase)}}
		if got := r.IsTerminal(job); got != tt.want {
			t.Errorf("IsTerminal(%q) = %v, want %v", tt.phase, got, tt.want)
		}
	}
}

func TestArchiveTTLRequeue(t *testing.T) {
	res := &JobResource{}

	// 刚进入终态，未到 TTL，应延迟
	now := metav1.Now()
	job := &rlarkv1alpha1.Job{
		Status: rlarkv1alpha1.JobStatus{
			Phase:   rlarkv1alpha1.JobPhaseStopped,
			EndTime: &now,
		},
	}

	ttl := DefaultArchiveTTL
	endTime := res.GetEndTime(job)
	if endTime == nil {
		t.Fatal("expected non-nil endTime")
	}
	remaining := ttl - time.Since(*endTime)
	if remaining <= 0 {
		t.Errorf("expected positive remaining for just-ended job, got %v", remaining)
	}

	// 已超过 TTL，应触发归档
	old := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	job.Status.EndTime = &old
	endTime = res.GetEndTime(job)
	remaining = ttl - time.Since(*endTime)
	if remaining > 0 {
		t.Errorf("expected negative remaining for old job, got %v", remaining)
	}
}

func TestDefaultArchiveTTL(t *testing.T) {
	if DefaultArchiveTTL <= 0 {
		t.Errorf("DefaultArchiveTTL should be positive, got %v", DefaultArchiveTTL)
	}
}
