package utils

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestExtractTaskImages_Nil(t *testing.T) {
	if got := ExtractTaskImages(nil); got != nil {
		t.Errorf("ExtractTaskImages(nil) = %v, want nil", got)
	}
}

func TestExtractTaskImages_EmptySpec(t *testing.T) {
	task := &rlarkv1alpha1.Task{}
	if got := ExtractTaskImages(task); len(got) != 0 {
		t.Errorf("ExtractTaskImages(empty task) = %v, want empty", got)
	}
}

func TestExtractTaskImages_PodTemplate(t *testing.T) {
	task := &rlarkv1alpha1.Task{
		Spec: rlarkv1alpha1.TaskSpec{
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Template: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Name: "init", Image: "busybox:1.36"}},
						Containers: []corev1.Container{
							{Name: "main", Image: "nginx:alpine"},
							{Name: "side", Image: "redis:7"},
						},
						EphemeralContainers: []corev1.EphemeralContainer{
							{EphemeralContainerCommon: corev1.EphemeralContainerCommon{Name: "debug", Image: "debug:latest"}},
						},
					},
				},
			},
		},
	}
	got := ExtractTaskImages(task)
	want := []string{"busybox:1.36", "debug:latest", "nginx:alpine", "redis:7"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractTaskImages(pod template) = %v, want %v", got, want)
	}
}

func TestExtractTaskImages_IsRuntimeIndependent(t *testing.T) {
	task := &rlarkv1alpha1.Task{
		Spec: rlarkv1alpha1.TaskSpec{
			Runtime: rlarkv1alpha1.TaskRuntimeSpec{Kind: "Docker"},
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Template: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{
					{Name: "a", Image: "nginx:alpine"},
					{Name: "b", Image: "redis:7"},
				}}},
			},
		},
	}
	got := ExtractTaskImages(task)
	want := []string{"nginx:alpine", "redis:7"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractTaskImages(docker) = %v, want %v", got, want)
	}
}

func TestExtractTaskImages_DeduplicatesAndSkipsEmpty(t *testing.T) {
	task := &rlarkv1alpha1.Task{
		Spec: rlarkv1alpha1.TaskSpec{
			Workload: rlarkv1alpha1.TaskWorkloadSpec{
				Template: &corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						InitContainers: []corev1.Container{{Image: "nginx:alpine"}},
						Containers:     []corev1.Container{{Image: "nginx:alpine"}, {Image: "redis:7"}, {Image: ""}},
					},
				},
			},
		},
	}
	got := ExtractTaskImages(task)
	want := []string{"nginx:alpine", "redis:7"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractTaskImages(dedup) = %v, want %v", got, want)
	}
}
