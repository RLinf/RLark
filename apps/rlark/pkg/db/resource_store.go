package db

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ListOptions controls list query behavior including filtering, sorting, and pagination.
type ListOptions struct {
	// Namespace filters by namespace (empty means all).
	Namespace string
	// FieldSelector filters by raw JSONB fields, e.g. "status.phase=Running".
	FieldSelector []FieldSelector
	// LabelSelector filters by raw JSONB labels, e.g. "tenant=acme".
	LabelSelector []LabelSelector
	// OrderBy specifies sorting, e.g. "created_at", "name", "created_at desc".
	OrderBy []string
	// Limit limits the number of results (0 means no limit).
	Limit int
	// Offset skips the first N results.
	Offset int
}

// FieldSelector represents a JSONB field filter.
type FieldSelector struct {
	Path  string // e.g. "status.phase"
	Op    string // "=", "!=", "in" (default: "=")
	Value string
}

// LabelSelector represents a label filter.
type LabelSelector struct {
	Key   string
	Op    string // "=", "!=", "in" (default: "=")
	Value string
}

// ListResult holds the result of a list query.
type ListResult struct {
	Items []map[string]any `json:"items"`
	Total int              `json:"total"`
}

// ResourceStore provides generic CRUD operations for a resource table.
type ResourceStore struct {
	db            *bun.DB
	tableName     string
	tableAlias    string
	newModel      func() ResourceModel
	newModelSlice func() any // returns a pointer to an empty slice of the concrete model type, e.g. func() any { return &[]NodeModel{} }
}

// NewNodeStore creates a ResourceStore for latest nodes.
func NewNodeStore(db *bun.DB) *ResourceStore {
	return &ResourceStore{
		db:            db,
		tableName:     "latest_nodes",
		tableAlias:    "ln",
		newModel:      func() ResourceModel { return &LatestNodeModel{} },
		newModelSlice: func() any { return &[]LatestNodeModel{} },
	}
}

// NewWorkflowStore creates a ResourceStore for latest workflows.
func NewWorkflowStore(db *bun.DB) *ResourceStore {
	return &ResourceStore{
		db:            db,
		tableName:     "latest_workflows",
		tableAlias:    "lw",
		newModel:      func() ResourceModel { return &LatestWorkflowModel{} },
		newModelSlice: func() any { return &[]LatestWorkflowModel{} },
	}
}

// NewJobStore creates a ResourceStore for latest jobs.
func NewJobStore(db *bun.DB) *ResourceStore {
	return &ResourceStore{
		db:            db,
		tableName:     "latest_jobs",
		tableAlias:    "lj",
		newModel:      func() ResourceModel { return &LatestJobModel{} },
		newModelSlice: func() any { return &[]LatestJobModel{} },
	}
}

// NewTaskStore creates a ResourceStore for latest tasks.
func NewTaskStore(db *bun.DB) *ResourceStore {
	return &ResourceStore{
		db:            db,
		tableName:     "latest_tasks",
		tableAlias:    "lt",
		newModel:      func() ResourceModel { return &LatestTaskModel{} },
		newModelSlice: func() any { return &[]LatestTaskModel{} },
	}
}

// List queries resources with filtering, sorting, and pagination.
func (q *ResourceStore) List(ctx context.Context, opts ListOptions) (*ListResult, error) {
	baseQuery := q.db.NewSelect().
		Model(q.newModel()).
		Where("deleted_at IS NULL")

	// Namespace filter
	if opts.Namespace != "" {
		baseQuery = baseQuery.Where("namespace = ?", opts.Namespace)
	}

	// Field selectors (JSONB expressions)
	for _, fs := range opts.FieldSelector {
		colExpr := fmt.Sprintf("%s.raw#>>'{%s}'", q.tableAlias, strings.ReplaceAll(fs.Path, ".", ","))
		baseQuery = applySelector(baseQuery, colExpr, fs.Op, fs.Value)
	}

	// Label selectors
	for _, ls := range opts.LabelSelector {
		colExpr := fmt.Sprintf("%s.raw#>>'{metadata,labels,%s}'", q.tableAlias, ls.Key)
		baseQuery = applySelector(baseQuery, colExpr, ls.Op, ls.Value)
	}

	// Count total before pagination
	total, err := baseQuery.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count %s: %w", q.tableName, err)
	}

	// Build the data query with ordering and pagination
	query := baseQuery.OrderExpr(q.tableAlias + ".created_at DESC")
	if len(opts.OrderBy) > 0 {
		query = baseQuery // reset ordering
		for _, ob := range opts.OrderBy {
			parts := strings.SplitN(strings.TrimSpace(ob), " ", 2)
			col := parts[0]
			dir := "ASC"
			if len(parts) == 2 && strings.EqualFold(parts[1], "desc") {
				dir = "DESC"
			}
			switch col {
			case "name", "namespace", "created_at", "uid":
				query = query.OrderExpr(fmt.Sprintf("%s.%s %s", q.tableAlias, col, dir))
			default:
				query = query.OrderExpr(fmt.Sprintf("%s.raw#>>'{%s}' %s", q.tableAlias, strings.ReplaceAll(col, ".", ","), dir))
			}
		}
	}

	// Pagination
	if opts.Limit > 0 {
		query = query.Limit(opts.Limit)
	}
	if opts.Offset > 0 {
		query = query.Offset(opts.Offset)
	}

	// Execute the query and scan results using the concrete model slice
	slicePtr := q.newModelSlice()
	if err := query.Scan(ctx, slicePtr); err != nil {
		return nil, fmt.Errorf("list %s: %w", q.tableName, err)
	}

	// Extract Raw from each model via ResourceModel interface
	var items []map[string]any
	items = scanResultsToItems(slicePtr)

	if items == nil {
		items = []map[string]any{}
	}

	return &ListResult{
		Items: items,
		Total: total,
	}, nil
}

// Get returns a single resource by name (and namespace if applicable).
func (q *ResourceStore) Get(ctx context.Context, namespace, name string) (map[string]any, error) {
	m := q.newModel()
	query := q.db.NewSelect().
		Model(m).
		Where("name = ?", name).
		Where("deleted_at IS NULL")

	if namespace != "" {
		query = query.Where("namespace = ?", namespace)
	}

	if err := query.Scan(ctx); err != nil {
		return nil, fmt.Errorf("get %s/%s from %s: %w", namespace, name, q.tableName, err)
	}
	var result map[string]any
	if err := json.Unmarshal(m.GetBase().Raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal raw %s/%s from %s: %w", namespace, name, q.tableName, err)
	}
	return result, nil
}

// Create inserts a new resource.
func (q *ResourceStore) Create(ctx context.Context, data map[string]any) error {
	m := q.newModel()
	m.FillFromRaw(data)

	_, err := q.db.NewInsert().Model(m).Exec(ctx)
	if err != nil {
		return fmt.Errorf("create in %s: %w", q.tableName, err)
	}
	return nil
}

// Update replaces an existing resource by name.
func (q *ResourceStore) Update(ctx context.Context, namespace, name string, data map[string]any) error {
	m := q.newModel()
	m.FillFromRaw(data)
	base := m.GetBase()
	base.Name = name
	base.Namespace = namespace

	_, err := q.db.NewUpdate().
		Model(m).
		Column("namespace", "name", "raw").
		Where("name = ?", name).
		Where("namespace = ?", namespace).
		Where("deleted_at IS NULL").
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("update %s/%s in %s: %w", namespace, name, q.tableName, err)
	}
	return nil
}

// Patch merges the provided data into an existing resource.
func (q *ResourceStore) Patch(ctx context.Context, namespace, name string, patch map[string]any) error {
	current, err := q.Get(ctx, namespace, name)
	if err != nil {
		return err
	}

	merged := deepMerge(current, patch)
	return q.Update(ctx, namespace, name, merged)
}

// Delete soft-deletes a resource by name.
func (q *ResourceStore) Delete(ctx context.Context, namespace, name string) error {
	now := time.Now()
	query := q.db.NewUpdate().
		TableExpr(q.tableName).
		Set("deleted_at = ?", now).
		Where("name = ?", name).
		Where("deleted_at IS NULL")

	if namespace != "" {
		query = query.Where("namespace = ?", namespace)
	}

	_, err := query.Exec(ctx)
	if err != nil {
		return fmt.Errorf("delete %s/%s from %s: %w", namespace, name, q.tableName, err)
	}
	return nil
}

// fillBaseFromRaw extracts metadata from the raw JSON and fills a BaseResourceModel.
func fillBaseFromRaw(base *BaseResourceModel, raw map[string]any) {
	rawBytes, err := json.Marshal(raw)
	if err != nil {
		return
	}
	base.Raw = rawBytes
	base.CreatedAt = time.Now()

	if meta, ok := raw["metadata"].(map[string]any); ok {
		if n, ok := meta["name"].(string); ok {
			base.Name = n
		}
		if ns, ok := meta["namespace"].(string); ok {
			base.Namespace = ns
		}
		if uid, ok := meta["uid"].(string); ok {
			base.UID = uid
		}
	}

	// Generate UID if not provided
	if base.UID == "" {
		base.UID = uuid.New().String()
	}

	// Build ID from namespace + name (latest table pattern)
	if base.Namespace != "" {
		base.ID = base.Namespace + "/" + base.Name
	} else {
		base.ID = base.Name
	}
}

// applySelector applies a selector operator to a query.
func applySelector(query *bun.SelectQuery, colExpr, op, value string) *bun.SelectQuery {
	switch op {
	case "!=":
		return query.Where(colExpr+" != ?", value)
	case "in":
		return query.Where(colExpr+" = ANY(?)", strings.Split(value, ","))
	default:
		return query.Where(colExpr+" = ?", value)
	}
}

// scanResultsToItems extracts Raw fields from a pointer to a slice of ResourceModel types.
// slicePtr is expected to be *[]T where T implements ResourceModel (via pointer receiver).
func scanResultsToItems(slicePtr any) []map[string]any {
	v := reflect.ValueOf(slicePtr)
	if v.Kind() != reflect.Pointer || v.Elem().Kind() != reflect.Slice {
		return nil
	}
	slice := v.Elem()
	items := make([]map[string]any, slice.Len())
	for i := 0; i < slice.Len(); i++ {
		elem := slice.Index(i)
		// Take address since ResourceModel methods use pointer receivers
		rm := elem.Addr().Interface().(ResourceModel)
		raw := rm.GetBase().Raw
		if err := json.Unmarshal(raw, &items[i]); err != nil {
			items[i] = nil
		}
	}
	return items
}

// deepMerge recursively merges patch into base. Map values are merged, scalar values are overwritten.
func deepMerge(base, patch map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range patch {
		if v == nil {
			delete(result, k)
			continue
		}
		baseVal, baseIsMap := base[k].(map[string]any)
		patchVal, patchIsMap := v.(map[string]any)
		if baseIsMap && patchIsMap {
			result[k] = deepMerge(baseVal, patchVal)
		} else {
			result[k] = v
		}
	}
	return result
}
