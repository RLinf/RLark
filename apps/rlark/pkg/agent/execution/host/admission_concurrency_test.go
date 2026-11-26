package host

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestAdmissionStatusCASRejectsConcurrentChanges(t *testing.T) {
	for _, change := range []string{"spec", "status", "deletion", "replacement"} {
		t.Run(change, func(t *testing.T) {
			h, tasks, store := conformanceFixture(t)
			changed := false
			h.Status = statusWriterFunc(func(ctx context.Context, task *rlarkv1alpha1.Task, intent StatusIntent) error {
				if !changed {
					changed = true
					var current rlarkv1alpha1.Task
					if err := tasks.Get(ctx, client.ObjectKeyFromObject(task), &current); err != nil {
						return err
					}
					switch change {
					case "spec":
						current.Generation++
						current.Spec.Suspended = true
						if err := tasks.Update(ctx, &current); err != nil {
							return err
						}
					case "status":
						current.Status.Conditions = []metav1.Condition{{Type: "OtherOwner", Status: metav1.ConditionTrue, Reason: "ConcurrentObservation", LastTransitionTime: metav1.Now()}}
						if err := tasks.Status().Update(ctx, &current); err != nil {
							return err
						}
					case "deletion":
						// Keep the deleting incarnation available to the Status API.
						current.Finalizers = []string{"other-owner/cleanup"}
						if err := tasks.Update(ctx, &current); err != nil {
							return err
						}
						if err := tasks.Delete(ctx, &current); err != nil {
							return err
						}
					case "replacement":
						if err := tasks.Delete(ctx, &current); err != nil {
							return err
						}
						replacement := hostTask()
						replacement.UID = "replacement-uid"
						if err := tasks.Create(ctx, replacement); err != nil {
							return err
						}
					}
				}
				return (KubernetesStatusWriter{Client: tasks}).WriteStatus(ctx, task, intent)
			})
			result, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask()))
			if !apierrors.IsConflict(err) || result.Retry.Class != RetryConflict {
				t.Fatalf("stale admission publication was accepted: %#v %v", result, err)
			}
			var current rlarkv1alpha1.Task
			if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &current); err != nil {
				t.Fatal(err)
			}
			if contains(current.Finalizers, TaskFinalizer) || apimeta.FindStatusCondition(current.Status.Conditions, ConditionExecutionAdmitted) != nil || len(store.applies)+len(store.deletes) != 0 {
				t.Fatal("stale decision published admission, claimed or executed")
			}
			if change == "status" {
				conformanceReconcile(t, h, OutcomeSucceeded)
				if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &current); err != nil {
					t.Fatal(err)
				}
				if apimeta.FindStatusCondition(current.Status.Conditions, "OtherOwner") == nil {
					t.Fatal("fresh retry erased the other Status owner")
				}
			}
		})
	}
}

type admissionClaimRaceClient struct {
	client.Client
	before bool
	mutate func() error
}

func (c *admissionClaimRaceClient) Patch(ctx context.Context, object client.Object, patch client.Patch, options ...client.PatchOption) error {
	mutate := c.mutate
	c.mutate = nil
	if mutate != nil && c.before {
		if err := mutate(); err != nil {
			return err
		}
	}
	if err := c.Client.Patch(ctx, object, patch, options...); err != nil {
		// The fake JSON patch engine returns a plain error on failed tests;
		// Kubernetes reports Invalid, which Host translates to a fresh-read conflict.
		if mutate != nil && c.before {
			return apierrors.NewInvalid(rlarkv1alpha1.GroupVersion.WithKind("Task").GroupKind(), object.GetName(), field.ErrorList{field.Invalid(field.NewPath("metadata", "resourceVersion"), object.GetResourceVersion(), err.Error())})
		}
		return err
	}
	if mutate != nil && !c.before {
		return mutate()
	}
	return nil
}

func TestAdmissionClaimRacesRecomputeBeforeExecution(t *testing.T) {
	for _, before := range []bool{true, false} {
		t.Run(map[bool]string{true: "before claim", false: "after claim"}[before], func(t *testing.T) {
			h, tasks, store := conformanceFixture(t)
			admissions := 0
			admit := h.Bundle.Admit
			h.Bundle.Admit = func(ctx context.Context, input LifecycleInput) error {
				admissions++
				return admit(ctx, input)
			}
			h.Tasks = &admissionClaimRaceClient{Client: tasks, before: before, mutate: func() error {
				var current rlarkv1alpha1.Task
				if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &current); err != nil {
					return err
				}
				current.Generation++
				return tasks.Update(context.Background(), &current)
			}}
			result, err := h.Reconcile(context.Background(), client.ObjectKeyFromObject(hostTask()))
			if !apierrors.IsConflict(err) || result.Retry.Class != RetryConflict || admissions != 1 || len(store.applies)+len(store.deletes) != 0 {
				t.Fatalf("claim race did not fence execution: %#v %v admissions=%d", result, err, admissions)
			}
			var current rlarkv1alpha1.Task
			if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &current); err != nil {
				t.Fatal(err)
			}
			if contains(current.Finalizers, TaskFinalizer) == before {
				t.Fatal("unexpected claim ownership after concurrent change")
			}
			conformanceReconcile(t, h, OutcomeSucceeded)
			if admissions != 2 || len(store.applies) != 2 || store.applies[0].Task.Generation != current.Generation {
				t.Fatal("recovery reused old admission instead of current generation")
			}
		})
	}
}

func TestStatusCASFailureDoesNotRefreshLocalSnapshot(t *testing.T) {
	_, tasks, _ := conformanceFixture(t)
	var stale rlarkv1alpha1.Task
	if err := tasks.Get(context.Background(), client.ObjectKeyFromObject(hostTask()), &stale); err != nil {
		t.Fatal(err)
	}
	version := stale.ResourceVersion
	conformanceUpdate(t, tasks, func(task *rlarkv1alpha1.Task) { task.Generation++ })
	intent := ConditionIntent(&stale, metav1.Condition{Type: ConditionExecutionAdmitted, Status: metav1.ConditionTrue, Reason: "ExecutionAdmissionSucceeded", ObservedGeneration: stale.Generation}, false, StatusPriorityInvalid)
	err := (KubernetesStatusWriter{Client: tasks}).WriteStatus(context.Background(), &stale, intent)
	if !apierrors.IsConflict(err) || stale.ResourceVersion != version || len(stale.Status.Conditions) != 0 {
		t.Fatalf("failed CAS changed local state: %v", err)
	}
}
