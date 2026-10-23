// Package podunit reads and writes Adapter-declared Kubernetes Pod and
// PodTemplate units without interpreting their enclosing workload kind.
package podunit

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/fault"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	unitaccessor "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime/unit"
)

// MutatePodSpec decodes one declared Pod or PodTemplate unit, invokes mutate on
// its PodSpec, and returns a new enclosing manifest with the unit written back.
// Locator paths are closed, dot-separated static fields rather than JSONPath.
func MutatePodSpec(manifest []byte, kind agentruntime.UnitKind, locator agentruntime.ObjectLocator, mutate func(*corev1.PodSpec) error) ([]byte, error) {
	return Mutate(manifest, kind, locator, func(_ *metav1.ObjectMeta, spec *corev1.PodSpec) error { return mutate(spec) })
}

// Mutate decodes one declared Pod or PodTemplate unit, invokes mutate on its
// complete metadata and PodSpec, and returns the enclosing manifest with the
// unit written back.
func Mutate(manifest []byte, kind agentruntime.UnitKind, locator agentruntime.ObjectLocator, mutate func(*metav1.ObjectMeta, *corev1.PodSpec) error) ([]byte, error) {
	value, err := unitaccessor.Read(manifest, locator.Path)
	if err != nil {
		return nil, err
	}
	if err := validateUnitShape(value); err != nil {
		return nil, err
	}
	var updated any
	switch kind {
	case agentruntime.UnitPod:
		var pod corev1.Pod
		if err := json.Unmarshal(value, &pod); err != nil {
			return nil, fault.Wrap(fault.InvalidArgument, "decode Kubernetes Pod unit", err)
		}
		if err := mutate(&pod.ObjectMeta, &pod.Spec); err != nil {
			return nil, err
		}
		updated = pod
	case agentruntime.UnitPodTemplate:
		var template corev1.PodTemplateSpec
		if err := json.Unmarshal(value, &template); err != nil {
			return nil, fault.Wrap(fault.InvalidArgument, "decode Kubernetes PodTemplate unit", err)
		}
		if err := mutate(&template.ObjectMeta, &template.Spec); err != nil {
			return nil, err
		}
		updated = template
	default:
		return nil, fault.New(fault.InvalidArgument, fmt.Sprintf("unsupported Kubernetes unit kind %q", kind))
	}
	updatedJSON, err := json.Marshal(updated)
	if err != nil {
		return nil, fault.Wrap(fault.Internal, "encode mutated Kubernetes unit", err)
	}
	return unitaccessor.Write(manifest, locator.Path, updatedJSON)
}

// Validate reads and decodes one declared Pod or PodTemplate unit without
// rewriting the enclosing manifest.
func Validate(manifest []byte, kind agentruntime.UnitKind, locator agentruntime.ObjectLocator) error {
	if err := ValidateLocator(kind, locator); err != nil {
		return err
	}
	value, err := unitaccessor.Read(manifest, locator.Path)
	if err != nil {
		return err
	}
	if err := validateUnitShape(value); err != nil {
		return err
	}
	switch kind {
	case agentruntime.UnitPod:
		var pod corev1.Pod
		if err := json.Unmarshal(value, &pod); err != nil {
			return fault.Wrap(fault.InvalidArgument, "decode Kubernetes Pod unit", err)
		}
	case agentruntime.UnitPodTemplate:
		var template corev1.PodTemplateSpec
		if err := json.Unmarshal(value, &template); err != nil {
			return fault.Wrap(fault.InvalidArgument, "decode Kubernetes PodTemplate unit", err)
		}
	default:
		return fault.New(fault.InvalidArgument, fmt.Sprintf("unsupported Kubernetes unit kind %q", kind))
	}
	return nil
}

func validateUnitShape(value []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(value, &object); err != nil {
		return fault.Wrap(fault.InvalidArgument, "decode Kubernetes unit structure", err)
	}
	spec, found := object["spec"]
	if !found || string(spec) == "null" {
		return fault.New(fault.InvalidArgument, "Kubernetes Pod unit requires an object spec")
	}
	var specObject map[string]json.RawMessage
	if err := json.Unmarshal(spec, &specObject); err != nil {
		return fault.Wrap(fault.InvalidArgument, "decode Kubernetes Pod unit spec", err)
	}
	return nil
}

// ValidateLocator validates the closed locator syntax accepted by MutatePodSpec.
func ValidateLocator(kind agentruntime.UnitKind, locator agentruntime.ObjectLocator) error {
	if kind != agentruntime.UnitPod && kind != agentruntime.UnitPodTemplate {
		return fault.New(fault.InvalidArgument, fmt.Sprintf("unsupported Kubernetes unit kind %q", kind))
	}
	_, err := unitaccessor.StaticPath(locator.Path)
	return err
}
