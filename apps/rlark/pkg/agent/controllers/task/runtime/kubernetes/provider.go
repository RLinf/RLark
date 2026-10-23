package kubernetes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	k8sptr "k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/observability"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

const (
	// DefaultID is the default registration ID of the local Kubernetes Runtime.
	DefaultID agentruntime.ID = "kubernetes"
	// Kind identifies Kubernetes Provider implementations.
	Kind agentruntime.Kind = "Kubernetes"
)

// Provider reconciles RLark Runtime objects through a Kubernetes API client.
type Provider struct {
	id        agentruntime.ID
	client    client.Client
	scheme    *runtime.Scheme
	namespace func(agentruntime.TaskIdentity) string
	observer  observability.Observer
	profiles  ProfileRegistry
}

// Option customizes a Provider during construction.
type Option func(*Provider)

// WithID overrides the Provider registration ID.
func WithID(id agentruntime.ID) Option { return func(provider *Provider) { provider.id = id } }

// WithObserver installs a best-effort Provider operation observer.
func WithObserver(observer observability.Observer) Option {
	return func(provider *Provider) { provider.observer = observability.Safe(observer, nil, nil, nil) }
}

// New constructs a Kubernetes Runtime Provider.
func New(kubeClient client.Client, scheme *runtime.Scheme, namespace func(agentruntime.TaskIdentity) string, options ...Option) *Provider {
	provider := &Provider{id: DefaultID, client: kubeClient, scheme: scheme, namespace: namespace, profiles: BuiltinProfileRegistry()}
	for _, option := range options {
		option(provider)
	}
	return provider
}

var _ agentruntime.Provider = (*Provider)(nil)
var _ agentruntime.InventoryProvider = (*Provider)(nil)
var _ agentruntime.ClaimedWorkloadProvider = (*Provider)(nil)

// ValidateDesired validates one Kubernetes declaration without Runtime I/O.
func (p *Provider) ValidateDesired(desired agentruntime.DesiredObject) error {
	if p == nil {
		return fault.New(fault.FailedPrecondition, "Kubernetes Provider is required")
	}
	if _, err := p.decodeDesired(desired); err != nil {
		return err
	}
	return p.profiles.Validate(desired)
}

// ClaimedWorkload recovers a legacy claim's implementation identity from the
// exactly one owned primary workload root. Ambiguous or incomplete evidence is
// rejected rather than guessed from names or the current Task Spec.
func (p *Provider) ClaimedWorkload(ctx context.Context, task agentruntime.TaskIdentity) (string, string, error) {
	if err := validateTask(task); err != nil {
		return "", "", err
	}
	namespace, err := p.taskNamespace(task)
	if err != nil {
		return "", "", err
	}
	var found *agentruntime.ExecutionRevision
	for _, object := range []client.Object{&appsv1.Deployment{}, &appsv1.DaemonSet{}, &appsv1.StatefulSet{}} {
		if err := p.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: task.Logical.Name}, object); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return "", "", mapKubernetesError("get claimed Kubernetes workload", err)
		}
		if !managedByTask(object, task) {
			continue
		}
		revision, err := ReadExecutionRevision(object)
		if err != nil {
			return "", "", fault.Wrap(fault.FailedPrecondition, "claimed Kubernetes workload has no complete execution identity", err)
		}
		if revision.RuntimeID != p.id || revision.RuntimeKind != Kind {
			return "", "", fault.New(fault.Conflict, "claimed Kubernetes workload belongs to another Runtime")
		}
		if found != nil {
			return "", "", fault.New(fault.Conflict, "multiple claimed Kubernetes workload roots were found")
		}
		found = &revision
	}
	if found == nil {
		return "", "", fault.New(fault.NotFound, "claimed Kubernetes workload root was not found")
	}
	return found.WorkloadType, found.WorkloadVersion, nil
}

// Info validates dependencies and returns the Provider identity.
func (p *Provider) Info(context.Context) (agentruntime.Info, error) {
	if p.client == nil || p.scheme == nil {
		return agentruntime.Info{}, fault.New(fault.FailedPrecondition, "Kubernetes client and scheme are required")
	}
	return agentruntime.Info{ID: p.id, Kind: Kind, Capabilities: []agentruntime.Capability{agentruntime.CapabilityCandidateInventory, agentruntime.CapabilityOwnedFieldProjection, agentruntime.CapabilityClaimedWorkloadRecovery, agentruntime.CapabilityIncarnationAdoption, agentruntime.CapabilityReconcileProfiles}}, nil
}

// Get retrieves one Kubernetes object using a UID-fenced Runtime reference.
func (p *Provider) Get(ctx context.Context, ref agentruntime.ObjectRef) (agentruntime.Object, error) {
	if err := p.validateRef(ref); err != nil {
		return agentruntime.Object{}, err
	}
	object, err := p.newObject(ref.ID)
	if err != nil {
		return agentruntime.Object{}, err
	}
	if err := p.client.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, object); err != nil {
		if apierrors.IsNotFound(err) {
			return agentruntime.Object{}, fault.Wrap(fault.NotFound, fmt.Sprintf("Kubernetes object %s/%s was not found", ref.Namespace, ref.Name), err)
		}
		return agentruntime.Object{}, fault.Wrap(fault.Internal, "get Kubernetes object", err)
	}
	if ref.UID != "" && string(object.GetUID()) != ref.UID {
		return agentruntime.Object{}, fault.New(fault.Conflict, "Kubernetes object UID does not match reference")
	}
	return p.convert(object, ref.LogicalTask)
}

// ListManaged returns a targeted snapshot of objects provably managed for task.
func (p *Provider) ListManaged(ctx context.Context, task agentruntime.TaskIdentity) (agentruntime.ObjectSnapshot, error) {
	// 1. Validate Task identity and resolve its configured Kubernetes namespace.
	if err := validateTask(task); err != nil {
		return agentruntime.ObjectSnapshot{}, err
	}
	namespace, err := p.taskNamespace(task)
	if err != nil {
		return agentruntime.ObjectSnapshot{}, err
	}
	// 2. Probe deterministic same-name primary workload candidates. Include only
	// objects with proven current ownership or the narrowly defined legacy shape;
	// a name match alone never authorizes auxiliary-resource adoption.
	objects := make([]client.Object, 0)
	for _, object := range []client.Object{&appsv1.Deployment{}, &appsv1.DaemonSet{}, &appsv1.StatefulSet{}} {
		if err := p.client.Get(ctx, types.NamespacedName{Namespace: namespace, Name: task.Logical.Name}, object); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return agentruntime.ObjectSnapshot{}, fault.Wrap(fault.Internal, "get managed Kubernetes workload", err)
		}
		if managedByTask(object, task) || legacyWorkloadForTask(object, task) {
			objects = append(objects, object)
		}
	}
	// 3. Convert and sort the complete result deterministically so callers can
	// diff it safely and derive a stable snapshot revision.
	result := make([]agentruntime.Object, 0, len(objects))
	for _, object := range objects {
		converted, err := p.convert(object, task.Logical)
		if err != nil {
			return agentruntime.ObjectSnapshot{}, err
		}
		result = append(result, converted)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Ref.ID+"/"+result[i].Ref.Name < result[j].Ref.ID+"/"+result[j].Ref.Name
	})
	return agentruntime.ObjectSnapshot{Revision: snapshotRevision(objects), Objects: result}, nil
}

// ListCandidates returns a complete snapshot for the bounded candidates
// declared by the selected Workload Adapter. Candidate location is not
// ownership proof; every observed object is filtered against the declaration.
func (p *Provider) ListCandidates(ctx context.Context, query agentruntime.ManagedObjectQuery) (agentruntime.ObjectSnapshot, error) {
	if err := validateTask(query.Task); err != nil {
		return agentruntime.ObjectSnapshot{}, err
	}
	namespace, err := p.taskNamespace(query.Task)
	if err != nil {
		return agentruntime.ObjectSnapshot{}, err
	}
	type locatedCandidate struct {
		object    client.Object
		candidate agentruntime.ObjectCandidate
	}
	located := make([]locatedCandidate, 0)
	objects := make([]client.Object, 0)
	seen := map[string]struct{}{}
	for _, candidate := range query.Candidates {
		if candidate.Namespace != namespace || candidate.Kind.Kind == "" || candidate.Kind.Version == "" {
			return agentruntime.ObjectSnapshot{}, fault.New(fault.InvalidArgument, "managed object candidate is incomplete or outside the Task namespace")
		}
		if (candidate.Name == "") == (len(candidate.Labels) == 0) {
			return agentruntime.ObjectSnapshot{}, fault.New(fault.InvalidArgument, "managed object candidate must use exactly one exact name or label query")
		}
		if err := candidate.Ownership.Validate(); err != nil {
			return agentruntime.ObjectSnapshot{}, fault.Wrap(fault.InvalidArgument, "invalid managed object candidate ownership", err)
		}
		if candidate.Name != "" {
			object, err := p.newObject(fmt.Sprintf("%s/%s, Kind=%s", candidate.Kind.Group, candidate.Kind.Version, candidate.Kind.Kind))
			if err != nil {
				return agentruntime.ObjectSnapshot{}, err
			}
			if err := p.client.Get(ctx, types.NamespacedName{Namespace: candidate.Namespace, Name: candidate.Name}, object); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return agentruntime.ObjectSnapshot{}, mapKubernetesError("get managed Kubernetes candidate", err)
			}
			located = append(located, locatedCandidate{object: object, candidate: candidate})
			continue
		}
		list, err := p.newObjectList(candidate.Kind)
		if err != nil {
			return agentruntime.ObjectSnapshot{}, err
		}
		if err := p.client.List(ctx, list, client.InNamespace(candidate.Namespace), client.MatchingLabels(candidate.Labels)); err != nil {
			return agentruntime.ObjectSnapshot{}, mapKubernetesError("list managed Kubernetes candidates", err)
		}
		listed, err := listObjects(list)
		if err != nil {
			return agentruntime.ObjectSnapshot{}, err
		}
		for _, object := range listed {
			located = append(located, locatedCandidate{object: object, candidate: candidate})
		}
	}
	provedUIDs := map[types.UID]struct{}{}
	provedWorkloadUIDs := map[types.UID]struct{}{}
	for _, item := range located {
		if candidateDirectlyOwnedByTask(item.object, query.Task, item.candidate.Ownership) {
			objects = appendUniqueObject(objects, seen, item.object)
			provedUIDs[item.object.GetUID()] = struct{}{}
			if workloadKind(item.object) != "" {
				provedWorkloadUIDs[item.object.GetUID()] = struct{}{}
			}
		}
	}
	for _, item := range located {
		if _, ok := provedUIDs[item.object.GetUID()]; ok {
			continue
		}
		if item.candidate.Ownership.IncarnationPolicy == agentruntime.IncarnationOwnershipAllowAdoption && !canonicalIdentityConflicts(item.object, query.Task) && controlledByProvedObject(item.object, provedWorkloadUIDs) {
			objects = appendUniqueObject(objects, seen, item.object)
			provedUIDs[item.object.GetUID()] = struct{}{}
		}
	}
	for _, item := range located {
		if _, ok := provedUIDs[item.object.GetUID()]; ok {
			continue
		}
		legacyExactAuxiliary := item.candidate.Name != "" &&
			item.candidate.Ownership.Scope == agentruntime.IdentityTaskIncarnation &&
			item.candidate.Ownership.IncarnationPolicy == agentruntime.IncarnationOwnershipAllowAdoption &&
			workloadKind(item.object) == "" && canonicalTaskUID(item.object) == "" &&
			!canonicalIdentityConflicts(item.object, query.Task) && metav1.GetControllerOf(item.object) == nil &&
			len(provedWorkloadUIDs) == 1
		if legacyExactAuxiliary {
			objects = appendUniqueObject(objects, seen, item.object)
			provedUIDs[item.object.GetUID()] = struct{}{}
		}
	}
	result := make([]agentruntime.Object, 0, len(objects))
	for _, object := range objects {
		converted, err := p.convert(object, query.Task.Logical)
		if err != nil {
			return agentruntime.ObjectSnapshot{}, err
		}
		result = append(result, converted)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Ref.ID+"/"+result[i].Ref.Name < result[j].Ref.ID+"/"+result[j].Ref.Name
	})
	return agentruntime.ObjectSnapshot{Revision: snapshotRevision(objects), Objects: result}, nil
}

// ListInstances returns Pods and Pod-scoped events for the requested workload roots.
func (p *Provider) ListInstances(ctx context.Context, query agentruntime.InstanceQuery) (agentruntime.InstanceSnapshot, error) {
	// 1. Validate Task identity, then process only the workload roots supplied by
	// Coordinator rather than scanning the Runtime namespace.
	if err := validateTask(query.Task); err != nil {
		return agentruntime.InstanceSnapshot{}, err
	}
	instances := make([]agentruntime.Instance, 0)
	objectsForRevision := make([]client.Object, 0)
	for _, workload := range query.Workloads {
		// 2. Re-read and UID-fence each root. A missing or replaced root contributes
		// no instances, preventing stale observations from crossing incarnations.
		if err := p.validateRef(workload); err != nil {
			return agentruntime.InstanceSnapshot{}, err
		}
		object, err := p.newObject(workload.ID)
		if err != nil {
			return agentruntime.InstanceSnapshot{}, err
		}
		if err := p.client.Get(ctx, types.NamespacedName{Namespace: workload.Namespace, Name: workload.Name}, object); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return agentruntime.InstanceSnapshot{}, fault.Wrap(fault.Internal, "get instance parent workload", err)
		}
		if workload.UID != "" && string(object.GetUID()) != workload.UID {
			continue
		}
		// 3. List selector-matching Pods, retain only controller-owned instances,
		// and fetch Pod UID-scoped Events for failure projection.
		pods, err := p.listPods(ctx, object)
		if err != nil {
			return agentruntime.InstanceSnapshot{}, err
		}
		parent, err := p.ref(object, query.Task.Logical)
		if err != nil {
			return agentruntime.InstanceSnapshot{}, err
		}
		events, err := p.podEvents(ctx, object.GetNamespace(), pods)
		if err != nil {
			return agentruntime.InstanceSnapshot{}, err
		}
		for i := range pods {
			pod := &pods[i]
			objectsForRevision = append(objectsForRevision, pod)
			for j := range events[pod.Name] {
				objectsForRevision = append(objectsForRevision, &events[pod.Name][j])
			}
			instances = append(instances, podInstance(p.id, query.Task, parent, pod, events[pod.Name]))
		}
	}
	// 4. Return deterministic instances and a revision derived from all observed
	// Pods and Events. Partial successful snapshots are never returned.
	sort.Slice(instances, func(i, j int) bool { return instances[i].Ref.ID < instances[j].Ref.ID })
	return agentruntime.InstanceSnapshot{Revision: snapshotRevision(objectsForRevision), Instances: instances}, nil
}

// Apply creates or updates one desired Kubernetes object idempotently.
func (p *Provider) Apply(ctx context.Context, request agentruntime.ApplyRequest) (agentruntime.ApplyResult, error) {
	ctx, scope := p.startObservation(ctx, "kubernetes.apply", request.Task)
	var finalErr error
	defer func() { scope.End(providerObservationResult(finalErr)) }()
	// 1. Decode and validate the desired object before external reads. Namespace
	// placement and embedded Task identity must agree with the Apply request.
	if err := validateTask(request.Task); err != nil {
		finalErr = err
		return agentruntime.ApplyResult{}, err
	}
	desired, err := p.decodeDesired(request.Desired)
	if err != nil {
		finalErr = err
		return agentruntime.ApplyResult{}, err
	}
	namespace, err := p.taskNamespace(request.Task)
	if err != nil {
		finalErr = err
		return agentruntime.ApplyResult{}, err
	}
	if desired.GetNamespace() == "" {
		desired.SetNamespace(namespace)
	}
	if desired.GetNamespace() != namespace {
		finalErr = fault.New(fault.PermissionDenied, "desired Kubernetes object is outside the Task namespace")
		return agentruntime.ApplyResult{}, finalErr
	}
	if desired.GetName() == "" {
		finalErr = fault.New(fault.InvalidArgument, "desired Kubernetes object name is required")
		return agentruntime.ApplyResult{}, finalErr
	}
	if err := validateDesiredIdentity(desired, request.Task); err != nil {
		finalErr = err
		return agentruntime.ApplyResult{}, err
	}
	if err := p.profiles.Validate(request.Desired); err != nil {
		finalErr = err
		return agentruntime.ApplyResult{}, err
	}
	if request.Desired.Role == "workload" {
		if err := WriteExecutionRevision(desired, request.Revision); err != nil {
			finalErr = fault.Wrap(fault.InvalidArgument, "invalid execution revision", err)
			return agentruntime.ApplyResult{}, finalErr
		}
	}
	// 2. Read the exact current object. Create only when it is absent and the
	// caller did not supply a still-existing current UID expectation.
	current, err := p.newObject(request.Desired.KindString())
	if err != nil {
		return agentruntime.ApplyResult{}, err
	}
	err = p.client.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if err != nil && !apierrors.IsNotFound(err) {
		finalErr = mapKubernetesError("get Kubernetes object for Apply", err)
		return agentruntime.ApplyResult{}, finalErr
	}
	if apierrors.IsNotFound(err) {
		if request.Current != nil && request.Current.UID != "" {
			return agentruntime.ApplyResult{}, fault.New(fault.Conflict, "current Kubernetes object no longer exists")
		}
		if err := p.client.Create(ctx, desired); err != nil {
			if !apierrors.IsAlreadyExists(err) || request.Desired.Ownership.Scope != agentruntime.IdentitySharedResource {
				finalErr = mapKubernetesError("create Kubernetes object", err)
				return agentruntime.ApplyResult{}, finalErr
			}
			if err := p.client.Get(ctx, client.ObjectKeyFromObject(desired), current); err != nil {
				return agentruntime.ApplyResult{}, mapKubernetesError("get existing shared Kubernetes object", err)
			}
			if request.Desired.ReconcileProfile != kubernetesobjects.SharedReconcileProfile {
				return agentruntime.ApplyResult{}, fault.New(fault.InvalidArgument, "shared Kubernetes object requires the shared-field reconcile profile")
			}
			if err := validateSharedObjectIdentity(current, desired, request.Desired.OwnedFields); err != nil {
				return agentruntime.ApplyResult{}, err
			}
			changed, err := applySharedObject(current, desired, request.Desired.OwnedFields)
			if err != nil {
				return agentruntime.ApplyResult{}, err
			}
			if changed {
				if err := p.client.Update(ctx, current); err != nil {
					return agentruntime.ApplyResult{}, mapKubernetesError("update existing shared Kubernetes object", err)
				}
			}
			return p.applyResult(current, request.Task.Logical)
		}
		scope.Event(observability.Event{Name: "kubernetes.create", Attributes: map[string]string{"kind": request.Desired.Kind.Kind, "result": "succeeded"}})
		return p.applyResult(desired, request.Task.Logical)
	}
	if request.Desired.Ownership.Scope == agentruntime.IdentitySharedResource {
		if err := validateSharedObjectIdentity(current, desired, request.Desired.OwnedFields); err != nil {
			return agentruntime.ApplyResult{}, err
		}
	}
	// 3. Fence updates by the observed object UID so a delete/recreate race cannot
	// mutate a different Kubernetes object incarnation.
	if request.Current != nil && request.Current.UID != "" && string(current.GetUID()) != request.Current.UID {
		return agentruntime.ApplyResult{}, fault.New(fault.Conflict, "current Kubernetes object UID does not match")
	}
	// 5. For exclusive resources, apply type-specific ownership, immutability,
	// legacy-adoption, and generation rules. Write only when that merge changed
	// the observed object; otherwise return a stable no-op result.
	if request.Desired.Ownership.Scope == agentruntime.IdentityTaskIncarnation && request.Desired.Ownership.IncarnationPolicy == agentruntime.IncarnationOwnershipAllowAdoption && !managedByTaskIncarnation(current, request.Task) && workloadKind(current) == "" {
		if err := p.validateProvenControllerOwner(ctx, current, request.Task, request.ProvenOwners); err != nil {
			finalErr = err
			return agentruntime.ApplyResult{}, err
		}
	}
	changed, err := p.profiles.Apply(current, desired, request.Task, request.Desired)
	if err != nil {
		finalErr = err
		return agentruntime.ApplyResult{}, err
	}
	if changed {
		if err := p.client.Update(ctx, current); err != nil {
			finalErr = mapKubernetesError("update Kubernetes object", err)
			return agentruntime.ApplyResult{}, finalErr
		}
		scope.Event(observability.Event{Name: "kubernetes.update", Attributes: map[string]string{"kind": request.Desired.Kind.Kind, "result": "succeeded"}})
	}
	return p.applyResult(current, request.Task.Logical)
}

func (p *Provider) validateProvenControllerOwner(ctx context.Context, object client.Object, task agentruntime.TaskIdentity, refs []agentruntime.ObjectRef) error {
	owner := metav1.GetControllerOf(object)
	if owner == nil || owner.UID == "" {
		return fault.New(fault.Conflict, "Kubernetes object has no proven controller owner for Task-incarnation adoption")
	}
	for _, ref := range refs {
		if ref.UID != string(owner.UID) || ref.Namespace != object.GetNamespace() || ref.Name != owner.Name {
			continue
		}
		if err := p.validateRef(ref); err != nil {
			return err
		}
		root, err := p.newObject(ref.ID)
		if err != nil {
			return err
		}
		if err := p.client.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, root); err != nil {
			return mapKubernetesError("get controller owner for incarnation adoption", err)
		}
		if root.GetUID() != owner.UID || !managedByTaskIncarnation(root, task) && !legacyWorkloadForTask(root, task) {
			return fault.New(fault.Conflict, "Kubernetes controller owner does not prove current Task ownership")
		}
		return nil
	}
	return fault.New(fault.Conflict, "Kubernetes controller owner is absent from the proven inventory snapshot")
}

func validateSharedObjectIdentity(current, desired client.Object, ownedFields []string) error {
	existing, ok := current.(*corev1.ConfigMap)
	want, desiredOK := desired.(*corev1.ConfigMap)
	if !ok || !desiredOK || existing.Name != want.Name || existing.Namespace != want.Namespace {
		return fault.New(fault.Conflict, "shared Kubernetes object identity does not match desired")
	}
	annotation, err := validateConfigMapProjection(ownedFields)
	if err != nil {
		return err
	}
	if value := existing.Annotations[annotation]; value != "" && value != want.Annotations[annotation] {
		return fault.New(fault.Conflict, "existing shared ConfigMap has conflicting owned metadata")
	}
	return nil
}

func applySharedObject(current, desired client.Object, ownedFields []string) (bool, error) {
	existing, ok := current.(*corev1.ConfigMap)
	want, desiredOK := desired.(*corev1.ConfigMap)
	if !ok || !desiredOK {
		return false, fault.New(fault.InvalidArgument, "shared Kubernetes object types do not match")
	}
	annotation, err := validateConfigMapProjection(ownedFields)
	if err != nil {
		return false, err
	}
	changed := !reflect.DeepEqual(existing.Data, want.Data) || !reflect.DeepEqual(existing.BinaryData, want.BinaryData)
	if changed {
		existing.Data = maps.Clone(want.Data)
		existing.BinaryData = maps.Clone(want.BinaryData)
	}
	if mergeAnnotation(existing, want, annotation) {
		changed = true
	}
	return changed, nil
}

func validateConfigMapProjection(fields []string) (string, error) {
	if len(fields) != 3 || fields[0] != "data" || fields[1] != "binaryData" {
		return "", fault.New(fault.InvalidArgument, "shared ConfigMap requires explicit data, binaryData, and annotation ownership")
	}
	const prefix = "metadata.annotations["
	if !strings.HasPrefix(fields[2], prefix) || !strings.HasSuffix(fields[2], "]") || len(fields[2]) <= len(prefix)+1 {
		return "", fault.New(fault.InvalidArgument, "shared ConfigMap annotation projection is invalid")
	}
	return strings.TrimSuffix(strings.TrimPrefix(fields[2], prefix), "]"), nil
}

func mergeAnnotation(current, desired client.Object, key string) bool {
	currentAnnotations := current.GetAnnotations()
	if currentAnnotations == nil {
		currentAnnotations = map[string]string{}
	}
	value := desired.GetAnnotations()[key]
	if currentAnnotations[key] == value {
		return false
	}
	currentAnnotations[key] = value
	current.SetAnnotations(currentAnnotations)
	return true
}

// Delete removes one Kubernetes object after identity and ownership validation.
func (p *Provider) Delete(ctx context.Context, request agentruntime.DeleteRequest) error {
	ctx, scope := p.startObservation(ctx, "kubernetes.delete", request.Task)
	var finalErr error
	defer func() { scope.End(providerObservationResult(finalErr)) }()
	if err := validateTask(request.Task); err != nil {
		finalErr = err
		return err
	}
	if err := p.validateRef(request.Ref); err != nil {
		finalErr = err
		return err
	}
	object, err := p.newObject(request.Ref.ID)
	if err != nil {
		finalErr = err
		return err
	}
	if err := p.client.Get(ctx, types.NamespacedName{Namespace: request.Ref.Namespace, Name: request.Ref.Name}, object); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		finalErr = mapKubernetesError("get Kubernetes object for Delete", err)
		return finalErr
	}
	if request.Ref.UID != "" && string(object.GetUID()) != request.Ref.UID {
		finalErr = fault.New(fault.Conflict, "Kubernetes object UID does not match Delete reference")
		return finalErr
	}
	options := []client.DeleteOption{}
	if workloadKind(object) != "" {
		options = append(options, client.PropagationPolicy(metav1.DeletePropagationForeground))
	}
	if err := p.client.Delete(ctx, object, options...); err != nil && !apierrors.IsNotFound(err) {
		finalErr = mapKubernetesError("delete Kubernetes object", err)
		return finalErr
	}
	scope.Event(observability.Event{Name: "kubernetes.delete.accepted", Attributes: map[string]string{"kind": workloadKind(object)}})
	return nil
}

func (p *Provider) startObservation(ctx context.Context, name string, task agentruntime.TaskIdentity) (context.Context, observability.Scope) {
	observer := p.observer
	if observer == nil {
		observer = observability.Noop()
	}
	return observer.Start(ctx, observability.Reconcile{Name: name, TaskKey: task.Logical.String(), TaskUID: task.ObjectUID, Generation: task.Generation, RuntimeID: string(p.id)})
}

func providerObservationResult(err error) observability.Result {
	if err == nil {
		return observability.Result{Outcome: "Succeeded"}
	}
	code := fault.Internal
	var typed *fault.Error
	if errors.As(err, &typed) {
		code = typed.Code
	}
	return observability.Result{Outcome: "Failed", FaultCode: string(code)}
}

func (p *Provider) taskNamespace(task agentruntime.TaskIdentity) (string, error) {
	if p.namespace == nil {
		return "", fault.New(fault.FailedPrecondition, "Kubernetes task namespace resolver is required")
	}
	namespace := p.namespace(task)
	if namespace == "" {
		return "", fault.New(fault.FailedPrecondition, "Kubernetes task namespace resolved empty")
	}
	return namespace, nil
}

func (p *Provider) validateRef(ref agentruntime.ObjectRef) error {
	if ref.RuntimeID != "" && ref.RuntimeID != p.id {
		return fault.New(fault.InvalidArgument, "object reference belongs to another Runtime")
	}
	if ref.ID == "" || ref.Namespace == "" || ref.Name == "" {
		return fault.New(fault.InvalidArgument, "object kind, namespace, and name are required")
	}
	return nil
}

func validateTask(task agentruntime.TaskIdentity) error {
	if err := task.Logical.Validate(); err != nil {
		return fault.Wrap(fault.InvalidArgument, "invalid Task identity", err)
	}
	return nil
}

func validateDesiredIdentity(object client.Object, task agentruntime.TaskIdentity) error {
	annotations := object.GetAnnotations()
	if uid := annotations[kubernetesobjects.ManagementTaskUIDAnnotation]; uid != "" && uid != task.ObjectUID {
		return fault.New(fault.Conflict, "desired Kubernetes object Task UID does not match Apply request")
	}
	if name := annotations[kubernetesobjects.ManagementTaskNameAnnotation]; name != "" && name != task.Logical.Name {
		return fault.New(fault.Conflict, "desired Kubernetes object Task name does not match Apply request")
	}
	if namespace := annotations[kubernetesobjects.ManagementTaskNamespaceAnnotation]; namespace != "" && namespace != task.Logical.Namespace {
		return fault.New(fault.Conflict, "desired Kubernetes object Task namespace does not match Apply request")
	}
	if generation := annotations[kubernetesobjects.ManagementTaskGenerationAnnotation]; generation != "" && generation != fmt.Sprintf("%d", task.Generation) {
		return fault.New(fault.Conflict, "desired Kubernetes object Task generation does not match Apply request")
	}
	return nil
}

func (p *Provider) decodeDesired(desired agentruntime.DesiredObject) (client.Object, error) {
	if desired.Kind.Kind == "" || desired.Kind.Version == "" {
		return nil, fault.New(fault.InvalidArgument, "desired Kubernetes object kind is required")
	}
	if desired.Manifest.Type != desired.Kind {
		return nil, fault.New(fault.InvalidArgument, "desired Kubernetes manifest type does not match object kind")
	}
	object, err := p.newObject(desired.KindString())
	if err != nil {
		return nil, err
	}
	if !json.Valid(desired.Manifest.Object) {
		return nil, fault.New(fault.InvalidArgument, "desired Kubernetes manifest is not valid JSON")
	}
	if err := json.Unmarshal(desired.Manifest.Object, object); err != nil {
		return nil, fault.Wrap(fault.InvalidArgument, "decode desired Kubernetes object", err)
	}
	gvk, err := p.gvk(object)
	if err != nil {
		return nil, err
	}
	if gvk.Group != desired.Kind.Group || gvk.Version != desired.Kind.Version || gvk.Kind != desired.Kind.Kind {
		return nil, fault.New(fault.InvalidArgument, "decoded Kubernetes object kind does not match desired kind")
	}
	return object, nil
}

func (p *Provider) applyResult(object client.Object, logical agentruntime.LogicalTaskID) (agentruntime.ApplyResult, error) {
	converted, err := p.convert(object, logical)
	if err != nil {
		return agentruntime.ApplyResult{}, err
	}
	return agentruntime.ApplyResult{Ref: converted.Ref, Object: converted}, nil
}

func applyWorkload(current, desired client.Object, task agentruntime.TaskIdentity, declared agentruntime.ResourceOwnership) (bool, error) {
	// 1. Reject authoritative ownership conflicts before inspecting update
	// compatibility. Unknown or foreign workload objects must remain untouched.
	compatTask := compatibilityTask(task)
	ownership := kubernetesobjects.OwnershipForTask(current, compatTask)
	if ownership == kubernetesobjects.OwnershipConflict {
		return false, fault.New(fault.Conflict, "Kubernetes workload is owned by another Task")
	}
	if ownership != kubernetesobjects.OwnershipCurrent && declared.IncarnationPolicy != agentruntime.IncarnationOwnershipAllowAdoption {
		return false, fault.New(fault.Conflict, "Kubernetes workload lacks canonical current Task ownership")
	}
	// 2. Kubernetes workload selectors are immutable. Permit only the narrowly
	// defined legacy identity-label migration; reject all semantic changes.
	selectorChanged := kubernetesobjects.WorkloadSelectorChanged(current, desired)
	if selectorChanged && !kubernetesobjects.WorkloadSelectorIdentityOnlyMigration(current, desired) {
		return false, fault.New(fault.Conflict, "Kubernetes workload has an immutable selector that differs from desired")
	}
	annotations := current.GetAnnotations()
	missingBaseline := annotations[kubernetesobjects.ManagementTaskGenerationAnnotation] == "" && annotations[kubernetesobjects.ManagementTaskAdoptedGenerationAnnotation] == ""
	// 3. First adopt a compatible legacy workload by adding identity metadata
	// only. Preserve its Pod template and defer desired Spec changes to a later
	// generation so adoption cannot silently roll out user workloads.
	if ownership == kubernetesobjects.OwnershipLegacy || (selectorChanged && missingBaseline) {
		kubernetesobjects.AdoptWorkloadAnnotations(current, compatTask)
		mergeExecutionRevisionAnnotations(current, desired)
		return true, nil
	}
	generation := fmt.Sprintf("%d", task.Generation)
	canonicalGeneration := annotations != nil && annotations[kubernetesobjects.ManagementTaskGenerationAnnotation] == generation
	adoptedGeneration := annotations != nil && annotations[kubernetesobjects.ManagementTaskAdoptedGenerationAnnotation] == generation
	if adoptedGeneration && !canonicalGeneration {
		revisionChanged := mergeExecutionRevisionAnnotations(current, desired)
		return revisionChanged, nil
	}
	// Canonical execution revision annotations include Runtime and Workload
	// identity plus derived digests, but are optimization evidence rather than
	// ownership authority. This canonical-owned path has proved Task UID ownership
	// and selector compatibility above; the Provider's UID-fenced current reference
	// also identifies this object instance. A full Apply may therefore repair the
	// entire revision even at the same Task generation. Legacy adoption returned
	// earlier after its narrower name/namespace/kind/selector proof.
	revisionChanged := mergeExecutionRevisionAnnotations(current, desired)
	// 4. Preserve the existing selector contract in the desired Pod template
	// before comparing and copying controller-owned mutable fields. Generation
	// metadata is an ordering fence, not proof that Runtime state still matches
	// desired state: periodic reconciliation must repair out-of-band drift at the
	// same Task generation.
	if err := kubernetesobjects.PrepareWorkloadTemplateUpdate(current, desired); err != nil {
		return false, fault.Wrap(fault.Conflict, "prepare Kubernetes workload update", err)
	}
	// Conservatively normalize the common built-in Pod defaults materialized by
	// the API server when the declarative manifest leaves them unspecified. This
	// avoids the common permanent update loop without pretending to reproduce the
	// complete, version-dependent Kubernetes defaulting implementation.
	preserveObservedPodDefaults(workloadTemplatePointer(current), workloadTemplatePointer(desired))
	annotationsBeforeMerge := maps.Clone(current.GetAnnotations())
	specChanged := false
	// TODO(execution): Replace the conservative common-default normalization and
	// whole-template comparison with an explicit Agent-owned PodTemplate
	// projection (or SSA field ownership). The current normalization intentionally
	// does not mirror every version-dependent nested container, probe, volume, or
	// ephemeral-container default. Whole-template ownership also cannot distinguish
	// fields injected by mutating admission webhooks or another controller, so such
	// fields may be overwritten and repeatedly re-injected.
	switch existing := current.(type) {
	case *appsv1.Deployment:
		want := desired.(*appsv1.Deployment)
		kubernetesobjects.MergeWorkloadAnnotations(existing, compatTask)
		specChanged = !reflect.DeepEqual(existing.Spec.Replicas, want.Spec.Replicas) || !reflect.DeepEqual(existing.Spec.Template, want.Spec.Template)
		existing.Spec.Replicas, existing.Spec.Template = want.Spec.Replicas, want.Spec.Template
	case *appsv1.DaemonSet:
		want := desired.(*appsv1.DaemonSet)
		kubernetesobjects.MergeWorkloadAnnotations(existing, compatTask)
		specChanged = !reflect.DeepEqual(existing.Spec.Template, want.Spec.Template)
		existing.Spec.Template = want.Spec.Template
	case *appsv1.StatefulSet:
		want := desired.(*appsv1.StatefulSet)
		kubernetesobjects.MergeWorkloadAnnotations(existing, compatTask)
		specChanged = !reflect.DeepEqual(existing.Spec.Replicas, want.Spec.Replicas) || !reflect.DeepEqual(existing.Spec.Template, want.Spec.Template)
		existing.Spec.Replicas, existing.Spec.Template = want.Spec.Replicas, want.Spec.Template
	}
	// 5. The successful update establishes the canonical generation marker and
	// removes the one-generation legacy-adoption marker.
	annotations = current.GetAnnotations()
	delete(annotations, kubernetesobjects.ManagementTaskAdoptedGenerationAnnotation)
	current.SetAnnotations(annotations)
	annotationsChanged := !reflect.DeepEqual(annotationsBeforeMerge, annotations)
	return revisionChanged || specChanged || annotationsChanged, nil
}

func applyService(existing, desired *corev1.Service, task agentruntime.TaskIdentity, ownership agentruntime.ResourceOwnership) (bool, error) {
	canonical := managedByTaskIncarnation(existing, task)
	if !canonical {
		if ownership.IncarnationPolicy != agentruntime.IncarnationOwnershipAllowAdoption || canonicalIdentityConflicts(existing, task) {
			return false, fault.New(fault.Conflict, "Kubernetes Service is not owned by the current Task incarnation")
		}
		kubernetesobjects.MergeWorkloadAnnotations(existing, compatibilityTask(task))
	}
	if desired.Spec.ClusterIP != "" && existing.Spec.ClusterIP != "" && desired.Spec.ClusterIP != existing.Spec.ClusterIP {
		return false, fault.New(fault.Conflict, "Kubernetes Service has an immutable clusterIP that differs from desired")
	}
	changed := !canonical
	existingManifest, err := typedManifest(existing)
	if err != nil {
		return false, fault.Wrap(fault.Internal, "encode existing Kubernetes Service", err)
	}
	desiredManifest, err := typedManifest(desired)
	if err != nil {
		return false, fault.Wrap(fault.InvalidArgument, "encode desired Kubernetes Service", err)
	}
	projectionEqual, err := kubernetesobjects.ServiceProjectionEqual(existingManifest, desiredManifest)
	if err != nil {
		return false, fault.Wrap(fault.InvalidArgument, "compare Kubernetes Service projection", err)
	}
	if projectionEqual && !changed {
		return false, nil
	}
	if !reflect.DeepEqual(existing.Spec.Selector, desired.Spec.Selector) {
		existing.Spec.Selector = maps.Clone(desired.Spec.Selector)
		changed = true
	}
	if !reflect.DeepEqual(servicePortsProjection(existing.Spec.Ports), servicePortsProjection(desired.Spec.Ports)) {
		existing.Spec.Ports = preserveAllocatedNodePorts(existing.Spec.Ports, desired.Spec.Ports)
		changed = true
	}
	if existing.Spec.PublishNotReadyAddresses != desired.Spec.PublishNotReadyAddresses {
		existing.Spec.PublishNotReadyAddresses = desired.Spec.PublishNotReadyAddresses
		changed = true
	}
	if existing.Spec.Type != desired.Spec.Type {
		existing.Spec.Type = desired.Spec.Type
		changed = true
	}
	if mergeOwnedMetadata(existing, desired) {
		changed = true
	}
	return changed, nil
}

func typedManifest(object client.Object) (agentruntime.TypedManifest, error) {
	data, err := json.Marshal(object)
	return agentruntime.TypedManifest{Object: data}, err
}

type servicePortProjection struct {
	Name        string
	Protocol    corev1.Protocol
	Port        int32
	TargetPort  intstr.IntOrString
	AppProtocol *string
}

func servicePortsProjection(ports []corev1.ServicePort) []servicePortProjection {
	result := make([]servicePortProjection, len(ports))
	for i := range ports {
		result[i] = servicePortProjection{Name: ports[i].Name, Protocol: ports[i].Protocol, Port: ports[i].Port, TargetPort: ports[i].TargetPort, AppProtocol: ports[i].AppProtocol}
	}
	return result
}

func preserveAllocatedNodePorts(current, desired []corev1.ServicePort) []corev1.ServicePort {
	result := append([]corev1.ServicePort(nil), desired...)
	allocated := make(map[string]int32, len(current))
	for i := range current {
		allocated[current[i].Name] = current[i].NodePort
	}
	for i := range result {
		if result[i].NodePort == 0 {
			result[i].NodePort = allocated[result[i].Name]
		}
	}
	return result
}

func mergeOwnedMetadata(current, desired client.Object) bool {
	labelsChanged := false
	labels := maps.Clone(current.GetLabels())
	if labels == nil {
		labels = map[string]string{}
	}
	for key, value := range desired.GetLabels() {
		if labels[key] != value {
			labels[key] = value
			labelsChanged = true
		}
	}
	annotationsChanged := false
	annotations := maps.Clone(current.GetAnnotations())
	if annotations == nil {
		annotations = map[string]string{}
	}
	for key, value := range desired.GetAnnotations() {
		if annotations[key] != value {
			annotations[key] = value
			annotationsChanged = true
		}
	}
	if labelsChanged {
		current.SetLabels(labels)
	}
	if annotationsChanged {
		current.SetAnnotations(annotations)
	}
	return labelsChanged || annotationsChanged
}

func preserveObservedPodDefaults(current, desired *corev1.PodTemplateSpec) {
	if current == nil || desired == nil {
		return
	}
	preserveKnownStringDefault((*string)(&desired.Spec.RestartPolicy), string(current.Spec.RestartPolicy), string(corev1.RestartPolicyAlways))
	preserveKnownStringDefault((*string)(&desired.Spec.DNSPolicy), string(current.Spec.DNSPolicy), string(corev1.DNSClusterFirst))
	preserveKnownStringDefault(&desired.Spec.SchedulerName, current.Spec.SchedulerName, corev1.DefaultSchedulerName)
	if desired.Spec.TerminationGracePeriodSeconds == nil && current.Spec.TerminationGracePeriodSeconds != nil && *current.Spec.TerminationGracePeriodSeconds == corev1.DefaultTerminationGracePeriodSeconds {
		desired.Spec.TerminationGracePeriodSeconds = k8sptr.To(*current.Spec.TerminationGracePeriodSeconds)
	}
	if desired.Spec.SecurityContext == nil && current.Spec.SecurityContext != nil && reflect.DeepEqual(*current.Spec.SecurityContext, corev1.PodSecurityContext{}) {
		desired.Spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	preserveContainerDefaults(current.Spec.InitContainers, desired.Spec.InitContainers)
	preserveContainerDefaults(current.Spec.Containers, desired.Spec.Containers)
}

func workloadTemplatePointer(object client.Object) *corev1.PodTemplateSpec {
	switch value := object.(type) {
	case *appsv1.Deployment:
		return &value.Spec.Template
	case *appsv1.DaemonSet:
		return &value.Spec.Template
	case *appsv1.StatefulSet:
		return &value.Spec.Template
	default:
		return nil
	}
}

func preserveContainerDefaults(current, desired []corev1.Container) {
	byName := make(map[string]*corev1.Container, len(desired))
	for i := range desired {
		byName[desired[i].Name] = &desired[i]
	}
	for i := range current {
		want := byName[current[i].Name]
		if want == nil {
			continue
		}
		preserveKnownStringDefault((*string)(&want.ImagePullPolicy), string(current[i].ImagePullPolicy), string(defaultImagePullPolicy(want.Image)))
		preserveKnownStringDefault(&want.TerminationMessagePath, current[i].TerminationMessagePath, corev1.TerminationMessagePathDefault)
		preserveKnownStringDefault((*string)(&want.TerminationMessagePolicy), string(current[i].TerminationMessagePolicy), string(corev1.TerminationMessageReadFile))
	}
}

func preserveKnownStringDefault(desired *string, current, knownDefault string) {
	if *desired == "" && current == knownDefault {
		*desired = current
	}
}

func defaultImagePullPolicy(image string) corev1.PullPolicy {
	lastSlash := strings.LastIndex(image, "/")
	lastColon := strings.LastIndex(image, ":")
	if !strings.Contains(image, "@") && (lastColon <= lastSlash || image[lastColon+1:] == "latest") {
		return corev1.PullAlways
	}
	return corev1.PullIfNotPresent
}

func mergeExecutionRevisionAnnotations(current, desired client.Object) bool {
	existing := current.GetAnnotations()
	if existing == nil {
		existing = make(map[string]string)
	}
	want := desired.GetAnnotations()
	changed := false
	for _, key := range []string{
		RuntimeIDAnnotation, RuntimeKindAnnotation, WorkloadTypeAnnotation, WorkloadVersionAnnotation,
		CompiledDigestAnnotation, ExecutableDigestAnnotation, BindingRevisionAnnotation,
	} {
		value := want[key]
		if value == "" {
			continue
		}
		if existing[key] != value {
			existing[key] = value
			changed = true
		}
	}
	current.SetAnnotations(existing)
	return changed
}

func compatibilityTask(task agentruntime.TaskIdentity) *rlarkv1alpha1.Task {
	result := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:       task.Logical.Name,
			Namespace:  task.Logical.Namespace,
			UID:        types.UID(task.ObjectUID),
			Generation: task.Generation,
		},
	}
	return result
}

func mapKubernetesError(operation string, err error) error {
	switch {
	case apierrors.IsNotFound(err):
		return fault.Wrap(fault.NotFound, operation, err)
	case apierrors.IsAlreadyExists(err):
		return fault.Wrap(fault.AlreadyExists, operation, err)
	case apierrors.IsConflict(err):
		return fault.Wrap(fault.Conflict, operation, err)
	case apierrors.IsForbidden(err):
		return fault.Wrap(fault.PermissionDenied, operation, err)
	case apierrors.IsInvalid(err):
		return fault.Wrap(fault.InvalidArgument, operation, err)
	default:
		return fault.Wrap(fault.Internal, operation, err)
	}
}

func (p *Provider) newObject(id string) (client.Object, error) {
	gvk, err := parseGVK(id)
	if err != nil {
		return nil, err
	}
	object, err := p.scheme.New(gvk)
	if err != nil {
		result := &unstructured.Unstructured{}
		result.SetGroupVersionKind(gvk)
		return result, nil
	}
	result, ok := object.(client.Object)
	if !ok {
		return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("Kubernetes kind %q is not an object", id))
	}
	return result, nil
}

func (p *Provider) newObjectList(kind agentruntime.ObjectKind) (client.ObjectList, error) {
	gvk := schema.GroupVersionKind{Group: kind.Group, Version: kind.Version, Kind: kind.Kind + "List"}
	object, err := p.scheme.New(gvk)
	if err != nil {
		result := &unstructured.UnstructuredList{}
		result.SetGroupVersionKind(gvk)
		return result, nil
	}
	result, ok := object.(client.ObjectList)
	if !ok {
		return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("Kubernetes kind %q is not an object list", gvk.String()))
	}
	return result, nil
}

func listObjects(list client.ObjectList) ([]client.Object, error) {
	items, err := apimeta.ExtractList(list)
	if err != nil {
		return nil, fault.Wrap(fault.Internal, "extract Kubernetes candidate list", err)
	}
	result := make([]client.Object, 0, len(items))
	for _, item := range items {
		object, ok := item.(client.Object)
		if !ok {
			return nil, fault.New(fault.Internal, fmt.Sprintf("Kubernetes candidate list item %T is not an object", item))
		}
		result = append(result, object)
	}
	return result, nil
}

func appendUniqueObject(objects []client.Object, seen map[string]struct{}, object client.Object) []client.Object {
	key := fmt.Sprintf("%T/%s/%s/%s", object, object.GetNamespace(), object.GetName(), object.GetUID())
	if _, found := seen[key]; found {
		return objects
	}
	seen[key] = struct{}{}
	return append(objects, object)
}

func parseGVK(id string) (schema.GroupVersionKind, error) {
	parts := strings.Split(id, ", Kind=")
	if len(parts) != 2 || parts[1] == "" {
		return schema.GroupVersionKind{}, fault.New(fault.InvalidArgument, fmt.Sprintf("invalid Kubernetes object kind %q", id))
	}
	groupVersion, err := schema.ParseGroupVersion(parts[0])
	if err != nil {
		return schema.GroupVersionKind{}, fault.Wrap(fault.InvalidArgument, fmt.Sprintf("invalid Kubernetes object kind %q", id), err)
	}
	return groupVersion.WithKind(parts[1]), nil
}

func (p *Provider) ref(object client.Object, logical agentruntime.LogicalTaskID) (agentruntime.ObjectRef, error) {
	gvk, err := p.gvk(object)
	if err != nil {
		return agentruntime.ObjectRef{}, err
	}
	return agentruntime.ObjectRef{RuntimeID: p.id, ID: gvk.String(), Namespace: object.GetNamespace(), Name: object.GetName(), UID: string(object.GetUID()), LogicalTask: logical}, nil
}

func (p *Provider) convert(object client.Object, logical agentruntime.LogicalTaskID) (agentruntime.Object, error) {
	gvk, err := p.gvk(object)
	if err != nil {
		return agentruntime.Object{}, err
	}
	data, err := json.Marshal(object)
	if err != nil {
		return agentruntime.Object{}, fault.Wrap(fault.Internal, "encode Kubernetes object", err)
	}
	ref, err := p.ref(object, logical)
	if err != nil {
		return agentruntime.Object{}, err
	}
	kind := agentruntime.ObjectKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind}
	return agentruntime.Object{Ref: ref, Manifest: agentruntime.TypedManifest{Type: kind, Object: data}}, nil
}

func (p *Provider) gvk(object runtime.Object) (schema.GroupVersionKind, error) {
	if gvk := object.GetObjectKind().GroupVersionKind(); !gvk.Empty() && gvk.Kind != "" && !strings.HasSuffix(gvk.Kind, "List") {
		return gvk, nil
	}
	gvks, _, err := p.scheme.ObjectKinds(object)
	if err != nil || len(gvks) == 0 {
		return schema.GroupVersionKind{}, fault.Wrap(fault.Internal, "resolve Kubernetes object kind", err)
	}
	for _, gvk := range gvks {
		if gvk.Kind != "" && !strings.HasSuffix(gvk.Kind, "List") {
			return gvk, nil
		}
	}
	return schema.GroupVersionKind{}, fault.New(fault.Internal, "Kubernetes object kind is unavailable")
}

func managedByTask(object client.Object, task agentruntime.TaskIdentity) bool {
	annotations := object.GetAnnotations()
	if uid := annotations[kubernetesobjects.ManagementTaskUIDAnnotation]; uid != "" {
		return task.ObjectUID != "" && uid == task.ObjectUID
	}
	if annotations[kubernetesobjects.ManagementTaskNameAnnotation] == task.Logical.Name && annotations[kubernetesobjects.ManagementTaskNamespaceAnnotation] == task.Logical.Namespace {
		return true
	}
	return false
}

func candidateDirectlyOwnedByTask(object client.Object, task agentruntime.TaskIdentity, ownership agentruntime.ResourceOwnership) bool {
	switch ownership.Scope {
	case agentruntime.IdentityTaskIncarnation:
		if ownership.IncarnationPolicy == agentruntime.IncarnationOwnershipAllowAdoption && workloadKind(object) != "" && legacyWorkloadForTask(object, task) {
			return true
		}
		return managedByTaskIncarnation(object, task)
	case agentruntime.IdentityLogicalTask:
		return managedByLogicalTask(object, task)
	default:
		return false
	}
}

func controlledByProvedObject(object client.Object, provedUIDs map[types.UID]struct{}) bool {
	owner := metav1.GetControllerOf(object)
	if owner == nil || owner.UID == "" || canonicalTaskUID(object) != "" {
		return false
	}
	_, ok := provedUIDs[owner.UID]
	return ok
}

func canonicalIdentityConflicts(object client.Object, task agentruntime.TaskIdentity) bool {
	annotations := object.GetAnnotations()
	if uid := annotations[kubernetesobjects.ManagementTaskUIDAnnotation]; uid != "" && uid != task.ObjectUID {
		return true
	}
	if name := annotations[kubernetesobjects.ManagementTaskNameAnnotation]; name != "" && name != task.Logical.Name {
		return true
	}
	return annotations[kubernetesobjects.ManagementTaskNamespaceAnnotation] != "" && annotations[kubernetesobjects.ManagementTaskNamespaceAnnotation] != task.Logical.Namespace
}

func canonicalTaskUID(object client.Object) string {
	return object.GetAnnotations()[kubernetesobjects.ManagementTaskUIDAnnotation]
}

func managedByTaskIncarnation(object client.Object, task agentruntime.TaskIdentity) bool {
	annotations := object.GetAnnotations()
	return task.ObjectUID != "" && annotations[kubernetesobjects.ManagementTaskUIDAnnotation] == task.ObjectUID && annotations[kubernetesobjects.ManagementTaskNameAnnotation] == task.Logical.Name && annotations[kubernetesobjects.ManagementTaskNamespaceAnnotation] == task.Logical.Namespace
}

func managedByLogicalTask(object client.Object, task agentruntime.TaskIdentity) bool {
	annotations := object.GetAnnotations()
	if name := annotations[kubernetesobjects.ManagementTaskNameAnnotation]; name != "" {
		return name == task.Logical.Name && annotations[kubernetesobjects.ManagementTaskNamespaceAnnotation] == task.Logical.Namespace
	}
	return false
}

// legacyWorkloadForTask recognizes only the primary workload shape written by
// the pre-Coordinator controller. That controller used the Task name as the
// workload name and could leave the workload without management annotations.
// Do not extend this name-only compatibility rule to auxiliary or shared
// resources: their ownership cannot be proven from a name alone.
func legacyWorkloadForTask(object client.Object, task agentruntime.TaskIdentity) bool {
	if workloadKind(object) == "" || object.GetName() != task.Logical.Name {
		return false
	}
	compatTask := compatibilityTask(task)
	return kubernetesobjects.OwnershipForTask(object, compatTask) != kubernetesobjects.OwnershipConflict
}

func snapshotRevision(objects []client.Object) string {
	parts := make([]string, 0, len(objects))
	for _, object := range objects {
		part := fmt.Sprintf("%T:%s:%s:%s:%d", object, object.GetUID(), object.GetName(), object.GetResourceVersion(), object.GetGeneration())
		if event, ok := object.(*corev1.Event); ok {
			part += fmt.Sprintf(":%s:%s:%s", event.InvolvedObject.UID, event.Reason, event.Message)
		}
		parts = append(parts, part)
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (p *Provider) listPods(ctx context.Context, workload client.Object) ([]corev1.Pod, error) {
	selector := workloadSelector(workload)
	if selector == nil {
		return nil, fault.New(fault.InvalidArgument, "workload has no Pod selector")
	}
	parsed, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		return nil, fault.Wrap(fault.InvalidArgument, "invalid workload Pod selector", err)
	}
	var list corev1.PodList
	if err := p.client.List(ctx, &list, client.InNamespace(workload.GetNamespace()), client.MatchingLabelsSelector{Selector: parsed}); err != nil {
		return nil, fault.Wrap(fault.Internal, "list Kubernetes workload Pods", err)
	}
	pods := list.Items[:0]
	for i := range list.Items {
		if podOwnedBy(ctx, p.client, &list.Items[i], workload) {
			pods = append(pods, list.Items[i])
		}
	}
	return pods, nil
}

func workloadSelector(object client.Object) *metav1.LabelSelector {
	switch value := object.(type) {
	case *appsv1.Deployment:
		return value.Spec.Selector
	case *appsv1.DaemonSet:
		return value.Spec.Selector
	case *appsv1.StatefulSet:
		return value.Spec.Selector
	}
	return nil
}

func podOwnedBy(ctx context.Context, kubeClient client.Client, pod *corev1.Pod, workload client.Object) bool {
	owner := metav1.GetControllerOf(pod)
	if owner == nil {
		return false
	}
	if owner.Kind == workloadKind(workload) && owner.Name == workload.GetName() && owner.UID == workload.GetUID() {
		return true
	}
	if _, ok := workload.(*appsv1.Deployment); !ok || owner.Kind != "ReplicaSet" {
		return false
	}
	var replicaSet appsv1.ReplicaSet
	if err := kubeClient.Get(ctx, types.NamespacedName{Name: owner.Name, Namespace: pod.Namespace}, &replicaSet); err != nil || replicaSet.UID != owner.UID {
		return false
	}
	parent := metav1.GetControllerOf(&replicaSet)
	return parent != nil && parent.Kind == "Deployment" && parent.Name == workload.GetName() && parent.UID == workload.GetUID()
}

func workloadKind(object client.Object) string {
	switch object.(type) {
	case *appsv1.Deployment:
		return "Deployment"
	case *appsv1.DaemonSet:
		return "DaemonSet"
	case *appsv1.StatefulSet:
		return "StatefulSet"
	}
	return ""
}

func (p *Provider) podEvents(ctx context.Context, namespace string, pods []corev1.Pod) (map[string][]corev1.Event, error) {
	if len(pods) == 0 {
		return nil, nil
	}
	result := make(map[string][]corev1.Event)
	// TODO(execution): Avoid one API-server Event List per Pod on every
	// observation. Although UID-scoped queries prevent a namespace-wide Event
	// scan, this loop still makes request volume grow linearly with Pod count.
	// Periodic reconciliation also repeats these queries while a workload is in
	// the Running steady state, where scheduling and startup Events usually no
	// longer affect the observation. Introduce a bounded shared Event
	// watch/index or another cache-backed batch lookup, and stop or reduce Event
	// reads after the relevant startup window, while retaining periodic
	// state-based repair as a fallback for lost notifications. Any optimization
	// must preserve Pod UID fencing so Events from a replaced Pod are ignored.
	for i := range pods {
		pod := &pods[i]
		var list corev1.EventList
		if err := p.client.List(ctx, &list, client.InNamespace(namespace), client.MatchingFields{"involvedObject.uid": string(pod.UID)}); err != nil {
			return nil, fault.Wrap(fault.Internal, "list Kubernetes Pod events", err)
		}
		for j := range list.Items {
			event := list.Items[j]
			if event.InvolvedObject.Kind != "Pod" || event.InvolvedObject.UID != pod.UID {
				continue
			}
			result[pod.Name] = append(result[pod.Name], event)
		}
	}
	return result, nil
}

func podInstance(runtimeID agentruntime.ID, task agentruntime.TaskIdentity, parent agentruntime.ObjectRef, pod *corev1.Pod, events []corev1.Event) agentruntime.Instance {
	conditions := make([]agentruntime.InstanceCondition, 0)
	if pod.Spec.NodeName != "" {
		conditions = append(conditions, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "node", Subject: pod.Spec.NodeName})
	}
	for _, status := range pod.Status.ContainerStatuses {
		if waiting := status.State.Waiting; waiting != nil {
			conditions = append(conditions, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "container", Subject: status.Name, Reason: waiting.Reason, Message: waiting.Message})
		}
		if terminated := status.State.Terminated; terminated != nil {
			conditions = append(conditions, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "container", Subject: status.Name, Reason: terminated.Reason, Message: terminated.Message})
		}
	}
	for _, event := range events {
		conditions = append(conditions, agentruntime.InstanceCondition{Source: "kubernetes", Scope: "instance", Subject: pod.Name, Reason: event.Reason, Message: event.Message})
	}
	sort.SliceStable(conditions, func(i, j int) bool {
		left, right := conditions[i], conditions[j]
		return left.Scope+"/"+left.Subject+"/"+left.Reason+"/"+left.Message < right.Scope+"/"+right.Subject+"/"+right.Reason+"/"+right.Message
	})
	return agentruntime.Instance{Ref: agentruntime.InstanceRef{RuntimeID: runtimeID, ID: "v1, Kind=Pod/" + pod.Namespace + "/" + pod.Name, UID: string(pod.UID), Parent: parent}, Task: task, Name: pod.Name, Phase: agentruntime.InstancePhase(pod.Status.Phase), Reason: pod.Status.Reason, Message: pod.Status.Message, Conditions: conditions}
}
