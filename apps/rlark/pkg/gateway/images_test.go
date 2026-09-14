package gateway

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestRecordJobImagesAggregatesUniqueImages(t *testing.T) {
	gateway := &Gateway{images: make(map[string]imageUsage)}
	older := time.Now().Add(-time.Hour)
	newer := time.Now()

	gateway.recordJobImages(&rlarkiov1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(older)},
		Spec: rlarkiov1alpha1.JobSpec{Tasks: []rlarkiov1alpha1.JobTaskTemplate{{
			TaskSpec: rlarkiov1alpha1.TaskSpec{Kubernetes: &rlarkiov1alpha1.KubernetesTaskSpec{
				Workload: &rlarkiov1alpha1.KubernetesWorkloadSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{{Image: "init:1"}},
					Containers:     []corev1.Container{{Image: "app:1"}, {Image: "app:1"}},
				}}},
			}},
		}}},
	})
	gateway.recordJobImages(&rlarkiov1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(newer)},
		Spec: rlarkiov1alpha1.JobSpec{Tasks: []rlarkiov1alpha1.JobTaskTemplate{{
			TaskSpec: rlarkiov1alpha1.TaskSpec{Docker: &rlarkiov1alpha1.DockerTaskSpec{
				Containers: []rlarkiov1alpha1.DockerContainerSpec{{Image: "app:1"}},
			}},
		}}},
	})

	if got := gateway.images["app:1"].UseCount; got != 2 {
		t.Fatalf("app:1 use count = %d, want 2", got)
	}
	if got := gateway.images["init:1"].UseCount; got != 1 {
		t.Fatalf("init:1 use count = %d, want 1", got)
	}
	if got := gateway.images["app:1"].LastUsedAt; !got.Equal(newer) {
		t.Fatalf("app:1 last used at = %s, want %s", got, newer)
	}
}

func TestJobImagesHandlesMissingWorkload(t *testing.T) {
	job := &rlarkiov1alpha1.Job{
		Spec: rlarkiov1alpha1.JobSpec{Tasks: []rlarkiov1alpha1.JobTaskTemplate{{
			TaskSpec: rlarkiov1alpha1.TaskSpec{Kubernetes: &rlarkiov1alpha1.KubernetesTaskSpec{}},
		}}},
	}
	if got := jobImages(job); len(got) != 0 {
		t.Fatalf("jobImages() = %v, want no images", got)
	}
}
