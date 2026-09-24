package job

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

func (r *Reconciler) resolveTaskNamespace(ctx context.Context, t *rlarkv1alpha1.JobTaskTemplate) (string, error) {
	if len(t.NodeSelector) == 0 {
		return "default", nil
	}

	simpleSelector := make(map[string]string)
	for k, v := range t.NodeSelector {
		simpleSelector[k] = strings.Split(v, ",")[0] // nodeSelector 的 value 可能是逗号分隔的多个值，取一个即可
	}

	selector := labels.SelectorFromSet(simpleSelector)

	var nodeList rlarkv1alpha1.NodeList
	if err := r.List(ctx, &nodeList, &client.ListOptions{LabelSelector: selector}); err != nil {
		return "", fmt.Errorf("list Nodes matching selector for Task template %q: %w", t.Name, err)
	}

	if len(nodeList.Items) == 0 {
		return "", fmt.Errorf("no Nodes match selector for Task template %q", t.Name)
	}

	sort.Slice(nodeList.Items, func(i, j int) bool {
		if nodeList.Items[i].Namespace == nodeList.Items[j].Namespace {
			return nodeList.Items[i].Name < nodeList.Items[j].Name
		}
		return nodeList.Items[i].Namespace < nodeList.Items[j].Namespace
	})
	return nodeList.Items[0].Namespace, nil
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
	taskSpec.Tags = make([]rlarkv1alpha1.JobTag, len(job.Spec.Tags))
	for i, tag := range job.Spec.Tags {
		taskSpec.Tags[i] = rlarkv1alpha1.JobTag{
			Key:    tag.Key,
			Values: append([]string(nil), tag.Values...),
		}
	}

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
			Annotations: utils.MergeAnnotations(buildRayAnnotations(job, t), map[string]string{
				utils.ParentUIDAnnotation:     string(job.UID),
				utils.ChildTemplateAnnotation: t.Name,
			}),
		},
		Spec: taskSpec,
	}
}

func syncTaskStatusSnapshot(job *rlarkv1alpha1.Job) bool {
	existing := buildTaskStatusMap(job)
	next := make([]rlarkv1alpha1.JobTaskStatus, len(job.Spec.Tasks))
	for i, template := range job.Spec.Tasks {
		next[i].Name = template.Name
		if status := existing[template.Name]; status != nil {
			next[i] = *status
		}
	}
	if reflect.DeepEqual(job.Status.Tasks, next) {
		return false
	}
	job.Status.Tasks = next
	return true
}

func buildRayAnnotations(job *rlarkv1alpha1.Job, t rlarkv1alpha1.JobTaskTemplate) map[string]string {
	annotations := map[string]string{
		rlarkv1alpha1.RayTotalNodesAnnotation:    strconv.Itoa(totalNodeCount(job.Spec.Tasks)),
		rlarkv1alpha1.RayNodeRankStartAnnotation: strconv.Itoa(rankStartForTask(job, t.Name)),
	}
	if restartedAt := job.Annotations[RestartedAtAnnotation]; restartedAt != "" {
		annotations[RestartedAtAnnotation] = restartedAt
	}
	if job.Spec.Stopped {
		annotations[StoppedAnnotation] = "true"
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

// taskReplicas 返回任务模板的副本数（pod 数量），未显式设置时默认为 1。
func taskReplicas(t rlarkv1alpha1.JobTaskTemplate) int32 {
	if t.Kubernetes != nil && t.Kubernetes.Workload != nil && t.Kubernetes.Workload.Replicas != nil {
		return *t.Kubernetes.Workload.Replicas
	}
	return 1
}

// validateHeadTask 校验 Job 中被标记为 ray head 的任务。
// ray head 任务只能有一个 pod（replicas == 1），且最多只能有一个 head 任务。
// 校验失败返回描述性错误。
func validateHeadTask(job *rlarkv1alpha1.Job) error {
	headCount := 0
	for _, t := range job.Spec.Tasks {
		if !t.Head {
			continue
		}
		headCount++
		if replicas := taskReplicas(t); replicas != 1 {
			return fmt.Errorf(
				"ray head task %q must have exactly one pod (replicas == 1), got %d",
				t.Name, replicas,
			)
		}
	}
	if headCount > 1 {
		return fmt.Errorf("only one ray head task is allowed, got %d", headCount)
	}
	return nil
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
