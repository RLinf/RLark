package grpc

import (
	"encoding/json"
	"fmt"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	commonv1 "github.com/rlinf/rlark/sdks/execution-go/gen/common/v1"
	workloadv1 "github.com/rlinf/rlark/sdks/execution-go/gen/workload/v1"
)

var (
	configurationType = agentruntime.ObjectKind{Group: "execution.rlark.io", Version: "v1", Kind: "WorkloadConfiguration"}
	requirementsType  = agentruntime.ObjectKind{Group: "execution.rlark.io", Version: "v1", Kind: "EnvironmentRequirements"}
	updatePolicyType  = agentruntime.ObjectKind{Group: "execution.rlark.io", Version: "v1", Kind: "UpdatePolicy"}
)

func toWireTask(task coordination.TaskSnapshot) (*workloadv1.TaskSnapshot, error) {
	if task.CarrierSelector.Type != task.Configuration.Carrier.Type || task.CarrierSelector.Version != task.Configuration.Carrier.Version {
		return nil, fmt.Errorf("carrier selector and configuration carrier must match")
	}
	configuration, err := jsonValue(configurationType, task.Configuration)
	if err != nil {
		return nil, err
	}
	requirements, err := jsonValue(requirementsType, task.Environment)
	if err != nil {
		return nil, err
	}
	return &workloadv1.TaskSnapshot{
		Identity: toWireTaskIdentity(task.Identity), RuntimeId: string(task.RuntimeSelector.ID), RuntimeKind: string(task.RuntimeSelector.Kind),
		WorkloadType: task.WorkloadSelector.Type, WorkloadVersion: task.WorkloadSelector.Version, Configuration: configuration,
		EnvironmentRequirements: requirements, Deleting: task.Deleting,
	}, nil
}

func fromWireTask(task *workloadv1.TaskSnapshot) (coordination.TaskSnapshot, error) {
	if task == nil {
		return coordination.TaskSnapshot{}, fmt.Errorf("task snapshot is required")
	}
	identity, err := fromWireTaskIdentity(task.GetIdentity())
	if err != nil {
		return coordination.TaskSnapshot{}, err
	}
	var configuration coordination.WorkloadConfiguration
	if err := decodeJSONValue(task.GetConfiguration(), configurationType, &configuration); err != nil {
		return coordination.TaskSnapshot{}, err
	}
	var requirements executionplan.Requirements
	if err := decodeJSONValue(task.GetEnvironmentRequirements(), requirementsType, &requirements); err != nil {
		return coordination.TaskSnapshot{}, err
	}
	return coordination.TaskSnapshot{
		Identity: identity,
		RuntimeSelector: coordination.RuntimeSelector{
			ID:   agentruntime.ID(task.GetRuntimeId()),
			Kind: agentruntime.Kind(task.GetRuntimeKind()),
		},
		WorkloadSelector: coordination.WorkloadSelector{
			Type:    task.GetWorkloadType(),
			Version: task.GetWorkloadVersion(),
		},
		CarrierSelector: coordination.CarrierSelector{
			Type:    configuration.Carrier.Type,
			Version: configuration.Carrier.Version,
		},
		Configuration: configuration,
		Environment:   requirements,
		Deleting:      task.GetDeleting(),
	}, nil
}

func toWirePlan(plan executionplan.Plan) (*workloadv1.WorkloadPlan, error) {
	requirements, err := jsonValue(requirementsType, plan.Environment)
	if err != nil {
		return nil, fault.Wrap(fault.Internal, "encode Environment requirements", err)
	}
	update, err := jsonValue(updatePolicyType, plan.Update)
	if err != nil {
		return nil, fault.Wrap(fault.Internal, "encode update policy", err)
	}
	result := &workloadv1.WorkloadPlan{AdapterId: plan.AdapterID, AdapterVersion: plan.AdapterVersion, CompiledHash: plan.CompiledHash, EnvironmentRequirements: requirements, Discovery: toWireDiscovery(plan.Discovery), UpdatePolicy: update}
	for _, object := range plan.Objects {
		result.Objects = append(result.Objects, toWireDesired(object))
	}
	for _, target := range plan.Targets {
		result.MutationTargets = append(result.MutationTargets, toWireTarget(target))
	}
	for _, unit := range plan.Units {
		result.WorkloadUnits = append(result.WorkloadUnits, toWireWorkloadUnit(unit))
	}
	for _, process := range plan.Processes {
		result.Processes = append(result.Processes, toWireProcess(process))
	}
	return result, nil
}

func fromWirePlan(plan *workloadv1.WorkloadPlan) (executionplan.Plan, error) {
	if plan == nil {
		return executionplan.Plan{}, fmt.Errorf("workload plan is required")
	}
	var requirements executionplan.Requirements
	if err := decodeJSONValue(plan.GetEnvironmentRequirements(), requirementsType, &requirements); err != nil {
		return executionplan.Plan{}, err
	}
	var update executionplan.UpdatePolicy
	if err := decodeJSONValue(plan.GetUpdatePolicy(), updatePolicyType, &update); err != nil {
		return executionplan.Plan{}, err
	}
	result := executionplan.Plan{AdapterID: plan.GetAdapterId(), AdapterVersion: plan.GetAdapterVersion(), CompiledHash: plan.GetCompiledHash(), Environment: requirements, Discovery: fromWireDiscovery(plan.GetDiscovery()), Update: update}
	for _, object := range plan.GetObjects() {
		converted, err := fromWireDesired(object)
		if err != nil {
			return executionplan.Plan{}, err
		}
		result.Objects = append(result.Objects, converted)
	}
	for _, target := range plan.GetMutationTargets() {
		result.Targets = append(result.Targets, fromWireTarget(target))
	}
	for _, unit := range plan.GetWorkloadUnits() {
		result.Units = append(result.Units, fromWireWorkloadUnit(unit))
	}
	for _, process := range plan.GetProcesses() {
		result.Processes = append(result.Processes, fromWireProcess(process))
	}
	return result, nil
}

func toWireExecutable(plan executionplan.ExecutablePlan) (*workloadv1.ExecutablePlan, error) {
	update, err := jsonValue(updatePolicyType, plan.Update)
	if err != nil {
		return nil, err
	}
	result := &workloadv1.ExecutablePlan{Digests: &workloadv1.PlanDigests{CompiledHash: plan.Digests.CompiledHash, ExecutableHash: plan.Digests.ExecutableHash, BindingRevision: plan.Digests.BindingRevision}, Discovery: toWireDiscovery(plan.Discovery), UpdatePolicy: update}
	for _, object := range plan.Objects {
		result.Objects = append(result.Objects, toWireDesired(object))
	}
	return result, nil
}

func fromWireExecutable(plan *workloadv1.ExecutablePlan) (executionplan.ExecutablePlan, error) {
	if plan == nil {
		return executionplan.ExecutablePlan{}, fmt.Errorf("executable plan is required")
	}
	var update executionplan.UpdatePolicy
	if err := decodeJSONValue(plan.GetUpdatePolicy(), updatePolicyType, &update); err != nil {
		return executionplan.ExecutablePlan{}, err
	}
	result := executionplan.ExecutablePlan{Digests: executionplan.Digests{CompiledHash: plan.GetDigests().GetCompiledHash(), ExecutableHash: plan.GetDigests().GetExecutableHash(), BindingRevision: plan.GetDigests().GetBindingRevision()}, Discovery: fromWireDiscovery(plan.GetDiscovery()), Update: update}
	for _, object := range plan.GetObjects() {
		converted, err := fromWireDesired(object)
		if err != nil {
			return executionplan.ExecutablePlan{}, err
		}
		result.Objects = append(result.Objects, converted)
	}
	return result, nil
}

func toWireDiscovery(plan executionplan.InstanceDiscovery) *workloadv1.InstanceDiscoveryPlan {
	result := &workloadv1.InstanceDiscoveryPlan{Roots: append([]string(nil), plan.Roots...)}
	if plan.Options != nil {
		result.Options = toWireValue(plan.Options.Type, plan.Options.Value)
	}
	return result
}

func fromWireDiscovery(plan *workloadv1.InstanceDiscoveryPlan) executionplan.InstanceDiscovery {
	if plan == nil {
		return executionplan.InstanceDiscovery{}
	}
	result := executionplan.InstanceDiscovery{Roots: append([]string(nil), plan.GetRoots()...)}
	if plan.GetOptions() != nil {
		result.Options = &agentruntime.TypedOptions{Type: fromWireType(plan.GetOptions().GetType()), Value: append([]byte(nil), plan.GetOptions().GetValue()...)}
	}
	return result
}

func toWireLocator(locator agentruntime.ObjectLocator) *commonv1.ObjectLocator {
	return &commonv1.ObjectLocator{Type: toWireType(locator.Type), Path: locator.Path}
}

func fromWireLocator(locator *commonv1.ObjectLocator) agentruntime.ObjectLocator {
	if locator == nil {
		return agentruntime.ObjectLocator{}
	}
	return agentruntime.ObjectLocator{Type: fromWireType(locator.GetType()), Path: locator.GetPath()}
}

func toWireTarget(target executionplan.MutationTarget) *workloadv1.MutationTarget {
	return &workloadv1.MutationTarget{Name: target.Name, ObjectName: target.ObjectName, Kind: string(target.Kind), Role: target.Role, Locator: toWireLocator(target.Locator)}
}

func fromWireTarget(target *workloadv1.MutationTarget) executionplan.MutationTarget {
	if target == nil {
		return executionplan.MutationTarget{}
	}
	return executionplan.MutationTarget{Name: target.GetName(), ObjectName: target.GetObjectName(), Kind: agentruntime.UnitKind(target.GetKind()), Role: target.GetRole(), Locator: fromWireLocator(target.GetLocator())}
}

func toWireWorkloadUnit(unit executionplan.WorkloadUnit) *workloadv1.WorkloadUnit {
	return &workloadv1.WorkloadUnit{Name: unit.Name, ObjectName: unit.ObjectName, Kind: string(unit.Kind), Role: unit.Role, Locator: toWireLocator(unit.Locator)}
}

func fromWireWorkloadUnit(unit *workloadv1.WorkloadUnit) executionplan.WorkloadUnit {
	if unit == nil {
		return executionplan.WorkloadUnit{}
	}
	return executionplan.WorkloadUnit{Name: unit.GetName(), ObjectName: unit.GetObjectName(), Kind: agentruntime.UnitKind(unit.GetKind()), Role: unit.GetRole(), Locator: fromWireLocator(unit.GetLocator())}
}

func toWireProcess(process executionprocess.Declaration) *workloadv1.ProcessDeclaration {
	return &workloadv1.ProcessDeclaration{Name: process.Name, Target: process.Target, Locator: toWireLocator(process.Locator), ContainerName: process.ContainerName, Role: string(process.Role), Executable: process.Executable, Args: append([]string(nil), process.Args...), RestartPolicy: string(process.RestartPolicy), Required: process.Required}
}

func fromWireProcess(process *workloadv1.ProcessDeclaration) executionprocess.Declaration {
	if process == nil {
		return executionprocess.Declaration{}
	}
	return executionprocess.Declaration{Name: process.GetName(), Target: process.GetTarget(), Locator: fromWireLocator(process.GetLocator()), ContainerName: process.GetContainerName(), Role: executionprocess.Role(process.GetRole()), Executable: process.GetExecutable(), Args: append([]string(nil), process.GetArgs()...), RestartPolicy: executionprocess.RestartPolicy(process.GetRestartPolicy()), Required: process.GetRequired()}
}

func jsonValue(kind agentruntime.ObjectKind, value any) (*commonv1.TypedValue, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return toWireValue(kind, data), nil
}

func decodeJSONValue(value *commonv1.TypedValue, want agentruntime.ObjectKind, target any) error {
	if value == nil {
		return fmt.Errorf("typed value %s is required", want.Kind)
	}
	if got := fromWireType(value.GetType()); got != want {
		return fmt.Errorf("unexpected typed value %#v, want %#v", got, want)
	}
	if value.GetContentType() != "application/json" {
		return fmt.Errorf("unexpected content type %q, want application/json", value.GetContentType())
	}
	if err := json.Unmarshal(value.GetValue(), target); err != nil {
		return fmt.Errorf("decode %s: %w", want.Kind, err)
	}
	return nil
}

func toWireTaskIdentity(task agentruntime.TaskIdentity) *commonv1.TaskIdentity {
	return &commonv1.TaskIdentity{LogicalId: &commonv1.LogicalTaskId{Namespace: task.Logical.Namespace, Name: task.Logical.Name}, ObjectUid: task.ObjectUID, Generation: task.Generation}
}

func fromWireTaskIdentity(task *commonv1.TaskIdentity) (agentruntime.TaskIdentity, error) {
	if task == nil || task.GetLogicalId() == nil {
		return agentruntime.TaskIdentity{}, fmt.Errorf("task identity and logical ID are required")
	}
	logical := agentruntime.LogicalTaskID{Namespace: task.GetLogicalId().GetNamespace(), Name: task.GetLogicalId().GetName()}
	if err := logical.Validate(); err != nil {
		return agentruntime.TaskIdentity{}, err
	}
	return agentruntime.TaskIdentity{Logical: logical, ObjectUID: task.GetObjectUid(), Generation: task.GetGeneration()}, nil
}

func toWireType(kind agentruntime.ObjectKind) *commonv1.TypeReference {
	return &commonv1.TypeReference{Group: kind.Group, Version: kind.Version, Kind: kind.Kind}
}
func fromWireType(kind *commonv1.TypeReference) agentruntime.ObjectKind {
	if kind == nil {
		return agentruntime.ObjectKind{}
	}
	return agentruntime.ObjectKind{Group: kind.GetGroup(), Version: kind.GetVersion(), Kind: kind.GetKind()}
}
func toWireValue(kind agentruntime.ObjectKind, value []byte) *commonv1.TypedValue {
	return &commonv1.TypedValue{Type: toWireType(kind), Value: value, ContentType: "application/json"}
}

func toWireDesired(object agentruntime.DesiredObject) *commonv1.DesiredRuntimeObject {
	result := &commonv1.DesiredRuntimeObject{LogicalName: object.LogicalName, Role: object.Role, Type: toWireType(object.Kind), Manifest: toWireValue(object.Manifest.Type, object.Manifest.Object), Ownership: toWireOwnership(object.Ownership), Lifecycle: toWireLifecycle(object.Lifecycle), DependsOn: append([]string(nil), object.DependsOn...), UpdateMode: toWireUpdateMode(object.UpdateMode), ReconcileProfile: toWireType(object.ReconcileProfile)}
	if len(object.OwnedFields) != 0 {
		result.OwnedFields = &commonv1.OwnedFieldProjection{Paths: append([]string(nil), object.OwnedFields...)}
	}
	for _, unit := range object.Units {
		result.Units = append(result.Units, &commonv1.RuntimeUnit{Name: unit.Name, Kind: string(unit.Kind), Role: unit.Role, Locator: toWireLocator(unit.Locator)})
	}
	return result
}

func fromWireDesired(object *commonv1.DesiredRuntimeObject) (agentruntime.DesiredObject, error) {
	if object == nil || object.GetManifest() == nil || object.GetOwnership() == nil {
		return agentruntime.DesiredObject{}, fmt.Errorf("desired object, manifest, and ownership are required")
	}
	if err := validateJSONValue(object.GetManifest()); err != nil {
		return agentruntime.DesiredObject{}, fmt.Errorf("desired object manifest: %w", err)
	}
	scope, err := fromWireScope(object.GetOwnership().GetScope())
	if err != nil {
		return agentruntime.DesiredObject{}, err
	}
	policy, err := fromWireIncarnationPolicy(object.GetOwnership().GetIncarnationPolicy())
	if err != nil {
		return agentruntime.DesiredObject{}, err
	}
	ownership := agentruntime.ResourceOwnership{Scope: scope, OwnerKey: object.GetOwnership().GetOwnerKey(), IncarnationPolicy: policy}
	if err := ownership.Validate(); err != nil {
		return agentruntime.DesiredObject{}, err
	}
	lifecycle, err := fromWireLifecycle(object.GetLifecycle())
	if err != nil {
		return agentruntime.DesiredObject{}, err
	}
	updateMode, err := fromWireUpdateMode(object.GetUpdateMode())
	if err != nil {
		return agentruntime.DesiredObject{}, err
	}
	result := agentruntime.DesiredObject{LogicalName: object.GetLogicalName(), Role: object.GetRole(), Kind: fromWireType(object.GetType()), Manifest: agentruntime.TypedManifest{Type: fromWireType(object.GetManifest().GetType()), Object: append([]byte(nil), object.GetManifest().GetValue()...)}, Ownership: ownership, Lifecycle: lifecycle, DependsOn: append([]string(nil), object.GetDependsOn()...), UpdateMode: updateMode, OwnedFields: append([]string(nil), object.GetOwnedFields().GetPaths()...), ReconcileProfile: fromWireType(object.GetReconcileProfile())}
	for _, unit := range object.GetUnits() {
		if unit == nil {
			return agentruntime.DesiredObject{}, fmt.Errorf("desired object Runtime unit is required")
		}
		result.Units = append(result.Units, agentruntime.RuntimeUnit{Name: unit.GetName(), Kind: agentruntime.UnitKind(unit.GetKind()), Role: unit.GetRole(), Locator: fromWireLocator(unit.GetLocator())})
	}
	return result, nil
}

func toWireCandidate(candidate agentruntime.ObjectCandidate) *commonv1.RuntimeObjectCandidate {
	return &commonv1.RuntimeObjectCandidate{Type: toWireType(candidate.Kind), Namespace: candidate.Namespace, Name: candidate.Name, Labels: candidate.Labels, Ownership: toWireOwnership(candidate.Ownership), Lifecycle: toWireLifecycle(candidate.Lifecycle)}
}

func fromWireCandidate(candidate *commonv1.RuntimeObjectCandidate) (agentruntime.ObjectCandidate, error) {
	if candidate == nil || candidate.GetType() == nil || candidate.GetOwnership() == nil {
		return agentruntime.ObjectCandidate{}, fmt.Errorf("runtime object candidate type and ownership are required")
	}
	scope, err := fromWireScope(candidate.GetOwnership().GetScope())
	if err != nil {
		return agentruntime.ObjectCandidate{}, err
	}
	lifecycle, err := fromWireLifecycle(candidate.GetLifecycle())
	if err != nil {
		return agentruntime.ObjectCandidate{}, err
	}
	policy, err := fromWireIncarnationPolicy(candidate.GetOwnership().GetIncarnationPolicy())
	if err != nil {
		return agentruntime.ObjectCandidate{}, err
	}
	ownership := agentruntime.ResourceOwnership{Scope: scope, OwnerKey: candidate.GetOwnership().GetOwnerKey(), IncarnationPolicy: policy}
	if err := ownership.Validate(); err != nil {
		return agentruntime.ObjectCandidate{}, err
	}
	return agentruntime.ObjectCandidate{Kind: fromWireType(candidate.GetType()), Namespace: candidate.GetNamespace(), Name: candidate.GetName(), Labels: candidate.GetLabels(), Ownership: ownership, Lifecycle: lifecycle}, nil
}

func toWireOwnership(value agentruntime.ResourceOwnership) *commonv1.ResourceOwnership {
	policy := commonv1.IncarnationOwnershipPolicy_INCARNATION_OWNERSHIP_POLICY_UNSPECIFIED
	if value.IncarnationPolicy == agentruntime.IncarnationOwnershipAllowAdoption {
		policy = commonv1.IncarnationOwnershipPolicy_INCARNATION_OWNERSHIP_POLICY_ALLOW_ADOPTION
	}
	return &commonv1.ResourceOwnership{Scope: toWireScope(value.Scope), OwnerKey: value.OwnerKey, IncarnationPolicy: policy}
}

func fromWireIncarnationPolicy(value commonv1.IncarnationOwnershipPolicy) (agentruntime.IncarnationOwnershipPolicy, error) {
	switch value {
	case commonv1.IncarnationOwnershipPolicy_INCARNATION_OWNERSHIP_POLICY_UNSPECIFIED:
		return agentruntime.IncarnationOwnershipRequireCanonical, nil
	case commonv1.IncarnationOwnershipPolicy_INCARNATION_OWNERSHIP_POLICY_ALLOW_ADOPTION:
		return agentruntime.IncarnationOwnershipAllowAdoption, nil
	default:
		return "", fmt.Errorf("unknown incarnation ownership policy %d", value)
	}
}

func toWireCapability(capability agentruntime.Capability) commonv1.ProtocolCapability {
	return map[agentruntime.Capability]commonv1.ProtocolCapability{
		agentruntime.CapabilityWorkloadInventory:       commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_WORKLOAD_INVENTORY,
		agentruntime.CapabilityCandidateInventory:      commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_RUNTIME_CANDIDATE_INVENTORY,
		agentruntime.CapabilityOwnedFieldProjection:    commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_OWNED_FIELD_PROJECTION,
		agentruntime.CapabilityClaimedWorkloadRecovery: commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_CLAIMED_WORKLOAD_RECOVERY,
		agentruntime.CapabilityIncarnationAdoption:     commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_INCARNATION_ADOPTION,
		agentruntime.CapabilityReconcileProfiles:       commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_RECONCILE_PROFILES,
	}[capability]
}

func fromWireCapability(capability commonv1.ProtocolCapability) (agentruntime.Capability, error) {
	result, ok := map[commonv1.ProtocolCapability]agentruntime.Capability{
		commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_WORKLOAD_INVENTORY:          agentruntime.CapabilityWorkloadInventory,
		commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_RUNTIME_CANDIDATE_INVENTORY: agentruntime.CapabilityCandidateInventory,
		commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_OWNED_FIELD_PROJECTION:      agentruntime.CapabilityOwnedFieldProjection,
		commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_CLAIMED_WORKLOAD_RECOVERY:   agentruntime.CapabilityClaimedWorkloadRecovery,
		commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_INCARNATION_ADOPTION:        agentruntime.CapabilityIncarnationAdoption,
		commonv1.ProtocolCapability_PROTOCOL_CAPABILITY_RECONCILE_PROFILES:          agentruntime.CapabilityReconcileProfiles,
	}[capability]
	if !ok {
		return "", fmt.Errorf("unknown protocol capability %d", capability)
	}
	return result, nil
}

func toWireScope(value agentruntime.IdentityScope) commonv1.IdentityScope {
	switch value {
	case agentruntime.IdentityTaskIncarnation:
		return commonv1.IdentityScope_IDENTITY_SCOPE_TASK_INCARNATION
	case agentruntime.IdentityLogicalTask:
		return commonv1.IdentityScope_IDENTITY_SCOPE_LOGICAL_TASK
	case agentruntime.IdentityRuntimeInstance:
		return commonv1.IdentityScope_IDENTITY_SCOPE_RUNTIME_INSTANCE
	case agentruntime.IdentitySharedResource:
		return commonv1.IdentityScope_IDENTITY_SCOPE_SHARED_RESOURCE
	case agentruntime.IdentityExternal:
		return commonv1.IdentityScope_IDENTITY_SCOPE_EXTERNAL
	default:
		return commonv1.IdentityScope_IDENTITY_SCOPE_UNSPECIFIED
	}
}
func fromWireScope(value commonv1.IdentityScope) (agentruntime.IdentityScope, error) {
	result, ok := map[commonv1.IdentityScope]agentruntime.IdentityScope{commonv1.IdentityScope_IDENTITY_SCOPE_TASK_INCARNATION: agentruntime.IdentityTaskIncarnation, commonv1.IdentityScope_IDENTITY_SCOPE_LOGICAL_TASK: agentruntime.IdentityLogicalTask, commonv1.IdentityScope_IDENTITY_SCOPE_RUNTIME_INSTANCE: agentruntime.IdentityRuntimeInstance, commonv1.IdentityScope_IDENTITY_SCOPE_SHARED_RESOURCE: agentruntime.IdentitySharedResource, commonv1.IdentityScope_IDENTITY_SCOPE_EXTERNAL: agentruntime.IdentityExternal}[value]
	if !ok {
		return "", fmt.Errorf("unknown identity scope %d", value)
	}
	return result, nil
}
func toWireLifecycle(value agentruntime.ResourceLifecycle) commonv1.ResourceLifecycle {
	return map[agentruntime.ResourceLifecycle]commonv1.ResourceLifecycle{agentruntime.LifecycleRetain: commonv1.ResourceLifecycle_RESOURCE_LIFECYCLE_RETAIN, agentruntime.LifecycleDeleteOnTaskCleanup: commonv1.ResourceLifecycle_RESOURCE_LIFECYCLE_DELETE_ON_TASK_CLEANUP, agentruntime.LifecycleRuntimeManaged: commonv1.ResourceLifecycle_RESOURCE_LIFECYCLE_RUNTIME_MANAGED, agentruntime.LifecycleExternal: commonv1.ResourceLifecycle_RESOURCE_LIFECYCLE_EXTERNAL}[value]
}
func fromWireLifecycle(value commonv1.ResourceLifecycle) (agentruntime.ResourceLifecycle, error) {
	result, ok := map[commonv1.ResourceLifecycle]agentruntime.ResourceLifecycle{commonv1.ResourceLifecycle_RESOURCE_LIFECYCLE_RETAIN: agentruntime.LifecycleRetain, commonv1.ResourceLifecycle_RESOURCE_LIFECYCLE_DELETE_ON_TASK_CLEANUP: agentruntime.LifecycleDeleteOnTaskCleanup, commonv1.ResourceLifecycle_RESOURCE_LIFECYCLE_RUNTIME_MANAGED: agentruntime.LifecycleRuntimeManaged, commonv1.ResourceLifecycle_RESOURCE_LIFECYCLE_EXTERNAL: agentruntime.LifecycleExternal}[value]
	if !ok {
		return "", fmt.Errorf("unknown resource lifecycle %d", value)
	}
	return result, nil
}
func toWireUpdateMode(value agentruntime.UpdateMode) commonv1.UpdateMode {
	return map[agentruntime.UpdateMode]commonv1.UpdateMode{agentruntime.UpdateApplyInPlace: commonv1.UpdateMode_UPDATE_MODE_APPLY_IN_PLACE, agentruntime.UpdateReplace: commonv1.UpdateMode_UPDATE_MODE_REPLACE, agentruntime.UpdateDeleteAndCreate: commonv1.UpdateMode_UPDATE_MODE_DELETE_AND_CREATE, agentruntime.UpdateUnsupported: commonv1.UpdateMode_UPDATE_MODE_UNSUPPORTED}[value]
}
func fromWireUpdateMode(value commonv1.UpdateMode) (agentruntime.UpdateMode, error) {
	result, ok := map[commonv1.UpdateMode]agentruntime.UpdateMode{commonv1.UpdateMode_UPDATE_MODE_APPLY_IN_PLACE: agentruntime.UpdateApplyInPlace, commonv1.UpdateMode_UPDATE_MODE_REPLACE: agentruntime.UpdateReplace, commonv1.UpdateMode_UPDATE_MODE_DELETE_AND_CREATE: agentruntime.UpdateDeleteAndCreate, commonv1.UpdateMode_UPDATE_MODE_UNSUPPORTED: agentruntime.UpdateUnsupported}[value]
	if !ok {
		return "", fmt.Errorf("unknown update mode %d", value)
	}
	return result, nil
}

func validateJSONValue(value *commonv1.TypedValue) error {
	if value.GetType() == nil {
		return fmt.Errorf("type is required")
	}
	if value.GetContentType() != "application/json" {
		return fmt.Errorf("unsupported content type %q", value.GetContentType())
	}
	return nil
}

func toWireObjectSnapshot(snapshot agentruntime.ObjectSnapshot) *commonv1.ObjectSnapshot {
	result := &commonv1.ObjectSnapshot{Revision: snapshot.Revision}
	for _, object := range snapshot.Objects {
		result.Objects = append(result.Objects, &commonv1.RuntimeObject{Reference: toWireObjectRef(object.Ref), Manifest: toWireValue(object.Manifest.Type, object.Manifest.Object)})
	}
	return result
}
func fromWireObjectSnapshot(snapshot *commonv1.ObjectSnapshot) (agentruntime.ObjectSnapshot, error) {
	if snapshot == nil {
		return agentruntime.ObjectSnapshot{}, fmt.Errorf("object snapshot is required")
	}
	result := agentruntime.ObjectSnapshot{Revision: snapshot.GetRevision()}
	for _, object := range snapshot.GetObjects() {
		if object == nil || object.GetManifest() == nil {
			return agentruntime.ObjectSnapshot{}, fmt.Errorf("runtime object and manifest are required")
		}
		if err := validateJSONValue(object.GetManifest()); err != nil {
			return agentruntime.ObjectSnapshot{}, fmt.Errorf("runtime object manifest: %w", err)
		}
		ref, err := fromWireObjectRef(object.GetReference())
		if err != nil {
			return agentruntime.ObjectSnapshot{}, err
		}
		result.Objects = append(result.Objects, agentruntime.Object{Ref: ref, Manifest: agentruntime.TypedManifest{Type: fromWireType(object.GetManifest().GetType()), Object: append([]byte(nil), object.GetManifest().GetValue()...)}})
	}
	return result, nil
}
func toWireObjectRef(ref agentruntime.ObjectRef) *commonv1.ObjectReference {
	return &commonv1.ObjectReference{RuntimeId: string(ref.RuntimeID), Id: ref.ID, Namespace: ref.Namespace, Name: ref.Name, Uid: ref.UID, LogicalTaskId: &commonv1.LogicalTaskId{Namespace: ref.LogicalTask.Namespace, Name: ref.LogicalTask.Name}}
}
func fromWireObjectRef(ref *commonv1.ObjectReference) (agentruntime.ObjectRef, error) {
	if ref == nil || ref.GetLogicalTaskId() == nil {
		return agentruntime.ObjectRef{}, fmt.Errorf("object reference and logical task ID are required")
	}
	logical := agentruntime.LogicalTaskID{Namespace: ref.GetLogicalTaskId().GetNamespace(), Name: ref.GetLogicalTaskId().GetName()}
	if err := logical.Validate(); err != nil {
		return agentruntime.ObjectRef{}, err
	}
	return agentruntime.ObjectRef{RuntimeID: agentruntime.ID(ref.GetRuntimeId()), ID: ref.GetId(), Namespace: ref.GetNamespace(), Name: ref.GetName(), UID: ref.GetUid(), LogicalTask: logical}, nil
}

func toWireInstanceSnapshot(snapshot agentruntime.InstanceSnapshot) *commonv1.InstanceSnapshot {
	result := &commonv1.InstanceSnapshot{Revision: snapshot.Revision}
	for _, instance := range snapshot.Instances {
		result.Instances = append(result.Instances, &commonv1.RuntimeInstance{RuntimeId: string(instance.Ref.RuntimeID), Id: instance.Ref.ID, Uid: instance.Ref.UID, Parent: toWireObjectRef(instance.Ref.Parent), Task: toWireTaskIdentity(instance.Task), Name: instance.Name, Role: instance.Role, Index: instance.Index, Rank: instance.Rank, Phase: string(instance.Phase), Reason: instance.Reason, Message: instance.Message})
	}
	return result
}
func fromWireInstanceSnapshot(snapshot *commonv1.InstanceSnapshot) (agentruntime.InstanceSnapshot, error) {
	if snapshot == nil {
		return agentruntime.InstanceSnapshot{}, fmt.Errorf("instance snapshot is required")
	}
	result := agentruntime.InstanceSnapshot{Revision: snapshot.GetRevision()}
	for _, instance := range snapshot.GetInstances() {
		if instance == nil {
			return agentruntime.InstanceSnapshot{}, fmt.Errorf("runtime instance is required")
		}
		parent, err := fromWireObjectRef(instance.GetParent())
		if err != nil {
			return agentruntime.InstanceSnapshot{}, err
		}
		task, err := fromWireTaskIdentity(instance.GetTask())
		if err != nil {
			return agentruntime.InstanceSnapshot{}, err
		}
		result.Instances = append(result.Instances, agentruntime.Instance{Ref: agentruntime.InstanceRef{RuntimeID: agentruntime.ID(instance.GetRuntimeId()), ID: instance.GetId(), UID: instance.GetUid(), Parent: parent}, Task: task, Name: instance.GetName(), Role: instance.GetRole(), Index: instance.Index, Rank: instance.Rank, Phase: agentruntime.InstancePhase(instance.GetPhase()), Reason: instance.GetReason(), Message: instance.GetMessage()})
	}
	return result, nil
}
