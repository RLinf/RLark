// Package persistence 提供统一的资源持久化存储抽象。
//
// Syncer（保存资源状态）和 Gateway（查询/恢复/删除）共用同一个 Backend 接口，
// 支持 PostgreSQL（生产）和 Secret（测试）两种实现。
package persistence

import (
	"context"
	"encoding/json"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

// Resource 是从持久化存储读取的资源记录。
type Resource struct {
	// Raw 是完整的 CR JSON（spec + status + metadata）。
	Raw json.RawMessage
}

// ListResult 是列表查询结果。
type ListResult struct {
	Items []map[string]any `json:"items"`
	Total int              `json:"total"`
}

// Backend 定义持久化存储接口。
// Syncer 用 Save（保存状态），Gateway 用 Load/List/Delete（查询/恢复/删除）。
type Backend interface {
	// Save 保存资源状态（由 Syncer 的 Handler 生成 ResourceModel）。
	Save(ctx context.Context, m db.ResourceModel) error

	// Load 读取单个资源。
	// namespace 用于 namespaced 资源（tasks/nodes），cluster-scoped 资源（jobs/workflows）传空字符串。
	Load(ctx context.Context, kind, namespace, name string) (*Resource, error)

	// List 读取资源列表。
	List(ctx context.Context, kind string, opts db.ListOptions) (*ListResult, error)

	// Delete 删除资源记录。
	Delete(ctx context.Context, kind, namespace, name string) error
}

// getResourceKind 从 ResourceModel 判断资源类型（复数形式，和路由一致）。
func getResourceKind(m db.ResourceModel) string {
	switch m.(type) {
	case *db.LatestJobModel:
		return "jobs"
	case *db.LatestTaskModel:
		return "tasks"
	case *db.LatestWorkflowModel:
		return "workflows"
	case *db.LatestNodeModel:
		return "nodes"
	default:
		return ""
	}
}
