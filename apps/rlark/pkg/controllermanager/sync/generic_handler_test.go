package sync

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

func TestDeletionTimestampDoesNotSetDeletedAt(t *testing.T) {
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
	// 资源删除不意味着在 PG 中被删除，不设置 DeletedAt
	if model.GetBase().DeletedAt != nil {
		t.Fatal("deleted_at should not be set by syncer")
	}

	job.Finalizers = []string{SyncFinalizer}
	model, err = handler.ToPersistedLastestModelObject(job)
	if err != nil {
		t.Fatal(err)
	}
	// 即使只剩 sync finalizer，也不设置 DeletedAt
	if model.GetBase().DeletedAt != nil {
		t.Fatal("deleted_at should not be set by syncer")
	}
}
