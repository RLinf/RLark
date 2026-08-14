package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	rlarkiov1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "task name is required"})
		return
	}

	if g.config.ServerAddress == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "server-address is not configured"})
		return
	}

	podName, err := g.findPodLocalNameForTask(c.Request.Context(), taskName)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	// Rewrite the request path so the server's podproxy handler receives
	// /api/podproxy/<localPodName>:<port>/<original-path>.
	path := c.Param("path")
	c.Request.URL.Path = "/api/podproxy/" + podName + ":" + tensorBoardPort + path

	serverURL, err := url.Parse(g.config.ServerAddress)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("parse server address: %v", err)})
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

// findPodLocalNameForTask lists the rlark Pod CRs associated with the given task
// (via the rlark.io/task-name label) and returns the local data-plane pod name
// (spec.podName) of the first matching running pod. It reads from the pod
// informer cache rather than issuing a direct API server List on every call.
func (g *Gateway) findPodLocalNameForTask(ctx context.Context, taskName string) (string, error) {
	selector := labels.SelectorFromSet(labels.Set{
		rlarkiov1alpha1.PodLabelTaskName: taskName,
	})
	pods, err := g.podLister.List(selector)
	if err != nil {
		return "", fmt.Errorf("list pods for task %s: %w", taskName, err)
	}
	if len(pods) == 0 {
		return "", fmt.Errorf("no pod found for task %s", taskName)
	}
	for _, pod := range pods {
		if pod.Spec.PodName == "" {
			continue
		}
		if pod.Status.Phase == rlarkiov1alpha1.PodPhaseRunning {
			return pod.Spec.PodName, nil
		}
	}
	// Fall back to the first pod with a local name even if not Running yet.
	for _, pod := range pods {
		if pod.Spec.PodName != "" {
			return pod.Spec.PodName, nil
		}
	}
	return "", fmt.Errorf("no ready pod found for task %s", taskName)
}

// taskTensorBoardProxyPath returns the relative gateway URL that proxies to the
// TensorBoard service of the given task.
func taskTensorBoardProxyPath(taskName string) string {
	return "/api/v1/rlinf.io/v1alpha1/tasks/" + taskName + "/tensorboard/"
}

// injectTensorBoardProxy sets status.tensorBoardProxy on the task when
// spec.tensorBoardDir is configured.
func injectTensorBoardProxy(task *rlarkiov1alpha1.Task) {
	if task == nil {
		return
	}
	if task.Spec.TensorBoardDir == nil || *task.Spec.TensorBoardDir == "" {
		return
	}
	task.Status.TensorBoardProxy = taskTensorBoardProxyPath(task.Name)
}

// injectTensorBoardProxyIntoMap sets status.tensorBoardProxy on a DB-backed
// task representation (map[string]any) when spec.tensorBoardDir is configured.
func injectTensorBoardProxyIntoMap(item map[string]any) {
	if item == nil {
		return
	}
	spec, ok := item["spec"].(map[string]any)
	if !ok {
		return
	}
	tbd, _ := spec["tensorBoardDir"].(string)
	if tbd == "" {
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "resource not configured"})
		return
	}
	opts := g.parseListOptions(c)
	result, err := store.List(c.Request.Context(), opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "resource not configured"})
		return
	}
	name := c.Param("name")
	namespace := c.Query("namespace")
	obj, err := store.Get(c.Request.Context(), namespace, name)
	if err != nil {
		if strings.Contains(err.Error(), "no rows") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	injectTensorBoardProxyIntoMap(obj)
	c.JSON(http.StatusOK, obj)
}

// --- K8s-backed handlers ---

func (g *Gateway) listTasksKubeWithProxy(c *gin.Context) {
	a, ok := g.accessors["tasks"]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: tasks"})
		return
	}

	ctx := c.Request.Context()
	namespace := c.Query("namespace")

	opts := metav1.ListOptions{}
	if limit := c.Query("limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			opts.Limit = int64(n)
		}
	}
	if cont := c.Query("continue"); cont != "" {
		opts.Continue = cont
	}
	if fs := c.Query("fieldSelector"); fs != "" {
		opts.FieldSelector = fs
	}
	if ls := c.Query("labelSelector"); ls != "" {
		opts.LabelSelector = ls
	}

	result, err := a.list(ctx, namespace, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if taskList, ok := result.(*rlarkiov1alpha1.TaskList); ok {
		for i := range taskList.Items {
			injectTensorBoardProxy(&taskList.Items[i])
		}
	}
	c.JSON(http.StatusOK, result)
}

func (g *Gateway) getTaskKubeWithProxy(c *gin.Context) {
	a, ok := g.accessors["tasks"]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown resource: tasks"})
		return
	}

	ctx := c.Request.Context()
	name := c.Param("name")
	namespace := c.Query("namespace")

	result, err := a.get(ctx, namespace, name, metav1.GetOptions{})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	if task, ok := result.(*rlarkiov1alpha1.Task); ok {
		injectTensorBoardProxy(task)
	}
	c.JSON(http.StatusOK, result)
}
