package gateway

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

// A robot's cameras are served by its model Addon (s1u-robot's video container, astribot/s1u/device/cameras.py) on
// the node's address, read-only; the Gateway reaches them through the member cluster's API server, as a proxy of that
// Pod, and decides who may look.
const (
	robotCameraNamespace = "rlark-system"
	robotCameraSelector  = "app.kubernetes.io/name=s1u-robot"
	robotCameraPort      = "18891"
)

// robotCameraViews are the views a robot may have, in the order of its camera wall (head, torso / wrist_left,
// wrist_right); a body has some of them.
var robotCameraViews = []string{"head", "torso", "wrist_left", "wrist_right"}

// robotCameraPath is the camera service's path of a request (with the one query it takes, fps), and whether it is
// one of the robot's: /cameras, /cameras/<view>.jpg, /cameras/<view>/stream, /cameras/wall.jpg, /cameras/wall/stream.
func robotCameraPath(c *gin.Context) (string, bool) {
	camera := c.Param("camera")
	if camera == "" {
		return "/cameras", true
	}
	if !strings.HasSuffix(c.FullPath(), "/stream") {
		view, image := strings.CutSuffix(camera, ".jpg")
		return "/cameras/" + camera, image && (view == "wall" || slices.Contains(robotCameraViews, view))
	}
	path := "/cameras/" + camera + "/stream"
	if fps := c.Query("fps"); fps != "" {
		path += "?" + url.Values{"fps": {fps}}.Encode()
	}
	return path, camera == "wall" || slices.Contains(robotCameraViews, camera)
}

// handleRobotCameras answers GET /api/v1/rlinf.io/v1alpha1/nodes/{name}/cameras[/…]: what the robot's camera
// service answers (its state, a view's newest JPEG or its stream, the wall), passed on as it comes, a stream part by
// part. Anyone signed in may look, as anyone may claim a robot.
func (g *Gateway) handleRobotCameras(c *gin.Context) {
	path, ok := robotCameraPath(c)
	if !ok {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "a robot's cameras are /cameras, /cameras/<view>.jpg, /cameras/<view>/stream, /cameras/wall.jpg and /cameras/wall/stream; the views are "+strings.Join(robotCameraViews, ", "))
		return
	}
	ctx, name, namespace := c.Request.Context(), c.Param("name"), c.Query("namespace")
	if !g.authorizeTenantObjectByName(c, "nodes", namespace, name) {
		return
	}
	if _, err := g.accessors["nodes"].get(ctx, namespace, name, metav1.GetOptions{}); err != nil {
		respondKubeError(c, err.Error(), err)
		return
	}
	agent, ok := strings.CutPrefix(namespace, apis.RLarkAgentNamespacePrefix)
	if !ok {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "namespace is the robot's cluster ("+apis.RLarkAgentNamespacePrefix+"<cluster>)")
		return
	}

	// The node's camera service: its s1u-robot Pod that runs.
	listed, err := g.proxyKubeRequest(ctx, http.MethodGet, agent, fmt.Sprintf("/api/v1/namespaces/%s/pods?%s", robotCameraNamespace,
		url.Values{"labelSelector": {robotCameraSelector}, "fieldSelector": {"spec.nodeName=" + name}}.Encode()), nil)
	if err != nil {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, fmt.Sprintf("the cluster's Pods: %v", err))
		return
	}
	var pods corev1.PodList
	err = json.NewDecoder(listed.Body).Decode(&pods)
	_ = listed.Body.Close()
	if listed.StatusCode != http.StatusOK || err != nil {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, fmt.Sprintf("the cluster's Pods: HTTP %d", listed.StatusCode))
		return
	}
	running := slices.IndexFunc(pods.Items, func(pod corev1.Pod) bool { return pod.Status.Phase == corev1.PodRunning })
	if running < 0 {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "no s1u-robot Pod runs on "+name)
		return
	}

	answer, err := g.proxyKubeRequest(ctx, http.MethodGet, agent, fmt.Sprintf("/api/v1/namespaces/%s/pods/%s:%s/proxy%s",
		robotCameraNamespace, pods.Items[running].Name, robotCameraPort, path), nil)
	if err != nil {
		respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, fmt.Sprintf("the robot's cameras: %v", err))
		return
	}
	defer func() { _ = answer.Body.Close() }()
	if answer.StatusCode != http.StatusOK {
		// The camera service's {"detail"}, or the API server's Status {"message"} when it does not reach the Pod.
		var refused struct{ Detail, Message string }
		body, _ := io.ReadAll(io.LimitReader(answer.Body, 4096))
		_ = json.Unmarshal(body, &refused)
		message := cmp.Or(refused.Detail, refused.Message, strings.TrimSpace(string(body)))
		switch answer.StatusCode {
		case http.StatusBadRequest:
			respondError(c, answer.StatusCode, CodeInvalidRequest, message)
		case http.StatusNotFound:
			respondError(c, answer.StatusCode, CodeResourceNotFound, message)
		case http.StatusServiceUnavailable:
			respondError(c, answer.StatusCode, CodeUpstreamUnavailable, message)
		default:
			respondError(c, http.StatusBadGateway, CodeUpstreamUnavailable, fmt.Sprintf("the robot's cameras: HTTP %d: %s", answer.StatusCode, message))
		}
		return
	}
	for _, header := range []string{"Content-Type", "Content-Length", "X-Age-Seconds"} {
		if value := answer.Header.Get(header); value != "" {
			c.Header(header, value)
		}
	}
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no") // the console's nginx passes each part of a stream on as it comes
	c.Status(http.StatusOK)
	c.Writer.Flush() // the headers at once: a stream's viewer knows it is there before its first part
	buffer := make([]byte, 64<<10)
	for {
		n, err := answer.Body.Read(buffer)
		if n > 0 {
			if _, err := c.Writer.Write(buffer[:n]); err != nil {
				return // the viewer left
			}
			c.Writer.Flush()
		}
		if err != nil {
			return // the end, or the robot's side gone: the viewer sees the stream end
		}
	}
}
