package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	versioned "github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// resourceAccessor encapsulates Kubernetes client operations for a specific resource type.
type resourceAccessor struct {
	list   func(ctx context.Context, namespace string, opts metav1.ListOptions) (any, error)
	get    func(ctx context.Context, namespace, name string, opts metav1.GetOptions) (any, error)
	create func(ctx context.Context, namespace string, body json.RawMessage) (any, error)
	update func(ctx context.Context, namespace, name string, body json.RawMessage) (any, error)
	patch  func(ctx context.Context, namespace, name string, data []byte) (any, error)
	delete func(ctx context.Context, namespace, name string) error
}

// kubeClient wraps the typed CRUD operations for a Kubernetes resource type T.
// It centralizes the common create/update/patch/delete patterns so that each
// resource only needs to provide thin closures that call the typed client methods.
type kubeClient[T any] struct {
	doList   func(ctx context.Context, namespace string, opts metav1.ListOptions) (any, error)
	doGet    func(ctx context.Context, namespace, name string, opts metav1.GetOptions) (*T, error)
	doCreate func(ctx context.Context, namespace string, obj *T, opts metav1.CreateOptions) (*T, error)
	doUpdate func(ctx context.Context, namespace string, obj *T, opts metav1.UpdateOptions) (*T, error)
	doPatch  func(ctx context.Context, namespace, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*T, error)
	doDelete func(ctx context.Context, namespace, name string, opts metav1.DeleteOptions) error
}

// accessor builds a resourceAccessor from the typed Kubernetes client,
// handling unmarshal, ResourceVersion propagation, and option wrapping.
func (c *kubeClient[T]) accessor() *resourceAccessor {
	return &resourceAccessor{
		list: c.doList,
		get: func(ctx context.Context, ns, name string, opts metav1.GetOptions) (any, error) {
			return c.doGet(ctx, ns, name, opts)
		},
		create: func(ctx context.Context, ns string, body json.RawMessage) (any, error) {
			obj := new(T)
			if err := json.Unmarshal(body, obj); err != nil {
				return nil, err
			}
			if task, ok := any(obj).(*rlarkiov1alpha1.Task); ok {
				task.Status = rlarkiov1alpha1.TaskStatus{}
			}
			return c.doCreate(ctx, ns, obj, metav1.CreateOptions{})
		},
		update: func(ctx context.Context, ns, name string, body json.RawMessage) (any, error) {
			current, err := c.doGet(ctx, ns, name, metav1.GetOptions{})
			if err != nil {
				return nil, err
			}
			obj := new(T)
			if err := json.Unmarshal(body, obj); err != nil {
				return nil, err
			}
			any(obj).(metav1.Object).SetResourceVersion(any(current).(metav1.Object).GetResourceVersion())
			return c.doUpdate(ctx, ns, obj, metav1.UpdateOptions{})
		},
		patch: func(ctx context.Context, ns, name string, data []byte) (any, error) {
			return c.doPatch(ctx, ns, name, types.MergePatchType, data, metav1.PatchOptions{})
		},
		delete: func(ctx context.Context, ns, name string) error {
			return c.doDelete(ctx, ns, name, metav1.DeleteOptions{})
		},
	}
}

// isNamespacedResource reports whether a resource type lives inside a Kubernetes
// namespace and therefore requires a namespace query parameter for single-object
// operations (get, update, patch, delete). Resources not listed here are treated
// as cluster-scoped.
func isNamespacedResource(resource string) bool {
	switch resource {
	case "nodes", "tasks", "pods":
		return true
	}
	return false
}

// registerAccessors builds the resourceAccessor map for all supported resources.
func registerAccessors(client versioned.Interface) map[string]*resourceAccessor {
	return map[string]*resourceAccessor{
		"jobs": (&kubeClient[rlarkiov1alpha1.Job]{
			doList: func(ctx context.Context, _ string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Jobs().List(ctx, opts)
			},
			doGet: func(ctx context.Context, _, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Job, error) {
				return client.RlinfV1alpha1().Jobs().Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Job, opts metav1.CreateOptions) (*rlarkiov1alpha1.Job, error) {
				return client.RlinfV1alpha1().Jobs().Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Job, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Job, error) {
				return client.RlinfV1alpha1().Jobs().Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, _, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Job, error) {
				return client.RlinfV1alpha1().Jobs().Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, _, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Jobs().Delete(ctx, name, opts)
			},
		}).accessor(),

		"workflows": (&kubeClient[rlarkiov1alpha1.Workflow]{
			doList: func(ctx context.Context, _ string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Workflows().List(ctx, opts)
			},
			doGet: func(ctx context.Context, _, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Workflow, error) {
				return client.RlinfV1alpha1().Workflows().Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Workflow, opts metav1.CreateOptions) (*rlarkiov1alpha1.Workflow, error) {
				return client.RlinfV1alpha1().Workflows().Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Workflow, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Workflow, error) {
				return client.RlinfV1alpha1().Workflows().Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, _, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Workflow, error) {
				return client.RlinfV1alpha1().Workflows().Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, _, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Workflows().Delete(ctx, name, opts)
			},
		}).accessor(),

		"nodes": (&kubeClient[rlarkiov1alpha1.Node]{
			doList: func(ctx context.Context, ns string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Nodes(ns).List(ctx, opts)
			},
			doGet: func(ctx context.Context, ns, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Node, error) {
				return client.RlinfV1alpha1().Nodes(ns).Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Node, opts metav1.CreateOptions) (*rlarkiov1alpha1.Node, error) {
				return client.RlinfV1alpha1().Nodes(ns).Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Node, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Node, error) {
				return client.RlinfV1alpha1().Nodes(ns).Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Node, error) {
				return client.RlinfV1alpha1().Nodes(ns).Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, ns, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Nodes(ns).Delete(ctx, name, opts)
			},
		}).accessor(),

		"tasks": (&kubeClient[rlarkiov1alpha1.Task]{
			doList: func(ctx context.Context, ns string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Tasks(ns).List(ctx, opts)
			},
			doGet: func(ctx context.Context, ns, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Task, error) {
				return client.RlinfV1alpha1().Tasks(ns).Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Task, opts metav1.CreateOptions) (*rlarkiov1alpha1.Task, error) {
				return client.RlinfV1alpha1().Tasks(ns).Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Task, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Task, error) {
				return client.RlinfV1alpha1().Tasks(ns).Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Task, error) {
				return client.RlinfV1alpha1().Tasks(ns).Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, ns, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Tasks(ns).Delete(ctx, name, opts)
			},
		}).accessor(),

		"pods": (&kubeClient[rlarkiov1alpha1.Pod]{
			doList: func(ctx context.Context, ns string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Pods(ns).List(ctx, opts)
			},
			doGet: func(ctx context.Context, ns, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Pod, error) {
				return client.RlinfV1alpha1().Pods(ns).Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Pod, opts metav1.CreateOptions) (*rlarkiov1alpha1.Pod, error) {
				return client.RlinfV1alpha1().Pods(ns).Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, ns string, obj *rlarkiov1alpha1.Pod, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Pod, error) {
				return client.RlinfV1alpha1().Pods(ns).Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, ns, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Pod, error) {
				return client.RlinfV1alpha1().Pods(ns).Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, ns, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Pods(ns).Delete(ctx, name, opts)
			},
		}).accessor(),

		"domains": (&kubeClient[rlarkiov1alpha1.Domain]{
			doList: func(ctx context.Context, _ string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().Domains().List(ctx, opts)
			},
			doGet: func(ctx context.Context, _, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.Domain, error) {
				return client.RlinfV1alpha1().Domains().Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Domain, opts metav1.CreateOptions) (*rlarkiov1alpha1.Domain, error) {
				return client.RlinfV1alpha1().Domains().Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.Domain, opts metav1.UpdateOptions) (*rlarkiov1alpha1.Domain, error) {
				return client.RlinfV1alpha1().Domains().Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, _, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.Domain, error) {
				return client.RlinfV1alpha1().Domains().Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, _, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().Domains().Delete(ctx, name, opts)
			},
		}).accessor(),

		"upgradejobs": (&kubeClient[rlarkiov1alpha1.UpgradeJob]{
			doList: func(ctx context.Context, _ string, opts metav1.ListOptions) (any, error) {
				return client.RlinfV1alpha1().UpgradeJobs().List(ctx, opts)
			},
			doGet: func(ctx context.Context, _, name string, opts metav1.GetOptions) (*rlarkiov1alpha1.UpgradeJob, error) {
				return client.RlinfV1alpha1().UpgradeJobs().Get(ctx, name, opts)
			},
			doCreate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.UpgradeJob, opts metav1.CreateOptions) (*rlarkiov1alpha1.UpgradeJob, error) {
				return client.RlinfV1alpha1().UpgradeJobs().Create(ctx, obj, opts)
			},
			doUpdate: func(ctx context.Context, _ string, obj *rlarkiov1alpha1.UpgradeJob, opts metav1.UpdateOptions) (*rlarkiov1alpha1.UpgradeJob, error) {
				return client.RlinfV1alpha1().UpgradeJobs().Update(ctx, obj, opts)
			},
			doPatch: func(ctx context.Context, _, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (*rlarkiov1alpha1.UpgradeJob, error) {
				return client.RlinfV1alpha1().UpgradeJobs().Patch(ctx, name, pt, data, opts, subresources...)
			},
			doDelete: func(ctx context.Context, _, name string, opts metav1.DeleteOptions) error {
				return client.RlinfV1alpha1().UpgradeJobs().Delete(ctx, name, opts)
			},
		}).accessor(),
	}
}

type nodeListContinue struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

type nodeLister interface {
	List(ctx context.Context, namespace string, opts metav1.ListOptions) (*rlarkiov1alpha1.NodeList, error)
}

type nodeListerFunc func(ctx context.Context, namespace string, opts metav1.ListOptions) (*rlarkiov1alpha1.NodeList, error)

func (f nodeListerFunc) List(ctx context.Context, namespace string, opts metav1.ListOptions) (*rlarkiov1alpha1.NodeList, error) {
	return f(ctx, namespace, opts)
}

type kubeNodeLister struct {
	client versioned.Interface
}

func (l kubeNodeLister) List(ctx context.Context, namespace string, opts metav1.ListOptions) (*rlarkiov1alpha1.NodeList, error) {
	return l.client.RlinfV1alpha1().Nodes(namespace).List(ctx, opts)
}

func (g *Gateway) nodes() nodeLister {
	if g.nodeLister != nil {
		return g.nodeLister
	}
	return kubeNodeLister{client: g.kubeClient}
}

func (g *Gateway) handleListNodes(c *gin.Context) {
	if g.kubeClient == nil {
		respondError(c, http.StatusInternalServerError, CodeNotConfigured, "Kubernetes client is not configured")
		return
	}

	opts, err := parseKubeListOptions(c)
	if err != nil {
		respondInvalidQueryParam(c, err)
		return
	}
	if opts.FieldSelector != "" || opts.LabelSelector != "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "fieldSelector and labelSelector are not supported for aggregated Node lists")
		return
	}

	clusters, err := g.kubeClient.RlinfV1alpha1().Clusters().List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	clusterNamespaces := uniqueClusterNamespaces(c.Request.Context(), visibleClusterNamespaces(clusters.Items, c.GetString(authRoleKey) == "admin"))

	requestedNamespace := c.Query("namespace")
	if requestedNamespace != "" {
		if _, ok := clusterNamespaces[requestedNamespace]; !ok {
			c.JSON(http.StatusOK, nodeListResponse(opts.Limit, []rlarkiov1alpha1.Node{}, "", 0))
			return
		}
		clusterNamespaces = map[string]struct{}{requestedNamespace: {}}
	}

	cursor, err := decodeNodeListContinue(opts.Continue)
	if err != nil {
		respondInvalidQueryParam(c, invalidParam("continue", err.Error()))
		return
	}

	items, hasMore, err := g.listAggregatedNodes(c.Request.Context(), clusterNamespaces, cursor, opts.Limit)
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	next := ""
	if hasMore {
		next = encodeNodeListContinue(items[len(items)-1])
	}
	c.JSON(http.StatusOK, nodeListResponse(opts.Limit, items, next, -1))
}

func (g *Gateway) listAggregatedNodes(ctx context.Context, namespaces map[string]struct{}, cursor *nodeListContinue, limit int64) ([]rlarkiov1alpha1.Node, bool, error) {
	items := make([]rlarkiov1alpha1.Node, 0, limit+1)
	orderedNamespaces := make([]string, 0, len(namespaces))
	for namespace := range namespaces {
		if cursor == nil || namespace >= cursor.Namespace {
			orderedNamespaces = append(orderedNamespaces, namespace)
		}
	}
	sort.Strings(orderedNamespaces)

	// orderedNamespaces is sorted and the management API returns objects in name
	// order within a namespace, so items is already globally ordered by
	// nodeListKey. Stop as soon as one extra item beyond limit is collected so
	// that memory and the number of upstream requests stay bounded by limit.
	for _, namespace := range orderedNamespaces {
		continueToken := ""
		for int64(len(items)) <= limit {
			nodes, err := g.nodes().List(ctx, namespace, metav1.ListOptions{Limit: limit + 1, Continue: continueToken})
			if err != nil {
				return nil, false, err
			}
			for _, node := range nodes.Items {
				if cursor == nil || nodeListKey(node) > nodeListCursorKey(*cursor) {
					items = append(items, node)
				}
			}
			if nodes.Continue == "" {
				break
			}
			continueToken = nodes.Continue
		}
		if int64(len(items)) > limit {
			break
		}
	}
	hasMore := int64(len(items)) > limit
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

func visibleClusterNamespaces(clusters []rlarkiov1alpha1.Cluster, includeDelisted bool) []rlarkiov1alpha1.Cluster {
	if includeDelisted {
		return clusters
	}

	visible := make([]rlarkiov1alpha1.Cluster, 0, len(clusters))
	for _, cluster := range clusters {
		if !cluster.Spec.Delisted {
			visible = append(visible, cluster)
		}
	}
	return visible
}

func uniqueClusterNamespaces(ctx context.Context, clusters []rlarkiov1alpha1.Cluster) map[string]struct{} {
	namespaces := make(map[string]string, len(clusters))
	for _, cluster := range clusters {
		namespace := cluster.Spec.AgentNamespace
		if namespace == "" {
			continue
		}
		if owner, ok := namespaces[namespace]; ok && owner != cluster.Name {
			log.FromContext(ctx).Info("Multiple Clusters claim the same agent namespace; using the namespace once", "agentNamespace", namespace, "clusters", []string{owner, cluster.Name})
			continue
		}
		namespaces[namespace] = cluster.Name
	}
	result := make(map[string]struct{}, len(namespaces))
	for namespace := range namespaces {
		result[namespace] = struct{}{}
	}
	return result
}

func decodeNodeListContinue(token string) (*nodeListContinue, error) {
	if token == "" {
		return nil, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("must be a valid Node list continue token")
	}
	var cursor nodeListContinue
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Namespace == "" || cursor.Name == "" {
		return nil, fmt.Errorf("must be a valid Node list continue token")
	}
	return &cursor, nil
}

func encodeNodeListContinue(node rlarkiov1alpha1.Node) string {
	data, _ := json.Marshal(nodeListContinue{Namespace: node.Namespace, Name: node.Name})
	return base64.RawURLEncoding.EncodeToString(data)
}

func nodeListKey(node rlarkiov1alpha1.Node) string {
	return node.Namespace + "\x00" + node.Name
}

func nodeListCursorKey(cursor nodeListContinue) string {
	return cursor.Namespace + "\x00" + cursor.Name
}

func nodeListResponse(limit int64, items []rlarkiov1alpha1.Node, continueToken string, total int) map[string]any {
	response := map[string]any{
		"apiVersion": rlarkiov1alpha1.SchemeGroupVersion.String(),
		"kind":       "NodeList",
		"items":      items,
		"limit":      limit,
		"hasMore":    continueToken != "",
		"total":      total,
	}
	if continueToken != "" {
		response["metadata"] = map[string]string{"continue": continueToken}
	}
	return normalizedTenantObject(response).(map[string]any)
}

// --- Generic Kubernetes client handlers using resourceAccessor ---

func (g *Gateway) handleListJobsKube(c *gin.Context) {
	a, ok := g.accessors["jobs"]
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: jobs")
		return
	}

	scope, ok := g.tenantScopeFor(c, "jobs")
	if !ok {
		return
	}
	if !authorizeTenantQuery(c, scope) {
		return
	}

	opts, err := parseKubeListOptions(c)
	if err != nil {
		respondInvalidQueryParam(c, err)
		return
	}

	result, err := a.list(c.Request.Context(), c.Query("namespace"), opts)
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	jobs := result.(*rlarkiov1alpha1.JobList)
	if err := scope.restrictKubeList(jobs); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to apply tenant scope")
		return
	}
	selectors := parseTagSelectors(c.Query("tagSelector"))
	if len(selectors) > 0 {
		items := jobs.Items[:0]
		for _, job := range jobs.Items {
			if jobMatchesTagSelectors(job, selectors) {
				items = append(items, job)
			}
		}
		jobs.Items = items
	}
	// tagSelector removes items after the server already applied the window, so
	// the reported window no longer describes the returned set. hasMore still
	// reflects the untruncated server list and total is left to the server.
	c.JSON(http.StatusOK, withListEnvelope(normalizedTenantObject(jobs), int(opts.Limit)))
}

func (g *Gateway) handleListJobTagsKube(c *gin.Context) {
	a, ok := g.accessors["jobs"]
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: jobs")
		return
	}
	scope, ok := g.tenantScopeFor(c, "jobs")
	if !ok {
		return
	}
	if !authorizeTenantQuery(c, scope) {
		return
	}
	result, err := a.list(c.Request.Context(), c.Query("namespace"), metav1.ListOptions{})
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	jobs := result.(*rlarkiov1alpha1.JobList)
	// The tag vocabulary is derived from Job objects, so it is restricted to
	// the caller scope as well: an unrestricted aggregation would disclose tag
	// keys and values belonging to other tenants.
	if err := scope.restrictKubeList(jobs); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to apply tenant scope")
		return
	}
	tags := collectJobTags(jobs.Items)
	c.JSON(http.StatusOK, gin.H{"items": tags})
}

func parseTagSelectors(raw string) map[string]map[string]struct{} {
	selectors := make(map[string]map[string]struct{})
	for _, pair := range strings.Split(raw, ",") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			continue
		}
		if selectors[parts[0]] == nil {
			selectors[parts[0]] = make(map[string]struct{})
		}
		selectors[parts[0]][parts[1]] = struct{}{}
	}
	return selectors
}

func jobMatchesTagSelectors(job rlarkiov1alpha1.Job, selectors map[string]map[string]struct{}) bool {
	for key, values := range selectors {
		matched := false
		for _, tag := range job.Spec.Tags {
			if tag.Key != key {
				continue
			}
			for _, value := range tag.Values {
				if _, ok := values[value]; ok {
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// parseKubeListOptions builds Kubernetes list options from the query string. An
// unusable value is rejected rather than silently ignored, so that a caller
// cannot believe a pagination window was applied when it was not. The window is
// bounded even when the caller asks for no window, so that an omitted limit
// cannot turn into an unbounded management-API scan.
func parseKubeListOptions(c *gin.Context) (metav1.ListOptions, error) {
	opts := metav1.ListOptions{Limit: maxListLimit}
	if limit := c.Query("limit"); limit != "" {
		n, err := strconv.Atoi(limit)
		switch {
		case err != nil:
			return opts, invalidParam("limit", "must be an integer")
		case n <= 0:
			return opts, invalidParam("limit", "must be greater than 0")
		case n > maxListLimit:
			return opts, invalidParam("limit", fmt.Sprintf("must not exceed %d", maxListLimit))
		}
		opts.Limit = int64(n)
	}
	// The management API paginates with an opaque continue token and has no
	// offset. Accepting "offset" here would return the first page while the
	// caller believes it skipped ahead, so the parameter is refused instead.
	if c.Query("offset") != "" {
		return opts, invalidParam("offset", "is not supported for this resource; page with \"continue\" instead")
	}
	opts.Continue = c.Query("continue")
	opts.FieldSelector = c.Query("fieldSelector")
	opts.LabelSelector = c.Query("labelSelector")
	return opts, nil
}

func (g *Gateway) handleListKube(c *gin.Context, resource string) {
	a, ok := g.accessors[resource]
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: "+resource)
		return
	}

	scope, ok := g.tenantScopeFor(c, resource)
	if !ok {
		return
	}
	if !authorizeTenantQuery(c, scope) {
		return
	}

	ctx := c.Request.Context()
	namespace := c.Query("namespace")

	opts, err := parseKubeListOptions(c)
	if err != nil {
		respondInvalidQueryParam(c, err)
		return
	}

	result, err := a.list(ctx, namespace, opts)
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	if err := scope.restrictKubeList(result); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to apply tenant scope")
		return
	}
	c.JSON(http.StatusOK, withListEnvelope(normalizedTenantObject(result), int(opts.Limit)))
}

func (g *Gateway) handleGetKube(c *gin.Context, resource string) {
	a, ok := g.accessors[resource]
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: "+resource)
		return
	}

	scope, ok := g.tenantScopeFor(c, resource)
	if !ok {
		return
	}

	ctx := c.Request.Context()
	name := c.Param("name")
	namespace := c.Query("namespace")

	if isNamespacedResource(resource) && namespace == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest,
			"namespace query parameter is required for namespaced resource: "+resource)
		return
	}

	result, err := a.get(ctx, namespace, name, metav1.GetOptions{})
	if err != nil {
		// Mapped from the Kubernetes reason rather than reported as 404, so that a
		// missing object is distinguishable from an unreachable management API.
		respondKubeError(c, err.Error(), err)
		return
	}
	tenantID, err := tenantIDOfObject(result)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to inspect object metadata")
		return
	}
	if !authorizeTenantObject(c, scope, tenantID) {
		return
	}
	c.JSON(http.StatusOK, normalizedTenantObject(withRestoredTypeMeta(result)))
}

// kubeObjectScheme resolves the GroupVersionKind of the typed objects the
// Kubernetes client returns. It is the only way back to the TypeMeta that the
// typed client deliberately clears.
var kubeObjectScheme = func() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(rlarkiov1alpha1.AddToScheme(scheme))
	return scheme
}()

// withRestoredTypeMeta restores apiVersion and kind on a typed object read from
// the Kubernetes API. The typed client clears TypeMeta and both fields are
// omitempty, so without this the Kubernetes-backed single-object read would
// answer without a kind while the database-backed read keeps it, leaving one
// REST endpoint with two response shapes depending on the resource.
//
// The object is copied before stamping because the accessor may hand back a
// pointer that other readers share.
func withRestoredTypeMeta(value any) any {
	object, ok := value.(runtime.Object)
	if !ok {
		return value
	}
	if !object.GetObjectKind().GroupVersionKind().Empty() {
		return value
	}
	kinds, _, err := kubeObjectScheme.ObjectKinds(object)
	if err != nil || len(kinds) == 0 {
		return value
	}
	restored := object.DeepCopyObject()
	restored.GetObjectKind().SetGroupVersionKind(kinds[0])
	return restored
}

func normalizedTenantObject(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var root map[string]any
	if json.Unmarshal(data, &root) != nil {
		return value
	}
	if items, ok := root["items"].([]any); ok {
		for _, item := range items {
			if object, ok := item.(map[string]any); ok {
				normalizeTenantMap(object)
			}
		}
	} else {
		normalizeTenantMap(root)
	}
	return root
}

// withListEnvelope adds the shared pagination fields to a normalized Kubernetes
// list response so that a client sees the same contract on both the Kubernetes
// and the database-backed list path.
//
// The Kubernetes continue token stays inside metadata on purpose: it is only
// valid for a direct Kubernetes list and must not become the archive pagination
// contract. offset is therefore not reported here, and total is only reported
// when the server actually gave us enough information to compute it.
func withListEnvelope(normalized any, limit int) any {
	root, ok := normalized.(map[string]any)
	if !ok {
		return normalized
	}
	items, ok := root["items"].([]any)
	if !ok {
		return normalized
	}

	cont := ""
	remaining := -1
	if meta, ok := root["metadata"].(map[string]any); ok {
		cont, _ = meta["continue"].(string)
		if raw, ok := meta["remainingItemCount"].(float64); ok {
			remaining = int(raw)
		}
	}

	root["limit"] = limit
	root["hasMore"] = cont != ""
	switch {
	case cont == "":
		root["total"] = len(items)
	case remaining >= 0:
		root["total"] = len(items) + remaining
	}
	return root
}

const (
	jobDisplayNameAnnotation = "rlark.io/display-name"
	maxJobNameLength         = 50
)

func prepareJobForCreate(body []byte) ([]byte, error) {
	var job rlarkiov1alpha1.Job
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, err
	}

	displayName := strings.TrimSpace(job.Annotations[jobDisplayNameAnnotation])
	if displayName == "" {
		displayName = job.Name
	}
	if len(displayName) > maxJobNameLength {
		return nil, fmt.Errorf("job name cannot exceed %d characters", maxJobNameLength)
	}
	if job.Annotations == nil {
		job.Annotations = make(map[string]string)
	}
	if displayName != "" {
		job.Annotations[jobDisplayNameAnnotation] = displayName
	}
	job.Name = "jo-" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	job.GenerateName = ""

	return json.Marshal(&job)
}

func prepareTenantResourceForCreate(body []byte, tenantID, createdBy string) ([]byte, error) {
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, err
	}
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
	labels[tenant.IDLabel] = tenantID
	annotations, _ := metadata["annotations"].(map[string]any)
	if annotations == nil {
		annotations = map[string]any{}
		metadata["annotations"] = annotations
	}
	annotations[tenant.CreatedByAnnotation] = createdBy
	return json.Marshal(object)
}

func preserveTenantIdentity(body []byte, existing metav1.Object) ([]byte, error) {
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		return nil, err
	}
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
	labels[tenant.IDLabel] = tenant.IDOf(existing)
	annotations, _ := metadata["annotations"].(map[string]any)
	if annotations == nil {
		annotations = map[string]any{}
		metadata["annotations"] = annotations
	}
	if createdBy := existing.GetAnnotations()[tenant.CreatedByAnnotation]; createdBy != "" {
		annotations[tenant.CreatedByAnnotation] = createdBy
	} else {
		delete(annotations, tenant.CreatedByAnnotation)
	}
	return json.Marshal(object)
}

func isTenantResource(resource string) bool {
	return resource == "workflows" || resource == "jobs" || resource == "tasks"
}

func (g *Gateway) validateTaskCreateNamespace(ctx context.Context, namespace, lang string) error {
	clusters, err := g.kubeClient.RlinfV1alpha1().Clusters().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Clusters for Task namespace %q: %w", namespace, err)
	}
	for _, cluster := range clusters.Items {
		if cluster.Spec.AgentNamespace != namespace {
			continue
		}
		if cluster.Spec.Cordoned || cluster.Spec.Draining || cluster.Spec.Delisted {
			return errors.New(localizedMessage(lang,
				fmt.Sprintf("集群 %q 不接受新任务", cluster.Name),
				fmt.Sprintf("cluster %q does not accept new tasks", cluster.Name)))
		}
	}
	return nil
}

// validateTaskUnsuspendNamespace rejects unsuspending a Task whose Cluster is
// cordoned, draining, or delisted. Unsuspending such a Task would let the data
// plane resume a workload on a cluster that is being taken offline, bypassing
// the scheduling-intent boundary that Cordon, Drain, and Delist establish.
func (g *Gateway) validateTaskUnsuspendNamespace(ctx context.Context, namespace, lang string) error {
	clusters, err := g.kubeClient.RlinfV1alpha1().Clusters().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Clusters for Task namespace %q: %w", namespace, err)
	}
	for _, cluster := range clusters.Items {
		if cluster.Spec.AgentNamespace != namespace {
			continue
		}
		if cluster.Spec.Cordoned || cluster.Spec.Draining || cluster.Spec.Delisted {
			return errors.New(localizedMessage(lang,
				fmt.Sprintf("集群 %q 不接受恢复任务", cluster.Name),
				fmt.Sprintf("cluster %q does not accept unsuspending tasks", cluster.Name)))
		}
	}
	return nil
}

// validateJobPlacement rejects creating or starting a Job when no Node
// selected by a task template can accept new workloads. The filter mirrors
// what the Job controller applies when resolving a Task namespace: cluster
// health, node schedulability (the Schedulable condition), and node
// capability (runtime + workload kind). A Job that the controller would
// never dispatch would otherwise stay Pending forever, so rejecting at the
// create and start entry points turns that silent wait into an immediate
// error.
func (g *Gateway) validateJobPlacement(ctx context.Context, job *rlarkiov1alpha1.Job, lang string) error {
	clusters, err := g.kubeClient.RlinfV1alpha1().Clusters().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("list Clusters for Job %q: %w", job.Name, err)
	}
	clustersByNamespace := make(map[string]*rlarkiov1alpha1.Cluster, len(clusters.Items))
	for i := range clusters.Items {
		clustersByNamespace[clusters.Items[i].Spec.AgentNamespace] = &clusters.Items[i]
	}

	for i := range job.Spec.Tasks {
		template := &job.Spec.Tasks[i]
		if len(template.Placement.NodeSelector) == 0 {
			continue
		}
		// List all Nodes and filter client-side using observedNodeLabels plus the
		// administrator annotations on the management Node CR. metadata.labels on
		// the CR only carries RLark-owned system labels, so Kubernetes
		// LabelSelector cannot match data-plane labels or managed annotations such
		// as gpu-model or rlark.io/node-category-*.
		nodes, err := g.kubeClient.RlinfV1alpha1().Nodes(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("list Nodes for Job %q: %w", job.Name, err)
		}
		selector := template.Placement.NodeSelector
		filtered := nodes.Items[:0]
		for j := range nodes.Items {
			if gatewayNodeMatchesSelector(&nodes.Items[j], selector) {
				filtered = append(filtered, nodes.Items[j])
			}
		}
		nodes.Items = filtered
		if len(nodes.Items) == 0 {
			continue
		}

		var (
			blockedNode    string
			blockedCluster string
			blockedReason  string // "unavailable" | "unschedulable" | "unsupported"
			available      bool
		)
		for j := range nodes.Items {
			node := &nodes.Items[j]
			cluster := clustersByNamespace[node.Namespace]
			if cluster == nil {
				if blockedNode == "" {
					blockedNode = node.Name
					blockedReason = "nocluster"
				}
				continue
			}
			if !clusterAcceptsWorkloads(cluster) {
				if blockedNode == "" {
					blockedNode = node.Name
					blockedCluster = cluster.Name
					blockedReason = "unavailable"
				}
				continue
			}
			if !nodeReadyForScheduling(node.Status.Conditions) {
				if blockedNode == "" {
					blockedNode = node.Name
					blockedReason = "unschedulable"
				}
				continue
			}
			if !nodeSupportsJobTask(node, template) {
				if blockedNode == "" {
					blockedNode = node.Name
					blockedReason = "unsupported"
				}
				continue
			}
			available = true
			break
		}
		if available {
			continue
		}
		switch blockedReason {
		case "nocluster":
			return errors.New(localizedMessage(lang,
				fmt.Sprintf("任务使用的节点 %s 不属于任何可用集群", blockedNode),
				fmt.Sprintf("node %s used by the job does not belong to an available cluster", blockedNode)))
		case "unschedulable":
			return errors.New(localizedMessage(lang,
				fmt.Sprintf("任务使用的节点 %s 当前不可调度", blockedNode),
				fmt.Sprintf("node %s used by the job is currently unschedulable", blockedNode)))
		case "unsupported":
			return errors.New(localizedMessage(lang,
				fmt.Sprintf("任务模板 %q 与节点 %s 的运行时能力不匹配", template.Name, blockedNode),
				fmt.Sprintf("task template %q is not supported by node %s", template.Name, blockedNode)))
		default: // unavailable
			return errors.New(localizedMessage(lang,
				fmt.Sprintf("任务使用的节点 %s 所在集群 %s 不可用或已下架", blockedNode, blockedCluster),
				fmt.Sprintf("the cluster %s of node %s used by the job is unavailable or delisted", blockedCluster, blockedNode)))
		}
	}
	return nil
}

// clusterAcceptsWorkloads mirrors the placement filter the Job controller applies
// when it resolves a Task namespace.
func clusterAcceptsWorkloads(cluster *rlarkiov1alpha1.Cluster) bool {
	if !cluster.DeletionTimestamp.IsZero() || cluster.Spec.Cordoned || cluster.Spec.Draining || cluster.Spec.Delisted {
		return false
	}
	return cluster.Status.Phase == rlarkiov1alpha1.ClusterOnline || cluster.Status.Phase == rlarkiov1alpha1.ClusterDowngraded
}

// gatewayNodeMatchesSelector reports whether a Node's observed labels or
// administrator-maintained annotations satisfy every entry in selector. Each selector value may be a comma-separated list;
// the Node matches if its observed label equals any one of the listed values.
// Uses status.observedNodeLabels rather than metadata.labels because the
// management Node CR only carries RLark-owned system labels in metadata.labels.
func gatewayNodeMatchesSelector(node *rlarkiov1alpha1.Node, selector map[string]string) bool {
	labels := node.Status.ObservedNodeLabels
	if labels == nil {
		labels = map[string]string{}
	}
	for key, rawValue := range selector {
		nodeValue, ok := labels[key]
		if !ok {
			// Administrator metadata such as rlark.io/node-category-* lives on
			// the management Node CR as annotations, not on the data plane.
			nodeValue, ok = node.Annotations[key]
		}
		if !ok {
			return false
		}
		values := strings.Split(rawValue, ",")
		matched := false
		for _, v := range values {
			if v == nodeValue {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// nodeReadyForScheduling reports whether a Node may receive new placement.
// The Node Controller owns this decision via the Schedulable condition, so the
// gateway must read that condition rather than re-deriving it.
func nodeReadyForScheduling(conditions []metav1.Condition) bool {
	for _, c := range conditions {
		if c.Type == rlarkiov1alpha1.ConditionTypeSchedulable {
			return c.Status == metav1.ConditionTrue
		}
	}
	return false
}

// nodeSupportsJobTask mirrors the runtime + workload-kind filter the Job
// controller applies in build.go:nodeSupportsTask. Device capacity is not
// checked here because available capacity changes dynamically and the gateway
// should not fail creation when the controller would simply select another
// Node among the same selector matches.
func nodeSupportsJobTask(node *rlarkiov1alpha1.Node, task *rlarkiov1alpha1.JobTaskTemplate) bool {
	capabilities := node.Status.CapabilitySnapshot
	taskRuntime := string(task.Runtime.Kind)
	taskWorkloadKind := string(task.Workload.Kind)
	if capabilities.Runtime.Type == "" && len(capabilities.Runtime.WorkloadKinds) == 0 {
		// Compatibility: an Agent that has not reported a capability snapshot yet
		// falls back to the declared Node runtime so existing workloads keep
		// scheduling.
		var runtimeType string
		if node.Spec.Runtime.Type != "" {
			runtimeType = string(node.Spec.Runtime.Type)
		} else {
			//nolint:staticcheck // Compatibility: read the deprecated Node field for resources that predate Runtime.Type.
			runtimeType = string(node.Spec.AgentType)
		}
		if runtimeType != "" && runtimeType != taskRuntime {
			return false
		}
		return true
	}
	if string(capabilities.Runtime.Type) != taskRuntime {
		return false
	}
	for _, kind := range capabilities.Runtime.WorkloadKinds {
		if string(kind) == taskWorkloadKind {
			return true
		}
	}
	return false
}

func (g *Gateway) handleKubeCreate(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := g.accessors[resource]
		if !ok {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: "+resource)
			return
		}

		ctx := c.Request.Context()
		namespace := c.Query("namespace")

		if isNamespacedResource(resource) && namespace == "" {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest,
				"namespace query parameter is required for namespaced resource: "+resource)
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
			return
		}
		if !enforceSpecQuotas(c, resource, bodyBytes) {
			return
		}
		if resource == "jobs" {
			bodyBytes, err = prepareJobForCreate(bodyBytes)
			if err != nil {
				respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
				return
			}
			// A Job created running starts immediately, so its placement has to be
			// usable now. A suspended Job is only a template and may be started
			// later, when the placement is validated again.
			var job rlarkiov1alpha1.Job
			if err := json.Unmarshal(bodyBytes, &job); err != nil {
				respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
				return
			}
			if !job.Spec.Suspended {
				if err := g.validateJobPlacement(ctx, &job, requestLanguage(c)); err != nil {
					respondError(c, http.StatusConflict, CodeResourceConflict, err.Error())
					return
				}
			}
		}
		if resource == "upgradejobs" {
			bodyBytes, err = prepareUpgradeJobForCreate(bodyBytes)
			if err != nil {
				respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
				return
			}
		}
		if resource == "tasks" {
			if err := g.validateTaskCreateNamespace(ctx, namespace, requestLanguage(c)); err != nil {
				respondError(c, http.StatusConflict, CodeResourceConflict, err.Error())
				return
			}
		}
		if isTenantResource(resource) {
			scope, ok := g.tenantScopeFor(c, resource)
			if !ok {
				return
			}
			// The tenant is taken from the authenticated identity. An explicit
			// tenant in the body is honoured only inside the caller scope, and
			// a tenant outside it is refused rather than silently rewritten, so
			// that the caller is not told the object was created where asked.
			tenantID := scope.writeTenant()
			if requested := requestedTenantID(bodyBytes); requested != "" {
				if !scope.allows(requested) {
					respondErrorWithDetails(c, http.StatusForbidden, CodeForbidden, "tenant is outside the caller scope", map[string]any{
						"tenant": requested,
					})
					return
				}
				tenantID = requested
			}
			if _, err := g.tenants.Get(ctx, tenantID); err != nil {
				respondErrorWithDetails(c, http.StatusBadRequest, CodeInvalidRequest, err.Error(), map[string]any{
					"tenant": tenantID,
				})
				return
			}
			bodyBytes, err = prepareTenantResourceForCreate(bodyBytes, tenantID, c.GetString(authSubjectKey))
			if err != nil {
				respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
				return
			}
		}

		result, err := a.create(ctx, namespace, bodyBytes)
		if err != nil {
			respondCreateError(c, err)
			return
		}
		if job, ok := result.(*rlarkiov1alpha1.Job); ok {
			g.recordJobImages(job)
		}
		c.JSON(http.StatusCreated, result)
	}
}

// respondCreateError reports a failed write. A body that the typed client cannot
// decode is the caller's fault, so it must not be reported as a server error.
func respondCreateError(c *gin.Context, err error) {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
		return
	}
	respondKubeError(c, err.Error(), err)
}

func (g *Gateway) handleKubeUpdate(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := g.accessors[resource]
		if !ok {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: "+resource)
			return
		}

		ctx := c.Request.Context()
		name := c.Param("name")
		namespace := c.Query("namespace")

		if isNamespacedResource(resource) && namespace == "" {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest,
				"namespace query parameter is required for namespaced resource: "+resource)
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
			return
		}
		var startedJob *rlarkiov1alpha1.Job
		var startedTask *rlarkiov1alpha1.Task
		if isTenantResource(resource) {
			scope, ok := g.tenantScopeFor(c, resource)
			if !ok {
				return
			}
			existing, getErr := a.get(ctx, namespace, name, metav1.GetOptions{})
			if getErr != nil {
				respondKubeError(c, getErr.Error(), getErr)
				return
			}
			metadataObject, ok := any(existing).(metav1.Object)
			if !ok {
				respondError(c, http.StatusInternalServerError, CodeInternalError, "resource metadata is unavailable")
				return
			}
			// Ownership is decided by the stored object, because the body may
			// name any tenant and is normalized away below.
			if !authorizeTenantObject(c, scope, tenant.IDOf(metadataObject)) {
				return
			}
			bodyBytes, err = preserveTenantIdentity(bodyBytes, metadataObject)
			if err != nil {
				respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
				return
			}
			// Reject unsuspending a Task whose Cluster is cordoned, draining, or
			// delisted. The existing Task is fetched above for tenant
			// authorization, so its current Spec.Suspended is authoritative.
			if resource == "tasks" {
				existingTask, ok := any(existing).(*rlarkiov1alpha1.Task)
				if !ok {
					respondError(c, http.StatusInternalServerError, CodeInternalError, "task is unavailable")
					return
				}
				var incoming rlarkiov1alpha1.Task
				if err := json.Unmarshal(bodyBytes, &incoming); err != nil {
					respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
					return
				}
				if existingTask.Spec.Suspended && !incoming.Spec.Suspended {
					if err := g.validateTaskUnsuspendNamespace(ctx, namespace, requestLanguage(c)); err != nil {
						respondError(c, http.StatusConflict, CodeResourceConflict, err.Error())
						return
					}
					startedTask = existingTask
				}
			}
			// Reject starting a Job whose selected Nodes belong to unavailable or
			// delisted Clusters, matching the placement filter the Job controller
			// applies afterwards.
			if resource == "jobs" {
				existingJob, ok := any(existing).(*rlarkiov1alpha1.Job)
				if !ok {
					respondError(c, http.StatusInternalServerError, CodeInternalError, "job is unavailable")
					return
				}
				var incoming rlarkiov1alpha1.Job
				if err := json.Unmarshal(bodyBytes, &incoming); err != nil {
					respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
					return
				}
				if existingJob.Spec.Suspended && !incoming.Spec.Suspended {
					if err := g.validateJobPlacement(ctx, &incoming, requestLanguage(c)); err != nil {
						respondError(c, http.StatusConflict, CodeResourceConflict, err.Error())
						return
					}
					startedJob = existingJob
				}
			}
		}

		if !enforceSpecQuotas(c, resource, bodyBytes) {
			return
		}
		result, err := a.update(ctx, namespace, name, bodyBytes)
		if err != nil {
			respondCreateError(c, err)
			return
		}
		// A Job or Task a cluster or node drain stopped keeps its marker across
		// uncordon and undrain. An explicit unsuspend is the resume action, so
		// release the markers once the stored object accepted the new desired state.
		if startedJob != nil {
			if err := g.clearClusterDrainStopsForJob(ctx, startedJob); err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
				return
			}
			if err := g.clearNodeDrainStopsForJob(ctx, startedJob); err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
				return
			}
		}
		if startedTask != nil {
			if err := clearClusterDrainStop(ctx, g.kubeClient, startedTask); err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
				return
			}
			if err := clearNodeDrainStop(ctx, g.kubeClient, startedTask); err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
				return
			}
		}
		c.JSON(http.StatusOK, result)
	}
}

func (g *Gateway) handleKubePatch(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := g.accessors[resource]
		if !ok {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: "+resource)
			return
		}

		ctx := c.Request.Context()
		name := c.Param("name")
		namespace := c.Query("namespace")

		if isNamespacedResource(resource) && namespace == "" {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest,
				"namespace query parameter is required for namespaced resource: "+resource)
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
			return
		}
		if !g.authorizeTenantObjectByName(c, resource, namespace, name) {
			return
		}
		var startedJob *rlarkiov1alpha1.Job
		var startedTask *rlarkiov1alpha1.Task
		if isTenantResource(resource) {
			var patch map[string]any
			if json.Unmarshal(bodyBytes, &patch) != nil {
				respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "invalid patch body")
				return
			}
			if metadata, ok := patch["metadata"].(map[string]any); ok {
				if labels, ok := metadata["labels"].(map[string]any); ok {
					delete(labels, tenant.IDLabel)
				}
				if annotations, ok := metadata["annotations"].(map[string]any); ok {
					delete(annotations, tenant.CreatedByAnnotation)
				}
			}
			// Reject unsuspending a Task whose Cluster is cordoned, draining, or
			// delisted. A merge patch sets spec.suspended only when the caller
			// intends to change it; fetch the current Task to distinguish an
			// actual unsuspend from a no-op write of the existing value.
			if resource == "tasks" {
				if spec, ok := patch["spec"].(map[string]any); ok {
					if suspended, ok := spec["suspended"].(bool); ok && !suspended {
						existing, getErr := a.get(ctx, namespace, name, metav1.GetOptions{})
						if getErr != nil {
							respondKubeError(c, getErr.Error(), getErr)
							return
						}
						existingTask, ok := any(existing).(*rlarkiov1alpha1.Task)
						if ok && existingTask.Spec.Suspended {
							if err := g.validateTaskUnsuspendNamespace(ctx, namespace, requestLanguage(c)); err != nil {
								respondError(c, http.StatusConflict, CodeResourceConflict, err.Error())
								return
							}
							startedTask = existingTask
						}
					}
				}
			}
			// Reject starting a Job whose selected Nodes belong to unavailable or
			// delisted Clusters. Restart is expressed as this patch, so the guard
			// has to cover it as well as POST /jobs/:name/start.
			if resource == "jobs" {
				if spec, ok := patch["spec"].(map[string]any); ok {
					if suspended, ok := spec["suspended"].(bool); ok && !suspended {
						existing, getErr := a.get(ctx, namespace, name, metav1.GetOptions{})
						if getErr != nil {
							respondKubeError(c, getErr.Error(), getErr)
							return
						}
						existingJob, ok := any(existing).(*rlarkiov1alpha1.Job)
						if ok {
							// A Job whose Tasks a cluster or node drain stopped is not
							// suspended itself, so the markers identify a resume here.
							stoppedByClusterDrain, checkErr := g.jobHasClusterDrainStops(ctx, existingJob)
							if checkErr != nil {
								respondError(c, http.StatusInternalServerError, CodeInternalError, checkErr.Error())
								return
							}
							stoppedByNodeDrain, checkErr := g.jobHasNodeDrainStops(ctx, existingJob)
							if checkErr != nil {
								respondError(c, http.StatusInternalServerError, CodeInternalError, checkErr.Error())
								return
							}
							if existingJob.Spec.Suspended || stoppedByClusterDrain || stoppedByNodeDrain {
								// The patch may change the placement in the same request
								// that starts the Job, so validate the configuration the
								// server will store rather than the one it holds now.
								mergedJob := existingJob.DeepCopy()
								if err := json.Unmarshal(bodyBytes, mergedJob); err != nil {
									respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
									return
								}
								if err := g.validateJobPlacement(ctx, mergedJob, requestLanguage(c)); err != nil {
									respondError(c, http.StatusConflict, CodeResourceConflict, err.Error())
									return
								}
							}
							startedJob = existingJob
						}
					}
				}
			}
			bodyBytes, err = json.Marshal(patch)
			if err != nil {
				respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, err.Error())
				return
			}
		}

		if !enforceSpecQuotas(c, resource, bodyBytes) {
			return
		}
		result, err := a.patch(ctx, namespace, name, bodyBytes)
		if err != nil {
			respondCreateError(c, err)
			return
		}
		// A Job or Task a cluster drain stopped keeps its marker across uncordon.
		// An explicit unsuspend is the resume action, so release the marker once the
		// patch accepted the new desired state.
		if startedJob != nil {
			if err := g.clearClusterDrainStopsForJob(ctx, startedJob); err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
				return
			}
			if err := g.clearNodeDrainStopsForJob(ctx, startedJob); err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
				return
			}
		}
		if startedTask != nil {
			if err := clearClusterDrainStop(ctx, g.kubeClient, startedTask); err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
				return
			}
			if err := clearNodeDrainStop(ctx, g.kubeClient, startedTask); err != nil {
				respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
				return
			}
		}
		c.JSON(http.StatusOK, result)
	}
}

func (g *Gateway) handleKubeDelete(resource string) gin.HandlerFunc {
	return func(c *gin.Context) {
		a, ok := g.accessors[resource]
		if !ok {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: "+resource)
			return
		}

		ctx := c.Request.Context()
		name := c.Param("name")
		namespace := c.Query("namespace")

		if isNamespacedResource(resource) && namespace == "" {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest,
				"namespace query parameter is required for namespaced resource: "+resource)
			return
		}

		if !g.authorizeTenantObjectByName(c, resource, namespace, name) {
			return
		}

		// 删除资源时，先从持久化 Backend 删除（PG 或 Secret）
		if g.persistenceBackend != nil {
			_ = g.persistenceBackend.Delete(ctx, resource, namespace, name)
		}

		if err := a.delete(ctx, namespace, name); err != nil {
			respondKubeError(c, err.Error(), err)
			return
		}
		c.Status(http.StatusOK)
	}
}
