// Package tensorboard binds a Workload-selected TensorBoard service
// to declared Kubernetes Pod and PodTemplate targets.
package tensorboard

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/kubernetes/podunit"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

const (
	ServiceTensorBoard = "tensorboard"
	ProfileType        = "KubernetesTensorBoard"
	ProfileName        = "tensorboard"
	ProfileVersion     = "v1"
	HandlerAlgorithm   = "kubernetes-tensorboard/v1"
	sidecarName        = "tensorboard"
	volumeName         = "rlark-tensorboard"
	port               = int32(6006)
)

type Profile struct{ Image string }

type profileValue struct {
	Image     string `json:"image"`
	Directory string `json:"directory"`
}

// Parameters returns canonical portable TensorBoard environment parameters.
func Parameters(directory string) []byte {
	value, _ := json.Marshal(taskapi.TensorBoardEnvironmentParametersV1{LogDir: directory})
	return value
}

type Resolver struct {
	RuntimeKind agentruntime.Kind
	Profile     Profile
}

func (r Resolver) Resolve(ctx context.Context, request environment.ResolveRequest) (environment.ResolvedEnvironment, error) {
	if err := ctx.Err(); err != nil {
		return environment.ResolvedEnvironment{}, fault.Wrap(fault.DeadlineExceeded, "resolve Kubernetes TensorBoard environment", err)
	}
	if request.Runtime.Kind != r.RuntimeKind || r.RuntimeKind == "" {
		return environment.ResolvedEnvironment{}, fault.New(fault.FailedPrecondition, "Kubernetes TensorBoard resolver requires its configured Runtime kind")
	}
	if len(request.Requirements.Environments) > 1 {
		return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "Kubernetes TensorBoard environment does not allow multiple declarations")
	}
	targets := make(map[string]executionplan.MutationTarget, len(request.Targets))
	for _, target := range request.Targets {
		if err := podunit.ValidateLocator(target.Kind, target.Locator); err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.InvalidArgument, fmt.Sprintf("TensorBoard target %q is not a supported Kubernetes Pod unit", target.Name), err)
		}
		if _, found := targets[target.Name]; found {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, fmt.Sprintf("duplicate environment target %q", target.Name))
		}
		targets[target.Name] = target
	}
	result := environment.ResolvedEnvironment{Task: request.Task}
	for _, requirement := range request.Requirements.Environments {
		if strings.TrimSpace(requirement.Name) == "" || requirement.Type != taskapi.EnvironmentTypeTensorBoard || requirement.Version != taskapi.EnvironmentVersionTensorBoardV1 {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "Kubernetes TensorBoard resolver supports only named tensorboard requirements")
		}
		if strings.TrimSpace(r.Profile.Image) == "" {
			return environment.ResolvedEnvironment{}, fault.New(fault.FailedPrecondition, "TensorBoard image is required")
		}
		options, err := taskapi.DecodeParametersStrict[taskapi.TensorBoardEnvironmentParametersV1](apiextensionsv1.JSON{
			Raw: append([]byte(nil), requirement.Parameters...),
		})
		if err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.InvalidArgument, "decode TensorBoard requirement options", err)
		}
		directory, err := validateDirectory(options.LogDir)
		if err != nil {
			return environment.ResolvedEnvironment{}, err
		}
		consumers, err := environment.ResolveConsumers(requirement.Consumers, targets)
		if err != nil {
			return environment.ResolvedEnvironment{}, err
		}
		value, err := json.Marshal(profileValue{Image: r.Profile.Image, Directory: directory})
		if err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.Internal, "encode Kubernetes TensorBoard profile", err)
		}
		result.Bindings = append(result.Bindings, environment.ResolvedBinding{
			Kind: environment.RequirementAuxiliaryService, RequirementName: requirement.Name,
			Profile:   environment.ProfileReference{Type: ProfileType, Name: ProfileName, Version: ProfileVersion, Revision: profileRevision(value)},
			Consumers: consumers, Algorithm: HandlerAlgorithm,
			Value: agentruntime.TypedOptions{Type: agentruntime.ObjectKind{Group: "rlark.io", Version: ProfileVersion, Kind: ProfileType}, Value: value},
		})
	}
	return result, nil
}

type Handler struct{ RuntimeKind agentruntime.Kind }

func (h Handler) Info() environment.HandlerInfo {
	// Preserve the legacy manifest order: TensorBoard, Network, then RLark tools.
	return environment.HandlerInfo{
		ID: "kubernetes-tensorboard",
		Key: environment.HandlerKey{
			RuntimeKind: h.RuntimeKind,
			BindingKind: environment.RequirementAuxiliaryService,
			ProfileType: ProfileType,
		},
		Before:    []environment.HandlerID{"kubernetes-network"},
		Algorithm: HandlerAlgorithm,
	}
}

func (Handler) Intents(_ context.Context, request environment.HandlerRequest) ([]environment.WriteIntent, error) {
	intents := make([]environment.WriteIntent, 0, len(request.Binding.Consumers))
	for _, consumer := range request.Binding.Consumers {
		intents = append(intents, environment.WriteIntent{Slot: environment.SemanticSlot("target/" + consumer.TargetName + "/tensorboard"), Value: []byte(request.Binding.Profile.Revision)})
	}
	return intents, nil
}

func (Handler) Apply(_ context.Context, request environment.HandlerRequest) error {
	var profile profileValue
	if err := json.Unmarshal(request.Binding.Value.Value, &profile); err != nil {
		return fault.Wrap(fault.InvalidArgument, "decode Kubernetes TensorBoard profile", err)
	}
	for _, consumer := range request.Binding.Consumers {
		target, found := request.Targets[consumer.TargetName]
		if !found {
			return fault.New(fault.InvalidArgument, fmt.Sprintf("TensorBoard consumer %q has no Kubernetes Pod target", consumer.TargetName))
		}
		if err := mutateObject(request.Plan, target, profile); err != nil {
			return err
		}
	}
	return nil
}

func mutateObject(plan *executionplan.Plan, target executionplan.MutationTarget, profile profileValue) error {
	for i := range plan.Objects {
		if plan.Objects[i].LogicalName != target.ObjectName {
			continue
		}
		if target.Locator.Type != plan.Objects[i].Kind || target.Locator.Type != plan.Objects[i].Manifest.Type {
			return fault.New(fault.InvalidArgument, fmt.Sprintf("TensorBoard target %q locator type does not match object", target.Name))
		}
		encoded, err := podunit.MutatePodSpec(plan.Objects[i].Manifest.Object, target.Kind, target.Locator, func(spec *corev1.PodSpec) error { return apply(spec, profile) })
		if err != nil {
			return err
		}
		plan.Objects[i].Manifest.Object = encoded
		return nil
	}
	return fault.New(fault.InvalidArgument, fmt.Sprintf("TensorBoard target object %q is missing", target.ObjectName))
}

func apply(spec *corev1.PodSpec, profile profileValue) error {
	directory, err := validateDirectory(profile.Directory)
	if err != nil {
		return err
	}
	if strings.TrimSpace(profile.Image) == "" {
		return fault.New(fault.InvalidArgument, "TensorBoard profile image is required")
	}
	main := -1
	for i := range spec.Containers {
		if spec.Containers[i].Name == "main" {
			main = i
			break
		}
	}
	if main < 0 {
		return fault.New(fault.InvalidArgument, "TensorBoard target requires the main container")
	}
	selectedVolume, mountPath, subPath := "", "", ""
	for _, mount := range spec.Containers[main].VolumeMounts {
		if pathCovers(mount.MountPath, directory) {
			selectedVolume, mountPath, subPath = mount.Name, mount.MountPath, mount.SubPath
			break
		}
	}
	if selectedVolume == "" {
		selectedVolume, mountPath = volumeName, directory
		desiredVolume := corev1.Volume{Name: selectedVolume, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}
		if err := ensureVolume(spec, desiredVolume); err != nil {
			return err
		}
		if err := ensureMount(&spec.Containers[main], corev1.VolumeMount{Name: selectedVolume, MountPath: mountPath}); err != nil {
			return err
		}
	}
	desired := corev1.Container{Name: sidecarName, Image: profile.Image, ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"tensorboard"}, Args: []string{"--logdir=" + directory, "--bind_all"}, Ports: []corev1.ContainerPort{{Name: "tensorboard", ContainerPort: port, Protocol: corev1.ProtocolTCP}}, VolumeMounts: []corev1.VolumeMount{{Name: selectedVolume, MountPath: mountPath, SubPath: subPath}}, RestartPolicy: ptr.To(corev1.ContainerRestartPolicyAlways)}
	for _, existing := range append(append([]corev1.Container(nil), spec.Containers...), spec.InitContainers...) {
		if existing.Name == sidecarName {
			if !jsonEqual(existing, desired) {
				return fault.New(fault.Conflict, "TensorBoard sidecar name is already used by a non-equivalent container")
			}
			return nil
		}
	}
	spec.InitContainers = append(spec.InitContainers, desired)
	return nil
}

func validateDirectory(directory string) (string, error) {
	if strings.TrimSpace(directory) == "" || !path.IsAbs(directory) {
		return "", fault.New(fault.InvalidArgument, "TensorBoard directory must be an absolute path")
	}
	return path.Clean(directory), nil
}

func pathCovers(mountPath, directory string) bool {
	mount, dir := path.Clean(mountPath), path.Clean(directory)
	return mount == "/" || dir == mount || strings.HasPrefix(dir, mount+"/")
}

func ensureVolume(spec *corev1.PodSpec, desired corev1.Volume) error {
	for _, existing := range spec.Volumes {
		if existing.Name == desired.Name {
			if !jsonEqual(existing, desired) {
				return fault.New(fault.Conflict, "TensorBoard volume name is already used by a non-equivalent volume")
			}
			return nil
		}
	}
	spec.Volumes = append(spec.Volumes, desired)
	return nil
}

func ensureMount(container *corev1.Container, desired corev1.VolumeMount) error {
	for _, existing := range container.VolumeMounts {
		if existing.Name == desired.Name || existing.MountPath == desired.MountPath {
			if !jsonEqual(existing, desired) {
				return fault.New(fault.Conflict, "TensorBoard mount conflicts with an existing main-container mount")
			}
			return nil
		}
	}
	container.VolumeMounts = append(container.VolumeMounts, desired)
	return nil
}

func jsonEqual(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}

func profileRevision(value []byte) string {
	sum := sha256.Sum256(value)
	return fmt.Sprintf("sha256:%x", sum[:])
}
