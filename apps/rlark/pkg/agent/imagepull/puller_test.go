package imagepull

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

// newTestPuller builds a Puller wired to a fake runtime and (optionally) a
// fake local Kubernetes client. Workers are NOT started; tests drain p.jobs
// synchronously via drain to keep behavior deterministic.
func newTestPuller(t *testing.T, runtime ImagePuller, agentType rlarkv1alpha1.AgentType, nodeName string, kube *fake.Clientset) *Puller {
	t.Helper()
	p := &Puller{
		log:         testr.New(t),
		runtime:     runtime,
		agentType:   agentType,
		nodeName:    nodeName,
		localKube:   kube,
		states:      make(map[types.UID]*taskState),
		jobs:        make(chan pullJob, jobsBuffer),
		progressMap: make(map[string]*imageProgressState),
	}
	return p
}

// drain processes all queued pull jobs synchronously so tests can assert on
// what was actually pulled.
func drain(t *testing.T, p *Puller) {
	t.Helper()
	for {
		select {
		case job := <-p.jobs:
			p.processJob(context.Background(), job)
		default:
			return
		}
	}
}

func sortedPulled(f *fakeExecPuller) []string {
	out := append([]string(nil), f.pulled...)
	sort.Strings(out)
	return out
}

func TestMatchNodeSelector(t *testing.T) {
	labels := map[string]string{"disktype": "ssd", "arch": "arm64", "zone": "z1"}
	cases := []struct {
		name     string
		labels   map[string]string
		selector map[string]string
		want     bool
	}{
		{"empty selector matches", labels, nil, true},
		{"exact match", labels, map[string]string{"disktype": "ssd"}, true},
		{"exact mismatch", labels, map[string]string{"disktype": "hdd"}, false},
		{"comma In match", labels, map[string]string{"disktype": "hdd,ssd,nvme"}, true},
		{"comma In mismatch", labels, map[string]string{"disktype": "hdd,nvme"}, false},
		{"comma with spaces", labels, map[string]string{"disktype": " hdd , ssd "}, true},
		{"multi-key AND all match", labels, map[string]string{"disktype": "ssd", "arch": "arm64"}, true},
		{"multi-key AND one fails", labels, map[string]string{"disktype": "ssd", "arch": "amd64"}, false},
		{"missing key fails", labels, map[string]string{"gpu": "true"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := matchNodeSelector(c.labels, c.selector); got != c.want {
				t.Errorf("matchNodeSelector(%v, %v) = %v, want %v", c.labels, c.selector, got, c.want)
			}
		})
	}
}

func mkTask(uid, rv string, agentType rlarkv1alpha1.AgentType, images []string, selector map[string]string) *rlarkv1alpha1.Task {
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "t-" + uid,
			Namespace:       "rlark-system",
			UID:             types.UID(uid),
			ResourceVersion: rv,
		},
		Spec: rlarkv1alpha1.TaskSpec{
			AgentType:    agentType,
			NodeSelector: selector,
			Docker:       &rlarkv1alpha1.DockerTaskSpec{},
		},
	}
	for _, img := range images {
		task.Spec.Docker.Containers = append(task.Spec.Docker.Containers, rlarkv1alpha1.DockerContainerSpec{Image: img})
	}
	return task
}

func TestHandle_DockerPullsAllImages(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "", nil)

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine", "redis:7"}, nil))
	drain(t, p)

	if got := sortedPulled(rt); !equalStrings(got, []string{"nginx:alpine", "redis:7"}) {
		t.Errorf("pulled = %v, want [nginx:alpine redis:7]", got)
	}
}

func TestHandle_IdempotentSameResourceVersion(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "", nil)
	task := mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine"}, nil)

	p.handle(task)
	drain(t, p)
	// Second handle with same rv and image already pulled -> no-op.
	p.handle(task)
	drain(t, p)

	if got := sortedPulled(rt); !equalStrings(got, []string{"nginx:alpine"}) {
		t.Errorf("pulled = %v, want single [nginx:alpine]", got)
	}
}

func TestHandle_ResourceVersionChangePullsOnlyNewImages(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "", nil)

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine"}, nil))
	drain(t, p)
	p.handle(mkTask("u1", "2", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine", "redis:7"}, nil))
	drain(t, p)

	if got := sortedPulled(rt); !equalStrings(got, []string{"nginx:alpine", "redis:7"}) {
		t.Errorf("pulled = %v, want [nginx:alpine redis:7]", got)
	}
	// nginx pulled exactly once across both events.
	if count := countOccurrences(rt.pulled, "nginx:alpine"); count != 1 {
		t.Errorf("nginx:alpine pulled %d times, want 1", count)
	}
}

func TestHandle_AgentTypeMismatchSkips(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeKubernetes, "node1", fake.NewSimpleClientset())

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine"}, nil))
	drain(t, p)

	if len(rt.pulled) != 0 {
		t.Errorf("expected no pulls on agent-type mismatch, got %v", rt.pulled)
	}
}

func TestHandle_NoImagesSkips(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "", nil)

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, nil, nil))
	drain(t, p)

	if len(rt.pulled) != 0 {
		t.Errorf("expected no pulls for image-less task, got %v", rt.pulled)
	}
}

func TestHandle_KubernetesNodeSelectorMismatchSkips(t *testing.T) {
	rt := &fakeExecPuller{}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{"disktype": "ssd"}}}
	kube := fake.NewSimpleClientset(node)
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeKubernetes, "node1", kube)

	// Selector asks for hdd; node is ssd -> skip.
	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeKubernetes, []string{"nginx:alpine"}, map[string]string{"disktype": "hdd"}))
	drain(t, p)

	if len(rt.pulled) != 0 {
		t.Errorf("expected no pulls when node selector mismatches, got %v", rt.pulled)
	}
}

func TestHandle_KubernetesNodeSelectorMatchPulls(t *testing.T) {
	rt := &fakeExecPuller{}
	node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1", Labels: map[string]string{"disktype": "ssd"}}}
	kube := fake.NewSimpleClientset(node)
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeKubernetes, "node1", kube)

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeKubernetes, []string{"nginx:alpine"}, map[string]string{"disktype": "ssd"}))
	drain(t, p)

	if got := sortedPulled(rt); !equalStrings(got, []string{"nginx:alpine"}) {
		t.Errorf("expected pull on node-selector match, got %v", got)
	}
}

func TestHandle_KubernetesEmptyNodeNamePullsAllWithWarning(t *testing.T) {
	rt := &fakeExecPuller{}
	// No local kube client / node name: falls back to pull-all.
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeKubernetes, "", nil)

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeKubernetes, []string{"nginx:alpine"}, map[string]string{"disktype": "ssd"}))
	drain(t, p)

	if got := sortedPulled(rt); !equalStrings(got, []string{"nginx:alpine"}) {
		t.Errorf("expected pull-all fallback when node name empty, got %v", got)
	}
}

func TestHandle_ImageExistsShortCircuitsPull(t *testing.T) {
	rt := &fakeExecPuller{exists: map[string]bool{"nginx:alpine": true}}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "", nil)

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine"}, nil))
	drain(t, p)

	if len(rt.pulled) != 0 {
		t.Errorf("expected no pull when image already exists, got %v", rt.pulled)
	}
	// State should still record it as pulled so subsequent events skip.
	if !pulledFor(t, p, "u1", "nginx:alpine") {
		t.Errorf("expected image marked pulled in task state")
	}
}

func TestHandle_PullFailureDoesNotMarkPulled(t *testing.T) {
	rt := &fakeExecPuller{pullFn: func(string) error { return context.DeadlineExceeded }}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "", nil)

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine"}, nil))
	drain(t, p)

	if pulledFor(t, p, "u1", "nginx:alpine") {
		t.Errorf("expected image NOT marked pulled on failure")
	}
}

func TestDeleteClearsState(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "", nil)
	task := mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine"}, nil)

	p.handle(task)
	drain(t, p)
	p.onDelete(task)

	// After delete, re-handling dispatches again (state was cleared).
	p.handle(task)
	drain(t, p)

	if count := countOccurrences(rt.pulled, "nginx:alpine"); count != 2 {
		t.Errorf("expected image pulled twice (once before delete, once after), got %d", count)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "", nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	// Give workers a moment to start, then cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}
}

// --- helpers ---

func pulledFor(t *testing.T, p *Puller, uid, image string) bool {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.states[types.UID(uid)]
	if !ok {
		return false
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	_, exists := st.pulled[image]
	return exists
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func countOccurrences(s []string, v string) int {
	n := 0
	for _, x := range s {
		if x == v {
			n++
		}
	}
	return n
}

// --- progress patcher tests ---

type fakeProgressPatcher struct {
	calls int
	err   error
}

func (f *fakeProgressPatcher) PatchPullProgress(ctx context.Context, nodeName, namespace string, progresses []rlarkv1alpha1.PullProgress) error {
	f.calls++
	return f.err
}

func TestBuildPullProgressMergePatch(t *testing.T) {
	progresses := []rlarkv1alpha1.PullProgress{
		{Image: "nginx:alpine", Downloaded: 1024, Total: 2048, Speed: 100, Status: "pulling"},
	}
	patch, err := buildPullProgressMergePatch(progresses)
	if err != nil {
		t.Fatalf("buildPullProgressMergePatch returned error: %v", err)
	}
	if len(patch) == 0 {
		t.Error("expected non-empty patch")
	}
}

func TestProgressReporterStoring(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "node1", nil)
	p.patcher = &fakeProgressPatcher{}
	p.mgmtNS = "test-ns"

	reporter := p.makeProgressReporter(context.Background(), "nginx:alpine")
	reporter(Progress{Image: "nginx:alpine", Downloaded: 512, Total: 1024, Speed: 50, Status: "pulling"})

	p.progressMu.Lock()
	state, ok := p.progressMap["nginx:alpine"]
	p.progressMu.Unlock()
	if !ok {
		t.Fatal("expected progress state to be stored")
	}
	if state.downloaded != 512 {
		t.Errorf("downloaded = %d, want 512", state.downloaded)
	}
	if state.total != 1024 {
		t.Errorf("total = %d, want 1024", state.total)
	}
}

func TestProgressReporterCompletionTriggersReport(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "node1", nil)
	fakePatcher := &fakeProgressPatcher{}
	p.patcher = fakePatcher
	p.mgmtNS = "test-ns"

	reporter := p.makeProgressReporter(context.Background(), "nginx:alpine")
	reporter(Progress{Image: "nginx:alpine", Status: "completed"})

	if fakePatcher.calls < 1 {
		t.Error("expected progress report to be called on completion")
	}
}

func TestProgressReporterNoPatcherNoReport(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "node1", nil)
	// patcher is nil, nodeName is set -> should not panic

	reporter := p.makeProgressReporter(context.Background(), "nginx:alpine")
	reporter(Progress{Image: "nginx:alpine", Status: "completed"})
	// should not panic
}

func TestProgressReporterNoNodeNameNoReport(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "", nil)
	fakePatcher := &fakeProgressPatcher{}
	p.patcher = fakePatcher
	p.mgmtNS = "test-ns"

	reporter := p.makeProgressReporter(context.Background(), "nginx:alpine")
	reporter(Progress{Image: "nginx:alpine", Status: "completed"})

	if fakePatcher.calls != 0 {
		t.Error("expected no report when nodeName is empty")
	}
}

func TestProgressReporterChangeDetection(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "node1", nil)
	fakePatcher := &fakeProgressPatcher{}
	p.patcher = fakePatcher
	p.mgmtNS = "test-ns"

	reporter := p.makeProgressReporter(context.Background(), "nginx:alpine")

	// First report: should always report
	reporter(Progress{Image: "nginx:alpine", Downloaded: 100, Total: 1000, Speed: 10, Status: "pulling", Message: "fetching: 10.0%"})
	if fakePatcher.calls != 1 {
		t.Errorf("expected 1 call, got %d", fakePatcher.calls)
	}

	// Same data: should not report (no change)
	reporter(Progress{Image: "nginx:alpine", Downloaded: 100, Total: 1000, Speed: 10, Status: "pulling", Message: "fetching: 10.0%"})
	if fakePatcher.calls != 1 {
		t.Errorf("expected still 1 call (no change), got %d", fakePatcher.calls)
	}

	// Changed data but throttle not elapsed: should not report yet
	reporter(Progress{Image: "nginx:alpine", Downloaded: 200, Total: 1000, Speed: 20, Status: "pulling", Message: "fetching: 20.0%"})
	if fakePatcher.calls != 1 {
		t.Errorf("expected still 1 call (throttle not elapsed), got %d", fakePatcher.calls)
	}

	// Status change is a terminal state transition: always reports
	reporter(Progress{Image: "nginx:alpine", Status: "failed", Message: "pull failed"})
	if fakePatcher.calls != 2 {
		t.Errorf("expected 2 calls (terminal always reports), got %d", fakePatcher.calls)
	}
}

func TestProgressReporterThrottleAllowsChange(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "node1", nil)
	fakePatcher := &fakeProgressPatcher{}
	p.patcher = fakePatcher
	p.mgmtNS = "test-ns"

	reporter := p.makeProgressReporter(context.Background(), "nginx:alpine")

	// First report
	reporter(Progress{Image: "nginx:alpine", Downloaded: 100, Total: 1000, Speed: 10, Status: "pulling", Message: "fetching: 10.0%"})
	if fakePatcher.calls != 1 {
		t.Errorf("expected 1 call, got %d", fakePatcher.calls)
	}

	// Wait for throttle to pass
	time.Sleep(progressReportThrottle + 10*time.Millisecond)

	// Changed data after throttle: should report
	reporter(Progress{Image: "nginx:alpine", Downloaded: 200, Total: 1000, Speed: 20, Status: "pulling", Message: "fetching: 20.0%"})
	if fakePatcher.calls != 2 {
		t.Errorf("expected 2 calls (changed + throttle elapsed), got %d", fakePatcher.calls)
	}
}

func TestProgressReporterFailedStatus(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "node1", nil)
	fakePatcher := &fakeProgressPatcher{}
	p.patcher = fakePatcher
	p.mgmtNS = "test-ns"

	reporter := p.makeProgressReporter(context.Background(), "nginx:alpine")
	reporter(Progress{Image: "nginx:alpine", Status: "failed", Message: "pull failed: context deadline exceeded"})

	if fakePatcher.calls < 1 {
		t.Error("expected progress report to be called on failure")
	}
}

func TestCleanupProgress(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "node1", nil)
	fakePatcher := &fakeProgressPatcher{}
	p.patcher = fakePatcher
	p.mgmtNS = "test-ns"

	// Add some progress state
	p.progressMu.Lock()
	p.progressMap["nginx:alpine"] = &imageProgressState{
		image: "nginx:alpine", downloaded: 100, total: 1000, status: "completed",
	}
	p.progressMap["redis:7"] = &imageProgressState{
		image: "redis:7", downloaded: 500, total: 2000, status: "pulling",
	}
	p.progressMu.Unlock()

	// Clean up one image
	p.cleanupProgress(context.Background(), "nginx:alpine")

	p.progressMu.Lock()
	_, exists := p.progressMap["nginx:alpine"]
	remaining := len(p.progressMap)
	p.progressMu.Unlock()

	if exists {
		t.Error("expected nginx:alpine to be removed from progress map")
	}
	if remaining != 1 {
		t.Errorf("expected 1 remaining entry, got %d", remaining)
	}

	// Clean up last image: reportProgress is called with an empty list so the
	// management Node CR's pullProgress field is cleared of stale entries.
	fakePatcher.calls = 0
	p.cleanupProgress(context.Background(), "redis:7")
	if fakePatcher.calls != 1 {
		t.Errorf("expected 1 report to clear stale entries when progress map becomes empty, got %d calls", fakePatcher.calls)
	}
}

func TestProgressMapCleanupOnProcessJobComplete(t *testing.T) {
	rt := &fakeExecPuller{}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "node1", nil)
	fakePatcher := &fakeProgressPatcher{}
	p.patcher = fakePatcher
	p.mgmtNS = "test-ns"

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine"}, nil))
	drain(t, p)

	// After successful pull + cleanup, progressMap should be empty
	p.progressMu.Lock()
	count := len(p.progressMap)
	p.progressMu.Unlock()

	if count != 0 {
		t.Errorf("expected progressMap to be empty after cleanup, got %d entries", count)
	}
}

func TestProgressMapCleanupOnProcessJobFailure(t *testing.T) {
	rt := &fakeExecPuller{pullFn: func(string) error { return context.DeadlineExceeded }}
	p := newTestPuller(t, rt, rlarkv1alpha1.AgentTypeDocker, "node1", nil)
	fakePatcher := &fakeProgressPatcher{}
	p.patcher = fakePatcher
	p.mgmtNS = "test-ns"

	p.handle(mkTask("u1", "1", rlarkv1alpha1.AgentTypeDocker, []string{"nginx:alpine"}, nil))
	drain(t, p)

	// After failed pull + cleanup, progressMap should be empty
	p.progressMu.Lock()
	count := len(p.progressMap)
	p.progressMu.Unlock()

	if count != 0 {
		t.Errorf("expected progressMap to be empty after cleanup on failure, got %d entries", count)
	}
}
