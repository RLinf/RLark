package db

import (
	"encoding/json"
	"time"

	"github.com/uptrace/bun"
)

// ResourceModel is the interface that all resource models must implement.
type ResourceModel interface {
	// GetBase returns the underlying BaseResourceModel.
	GetBase() *BaseResourceModel
	// FillFromRaw extracts metadata from raw JSON and fills the model's base fields.
	FillFromRaw(data map[string]any)
}

// BaseResourceModel contains the common fields for all resource tables.
// Each resource type embeds this and overrides TableName().
type BaseResourceModel struct {
	// Primary key
	ID string `bun:"id,pk,notnull"`

	// Kubernetes metadata
	Namespace string `bun:"namespace"`
	Name      string `bun:"name,notnull"`
	UID       string `bun:"uid,notnull"`

	// Timestamps
	CreatedAt time.Time  `bun:"created_at,notnull"`
	DeletedAt *time.Time `bun:"deleted_at"`

	// Raw resource data (spec + status + metadata) as JSON
	Raw json.RawMessage `bun:"raw,type:jsonb,notnull"`
}

// GetBase returns the base.
func (b *BaseResourceModel) GetBase() *BaseResourceModel {
	return b
}

// FillFromRaw fills the fromRaw.
func (b *BaseResourceModel) FillFromRaw(data map[string]any) {
	fillBaseFromRaw(b, data)
}

// ----- Resource-specific Models (history tables) -----

// JobModel is the database model for Job resources.
type JobModel struct {
	bun.BaseModel `bun:"table:jobs,alias:j"`

	BaseResourceModel
}

// NodeModel is the database model for Node resources.
type NodeModel struct {
	bun.BaseModel `bun:"table:nodes,alias:n"`

	BaseResourceModel
}

// TaskModel is the database model for Task resources.
type TaskModel struct {
	bun.BaseModel `bun:"table:tasks,alias:t"`

	BaseResourceModel
}

// WorkflowModel is the database model for Workflow resources.
type WorkflowModel struct {
	bun.BaseModel `bun:"table:workflows,alias:w"`

	BaseResourceModel
}

// ----- Latest Resource Models -----

// LatestJobModel is the database model for the latest Job resources.
type LatestJobModel struct {
	bun.BaseModel `bun:"table:latest_jobs,alias:lj"`

	BaseResourceModel
}

// LatestNodeModel is the database model for the latest Node resources.
type LatestNodeModel struct {
	bun.BaseModel `bun:"table:latest_nodes,alias:ln"`

	BaseResourceModel
}

// LatestTaskModel is the database model for the latest Task resources.
type LatestTaskModel struct {
	bun.BaseModel `bun:"table:latest_tasks,alias:lt"`

	BaseResourceModel
}

// LatestWorkflowModel is the database model for the latest Workflow resources.
type LatestWorkflowModel struct {
	bun.BaseModel `bun:"table:latest_workflows,alias:lw"`

	BaseResourceModel
}
