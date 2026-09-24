package gateway

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestBuildClusterInfoUsesCityMetadata(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID: "cluster-a",
				"rlark.io/city":              "label-city",
				"rlark.io/location":          "legacy-location",
			},
			Annotations: map[string]string{
				"rlark.io/city":        "annotation-city",
				"rlark.io/ip-location": `{"city":"legacy-ip-city"}`,
			},
		},
	}}

	info := buildClusterInfo("cluster-a", nodes)
	if info.Location != "annotation-city" {
		t.Fatalf("expected annotation city, got %q", info.Location)
	}
}

func TestBuildClusterInfoFallsBackToCityLabel(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			rlarkv1alpha1.LabelClusterID: "cluster-a",
			"rlark.io/city":              "label-city",
		}},
	}}

	info := buildClusterInfo("cluster-a", nodes)
	if info.Location != "label-city" {
		t.Fatalf("expected label city, got %q", info.Location)
	}
}

// 回归：批量编辑使用布尔位 label（rlark.io/node-category-cloud=true），
// 而不是枚举值 label（rlark.io/node-category=cloud）。buildClusterInfo
// 必须识别布尔位，否则 Type 会错判为 Hybrid。
func TestBuildClusterInfoBooleanCategoryLabel(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID:               "cluster-a",
				rlarkv1alpha1.LabelNodeCategory + "-cloud": "true",
			}},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID:               "cluster-a",
				rlarkv1alpha1.LabelNodeCategory + "-cloud": "true",
			}},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
	}

	info := buildClusterInfo("cluster-a", nodes)
	if info.Type != "Cloud" {
		t.Fatalf("expected Cloud, got %q (cloud=%d embodied=%d robots=%d)",
			info.Type, info.CloudNodes, info.EmbodiedNodes, info.Robots)
	}
	if info.CloudNodes != 2 {
		t.Fatalf("expected 2 cloud nodes, got %d", info.CloudNodes)
	}
}

// 两种 label 形式同时存在时也要合并识别（去重）
func TestBuildClusterInfoMixedCategoryLabelForms(t *testing.T) {
	nodes := []rlarkv1alpha1.Node{
		{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID:    "cluster-a",
				rlarkv1alpha1.LabelNodeCategory: "cloud,edge",
			}},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
				rlarkv1alpha1.LabelClusterID:               "cluster-a",
				rlarkv1alpha1.LabelNodeCategory + "-robot": "true",
			}},
			Status: rlarkv1alpha1.NodeStatus{Phase: rlarkv1alpha1.NodeOnline},
		},
	}

	info := buildClusterInfo("cluster-a", nodes)
	if info.Type != "Hybrid" {
		t.Fatalf("expected Hybrid, got %q (cloud=%d embodied=%d robots=%d)",
			info.Type, info.CloudNodes, info.EmbodiedNodes, info.Robots)
	}
	if info.CloudNodes != 1 || info.EmbodiedNodes != 1 || info.Robots != 1 {
		t.Fatalf("expected 1/1/1, got %d/%d/%d",
			info.CloudNodes, info.EmbodiedNodes, info.Robots)
	}
}
