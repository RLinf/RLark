package framework

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (r *Run) ListOptions() metav1.ListOptions {
	return metav1.ListOptions{LabelSelector: fmt.Sprintf("%s=%s", r.RunIDLabel, r.ID)}
}

func (r *Run) ProvesOwnership(labels map[string]string) bool {
	return r.RunIDLabel != "" && labels[r.RunIDLabel] == r.ID
}
