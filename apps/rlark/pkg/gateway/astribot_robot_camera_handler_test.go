package gateway

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestARobotsCamerasArePassedOnFromItsPodPartByPart(t *testing.T) {
	gin.SetMode(gin.TestMode)
	// The server's proxy to the robot cluster's API server: the node's s1u-robot Pods (one gone, one running) and
	// that Pod's camera service.
	var mu sync.Mutex
	var asked []string
	parts := make(chan string)
	partsBob := make(chan string)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.URL.RequestURI())
		mu.Unlock()
		kube, ok := strings.CutPrefix(r.URL.Path, "/api/proxy/robot/api/kubernetes/api/v1/namespaces/rlark-system/pods")
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch kube {
		case "":
			if r.URL.Query().Get("labelSelector") != "app.kubernetes.io/name=s1u-robot" || r.URL.Query().Get("fieldSelector") != "spec.nodeName=s1u-03-x86" {
				_ = json.NewEncoder(w).Encode(corev1.PodList{})
				return
			}
			_ = json.NewEncoder(w).Encode(corev1.PodList{Items: []corev1.Pod{
				{ObjectMeta: metav1.ObjectMeta{Name: "s1u-robot-old"}, Status: corev1.PodStatus{Phase: corev1.PodFailed}},
				{ObjectMeta: metav1.ObjectMeta{Name: "s1u-robot-x7k2p"}, Status: corev1.PodStatus{Phase: corev1.PodRunning}},
			}})
		case "/s1u-robot-x7k2p:18891/proxy/cameras":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"state":"ok","cameras":{"head":{"state":"ok","fps":30.0}},"updated_at":1795000000.0}`)
		case "/s1u-robot-x7k2p:18891/proxy/cameras/head.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("X-Age-Seconds", "0.04")
			_, _ = io.WriteString(w, "\xff\xd8JPEG\xff\xd9")
		case "/s1u-robot-x7k2p:18891/proxy/cameras/wrist_left.jpg":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"detail": "Not found"}`)
		case "/s1u-robot-x7k2p:18891/proxy/cameras/torso.jpg":
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"detail": "no frame of torso yet"}`)
		case "/s1u-robot-x7k2p:18891/proxy/cameras/head/stream":
			w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=s1ucamera")
			w.(http.Flusher).Flush()
			for part := range parts {
				_, _ = io.WriteString(w, part)
				w.(http.Flusher).Flush()
			}
		case "/s1u-robot-x7k2p:18891/proxy/cameras/wrist_left/stream":
			w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=s1ucamera")
			w.(http.Flusher).Flush()
			for part := range partsBob {
				_, _ = io.WriteString(w, part)
				w.(http.Flusher).Flush()
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	node := &rlarkv1alpha1.Node{ObjectMeta: metav1.ObjectMeta{Namespace: "rlark-robot", Name: "s1u-03-x86"}}
	g := &Gateway{config: Config{ServerAddress: upstream.URL, CameraMaxLiveStreams: 2, CameraMaxLiveStreamsPerUser: 1}, serverTransport: http.DefaultTransport.(*http.Transport),
		accessors: map[string]*resourceAccessor{"nodes": {get: func(_ context.Context, namespace, name string, _ metav1.GetOptions) (any, error) {
			if namespace != node.Namespace || name != node.Name {
				return nil, apierrors.NewNotFound(schema.GroupResource{Group: "rlinf.io", Resource: "nodes"}, name)
			}
			return node, nil
		}}}}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(authSubjectKey, c.GetHeader("X-Test-Auth-Subject"))
		c.Next()
	})
	g.registerRobotRoutes(router.Group("/nodes"))
	gateway := httptest.NewServer(router)
	defer gateway.Close()
	getWithSubject := func(path, user, subject string) *http.Response {
		request, err := http.NewRequest(http.MethodGet, gateway.URL+"/nodes/"+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("X-IAM-User", user)
		request.Header.Set("X-Test-Auth-Subject", subject)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	getAs := func(path, user string) *http.Response {
		return getWithSubject(path, user, "service-account:astribot-iam-admin")
	}
	get := func(path string) *http.Response { return getAs(path, "alice") }
	read := func(response *http.Response) string {
		defer func() { _ = response.Body.Close() }()
		body, _ := io.ReadAll(response.Body)
		return string(body)
	}
	lastAsked := func() string {
		mu.Lock()
		defer mu.Unlock()
		return asked[len(asked)-1]
	}
	count := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(asked)
	}

	// The state, from the node's running Pod; a view's JPEG with its age.
	if response := get("s1u-03-x86/cameras?namespace=rlark-robot"); response.StatusCode != http.StatusOK ||
		response.Header.Get("Content-Type") != "application/json" || !strings.Contains(read(response), `"head":{"state":"ok"`) {
		t.Fatalf("state: %d %v", response.StatusCode, response.Header)
	}
	if response := get("s1u-03-x86/cameras/head.jpg?namespace=rlark-robot"); response.StatusCode != http.StatusOK ||
		response.Header.Get("X-Age-Seconds") != "0.04" || response.Header.Get("Cache-Control") != "no-store" || read(response) != "\xff\xd8JPEG\xff\xd9" {
		t.Fatalf("head.jpg: %d %v", response.StatusCode, response.Header)
	}
	// What the robot refuses keeps its status, with the robot's word as the message.
	for path, want := range map[string]int{"wrist_left.jpg": http.StatusNotFound, "torso.jpg": http.StatusServiceUnavailable} {
		response := get("s1u-03-x86/cameras/" + path + "?namespace=rlark-robot")
		var refused map[string]any
		_ = json.Unmarshal([]byte(read(response)), &refused)
		if response.StatusCode != want || refused["message"] == "" || refused["message"] == nil {
			t.Fatalf("%s: %d %v", path, response.StatusCode, refused)
		}
	}
	// Only the robot's camera paths reach the cluster, and of the query only fps.
	before := count()
	for _, path := range []string{"cameras/stereo.jpg", "cameras/head", "cameras/head.png", "cameras/wall.jpg/stream", "cameras/stereo/stream", "cameras/../../exec"} {
		if response := get("s1u-03-x86/" + path + "?namespace=rlark-robot"); response.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: %d", path, response.StatusCode)
		}
	}
	if count() != before {
		t.Fatalf("asked the cluster %s", lastAsked())
	}
	// No such Node, a namespace that is no cluster's, a node without the Pod.
	for path, want := range map[string]int{
		"s1u-09-x86/cameras?namespace=rlark-robot":          http.StatusNotFound,
		"s1u-03-x86/cameras?namespace=elsewhere":            http.StatusNotFound,
		"s1u-03-x86/cameras/wall.jpg?namespace=rlark-robot": http.StatusNotFound,
	} {
		if response := get(path); response.StatusCode != want {
			t.Fatalf("%s: %d, want %d", path, response.StatusCode, want)
		}
	}

	// A stream: each part reaches the viewer while the robot is still sending, nothing held back.
	response := get("s1u-03-x86/cameras/head/stream?namespace=rlark-robot&fps=15&token=x")
	defer func() { _ = response.Body.Close() }()
	if !strings.HasSuffix(lastAsked(), "/proxy/cameras/head/stream?fps=15") {
		t.Fatalf("asked %s", lastAsked())
	}
	if response.StatusCode != http.StatusOK || response.Header.Get("Content-Type") != "multipart/x-mixed-replace; boundary=s1ucamera" ||
		response.Header.Get("X-Accel-Buffering") != "no" {
		t.Fatalf("stream: %d %v", response.StatusCode, response.Header)
	}
	limited := get("s1u-03-x86/cameras/head/stream?namespace=rlark-robot")
	assertCameraStreamLimited(t, limited, "per-user limit")
	if snapshot := get("s1u-03-x86/cameras/head.jpg?namespace=rlark-robot"); snapshot.StatusCode != http.StatusOK {
		t.Fatalf("snapshot while stream active: %d", snapshot.StatusCode)
	} else {
		_ = snapshot.Body.Close()
	}
	if listing := get("s1u-03-x86/cameras?namespace=rlark-robot"); listing.StatusCode != http.StatusOK {
		t.Fatalf("camera list while stream active: %d", listing.StatusCode)
	} else {
		_ = listing.Body.Close()
	}
	bob := getAs("s1u-03-x86/cameras/wrist_left/stream?namespace=rlark-robot", "bob")
	if bob.StatusCode != http.StatusOK {
		t.Fatalf("second user's stream: %d", bob.StatusCode)
	}
	limited = getAs("s1u-03-x86/cameras/torso/stream?namespace=rlark-robot", "carol")
	assertCameraStreamLimited(t, limited, "global limit")
	reader := bufio.NewReader(response.Body)
	for _, part := range []string{"--s1ucamera\r\nContent-Type: image/jpeg\r\nContent-Length: 4\r\n\r\nJPEG\r\n", "--s1ucamera\r\n"} {
		parts <- part
		got := make([]byte, len(part))
		if _, err := io.ReadFull(reader, got); err != nil || string(got) != part {
			t.Fatalf("part %q: %q %v", part, got, err)
		}
	}
	_ = response.Body.Close()
	_ = bob.Body.Close()
	// Closing downstream streams cancels their proxy requests and releases their slots.
	close(parts)
	close(partsBob)
	var reopened *http.Response
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		reopened = getWithSubject("s1u-03-x86/cameras/head/stream?namespace=rlark-robot", "forged", "alice")
		if reopened.StatusCode == http.StatusOK {
			break
		}
		_ = reopened.Body.Close()
		time.Sleep(10 * time.Millisecond)
	}
	if reopened == nil || reopened.StatusCode != http.StatusOK {
		t.Fatal("camera stream quota was not released after disconnect")
	}
	_ = reopened.Body.Close()
}

func TestCameraStreamOwnerTrustsIAMInjectedUserOnlyForIAMServiceAccounts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	owner := func(subject, iamUser string) string {
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		ctx.Request.RemoteAddr = "192.0.2.8:12345"
		ctx.Request.Header.Set("X-IAM-User", iamUser)
		ctx.Set(authSubjectKey, subject)
		return cameraStreamOwner(ctx)
	}
	iamAlice := owner("service-account:astribot-iam-operator", "alice")
	iamBob := owner("service-account:astribot-iam-operator", "bob")
	if iamAlice != "alice" || iamBob != "bob" {
		t.Fatalf("IAM subjects use owners %q and %q", iamAlice, iamBob)
	}
	ordinary := owner("alice", "bob")
	spoofed := owner("alice", "charlie")
	var limiter cameraStreamLimiter
	if ordinary != "alice" || spoofed != ordinary {
		t.Fatalf("ordinary subject owners differ: %q and %q", ordinary, spoofed)
	}
	if !limiter.acquire(ordinary, 0, 1) {
		t.Fatal("first ordinary stream refused")
	}
	if limiter.acquire(spoofed, 0, 1) {
		t.Fatal("forged X-IAM-User bypassed the ordinary user's quota")
	}
}

func assertCameraStreamLimited(t *testing.T, response *http.Response, label string) {
	t.Helper()
	defer response.Body.Close()
	var body ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("%s response: %v", label, err)
	}
	if response.StatusCode != http.StatusTooManyRequests || response.Header.Get("Retry-After") != "2" || body.Code != "camera_stream_limit_exceeded" {
		t.Fatalf("%s response: HTTP %d retry=%q body=%+v", label, response.StatusCode, response.Header.Get("Retry-After"), body)
	}
}

func TestCameraStreamLimiterEnforcesAndReleasesLimits(t *testing.T) {
	var limiter cameraStreamLimiter
	if !limiter.acquire("alice", 2, 1) {
		t.Fatal("first stream refused")
	}
	if limiter.acquire("alice", 2, 1) {
		t.Fatal("per-user limit was not enforced")
	}
	if !limiter.acquire("bob", 2, 1) {
		t.Fatal("second user stream refused")
	}
	if limiter.acquire("carol", 2, 1) {
		t.Fatal("global limit was not enforced")
	}
	limiter.release("alice")
	if !limiter.acquire("alice", 2, 1) {
		t.Fatal("released slot was not reusable")
	}
}

func TestCameraStreamLimiterConcurrentAcquisitions(t *testing.T) {
	var limiter cameraStreamLimiter
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.acquire("shared", 10, 10) {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if accepted != 10 {
		t.Fatalf("accepted %d concurrent streams, want 10", accepted)
	}
}
