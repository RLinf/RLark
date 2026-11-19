package kubernetesobjects_test

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
)

func TestLegacyNamesAndMetadataRemainStable(t *testing.T) {
	if kubernetesobjects.ManagementTaskUIDAnnotation != "rlark.io/management-task-uid" {
		t.Fatal("legacy metadata keys changed")
	}
}

func TestWorkloadAnnotationsDoNotInjectNetworkDomain(t *testing.T) {
	t.Parallel()

	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "cluster", UID: "uid", Generation: 2},
	}
	annotations := kubernetesobjects.WorkloadAnnotations(task)
	if _, found := annotations[kubernetesobjects.ManagementTaskDomainAnnotation]; found {
		t.Fatalf("identity annotations contained network domain: %v", annotations)
	}
}
