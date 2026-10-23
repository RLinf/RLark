package gateway

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	taskapi "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/api"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// tensorBoardPort is the default TensorBoard listening port inside the task pod.
const tensorBoardPort = "6006"

// handleTaskTensorBoardProxy proxies HTTP requests to the TensorBoard service
// (pod:6006) of the task identified by :name. The gateway forwards the request
// to the rlark server's /api/podproxy endpoint, which in turn routes it to the
// target agent and data-plane pod.
func (g *Gateway) handleTaskTensorBoardProxy(c *gin.Context) {
	taskName := c.Param("name")
	if taskName == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidQueryParameter, "task name is required")
		return
	}

	if g.config.ServerAddress == "" {
		respondError(c, http.StatusServiceUnavailable, CodeNotConfigured, "server-address is not configured")
		return
	}

	pod, err := g.findPodForTask(c.Request.Context(), taskName)
	if err != nil {
		respondError(c, http.StatusServiceUnavailable, CodeUpstreamUnavailable, err.Error())
		return
	}

	// The tenant boundary is taken from the Pod that serves TensorBoard, which
	// inherits the identity of its Task. Checking it here keeps a refusal an
	// ordinary HTTP response instead of a proxied one.
	scope, scopeOK := g.tenantScopeFor(c, "pods")
	if !scopeOK {
		return
	}
	if !authorizeTenantObject(c, scope, tenant.IDOf(pod)) {
		return
	}

	// Rewrite the request path so the server's podproxy handler receives
	// /api/podproxy/<localPodName>:<port>/<original-path>.
	path := c.Param("path")
	c.Request.URL.Path = "/api/podproxy/" + pod.Spec.PodName + ":" + tensorBoardPort + path

	serverURL, err := url.Parse(g.config.ServerAddress)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("parse server address: %v", err))
		return
	}

	// proxyPrefix is the gateway URL path (without trailing slash) that maps
	// to TensorBoard's root. Used to rewrite absolute paths (href="/static/...",
	// src="/data/...", url(/...), etc.) in TensorBoard's HTML/CSS/JS responses
	// so the browser requests them through the proxy instead of the site root.
	proxyPrefix := strings.TrimSuffix(taskTensorBoardProxyPath(taskName), "/")

	// Strip Accept-Encoding so the upstream sends uncompressed content,
	// allowing us to rewrite the response body. Use Rewrite (rather than
	// the deprecated Director) to forward requests to the server and drop
	// Accept-Encoding.
	proxy := &httputil.ReverseProxy{
		Transport: g.serverTransport,
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(serverURL)
			pr.Out.Header.Del("Accept-Encoding")
		},
		// Rewrite absolute paths in HTML/CSS/JS responses and redirect
		// Location headers so resources are served through the proxy prefix.
		ModifyResponse: func(resp *http.Response) error {
			return rewriteTensorBoardResponse(resp, proxyPrefix)
		},
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

// findPodForTask lists the rlark Pod CRs associated with the given task
// (via the rlark.io/task-name label) and returns the first matching running pod
// that already has a local data-plane name (spec.podName). It reads from the pod
// informer cache rather than issuing a direct API server List on every call.
func (g *Gateway) findPodForTask(ctx context.Context, taskName string) (*rlarkiov1alpha1.Pod, error) {
	selector := labels.SelectorFromSet(labels.Set{
		rlarkiov1alpha1.PodLabelTaskName: taskName,
	})
	pods, err := g.podLister.List(selector)
	if err != nil {
		return nil, fmt.Errorf("list pods for task %s: %w", taskName, err)
	}
	if len(pods) == 0 {
		return nil, fmt.Errorf("no pod found for task %s", taskName)
	}
	for _, pod := range pods {
		if pod.Spec.PodName == "" {
			continue
		}
		if pod.Status.Phase == rlarkiov1alpha1.PodPhaseRunning {
			return pod, nil
		}
	}
	// Fall back to the first pod with a local name even if not Running yet.
	for _, pod := range pods {
		if pod.Spec.PodName != "" {
			return pod, nil
		}
	}
	return nil, fmt.Errorf("no ready pod found for task %s", taskName)
}

// taskTensorBoardProxyPath returns the relative gateway URL that proxies to the
// TensorBoard service of the given task.
func taskTensorBoardProxyPath(taskName string) string {
	return "/api/v1/rlinf.io/v1alpha1/tasks/" + taskName + "/tensorboard/"
}

// injectTensorBoardProxy sets status.tensorBoardProxy when a valid TensorBoard
// environment with a non-empty log directory is configured.
func injectTensorBoardProxy(task *rlarkiov1alpha1.Task) {
	if task == nil {
		return
	}
	if !hasTensorBoardEnvironment(task.Spec.Environments) {
		return
	}
	task.Status.TensorBoardProxy = taskTensorBoardProxyPath(task.Name)
}

func hasTensorBoardEnvironment(environments []rlarkiov1alpha1.TaskEnvironment) bool {
	for _, environment := range environments {
		if environment.Type != taskapi.EnvironmentTypeTensorBoard || environment.Version != taskapi.EnvironmentVersionTensorBoardV1 {
			continue
		}
		parameters, err := taskapi.DecodeParametersStrict[taskapi.TensorBoardEnvironmentParametersV1](environment.Parameters)
		if err == nil && parameters.LogDir != "" {
			return true
		}
	}
	return false
}

// injectTensorBoardProxyIntoMap sets status.tensorBoardProxy on a DB-backed
// task representation (map[string]any) when a TensorBoard environment is configured.
func injectTensorBoardProxyIntoMap(item map[string]any) {
	if item == nil {
		return
	}
	spec, ok := item["spec"].(map[string]any)
	if !ok {
		return
	}
	environments, ok := spec["environments"].([]any)
	if !ok || !hasTensorBoardEnvironmentMap(environments) {
		return
	}
	meta, ok := item["metadata"].(map[string]any)
	if !ok {
		return
	}
	name, _ := meta["name"].(string)
	if name == "" {
		return
	}
	status, ok := item["status"].(map[string]any)
	if !ok {
		status = make(map[string]any)
		item["status"] = status
	}
	status["tensorBoardProxy"] = taskTensorBoardProxyPath(name)
}

func hasTensorBoardEnvironmentMap(environments []any) bool {
	for _, value := range environments {
		environment, ok := value.(map[string]any)
		if !ok || environment["type"] != taskapi.EnvironmentTypeTensorBoard || environment["version"] != taskapi.EnvironmentVersionTensorBoardV1 {
			continue
		}
		parameters, ok := environment["parameters"].(map[string]any)
		if !ok {
			continue
		}
		if logDir, _ := parameters["logDir"].(string); logDir != "" {
			return true
		}
	}
	return false
}

// --- Task list/get handlers with tensorBoardProxy injection ---

func (g *Gateway) rlinfv1alpha1ListTasksWithProxy(c *gin.Context) {
	if g.dbClient != nil {
		g.listTasksDBWithProxy(c)
		return
	}
	g.listTasksKubeWithProxy(c)
}

func (g *Gateway) rlinfv1alpha1GetTaskWithProxy(c *gin.Context) {
	if g.dbClient != nil {
		g.getTaskDBWithProxy(c)
		return
	}
	g.getTaskKubeWithProxy(c)
}

// --- DB-backed handlers ---

func (g *Gateway) listTasksDBWithProxy(c *gin.Context) {
	store, ok := g.stores["tasks"]
	if !ok {
		respondError(c, http.StatusInternalServerError, CodeNotConfigured, "resource not configured")
		return
	}
	scope, ok := g.tenantScopeFor(c, "tasks")
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
	result, err := store.List(c.Request.Context(), opts)
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	for i := range result.Items {
		injectTensorBoardProxyIntoMap(result.Items[i])
	}
	c.JSON(http.StatusOK, result)
}

func (g *Gateway) getTaskDBWithProxy(c *gin.Context) {
	store, ok := g.stores["tasks"]
	if !ok {
		respondError(c, http.StatusInternalServerError, CodeNotConfigured, "resource not configured")
		return
	}
	scope, ok := g.tenantScopeFor(c, "tasks")
	if !ok {
		return
	}
	name := c.Param("name")
	namespace := c.Query("namespace")
	obj, err := store.Get(c.Request.Context(), namespace, name)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			respondError(c, http.StatusNotFound, CodeResourceNotFound, "not found")
			return
		}
		respondError(c, http.StatusInternalServerError, CodeInternalError, err.Error())
		return
	}
	if !authorizeTenantObject(c, scope, tenantIDOfMap(obj)) {
		return
	}
	injectTensorBoardProxyIntoMap(obj)
	c.JSON(http.StatusOK, obj)
}

// --- K8s-backed handlers ---

func (g *Gateway) listTasksKubeWithProxy(c *gin.Context) {
	a, ok := g.accessors["tasks"]
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: tasks")
		return
	}

	ctx := c.Request.Context()
	namespace := c.Query("namespace")

	scope, ok := g.tenantScopeFor(c, "tasks")
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

	result, err := a.list(ctx, namespace, opts)
	if err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}

	if err := scope.restrictKubeList(result); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to apply tenant scope")
		return
	}

	if taskList, ok := result.(*rlarkiov1alpha1.TaskList); ok {
		for i := range taskList.Items {
			injectTensorBoardProxy(&taskList.Items[i])
		}
	}
	c.JSON(http.StatusOK, withListEnvelope(normalizedTenantObject(result), int(opts.Limit)))
}

func (g *Gateway) getTaskKubeWithProxy(c *gin.Context) {
	a, ok := g.accessors["tasks"]
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "unknown resource: tasks")
		return
	}

	ctx := c.Request.Context()
	name := c.Param("name")
	namespace := c.Query("namespace")

	scope, ok := g.tenantScopeFor(c, "tasks")
	if !ok {
		return
	}

	result, err := a.get(ctx, namespace, name, metav1.GetOptions{})
	if err != nil {
		// Map the error to its real status instead of reporting every failure as
		// a missing resource; a transport or API error is not a 404.
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

	if task, ok := result.(*rlarkiov1alpha1.Task); ok {
		injectTensorBoardProxy(task)
	}
	c.JSON(http.StatusOK, result)
}
