package sync

import (
	"encoding/json"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

// Handler defines the interface for syncing resources to the database.
type Handler interface {
	GetTableName() string
	GetResourceType() string
	ShouldSyncObject(obj client.Object) bool
	ToPersistedModelObject(obj client.Object) (db.ResourceModel, error)
	ToPersistedLastestModelObject(obj client.Object) (db.ResourceModel, error)
}

type genericSyncHandler struct {
	tableName           string
	resourceType        string
	isNamespaced        bool
	wrapBaseModel       func(base db.BaseResourceModel) db.ResourceModel
	wrapLatestBaseModel func(base db.BaseResourceModel) db.ResourceModel
}

var _ Handler = (*genericSyncHandler)(nil)

// GetTableName returns the tableName.
func (h *genericSyncHandler) GetTableName() string {
	return h.tableName
}

// GetResourceType returns the resourceType.
func (h *genericSyncHandler) GetResourceType() string {
	return h.resourceType
}

// ShouldSyncObject is an exported method.
func (h *genericSyncHandler) ShouldSyncObject(obj client.Object) bool {
	return CheckSync(obj)
}

func (h *genericSyncHandler) buildHistoryID(obj client.Object) string {
	if h.isNamespaced {
		return obj.GetNamespace() + "/" + obj.GetName() + "/" + string(obj.GetUID())
	}
	return obj.GetName() + "/" + string(obj.GetUID())
}

func (h *genericSyncHandler) buildLatestID(obj client.Object) string {
	if h.isNamespaced {
		return obj.GetNamespace() + "/" + obj.GetName()
	}
	return obj.GetName()
}

// ToPersistedModelObject is an exported method.
func (h *genericSyncHandler) ToPersistedModelObject(obj client.Object) (db.ResourceModel, error) {
	rawData, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal object: %w", err)
	}

	m := db.BaseResourceModel{
		ID:        h.buildHistoryID(obj),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		UID:       string(obj.GetUID()),
		CreatedAt: obj.GetCreationTimestamp().Time,
		Raw:       rawData,
	}
	if obj.GetDeletionTimestamp() != nil {
		deletedAt := obj.GetDeletionTimestamp().Time
		m.DeletedAt = &deletedAt
	}
	return h.wrapBaseModel(m), nil
}

// ToPersistedLastestModelObject is an exported method.
func (h *genericSyncHandler) ToPersistedLastestModelObject(obj client.Object) (db.ResourceModel, error) {
	rawData, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("marshal object: %w", err)
	}

	m := db.BaseResourceModel{
		ID:        h.buildLatestID(obj),
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
		UID:       string(obj.GetUID()),
		CreatedAt: obj.GetCreationTimestamp().Time,
		Raw:       rawData,
	}
	if obj.GetDeletionTimestamp() != nil {
		deletedAt := obj.GetDeletionTimestamp().Time
		m.DeletedAt = &deletedAt
	}
	return h.wrapLatestBaseModel(m), nil
}
