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

func TestExtractTaskImages_Kubernetes(t *testing.T) {
	task := &rlarkv1alpha1.Task{
		Spec: rlarkv1alpha1.TaskSpec{
			Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{
				Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{
								{Name: "init", Image: "busybox:1.36"},
							},
							Containers: []corev1.Container{
								{Name: "main", Image: "nginx:alpine"},
								{Name: "side", Image: "redis:7"},
							},
						},
					},
				},
			},
		},
	}
	got := ExtractTaskImages(task)
	want := []string{"busybox:1.36", "nginx:alpine", "redis:7"} // sorted
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractTaskImages(k8s) = %v, want %v", got, want)
	}
}

func TestExtractTaskImages_Docker(t *testing.T) {
	task := &rlarkv1alpha1.Task{
		Spec: rlarkv1alpha1.TaskSpec{
			Docker: &rlarkv1alpha1.DockerTaskSpec{
				Containers: []rlarkv1alpha1.DockerContainerSpec{
					{Name: "a", Image: "nginx:alpine"},
					{Name: "b", Image: "redis:7"},
				},
			},
		},
	}
	got := ExtractTaskImages(task)
	want := []string{"nginx:alpine", "redis:7"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractTaskImages(docker) = %v, want %v", got, want)
	}
}

func TestExtractTaskImages_Dedup(t *testing.T) {
	task := &rlarkv1alpha1.Task{
		Spec: rlarkv1alpha1.TaskSpec{
			Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{
				Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							InitContainers: []corev1.Container{{Image: "nginx:alpine"}},
							Containers:     []corev1.Container{{Image: "nginx:alpine"}, {Image: "redis:7"}},
						},
					},
				},
			},
			Docker: &rlarkv1alpha1.DockerTaskSpec{
				Containers: []rlarkv1alpha1.DockerContainerSpec{{Image: "nginx:alpine"}},
			},
		},
	}
	got := ExtractTaskImages(task)
	want := []string{"nginx:alpine", "redis:7"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractTaskImages(dedup) = %v, want %v", got, want)
	}
}

func TestExtractTaskImages_RawHasNoImages(t *testing.T) {
	task := &rlarkv1alpha1.Task{
		Spec: rlarkv1alpha1.TaskSpec{
			Raw: &rlarkv1alpha1.RawTaskSpec{Artifact: "file:///tmp/artifact.tar"},
		},
	}
	if got := ExtractTaskImages(task); len(got) != 0 {
		t.Errorf("ExtractTaskImages(raw) = %v, want empty", got)
	}
}

func TestExtractTaskImages_SkipsEmptyImageStrings(t *testing.T) {
	task := &rlarkv1alpha1.Task{
		Spec: rlarkv1alpha1.TaskSpec{
			Docker: &rlarkv1alpha1.DockerTaskSpec{
				Containers: []rlarkv1alpha1.DockerContainerSpec{
					{Name: "a", Image: ""},
					{Name: "b", Image: "nginx:alpine"},
				},
			},
		},
	}
	got := ExtractTaskImages(task)
	want := []string{"nginx:alpine"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ExtractTaskImages(empty image) = %v, want %v", got, want)
	}
}
