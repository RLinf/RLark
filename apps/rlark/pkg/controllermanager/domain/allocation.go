package domain

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// podTaskRef are the Task coordinates a reported Pod carries. A control-plane
// Pod records only its Task, never its Job, so the Job dimension of an IP
// allocation has to be resolved from the Task itself.
type podTaskRef struct {
	namespace string
	name      string
}

// taskJobResolver answers "which Job owns this Task" for one reconcile pass.
//
// The answer comes from the Task's controller owner reference, which is the
// only provable ownership link; a Job label would be a hint the Job controller
// does not guarantee. Results are memoised for the pass because a single Job
// usually contributes many Pods, and a miss is memoised too so a Task that is
// already gone is not looked up once per Pod.
type taskJobResolver struct {
	reader client.Reader
	jobs   map[string]string
}

func newTaskJobResolver(reader client.Reader) *taskJobResolver {
	return &taskJobResolver{reader: reader, jobs: make(map[string]string)}
}

// jobFor returns the name of the Job owning the referenced Task. An empty name
// is a valid answer: the Task may be gone, or it may have been created by
// something other than a Job. The allocation is still recorded in that case,
// only without the Job dimension.
func (r *taskJobResolver) jobFor(ctx context.Context, ref podTaskRef) (string, error) {
	if ref.namespace == "" || ref.name == "" {
		return "", nil
	}
	key := ref.namespace + "/" + ref.name
	if job, ok := r.jobs[key]; ok {
		return job, nil
	}
	var task rlarkv1alpha1.Task
	if err := r.reader.Get(ctx, types.NamespacedName{Namespace: ref.namespace, Name: ref.name}, &task); err != nil {
		if apierrors.IsNotFound(err) {
			r.jobs[key] = ""
			return "", nil
		}
		return "", fmt.Errorf("get Task %s: %w", key, err)
	}
	job := ""
	if owner := metav1.GetControllerOf(&task); owner != nil &&
		owner.Kind == "Job" && owner.APIVersion == rlarkv1alpha1.GroupVersion.String() {
		job = owner.Name
	}
	r.jobs[key] = job
	return job, nil
}

// resolveAllocationOwners fills in the Job and Task dimensions of the
// allocations whose Pod is still reported in this Domain.
//
// It runs over the whole list rather than only over freshly allocated entries
// on purpose. An allocation retained across passes is never rewritten, so
// entries written by an older control plane -- which left both fields empty --
// would keep their blanks forever otherwise. An allocation whose Pod is no
// longer reported is left untouched: the only evidence of its Task has gone
// with the Pod, and blanking what is already recorded would lose information.
func resolveAllocationOwners(
	ctx context.Context,
	resolver *taskJobResolver,
	allocations []rlarkv1alpha1.DomainIPAllocation,
	refs map[string]podTaskRef,
) error {
	for i := range allocations {
		alloc := &allocations[i]
		if alloc.Task != "" {
			continue
		}
		ref, ok := refs[alloc.Pod]
		if !ok || ref.namespace == "" || ref.name == "" {
			continue
		}
		job, err := resolver.jobFor(ctx, ref)
		if err != nil {
			return err
		}
		alloc.Job = job
		alloc.Task = ref.namespace + "/" + ref.name
	}
	return nil
}
