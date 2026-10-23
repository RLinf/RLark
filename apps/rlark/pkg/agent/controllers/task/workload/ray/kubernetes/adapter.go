package kubernetes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	kubernetesssh "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/environment/kubernetes/ssh"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/workload"
)

// NamespaceResolver maps a management Task identity to local Kubernetes placement.
type NamespaceResolver func(agentruntime.TaskIdentity) string

const workloadPodTemplateTarget = "workload-pod-template"

// Adapter implements the Ray workload contract on Kubernetes carriers.
type Adapter struct {
	typeID    workload.TypeID
	namespace NamespaceResolver
}

// NewAdapter constructs the legacy Ray-oriented Kubernetes Workload Adapter.
// typeID and kind remain the existing API discriminators for compatibility.
func NewAdapter(typeID workload.TypeID, namespace NamespaceResolver) *Adapter {
	return &Adapter{
		typeID:    typeID,
		namespace: namespace,
	}
}

var _ workload.Adapter = (*Adapter)(nil)
var _ workload.InventoryAdapter = (*Adapter)(nil)

// Info returns the Adapter's versioned Workload contract.
func (a *Adapter) Info(context.Context) (workload.TypeInfo, error) {
	return workload.TypeInfo{
		ID:                    a.typeID,
		Version:               taskapi.WorkloadVersionRayV1,
		SupportedRuntimeKinds: []agentruntime.Kind{compatibility.RuntimeKindKubernetes},
		Configuration: workload.ConfigurationContract{
			Version: taskapi.WorkloadVersionRayV1,
		},
		Capabilities: []agentruntime.Capability{
			agentruntime.CapabilityWorkloadInventory,
			agentruntime.CapabilityOwnedFieldProjection,
			agentruntime.CapabilityIncarnationAdoption,
			agentruntime.CapabilityReconcileProfiles,
		},
	}, nil
}

// Inventory reconstructs bounded cleanup candidates from frozen Task identity.
// It does not decode or compile the current Task Spec.
func (a *Adapter) Inventory(_ context.Context, request workload.InventoryRequest) (workload.Inventory, error) {
	if request.Runtime.Kind != compatibility.RuntimeKindKubernetes {
		return workload.Inventory{}, fault.New(fault.FailedPrecondition, "builtin Kubernetes workload requires a Kubernetes Runtime")
	}
	if err := request.Task.Logical.Validate(); err != nil {
		return workload.Inventory{}, fault.Wrap(fault.InvalidArgument, "invalid Task identity", err)
	}
	namespace := ""
	if a.namespace != nil {
		namespace = a.namespace(request.Task)
	}
	if namespace == "" {
		return workload.Inventory{}, fault.New(fault.FailedPrecondition, "Kubernetes namespace resolver returned empty")
	}
	kind, err := kubernetesCarrierKind(coordination.VersionedConfiguration{
		Type:    request.Carrier.Type,
		Version: request.Carrier.Version,
	})
	if err != nil {
		return workload.Inventory{}, err
	}
	primaryKind := agentruntime.ObjectKind{
		Group:   appsv1.GroupName,
		Version: appsv1.SchemeGroupVersion.Version,
		Kind:    string(kind),
	}
	return workload.Inventory{Candidates: []agentruntime.ObjectCandidate{
		// Inventory is declared in create order. Identity cleanup reverses exact
		// DeleteOnTaskCleanup candidates one at a time, so the Secret outlives all
		// workload consumers during identity cleanup.
		{
			Kind: agentruntime.ObjectKind{
				Version: corev1.SchemeGroupVersion.Version,
				Kind:    "ConfigMap",
			},
			Namespace: namespace,
			Name:      kubernetesobjects.SupervisorConfigMapName(request.Task),
			Ownership: agentruntime.ResourceOwnership{
				Scope:    agentruntime.IdentityTaskIncarnation,
				OwnerKey: request.Task.ObjectUID,
			},
			Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup,
		},
		{
			Kind: agentruntime.ObjectKind{
				Version: corev1.SchemeGroupVersion.Version,
				Kind:    "Secret",
			},
			Namespace: namespace,
			Name:      kubernetesssh.SecretName(request.Task),
			Ownership: agentruntime.ResourceOwnership{
				Scope:    agentruntime.IdentityTaskIncarnation,
				OwnerKey: request.Task.ObjectUID,
			},
			Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup,
		},
		{
			Kind:      primaryKind,
			Namespace: namespace,
			Name:      request.Task.Logical.Name,
			Ownership: incarnationOwnership(request.Task),
			Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup,
		},
		{
			Kind: agentruntime.ObjectKind{
				Version: corev1.SchemeGroupVersion.Version,
				Kind:    "Service",
			},
			Namespace: namespace,
			Name:      RayHeadServiceName(request.Task.Logical.Name),
			Ownership: incarnationOwnership(request.Task),
			Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup,
		},
	}}, nil
}

// Compile produces a complete Kubernetes object DAG for a Task snapshot.
func (a *Adapter) Compile(_ context.Context, request workload.CompileRequest) (executionplan.Plan, error) {
	// 1. Validate the Runtime/Adapter pairing and decode the typed Ray execution
	// plus the selected common workload configuration.
	if request.Runtime.Kind != compatibility.RuntimeKindKubernetes {
		return executionplan.Plan{}, fault.New(fault.FailedPrecondition, "builtin Kubernetes workload requires a Kubernetes Runtime")
	}
	task, spec, ray, err := decodeConfiguration(request)
	if err != nil {
		return executionplan.Plan{}, err
	}
	kind, replicas, err := decodeKubernetesCarrier(request.Task.Configuration.Carrier)
	if err != nil {
		return executionplan.Plan{}, err
	}
	namespace := ""
	if a.namespace != nil {
		namespace = a.namespace(request.Task.Identity)
	}
	if namespace == "" {
		return executionplan.Plan{}, fault.New(fault.FailedPrecondition, "Kubernetes namespace resolver returned empty")
	}
	// 2. Apply compatibility mutations and stable Task-incarnation identity to
	// the Pod template before rendering any desired Runtime object.
	ApplyTemplateMutations(spec.Template, task)
	ApplyWorkloadIdentity(spec.Template, task)
	domain, err := networkDomain(request.Task.Environment)
	if err != nil {
		return executionplan.Plan{}, err
	}
	if err := applyRayEnvironment(&spec.Template.Spec, ray, domain); err != nil {
		return executionplan.Plan{}, err
	}
	objects := make([]agentruntime.DesiredObject, 0)
	dependencies := make([]string, 0)
	// 3. Compile optional Ray helpers. The head Service has Task identity and is cleaned up.
	role := string(ray.Role)
	if role != "" {
		if role == rlarkv1alpha1.RayRoleHead {
			service := BuildRayHeadService(namespace, task)
			object, err := desiredObject("ray-service", "service", service, request.Task.Identity, agentruntime.IdentityTaskIncarnation, agentruntime.LifecycleDeleteOnTaskCleanup)
			if err != nil {
				return executionplan.Plan{}, err
			}
			object.Ownership = incarnationOwnership(request.Task.Identity)
			object.ReconcileProfile = kubernetesobjects.ServiceReconcileProfile
			object.UpdateMode = agentruntime.UpdateReplace
			objects, dependencies = append(objects, object), append(dependencies, "ray-service")
		}
	}
	// 4. Render the selected primary workload as the instance-discovery root. It
	// depends on required Ray helpers and is deleted on Task cleanup.
	var primary runtime.Object
	switch kind {
	case CarrierDeployment:
		primary = BuildDeployment(task, namespace, spec, replicas)
	case CarrierDaemonSet:
		primary = BuildDaemonSet(task, namespace, spec)
	case CarrierStatefulSet:
		primary = BuildStatefulSet(task, namespace, spec, replicas)
	default:
		return executionplan.Plan{}, fault.New(fault.InvalidArgument, "unsupported builtin Kubernetes carrier kind")
	}
	main, err := desiredObject("workload", "workload", primary, request.Task.Identity, agentruntime.IdentityTaskIncarnation, agentruntime.LifecycleDeleteOnTaskCleanup)
	if err != nil {
		return executionplan.Plan{}, err
	}
	main.Ownership = incarnationOwnership(request.Task.Identity)
	main.ReconcileProfile = kubernetesobjects.WorkloadReconcileProfile
	main.DependsOn = dependencies
	objects = append(objects, main)
	// 5. Declare the only PodTemplate location that trusted Environment handlers
	// may mutate. Cross-domain networking applies to the complete workload Pod.
	targets := []executionplan.MutationTarget{{
		Name:       workloadPodTemplateTarget,
		ObjectName: "workload",
		Kind:       executionplan.TargetPodTemplate,
		Role:       "workload",
		Locator: agentruntime.ObjectLocator{
			Type: main.Kind,
			Path: "spec.template",
		},
	}}
	units := []executionplan.WorkloadUnit{{
		Name:       "workload-pod-template",
		ObjectName: "workload",
		Kind:       agentruntime.UnitPodTemplate,
		Role:       "workload",
		Locator: agentruntime.ObjectLocator{
			Type: main.Kind,
			Path: "spec.template",
		},
	}}
	processes := []executionprocess.Declaration{}
	if role == rlarkv1alpha1.RayRoleHead || role == rlarkv1alpha1.RayRoleWorker {
		args := []string{"ray-launcher", "--role=" + role}
		if domain != "" {
			args = append(args, "--wait-network")
		}
		processes = append(processes, executionprocess.Declaration{
			Name: "ray-launcher", Target: workloadPodTemplateTarget, Locator: targets[0].Locator, ContainerName: "main",
			Role: executionprocess.RolePrimary, Executable: "/rlark-tools/rlark-tools", Args: args,
			RestartPolicy: executionprocess.RestartNever, Required: true,
		})
	}
	// 6. Hash Adapter-owned objects and process declarations independently of
	// execution ordering. Environment requirements and targets bind separately.
	hash, err := compiledHash(objects, processes)
	if err != nil {
		return executionplan.Plan{}, err
	}
	return executionplan.Plan{
		AdapterID:      string(a.typeID),
		AdapterVersion: taskapi.WorkloadVersionRayV1,
		CompiledHash:   hash,
		Objects:        objects,
		Targets:        targets,
		Units:          units,
		Processes:      processes,
		Discovery: executionplan.InstanceDiscovery{
			Roots: []string{"workload"},
		},
	}, nil
}

func networkDomain(requirements executionplan.Requirements) (string, error) {
	domain := ""
	for _, requirement := range requirements.Environments {
		if requirement.Type != taskapi.EnvironmentTypeNetwork {
			continue
		}
		if requirement.Version != taskapi.EnvironmentVersionNetworkV1 {
			return "", fault.New(fault.InvalidArgument, "Ray workload requires a supported Network environment version")
		}
		if domain != "" {
			return "", fault.New(fault.InvalidArgument, "Ray workload has duplicate Network environments")
		}
		parameters, err := taskapi.DecodeParametersStrict[taskapi.NetworkEnvironmentParametersV1](apiextensionsv1.JSON{Raw: append([]byte(nil), requirement.Parameters...)})
		if err != nil {
			return "", fault.Wrap(fault.InvalidArgument, "decode Ray Network environment", err)
		}
		domain = parameters.Domain
	}
	return domain, nil
}

func applyRayEnvironment(spec *corev1.PodSpec, ray taskapi.RayExecutionParametersV1, domain string) error {
	main := -1
	mainCount := 0
	for i := range spec.Containers {
		if spec.Containers[i].Name == "main" {
			main = i
			mainCount++
		}
	}
	if mainCount != 1 {
		return fault.New(fault.InvalidArgument, fmt.Sprintf("Ray workload requires exactly one main container, got %d", mainCount))
	}

	desired := map[string]corev1.EnvVar{
		"RLARK_RAY_PORT":        {Name: "RLARK_RAY_PORT", Value: "6379"},
		"RLARK_NODE_RANK_START": {Name: "RLARK_NODE_RANK_START", Value: strconv.Itoa(int(ray.NodeRankStart))},
		"POD_NAME":              {Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
	}
	if ray.Prepare != nil {
		desired["RLARK_PREPARE_SCRIPT"] = corev1.EnvVar{Name: "RLARK_PREPARE_SCRIPT", Value: ray.Prepare.Inline}
	}
	if ray.Run != nil {
		desired["RLARK_RUN_SCRIPT"] = corev1.EnvVar{Name: "RLARK_RUN_SCRIPT", Value: ray.Run.Inline}
	}
	switch ray.Role {
	case taskapi.RayRoleWorker:
		headAddress := RayHeadServiceName(ray.HeadTaskName)
		if domain != "" {
			headAddress = ray.HeadTaskName + "-0.rlark-domain"
		}
		desired["RLARK_HEAD_ADDRESS"] = corev1.EnvVar{Name: "RLARK_HEAD_ADDRESS", Value: headAddress}
	case taskapi.RayRoleHead:
		desired["RLARK_TOTAL_NODES"] = corev1.EnvVar{Name: "RLARK_TOTAL_NODES", Value: strconv.Itoa(int(ray.TotalNodes))}
		desired["RLARK_NODE_IP"] = corev1.EnvVar{Name: "RLARK_NODE_IP", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.podIP"}}}
		if domain != "" {
			desired["RLARK_DOMAIN"] = corev1.EnvVar{Name: "RLARK_DOMAIN", Value: domain}
		}
	}

	container := &spec.Containers[main]
	for i := range container.Env {
		if replacement, found := desired[container.Env[i].Name]; found {
			container.Env[i] = replacement
			delete(desired, container.Env[i].Name)
		}
	}
	for _, name := range []string{"RLARK_RAY_PORT", "RLARK_HEAD_ADDRESS", "RLARK_TOTAL_NODES", "RLARK_NODE_RANK_START", "RLARK_NODE_IP", "RLARK_DOMAIN", "POD_NAME", "RLARK_PREPARE_SCRIPT", "RLARK_RUN_SCRIPT"} {
		if value, found := desired[name]; found {
			container.Env = append(container.Env, value)
		}
	}
	return nil
}

func incarnationOwnership(task agentruntime.TaskIdentity) agentruntime.ResourceOwnership {
	return agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: ownerKey(agentruntime.IdentityTaskIncarnation, task), IncarnationPolicy: agentruntime.IncarnationOwnershipAllowAdoption}
}

// Observe projects Kubernetes object and Pod snapshots into Task state.
func (a *Adapter) Observe(_ context.Context, request workload.ObserveRequest) (workload.Observation, error) {
	kind, err := kubernetesCarrierKind(request.Task.Configuration.Carrier)
	if err != nil {
		return workload.Observation{}, err
	}
	var primary agentruntime.Object
	found := false
	for _, object := range request.Objects.Objects {
		if object.Manifest.Type.Kind == string(kind) {
			primary, found = object, true
			break
		}
	}
	if !found {
		return workload.Observation{Phase: workload.Phase(rlarkv1alpha1.TaskPhasePending)}, nil
	}
	phase, message, err := ObserveObject(kind, primary.Manifest.Object, request.Instances)
	if err != nil {
		return workload.Observation{}, err
	}
	if phase == rlarkv1alpha1.TaskPhasePending && HasFailedScheduling(request.Instances) {
		message = "FailedScheduling"
	}
	return workload.Observation{Phase: workload.Phase(phase), Message: message}, nil
}

func decodeConfiguration(request workload.CompileRequest) (*rlarkv1alpha1.Task, *rlarkv1alpha1.TaskWorkloadSpec, taskapi.RayExecutionParametersV1, error) {
	if request.Task.WorkloadSelector.Type != taskapi.WorkloadTypeRay || request.Task.WorkloadSelector.Version != taskapi.WorkloadVersionRayV1 {
		return nil, nil, taskapi.RayExecutionParametersV1{}, fault.New(fault.InvalidArgument, "builtin Ray adapter requires rlinf.io/ray/v1 workload")
	}
	ray, err := taskapi.DecodeParametersStrict[taskapi.RayExecutionParametersV1](apiextensionsv1.JSON{Raw: append([]byte(nil), request.Task.Configuration.Parameters...)})
	if err != nil {
		return nil, nil, taskapi.RayExecutionParametersV1{}, fault.Wrap(fault.InvalidArgument, "decode Ray execution parameters", err)
	}
	if ray.Role != taskapi.RayRoleHead && ray.Role != taskapi.RayRoleWorker {
		return nil, nil, taskapi.RayExecutionParametersV1{}, fault.New(fault.InvalidArgument, "Ray execution role must be head or worker")
	}
	identity := request.Task.Identity
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: identity.Logical.Name, Namespace: identity.Logical.Namespace, UID: types.UID(identity.ObjectUID), Generation: identity.Generation}}
	task.Annotations = map[string]string{rlarkv1alpha1.RayRoleAnnotation: string(ray.Role)}
	if ray.HeadTaskName != "" {
		task.Annotations[rlarkv1alpha1.RayHeadTaskNameAnnotation] = ray.HeadTaskName
	}
	spec := rlarkv1alpha1.TaskWorkloadSpec{
		Kind:     taskapi.WorkloadTypeRay,
		Version:  taskapi.WorkloadVersionRayV1,
		Template: request.Task.Configuration.Template.DeepCopy(),
		Carrier:  rlarkv1alpha1.TaskWorkloadCarrier{Type: request.Task.Configuration.Carrier.Type, Version: request.Task.Configuration.Carrier.Version},
	}
	task.Spec.Workload = *spec.DeepCopy()
	return task, &spec, ray, nil
}

func kubernetesCarrierKind(carrier coordination.VersionedConfiguration) (string, error) {
	if carrier.Version != taskapi.CarrierVersionKubernetesV1 {
		return "", fault.New(fault.InvalidArgument, "builtin Kubernetes carrier requires version v1")
	}
	switch carrier.Type {
	case taskapi.CarrierTypeKubernetesDeployment:
		return CarrierDeployment, nil
	case taskapi.CarrierTypeKubernetesDaemonSet:
		return CarrierDaemonSet, nil
	case taskapi.CarrierTypeKubernetesStatefulSet:
		return CarrierStatefulSet, nil
	default:
		return "", fault.New(fault.InvalidArgument, "unsupported Kubernetes carrier type")
	}
}

func decodeKubernetesCarrier(carrier coordination.VersionedConfiguration) (string, *int32, error) {
	kind, err := kubernetesCarrierKind(carrier)
	if err != nil {
		return "", nil, err
	}
	parameters := apiextensionsv1.JSON{Raw: append([]byte(nil), carrier.Parameters...)}
	switch kind {
	case CarrierDeployment, CarrierStatefulSet:
		decoded, err := taskapi.DecodeParametersStrict[taskapi.KubernetesReplicatedCarrierParametersV1](parameters)
		if err != nil {
			return "", nil, fault.Wrap(fault.InvalidArgument, "decode replicated Kubernetes carrier parameters", err)
		}
		return kind, decoded.Replicas, nil
	case CarrierDaemonSet:
		if _, err := taskapi.DecodeParametersStrict[taskapi.KubernetesDaemonSetCarrierParametersV1](parameters); err != nil {
			return "", nil, fault.Wrap(fault.InvalidArgument, "decode DaemonSet carrier parameters", err)
		}
		return kind, nil, nil
	default:
		return "", nil, fault.New(fault.InvalidArgument, "unsupported Kubernetes carrier type")
	}
}

func desiredObject(logicalName, role string, object runtime.Object, task agentruntime.TaskIdentity, scope agentruntime.IdentityScope, lifecycle agentruntime.ResourceLifecycle) (agentruntime.DesiredObject, error) {
	gvk := object.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		gvk = gvkFor(object)
	}
	data, err := json.Marshal(object)
	if err != nil {
		return agentruntime.DesiredObject{}, err
	}
	kind := agentruntime.ObjectKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind}
	return agentruntime.DesiredObject{
		LogicalName: logicalName,
		Role:        role,
		Kind:        kind,
		Manifest: agentruntime.TypedManifest{
			Type:   kind,
			Object: data,
		},
		Ownership: agentruntime.ResourceOwnership{
			Scope:    scope,
			OwnerKey: ownerKey(scope, task),
		},
		Lifecycle:  lifecycle,
		UpdateMode: agentruntime.UpdateApplyInPlace,
	}, nil
}

func gvkFor(object runtime.Object) schema.GroupVersionKind {
	switch object.(type) {
	case *appsv1.Deployment:
		return appsv1.SchemeGroupVersion.WithKind("Deployment")
	case *appsv1.DaemonSet:
		return appsv1.SchemeGroupVersion.WithKind("DaemonSet")
	case *appsv1.StatefulSet:
		return appsv1.SchemeGroupVersion.WithKind("StatefulSet")
	case *corev1.ConfigMap:
		return corev1.SchemeGroupVersion.WithKind("ConfigMap")
	case *corev1.Service:
		return corev1.SchemeGroupVersion.WithKind("Service")
	}
	return schema.GroupVersionKind{}
}

func ownerKey(scope agentruntime.IdentityScope, task agentruntime.TaskIdentity) string {
	if scope == agentruntime.IdentityTaskIncarnation {
		return task.ObjectUID
	}
	if scope == agentruntime.IdentitySharedResource {
		return "kubernetes/ray-config"
	}
	return task.Logical.String()
}

func compiledHash(objects []agentruntime.DesiredObject, processes []executionprocess.Declaration) (string, error) {
	copyObjects := append([]agentruntime.DesiredObject(nil), objects...)
	sort.Slice(copyObjects, func(i, j int) bool { return copyObjects[i].LogicalName < copyObjects[j].LogicalName })
	copyProcesses := append([]executionprocess.Declaration(nil), processes...)
	sort.Slice(copyProcesses, func(i, j int) bool { return copyProcesses[i].Name < copyProcesses[j].Name })
	data, err := json.Marshal(struct {
		Objects   []agentruntime.DesiredObject
		Processes []executionprocess.Declaration
	}{copyObjects, copyProcesses})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// ObserveObject projects one primary Kubernetes workload and its instances into Task state.
func ObserveObject(kind string, manifest []byte, instances agentruntime.InstanceSnapshot) (rlarkv1alpha1.TaskPhase, string, error) {
	var phase rlarkv1alpha1.TaskPhase
	var message string
	switch kind {
	case CarrierDeployment:
		var object appsv1.Deployment
		if err := json.Unmarshal(manifest, &object); err != nil {
			return "", "", err
		}
		phase, message = DeploymentStatusPhase(&object)
	case CarrierDaemonSet:
		var object appsv1.DaemonSet
		if err := json.Unmarshal(manifest, &object); err != nil {
			return "", "", err
		}
		phase = DaemonSetStatusPhase(&object)
	case CarrierStatefulSet:
		var object appsv1.StatefulSet
		if err := json.Unmarshal(manifest, &object); err != nil {
			return "", "", err
		}
		phase = StatefulSetStatusPhase(&object)
	default:
		return "", "", fmt.Errorf("unsupported workload kind %q", kind)
	}
	if failure, found := InstanceFailureMessage(instances); found && phase != rlarkv1alpha1.TaskPhaseSucceeded {
		return rlarkv1alpha1.TaskPhaseFailed, failure, nil
	}
	return phase, message, nil
}
