package workflow

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func buildJobStatusMap(wf *rlarkv1alpha1.Workflow) map[string]*rlarkv1alpha1.WorkflowJobStatus {
	m := make(map[string]*rlarkv1alpha1.WorkflowJobStatus, len(wf.Status.Jobs))
	for i := range wf.Status.Jobs {
		m[wf.Status.Jobs[i].Name] = &wf.Status.Jobs[i]
	}
	return m
}

func buildJob(wf *rlarkv1alpha1.Workflow, jt rlarkv1alpha1.WorkflowJobTemplate, jobName string) *rlarkv1alpha1.Job {
	return &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: jobName,
			Labels: map[string]string{
				"rlinf.io/workflow": wf.Name,
			},
		},
		Spec: jt.Spec,
	}
}
