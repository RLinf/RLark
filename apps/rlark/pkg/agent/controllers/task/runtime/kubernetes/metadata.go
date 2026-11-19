package kubernetes

import (
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/evidence"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

const (
	// RuntimeIDAnnotation identifies the selected Runtime Provider registration.
	RuntimeIDAnnotation = "execution.rlark.io/runtime-id"
	// RuntimeKindAnnotation identifies the Runtime implementation kind.
	RuntimeKindAnnotation = "execution.rlark.io/runtime-kind"
	// WorkloadTypeAnnotation identifies the selected workload implementation.
	WorkloadTypeAnnotation = "execution.rlark.io/workload-type"
	// WorkloadVersionAnnotation identifies the selected workload contract version.
	WorkloadVersionAnnotation = "execution.rlark.io/workload-version"
	// CompiledDigestAnnotation stores the canonical compiled-plan digest.
	CompiledDigestAnnotation = "execution.rlark.io/compiled-digest"
	// ExecutableDigestAnnotation stores the canonical bound executable-plan digest.
	ExecutableDigestAnnotation = "execution.rlark.io/executable-digest"
	// BindingRevisionAnnotation stores the canonical environment binding revision.
	BindingRevisionAnnotation = "execution.rlark.io/binding-revision"
)

// WriteExecutionRevision writes the canonical root evidence form without
// modifying Pod-template metadata. Callers must prove ownership separately.
func WriteExecutionRevision(object client.Object, revision agentruntime.ExecutionRevision) error {
	if err := validateExecutionRevision(revision); err != nil {
		return err
	}
	annotations := object.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[RuntimeIDAnnotation] = string(revision.RuntimeID)
	annotations[RuntimeKindAnnotation] = string(revision.RuntimeKind)
	annotations[WorkloadTypeAnnotation] = revision.WorkloadType
	annotations[WorkloadVersionAnnotation] = revision.WorkloadVersion
	annotations[CompiledDigestAnnotation] = revision.CompiledDigest
	annotations[ExecutableDigestAnnotation] = revision.ExecutableDigest
	annotations[BindingRevisionAnnotation] = revision.BindingRevision
	object.SetAnnotations(annotations)
	return nil
}

// ReadExecutionRevision reads only the canonical root evidence form. Missing
// or malformed legacy evidence is rejected so callers fall back to full reconcile.
func ReadExecutionRevision(object client.Object) (agentruntime.ExecutionRevision, error) {
	annotations := object.GetAnnotations()
	revision := agentruntime.ExecutionRevision{
		RuntimeID: agentruntime.ID(annotations[RuntimeIDAnnotation]), RuntimeKind: agentruntime.Kind(annotations[RuntimeKindAnnotation]),
		WorkloadType: annotations[WorkloadTypeAnnotation], WorkloadVersion: annotations[WorkloadVersionAnnotation],
		CompiledDigest: annotations[CompiledDigestAnnotation], ExecutableDigest: annotations[ExecutableDigestAnnotation], BindingRevision: annotations[BindingRevisionAnnotation],
	}
	return revision, validateExecutionRevision(revision)
}

func validateExecutionRevision(revision agentruntime.ExecutionRevision) error {
	if revision.RuntimeID == "" || revision.RuntimeKind == "" || revision.WorkloadType == "" || revision.WorkloadVersion == "" {
		return errors.New("execution root identity metadata is incomplete")
	}
	for _, value := range []string{revision.CompiledDigest, revision.ExecutableDigest, revision.BindingRevision} {
		if err := evidence.Digest(value).Validate(); err != nil {
			return errors.New("execution root revision metadata is incomplete or malformed")
		}
	}
	return nil
}
