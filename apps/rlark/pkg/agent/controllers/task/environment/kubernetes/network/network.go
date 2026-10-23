// Package network binds RLark cross-domain networking to Kubernetes
// Pod templates. Trusted assembly supplies the immutable sidecar profile.
package network

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/kubernetes/podunit"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

const (
	ProfileType      = "KubernetesCrossDomainNetwork"
	ProfileName      = "rlark-network-sidecar"
	ProfileVersion   = "v1"
	HandlerAlgorithm = "kubernetes-cross-domain-network/v1"

	sidecarName = "rlark-network-sidecar"
	volumeName  = "rlark-nodeserver-socket"
	socketPath  = "/var/run/rlark"
)

type Profile struct {
	Image string
}

type profileValue struct {
	Image  string `json:"image"`
	Domain string `json:"domain"`
}

type Resolver struct {
	RuntimeKind agentruntime.Kind
	Profile     Profile
}

func (r Resolver) Resolve(ctx context.Context, request environment.ResolveRequest) (environment.ResolvedEnvironment, error) {
	if err := ctx.Err(); err != nil {
		return environment.ResolvedEnvironment{}, fault.Wrap(fault.DeadlineExceeded, "resolve Kubernetes network environment", err)
	}
	if request.Runtime.Kind != r.RuntimeKind || r.RuntimeKind == "" {
		return environment.ResolvedEnvironment{}, fault.New(fault.FailedPrecondition, "Kubernetes network resolver requires its configured Runtime kind")
	}
	if len(request.Requirements.Environments) > 1 {
		return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "Kubernetes network environment does not allow multiple declarations")
	}
	// Resolver has no Plan manifests. It validates target identity and shape;
	// Binder later proves each target against the concrete desired object.
	targets := make(map[string]executionplan.MutationTarget, len(request.Targets))
	for _, target := range request.Targets {
		if strings.TrimSpace(target.Name) == "" {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "environment target name is required")
		}
		if err := podunit.ValidateLocator(target.Kind, target.Locator); err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.InvalidArgument, fmt.Sprintf("environment target %q is not a supported Kubernetes Pod unit", target.Name), err)
		}
		if _, exists := targets[target.Name]; exists {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, fmt.Sprintf("duplicate environment target %q", target.Name))
		}
		targets[target.Name] = target
	}
	result := environment.ResolvedEnvironment{Task: request.Task}
	for _, requirement := range request.Requirements.Environments {
		if requirement.Name == "" || requirement.Type != taskapi.EnvironmentTypeNetwork || requirement.Version != taskapi.EnvironmentVersionNetworkV1 {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "Kubernetes network resolver supports only named rlinf.io/network/v1 requirements")
		}
		parameters, err := taskapi.DecodeParametersStrict[taskapi.NetworkEnvironmentParametersV1](apiextensionsv1.JSON{
			Raw: append([]byte(nil), requirement.Parameters...),
		})
		if err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.InvalidArgument, "decode cross-domain network requirement options", err)
		}
		if strings.TrimSpace(parameters.Domain) == "" {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "cross-domain network requirement domain is required")
		}
		if strings.TrimSpace(r.Profile.Image) == "" {
			return environment.ResolvedEnvironment{}, fault.New(fault.FailedPrecondition, "cross-domain network sidecar image is required")
		}
		consumers, err := environment.ResolveConsumers(requirement.Consumers, targets)
		if err != nil {
			return environment.ResolvedEnvironment{}, err
		}
		value, err := json.Marshal(profileValue{Image: r.Profile.Image, Domain: parameters.Domain})
		if err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.Internal, "encode Kubernetes network profile", err)
		}
		result.Bindings = append(result.Bindings, environment.ResolvedBinding{
			Kind: environment.RequirementNetwork, RequirementName: requirement.Name,
			Profile:   environment.ProfileReference{Type: ProfileType, Name: ProfileName, Version: ProfileVersion, Revision: profileRevision(value)},
			Consumers: consumers, Algorithm: HandlerAlgorithm,
			Value: agentruntime.TypedOptions{Type: agentruntime.ObjectKind{Group: "rlark.io", Version: ProfileVersion, Kind: ProfileType}, Value: value},
		})
	}
	return result, nil
}

type Handler struct {
	RuntimeKind agentruntime.Kind
}

func (h Handler) Info() environment.HandlerInfo {
	// Preserve the legacy manifest order: TensorBoard, Network, then RLark tools.
	return environment.HandlerInfo{
		ID: "kubernetes-network",
		Key: environment.HandlerKey{
			RuntimeKind: h.RuntimeKind,
			BindingKind: environment.RequirementNetwork,
			ProfileType: ProfileType,
		},
		Before:    []environment.HandlerID{"kubernetes-tools"},
		Algorithm: HandlerAlgorithm,
	}
}

func (Handler) Intents(_ context.Context, request environment.HandlerRequest) ([]environment.WriteIntent, error) {
	intents := make([]environment.WriteIntent, 0, len(request.Binding.Consumers))
	for _, consumer := range request.Binding.Consumers {
		intents = append(intents, environment.WriteIntent{Slot: environment.SemanticSlot("target/" + consumer.TargetName + "/cross-domain-network"), Value: []byte(request.Binding.Profile.Revision)})
	}
	return intents, nil
}

func (Handler) Apply(_ context.Context, request environment.HandlerRequest) error {
	var profile profileValue
	if err := json.Unmarshal(request.Binding.Value.Value, &profile); err != nil {
		return fault.Wrap(fault.InvalidArgument, "decode Kubernetes network profile", err)
	}
	for _, consumer := range request.Binding.Consumers {
		target, found := request.Targets[consumer.TargetName]
		if !found {
			return fault.New(fault.InvalidArgument, fmt.Sprintf("network consumer %q has no Kubernetes Pod target", consumer.TargetName))
		}
		if err := podunit.ValidateLocator(target.Kind, target.Locator); err != nil {
			return fault.Wrap(fault.InvalidArgument, fmt.Sprintf("network consumer %q has no supported Kubernetes Pod unit", consumer.TargetName), err)
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
			return fault.New(fault.InvalidArgument, fmt.Sprintf("network target %q locator type does not match object", target.Name))
		}
		encoded, err := podunit.Mutate(plan.Objects[i].Manifest.Object, target.Kind, target.Locator, func(metadata *metav1.ObjectMeta, spec *corev1.PodSpec) error {
			if err := applyDomainAnnotation(metadata, profile.Domain); err != nil {
				return err
			}
			return apply(spec, profile)
		})
		if err != nil {
			return err
		}
		plan.Objects[i].Manifest.Object = encoded
		return nil
	}
	return fault.New(fault.InvalidArgument, fmt.Sprintf("network target object %q is missing", target.ObjectName))
}

func applyDomainAnnotation(metadata *metav1.ObjectMeta, domain string) error {
	if strings.TrimSpace(domain) == "" {
		return fault.New(fault.InvalidArgument, "Kubernetes network profile domain is required")
	}
	if metadata.Annotations == nil {
		metadata.Annotations = map[string]string{}
	}
	metadata.Annotations[kubernetesobjects.ManagementTaskDomainAnnotation] = domain
	return nil
}

func apply(spec *corev1.PodSpec, profile profileValue) error {
	desired := corev1.Container{Name: sidecarName, Image: profile.Image, ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"/usr/local/bin/network-sidecar"}, Env: []corev1.EnvVar{{Name: "LOG_LEVEL", Value: "debug"}}, SecurityContext: &corev1.SecurityContext{Privileged: ptr.To(true), Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"NET_ADMIN"}}}, VolumeMounts: []corev1.VolumeMount{{Name: volumeName, MountPath: socketPath}}}
	desired.RestartPolicy = ptr.To(corev1.ContainerRestartPolicyAlways)
	for _, existing := range append(append([]corev1.Container(nil), spec.Containers...), spec.InitContainers...) {
		if existing.Name == sidecarName {
			if !containersEqual(existing, desired) {
				return fault.New(fault.Conflict, "network sidecar name is already used by a non-equivalent container")
			}
			return ensureVolume(spec)
		}
	}
	spec.InitContainers = append(spec.InitContainers, desired)
	return ensureVolume(spec)
}

func ensureVolume(spec *corev1.PodSpec) error {
	desired := corev1.Volume{Name: volumeName, VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: socketPath}}}
	for _, existing := range spec.Volumes {
		if existing.Name == volumeName {
			if !volumesEqual(existing, desired) {
				return fault.New(fault.Conflict, "network socket volume name is already used by a non-equivalent volume")
			}
			return nil
		}
	}
	spec.Volumes = append(spec.Volumes, desired)
	return nil
}

func containersEqual(left, right corev1.Container) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}
func volumesEqual(left, right corev1.Volume) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}
func profileRevision(value []byte) string {
	sum := sha256.Sum256(value)
	return fmt.Sprintf("sha256:%x", sum[:])
}
