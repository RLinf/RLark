package job

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

func (r *Reconciler) resolveTaskNamespace(ctx context.Context, t *rlarkv1alpha1.JobTaskTemplate) (string, error) {
	if len(t.Placement.NodeSelector) == 0 {
		return "default", nil
	}

	simpleSelector := make(map[string]string)
	for k, v := range t.Placement.NodeSelector {
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
	taskSpec := *t.TaskSpec.DeepCopy()
	setNetworkEnvironment(&taskSpec, job.Spec.Domain)
	setSSHEnvironment(&taskSpec, job.Spec.SSHPublicKey)
	setRayExecution(&taskSpec, job, t)
	taskSpec.Tags = make([]rlarkv1alpha1.JobTag, len(job.Spec.Tags))
	for i, tag := range job.Spec.Tags {
		taskSpec.Tags[i] = rlarkv1alpha1.JobTag{
			Key:    tag.Key,
			Values: append([]string(nil), tag.Values...),
		}
	}

	taskSpec.Suspended = job.Spec.Suspended

	result := &rlarkv1alpha1.Task{
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
	tenant.Inherit(job, result)
	return result
}

func setNetworkEnvironment(spec *rlarkv1alpha1.TaskSpec, domain string) {
	filtered := make([]rlarkv1alpha1.TaskEnvironment, 0, len(spec.Environments)+1)
	for _, environment := range spec.Environments {
		if environment.Type == taskapi.EnvironmentTypeNetwork && environment.Version == taskapi.EnvironmentVersionNetworkV1 {
			continue
		}
		filtered = append(filtered, environment)
	}
	spec.Environments = filtered
	if domain == "" {
		return
	}
	parameters, _ := json.Marshal(taskapi.NetworkEnvironmentParametersV1{Domain: domain})
	spec.Environments = append(spec.Environments, rlarkv1alpha1.TaskEnvironment{
		Type:       taskapi.EnvironmentTypeNetwork,
		Version:    taskapi.EnvironmentVersionNetworkV1,
		Parameters: apiextensionsv1.JSON{Raw: parameters},
	})
}

func setSSHEnvironment(spec *rlarkv1alpha1.TaskSpec, publicKey string) {
	parameters, _ := json.Marshal(taskapi.SSHEnvironmentParametersV1{AuthorizedKeys: []string{publicKey}})
	environment := rlarkv1alpha1.TaskEnvironment{Type: taskapi.EnvironmentTypeSSH, Version: taskapi.EnvironmentVersionSSHV1, Parameters: apiextensionsv1.JSON{Raw: parameters}}
	for i := range spec.Environments {
		if spec.Environments[i].Type == environment.Type && spec.Environments[i].Version == environment.Version {
			spec.Environments[i] = environment
			return
		}
	}
	spec.Environments = append(spec.Environments, environment)
}

func setRayExecution(spec *rlarkv1alpha1.TaskSpec, job *rlarkv1alpha1.Job, template rlarkv1alpha1.JobTaskTemplate) {
	workload := &spec.Workload
	if workload.Kind != "" && workload.Kind != taskapi.WorkloadTypeRay {
		return
	}
	if workload.Version != "" && workload.Version != taskapi.WorkloadVersionRayV1 {
		return
	}
	parameters := taskapi.RayExecutionParametersV1{}
	if len(bytes.TrimSpace(workload.Parameters.Raw)) != 0 {
		decoded, err := taskapi.DecodeParametersStrict[taskapi.RayExecutionParametersV1](workload.Parameters)
		if err != nil {
			return
		}
		parameters = decoded
	}
	parameters.Role = taskapi.RayRoleWorker
	parameters.HeadTaskName = findHeadTaskName(job)
	parameters.TotalNodes = int32(totalNodeCount(job.Spec.Tasks))
	parameters.NodeRankStart = int32(rankStartForTask(job, template.Name))
	if template.Head {
		parameters.Role = taskapi.RayRoleHead
		parameters.HeadTaskName = ""
	}
	raw, _ := json.Marshal(parameters)
	workload.Kind = taskapi.WorkloadTypeRay
	workload.Version = taskapi.WorkloadVersionRayV1
	workload.Parameters = apiextensionsv1.JSON{Raw: raw}
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
		rank += int(taskReplicas(t))
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
	if t.Workload.Carrier.Type != taskapi.CarrierTypeKubernetesDeployment && t.Workload.Carrier.Type != taskapi.CarrierTypeKubernetesStatefulSet {
		return 1
	}
	parameters, err := taskapi.DecodeParametersStrict[taskapi.KubernetesReplicatedCarrierParametersV1](t.Workload.Carrier.Parameters)
	if err == nil && parameters.Replicas != nil {
		return *parameters.Replicas
	}
	return 1
}

// validateTaskTemplates checks the common structure required before the Job
// controller can safely derive Task identities and dispatch complete Task
// declarations. Runtime, workload, and carrier identifiers remain open strings;
// the Agent validates support and strictly decodes their parameters.
func validateTaskTemplates(job *rlarkv1alpha1.Job) error {
	names := make(map[string]struct{}, len(job.Spec.Tasks))
	for i := range job.Spec.Tasks {
		template := &job.Spec.Tasks[i]
		path := fmt.Sprintf("spec.tasks[%d]", i)
		name := strings.TrimSpace(template.Name)
		if name == "" {
			return fmt.Errorf("%s.name must not be empty", path)
		}
		if _, found := names[name]; found {
			return fmt.Errorf("%s.name %q is duplicated", path, name)
		}
		names[name] = struct{}{}

		if strings.TrimSpace(string(template.Runtime.Kind)) == "" {
			return fmt.Errorf("%s.runtime.kind must not be empty", path)
		}
		if strings.TrimSpace(string(template.Workload.Kind)) == "" {
			return fmt.Errorf("%s.workload.kind must not be empty", path)
		}
		if strings.TrimSpace(template.Workload.Version) == "" {
			return fmt.Errorf("%s.workload.version must not be empty", path)
		}
		if strings.TrimSpace(template.Workload.Carrier.Type) == "" {
			return fmt.Errorf("%s.workload.carrier.type must not be empty", path)
		}
		if strings.TrimSpace(template.Workload.Carrier.Version) == "" {
			return fmt.Errorf("%s.workload.carrier.version must not be empty", path)
		}

		if template.Workload.Template == nil {
			return fmt.Errorf("%s.workload.template is required by the current Job Ray/Kubernetes implementation", path)
		}
		mainContainers := 0
		for _, container := range template.Workload.Template.Spec.Containers {
			if container.Name == "main" {
				mainContainers++
			}
		}
		if mainContainers != 1 {
			return fmt.Errorf(
				"%s.workload.template.spec.containers must contain exactly one container named %q, got %d",
				path,
				"main",
				mainContainers,
			)
		}
	}
	return nil
}

func totalNodeCount(tasks []rlarkv1alpha1.JobTaskTemplate) int {
	total := 0
	for _, t := range tasks {
		total += int(taskReplicas(t))
	}
	return total
}
