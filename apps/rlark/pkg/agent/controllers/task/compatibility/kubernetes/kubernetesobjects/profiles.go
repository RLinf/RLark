package kubernetesobjects

import (
	"encoding/json"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// Typed Kubernetes reconcile profiles selected by Workload Adapters and
// executed by compatible Runtime Providers.
var (
	WorkloadReconcileProfile = agentruntime.ObjectKind{Group: "kubernetes.execution.rlark.io", Version: "v1", Kind: "WorkloadProjection"}
	ServiceReconcileProfile  = agentruntime.ObjectKind{Group: "kubernetes.execution.rlark.io", Version: "v1", Kind: "ServiceProjection"}
	SharedReconcileProfile   = agentruntime.ObjectKind{Group: "kubernetes.execution.rlark.io", Version: "v1", Kind: "SharedFieldProjection"}
	SecretReconcileProfile   = agentruntime.ObjectKind{Group: "kubernetes.execution.rlark.io", Version: "v1", Kind: "TaskSecretProjection"}
	ConfigReconcileProfile   = agentruntime.ObjectKind{Group: "kubernetes.execution.rlark.io", Version: "v1", Kind: "TaskConfigProjection"}
)

type serviceProjection struct {
	Labels                   map[string]string
	Annotations              map[string]string
	Selector                 map[string]string
	Ports                    []servicePortProjection
	PublishNotReadyAddresses bool
	Type                     corev1.ServiceType
}

type servicePortProjection struct {
	Name        string
	Protocol    corev1.Protocol
	Port        int32
	TargetPort  intstr.IntOrString
	AppProtocol *string
}

// ServiceProjectionEqual compares only Adapter-owned Service fields. Kubernetes
// defaults and allocated fields outside this projection do not cause drift.
func ServiceProjectionEqual(current, desired agentruntime.TypedManifest) (bool, error) {
	var existing, want corev1.Service
	if err := json.Unmarshal(current.Object, &existing); err != nil {
		return false, err
	}
	if err := json.Unmarshal(desired.Object, &want); err != nil {
		return false, err
	}
	return reflect.DeepEqual(projectService(&existing, &want), projectService(&want, &want)), nil
}

func projectService(service, ownership *corev1.Service) serviceProjection {
	ports := make([]servicePortProjection, len(service.Spec.Ports))
	for i := range service.Spec.Ports {
		port := service.Spec.Ports[i]
		protocol := port.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		targetPort := port.TargetPort
		if targetPort == (intstr.IntOrString{}) {
			targetPort = intstr.FromInt32(port.Port)
		}
		ports[i] = servicePortProjection{Name: port.Name, Protocol: protocol, Port: port.Port, TargetPort: targetPort, AppProtocol: port.AppProtocol}
	}
	serviceType := service.Spec.Type
	if serviceType == "" {
		serviceType = corev1.ServiceTypeClusterIP
	}
	return serviceProjection{Labels: projectStringMap(service.Labels, ownership.Labels), Annotations: projectStringMap(service.Annotations, ownership.Annotations), Selector: service.Spec.Selector, Ports: ports, PublishNotReadyAddresses: service.Spec.PublishNotReadyAddresses, Type: serviceType}
}

func projectStringMap(values, owned map[string]string) map[string]string {
	if len(owned) == 0 {
		return nil
	}
	result := make(map[string]string, len(owned))
	for key := range owned {
		result[key] = values[key]
	}
	return result
}
