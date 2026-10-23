package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/logquery"
)

const (
	// defaultLogQueryLimit is the backend page size used when the caller does
	// not ask for one.
	defaultLogQueryLimit = 99
	// maxLogQueryLimit bounds a single backend log page so that one request
	// cannot ask the backend for an unbounded result set.
	maxLogQueryLimit = 1000
)

// parseLogTimeRange reads the RFC 3339 log window from the query string.
//
// A malformed value must be rejected instead of being dropped: "from" decides
// whether the backend is queried at all, so silently ignoring it would answer a
// historical query with current Pod logs and look like missing data.
func parseLogTimeRange(c *gin.Context) (from, to time.Time, fromProvided bool, err error) {
	if v := c.Query("from"); v != "" {
		parsed, perr := time.Parse(time.RFC3339, v)
		if perr != nil {
			return time.Time{}, time.Time{}, false, invalidParam("from", "must be an RFC 3339 timestamp")
		}
		from = parsed
		fromProvided = true
	}
	if v := c.Query("to"); v != "" {
		parsed, perr := time.Parse(time.RFC3339, v)
		if perr != nil {
			return time.Time{}, time.Time{}, false, invalidParam("to", "must be an RFC 3339 timestamp")
		}
		to = parsed
	}
	if fromProvided && !to.IsZero() && to.Before(from) {
		return time.Time{}, time.Time{}, false, invalidParam("to", "must not be earlier than from")
	}
	return from, to, fromProvided, nil
}

// parseLogLimit reads the backend page size from the query string.
func parseLogLimit(c *gin.Context) (int, error) {
	v := c.Query("limit")
	if v == "" {
		return defaultLogQueryLimit, nil
	}
	n, err := strconv.Atoi(v)
	switch {
	case err != nil:
		return 0, invalidParam("limit", "must be an integer")
	case n <= 0:
		return 0, invalidParam("limit", "must be greater than 0")
	case n > maxLogQueryLimit:
		return 0, invalidParam("limit", fmt.Sprintf("must not exceed %d", maxLogQueryLimit))
	}
	return n, nil
}

type podLogInfo struct {
	TaskName string `json:"taskName"`
	PodName  string `json:"podName"`
	Phase    string `json:"phase"`
	Node     string `json:"node"`
	Logs     string `json:"logs"`
}

type logQuerierCache struct {
	mu          sync.RWMutex
	querier     logquery.Querier
	fingerprint string
}

var gatewayLogQuerierCache = &logQuerierCache{}

func (c *logQuerierCache) get(fingerprint string) logquery.Querier {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.fingerprint == fingerprint {
		return c.querier
	}
	return nil
}

func (c *logQuerierCache) put(fingerprint string, q logquery.Querier) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fingerprint = fingerprint
	c.querier = q
}

func (g *Gateway) getLogQuerier(ctx context.Context) logquery.Querier {
	secret, err := g.getSystemConfigSecret(ctx)
	if err != nil {
		return nil
	}
	cfg := readSystemConfig(secret)
	if cfg.Log == nil || cfg.Log.Backend == "" || cfg.Log.Backend == "none" {
		return nil
	}

	fingerprint := fmt.Sprintf("%s/%v", cfg.Log.Backend, cfg.Log.Config)
	if q := gatewayLogQuerierCache.get(fingerprint); q != nil {
		return q
	}

	q, err := logquery.NewQuerier(cfg.Log)
	if err != nil {
		return nil
	}
	gatewayLogQuerierCache.put(fingerprint, q)
	return q
}

func (g *Gateway) rlinfv1alpha1JobLogs(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()
	jobName := c.Param("name")

	// Logs are derived from the Job, so they follow the tenant boundary of the
	// Job object. The check reads the stored Job because neither the path nor
	// the query carries a tenant identity. Job is cluster scoped, so the
	// lookup namespace is empty.
	if !g.authorizeTenantObjectByName(c, "jobs", "", jobName) {
		return
	}

	// 从日志后端存储查询
	fromTime, toTime, timeRangeProvided, err := parseLogTimeRange(c)
	if err != nil {
		respondInvalidQueryParam(c, err)
		return
	}
	limit, err := parseLogLimit(c)
	if err != nil {
		respondInvalidQueryParam(c, err)
		return
	}

	// Optional filters for backend query
	taskName := c.Query("task")
	podName := c.Query("pod")
	rawQuery := c.Query("query")
	cursor := c.Query("cursor")
	reverse := c.Query("order") != "asc"

	if timeRangeProvided {
		if querier := g.getLogQuerier(ctx); querier != nil {
			result, err := g.queryJobLogsFromBackend(ctx, querier, jobName, fromTime, toTime, taskName, podName, rawQuery, cursor, reverse, limit)
			if err == nil {
				c.JSON(http.StatusOK, gin.H{
					"source":     "backend",
					"entries":    result.Entries,
					"hasMore":    result.HasMore,
					"nextCursor": result.NextCursor,
				})
				return
			}
			logger.Error(err, "log backend query failed, falling back to pod logs", "job", jobName)
		}
	}

	// Fallback: 读 pod 日志
	g.serveJobPodLogs(c, jobName)
}

// rlinfv1alpha1JobLogLabelValues 返回指定 Job 在指定时间范围内，某个标签（如 pod）的所有唯一值。
// 用于前端 Worker 下拉框，支持查询已停止任务的历史日志。
func (g *Gateway) rlinfv1alpha1JobLogLabelValues(c *gin.Context) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()
	jobName := c.Param("name")

	// The label vocabulary describes the Job logs, so it follows the same
	// tenant boundary as the log entries themselves.
	if !g.authorizeTenantObjectByName(c, "jobs", "", jobName) {
		return
	}

	// 时间范围参数
	fromTime, toTime, _, err := parseLogTimeRange(c)
	if err != nil {
		respondInvalidQueryParam(c, err)
		return
	}

	// 标签名参数，默认为 "pod"
	label := c.Query("label")
	if label == "" {
		label = "pod"
	}

	// 可选过滤参数
	taskName := c.Query("task")
	podName := c.Query("pod")

	// 构建过滤器
	filters := map[string]string{}
	if taskName != "" {
		filters["task"] = taskName
	}
	if podName != "" {
		filters["pod"] = podName
	}

	// 获取日志查询器
	querier := g.getLogQuerier(ctx)
	if querier == nil {
		c.JSON(http.StatusOK, gin.H{"values": []string{}})
		return
	}

	// 调用 LabelValues 接口
	values, err := querier.LabelValues(ctx, label, fromTime, toTime, filters)
	if err != nil {
		logger.Error(err, "failed to query label values", "job", jobName, "label", label)
		respondError(c, http.StatusInternalServerError, CodeUpstreamUnavailable, fmt.Sprintf("failed to query label values: %v", err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"values": values})
}

func (g *Gateway) queryJobLogsFromBackend(ctx context.Context, querier logquery.Querier, jobName string, from, to time.Time, taskName, podName, rawQuery, cursor string, reverse bool, limit int) (*logquery.Result, error) {
	if to.IsZero() {
		to = time.Now()
	}

	labels := map[string]string{}
	if taskName != "" {
		labels["task"] = taskName
	}

	if podName != "" {
		labels["pod"] = podName
	}

	// Default to only show main container logs, filter out sidecars
	labels["container"] = "main"

	return querier.Query(ctx, logquery.Query{
		Raw:     rawQuery,
		From:    from,
		To:      to,
		Limit:   limit,
		Labels:  labels,
		Cursor:  cursor,
		Reverse: reverse,
	})
}

func (g *Gateway) serveJobPodLogs(c *gin.Context, jobName string) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	tasks, err := g.kubeClient.RlinfV1alpha1().Tasks(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("rlinf.io/job=%s", jobName),
	})
	if err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, fmt.Sprintf("list tasks: %v", err))
		return
	}

	var results []podLogInfo
	for _, task := range tasks.Items {
		taskNS := task.Namespace
		agentID, ok := strings.CutPrefix(taskNS, apis.RLarkAgentNamespacePrefix)
		if !ok {
			logger.Info("task namespace is not an agent namespace, skipping", "namespace", taskNS, "task", task.Name)
			continue
		}

		podList, err := g.kubeClient.RlinfV1alpha1().Pods(taskNS).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", rlarkv1alpha1.PodLabelTaskName, task.Name),
		})
		if err != nil {
			logger.Error(err, "failed to list pods for task", "task", task.Name, "namespace", taskNS)
			continue
		}

		for _, pod := range podList.Items {
			info := podLogInfo{
				TaskName: task.Name,
				PodName:  pod.Spec.PodName,
				Phase:    string(pod.Status.Phase),
				Node:     pod.Status.Node,
			}
			if pod.Status.Phase == rlarkv1alpha1.PodPhaseRunning || pod.Status.Phase == rlarkv1alpha1.PodPhaseSucceeded || pod.Status.Phase == rlarkv1alpha1.PodPhaseFailed {
				logs, err := g.fetchPodLogs(ctx, agentID, pod.Spec.PodNamespace, pod.Spec.PodName)
				if err != nil {
					logger.Error(err, "failed to fetch pod logs", "agentID", agentID, "pod", pod.Spec.PodName)
					info.Logs = fmt.Sprintf("error fetching logs: %v", err)
				} else {
					info.Logs = logs
				}
			} else {
				info.Logs = fmt.Sprintf("pod is %s, no logs available", pod.Status.Phase)
			}
			results = append(results, info)
		}
	}

	c.JSON(http.StatusOK, gin.H{"source": "pod", "pods": results})
}

func (g *Gateway) fetchPodLogs(ctx context.Context, agentID, podNamespace, podName string) (string, error) {
	if g.config.ServerAddress == "" {
		return "", fmt.Errorf("server-address is not configured")
	}

	httpClient := &http.Client{Transport: g.serverTransport}
	logsURL := fmt.Sprintf("%s/api/proxy/%s/api/kubernetes/api/v1/namespaces/%s/pods/%s/log?tailLines=1000&container=main",
		strings.TrimSuffix(g.config.ServerAddress, "/"),
		agentID,
		podNamespace,
		podName,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, logsURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call server proxy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read log response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server proxy returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}
