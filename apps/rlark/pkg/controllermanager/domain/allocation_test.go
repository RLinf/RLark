package domain

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func ownedTask(namespace, name, job string) *rlarkv1alpha1.Task {
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
	if job != "" {
		controller := true
		task.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: rlarkv1alpha1.GroupVersion.String(),
			Kind:       "Job",
			Name:       job,
			UID:        "job-uid",
			Controller: &controller,
		}}
	}
	return task
}

func TestJobForReadsTheControllerOwnerOfTheTask(t *testing.T) {
	c := newTestClientBuilder(t).WithObjects(ownedTask("cluster-a", "job-worker", "job")).Build()
	resolver := newTaskJobResolver(c)

	job, err := resolver.jobFor(context.Background(), podTaskRef{namespace: "cluster-a", name: "job-worker"})
	if err != nil {
		t.Fatalf("jobFor() error = %v", err)
	}
	if job != "job" {
		t.Fatalf("job = %q, want %q", job, "job")
	}
}

func TestJobForYieldsNoJobWhenOwnershipIsNotProvable(t *testing.T) {
	ownedByOther := ownedTask("cluster-a", "foreign", "")
	ownedByOther.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1", Kind: "StatefulSet", Name: "set", UID: "set-uid",
	}}
	c := newTestClientBuilder(t).WithObjects(
		ownedTask("cluster-a", "standalone", ""),
		ownedByOther,
	).Build()
	resolver := newTaskJobResolver(c)

	for _, name := range []string{"standalone", "foreign", "gone"} {
		job, err := resolver.jobFor(context.Background(), podTaskRef{namespace: "cluster-a", name: name})
		if err != nil {
			t.Fatalf("jobFor(%s) error = %v", name, err)
		}
		if job != "" {
			t.Fatalf("jobFor(%s) = %q, want no job", name, job)
		}
	}
}

func TestJobForLooksUpEachTaskOnlyOnce(t *testing.T) {
	c := newTestClientBuilder(t).WithObjects(ownedTask("cluster-a", "job-worker", "job")).Build()
	resolver := newTaskJobResolver(c)
	ref := podTaskRef{namespace: "cluster-a", name: "job-worker"}

	if _, err := resolver.jobFor(context.Background(), ref); err != nil {
		t.Fatalf("jobFor() error = %v", err)
	}
	// A Job contributes many Pods, so the second answer must come from the
	// memo rather than from another read.
	if _, ok := resolver.jobs["cluster-a/job-worker"]; !ok {
		t.Fatal("resolved Task was not memoised")
	}
	if len(resolver.jobs) != 1 {
		t.Fatalf("memo holds %d entries, want 1", len(resolver.jobs))
	}
}

func TestResolveAllocationOwnersFillsJobAndTaskForReportedPods(t *testing.T) {
	c := newTestClientBuilder(t).WithObjects(ownedTask("cluster-a", "job-worker", "job")).Build()
	allocations := []rlarkv1alpha1.DomainIPAllocation{
		{IP: "10.0.0.1", Pod: "cluster-a/default/pod-0"},
		{IP: "10.0.0.2", Pod: "cluster-a/default/pod-1"},
	}
	refs := map[string]podTaskRef{
		"cluster-a/default/pod-0": {namespace: "cluster-a", name: "job-worker"},
		"cluster-a/default/pod-1": {namespace: "cluster-a", name: "job-worker"},
	}

	if err := resolveAllocationOwners(context.Background(), newTaskJobResolver(c), allocations, refs); err != nil {
		t.Fatalf("resolveAllocationOwners() error = %v", err)
	}
	for _, alloc := range allocations {
		if alloc.Job != "job" {
			t.Errorf("allocation %s job = %q, want %q", alloc.Pod, alloc.Job, "job")
		}
		if alloc.Task != "cluster-a/job-worker" {
			t.Errorf("allocation %s task = %q, want %q", alloc.Pod, alloc.Task, "cluster-a/job-worker")
		}
	}
}

func TestResolveAllocationOwnersBackfillsAllocationsWrittenWithoutThem(t *testing.T) {
	// An allocation retained across passes is never rewritten, so an entry
	// written by an older control plane keeps its blank Job and Task unless
	// this pass fills them in.
	c := newTestClientBuilder(t).WithObjects(ownedTask("cluster-a", "job-worker", "job")).Build()
	allocations := []rlarkv1alpha1.DomainIPAllocation{{IP: "10.0.0.1", Pod: "cluster-a/default/pod-0"}}
	refs := map[string]podTaskRef{
		"cluster-a/default/pod-0": {namespace: "cluster-a", name: "job-worker"},
	}

	if err := resolveAllocationOwners(context.Background(), newTaskJobResolver(c), allocations, refs); err != nil {
		t.Fatalf("resolveAllocationOwners() error = %v", err)
	}
	if allocations[0].Task != "cluster-a/job-worker" || allocations[0].Job != "job" {
		t.Fatalf("allocation = %+v, want the Job and Task backfilled", allocations[0])
	}
}

func TestResolveAllocationOwnersKeepsWhatItCannotReResolve(t *testing.T) {
	c := newTestClientBuilder(t).Build()
	allocations := []rlarkv1alpha1.DomainIPAllocation{
		// The Pod is no longer reported: the only evidence of its Task went
		// with it, so the recorded dimensions must survive untouched.
		{IP: "10.0.0.1", Pod: "cluster-a/default/pod-0", Job: "job", Task: "cluster-a/job-worker"},
		// Reported, but the Pod never carried Task coordinates.
		{IP: "10.0.0.2", Pod: "cluster-a/default/pod-1"},
	}
	refs := map[string]podTaskRef{"cluster-a/default/pod-1": {}}

	if err := resolveAllocationOwners(context.Background(), newTaskJobResolver(c), allocations, refs); err != nil {
		t.Fatalf("resolveAllocationOwners() error = %v", err)
	}
	if allocations[0].Job != "job" || allocations[0].Task != "cluster-a/job-worker" {
		t.Errorf("unreported allocation = %+v, want it preserved", allocations[0])
	}
	if allocations[1].Job != "" || allocations[1].Task != "" {
		t.Errorf("allocation without Task coordinates = %+v, want both blank", allocations[1])
	}
}
