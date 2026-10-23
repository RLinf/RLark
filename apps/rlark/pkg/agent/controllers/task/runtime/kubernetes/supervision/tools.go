package supervision

import (
	"encoding/json"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/kubernetes/podunit"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	executionplan "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/plan"
)

const (
	initContainerName = "rlark-tools-init"
	volumeName        = "rlark-tools"
	toolSourcePath    = "/usr/local/bin/rlark-tools"
	toolTargetPath    = "/rlark-tools/rlark-tools"
	toolTargetDir     = "/rlark-tools"
)

// installTools installs the trusted RLark tools binary into one workload unit.
// Tools are a mandatory Runtime facility, not an Environment requirement.
func installTools(plan *executionplan.Plan, unit executionplan.WorkloadUnit, image string) error {
	if plan == nil {
		return fault.New(fault.Internal, "RLark tools injector received no Plan")
	}
	return mutateToolsObject(plan, unit, image)
}

func mutateToolsObject(plan *executionplan.Plan, unit executionplan.WorkloadUnit, image string) error {
	for i := range plan.Objects {
		if plan.Objects[i].LogicalName != unit.ObjectName {
			continue
		}
		if unit.Locator.Type != plan.Objects[i].Kind || unit.Locator.Type != plan.Objects[i].Manifest.Type {
			return fault.New(fault.InvalidArgument, fmt.Sprintf("tools workload unit %q locator type does not match object", unit.Name))
		}
		encoded, err := podunit.MutatePodSpec(
			plan.Objects[i].Manifest.Object,
			unit.Kind,
			unit.Locator,
			func(spec *corev1.PodSpec) error { return applyTools(spec, image) },
		)
		if err != nil {
			return err
		}
		plan.Objects[i].Manifest.Object = encoded
		return nil
	}
	return fault.New(fault.InvalidArgument, fmt.Sprintf("tools workload object %q is missing", unit.ObjectName))
}

func applyTools(spec *corev1.PodSpec, image string) error {
	if strings.TrimSpace(image) == "" {
		return fault.New(fault.InvalidArgument, "RLark tools profile image is required")
	}
	main := -1
	for i := range spec.Containers {
		if spec.Containers[i].Name == "main" {
			main = i
			break
		}
	}
	desiredVolume := corev1.Volume{Name: volumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}
	if err := ensureNamedVolume(spec, desiredVolume); err != nil {
		return err
	}
	desiredMount := corev1.VolumeMount{Name: volumeName, MountPath: toolTargetDir}
	if main >= 0 {
		if err := ensureNamedMount(&spec.Containers[main], desiredMount); err != nil {
			return err
		}
	}
	desiredInit := corev1.Container{Name: initContainerName, Image: image, ImagePullPolicy: corev1.PullIfNotPresent, Command: []string{"cp", toolSourcePath, toolTargetPath}, VolumeMounts: []corev1.VolumeMount{desiredMount}}
	return ensureNamedInitContainer(spec, desiredInit)
}

func ensureNamedVolume(spec *corev1.PodSpec, desired corev1.Volume) error {
	for _, existing := range spec.Volumes {
		if existing.Name == desired.Name {
			if !jsonEqual(existing, desired) {
				return fault.New(fault.Conflict, "RLark tools volume name is already used by a non-equivalent volume")
			}
			return nil
		}
	}
	spec.Volumes = append(spec.Volumes, desired)
	return nil
}

func ensureNamedMount(container *corev1.Container, desired corev1.VolumeMount) error {
	for _, existing := range container.VolumeMounts {
		if existing.Name == desired.Name || existing.MountPath == desired.MountPath {
			if !jsonEqual(existing, desired) {
				return fault.New(fault.Conflict, "RLark tools mount conflicts with an existing main-container mount")
			}
			return nil
		}
	}
	container.VolumeMounts = append(container.VolumeMounts, desired)
	return nil
}

func ensureNamedInitContainer(spec *corev1.PodSpec, desired corev1.Container) error {
	for _, existing := range append(append([]corev1.Container(nil), spec.InitContainers...), spec.Containers...) {
		if existing.Name == desired.Name {
			if !jsonEqual(existing, desired) {
				return fault.New(fault.Conflict, "RLark tools init-container name is already used by a non-equivalent container")
			}
			return nil
		}
	}
	spec.InitContainers = append(spec.InitContainers, desired)
	return nil
}

func jsonEqual(left, right any) bool {
	a, _ := json.Marshal(left)
	b, _ := json.Marshal(right)
	return string(a) == string(b)
}
