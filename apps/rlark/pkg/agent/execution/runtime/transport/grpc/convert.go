package grpc

import (
	"fmt"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	commonv1 "github.com/rlinf/rlark/sdks/execution-go/gen/common/v1"
)

func toWireTask(task agentruntime.TaskIdentity) *commonv1.TaskIdentity {
	return &commonv1.TaskIdentity{
		LogicalId:  toWireLogicalTaskID(task.Logical),
		ObjectUid:  task.ObjectUID,
		Generation: task.Generation,
	}
}

func fromWireTask(task *commonv1.TaskIdentity) (agentruntime.TaskIdentity, error) {
	if task == nil {
		return agentruntime.TaskIdentity{}, fmt.Errorf("task identity is required")
	}
	logical, err := fromWireLogicalTaskID(task.GetLogicalId())
	if err != nil {
		return agentruntime.TaskIdentity{}, err
	}
	return agentruntime.TaskIdentity{Logical: logical, ObjectUID: task.GetObjectUid(), Generation: task.GetGeneration()}, nil
}

func toWireLogicalTaskID(id agentruntime.LogicalTaskID) *commonv1.LogicalTaskId {
	return &commonv1.LogicalTaskId{Namespace: id.Namespace, Name: id.Name}
}

func fromWireLogicalTaskID(id *commonv1.LogicalTaskId) (agentruntime.LogicalTaskID, error) {
	if id == nil {
		return agentruntime.LogicalTaskID{}, fmt.Errorf("logical task ID is required")
	}
	result := agentruntime.LogicalTaskID{Namespace: id.GetNamespace(), Name: id.GetName()}
	if err := result.Validate(); err != nil {
		return agentruntime.LogicalTaskID{}, err
	}
	return result, nil
}

func toWireObjectRef(ref agentruntime.ObjectRef) *commonv1.ObjectReference {
	return &commonv1.ObjectReference{RuntimeId: string(ref.RuntimeID), Id: ref.ID, Namespace: ref.Namespace, Name: ref.Name, Uid: ref.UID, LogicalTaskId: toWireLogicalTaskID(ref.LogicalTask)}
}

func fromWireObjectRef(ref *commonv1.ObjectReference) (agentruntime.ObjectRef, error) {
	if ref == nil {
		return agentruntime.ObjectRef{}, fmt.Errorf("object reference is required")
	}
	logical, err := fromWireLogicalTaskID(ref.GetLogicalTaskId())
	if err != nil {
		return agentruntime.ObjectRef{}, err
	}
	return agentruntime.ObjectRef{RuntimeID: agentruntime.ID(ref.GetRuntimeId()), ID: ref.GetId(), Namespace: ref.GetNamespace(), Name: ref.GetName(), UID: ref.GetUid(), LogicalTask: logical}, nil
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

func toWireObject(object agentruntime.Object) *commonv1.RuntimeObject {
	return &commonv1.RuntimeObject{Reference: toWireObjectRef(object.Ref), Manifest: toWireValue(object.Manifest.Type, object.Manifest.Object)}
}

func fromWireObject(object *commonv1.RuntimeObject) (agentruntime.Object, error) {
	if object == nil {
		return agentruntime.Object{}, fmt.Errorf("runtime object is required")
	}
	ref, err := fromWireObjectRef(object.GetReference())
	if err != nil {
		return agentruntime.Object{}, err
	}
	manifest := object.GetManifest()
	if manifest == nil {
		return agentruntime.Object{}, fmt.Errorf("runtime object manifest is required")
	}
	if err := validateJSONValue(manifest); err != nil {
		return agentruntime.Object{}, fmt.Errorf("runtime object manifest: %w", err)
	}
	return agentruntime.Object{Ref: ref, Manifest: agentruntime.TypedManifest{Type: fromWireType(manifest.GetType()), Object: append([]byte(nil), manifest.GetValue()...)}}, nil
}

func toWireDesired(object agentruntime.DesiredObject) *commonv1.DesiredRuntimeObject {
	result := &commonv1.DesiredRuntimeObject{
		LogicalName:      object.LogicalName,
		Role:             object.Role,
		Type:             toWireType(object.Kind),
		Manifest:         toWireValue(object.Manifest.Type, object.Manifest.Object),
		Ownership:        toWireOwnership(object.Ownership),
		Lifecycle:        toWireLifecycle(object.Lifecycle),
		DependsOn:        append([]string(nil), object.DependsOn...),
		UpdateMode:       toWireUpdateMode(object.UpdateMode),
		ReconcileProfile: toWireType(object.ReconcileProfile),
	}
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
	result := agentruntime.DesiredObject{
		LogicalName: object.GetLogicalName(), Role: object.GetRole(), Kind: fromWireType(object.GetType()),
		Manifest:  agentruntime.TypedManifest{Type: fromWireType(object.GetManifest().GetType()), Object: append([]byte(nil), object.GetManifest().GetValue()...)},
		Ownership: ownership, Lifecycle: lifecycle, DependsOn: append([]string(nil), object.GetDependsOn()...), UpdateMode: updateMode,
		OwnedFields:      append([]string(nil), object.GetOwnedFields().GetPaths()...),
		ReconcileProfile: fromWireType(object.GetReconcileProfile()),
	}
	for _, unit := range object.GetUnits() {
		if unit == nil {
			return agentruntime.DesiredObject{}, fmt.Errorf("desired object Runtime unit is required")
		}
		result.Units = append(result.Units, agentruntime.RuntimeUnit{Name: unit.GetName(), Kind: agentruntime.UnitKind(unit.GetKind()), Role: unit.GetRole(), Locator: fromWireLocator(unit.GetLocator())})
	}
	return result, nil
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

func toWireInstance(instance agentruntime.Instance) *commonv1.RuntimeInstance {
	conditions := make([]*commonv1.InstanceCondition, 0, len(instance.Conditions))
	for _, condition := range instance.Conditions {
		conditions = append(conditions, &commonv1.InstanceCondition{Source: condition.Source, Scope: condition.Scope, Subject: condition.Subject, Reason: condition.Reason, Message: condition.Message})
	}
	return &commonv1.RuntimeInstance{RuntimeId: string(instance.Ref.RuntimeID), Id: instance.Ref.ID, Uid: instance.Ref.UID, Parent: toWireObjectRef(instance.Ref.Parent), Task: toWireTask(instance.Task), Name: instance.Name, Role: instance.Role, Index: instance.Index, Rank: instance.Rank, Phase: string(instance.Phase), Reason: instance.Reason, Message: instance.Message, Conditions: conditions}
}

func fromWireInstance(instance *commonv1.RuntimeInstance) (agentruntime.Instance, error) {
	if instance == nil {
		return agentruntime.Instance{}, fmt.Errorf("runtime instance is required")
	}
	parent, err := fromWireObjectRef(instance.GetParent())
	if err != nil {
		return agentruntime.Instance{}, err
	}
	task, err := fromWireTask(instance.GetTask())
	if err != nil {
		return agentruntime.Instance{}, err
	}
	conditions := make([]agentruntime.InstanceCondition, 0, len(instance.GetConditions()))
	for _, condition := range instance.GetConditions() {
		conditions = append(conditions, agentruntime.InstanceCondition{Source: condition.GetSource(), Scope: condition.GetScope(), Subject: condition.GetSubject(), Reason: condition.GetReason(), Message: condition.GetMessage()})
	}
	return agentruntime.Instance{Ref: agentruntime.InstanceRef{RuntimeID: agentruntime.ID(instance.GetRuntimeId()), ID: instance.GetId(), UID: instance.GetUid(), Parent: parent}, Task: task, Name: instance.GetName(), Role: instance.GetRole(), Index: instance.Index, Rank: instance.Rank, Phase: agentruntime.InstancePhase(instance.GetPhase()), Reason: instance.GetReason(), Message: instance.GetMessage(), Conditions: conditions}, nil
}

func toWireScope(scope agentruntime.IdentityScope) commonv1.IdentityScope {
	return map[agentruntime.IdentityScope]commonv1.IdentityScope{agentruntime.IdentityTaskIncarnation: commonv1.IdentityScope_IDENTITY_SCOPE_TASK_INCARNATION, agentruntime.IdentityLogicalTask: commonv1.IdentityScope_IDENTITY_SCOPE_LOGICAL_TASK, agentruntime.IdentityRuntimeInstance: commonv1.IdentityScope_IDENTITY_SCOPE_RUNTIME_INSTANCE, agentruntime.IdentitySharedResource: commonv1.IdentityScope_IDENTITY_SCOPE_SHARED_RESOURCE, agentruntime.IdentityExternal: commonv1.IdentityScope_IDENTITY_SCOPE_EXTERNAL}[scope]
}

func fromWireScope(scope commonv1.IdentityScope) (agentruntime.IdentityScope, error) {
	result, ok := map[commonv1.IdentityScope]agentruntime.IdentityScope{commonv1.IdentityScope_IDENTITY_SCOPE_TASK_INCARNATION: agentruntime.IdentityTaskIncarnation, commonv1.IdentityScope_IDENTITY_SCOPE_LOGICAL_TASK: agentruntime.IdentityLogicalTask, commonv1.IdentityScope_IDENTITY_SCOPE_RUNTIME_INSTANCE: agentruntime.IdentityRuntimeInstance, commonv1.IdentityScope_IDENTITY_SCOPE_SHARED_RESOURCE: agentruntime.IdentitySharedResource, commonv1.IdentityScope_IDENTITY_SCOPE_EXTERNAL: agentruntime.IdentityExternal}[scope]
	if !ok {
		return "", fmt.Errorf("unknown identity scope %d", scope)
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
