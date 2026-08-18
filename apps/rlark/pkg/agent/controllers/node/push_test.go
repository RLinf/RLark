package node

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func TestMergeManagementNodeMetadata(t *testing.T) {
	managementLabels := map[string]string{
		"rlark.io/node-category-cloud": "true",
		"admin.example/obsolete":       "keep-out",
	}
	discoveredLabels := map[string]string{
		"kubernetes.io/hostname":       "gpu47",
		"rlark.io/node-category-cloud": "false",
	}
	wantLabels := map[string]string{
		"kubernetes.io/hostname":       "gpu47",
		"rlark.io/node-category-cloud": "true",
	}
	if got := mergeManagementMetadata(managementLabels, discoveredLabels, isManagementOwnedNodeLabel); !reflect.DeepEqual(got, wantLabels) {
		t.Fatalf("merged labels = %#v, want %#v", got, wantLabels)
	}

	managementAnnotations := map[string]string{
		"rlark.io/city":         "深圳市",
		"rlark.io/gpu-model":    "NVIDIA 4090",
		"rlark.io/device-model": "Unitree G1",
		"other.example/note":    "management-only",
	}
	discoveredAnnotations := map[string]string{
		"rlark.io/agent-note": "reported",
		"rlark.io/city":       "stale-local-value",
	}
	wantAnnotations := map[string]string{
		"rlark.io/city":         "深圳市",
		"rlark.io/gpu-model":    "NVIDIA 4090",
		"rlark.io/device-model": "Unitree G1",
		"rlark.io/agent-note":   "reported",
		"other.example/note":    "management-only",
	}
	if got := mergeManagementAnnotations(managementAnnotations, discoveredAnnotations); !reflect.DeepEqual(got, wantAnnotations) {
		t.Fatalf("merged annotations = %#v, want %#v", got, wantAnnotations)
	}
}

func TestRequestedResourcesForNode(t *testing.T) {
	pods := []corev1.Pod{
		{
			Spec: corev1.PodSpec{
				NodeName: "gpu20",
				Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU:                    resource.MustParse("1500m"),
					corev1.ResourceMemory:                 resource.MustParse("2Gi"),
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
				}}}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			Spec: corev1.PodSpec{
				NodeName: "gpu20",
				Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"),
				}}}},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
		{
			Spec: corev1.PodSpec{NodeName: "gpu20", Containers: []corev1.Container{{Resources: corev1.ResourceRequirements{Requests: corev1.ResourceList{
				corev1.ResourceCPU: resource.MustParse("8"),
			}}}}},
			Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
	}

	got := requestedResourcesForNode("gpu20", pods)
	if cpu := got.Cpu().String(); cpu != "2" {
		t.Fatalf("cpu requests = %s, want 2", cpu)
	}
	if memory := got.Memory().String(); memory != "2Gi" {
		t.Fatalf("memory requests = %s, want 2Gi", memory)
	}
	if gpu := got[corev1.ResourceName("nvidia.com/gpu")]; gpu.String() != "1" {
		t.Fatalf("gpu requests = %s, want 1", gpu.String())
	}
}

func TestDiskPressure(t *testing.T) {
	tests := []struct {
		name       string
		conditions []corev1.NodeCondition
		want       *bool
	}{
		{name: "not reported", want: nil},
		{
			name: "unknown",
			conditions: []corev1.NodeCondition{{
				Type: corev1.NodeDiskPressure, Status: corev1.ConditionUnknown,
			}},
			want: nil,
		},
		{
			name: "normal",
			conditions: []corev1.NodeCondition{{
				Type: corev1.NodeDiskPressure, Status: corev1.ConditionFalse,
			}},
			want: boolPointer(false),
		},
		{
			name: "pressure",
			conditions: []corev1.NodeCondition{{
				Type: corev1.NodeDiskPressure, Status: corev1.ConditionTrue,
			}},
			want: boolPointer(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := diskPressure(&corev1.Node{Status: corev1.NodeStatus{Conditions: tt.conditions}})
			if tt.want == nil {
				if got != nil {
					t.Fatalf("diskPressure() = %v, want nil", *got)
				}
				return
			}
			if got == nil || *got != *tt.want {
				t.Fatalf("diskPressure() = %v, want %v", got, *tt.want)
			}
		})
	}
}

func boolPointer(value bool) *bool {
	return &value
}
