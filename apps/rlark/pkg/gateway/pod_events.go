package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

// handlePodEvents reads kubelet events through the existing agent proxy. This
// gives the UI immediate, Worker-scoped pull state without waiting for a
// node-agent status aggregation cycle.
func (g *Gateway) handlePodEvents(c *gin.Context) {
	ctx := c.Request.Context()
	podCRName := c.Param("name")
	pods, err := g.kubeClient.RlinfV1alpha1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", podCRName),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("list pods: %v", err)})
		return
	}
	if len(pods.Items) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("pod %s not found", podCRName)})
		return
	}

	pod := pods.Items[0]
	agentID, ok := strings.CutPrefix(pod.Namespace, apis.RLarkAgentNamespacePrefix)
	if !ok || pod.Spec.PodNamespace == "" || pod.Spec.PodName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pod does not reference a data-plane pod"})
		return
	}

	podPath := fmt.Sprintf("/api/v1/namespaces/%s/pods/%s",
		url.PathEscape(pod.Spec.PodNamespace), url.PathEscape(pod.Spec.PodName))
	podResp, err := g.proxyKubeRequest(ctx, http.MethodGet, agentID, podPath, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("fetch pod: %v", err)})
		return
	}
	podBody, readErr := io.ReadAll(podResp.Body)
	_ = podResp.Body.Close()
	if readErr != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("read pod: %v", readErr)})
		return
	}
	if podResp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("agent returned HTTP %d", podResp.StatusCode)})
		return
	}
	var dataPlanePod corev1.Pod
	if err := json.Unmarshal(podBody, &dataPlanePod); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("decode pod: %v", err)})
		return
	}

	selector := url.QueryEscape(fmt.Sprintf("involvedObject.kind=Pod,involvedObject.name=%s,involvedObject.uid=%s",
		pod.Spec.PodName, dataPlanePod.UID))
	kubePath := fmt.Sprintf("/api/v1/namespaces/%s/events?fieldSelector=%s",
		url.PathEscape(pod.Spec.PodNamespace), selector)
	resp, err := g.proxyKubeRequest(ctx, http.MethodGet, agentID, kubePath, nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("fetch pod events: %v", err)})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("read pod events: %v", err)})
		return
	}
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("agent returned HTTP %d", resp.StatusCode)})
		return
	}

	var list corev1.EventList
	if err := json.Unmarshal(body, &list); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("decode pod events: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": relevantPodEvents(list.Items, dataPlanePod.UID)})
}

func relevantPodEvents(events []corev1.Event, podUID types.UID) []rlarkv1alpha1.NodeEvent {
	result := make([]rlarkv1alpha1.NodeEvent, 0, len(events))
	for i := range events {
		event := &events[i]
		if event.InvolvedObject.UID != podUID {
			continue
		}
		if event.Type != corev1.EventTypeWarning &&
			(event.Type != corev1.EventTypeNormal ||
				(event.Reason != "Pulling" && event.Reason != "Pulled")) {
			continue
		}
		lastTime := event.LastTimestamp
		if lastTime.IsZero() && !event.EventTime.IsZero() {
			lastTime = metav1.NewTime(event.EventTime.Time)
		}
		result = append(result, rlarkv1alpha1.NodeEvent{
			Type: event.Type, Reason: event.Reason, Message: event.Message,
			LastTime: lastTime, Count: event.Count, Source: event.Source.Component,
			ObjectKind: event.InvolvedObject.Kind, ObjectName: event.InvolvedObject.Name,
		})
	}
	return result
}
