package job

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/storageref"
)

// jobConditionStorageAvailable records whether every storage the Job's Task
// templates name is available in the cluster each Task goes to. It is set only
// on Jobs whose templates name a storage.
const (
	jobConditionStorageAvailable = "StorageAvailable"
	jobReasonStorageResolved     = "StorageResolved"
	jobReasonStorageUnavailable  = "StorageUnavailable"
)

// storageUnavailableError is a template naming a storage its cluster does not
// have (yet): the Job waits rather than fails, since the storage may be
// created or delivered later.
type storageUnavailableError struct{ message string }

func (e *storageUnavailableError) Error() string { return e.message }

// resolveTemplateStorage returns the template with the volumes that name a
// storage rewritten from the storage Views delivered to the Task's cluster
// (namespace). A template naming no storage is returned as is.
func (r *Reconciler) resolveTemplateStorage(ctx context.Context, t rlarkv1alpha1.JobTaskTemplate, namespace string) (rlarkv1alpha1.JobTaskTemplate, error) {
	if t.Workload.Template == nil || !storageref.HasReferences(&t.Workload.Template.Spec) {
		return t, nil
	}
	var configMaps corev1.ConfigMapList
	if err := r.List(ctx, &configMaps, client.InNamespace(namespace), client.MatchingLabels{storageref.ViewLabel: "true"}); err != nil {
		return t, fmt.Errorf("list storage views of cluster %q: %w", namespace, err)
	}
	views := make([]storageref.View, 0, len(configMaps.Items))
	for i := range configMaps.Items {
		view, err := storageref.DecodeView(&configMaps.Items[i])
		if err != nil {
			return t, err
		}
		views = append(views, view)
	}
	resolved := *t.DeepCopy()
	if err := storageref.Resolve(&resolved.Workload.Template.Spec, views, namespace); err != nil {
		return t, &storageUnavailableError{message: fmt.Sprintf("task %q: %v", t.Name, err)}
	}
	return resolved, nil
}

// setStorageCondition records the first storage a template could not
// resolve, or that every storage the Job names resolves.
func setStorageCondition(job *rlarkv1alpha1.Job, err error) bool {
	var unavailable *storageUnavailableError
	if errors.As(err, &unavailable) {
		return apimeta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
			Type: jobConditionStorageAvailable, Status: metav1.ConditionFalse, Reason: jobReasonStorageUnavailable,
			Message: unavailable.message, ObservedGeneration: job.Generation,
		})
	}
	return apimeta.SetStatusCondition(&job.Status.Conditions, metav1.Condition{
		Type: jobConditionStorageAvailable, Status: metav1.ConditionTrue, Reason: jobReasonStorageResolved, ObservedGeneration: job.Generation,
	})
}
