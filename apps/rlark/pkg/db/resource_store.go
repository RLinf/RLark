package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"

	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// ErrNotFound reports that the requested resource does not exist. Callers must
// detect it with errors.Is instead of matching driver error text.
var ErrNotFound = errors.New("resource not found")

// ListOptions controls list query behavior including filtering, sorting, and pagination.
type ListOptions struct {
	// Namespace filters by namespace (empty means all).
	Namespace string
	// FieldSelector filters by raw JSONB fields, e.g. "status.phase=Running".
	FieldSelector []FieldSelector
	// LabelSelector filters by raw JSONB labels, e.g. "tenant=acme".
	LabelSelector []LabelSelector
	// TagSelector filters Job spec.tags by key/value pairs.
	TagSelector []TagSelector
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

// TagSelector represents a Job spec.tags key/value filter.
type TagSelector struct {
	Key   string
	Value string
}

// ListResult holds the result of a list query. Limit, Offset and HasMore echo
// the applied pagination window so that the archive path reports the same
// envelope as the Kubernetes path and a client does not have to reconstruct it.
type ListResult struct {
	Items   []map[string]any `json:"items"`
	Total   int              `json:"total"`
	Limit   int              `json:"limit"`
	Offset  int              `json:"offset"`
	HasMore bool             `json:"hasMore"`
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
		colExpr, colArgs := jsonPathExpr(q.tableAlias, strings.Split(fs.Path, "."))
		baseQuery = applySelector(baseQuery, colExpr, colArgs, fs.Op, fs.Value)
	}

	// Label selectors
	for _, ls := range opts.LabelSelector {
		colExpr, colArgs := labelColumnExpr(q.tableAlias, ls.Key)
		baseQuery = applySelector(baseQuery, colExpr, colArgs, ls.Op, ls.Value)
	}

	// Tag selectors: values of the same key are ORed; different keys are ANDed.
	baseQuery = applyTagSelectors(baseQuery, opts.TagSelector)

	// Count total before pagination
	total, err := baseQuery.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("count %s: %w", q.tableName, err)
	}

	// Ordering must be deterministic, otherwise paginating over equal sort keys
	// can drop or duplicate rows between requests. A "name" tie breaker is
	// always appended, including for caller supplied ordering.
	query := baseQuery
	for _, ob := range opts.OrderBy {
		parts := strings.SplitN(strings.TrimSpace(ob), " ", 2)
		col := parts[0]
		dir := "ASC"
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[1]), "desc") {
			dir = "DESC"
		}
		switch col {
		case "name", "namespace", "created_at", "uid":
			query = query.OrderExpr(fmt.Sprintf("%s.%s %s", q.tableAlias, col, dir))
		default:
			colExpr, colArgs := jsonPathExpr(q.tableAlias, strings.Split(col, "."))
			query = query.OrderExpr(colExpr+" "+dir, colArgs...)
		}
	}
	if len(opts.OrderBy) == 0 {
		query = query.OrderExpr(q.tableAlias + ".created_at DESC")
	}
	query = query.OrderExpr(q.tableAlias + ".name ASC")

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
		Items:   items,
		Total:   total,
		Limit:   opts.Limit,
		Offset:  opts.Offset,
		HasMore: opts.Offset+len(items) < total,
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
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get %s/%s from %s: %w", namespace, name, q.tableName, ErrNotFound)
		}
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

	// A missing UID stays empty on purpose. Synthesizing one would claim an
	// object instance identity that the management API never issued, and a
	// reader could not tell a real UID from a fabricated one.

	// Build ID from namespace + name (latest table pattern)
	if base.Namespace != "" {
		base.ID = base.Namespace + "/" + base.Name
	} else {
		base.ID = base.Name
	}
}

// jsonPathExpr returns the SQL expression that extracts one JSONB path from the
// raw document of the given table alias, together with its bind arguments.
//
// The path is bound as a text[] parameter instead of being formatted into the
// statement. Selector paths and label keys come from untrusted query strings, so
// formatting them into a quoted SQL literal would let a caller terminate the
// literal and append arbitrary predicates.
func jsonPathExpr(alias string, path []string) (string, []any) {
	return fmt.Sprintf("%s.raw #>> ?", alias), []any{pgdialect.Array(path)}
}

// labelColumnExpr returns the SQL expression that reads one label value for the
// given table alias, together with its bind arguments.
//
// The tenant label is normalized: an object stored without the label, or with an
// empty value, belongs to the default tenant, exactly as tenant.IDOf reports and
// as the gateway reports when it serializes an object. A label selector cannot
// express "value in (...) or label absent", so the missing value is normalized
// here instead. Filtering in SQL keeps total and the paging counters consistent
// with the rows the caller is allowed to see.
func labelColumnExpr(alias, key string) (string, []any) {
	expr, args := jsonPathExpr(alias, []string{"metadata", "labels", key})
	if key == tenant.IDLabel {
		expr = fmt.Sprintf("COALESCE(NULLIF(%s, ''), ?)", expr)
		args = append(args, tenant.DefaultID)
	}
	return expr, args
}

// applySelector applies a selector operator to a query. colArgs carries the bind
// arguments of colExpr and must precede the value argument.
//
// "!=" uses IS DISTINCT FROM so that an object without the field or label
// matches, which is the selector semantics Kubernetes defines and the semantics
// the Kubernetes backed list path already implements. A plain "!=" comparison
// evaluates to NULL for a missing key and would silently drop those objects.
func applySelector(query *bun.SelectQuery, colExpr string, colArgs []any, op, value string) *bun.SelectQuery {
	args := make([]any, 0, len(colArgs)+1)
	args = append(args, colArgs...)
	switch op {
	case "!=":
		return query.Where(colExpr+" IS DISTINCT FROM ?", append(args, value)...)
	case "in":
		// The candidate list must be bound as a text[] parameter. A plain
		// []string is rendered as a JSON array, which PostgreSQL rejects with
		// "malformed array literal".
		return query.Where(colExpr+" = ANY(?)", append(args, pgdialect.Array(strings.Split(value, ",")))...)
	default:
		return query.Where(colExpr+" = ?", append(args, value)...)
	}
}

// applyTagSelectors restricts the query to objects carrying the requested tags.
// Values of the same key are ORed, different keys are ANDed. Keys are applied in
// a stable order so that the same selector always renders the same SQL.
//
// "?TableAlias" is a named placeholder: bun substitutes it from the query model
// and it does not consume a positional argument. Passing the alias as an
// argument as well shifts every remaining argument by one position, which makes
// the key comparison resolve against the table row type and the database reject
// the statement.
func applyTagSelectors(query *bun.SelectQuery, selectors []TagSelector) *bun.SelectQuery {
	if len(selectors) == 0 {
		return query
	}

	valuesByKey := make(map[string][]string, len(selectors))
	keys := make([]string, 0, len(selectors))
	for _, ts := range selectors {
		if _, seen := valuesByKey[ts.Key]; !seen {
			keys = append(keys, ts.Key)
		}
		valuesByKey[ts.Key] = append(valuesByKey[ts.Key], ts.Value)
	}
	sort.Strings(keys)

	for _, key := range keys {
		query = query.Where(
			"EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(?TableAlias.raw->'spec'->'tags', '[]'::jsonb)) AS tag WHERE tag->>'key' = ? AND EXISTS (SELECT 1 FROM jsonb_array_elements_text(COALESCE(tag->'values', '[]'::jsonb)) AS value WHERE value IN (?)))",
			key, bun.List(valuesByKey[key]),
		)
	}
	return query
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
