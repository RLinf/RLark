package job

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	sharedreconcile "github.com/rlinf/rlark/apps/rlark/pkg/reconcile"
)

func digestTestJob() *rlarkv1alpha1.Job {
	return &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: "job", UID: "job-uid"},
		Spec: rlarkv1alpha1.JobSpec{
			Domain: "training-network",
			Tasks:  []rlarkv1alpha1.JobTaskTemplate{{Name: "worker"}},
		},
	}
}

func TestDesiredTaskStampsCanonicalDigest(t *testing.T) {
	job := digestTestJob()
	task := mustDesiredTask(t, job, job.Spec.Tasks[0])

	digest := sharedreconcile.Digest(task.Annotations[DesiredDigestAnnotation])
	if err := digest.Validate(); err != nil {
		t.Fatalf("desired digest %q is not canonical: %v", digest, err)
	}

	again := mustDesiredTask(t, job, job.Spec.Tasks[0])
	if again.Annotations[DesiredDigestAnnotation] != string(digest) {
		t.Fatal("the same desired state must produce the same digest on every pass")
	}
}

func TestTaskUpToDateIgnoresWritesThisControllerDidNotMake(t *testing.T) {
	job := digestTestJob()
	live := mustDesiredTask(t, job, job.Spec.Tasks[0])

	// Stand in for API-server defaulting and fields other writers own: the
	// live Task no longer matches the compiled spec structurally, but this
	// controller's decision has not changed.
	live.Spec.Environments = append(live.Spec.Environments, rlarkv1alpha1.TaskEnvironment{
		Type: "defaulted-by-someone-else", Version: "v1",
	})

	if !taskUpToDate(live, mustDesiredTask(t, job, job.Spec.Tasks[0])) {
		t.Fatal("a foreign write must not be reported as a required rewrite")
	}
}

func TestTaskUpToDateDetectsDesiredStateChange(t *testing.T) {
	job := digestTestJob()
	live := mustDesiredTask(t, job, job.Spec.Tasks[0])

	job.Spec.Domain = "other-network"
	if taskUpToDate(live, mustDesiredTask(t, job, job.Spec.Tasks[0])) {
		t.Fatal("a changed Job spec must require a Task rewrite")
	}
}

func TestTaskUpToDateFallsBackToStructuralComparisonForLegacyTasks(t *testing.T) {
	job := digestTestJob()
	legacy := mustDesiredTask(t, job, job.Spec.Tasks[0])
	delete(legacy.Annotations, DesiredDigestAnnotation)

	if !taskUpToDate(legacy, mustDesiredTask(t, job, job.Spec.Tasks[0])) {
		t.Fatal("an undigested Task that already matches must not be rewritten on upgrade")
	}

	job.Spec.Domain = "other-network"
	if taskUpToDate(legacy, mustDesiredTask(t, job, job.Spec.Tasks[0])) {
		t.Fatal("an undigested Task with a stale spec must still be rewritten")
	}
}
