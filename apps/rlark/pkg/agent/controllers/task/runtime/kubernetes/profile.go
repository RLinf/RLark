package kubernetes

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// ReconcileProfileExecutor validates, compares, and applies one immutable
// Kubernetes reconcile contract. Implementations perform no Runtime I/O.
type ReconcileProfileExecutor interface {
	Profile() agentruntime.ObjectKind
	Validate(agentruntime.DesiredObject) error
	Equal(agentruntime.TypedManifest, agentruntime.DesiredObject) (bool, error)
	Apply(client.Object, client.Object, agentruntime.TaskIdentity, agentruntime.DesiredObject) (bool, error)
}

// ProfileRegistry is an immutable lookup of explicitly registered reconcile
// contracts. Unknown profiles fail closed.
type ProfileRegistry struct {
	executors map[agentruntime.ObjectKind]ReconcileProfileExecutor
}

// NewProfileRegistry constructs a registry and rejects duplicate or empty IDs.
func NewProfileRegistry(executors ...ReconcileProfileExecutor) (ProfileRegistry, error) {
	result := ProfileRegistry{executors: make(map[agentruntime.ObjectKind]ReconcileProfileExecutor, len(executors))}
	for _, executor := range executors {
		if executor == nil {
			return ProfileRegistry{}, fault.New(fault.InvalidArgument, "Kubernetes reconcile profile executor is required")
		}
		profile := executor.Profile()
		if profile.Group == "" || profile.Version == "" || profile.Kind == "" {
			return ProfileRegistry{}, fault.New(fault.InvalidArgument, "Kubernetes reconcile profile ID is incomplete")
		}
		if _, found := result.executors[profile]; found {
			return ProfileRegistry{}, fault.New(fault.Conflict, fmt.Sprintf("duplicate Kubernetes reconcile profile %s", profileID(profile)))
		}
		result.executors[profile] = executor
	}
	return result, nil
}

// BuiltinProfileRegistry returns the immutable contracts supported by the
// built-in Kubernetes Runtime implementation.
func BuiltinProfileRegistry() ProfileRegistry {
	registry, err := NewProfileRegistry(
		workloadProfileExecutor{}, serviceProfileExecutor{}, sharedProfileExecutor{}, secretProfileExecutor{}, configProfileExecutor{},
	)
	if err != nil {
		panic(err)
	}
	return registry
}

func (r ProfileRegistry) executor(profile agentruntime.ObjectKind) (ReconcileProfileExecutor, error) {
	executor, found := r.executors[profile]
	if !found {
		return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("unsupported Kubernetes reconcile profile %s", profileID(profile)))
	}
	return executor, nil
}

func (r ProfileRegistry) Validate(desired agentruntime.DesiredObject) error {
	executor, err := r.executor(desired.ReconcileProfile)
	if err != nil {
		return err
	}
	return executor.Validate(desired)
}

func (r ProfileRegistry) Equal(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
	executor, err := r.executor(desired.ReconcileProfile)
	if err != nil {
		return false, err
	}
	return executor.Equal(current, desired)
}

func (r ProfileRegistry) Apply(current, desired client.Object, task agentruntime.TaskIdentity, declaration agentruntime.DesiredObject) (bool, error) {
	executor, err := r.executor(declaration.ReconcileProfile)
	if err != nil {
		return false, err
	}
	return executor.Apply(current, desired, task, declaration)
}

func profileID(profile agentruntime.ObjectKind) string {
	return fmt.Sprintf("%s/%s, Kind=%s", profile.Group, profile.Version, profile.Kind)
}

func semanticJSONEqual(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
	var left, right any
	if err := json.Unmarshal(current.Object, &left); err != nil {
		return false, err
	}
	if err := json.Unmarshal(desired.Manifest.Object, &right); err != nil {
		return false, err
	}
	return reflect.DeepEqual(left, right), nil
}

type workloadProfileExecutor struct{}

func (workloadProfileExecutor) Profile() agentruntime.ObjectKind {
	return kubernetesobjects.WorkloadReconcileProfile
}
func (workloadProfileExecutor) Validate(agentruntime.DesiredObject) error { return nil }
func (workloadProfileExecutor) Equal(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
	return semanticJSONEqual(current, desired)
}
func (workloadProfileExecutor) Apply(current, desired client.Object, task agentruntime.TaskIdentity, declaration agentruntime.DesiredObject) (bool, error) {
	if workloadKind(current) == "" || workloadKind(desired) == "" || workloadKind(current) != workloadKind(desired) {
		return false, fault.New(fault.InvalidArgument, "Kubernetes workload profile requires matching workload objects")
	}
	return applyWorkload(current, desired, task, declaration.Ownership)
}

type serviceProfileExecutor struct{}

func (serviceProfileExecutor) Profile() agentruntime.ObjectKind {
	return kubernetesobjects.ServiceReconcileProfile
}
func (serviceProfileExecutor) Validate(agentruntime.DesiredObject) error { return nil }
func (serviceProfileExecutor) Equal(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
	return kubernetesobjects.ServiceProjectionEqual(current, desired.Manifest)
}
func (serviceProfileExecutor) Apply(current, desired client.Object, task agentruntime.TaskIdentity, declaration agentruntime.DesiredObject) (bool, error) {
	existing, existingOK := current.(*corev1.Service)
	want, desiredOK := desired.(*corev1.Service)
	if !existingOK || !desiredOK {
		return false, fault.New(fault.InvalidArgument, "Kubernetes Service profile requires Service objects")
	}
	return applyService(existing, want, task, declaration.Ownership)
}

type sharedProfileExecutor struct{}

func (sharedProfileExecutor) Profile() agentruntime.ObjectKind {
	return kubernetesobjects.SharedReconcileProfile
}

type secretProfileExecutor struct{}

func (secretProfileExecutor) Profile() agentruntime.ObjectKind {
	return kubernetesobjects.SecretReconcileProfile
}

type configProfileExecutor struct{}

func (configProfileExecutor) Profile() agentruntime.ObjectKind {
	return kubernetesobjects.ConfigReconcileProfile
}
func (configProfileExecutor) Validate(desired agentruntime.DesiredObject) error {
	if desired.Ownership.Scope != agentruntime.IdentityTaskIncarnation || desired.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup {
		return fault.New(fault.InvalidArgument, "Kubernetes ConfigMap profile requires Task-incarnation ownership and cleanup")
	}
	var config corev1.ConfigMap
	if err := json.Unmarshal(desired.Manifest.Object, &config); err != nil {
		return fault.Wrap(fault.InvalidArgument, "decode Kubernetes ConfigMap declaration", err)
	}
	if config.Kind != "ConfigMap" || config.APIVersion != "v1" || len(config.Data) == 0 {
		return fault.New(fault.InvalidArgument, "Kubernetes ConfigMap profile requires non-empty data")
	}
	return nil
}
func (configProfileExecutor) Equal(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
	var existing, want corev1.ConfigMap
	if err := json.Unmarshal(current.Object, &existing); err != nil {
		return false, err
	}
	if err := json.Unmarshal(desired.Manifest.Object, &want); err != nil {
		return false, err
	}
	return reflect.DeepEqual(existing.Data, want.Data) && reflect.DeepEqual(existing.BinaryData, want.BinaryData) && reflect.DeepEqual(projectStringMap(existing.Annotations, want.Annotations), want.Annotations), nil
}
func (configProfileExecutor) Apply(current, desired client.Object, task agentruntime.TaskIdentity, declaration agentruntime.DesiredObject) (bool, error) {
	if err := (configProfileExecutor{}).Validate(declaration); err != nil {
		return false, err
	}
	existing, existingOK := current.(*corev1.ConfigMap)
	want, desiredOK := desired.(*corev1.ConfigMap)
	if !existingOK || !desiredOK {
		return false, fault.New(fault.InvalidArgument, "Kubernetes ConfigMap profile requires ConfigMap objects")
	}
	if !managedByTaskIncarnation(existing, task) {
		return false, fault.New(fault.Conflict, "Kubernetes ConfigMap is not owned by the current Task incarnation")
	}
	changed := !reflect.DeepEqual(existing.Data, want.Data) || !reflect.DeepEqual(existing.BinaryData, want.BinaryData)
	if changed {
		existing.Data = maps.Clone(want.Data)
		existing.BinaryData = maps.Clone(want.BinaryData)
	}
	if !reflect.DeepEqual(existing.Annotations, want.Annotations) {
		existing.Annotations = maps.Clone(want.Annotations)
		changed = true
	}
	return changed, nil
}
func (secretProfileExecutor) Validate(desired agentruntime.DesiredObject) error {
	if desired.Ownership.Scope != agentruntime.IdentityTaskIncarnation || desired.Lifecycle != agentruntime.LifecycleDeleteOnTaskCleanup {
		return fault.New(fault.InvalidArgument, "Kubernetes Secret profile requires Task-incarnation ownership and cleanup")
	}
	var secret corev1.Secret
	if err := json.Unmarshal(desired.Manifest.Object, &secret); err != nil {
		return fault.Wrap(fault.InvalidArgument, "decode Kubernetes Secret declaration", err)
	}
	if secret.Kind != "Secret" || secret.APIVersion != "v1" || secret.Type != corev1.SecretTypeOpaque || len(secret.Data) != 1 {
		return fault.New(fault.InvalidArgument, "Kubernetes Secret profile requires one opaque data entry")
	}
	return nil
}
func (secretProfileExecutor) Equal(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
	var existing, want corev1.Secret
	if err := json.Unmarshal(current.Object, &existing); err != nil {
		return false, err
	}
	if err := json.Unmarshal(desired.Manifest.Object, &want); err != nil {
		return false, err
	}
	return existing.Type == want.Type && reflect.DeepEqual(existing.Data, want.Data) && reflect.DeepEqual(projectStringMap(existing.Annotations, want.Annotations), want.Annotations), nil
}
func (secretProfileExecutor) Apply(current, desired client.Object, task agentruntime.TaskIdentity, declaration agentruntime.DesiredObject) (bool, error) {
	if err := (secretProfileExecutor{}).Validate(declaration); err != nil {
		return false, err
	}
	existing, existingOK := current.(*corev1.Secret)
	want, desiredOK := desired.(*corev1.Secret)
	if !existingOK || !desiredOK {
		return false, fault.New(fault.InvalidArgument, "Kubernetes Secret profile requires Secret objects")
	}
	if !managedByTaskIncarnation(existing, task) {
		return false, fault.New(fault.Conflict, "Kubernetes Secret is not owned by the current Task incarnation")
	}
	changed := !reflect.DeepEqual(existing.Data, want.Data) || existing.Type != want.Type
	if changed {
		existing.Data = want.DeepCopy().Data
		existing.Type = want.Type
	}
	if !reflect.DeepEqual(existing.Annotations, want.Annotations) {
		existing.Annotations = maps.Clone(want.Annotations)
		changed = true
	}
	return changed, nil
}

func projectStringMap(values, owned map[string]string) map[string]string {
	result := make(map[string]string, len(owned))
	for key := range owned {
		result[key] = values[key]
	}
	return result
}
func (sharedProfileExecutor) Validate(desired agentruntime.DesiredObject) error {
	if desired.Ownership.Scope != agentruntime.IdentitySharedResource {
		return fault.New(fault.InvalidArgument, "Kubernetes shared projection profile requires shared-resource ownership")
	}
	return nil
}
func (sharedProfileExecutor) Equal(current agentruntime.TypedManifest, desired agentruntime.DesiredObject) (bool, error) {
	return semanticJSONEqual(current, desired)
}
func (sharedProfileExecutor) Apply(current, desired client.Object, _ agentruntime.TaskIdentity, declaration agentruntime.DesiredObject) (bool, error) {
	if err := (sharedProfileExecutor{}).Validate(declaration); err != nil {
		return false, err
	}
	return applySharedObject(current, desired, declaration.OwnedFields)
}
