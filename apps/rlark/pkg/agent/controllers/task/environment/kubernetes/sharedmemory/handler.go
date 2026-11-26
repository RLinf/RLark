// Package sharedmemory binds a Task's shared-memory (/dev/shm) requirement by
// mounting a memory-backed emptyDir into every regular workload container.
// The emptyDir lives for the Pod lifetime and is discarded with the Pod.
package sharedmemory

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/kubernetes/podunit"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const (
	RequirementName  = taskapi.EnvironmentTypeSharedMemory
	ProfileType      = "KubernetesWorkloadSharedMemory"
	ProfileName      = "emptydir-memory"
	ProfileVersion   = "v1"
	HandlerAlgorithm = "kubernetes-workload-shared-memory/v1"

	VolumeName = "rlark-dev-shm"
	MountPath  = "/dev/shm"
)

type profileValue struct {
	Size string `json:"size"`
}

// Resolver authorizes the built-in workload shared-memory profile. It resolves
// no raw material and performs no management or Runtime I/O.
type Resolver struct{ RuntimeKind agentruntime.Kind }

func (r Resolver) Resolve(ctx context.Context, request environment.ResolveRequest) (environment.ResolvedEnvironment, error) {
	if err := ctx.Err(); err != nil {
		return environment.ResolvedEnvironment{}, fault.Wrap(fault.DeadlineExceeded, "resolve Kubernetes workload shared memory environment", err)
	}
	if request.Runtime.Kind != r.RuntimeKind || r.RuntimeKind == "" {
		return environment.ResolvedEnvironment{}, fault.New(fault.FailedPrecondition, "Kubernetes workload shared memory resolver requires its configured Runtime kind")
	}
	targets := make(map[string]executionplan.MutationTarget, len(request.Targets))
	for _, target := range request.Targets {
		if err := podunit.ValidateLocator(target.Kind, target.Locator); err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.InvalidArgument, fmt.Sprintf("shared memory target %q is not a supported Kubernetes Pod unit", target.Name), err)
		}
		if _, found := targets[target.Name]; found {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, fmt.Sprintf("duplicate environment target %q", target.Name))
		}
		targets[target.Name] = target
	}
	if len(request.Requirements.Environments) > 1 {
		return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "Kubernetes workload shared memory does not allow multiple declarations")
	}
	result := environment.ResolvedEnvironment{Task: request.Task}
	for _, requirement := range request.Requirements.Environments {
		if requirement.Name != RequirementName || requirement.Type != taskapi.EnvironmentTypeSharedMemory || requirement.Version != taskapi.EnvironmentVersionSharedMemoryV1 {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "Kubernetes workload shared memory resolver supports only the built-in Task shared memory source")
		}
		consumers, err := environment.ResolveConsumers(requirement.Consumers, targets)
		if err != nil {
			return environment.ResolvedEnvironment{}, err
		}
		options, err := taskapi.DecodeParametersStrict[taskapi.SharedMemoryEnvironmentParametersV1](apiextensionsv1.JSON{Raw: append([]byte(nil), requirement.Parameters...)})
		if err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.InvalidArgument, "decode workload shared memory requirement options", err)
		}
		size, err := resource.ParseQuantity(options.Size)
		if err != nil || size.IsZero() || size.Sign() <= 0 {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "workload shared memory size must be a positive quantity")
		}
		value, err := json.Marshal(profileValue{Size: size.String()})
		if err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.Internal, "encode Kubernetes workload shared memory profile", err)
		}
		result.Bindings = append(result.Bindings, environment.ResolvedBinding{
			Kind: environment.RequirementSharedMemory, RequirementName: requirement.Name,
			Profile:   environment.ProfileReference{Type: ProfileType, Name: ProfileName, Version: ProfileVersion, Revision: profileRevision(value)},
			Consumers: consumers, Ownership: incarnationOwnership(request.Task), Lifecycle: agentruntime.LifecycleRuntimeManaged,
			SourceRevision: sourceRevision(request.Task), Algorithm: HandlerAlgorithm,
			Value: agentruntime.TypedOptions{Type: agentruntime.ObjectKind{Group: "rlark.io", Version: ProfileVersion, Kind: ProfileType}, Value: value},
		})
	}
	return result, nil
}

// Handler mounts a memory-backed emptyDir as /dev/shm into every regular
// container of the authorized Kubernetes Pod targets.
type Handler struct{ RuntimeKind agentruntime.Kind }

func (h Handler) Info() environment.HandlerInfo {
	return environment.HandlerInfo{
		ID:        "kubernetes-workload-shared-memory",
		Key:       environment.HandlerKey{RuntimeKind: h.RuntimeKind, BindingKind: environment.RequirementSharedMemory, ProfileType: ProfileType},
		Algorithm: HandlerAlgorithm,
	}
}

func (Handler) Intents(_ context.Context, request environment.HandlerRequest) ([]environment.WriteIntent, error) {
	var profile profileValue
	if err := json.Unmarshal(request.Binding.Value.Value, &profile); err != nil {
		return nil, fault.Wrap(fault.InvalidArgument, "decode Kubernetes workload shared memory profile", err)
	}
	intents := make([]environment.WriteIntent, 0, len(request.Binding.Consumers))
	for _, consumer := range request.Binding.Consumers {
		intents = append(intents, environment.WriteIntent{
			Slot:  environment.SemanticSlot("target/" + consumer.TargetName + "/shared-memory"),
			Value: []byte(profile.Size),
		})
	}
	return intents, nil
}

func (Handler) Apply(ctx context.Context, request environment.HandlerRequest) error {
	if err := ctx.Err(); err != nil {
		return fault.Wrap(fault.DeadlineExceeded, "apply Kubernetes workload shared memory", err)
	}
	var profile profileValue
	if err := json.Unmarshal(request.Binding.Value.Value, &profile); err != nil {
		return fault.Wrap(fault.InvalidArgument, "decode Kubernetes workload shared memory profile", err)
	}
	size, err := resource.ParseQuantity(profile.Size)
	if err != nil || size.IsZero() || size.Sign() <= 0 {
		return fault.New(fault.InvalidArgument, "Kubernetes workload shared memory profile size is invalid")
	}
	for _, consumer := range request.Binding.Consumers {
		target, found := request.Targets[consumer.TargetName]
		if !found {
			return fault.New(fault.InvalidArgument, fmt.Sprintf("shared memory consumer %q has no Kubernetes Pod target", consumer.TargetName))
		}
		if err := mutateTarget(request.Plan, target, size); err != nil {
			return err
		}
	}
	return nil
}

func mutateTarget(plan *executionplan.Plan, target executionplan.MutationTarget, size resource.Quantity) error {
	for i := range plan.Objects {
		if plan.Objects[i].LogicalName != target.ObjectName {
			continue
		}
		encoded, err := podunit.MutatePodSpec(plan.Objects[i].Manifest.Object, target.Kind, target.Locator, func(spec *corev1.PodSpec) error {
			return mountSharedMemory(spec, size)
		})
		if err != nil {
			return err
		}
		plan.Objects[i].Manifest.Object = encoded
		return nil
	}
	return fault.New(fault.InvalidArgument, fmt.Sprintf("shared memory target object %q is missing", target.ObjectName))
}

func mountSharedMemory(spec *corev1.PodSpec, size resource.Quantity) error {
	sizeLimit := size.DeepCopy()
	desiredVolume := corev1.Volume{
		Name: VolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory, SizeLimit: &sizeLimit},
		},
	}
	for _, volume := range spec.Volumes {
		if volume.Name == VolumeName {
			if !utils.JSONEqual(volume, desiredVolume) {
				return fault.New(fault.Conflict, "workload shared memory volume name is already used by a non-equivalent volume")
			}
			return mountAllContainers(spec)
		}
	}
	spec.Volumes = append(spec.Volumes, desiredVolume)
	return mountAllContainers(spec)
}

// desiredMount is the exact VolumeMount the handler wants to inject. Any
// existing mount at /dev/shm must be semantically equivalent to be accepted.
var desiredMount = corev1.VolumeMount{Name: VolumeName, MountPath: MountPath}

func mountAllContainers(spec *corev1.PodSpec) error {
	if len(spec.Containers) == 0 {
		return fault.New(fault.InvalidArgument, "shared memory target has no regular containers")
	}
	for i := range spec.Containers {
		mounted := false
		for _, existing := range spec.Containers[i].VolumeMounts {
			if existing.MountPath != MountPath {
				continue
			}
			if !mountEquivalent(existing) {
				return fault.New(fault.Conflict, fmt.Sprintf("shared memory mount path %q is already used by a non-equivalent mount in container %q", MountPath, spec.Containers[i].Name))
			}
			mounted = true
			break
		}
		if mounted {
			continue
		}
		spec.Containers[i].VolumeMounts = append(spec.Containers[i].VolumeMounts, desiredMount)
	}
	return nil
}

// mountEquivalent reports whether an existing mount at /dev/shm is
// semantically equivalent to the handler's desired mount. A mount is
// equivalent only when volume name, mount path, read-only flag, and sub-path
// fields all match. Read-only mounts or mounts with sub-paths are NOT
// equivalent because they change the semantics of the shared memory volume.
func mountEquivalent(existing corev1.VolumeMount) bool {
	return existing.Name == VolumeName &&
		existing.MountPath == MountPath &&
		!existing.ReadOnly &&
		existing.SubPath == "" &&
		existing.SubPathExpr == ""
}

func sourceRevision(task agentruntime.TaskIdentity) string {
	return fmt.Sprintf("task-generation:%d", task.Generation)
}
func profileRevision(value []byte) string {
	sum := sha256.Sum256(value)
	return fmt.Sprintf("sha256:%x", sum[:])
}
func incarnationOwnership(task agentruntime.TaskIdentity) agentruntime.ResourceOwnership {
	return agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: task.ObjectUID}
}
