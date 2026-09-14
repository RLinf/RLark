package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
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

	// 从日志后端存储查询
	var fromTime, toTime time.Time
	var timeRangeProvided bool
	if v := c.Query("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			fromTime = t
			timeRangeProvided = true
		}
	}
	if v := c.Query("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			toTime = t
		}
	}

	// Optional filters for backend query
	taskName := c.Query("task")
	podName := c.Query("pod")
	rawQuery := c.Query("query")
	cursor := c.Query("cursor")

	if timeRangeProvided {
		if querier := g.getLogQuerier(ctx); querier != nil {
			result, err := g.queryJobLogsFromBackend(ctx, querier, jobName, fromTime, toTime, taskName, podName, rawQuery, cursor)
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

func (g *Gateway) queryJobLogsFromBackend(ctx context.Context, querier logquery.Querier, jobName string, from, to time.Time, taskName, podName, rawQuery, cursor string) (*logquery.Result, error) {
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
		Raw:    rawQuery,
		From:   from,
		To:     to,
		Limit:  99,
		Labels: labels,
		Cursor: cursor,
	})
}

func (g *Gateway) serveJobPodLogs(c *gin.Context, jobName string) {
	logger := log.FromContext(c.Request.Context())
	ctx := c.Request.Context()

	tasks, err := g.kubeClient.RlinfV1alpha1().Tasks(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("rlinf.io/job=%s", jobName),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("list tasks: %v", err)})
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
