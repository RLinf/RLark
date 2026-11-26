package job

import (
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/sweep"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// OrphanTaskRule reclaims Tasks whose Job is gone.
//
// Task cleanup is owned by the Job controller's finalizer and by Kubernetes
// garbage collection; this rule is the backstop for the leftovers neither
// handles. Parent name candidates come from the same helper as the field
// index, which derives them from the controller OwnerReference and the legacy
// Job label. A Task created by hand references no Job and therefore yields no
// candidate, so the sweeper leaves it alone instead of guessing. Candidates
// are only candidates: the sweeper proves ownership against the same identity
// the Job controller uses before it reclaims anything.
func OrphanTaskRule() sweep.Rule {
	return sweep.Rule{
		Name:        "Task",
		Child:       func() client.Object { return &rlarkv1alpha1.Task{} },
		ChildList:   func() client.ObjectList { return &rlarkv1alpha1.TaskList{} },
		Parent:      func() client.Object { return &rlarkv1alpha1.Job{} },
		ParentNames: indexTaskByJob,
		ParentRef: func(parent client.Object) utils.ParentRef {
			job, ok := parent.(*rlarkv1alpha1.Job)
			if !ok {
				return utils.ParentRef{}
			}
			return jobParentRef(job)
		},
	}
}
