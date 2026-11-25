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

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// nodeMatchesSelector reports whether a Node's observed labels or
// administrator-maintained annotations satisfy every entry in selector. Each
// selector value may be a comma-separated list; the Node matches if the
// corresponding label or annotation equals any one of the listed values.
func nodeMatchesSelector(node *rlarkv1alpha1.Node, selector map[string]string) bool {
	labels := node.Status.ObservedNodeLabels
	if labels == nil {
		labels = map[string]string{}
	}
	for key, rawValue := range selector {
		nodeValue, ok := labels[key]
		if !ok {
			// Administrator metadata such as rlark.io/node-category-* lives on
			// the management Node CR as annotations, not on the data plane.
			nodeValue, ok = node.Annotations[key]
		}
		if !ok {
			return false
		}
		values := strings.Split(rawValue, ",")
		matched := false
		for _, v := range values {
			if v == nodeValue {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func (r *Reconciler) resolveTaskNamespace(ctx context.Context, t *rlarkv1alpha1.JobTaskTemplate) (string, error) {
	var nodeList rlarkv1alpha1.NodeList
	if err := r.List(ctx, &nodeList); err != nil {
		return "", fmt.Errorf("list Nodes for Task template %q: %w", t.Name, err)
	}
	if len(nodeList.Items) == 0 {
		return "", fmt.Errorf("no Nodes available for Task template %q", t.Name)
	}

	// Filter using observedNodeLabels plus administrator annotations: the
	// management Node CR's metadata.labels only carries RLark-owned system
	// labels, so Kubernetes LabelSelector cannot find data-plane labels or
	// managed annotations such as gpu-model or rlark.io/node-category-*.
	selector := t.Placement.NodeSelector
	filtered := make([]rlarkv1alpha1.Node, 0, len(nodeList.Items))
	for i := range nodeList.Items {
		if len(selector) == 0 || nodeMatchesSelector(&nodeList.Items[i], selector) {
			filtered = append(filtered, nodeList.Items[i])
		}
	}
	if len(filtered) == 0 {
		return "", fmt.Errorf("no Nodes match selector for Task template %q", t.Name)
	}
	nodeList.Items = filtered

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
		if !nodeReadyForScheduling(node.Status.Conditions) || !nodeSupportsTask(node, t) {
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
		return "", fmt.Errorf("no Node with Schedulable=True and supported runtime and device capacity in an Online or Downgraded Cluster matches selector for Task template %q", t.Name)
	}

	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].Namespace == eligible[j].Namespace {
			return eligible[i].Name < eligible[j].Name
		}
		return eligible[i].Namespace < eligible[j].Namespace
	})
	return eligible[0].Namespace, nil
}

// nodeReadyForScheduling reports whether a Node may receive new placement. The
// Node Controller owns this decision in the Schedulable condition; the scheduler
// must not re-derive it from individual conditions, or its filter can drift from
// what the console and other consumers report.
func nodeReadyForScheduling(conditions []metav1.Condition) bool {
	return conditionIsTrue(conditions, rlarkv1alpha1.ConditionTypeSchedulable)
}

func nodeSupportsTask(node rlarkv1alpha1.Node, task *rlarkv1alpha1.JobTaskTemplate) bool {
	capabilities := node.Status.CapabilitySnapshot
	if capabilities.Runtime.Type == "" && len(capabilities.Runtime.WorkloadKinds) == 0 && len(capabilities.Devices) == 0 {
		// Compatibility: an Agent that has not reported a capability snapshot yet (for
		// example during a rolling upgrade) cannot be filtered by capability. Fall back
		// to the declared Node runtime so existing workloads keep scheduling.
		return legacyNodeSupportsTask(node, task)
	}
	if capabilities.Runtime.Type != rlarkv1alpha1.AgentType(task.Runtime.Kind) || !stringInSlice(string(task.Workload.Kind), capabilities.Runtime.WorkloadKinds) {
		return false
	}
	if task.Workload.Template == nil {
		return true
	}
	return taskFitsDevices(capabilities.Devices, taskDeviceRequests(task))
}

func legacyNodeSupportsTask(node rlarkv1alpha1.Node, task *rlarkv1alpha1.JobTaskTemplate) bool {
	runtimeType := node.Spec.Runtime.Type
	if runtimeType == "" {
		//nolint:staticcheck // Compatibility: read the deprecated Node field for resources that predate Runtime.Type.
		runtimeType = node.Spec.AgentType
	}
	if runtimeType != "" && runtimeType != rlarkv1alpha1.AgentType(task.Runtime.Kind) {
		return false
	}
	return true
}

func taskDeviceRequests(task *rlarkv1alpha1.JobTaskTemplate) map[string]int64 {
	requests := make(map[string]int64)
	if task.Workload.Template == nil {
		return requests
	}
	// Pod-level resources describe the whole Pod, so they replace the per-container sum.
	if task.Workload.Template.Spec.Resources != nil {
		addExtendedRequests(requests, task.Workload.Template.Spec.Resources.Requests)
		return requests
	}
	for _, container := range task.Workload.Template.Spec.InitContainers {
		addExtendedRequests(requests, container.Resources.Requests)
	}
	for _, container := range task.Workload.Template.Spec.Containers {
		addExtendedRequests(requests, container.Resources.Requests)
	}
	return requests
}

func addExtendedRequests(target map[string]int64, resources corev1.ResourceList) {
	for name, quantity := range resources {
		if strings.Contains(string(name), "/") {
			target[string(name)] += quantity.Value()
		}
	}
}

func taskFitsDevices(devices []rlarkv1alpha1.NodeDeviceCapability, requests map[string]int64) bool {
	for name, requested := range requests {
		available := int64(0)
		for _, device := range devices {
			if device.Category == name {
				available = int64(device.Available)
				break
			}
		}
		if available < requested {
			return false
		}
	}
	return true
}

func stringInSlice(value string, values []string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func conditionIsTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.Status == metav1.ConditionTrue
		}
	}
	return false
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
	// DrainRequests is a one-shot projection owned by the Node Agent. A Task
	// template may carry stale entries (for example when a Job is cloned or
	// restored), so never copy them into a new Task.
	taskSpec.DrainRequests = nil
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
	parameters, _ := json.Marshal(taskapi.SSHEnvironmentParametersV1{AuthorizedKeys: []string{publicKey}, Port: 2222})
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
