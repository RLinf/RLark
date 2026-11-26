package job

import (
	"fmt"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

// DesiredDigestAnnotation records the digest of the desired Task state this
// controller last submitted.
//
// It exists so "does the child need to be rewritten" can be answered by
// comparing two decisions of this controller, instead of comparing the desired
// state against the live object. A live Task also carries API-server
// defaulting, admission mutation and fields other writers own; diffing against
// it reports a change on every pass and rewrites the Task forever.
const DesiredDigestAnnotation = "rlark.io/desired-digest"

// desiredTaskProjection is the canonical projection the digest is taken over.
//
// It covers exactly what this controller decides: the Task spec it compiled
// from the Job, and the restart operation that spec belongs to. Field order is
// fixed by the struct and map keys are ordered by encoding/json, so equivalent
// desired states always hash to the same digest.
type desiredTaskProjection struct {
	Spec        rlarkv1alpha1.TaskSpec `json:"spec"`
	RestartedAt string                 `json:"restartedAt,omitempty"`
}

// desiredTask builds the desired Task and stamps its canonical digest.
//
// clusterStopped carries the Cluster drain decision already recorded on the
// live Task. It is applied before the digest is taken, so the digest always
// describes the state actually submitted; otherwise a drain and its undrain
// would hash identically and the Task would never be resumed.
func desiredTask(
	job *rlarkv1alpha1.Job,
	t rlarkv1alpha1.JobTaskTemplate,
	taskName, namespace string,
	clusterStopped bool,
) (*rlarkv1alpha1.Task, error) {
	task := buildTask(job, t, taskName, namespace)
	if clusterStopped {
		task.Spec.Suspended = true
	}
	digest, err := sharedreconcile.DigestJSON(desiredTaskProjection{
		Spec:        task.Spec,
		RestartedAt: task.Annotations[RestartedAtAnnotation],
	})
	if err != nil {
		return nil, fmt.Errorf("digest desired Task %s/%s: %w", namespace, taskName, err)
	}
	task.Annotations[DesiredDigestAnnotation] = string(digest)
	return task, nil
}
