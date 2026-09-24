package sync

import (
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestDeletionTimestampOnlySetsDeletedAtWhenPersistFinalizerIsLast(t *testing.T) {
	deletedAt := metav1.NewTime(time.Now())
	job := &rlarkv1alpha1.Job{ObjectMeta: metav1.ObjectMeta{
		Name:              "job",
		DeletionTimestamp: &deletedAt,
		Finalizers:        []string{"jobs.rlinf.io/task-cleanup", SyncFinalizer},
	}}
	handler := newJobSyncHandler()

	model, err := handler.ToPersistedLastestModelObject(job)
	if err != nil {
		t.Fatal(err)
	}
	if model.GetBase().DeletedAt != nil {
		t.Fatal("resource should remain visible while cleanup finalizers are present")
	}
	var raw map[string]any
	if err := json.Unmarshal(model.GetBase().Raw, &raw); err != nil {
		t.Fatal(err)
	}
	metadata := raw["metadata"].(map[string]any)
	if metadata["deletionTimestamp"] == nil {
		t.Fatal("raw resource must expose deletionTimestamp during cleanup")
	}

	job.Finalizers = []string{SyncFinalizer}
	model, err = handler.ToPersistedLastestModelObject(job)
	if err != nil {
		t.Fatal(err)
	}
	if model.GetBase().DeletedAt == nil || !model.GetBase().DeletedAt.Equal(deletedAt.Time) {
		t.Fatalf("deleted_at was not set at final deletion: %v", model.GetBase().DeletedAt)
	}
}
