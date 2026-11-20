package job

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
)

// perNodeReplicas: a per-node Task counts 0 replicas. Its number of instances follows the members of its class of
// nodes; a Ray head cannot wait for a number that changes, and per-node instances have no stable rank.
const perNodeReplicas = 0

// resolvePerNodeNamespace: a per-node Task is one DaemonSet in one cluster, so it goes to the
// cluster its class of nodes is in. Every matching Node counts, whatever
// its phase or its cluster's state: a node that is offline or cordoned is
// still in the class, and its going away must not move the Task.
func (r *Reconciler) resolvePerNodeNamespace(ctx context.Context, t *rlarkv1alpha1.JobTaskTemplate) (string, error) {
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

// validatePerNodeTemplate checks a template's per-node placement against its carrier.
func validatePerNodeTemplate(template *rlarkv1alpha1.JobTaskTemplate, path string) error {
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
	return nil
}

// setDeclaredSSHEnvironment gives the Job's key to the Task's own SSH
// environment, when its template declares one, keeping the port it names (2222
// when it names none). Tasks whose pods share a node's network (a robot's device
// layer and its station) cannot all listen on the same port, so their template
// says which. Of several declared SSH environments the last one is replaced.
func setDeclaredSSHEnvironment(spec *rlarkv1alpha1.TaskSpec, publicKey string) bool {
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
	if index < 0 {
		return false
	}
	parameters, _ := json.Marshal(taskapi.SSHEnvironmentParametersV1{AuthorizedKeys: []string{publicKey}, Port: port})
	spec.Environments[index] = rlarkv1alpha1.TaskEnvironment{Type: taskapi.EnvironmentTypeSSH, Version: taskapi.EnvironmentVersionSSHV1, Parameters: apiextensionsv1.JSON{Raw: parameters}}
	return true
}

// A Job without a head forms no Ray cluster: its Tasks have no Ray role and only run their own scripts, rather
// than waiting as workers for a head that never comes. The official code makes every Task that is not the head a
// worker; these take the role off again where there is no head to work for.

func clearRoleWithoutHead(parameters *taskapi.RayExecutionParametersV1) {
	if parameters.Role == taskapi.RayRoleWorker && parameters.HeadTaskName == "" {
		parameters.Role = ""
	}
}

func dropRoleWithoutHead(annotations map[string]string) {
	if annotations[rlarkv1alpha1.RayRoleAnnotation] == rlarkv1alpha1.RayRoleWorker && annotations[rlarkv1alpha1.RayHeadTaskNameAnnotation] == "" {
		delete(annotations, rlarkv1alpha1.RayRoleAnnotation)
	}
}
