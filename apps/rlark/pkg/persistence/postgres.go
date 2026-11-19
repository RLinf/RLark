package persistence

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/uptrace/bun"

	"github.com/rlinf/rlark/apps/rlark/pkg/db"
)

// PostgresBackend 使用 PostgreSQL 作为持久化存储（生产模式）。
type PostgresBackend struct {
	db *bun.DB
	// stores 缓存各资源的 ResourceStore，避免重复创建。
	stores map[string]*db.ResourceStore
}

// NewPostgresBackend 创建一个 PostgreSQL Backend。
func NewPostgresBackend(dbConn *bun.DB) *PostgresBackend {
	return &PostgresBackend{
		db: dbConn,
		stores: map[string]*db.ResourceStore{
			"jobs":      db.NewJobStore(dbConn),
			"tasks":     db.NewTaskStore(dbConn),
			"workflows": db.NewWorkflowStore(dbConn),
			"nodes":     db.NewNodeStore(dbConn),
		},
	}
}

// Save 保存资源状态（和之前 Syncer 的逻辑一样，upsert 到对应表）。
func (b *PostgresBackend) Save(ctx context.Context, m db.ResourceModel) error {
	_, err := b.db.NewInsert().
		Model(m).
		On("CONFLICT (id) DO UPDATE").
		Set("namespace = EXCLUDED.namespace").
		Set("name = EXCLUDED.name").
		Set("uid = EXCLUDED.uid").
		Set("raw = EXCLUDED.raw").
		Exec(ctx)
	return err
}

// Load 读取单个资源。
func (b *PostgresBackend) Load(ctx context.Context, kind, namespace, name string) (*Resource, error) {
	store, ok := b.getResourceStore(kind)
	if !ok {
		return nil, fmt.Errorf("unsupported resource kind: %s", kind)
	}

	item, err := store.Get(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	raw, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}

	return &Resource{Raw: raw}, nil
}

// List 读取资源列表。
// 通过 ResourceStore 封装查询逻辑，支持分页、过滤、排序。
func (b *PostgresBackend) List(ctx context.Context, kind string, opts db.ListOptions) (*ListResult, error) {
	store, ok := b.getResourceStore(kind)
	if !ok {
		return nil, fmt.Errorf("unsupported resource kind: %s", kind)
	}

	result, err := store.List(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &ListResult{
		Items: result.Items,
		Total: result.Total,
	}, nil
}

// getResourceStore 根据 kind 返回对应的 ResourceStore（从缓存取）。
func (b *PostgresBackend) getResourceStore(kind string) (*db.ResourceStore, bool) {
	store, ok := b.stores[kind]
	return store, ok
}

// Delete 软删除资源记录（标记 deleted_at）。
func (b *PostgresBackend) Delete(ctx context.Context, kind, namespace, name string) error {
	store, ok := b.getResourceStore(kind)
	if !ok {
		return fmt.Errorf("unsupported resource kind: %s", kind)
	}

	return store.Delete(ctx, namespace, name)
}
