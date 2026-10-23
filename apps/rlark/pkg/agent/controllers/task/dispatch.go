package task

import (
	"context"
	"fmt"
	"slices"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes"
	kubernetesobjects "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/compatibility/kubernetes/kubernetesobjects"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/evidence"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
)

func (c *Controller) recordVerifiedExecution(snapshot coordination.TaskSnapshot, result coordinator.ReconcileResult) {
	if result.Revision.RuntimeID == "" || len(result.Evaluation.Roots) == 0 {
		return
	}
	revision := evidence.Revision{
		Runtime: snapshot.RuntimeSelector, Workload: snapshot.WorkloadSelector,
		CompiledDigest: evidence.Digest(result.Revision.CompiledDigest), ExecutableDigest: evidence.Digest(result.Revision.ExecutableDigest), BindingRevision: evidence.Digest(result.Revision.BindingRevision),
	}
	roots := make(map[string]struct{}, len(result.Evaluation.Roots))
	for _, root := range result.Evaluation.Roots {
		roots[root.UID] = struct{}{}
	}
	key := trigger.Key{Namespace: snapshot.Identity.Logical.Namespace, Name: snapshot.Identity.Logical.Name}
	c.evidenceMu.Lock()
	if c.evidence == nil {
		c.evidence = make(map[trigger.Key]verifiedExecution)
	}
	c.evidence[key] = verifiedExecution{taskUID: snapshot.Identity.ObjectUID, generation: snapshot.Identity.Generation, revision: revision, roots: roots}
	c.evidenceMu.Unlock()
}

func (c *Controller) clearVerifiedExecution(key trigger.Key) {
	c.evidenceMu.Lock()
	delete(c.evidence, key)
	c.evidenceMu.Unlock()
}

func (c *Controller) dispatch(ctx context.Context, key types.NamespacedName, hints trigger.Set) (host.Result, error) {
	if c.host == nil {
		return host.Result{}, fmt.Errorf("task Host is not configured")
	}
	// This is the first-stage hint gate: only one-root Runtime hints can qualify
	// for verified fast observation. evidence.Decide evaluates the richer Task,
	// ownership, and revision evidence only after a candidate passes this gate.
	if hints.FullReconcileRequired || hints.Has(trigger.DesiredState) || hints.Has(trigger.Periodic) || len(hints.Roots) != 1 {
		c.clearVerifiedExecution(hints.Key)
		return c.host.Reconcile(ctx, key)
	}
	// A partially assembled controller must never panic in the optimization.
	// Setup validation rejects this state in production; retain a runtime guard
	// for direct reconciler use and tests.
	if err := c.validateExecutionDependencies(); err != nil {
		return host.Result{}, err
	}
	fast, ok := c.fastCandidate(ctx, key, hints)
	if !ok {
		return c.host.Reconcile(ctx, key)
	}
	// Verified fast observation intentionally bypasses Host.Reconcile, including
	// its keyed lock and Lifecycle Before/After hooks. It must remain read-only
	// against Runtime desired state and may perform only one UID/generation-fenced
	// workload Status write. Effects required by every reconciliation (including
	// credential, Secret, or lifecycle preparation) must not live exclusively in
	// Lifecycle hooks; adding such an effect requires disabling or extending this
	// gate so the invariant is proved before the fast path is selected.
	observed, err := c.execution.ObserveRoot(ctx, fast.snapshot, fast.root)
	if err != nil {
		return c.host.Reconcile(ctx, key)
	}
	intent := host.IntentForTask(fast.task, rlarkv1alpha1.TaskPhase(observed.Observation.Phase), observed.Observation.Message, observedRootInstanceNodes(observed), host.StatusPriorityWorkload)
	if err := c.status.WriteStatus(ctx, fast.task, intent); err != nil {
		return host.Result{Outcome: host.OutcomeRetrying, Retry: host.RetryDirective{Class: host.RetryConflict}}, err
	}
	return host.Result{Outcome: host.OutcomeSucceeded, Retry: host.RetryDirective{Class: host.RetryNone}, Reason: "ObservedRuntimeRoot", Status: &intent}, nil
}

type fastExecution struct {
	task     *rlarkv1alpha1.Task
	snapshot coordination.TaskSnapshot
	root     agentruntime.ObjectRef
}

func (c *Controller) fastCandidate(ctx context.Context, key types.NamespacedName, hints trigger.Set) (fastExecution, bool) {
	var task rlarkv1alpha1.Task
	if err := c.ManagementClient.Get(ctx, key, &task); err != nil {
		return fastExecution{}, false
	}
	snapshot, err := compatibility.DecodeTask(&task)
	if err != nil {
		return fastExecution{}, false
	}
	snapshot.RuntimeSelector = c.host.Bundle.RuntimeSelector
	c.evidenceMu.RLock()
	verified, found := c.evidence[hints.Key]
	c.evidenceMu.RUnlock()
	root := hints.Roots[0]
	if !found || verified.taskUID != string(task.UID) || verified.generation != task.Generation {
		return fastExecution{}, false
	}
	if _, found := verified.roots[root.UID]; !found {
		return fastExecution{}, false
	}
	object, err := c.executionRootObject(ctx, root)
	if err != nil {
		return fastExecution{}, false
	}
	revision, err := kubernetesruntime.ReadExecutionRevision(object)
	if err != nil {
		return fastExecution{}, false
	}
	rootRevision := evidence.Revision{
		Runtime:        coordination.RuntimeSelector{ID: revision.RuntimeID, Kind: revision.RuntimeKind},
		Workload:       coordination.WorkloadSelector{Type: revision.WorkloadType, Version: revision.WorkloadVersion},
		CompiledDigest: evidence.Digest(revision.CompiledDigest), ExecutableDigest: evidence.Digest(revision.ExecutableDigest), BindingRevision: evidence.Digest(revision.BindingRevision),
	}
	ownership := kubernetesobjects.OwnershipForTask(object, &task)
	owner := agentruntime.ResourceOwnership{}
	if ownership == kubernetesobjects.OwnershipCurrent {
		owner = agentruntime.ResourceOwnership{Scope: agentruntime.IdentityTaskIncarnation, OwnerKey: string(task.UID)}
	}
	decision := evidence.Decide(evidence.Current{
		Identity: snapshot.Identity, Runtime: snapshot.RuntimeSelector, Workload: snapshot.WorkloadSelector,
		Claimed: slices.Contains(task.Finalizers, host.TaskFinalizer), ClaimedByHost: task.Annotations[host.TaskClaimantAnnotation] == c.host.Claimant,
		Deleting: task.DeletionTimestamp != nil, LifecyclePending: task.Spec.Suspended,
	}, evidence.Candidate{
		ObservedTaskUID: hints.ObjectUID, Generation: hints.Generation, RuntimeID: hints.RuntimeID,
		Root: evidence.Root{Ref: root, Task: snapshot.Identity, Revision: rootRevision, Ownership: owner}, Expected: verified.revision,
	})
	if decision.Mode != evidence.ModeObserveRoot {
		return fastExecution{}, false
	}
	return fastExecution{task: &task, snapshot: snapshot, root: root}, true
}

func (c *Controller) executionRootObject(ctx context.Context, ref agentruntime.ObjectRef) (client.Object, error) {
	var object client.Object
	switch ref.ID {
	case "apps/v1, Kind=Deployment":
		object = &appsv1.Deployment{}
	case "apps/v1, Kind=DaemonSet":
		object = &appsv1.DaemonSet{}
	case "apps/v1, Kind=StatefulSet":
		object = &appsv1.StatefulSet{}
	default:
		return nil, fmt.Errorf("unsupported execution root %q", ref.ID)
	}
	if err := c.LocalKubeClient.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, object); err != nil {
		return nil, err
	}
	if ref.UID == "" || string(object.GetUID()) != ref.UID {
		return nil, fmt.Errorf("execution root UID changed")
	}
	return object, nil
}
