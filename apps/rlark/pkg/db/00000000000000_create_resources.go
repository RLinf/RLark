package db

import (
	"context"
	"fmt"

	"github.com/uptrace/bun"
)

func init() {
	Migrations.MustRegister(func(ctx context.Context, db *bun.DB) error {
		// Create tables for each resource type
		models := []any{
			(*RevokedCertificateModel)(nil),
			(*SSHUserKeyModel)(nil),
			(*JobModel)(nil),
			(*LatestJobModel)(nil),
			(*NodeModel)(nil),
			(*LatestNodeModel)(nil),
			(*TaskModel)(nil),
			(*LatestTaskModel)(nil),
			(*WorkflowModel)(nil),
			(*LatestWorkflowModel)(nil),
		}
		for _, m := range models {
			if _, err := db.NewCreateTable().Model(m).IfNotExists().Exec(ctx); err != nil {
				return fmt.Errorf("create table: %w", err)
			}
		}

		// Indexes: cluster-scoped resources use (name), namespace-scoped use (namespace, name)
		commonIndexes := map[string][]string{
			"ssh_user_keys": {
				"CREATE INDEX IF NOT EXISTS idx_ssh_user_keys_user_added_at ON ssh_user_keys (\"user\", added_at)",
				"CREATE INDEX IF NOT EXISTS idx_ssh_user_keys_added_at ON ssh_user_keys (added_at)",
			},
			"jobs": {
				"CREATE INDEX IF NOT EXISTS idx_jobs_name ON jobs (name)",
				"CREATE INDEX IF NOT EXISTS idx_jobs_deleted_at ON jobs (deleted_at)",
				"CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs (created_at)",
				"CREATE INDEX IF NOT EXISTS idx_jobs_raw_phase ON jobs ((raw->>'status.phase'))",
				"CREATE INDEX IF NOT EXISTS idx_jobs_raw_labels_tenant ON jobs ((raw->'metadata.labels'->>'tenant'))",
			},
			"latest_jobs": {
				"CREATE INDEX IF NOT EXISTS idx_latest_jobs_name ON latest_jobs (name)",
				"CREATE INDEX IF NOT EXISTS idx_latest_jobs_deleted_at ON latest_jobs (deleted_at)",
				"CREATE INDEX IF NOT EXISTS idx_latest_jobs_created_at ON latest_jobs (created_at)",
				"CREATE INDEX IF NOT EXISTS idx_latest_jobs_raw_phase ON latest_jobs ((raw->>'status.phase'))",
				"CREATE INDEX IF NOT EXISTS idx_latest_jobs_raw_labels_tenant ON latest_jobs ((raw->'metadata.labels'->>'tenant'))",
			},
			"nodes": {
				"CREATE INDEX IF NOT EXISTS idx_nodes_namespace_name ON nodes (namespace, name)",
				"CREATE INDEX IF NOT EXISTS idx_nodes_deleted_at ON nodes (deleted_at)",
				"CREATE INDEX IF NOT EXISTS idx_nodes_created_at ON nodes (created_at)",
				"CREATE INDEX IF NOT EXISTS idx_nodes_raw_phase ON nodes ((raw->>'status.phase'))",
				"CREATE INDEX IF NOT EXISTS idx_nodes_raw_agent_type ON nodes ((raw->>'spec.agentType'))",
			},
			"latest_nodes": {
				"CREATE INDEX IF NOT EXISTS idx_latest_nodes_namespace_name ON latest_nodes (namespace, name)",
				"CREATE INDEX IF NOT EXISTS idx_latest_nodes_deleted_at ON latest_nodes (deleted_at)",
				"CREATE INDEX IF NOT EXISTS idx_latest_nodes_created_at ON latest_nodes (created_at)",
				"CREATE INDEX IF NOT EXISTS idx_latest_nodes_raw_phase ON latest_nodes ((raw->>'status.phase'))",
				"CREATE INDEX IF NOT EXISTS idx_latest_nodes_raw_agent_type ON latest_nodes ((raw->>'spec.agentType'))",
			},
			"tasks": {
				"CREATE INDEX IF NOT EXISTS idx_tasks_namespace_name ON tasks (namespace, name)",
				"CREATE INDEX IF NOT EXISTS idx_tasks_deleted_at ON tasks (deleted_at)",
				"CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks (created_at)",
				"CREATE INDEX IF NOT EXISTS idx_tasks_raw_phase ON tasks ((raw->>'status.phase'))",
				"CREATE INDEX IF NOT EXISTS idx_tasks_raw_agent_type ON tasks ((raw->>'spec.agentType'))",
				"CREATE INDEX IF NOT EXISTS idx_tasks_raw_role ON tasks ((raw->>'spec.role'))",
			},
			"latest_tasks": {
				"CREATE INDEX IF NOT EXISTS idx_latest_tasks_namespace_name ON latest_tasks (namespace, name)",
				"CREATE INDEX IF NOT EXISTS idx_latest_tasks_deleted_at ON latest_tasks (deleted_at)",
				"CREATE INDEX IF NOT EXISTS idx_latest_tasks_created_at ON latest_tasks (created_at)",
				"CREATE INDEX IF NOT EXISTS idx_latest_tasks_raw_phase ON latest_tasks ((raw->>'status.phase'))",
				"CREATE INDEX IF NOT EXISTS idx_latest_tasks_raw_agent_type ON latest_tasks ((raw->>'spec.agentType'))",
				"CREATE INDEX IF NOT EXISTS idx_latest_tasks_raw_role ON latest_tasks ((raw->>'spec.role'))",
			},
			"workflows": {
				"CREATE INDEX IF NOT EXISTS idx_workflows_name ON workflows (name)",
				"CREATE INDEX IF NOT EXISTS idx_workflows_deleted_at ON workflows (deleted_at)",
				"CREATE INDEX IF NOT EXISTS idx_workflows_created_at ON workflows (created_at)",
				"CREATE INDEX IF NOT EXISTS idx_workflows_raw_phase ON workflows ((raw->>'status.phase'))",
				"CREATE INDEX IF NOT EXISTS idx_workflows_raw_labels_tenant ON workflows ((raw->'metadata.labels'->>'tenant'))",
			},
			"latest_workflows": {
				"CREATE INDEX IF NOT EXISTS idx_latest_workflows_name ON latest_workflows (name)",
				"CREATE INDEX IF NOT EXISTS idx_latest_workflows_deleted_at ON latest_workflows (deleted_at)",
				"CREATE INDEX IF NOT EXISTS idx_latest_workflows_created_at ON latest_workflows (created_at)",
				"CREATE INDEX IF NOT EXISTS idx_latest_workflows_raw_phase ON latest_workflows ((raw->>'status.phase'))",
				"CREATE INDEX IF NOT EXISTS idx_latest_workflows_raw_labels_tenant ON latest_workflows ((raw->'metadata.labels'->>'tenant'))",
			},
		}

		for table, indexes := range commonIndexes {
			for _, idx := range indexes {
				if _, err := db.ExecContext(ctx, idx); err != nil {
					return fmt.Errorf("create index on %s: %w", table, err)
				}
			}
		}

		return nil
	}, func(ctx context.Context, db *bun.DB) error {
		tables := []string{"latest_workflows", "latest_tasks", "latest_nodes", "latest_jobs", "workflows", "tasks", "nodes", "jobs"}
		for _, t := range tables {
			if _, err := db.ExecContext(ctx, "DROP TABLE IF EXISTS "+t+" CASCADE"); err != nil {
				return fmt.Errorf("drop table %s: %w", t, err)
			}
		}
		return nil
	})
}
