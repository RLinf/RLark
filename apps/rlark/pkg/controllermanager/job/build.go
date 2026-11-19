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
	"k8s.io/apimachinery/pkg/selection"
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
	if t.Placement.PerNode {
		// A per-node Task is one DaemonSet in one cluster, so it goes to the
		// cluster its class of nodes is in. Every matching Node counts, whatever
		// its phase or its cluster's state: a node that is offline or cordoned is
		// still in the class, and its going away must not move the Task.
		selector := labels.NewSelector()
		for key, value := range t.Placement.NodeSelector {
			requirement, err := labels.NewRequirement(key, selection.In, strings.Split(value, ","))
			if err != nil {
				return "", fmt.Errorf("nodeSelector of per-node Task template %q: %w", t.Name, err)
			}
			selector = selector.Add(*requirement)
		}
		var nodeList rlarkv1alpha1.NodeList
		if err := r.List(ctx, &nodeList, &client.ListOptions{LabelSelector: selector}); err != nil {
			return "", fmt.Errorf("list Nodes matching selector for Task template %q: %w", t.Name, err)
		}
		namespaces := map[string]struct{}{}
		for _, node := range nodeList.Items {
			namespaces[node.Namespace] = struct{}{}
		}
		switch len(namespaces) {
		case 0:
			return "", fmt.Errorf("no Nodes match selector for per-node Task template %q", t.Name)
		case 1:
			return nodeList.Items[0].Namespace, nil
		default:
			return "", fmt.Errorf("selector of per-node Task template %q matches Nodes of %d clusters; a per-node Task runs in one cluster, so narrow its nodeSelector to one", t.Name, len(namespaces))
		}
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

	var clusterList rlarkv1alpha1.ClusterList
	if err := r.List(ctx, &clusterList); err != nil {
		return "", fmt.Errorf("list Clusters for Task template %q: %w", t.Name, err)
	}
	clustersByNamespace := make(map[string][]rlarkv1alpha1.Cluster)
	for i := range clusterList.Items {
		cluster := clusterList.Items[i]
		clustersByNamespace[cluster.Spec.AgentNamespace] = append(clustersByNamespace[cluster.Spec.AgentNamespace], cluster)
	}

	eligible := nodeList.Items[:0]
	for _, node := range nodeList.Items {
		if node.Spec.Unschedulable || node.Status.Phase != rlarkv1alpha1.NodeOnline {
			continue
		}
		clusters := clustersByNamespace[node.Namespace]
		if len(clusters) != 1 {
			continue
		}
		cluster := clusters[0]
		if !cluster.DeletionTimestamp.IsZero() || cluster.Spec.Cordoned || cluster.Spec.Draining || cluster.Spec.Delisted || (cluster.Status.Phase != rlarkv1alpha1.ClusterOnline && cluster.Status.Phase != rlarkv1alpha1.ClusterDowngraded) {
			continue
		}
		eligible = append(eligible, node)
	}
	if len(eligible) == 0 {
		return "", fmt.Errorf("no schedulable Nodes in an Online or Downgraded Cluster match selector for Task template %q", t.Name)
	}

	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Namespace == eligible[j].Namespace {
			return eligible[i].Name < eligible[j].Name
		}
		return eligible[i].Namespace < eligible[j].Namespace
	})
	return eligible[0].Namespace, nil
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

// setSSHEnvironment gives the Task the Job's key and an sshd port. The port is
// 2222 unless the Task's own SSH environment names one: Tasks whose pods share
// a node's network (a robot's device layer and its station) cannot all listen
// on the same port, so their template says which.
func setSSHEnvironment(spec *rlarkv1alpha1.TaskSpec, publicKey string) {
	port := int32(2222)
	index := -1
	for i := range spec.Environments {
		if spec.Environments[i].Type == taskapi.EnvironmentTypeSSH && spec.Environments[i].Version == taskapi.EnvironmentVersionSSHV1 {
			index = i
			var declared taskapi.SSHEnvironmentParametersV1
			if err := json.Unmarshal(spec.Environments[i].Parameters.Raw, &declared); err == nil && declared.Port >= 1 && declared.Port <= 65535 {
				port = declared.Port
			}
		}
	}
	parameters, _ := json.Marshal(taskapi.SSHEnvironmentParametersV1{AuthorizedKeys: []string{publicKey}, Port: port})
	environment := rlarkv1alpha1.TaskEnvironment{Type: taskapi.EnvironmentTypeSSH, Version: taskapi.EnvironmentVersionSSHV1, Parameters: apiextensionsv1.JSON{Raw: parameters}}
	if index >= 0 {
		spec.Environments[index] = environment
		return
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
	parameters.Role = ""
	parameters.HeadTaskName = findHeadTaskName(job)
	parameters.TotalNodes = int32(totalNodeCount(job.Spec.Tasks))
	parameters.NodeRankStart = int32(rankStartForTask(job, template.Name))
	// A Job without a head forms no Ray cluster: its Tasks have no Ray role and
	// only run their own scripts, rather than waiting as workers for a head
	// that never comes.
	switch {
	case template.Head:
		parameters.Role = taskapi.RayRoleHead
		parameters.HeadTaskName = ""
	case parameters.HeadTaskName != "":
		parameters.Role = taskapi.RayRoleWorker
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
	headTaskName := findHeadTaskName(job)
	switch {
	case t.Head:
		annotations[rlarkv1alpha1.RayRoleAnnotation] = rlarkv1alpha1.RayRoleHead
	case headTaskName != "":
		annotations[rlarkv1alpha1.RayRoleAnnotation] = rlarkv1alpha1.RayRoleWorker
	}
	if headTaskName != "" {
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
// 按节点放置的 Task 计 0：它的实例数随节点类别的成员变化，Ray 头节点不能等待
// 一个会变的数，按节点的实例也没有稳定的 rank。
func taskReplicas(t rlarkv1alpha1.JobTaskTemplate) int32 {
	if t.Placement.PerNode {
		return 0
	}
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

		perNode := template.Placement.PerNode
		daemonSet := template.Workload.Carrier.Type == taskapi.CarrierTypeKubernetesDaemonSet
		switch {
		case perNode && len(template.Placement.NodeSelector) == 0:
			return fmt.Errorf("%s.placement.perNode requires placement.nodeSelector to name the class of nodes", path)
		case perNode && template.Head:
			// The head is the one instance the Job's other Tasks reach; a
			// per-node Task has one instance per node.
			return fmt.Errorf("%s.head must be false for a per-node Task", path)
		case perNode && template.Runtime.Kind == rlarkv1alpha1.RuntimeKindKubernetes && (!daemonSet || template.Workload.Carrier.Version != taskapi.CarrierVersionKubernetesV1):
			return fmt.Errorf("%s.workload.carrier must be %s %s for a per-node Task", path, taskapi.CarrierTypeKubernetesDaemonSet, taskapi.CarrierVersionKubernetesV1)
		case daemonSet && !perNode:
			return fmt.Errorf("%s.workload.carrier %s requires placement.perNode", path, taskapi.CarrierTypeKubernetesDaemonSet)
		}
		if daemonSet {
			if _, err := taskapi.DecodeKubernetesDaemonSetCarrierParameters(template.Workload.Carrier.Parameters); err != nil {
				return fmt.Errorf("%s.workload.carrier.parameters: %w", path, err)
			}
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
