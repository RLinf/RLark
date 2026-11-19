package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ID uniquely identifies a configured Runtime Provider.
type ID string

// Kind identifies the implementation family of a Runtime Provider.
type Kind string

// Capability identifies one optional versioned execution protocol feature.
type Capability string

const (
	CapabilityWorkloadInventory       Capability = "WorkloadInventory"
	CapabilityCandidateInventory      Capability = "RuntimeCandidateInventory"
	CapabilityOwnedFieldProjection    Capability = "OwnedFieldProjection"
	CapabilityClaimedWorkloadRecovery Capability = "ClaimedWorkloadRecovery"
	CapabilityIncarnationAdoption     Capability = "IncarnationAdoption"
	CapabilityReconcileProfiles       Capability = "ReconcileProfiles"
)

// LogicalTaskID identifies a Task across Kubernetes object incarnations.
type LogicalTaskID struct {
	Namespace string
	Name      string
}

// Validate reports whether both components of the logical identity are set.
func (id LogicalTaskID) Validate() error {
	if id.Namespace == "" {
		return errors.New("logical task namespace is required")
	}
	if id.Name == "" {
		return errors.New("logical task name is required")
	}
	return nil
}

// String returns the namespace-qualified Task name.
func (id LogicalTaskID) String() string {
	return id.Namespace + "/" + id.Name
}

// TaskIdentity combines stable Task identity with the current object incarnation.
type TaskIdentity struct {
	Logical    LogicalTaskID
	ObjectUID  string
	Generation int64
}

// IdentityScope defines the authority used to prove Runtime object ownership.
type IdentityScope string

const (
	// IdentityTaskIncarnation binds an object to one management Task UID.
	IdentityTaskIncarnation IdentityScope = "TaskIncarnation"
	// IdentityLogicalTask binds an object to a namespace-qualified logical Task.
	IdentityLogicalTask IdentityScope = "LogicalTask"
	// IdentityRuntimeInstance binds an object to one Runtime instance incarnation.
	IdentityRuntimeInstance IdentityScope = "RuntimeInstance"
	// IdentitySharedResource binds an object to an explicitly shared owner key.
	IdentitySharedResource IdentityScope = "SharedResource"
	// IdentityExternal marks an object as referenced but not owned by RLark.
	IdentityExternal IdentityScope = "External"
)

// ResourceOwnership describes how ownership of a Runtime object is proven.
type ResourceOwnership struct {
	Scope             IdentityScope
	OwnerKey          string
	IncarnationPolicy IncarnationOwnershipPolicy
}

// IncarnationOwnershipPolicy controls whether a Task-incarnation object that
// lacks canonical Task UID metadata may be adopted using stronger Runtime
// ownership evidence. The zero value requires canonical ownership.
type IncarnationOwnershipPolicy string

const (
	IncarnationOwnershipRequireCanonical IncarnationOwnershipPolicy = ""
	IncarnationOwnershipAllowAdoption    IncarnationOwnershipPolicy = "AllowAdoption"
)

// Validate reports whether the ownership declaration is complete and recognized.
func (o ResourceOwnership) Validate() error {
	switch o.Scope {
	case IdentityTaskIncarnation, IdentityLogicalTask, IdentityRuntimeInstance, IdentitySharedResource, IdentityExternal:
	default:
		return fmt.Errorf("unknown identity scope %q", o.Scope)
	}
	if o.OwnerKey == "" {
		return errors.New("owner key is required")
	}
	switch o.IncarnationPolicy {
	case IncarnationOwnershipRequireCanonical:
	case IncarnationOwnershipAllowAdoption:
		if o.Scope != IdentityTaskIncarnation {
			return errors.New("incarnation adoption is valid only for Task-incarnation ownership")
		}
	default:
		return fmt.Errorf("unknown incarnation ownership policy %q", o.IncarnationPolicy)
	}
	return nil
}

// ResourceLifecycle defines who removes an object and at which lifecycle boundary.
type ResourceLifecycle string

const (
	// LifecycleRetain preserves an object during Task cleanup.
	LifecycleRetain ResourceLifecycle = "Retain"
	// LifecycleDeleteOnTaskCleanup removes an owned object during Task cleanup.
	LifecycleDeleteOnTaskCleanup ResourceLifecycle = "DeleteOnTaskCleanup"
	// LifecycleRuntimeManaged delegates object lifecycle to the Runtime.
	LifecycleRuntimeManaged ResourceLifecycle = "RuntimeManaged"
	// LifecycleExternal leaves lifecycle management to an external owner.
	LifecycleExternal ResourceLifecycle = "External"
)

// ObjectKind identifies the schema of a typed Runtime value.
type ObjectKind struct {
	Group   string
	Version string
	Kind    string
}

// TypedManifest contains a Runtime object encoded against an explicit schema.
type TypedManifest struct {
	Type   ObjectKind
	Object json.RawMessage
}

// TypedOptions contains versioned provider- or adapter-specific configuration.
type TypedOptions struct {
	Type  ObjectKind
	Value json.RawMessage
}

// UpdateMode declares how a Provider may move an existing object to desired state.
type UpdateMode string

const (
	// UpdateApplyInPlace permits mutation of the existing object incarnation.
	UpdateApplyInPlace UpdateMode = "ApplyInPlace"
	// UpdateReplace permits replacing an object while preserving its logical identity.
	UpdateReplace UpdateMode = "Replace"
	// UpdateDeleteAndCreate requires confirmed deletion before recreation.
	UpdateDeleteAndCreate UpdateMode = "DeleteAndCreate"
	// UpdateUnsupported rejects changes to an existing object.
	UpdateUnsupported UpdateMode = "Unsupported"
)

// DesiredObject declares one Runtime object in an executable Workload plan.
type DesiredObject struct {
	LogicalName string
	Role        string
	Kind        ObjectKind
	Manifest    TypedManifest
	Ownership   ResourceOwnership
	Lifecycle   ResourceLifecycle
	DependsOn   []string
	UpdateMode  UpdateMode
	// ReconcileProfile selects a typed Provider mechanism. Providers must reject
	// unknown profiles and profile/manifest mismatches rather than infer policy
	// from the manifest kind.
	ReconcileProfile ObjectKind
	// OwnedFields is the explicit projection a Provider may reconcile for a
	// shared object. Empty means the Provider must not infer shared ownership.
	OwnedFields []string
	// Units are Adapter-declared, read-only Pod or PodTemplate semantics located
	// within this object. Providers may use them for version/capability-aware
	// handling and observation, but they grant no authority to mutate outside the
	// final desired manifest.
	Units []RuntimeUnit
}

// UnitKind identifies one declared execution-unit shape shared by Workload
// declarations, Environment mutation targets, and Runtime projections. The
// enum is intentionally extensible beyond the currently supported Pod shapes.
type UnitKind string

const (
	UnitPodTemplate UnitKind = "PodTemplate"
	UnitPod         UnitKind = "Pod"
)

// RuntimeUnit is the object-relative, read-only projection of an Adapter's
// WorkloadUnit declaration after Runtime-specific lowering.
type RuntimeUnit struct {
	Name    string
	Kind    UnitKind
	Role    string
	Locator ObjectLocator
}

// ObjectLocator identifies a closed Runtime schema location shared by Workload
// unit declarations, Environment mutation targets, and Runtime projections.
// Path is a static object field locator, not an arbitrary JSONPath expression.
type ObjectLocator struct {
	Type ObjectKind
	Path string
}

// ObjectCandidate is an Adapter-declared, identity-derived object location.
// Exact candidates are namespaced names; label candidates are bounded to one
// namespace and kind. Providers must still prove ownership after locating them.
type ObjectCandidate struct {
	Kind      ObjectKind
	Namespace string
	Name      string
	Labels    map[string]string
	Ownership ResourceOwnership
	Lifecycle ResourceLifecycle
}

// CandidateKey returns the stable identity of an exact-name inventory candidate.
// Label candidates have no single object identity and return an empty string.
func CandidateKey(candidate ObjectCandidate) string {
	if candidate.Name == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", candidate.Kind.Group, candidate.Kind.Version, candidate.Kind.Kind, candidate.Namespace, candidate.Name)
}

// ManagedObjectQuery asks a Provider to observe only Adapter-declared candidates.
type ManagedObjectQuery struct {
	Task       TaskIdentity
	Candidates []ObjectCandidate
}

// KindString returns the conventional group/version/kind representation.
func (o DesiredObject) KindString() string {
	return fmt.Sprintf("%s/%s, Kind=%s", o.Kind.Group, o.Kind.Version, o.Kind.Kind)
}

// ObjectRef identifies one current Runtime object incarnation.
type ObjectRef struct {
	RuntimeID   ID
	ID          string
	Namespace   string
	Name        string
	UID         string
	LogicalTask LogicalTaskID
}

// Object contains a current Runtime object reference and manifest.
type Object struct {
	Ref      ObjectRef
	Manifest TypedManifest
}

// ObjectSnapshot is a complete, revisioned view of objects managed for a Task.
type ObjectSnapshot struct {
	Revision string
	Objects  []Object
}

// InstanceRef identifies one Runtime execution instance and its parent object.
type InstanceRef struct {
	RuntimeID ID
	ID        string
	UID       string
	Parent    ObjectRef
}

// InstancePhase is the Runtime-native lifecycle phase of an execution instance.
type InstancePhase string

// InstanceCondition reports one current, structured fact about an instance.
type InstanceCondition struct {
	Source  string
	Scope   string
	Subject string
	Reason  string
	Message string
}

// Instance is a normalized observation of a Pod, container, process, or equivalent unit.
type Instance struct {
	Ref        InstanceRef
	Task       TaskIdentity
	Name       string
	Role       string
	Index      *int32
	Rank       *int32
	Phase      InstancePhase
	Reason     string
	Message    string
	Conditions []InstanceCondition
}

// InstanceQuery selects the instances belonging to an observed Workload plan.
type InstanceQuery struct {
	Task      TaskIdentity
	Workloads []ObjectRef
	Roots     []string
	Options   *TypedOptions
}

// InstanceSnapshot is a complete, revisioned result for an InstanceQuery.
type InstanceSnapshot struct {
	Revision  string
	Instances []Instance
}

// Provider list operations are atomic: a successful snapshot contains the
// complete result set. Providers must return an error rather than partial data.

// Info identifies a Runtime Provider registration.
type Info struct {
	ID           ID
	Kind         Kind
	Capabilities []Capability
}

// ExecutionRevision is the canonical, non-secret plan identity persisted on a
// primary Runtime root. It is evidence for targeted observation, not ownership
// authority by itself.
type ExecutionRevision struct {
	// RuntimeID identifies the selected Provider registration.
	RuntimeID ID
	// RuntimeKind identifies the Provider implementation kind.
	RuntimeKind Kind
	// WorkloadType identifies the selected workload implementation.
	WorkloadType string
	// WorkloadVersion identifies the workload contract version.
	WorkloadVersion string
	// CompiledDigest identifies the canonical compiled plan.
	CompiledDigest string
	// ExecutableDigest identifies the canonical environment-bound plan.
	ExecutableDigest string
	// BindingRevision identifies the resolved environment binding.
	BindingRevision string
}

// ApplyRequest asks a Provider to ensure one desired Runtime object.
type ApplyRequest struct {
	IdempotencyKey string
	Task           TaskIdentity
	Desired        DesiredObject
	Current        *ObjectRef
	// ProvenOwners contains current objects from the same complete inventory
	// snapshot that may prove controller ownership for incarnation adoption.
	ProvenOwners []ObjectRef
	Revision     ExecutionRevision
}

// ApplyResult reports the current object after a successful apply.
type ApplyResult struct {
	Ref    ObjectRef
	Object Object
}

// DeleteRequest asks a Provider to remove one proven-owned object incarnation.
type DeleteRequest struct {
	IdempotencyKey string
	Task           TaskIdentity
	Ref            ObjectRef
}

// Provider reconciles Runtime objects and returns complete current-state snapshots.
// Implementations must make external effects idempotent and honor context cancellation.
type Provider interface {
	Info(context.Context) (Info, error)
	Get(context.Context, ObjectRef) (Object, error)
	ListManaged(context.Context, TaskIdentity) (ObjectSnapshot, error)
	Apply(context.Context, ApplyRequest) (ApplyResult, error)
	Delete(context.Context, DeleteRequest) error
	ListInstances(context.Context, InstanceQuery) (InstanceSnapshot, error)
}

// InventoryProvider supports bounded Adapter-directed managed-object discovery.
// The versioned remote Runtime protocol exposes the same optional capability.
type InventoryProvider interface {
	ListCandidates(context.Context, ManagedObjectQuery) (ObjectSnapshot, error)
}

// ClaimedWorkloadProvider recovers a legacy claim's frozen Workload identity
// from Provider-owned root execution evidence.
type ClaimedWorkloadProvider interface {
	ClaimedWorkload(context.Context, TaskIdentity) (workloadType, workloadVersion string, err error)
}
