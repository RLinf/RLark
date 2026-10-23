package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/gateway/restore"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// maxListLimit bounds every list endpoint so that a single request cannot
// trigger an unbounded scan. It is also the implicit page size when the client
// does not pass "limit"; the response reports "total" and "hasMore" so that a
// truncated result set is always detectable.
const maxListLimit = 500

// fieldPathPattern restricts user supplied sort columns and field selector
// paths to plain dotted identifiers. The store binds those paths as query
// arguments, so this check is defence in depth: it rejects a nonsensical path at
// the edge instead of letting it reach the database and fail there.
var fieldPathPattern = regexp.MustCompile(`^[A-Za-z0-9_]+(\.[A-Za-z0-9_]+)*$`)

// invalidQueryParam reports a malformed query parameter. Handlers translate it
// into a 400 response instead of silently dropping the filter.
type invalidQueryParam struct {
	Param  string
	Reason string
}

func (e *invalidQueryParam) Error() string {
	return fmt.Sprintf("invalid query parameter %q: %s", e.Param, e.Reason)
}

func invalidParam(param, reason string) error {
	return &invalidQueryParam{Param: param, Reason: reason}
}

// respondInvalidQueryParam rejects a malformed query parameter with 400 and
// reports which parameter was refused, so that a client can tell a rejected
// filter apart from a filter that matched nothing.
func respondInvalidQueryParam(c *gin.Context, err error) {
	var invalid *invalidQueryParam
	if errors.As(err, &invalid) {
		respondErrorWithDetails(c, http.StatusBadRequest, CodeInvalidQueryParameter, invalid.Error(), map[string]any{
			"parameter": invalid.Param,
			"reason":    invalid.Reason,
		})
		return
	}
	respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, err.Error())
}

// --- Query parameter parsing ---

// parseListOptions parses and validates list query parameters. Invalid values are
// errors: silently ignoring an unknown selector or a malformed limit would make
// a client believe it received an unfiltered result set.
func (g *Gateway) parseListOptions(c *gin.Context) (db.ListOptions, error) {
	opts := db.ListOptions{
		Namespace: c.Query("namespace"),
		Limit:     maxListLimit,
	}

	// Pagination
	if limitStr := c.Query("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return opts, invalidParam("limit", "must be an integer")
		}
		if limit <= 0 {
			return opts, invalidParam("limit", "must be greater than 0")
		}
		if limit > maxListLimit {
			return opts, invalidParam("limit", fmt.Sprintf("must not exceed %d", maxListLimit))
		}
		opts.Limit = limit
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return opts, invalidParam("offset", "must be an integer")
		}
		if offset < 0 {
			return opts, invalidParam("offset", "must not be negative")
		}
		opts.Offset = offset
	}

	// Ordering: comma-separated, e.g. "created_at desc,name"
	if orderStr := c.Query("order"); orderStr != "" {
		for _, field := range strings.Split(orderStr, ",") {
			field = strings.TrimSpace(field)
			if field == "" {
				continue
			}
			parts := strings.SplitN(field, " ", 2)
			if !fieldPathPattern.MatchString(parts[0]) {
				return opts, invalidParam("order", fmt.Sprintf("unsupported field %q", parts[0]))
			}
			dir := ""
			if len(parts) == 2 {
				dir = strings.ToLower(strings.TrimSpace(parts[1]))
				if dir != "asc" && dir != "desc" {
					return opts, invalidParam("order", fmt.Sprintf("unsupported direction %q", parts[1]))
				}
			}
			// Hand the store a canonical "field" or "field dir" term. Passing the
			// raw text through would let "name  desc" reach the store with a
			// direction it does not recognise and sort the other way.
			if dir != "" {
				field = parts[0] + " " + dir
			} else {
				field = parts[0]
			}
			opts.OrderBy = append(opts.OrderBy, field)
		}
	}

	// Field selectors: comma-separated, e.g. "status.phase=Running,spec.agentType=Kubernetes"
	if fsStr := c.Query("fieldSelector"); fsStr != "" {
		for _, pair := range strings.Split(fsStr, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			fs, err := parseFieldSelector(pair)
			if err != nil {
				return opts, invalidParam("fieldSelector", fmt.Sprintf("cannot parse %q", pair))
			}
			if !fieldPathPattern.MatchString(fs.Path) {
				return opts, invalidParam("fieldSelector", fmt.Sprintf("unsupported field %q", fs.Path))
			}
			opts.FieldSelector = append(opts.FieldSelector, fs)
		}
	}

	// Label selectors: comma-separated, e.g. "env=prod"
	if lsStr := c.Query("labelSelector"); lsStr != "" {
		for _, pair := range strings.Split(lsStr, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			ls, err := parseLabelSelector(pair)
			if err != nil {
				return opts, invalidParam("labelSelector", fmt.Sprintf("cannot parse %q", pair))
			}
			if errs := validation.IsQualifiedName(ls.Key); len(errs) > 0 {
				return opts, invalidParam("labelSelector", fmt.Sprintf("invalid label key %q: %s", ls.Key, strings.Join(errs, "; ")))
			}
			opts.LabelSelector = append(opts.LabelSelector, ls)
		}
	}

	if tsStr := c.Query("tagSelector"); tsStr != "" {
		for _, pair := range strings.Split(tsStr, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			ts, err := parseTagSelector(pair)
			if err != nil {
				return opts, invalidParam("tagSelector", fmt.Sprintf("cannot parse %q", pair))
			}
			opts.TagSelector = append(opts.TagSelector, ts)
		}
	}

	// First class filters. Callers should not have to know that the tenant is
	// stored as a label and the lifecycle phase is stored under status.phase.
	if tenantID := strings.TrimSpace(c.Query("tenant")); tenantID != "" {
		opts.LabelSelector = append(opts.LabelSelector, db.LabelSelector{
			Key: tenant.IDLabel, Op: "=", Value: tenantID,
		})
	}
	if phase := strings.TrimSpace(c.Query("phase")); phase != "" {
		opts.FieldSelector = append(opts.FieldSelector, db.FieldSelector{
			Path: "status.phase", Op: "=", Value: phase,
		})
	}

	return opts, nil
}

// parseFieldSelector parses "path=value" or "path!=value" into a FieldSelector.
func parseFieldSelector(s string) (db.FieldSelector, error) {
	if strings.Contains(s, "!=") {
		parts := strings.SplitN(s, "!=", 2)
		if parts[0] == "" || parts[1] == "" {
			return db.FieldSelector{}, strconv.ErrSyntax
		}
		return db.FieldSelector{Path: parts[0], Op: "!=", Value: parts[1]}, nil
	}
	if strings.Contains(s, "=") {
		parts := strings.SplitN(s, "=", 2)
		if parts[0] == "" || parts[1] == "" {
			return db.FieldSelector{}, strconv.ErrSyntax
		}
		return db.FieldSelector{Path: parts[0], Op: "=", Value: parts[1]}, nil
	}
	return db.FieldSelector{}, strconv.ErrSyntax
}

// parseLabelSelector parses "key=value" or "key!=value" into a LabelSelector.
func parseLabelSelector(s string) (db.LabelSelector, error) {
	if strings.Contains(s, "!=") {
		parts := strings.SplitN(s, "!=", 2)
		if parts[0] == "" || parts[1] == "" {
			return db.LabelSelector{}, strconv.ErrSyntax
		}
		return db.LabelSelector{Key: parts[0], Op: "!=", Value: parts[1]}, nil
	}
	if strings.Contains(s, "=") {
		parts := strings.SplitN(s, "=", 2)
		if parts[0] == "" || parts[1] == "" {
			return db.LabelSelector{}, strconv.ErrSyntax
		}
		return db.LabelSelector{Key: parts[0], Op: "=", Value: parts[1]}, nil
	}
	return db.LabelSelector{}, strconv.ErrSyntax
}

func parseTagSelector(s string) (db.TagSelector, error) {
	parts := strings.SplitN(s, "=", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return db.TagSelector{}, strconv.ErrSyntax
	}
	return db.TagSelector{Key: parts[0], Value: parts[1]}, nil
}

// --- Read handler implementations (database or Kubernetes API-backed) ---

// usePersistenceBackend 判断资源是否走 persistence Backend。
// 只有配置在 SyncResources 里的资源才走 Backend，其他走 KCP。
func (g *Gateway) usePersistenceBackend(resource string) bool {
	for _, r := range g.config.SyncResources {
		if r == resource {
			return true
		}
	}
	return false
}

func (g *Gateway) handleList(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.usePersistenceBackend(resource) && g.persistenceBackend != nil {
			g.handleListFromPersistence(c, resource)
			return
		}
		g.handleListKube(c, resource)
	}
}

func (g *Gateway) handleGet(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if g.usePersistenceBackend(resource) && g.persistenceBackend != nil {
			g.handleGetFromPersistence(c, resource)
			return
		}
		// 无 Backend：从 KCP 查
		g.handleGetKube(c, resource)
	}
}

func (g *Gateway) handleListJobs(c *gin.Context) {
	if g.persistenceBackend != nil {
		g.handleListFromPersistence(c, "jobs")
		return
	}
	g.handleListJobsKube(c)
}

type jobTag struct {
	Key    string   `json:"key"`
	Values []string `json:"values"`
}

func (g *Gateway) handleListJobTags(c *gin.Context) {
	if g.persistenceBackend != nil {
		g.handleListJobTagsFromPersistence(c)
		return
	}
	g.handleListJobTagsKube(c)
}

// --- Database-backed read handlers ---

// handleListFromPersistence 从持久化存储读列表（统一 PG 和 Secret）。
func (g *Gateway) handleListFromPersistence(c *gin.Context, kind string) {
	scope, ok := g.tenantScopeFor(c, kind)
	if !ok {
		return
	}
	if !authorizeTenantQuery(c, scope) {
		return
	}

	opts, err := g.parseListOptions(c)
	if err != nil {
		respondInvalidQueryParam(c, err)
		return
	}
	scope.restrictListOptions(&opts)
	result, err := g.persistenceBackend.List(c, kind, opts)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	for _, item := range result.Items {
		normalizeTenantMap(item)
	}
	c.JSON(http.StatusOK, result)
}

// handleGetFromPersistence 从持久化存储读单个资源（统一 PG 和 Secret）。
func (g *Gateway) handleGetFromPersistence(c *gin.Context, kind string) {
	scope, ok := g.tenantScopeFor(c, kind)
	if !ok {
		return
	}
	if !authorizeTenantQuery(c, scope) {
		return
	}

	name := c.Param("name")
	namespace := c.Query("namespace")

	resourceData, err := g.persistenceBackend.Load(c, kind, namespace, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "not found")
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	var item map[string]any
	if err := json.Unmarshal(resourceData.Raw, &item); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("unmarshal %s %s: %v", kind, name, err))
		return
	}
	if !authorizeTenantObject(c, scope, tenantIDOfMap(item)) {
		return
	}
	normalizeTenantMap(item)
	c.JSON(http.StatusOK, item)
}

func collectJobTags(jobs []rlarkiov1alpha1.Job) []jobTag {
	seen := make(map[string]map[string]struct{})
	for _, job := range jobs {
		for _, tag := range job.Spec.Tags {
			if seen[tag.Key] == nil {
				seen[tag.Key] = make(map[string]struct{})
			}
			for _, value := range tag.Values {
				seen[tag.Key][value] = struct{}{}
			}
		}
	}

	tags := make([]jobTag, 0, len(seen))
	for key, values := range seen {
		tag := jobTag{Key: key, Values: make([]string, 0, len(values))}
		for value := range values {
			tag.Values = append(tag.Values, value)
		}
		sort.Strings(tag.Values)
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Key < tags[j].Key
	})
	return tags
}

// jobTagListOptions selects the Job objects whose tags form the reported
// vocabulary.
//
// The vocabulary is derived from Job objects, so it is restricted to the caller
// scope as well: an unrestricted aggregation would disclose tag keys and values
// belonging to other tenants. An explicit "tenant" filter narrows the
// aggregation exactly as it narrows a Job list, so that the reported vocabulary
// describes the same object set as the list it is meant to filter.
func jobTagListOptions(c *gin.Context, scope tenantScope) db.ListOptions {
	opts := db.ListOptions{Namespace: c.Query("namespace")}
	if tenantID := strings.TrimSpace(c.Query("tenant")); tenantID != "" {
		opts.LabelSelector = append(opts.LabelSelector, db.LabelSelector{
			Key: tenant.IDLabel, Op: "=", Value: tenantID,
		})
	}
	scope.restrictListOptions(&opts)
	return opts
}

// handleListJobTagsFromPersistence 从持久化存储读 Job 标签（统一 PG 和 Secret）。
func (g *Gateway) handleListJobTagsFromPersistence(c *gin.Context) {
	scope, ok := g.tenantScopeFor(c, "jobs")
	if !ok {
		return
	}
	if !authorizeTenantQuery(c, scope) {
		return
	}

	result, err := g.persistenceBackend.List(c, "jobs", jobTagListOptions(c, scope))
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	seen := make(map[string]map[string]struct{})
	for _, item := range result.Items {
		var job struct {
			Spec struct {
				Tags []jobTag `json:"tags"`
			} `json:"spec"`
		}
		data, _ := json.Marshal(item)
		if json.Unmarshal(data, &job) != nil {
			continue
		}
		for _, tag := range job.Spec.Tags {
			if seen[tag.Key] == nil {
				seen[tag.Key] = make(map[string]struct{})
			}
			for _, value := range tag.Values {
				seen[tag.Key][value] = struct{}{}
			}
		}
	}

	tags := make([]jobTag, 0, len(seen))
	for key, values := range seen {
		tag := jobTag{Key: key, Values: make([]string, 0, len(values))}
		for value := range values {
			tag.Values = append(tag.Values, value)
		}
		sort.Strings(tag.Values)
		tags = append(tags, tag)
	}
	sort.Slice(tags, func(i, j int) bool {
		return tags[i].Key < tags[j].Key
	})
	c.JSON(http.StatusOK, gin.H{"items": tags})
}

func normalizeTenantMap(object map[string]any) {
	metadata, _ := object["metadata"].(map[string]any)
	if metadata == nil {
		metadata = map[string]any{}
		object["metadata"] = metadata
	}
	labels, _ := metadata["labels"].(map[string]any)
	if labels == nil {
		labels = map[string]any{}
		metadata["labels"] = labels
	}
	if value, _ := labels[tenant.IDLabel].(string); value == "" {
		labels[tenant.IDLabel] = tenant.DefaultID
	}
}

// handleStartJob 处理 POST /jobs/:name/start 请求。
// 统一启动接口：活跃 Job 直接 Patch，归档 Job 从 Backend 恢复。
//
// The request carries no tenant identity of its own, so both branches decide
// ownership from the stored object: the active branch from the management API
// object it is about to patch, the archive branch from the snapshot it is about
// to recreate.
func (g *Gateway) handleStartJob(c *gin.Context) {
	name := c.Param("name")

	scope, ok := g.tenantScopeFor(c, "jobs")
	if !ok {
		return
	}

	// 活跃 Job：KCP 有 CR，直接 Patch spec.stopped=false
	existing, err := g.accessors["jobs"].get(c, c.Query("namespace"), name, metav1.GetOptions{})
	if err == nil {
		tenantID, tenantErr := tenantIDOfObject(existing)
		if tenantErr != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to inspect object metadata")
			return
		}
		if !authorizeTenantObject(c, scope, tenantID) {
			return
		}

		patch := []byte(`{"spec":{"suspended":false}}`)
		result, patchErr := g.accessors["jobs"].patch(c, c.Query("namespace"), name, patch)
		if patchErr != nil {
			respondError(c, http.StatusInternalServerError, CodeInternalError, patchErr.Error())
			return
		}
		c.JSON(http.StatusOK, result)
		return
	}
	// 只有 not found 时才走归档逻辑，其他错误直接返回
	if !apierrors.IsNotFound(err) {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}

	if g.persistenceBackend == nil {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, fmt.Sprintf("job %s not found", name))
		return
	}

	restorable := restore.Registry["jobs"]
	arch, err := g.persistenceBackend.Load(c, "jobs", "", name)
	if err != nil {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, fmt.Sprintf("job %s not archived or not found", name))
		return
	}

	var archived map[string]any
	if err := json.Unmarshal(arch.Raw, &archived); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("unmarshal archived job: %v", err))
		return
	}
	if !authorizeTenantObject(c, scope, tenantIDOfMap(archived)) {
		return
	}

	newObj, err := restorable.BuildRestoredObject(archived, name)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("build restored job: %v", err))
		return
	}

	setSuspendedFalse(newObj)

	created, err := g.createRestoredResource(c, "jobs", newObj)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("create restored %s: %v", restorable.Kind(), err))
		return
	}

	c.JSON(http.StatusOK, created)
}

// createRestoredResource 根据资源类型调用对应的 KCP Create。
func (g *Gateway) createRestoredResource(ctx context.Context, resource string, obj client.Object) (client.Object, error) {
	return restore.NewCreator(g.kubeClient).Create(ctx, resource, obj)
}

// setSuspendedFalse 通用方式设置 spec.suspended = false。
// 当前只支持 Job，后续支持其他资源时按需扩展。
func setSuspendedFalse(obj client.Object) {
	switch o := obj.(type) {
	case *rlarkiov1alpha1.Job:
		o.Spec.Suspended = false
	}
}
