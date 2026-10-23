// Package supervision installs mandatory RLark workload tools and lowers
// trusted process declarations into Kubernetes supervisor configuration.
package supervision

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/kubernetes/podunit"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/binding"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
	executionprocess "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/process"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	workloadsupervisor "github.com/rlinf/rlark/apps/rlark/pkg/rlarktools/supervisor"
)

const (
	LogicalName        = "environment/process-supervisor"
	DataKey            = "supervisor.json"
	VolumeName         = "rlark-process-supervisor"
	MountPath          = "/var/run/rlark/processes"
	ConfigPath         = MountPath + "/" + DataKey
	RevisionAnnotation = "rlark.io/supervisor-config-revision"
)

const (
	defaultPrimaryName = "rlark-idle"
	toolsExecutable    = "/rlark-tools/rlark-tools"
)

type Assembler struct {
	ToolsImage string
}

func (a Assembler) Assemble(ctx context.Context, request binding.BindRequest, plan *executionplan.Plan) error {
	if err := ctx.Err(); err != nil {
		return fault.Wrap(fault.DeadlineExceeded, "assemble supervisor processes", err)
	}
	if plan == nil {
		return fault.New(fault.Internal, "supervisor assembler received no Plan")
	}
	if len(plan.Units) == 0 {
		return nil
	}
	if len(plan.Units) != 1 {
		return fault.New(fault.FailedPrecondition, "Kubernetes supervisor currently requires exactly one workload unit")
	}
	unit := plan.Units[0]
	targets := make(map[string]executionplan.MutationTarget, len(plan.Targets))
	for _, target := range plan.Targets {
		if _, found := targets[target.Name]; found {
			return fault.New(fault.InvalidArgument, "duplicate process target name")
		}
		targets[target.Name] = target
	}
	unitTarget, found := targetForUnit(unit, targets)
	if !found {
		return fault.New(fault.FailedPrecondition, "workload unit has no matching process target")
	}
	if err := installTools(plan, unit, a.ToolsImage); err != nil {
		return err
	}
	declarations := append([]executionprocess.Declaration(nil), plan.Processes...)
	if !hasPrimary(declarations) {
		declarations = append(declarations, executionprocess.Declaration{
			Name:          defaultPrimaryName,
			Target:        unitTarget.Name,
			Locator:       unitTarget.Locator,
			ContainerName: "main",
			Role:          executionprocess.RolePrimary,
			Executable:    toolsExecutable,
			Args:          []string{"idle"},
			RestartPolicy: executionprocess.RestartNever,
			Required:      true,
		})
	}
	sort.SliceStable(declarations, func(i, j int) bool {
		if declarations[i].Role != declarations[j].Role {
			return declarations[i].Role == executionprocess.RolePrimary
		}
		return declarations[i].Name < declarations[j].Name
	})
	var target executionplan.MutationTarget
	primary := 0
	seen := map[string]struct{}{}
	config := workloadsupervisor.Config{APIVersion: workloadsupervisor.APIVersion}
	for _, declaration := range declarations {
		resolved, found := targets[declaration.Target]
		if !found || resolved.Locator != declaration.Locator {
			return fault.New(fault.InvalidArgument, "process declaration references an unknown or mismatched target")
		}
		if resolved.Name != unitTarget.Name {
			return fault.New(fault.InvalidArgument, "process declaration does not reference the workload unit")
		}
		if target.Name == "" {
			target = resolved
		} else if target.Name != resolved.Name {
			return fault.New(fault.InvalidArgument, "process declarations must target one execution unit")
		}
		if declaration.ContainerName != "main" || strings.TrimSpace(declaration.Name) == "" {
			return fault.New(fault.InvalidArgument, "process declaration requires a name and main container target")
		}
		if _, found := seen[declaration.Name]; found {
			return fault.New(fault.InvalidArgument, "duplicate process declaration name")
		}
		seen[declaration.Name] = struct{}{}
		if !filepath.IsAbs(declaration.Executable) || strings.ContainsRune(declaration.Executable, '\x00') {
			return fault.New(fault.InvalidArgument, "process executable must be an absolute path without NUL")
		}
		for _, arg := range declaration.Args {
			if strings.ContainsRune(arg, '\x00') {
				return fault.New(fault.InvalidArgument, "process argument contains NUL")
			}
		}
		switch declaration.Role {
		case executionprocess.RolePrimary:
			primary++
			if declaration.RestartPolicy != executionprocess.RestartNever || !declaration.Required {
				return fault.New(fault.InvalidArgument, "primary process must be required and use RestartNever")
			}
		case executionprocess.RoleAuxiliary:
			if declaration.RestartPolicy != executionprocess.RestartNever && declaration.RestartPolicy != executionprocess.RestartOnFailure {
				return fault.New(fault.InvalidArgument, "auxiliary process has an unsupported restart policy")
			}
		default:
			return fault.New(fault.InvalidArgument, "process declaration has an unknown role")
		}
		config.Processes = append(config.Processes, workloadsupervisor.ProcessConfig{Name: declaration.Name, Role: declaration.Role, Argv: append([]string{declaration.Executable}, declaration.Args...), RestartPolicy: declaration.RestartPolicy, Required: declaration.Required})
	}
	if primary != 1 {
		return fault.New(fault.InvalidArgument, "supervisor target must declare exactly one primary process")
	}
	data, err := json.Marshal(config)
	if err != nil {
		return fault.Wrap(fault.Internal, "encode supervisor config", err)
	}
	name := kubernetesobjects.SupervisorConfigMapName(request.Environment.Task)
	if err := addConfigMap(plan, request.Environment.Task, name, data, target.ObjectName); err != nil {
		return err
	}
	return mutateTarget(plan, target, name, data)
}

func targetForUnit(unit executionplan.WorkloadUnit, targets map[string]executionplan.MutationTarget) (executionplan.MutationTarget, bool) {
	for _, target := range targets {
		if target.ObjectName == unit.ObjectName && target.Kind == unit.Kind && target.Locator == unit.Locator {
			return target, true
		}
	}
	return executionplan.MutationTarget{}, false
}

func hasPrimary(declarations []executionprocess.Declaration) bool {
	for _, declaration := range declarations {
		if declaration.Role == executionprocess.RolePrimary {
			return true
		}
	}
	return false
}

func addConfigMap(plan *executionplan.Plan, task agentruntime.TaskIdentity, name string, data []byte, consumer string) error {
	for _, object := range plan.Objects {
		if object.LogicalName == LogicalName {
			return fault.New(fault.Conflict, "supervisor ConfigMap logical name is already declared")
		}
	}
	cm := corev1.ConfigMap{TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"}, ObjectMeta: metav1.ObjectMeta{Name: name, Annotations: map[string]string{kubernetesobjects.ManagementTaskNameAnnotation: task.Logical.Name, kubernetesobjects.ManagementTaskNamespaceAnnotation: task.Logical.Namespace, kubernetesobjects.ManagementTaskUIDAnnotation: task.ObjectUID, kubernetesobjects.ManagementTaskGenerationAnnotation: fmt.Sprintf("%d", task.Generation)}}, Data: map[string]string{DataKey: string(data)}}
	manifest, err := json.Marshal(cm)
	if err != nil {
		return fault.Wrap(fault.Internal, "encode supervisor ConfigMap", err)
	}
	kind := agentruntime.ObjectKind{Version: "v1", Kind: "ConfigMap"}
	plan.Objects = append(plan.Objects, agentruntime.DesiredObject{LogicalName: LogicalName, Role: "environment-config", Kind: kind, Manifest: agentruntime.TypedManifest{Type: kind, Object: manifest}, Ownership: agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: task.ObjectUID}, Lifecycle: agentruntime.LifecycleDeleteOnTaskCleanup, UpdateMode: agentruntime.UpdateApplyInPlace, ReconcileProfile: kubernetesobjects.ConfigReconcileProfile})
	for i := range plan.Objects {
		if plan.Objects[i].LogicalName == consumer {
			plan.Objects[i].DependsOn = append(plan.Objects[i].DependsOn, LogicalName)
		}
	}
	return nil
}

func mutateTarget(plan *executionplan.Plan, target executionplan.MutationTarget, name string, config []byte) error {
	for i := range plan.Objects {
		if plan.Objects[i].LogicalName != target.ObjectName {
			continue
		}
		encoded, err := podunit.Mutate(plan.Objects[i].Manifest.Object, target.Kind, target.Locator, func(metadata *metav1.ObjectMeta, spec *corev1.PodSpec) error {
			sum := sha256.Sum256(config)
			if metadata.Annotations == nil {
				metadata.Annotations = map[string]string{}
			}
			metadata.Annotations[RevisionAnnotation] = fmt.Sprintf("sha256:%x", sum[:])
			desiredVolume := corev1.Volume{Name: VolumeName, VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: name}, Items: []corev1.KeyToPath{{Key: DataKey, Path: DataKey}}}}}
			for _, volume := range spec.Volumes {
				if volume.Name == VolumeName {
					return fault.New(fault.Conflict, "supervisor volume name is already used")
				}
			}
			spec.Volumes = append(spec.Volumes, desiredVolume)
			for j := range spec.Containers {
				if spec.Containers[j].Name == "main" {
					toolsMounted := false
					for _, mount := range spec.Containers[j].VolumeMounts {
						if mount.MountPath == "/rlark-tools" {
							toolsMounted = true
						}
						if mount.Name == VolumeName || mount.MountPath == MountPath {
							return fault.New(fault.Conflict, "supervisor mount conflicts with an existing main-container mount")
						}
					}
					if !toolsMounted {
						return fault.New(fault.FailedPrecondition, "supervisor requires the RLark tools mount")
					}
					spec.Containers[j].Command = []string{"/rlark-tools/rlark-tools"}
					spec.Containers[j].Args = []string{"supervisor", "--config=" + ConfigPath}
					spec.Containers[j].VolumeMounts = append(spec.Containers[j].VolumeMounts, corev1.VolumeMount{Name: VolumeName, MountPath: MountPath, ReadOnly: true})
					return nil
				}
			}
			return fault.New(fault.InvalidArgument, "supervisor target has no main container")
		})
		if err != nil {
			return err
		}
		plan.Objects[i].Manifest.Object = encoded
		return nil
	}
	return fault.New(fault.InvalidArgument, "supervisor target object is missing")
}
