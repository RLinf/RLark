package workflow

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
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
			Annotations: map[string]string{
				utils.ParentUIDAnnotation:     string(wf.UID),
				utils.ChildTemplateAnnotation: jt.Name,
			},
		},
		Spec: jt.Spec,
	}
}

func syncJobStatusSnapshot(wf *rlarkv1alpha1.Workflow) bool {
	existing := buildJobStatusMap(wf)
	next := make([]rlarkv1alpha1.WorkflowJobStatus, len(wf.Spec.JobTemplates))
	for i, template := range wf.Spec.JobTemplates {
		next[i].Name = template.Name
		if status := existing[template.Name]; status != nil {
			next[i] = *status
		}
	}
	if reflect.DeepEqual(wf.Status.Jobs, next) {
		return false
	}
	wf.Status.Jobs = next
	return true
}
