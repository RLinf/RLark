// Package ssh binds a Task's workload SSH authorized keys through a Kubernetes
// Secret volume. The Secret remains mounted for the Task lifetime; an empty key
// set disables key authentication without changing the Pod template.
package ssh

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"

	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/kubernetes/podunit"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/environment"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

const (
	RequirementName  = taskapi.EnvironmentTypeSSH
	ProfileType      = "KubernetesWorkloadSSH"
	ProfileName      = "secret-volume"
	ProfileVersion   = "v1"
	HandlerAlgorithm = "kubernetes-workload-ssh/v1"

	MaterialAuthorizedKey environment.MaterialName = "authorized-key"
	SecretLogicalName                              = "environment/ssh-authorized-key"
	SecretDataKey                                  = "authorized_key"
	VolumeName                                     = "rlark-ssh-authorized-key"
	MountPath                                      = "/var/run/rlark/ssh"
	AuthorizedKeyPath                              = MountPath + "/" + SecretDataKey
)

type profileValue struct {
	SecretName string `json:"secretName"`
	Port       int32  `json:"port"`
}

type requirementOptions struct {
	Port int32 `json:"port"`
}

// Resolver authorizes the built-in workload SSH Secret profile. It resolves no
// raw material and performs no management or Runtime I/O.
type Resolver struct{ RuntimeKind agentruntime.Kind }

func (r Resolver) Resolve(ctx context.Context, request environment.ResolveRequest) (environment.ResolvedEnvironment, error) {
	if err := ctx.Err(); err != nil {
		return environment.ResolvedEnvironment{}, fault.Wrap(fault.DeadlineExceeded, "resolve Kubernetes workload SSH environment", err)
	}
	if request.Runtime.Kind != r.RuntimeKind || r.RuntimeKind == "" {
		return environment.ResolvedEnvironment{}, fault.New(fault.FailedPrecondition, "Kubernetes workload SSH resolver requires its configured Runtime kind")
	}
	if len(request.Requirements.Environments) > 1 {
		return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "Kubernetes workload SSH environment does not allow multiple declarations")
	}
	targets := make(map[string]executionplan.MutationTarget, len(request.Targets))
	for _, target := range request.Targets {
		if err := podunit.ValidateLocator(target.Kind, target.Locator); err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.InvalidArgument, fmt.Sprintf("SSH target %q is not a supported Kubernetes Pod unit", target.Name), err)
		}
		if _, found := targets[target.Name]; found {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, fmt.Sprintf("duplicate environment target %q", target.Name))
		}
		targets[target.Name] = target
	}
	result := environment.ResolvedEnvironment{Task: request.Task}
	for _, requirement := range request.Requirements.Environments {
		if requirement.Name != RequirementName || requirement.Type != taskapi.EnvironmentTypeSSH || requirement.Version != taskapi.EnvironmentVersionSSHV1 {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "Kubernetes workload SSH resolver supports only the built-in Task SSH key source")
		}
		consumers, err := environment.ResolveConsumers(requirement.Consumers, targets)
		if err != nil {
			return environment.ResolvedEnvironment{}, err
		}
		options := requirementOptions{Port: 22}
		if len(requirement.Parameters) != 0 {
			decoded, err := taskapi.DecodeParametersStrict[requirementOptions](apiextensionsv1.JSON{
				Raw: append([]byte(nil), requirement.Parameters...),
			})
			if err != nil {
				return environment.ResolvedEnvironment{}, fault.Wrap(fault.InvalidArgument, "decode workload SSH requirement options", err)
			}
			options = decoded
		}
		if options.Port < 1 || options.Port > 65535 {
			return environment.ResolvedEnvironment{}, fault.New(fault.InvalidArgument, "workload SSH port must be between 1 and 65535")
		}
		value, err := json.Marshal(profileValue{SecretName: SecretName(request.Task), Port: options.Port})
		if err != nil {
			return environment.ResolvedEnvironment{}, fault.Wrap(fault.Internal, "encode Kubernetes workload SSH profile", err)
		}
		result.Bindings = append(result.Bindings, environment.ResolvedBinding{
			Kind: environment.RequirementSecret, RequirementName: requirement.Name,
			Profile:   environment.ProfileReference{Type: ProfileType, Name: ProfileName, Version: ProfileVersion, Revision: profileRevision(value)},
			Consumers: consumers, Ownership: incarnationOwnership(request.Task), Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup,
			SourceRevision: sourceRevision(request.Task), Algorithm: HandlerAlgorithm,
			Value: agentruntime.TypedOptions{Type: agentruntime.ObjectKind{Group: "rlark.io", Version: ProfileVersion, Kind: ProfileType}, Value: value},
		})
	}
	return result, nil
}

// Handler adds the Task-incarnation Secret declaration and mounts it into the
// exact Adapter-authorized main-container target.
type Handler struct{ RuntimeKind agentruntime.Kind }

func (h Handler) Info() environment.HandlerInfo {
	return environment.HandlerInfo{
		ID:     "kubernetes-workload-ssh",
		Key:    environment.HandlerKey{RuntimeKind: h.RuntimeKind, BindingKind: environment.RequirementSecret, ProfileType: ProfileType},
		Before: []environment.HandlerID{"kubernetes-tools"}, Algorithm: HandlerAlgorithm,
	}
}

func (Handler) Intents(_ context.Context, request environment.HandlerRequest) ([]environment.WriteIntent, error) {
	intents := []environment.WriteIntent{{Slot: "object/" + SecretLogicalName, Value: []byte(request.Binding.Profile.Revision)}}
	for _, consumer := range request.Binding.Consumers {
		intents = append(intents, environment.WriteIntent{Slot: environment.SemanticSlot("target/" + consumer.TargetName + "/workload-ssh"), Value: []byte(request.Binding.Profile.Revision)})
	}
	return intents, nil
}

func (Handler) Apply(ctx context.Context, request environment.HandlerRequest) error {
	if request.Materials == nil {
		return fault.New(fault.FailedPrecondition, "workload SSH material access is required")
	}
	lease, err := request.Materials.Acquire(ctx, MaterialAuthorizedKey)
	if err != nil {
		return err
	}
	var profile profileValue
	if err := json.Unmarshal(request.Binding.Value.Value, &profile); err != nil {
		return fault.Wrap(fault.InvalidArgument, "decode Kubernetes workload SSH profile", err)
	}
	if profile.SecretName == "" || len(validation.IsDNS1123Subdomain(profile.SecretName)) != 0 {
		return fault.New(fault.InvalidArgument, "Kubernetes workload SSH Secret name is invalid")
	}
	if profile.Port < 1 || profile.Port > 65535 {
		return fault.New(fault.InvalidArgument, "Kubernetes workload SSH profile port is invalid")
	}
	if err := addSecret(request, profile.SecretName, lease.Bytes()); err != nil {
		return err
	}
	for _, consumer := range request.Binding.Consumers {
		target, found := request.Targets[consumer.TargetName]
		if !found {
			return fault.New(fault.InvalidArgument, fmt.Sprintf("SSH consumer %q has no Kubernetes Pod target", consumer.TargetName))
		}
		if err := mutateTarget(request.Plan, target, profile.SecretName); err != nil {
			return err
		}
		request.Plan.Processes = append(request.Plan.Processes, executionprocess.Declaration{
			Name: "sshd", Target: target.Name, Locator: target.Locator, ContainerName: "main",
			Role: executionprocess.RoleAuxiliary, Executable: "/rlark-tools/rlark-tools",
			Args:          []string{"sshd", fmt.Sprintf("--port=%d", profile.Port), "--authorized-key-file=" + AuthorizedKeyPath},
			RestartPolicy: executionprocess.RestartOnFailure, Required: false,
		})
	}
	return nil
}

func addSecret(request environment.HandlerRequest, name string, key []byte) error {
	for _, object := range request.Plan.Objects {
		if object.LogicalName == SecretLogicalName {
			return fault.New(fault.Conflict, "workload SSH Secret logical name is already declared")
		}
	}
	secret := corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{
			kubernetesobjects.ManagementTaskNameAnnotation:       request.Task.Logical.Name,
			kubernetesobjects.ManagementTaskNamespaceAnnotation:  request.Task.Logical.Namespace,
			kubernetesobjects.ManagementTaskUIDAnnotation:        request.Task.ObjectUID,
			kubernetesobjects.ManagementTaskGenerationAnnotation: fmt.Sprintf("%d", request.Task.Generation),
		}},
		Type: corev1.SecretTypeOpaque, Data: map[string][]byte{SecretDataKey: append([]byte(nil), key...)},
	}
	data, err := json.Marshal(&secret)
	if err != nil {
		return fault.Wrap(fault.Internal, "encode Kubernetes workload SSH Secret", err)
	}
	kind := agentruntime.ObjectKind{Version: "v1", Kind: "Secret"}
	request.Plan.Objects = append(request.Plan.Objects, agentruntime.DesiredObject{
		LogicalName: SecretLogicalName, Role: "environment-secret", Kind: kind,
		Manifest: agentruntime.TypedManifest{Type: kind, Object: data}, Ownership: request.Binding.Ownership,
		Lifecycle: request.Binding.Lifecycle, UpdateMode: agentruntime.UpdateApplyInPlace,
		ReconcileProfile: kubernetesobjects.SecretReconcileProfile,
	})
	for i := range request.Plan.Objects {
		if request.Plan.Objects[i].LogicalName != SecretLogicalName && containsConsumerObject(request.Binding.Consumers, request.Plan.Objects[i].LogicalName) {
			if !contains(request.Plan.Objects[i].DependsOn, SecretLogicalName) {
				request.Plan.Objects[i].DependsOn = append(request.Plan.Objects[i].DependsOn, SecretLogicalName)
			}
		}
	}
	return nil
}

func mutateTarget(plan *executionplan.Plan, target executionplan.MutationTarget, secretName string) error {
	for i := range plan.Objects {
		if plan.Objects[i].LogicalName != target.ObjectName {
			continue
		}
		encoded, err := podunit.MutatePodSpec(plan.Objects[i].Manifest.Object, target.Kind, target.Locator, func(spec *corev1.PodSpec) error {
			return mountSecret(spec, secretName)
		})
		if err != nil {
			return err
		}
		plan.Objects[i].Manifest.Object = encoded
		return nil
	}
	return fault.New(fault.InvalidArgument, fmt.Sprintf("SSH target object %q is missing", target.ObjectName))
}

func mountSecret(spec *corev1.PodSpec, secretName string) error {
	desiredVolume := corev1.Volume{Name: VolumeName, VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: secretName, Items: []corev1.KeyToPath{{Key: SecretDataKey, Path: SecretDataKey}}}}}
	for _, volume := range spec.Volumes {
		if volume.Name == VolumeName {
			if !jsonEqual(volume, desiredVolume) {
				return fault.New(fault.Conflict, "workload SSH volume name is already used by a non-equivalent volume")
			}
			goto mount
		}
	}
	spec.Volumes = append(spec.Volumes, desiredVolume)
mount:
	for i := range spec.Containers {
		if spec.Containers[i].Name != "main" {
			continue
		}
		desiredMount := corev1.VolumeMount{Name: VolumeName, MountPath: MountPath, ReadOnly: true}
		for _, existing := range spec.Containers[i].VolumeMounts {
			if existing.Name == VolumeName || existing.MountPath == MountPath {
				if !jsonEqual(existing, desiredMount) {
					return fault.New(fault.Conflict, "workload SSH mount conflicts with an existing main-container mount")
				}
				return nil
			}
		}
		spec.Containers[i].VolumeMounts = append(spec.Containers[i].VolumeMounts, desiredMount)
		return nil
	}
	return fault.New(fault.InvalidArgument, "workload SSH target has no main container")
}

// AuthorizedKeys decodes SSH-owned local Task configuration. The public keys
// stay out of portable Environment requirements and workload transport.
func AuthorizedKeys(declarations []coordination.LocalEnvironmentDeclaration) ([]string, error) {
	for _, declaration := range declarations {
		if declaration.Type != taskapi.EnvironmentTypeSSH {
			continue
		}
		if declaration.Version != taskapi.EnvironmentVersionSSHV1 {
			return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("workload SSH environment version %q is unsupported", declaration.Version))
		}
		parameters, err := taskapi.DecodeParametersStrict[taskapi.SSHEnvironmentParametersV1](declaration.Parameters)
		if err != nil {
			return nil, fault.Wrap(fault.InvalidArgument, "decode workload SSH environment parameters", err)
		}
		return append([]string(nil), parameters.AuthorizedKeys...), nil
	}
	return nil, nil
}

// MaterialProvider exposes only locally decoded Task SSH authorized keys to the
// matching binding in the same reconcile. It performs no external I/O and is
// not part of portable requirements, Plans, or transport contracts.
type MaterialProvider struct {
	task agentruntime.TaskIdentity
	keys []byte
}

func NewMaterialProvider(task agentruntime.TaskIdentity, authorizedKeys []string) *MaterialProvider {
	keys := strings.Join(authorizedKeys, "\n")
	if keys != "" {
		keys += "\n"
	}
	return &MaterialProvider{task: task, keys: []byte(keys)}
}

func (p *MaterialProvider) Acquire(ctx context.Context, request environment.MaterialRequest) (environment.MaterialLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, fault.Wrap(fault.DeadlineExceeded, "acquire workload SSH material", err)
	}
	if p == nil || request.Task != p.task || request.Kind != environment.RequirementSecret || request.RequirementName != RequirementName || request.SourceRevision != sourceRevision(request.Task) || request.Name != MaterialAuthorizedKey {
		return nil, fault.New(fault.PermissionDenied, "workload SSH material request is not authorized")
	}
	return environment.NewByteMaterial(p.keys), nil
}

func SecretName(task agentruntime.TaskIdentity) string {
	sum := sha256.Sum256([]byte(task.ObjectUID))
	suffix := hex.EncodeToString(sum[:])[:10]
	base := strings.Trim(strings.ToLower(task.Logical.Name), "-.")
	if len(base) > 240 {
		base = base[:240]
	}
	return base + "-ssh-" + suffix
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
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func containsConsumerObject(consumers []environment.ResolvedConsumer, name string) bool {
	for _, consumer := range consumers {
		if consumer.ObjectName == name {
			return true
		}
	}
	return false
}
func jsonEqual(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}
