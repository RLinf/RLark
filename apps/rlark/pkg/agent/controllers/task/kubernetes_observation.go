package task

import (
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/observation"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

type kubernetesObservation struct {
	observation.Kubernetes
	name       string
	typeObject client.Object
	newObject  func() client.Object
}

// KubernetesObservationsFromImplementations projects Adapter-provided Runtime
// observation contracts into the Kubernetes watcher boundary.
func KubernetesObservationsFromImplementations(registry host.ImplementationRegistry, runtimeKind agentruntime.Kind) ([]observation.Kubernetes, error) {
	if registry == nil {
		return nil, fmt.Errorf("implementation registry is required")
	}
	var result []observation.Kubernetes
	for _, descriptor := range registry.Describe() {
		if descriptor.Key.RuntimeKind != runtimeKind {
			continue
		}
		for _, contract := range descriptor.Info.Observations {
			if contract.RuntimeKind != runtimeKind {
				continue
			}
			for _, root := range contract.Roots {
				result = append(result, observation.Kubernetes{GVK: schema.GroupVersionKind{Group: root.Group, Version: root.Version, Kind: root.Kind}, CarrierType: root.CarrierType, InstanceProfile: root.InstanceProfile})
			}
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no Kubernetes workload observations are registered")
	}
	return result, nil
}

// WithKubernetesObservations validates and installs the immutable set of
// Kubernetes workload roots observed by this Controller.
func (c *Controller) WithKubernetesObservations(scheme *runtime.Scheme, mapper meta.RESTMapper, registrations []observation.Kubernetes) error {
	if c.observationScope != nil {
		return fmt.Errorf("cannot replace configured observation sources")
	}
	observations, byID, err := newKubernetesObservations(scheme, mapper, registrations)
	if err != nil {
		return err
	}
	c.observations = observations
	c.observationsByID = byID
	return nil
}

func newKubernetesObservations(scheme *runtime.Scheme, mapper meta.RESTMapper, registrations []observation.Kubernetes) ([]kubernetesObservation, map[string]kubernetesObservation, error) {
	if scheme == nil {
		return nil, nil, fmt.Errorf("kubernetes observation scheme is required")
	}
	if len(registrations) == 0 {
		return nil, nil, fmt.Errorf("at least one Kubernetes observation is required")
	}
	if mapper == nil {
		return nil, nil, fmt.Errorf("kubernetes observation REST mapper is required")
	}
	result := make([]kubernetesObservation, 0, len(registrations))
	byID := make(map[string]kubernetesObservation, len(registrations))
	carriers := make(map[string]struct{}, len(registrations))
	for _, registration := range registrations {
		if registration.GVK.Empty() || registration.GVK.Version == "" || registration.GVK.Kind == "" {
			return nil, nil, fmt.Errorf("kubernetes observation GVK must include version and kind")
		}
		if strings.TrimSpace(registration.CarrierType) == "" {
			return nil, nil, fmt.Errorf("kubernetes observation %s carrier type is required", registration.GVK)
		}
		id := registration.GVK.String()
		if _, found := byID[id]; found {
			return nil, nil, fmt.Errorf("duplicate Kubernetes observation %s", registration.GVK)
		}
		if _, found := carriers[registration.CarrierType]; found {
			return nil, nil, fmt.Errorf("duplicate Kubernetes observation carrier %q", registration.CarrierType)
		}
		mapping, err := mapper.RESTMapping(registration.GVK.GroupKind(), registration.GVK.Version)
		if err != nil {
			return nil, nil, fmt.Errorf("map Kubernetes observation %s: %w", registration.GVK, err)
		}
		if mapping.Scope == nil || mapping.Scope.Name() != meta.RESTScopeNameNamespace {
			return nil, nil, fmt.Errorf("kubernetes observation %s must be namespaced", registration.GVK)
		}
		typeObject, err := kubernetesObservationTypeObject(scheme, registration.GVK)
		if err != nil {
			return nil, nil, err
		}
		prototype := typeObject.DeepCopyObject().(client.Object)
		entry := kubernetesObservation{
			Kubernetes: registration,
			name:       observationName(registration.GVK),
			typeObject: typeObject,
			newObject: func() client.Object {
				return prototype.DeepCopyObject().(client.Object)
			},
		}
		result = append(result, entry)
		byID[id] = entry
		carriers[registration.CarrierType] = struct{}{}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].name < result[j].name })
	return result, byID, nil
}

func observationName(gvk schema.GroupVersionKind) string {
	group := gvk.Group
	if group == "" {
		group = "core"
	}
	replacer := strings.NewReplacer(".", "-", "/", "-", "_", "-")
	return "task-runtime-" + strings.ToLower(replacer.Replace(group+"-"+gvk.Version+"-"+gvk.Kind))
}

func kubernetesObservationTypeObject(scheme *runtime.Scheme, gvk schema.GroupVersionKind) (client.Object, error) {
	object, err := scheme.New(gvk)
	if runtime.IsNotRegisteredError(err) {
		result := &unstructured.Unstructured{}
		result.SetGroupVersionKind(gvk)
		return result, nil
	}
	if err != nil {
		return nil, fmt.Errorf("construct Kubernetes observation %s: %w", gvk, err)
	}
	result, ok := object.(client.Object)
	if !ok {
		return nil, fmt.Errorf("kubernetes observation %s is not an object", gvk)
	}
	return result, nil
}
