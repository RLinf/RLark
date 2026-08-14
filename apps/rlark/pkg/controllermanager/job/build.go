package job

import (
	"context"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

func (r *Reconciler) resolveTaskNamespace(ctx context.Context, t *rlarkv1alpha1.JobTaskTemplate) string {
	if len(t.NodeSelector) == 0 {
		return "default"
	}

	simpleSelector := make(map[string]string)
	for k, v := range t.NodeSelector {
		simpleSelector[k] = strings.Split(v, ",")[0] // nodeSelector 的 value 可能是逗号分隔的多个值，取一个即可
	}

	selector := labels.SelectorFromSet(simpleSelector)

	var nodeList rlarkv1alpha1.NodeList
	if err := r.List(ctx, &nodeList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return "default"
	}

	if len(nodeList.Items) == 0 {
		return "default"
	}

	return nodeList.Items[0].Namespace
}

func buildTaskStatusMap(job *rlarkv1alpha1.Job) map[string]*rlarkv1alpha1.JobTaskStatus {
	m := make(map[string]*rlarkv1alpha1.JobTaskStatus, len(job.Status.Tasks))
	for i := range job.Status.Tasks {
		m[job.Status.Tasks[i].Name] = &job.Status.Tasks[i]
	}
	return m
}

func buildTask(
	job *rlarkv1alpha1.Job,
	t rlarkv1alpha1.JobTaskTemplate,
	taskName, namespace string,
) *rlarkv1alpha1.Task {
	taskSpec := t.TaskSpec
	taskSpec.Domain = job.Spec.Domain
	taskSpec.SSHPublicKey = job.Spec.SSHPublicKey

	if job.Spec.Stopped && taskSpec.Kubernetes != nil && taskSpec.Kubernetes.Workload != nil {
		taskSpec.Kubernetes.Workload.Replicas = ptr.To(int32(0))
	}

	return &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskName,
			Namespace: namespace,
			Labels: map[string]string{
				"rlinf.io/job": job.Name,
			},
			Annotations: buildRayAnnotations(job, t),
		},
		Spec: taskSpec,
	}
}

func buildRayAnnotations(job *rlarkv1alpha1.Job, t rlarkv1alpha1.JobTaskTemplate) map[string]string {
	annotations := map[string]string{
		rlarkv1alpha1.RayTotalNodesAnnotation:    strconv.Itoa(totalNodeCount(job.Spec.Tasks)),
		rlarkv1alpha1.RayNodeRankStartAnnotation: strconv.Itoa(rankStartForTask(job, t.Name)),
	}
	if t.Head {
		annotations[rlarkv1alpha1.RayRoleAnnotation] = rlarkv1alpha1.RayRoleHead
	} else {
		annotations[rlarkv1alpha1.RayRoleAnnotation] = rlarkv1alpha1.RayRoleWorker
	}
	if headTaskName := findHeadTaskName(job); headTaskName != "" {
		annotations[rlarkv1alpha1.RayHeadTaskNameAnnotation] = headTaskName
	}
	return annotations
}

func rankStartForTask(job *rlarkv1alpha1.Job, taskName string) int {
	rank := 0
	for _, t := range job.Spec.Tasks {
		if t.Name == taskName {
			break
		}
		if t.Kubernetes != nil && t.Kubernetes.Workload != nil && t.Kubernetes.Workload.Replicas != nil {
			rank += int(*t.Kubernetes.Workload.Replicas)
		} else {
			rank += 1
		}
	}
	return rank
}

func findHeadTaskName(job *rlarkv1alpha1.Job) string {
	for _, t := range job.Spec.Tasks {
		if t.Head {
			return utils.ChildName(job.Name, t.Name)
		}
	}
	return ""
}

func totalNodeCount(tasks []rlarkv1alpha1.JobTaskTemplate) int {
	total := 0
	for _, t := range tasks {
		if t.Kubernetes != nil && t.Kubernetes.Workload != nil && t.Kubernetes.Workload.Replicas != nil {
			total += int(*t.Kubernetes.Workload.Replicas)
		} else {
			total += 1
		}
	}
	return total
}
