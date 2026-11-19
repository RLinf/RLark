package host

import (
	"context"
	"errors"
	"fmt"
	"slices"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

// ErrImplementationNotFound indicates that an exact Workload x Runtime
// implementation was not explicitly registered. It wraps the common Adapter
// not-found category so Coordinator preserves the existing NotFound fault code.
var ErrImplementationNotFound = fmt.Errorf("workload runtime implementation not found: %w", workload.ErrAdapterNotFound)

type implementationRegistry struct {
	adapters    map[ImplementationKey]workload.Adapter
	descriptors []ImplementationDescriptor
}

// NewImplementationRegistry validates registrations and returns an immutable
// registry. Adapter Info must exactly match the registration key.
func NewImplementationRegistry(ctx context.Context, registrations []WorkloadRegistration) (ImplementationRegistry, error) {
	registry := &implementationRegistry{adapters: make(map[ImplementationKey]workload.Adapter, len(registrations))}
	for _, registration := range registrations {
		if registration.Key.WorkloadType == "" || registration.Key.WorkloadVersion == "" || registration.Key.RuntimeKind == "" {
			return nil, errors.New("implementation key type, version, and Runtime kind are required")
		}
		if registration.Adapter == nil {
			return nil, fmt.Errorf("implementation %v has no Adapter", registration.Key)
		}
		if _, exists := registry.adapters[registration.Key]; exists {
			return nil, fmt.Errorf("duplicate implementation registration %v", registration.Key)
		}
		info, err := registration.Adapter.Info(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe implementation %v: %w", registration.Key, err)
		}
		if info.ID != registration.Key.WorkloadType || info.Version != registration.Key.WorkloadVersion || !slices.Contains(info.SupportedRuntimeKinds, registration.Key.RuntimeKind) {
			return nil, fmt.Errorf("implementation %v does not match Adapter info type=%q version=%q Runtime kinds=%v", registration.Key, info.ID, info.Version, info.SupportedRuntimeKinds)
		}
		registry.adapters[registration.Key] = registration.Adapter
		registry.descriptors = append(registry.descriptors, ImplementationDescriptor{Key: registration.Key, Info: cloneTypeInfo(info)})
	}
	slices.SortFunc(registry.descriptors, func(left, right ImplementationDescriptor) int {
		if value := string(left.Key.RuntimeKind) + "\x00" + string(left.Key.WorkloadType) + "\x00" + left.Key.WorkloadVersion; value < string(right.Key.RuntimeKind)+"\x00"+string(right.Key.WorkloadType)+"\x00"+right.Key.WorkloadVersion {
			return -1
		} else if value > string(right.Key.RuntimeKind)+"\x00"+string(right.Key.WorkloadType)+"\x00"+right.Key.WorkloadVersion {
			return 1
		}
		return 0
	})
	return registry, nil
}

func (r *implementationRegistry) Select(key ImplementationKey) (workload.Adapter, error) {
	adapter, found := r.adapters[key]
	if !found {
		return nil, fmt.Errorf("%w: type=%q version=%q Runtime kind=%q", ErrImplementationNotFound, key.WorkloadType, key.WorkloadVersion, key.RuntimeKind)
	}
	return adapter, nil
}

// SelectImplementation adapts the registry to Coordinator's narrow selector
// without introducing a package cycle.
func (r *implementationRegistry) SelectImplementation(workloadType workload.TypeID, version string, runtimeKind agentruntime.Kind) (workload.Adapter, error) {
	return r.Select(ImplementationKey{WorkloadType: workloadType, WorkloadVersion: version, RuntimeKind: runtimeKind})
}

func (r *implementationRegistry) Describe() []ImplementationDescriptor {
	result := make([]ImplementationDescriptor, len(r.descriptors))
	for i, descriptor := range r.descriptors {
		result[i] = ImplementationDescriptor{Key: descriptor.Key, Info: cloneTypeInfo(descriptor.Info)}
	}
	return result
}

func cloneTypeInfo(info workload.TypeInfo) workload.TypeInfo {
	result := info
	result.SupportedRuntimeKinds = slices.Clone(info.SupportedRuntimeKinds)
	result.Configuration.Schema = slices.Clone(info.Configuration.Schema)
	return result
}
