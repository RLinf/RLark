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
