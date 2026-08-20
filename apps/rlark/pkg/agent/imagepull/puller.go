package imagepull

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

const (
	// numWorkers bounds concurrent image pulls so a burst of tasks cannot
	// fork-bomb the node. Slow pulls queue in the buffered jobs channel.
	numWorkers = 4
	// jobsBuffer bounds the backlog of pending pull jobs, applying backpressure
	// to informer handlers when pulls cannot keep up.
	jobsBuffer = 64
	// nodeLookupTimeout caps a local Kubernetes Node fetch so a hung API call
	// cannot stall the informer event stream indefinitely.
	nodeLookupTimeout = 15 * time.Second
	// progressReportThrottle is the minimum interval between progress reports
	// to the management node status for ongoing pulls. Status transitions
	// (completed/failed) are always reported immediately.
	progressReportThrottle = 1 * time.Second
)

// ProgressPatcher patches the node status with pull progress entries.
// Implementations should use a merge patch to update only the pullProgress field.
type ProgressPatcher interface {
	PatchPullProgress(ctx context.Context, nodeName, namespace string, progresses []rlarkv1alpha1.PullProgress) error
}

// PullerConfig configures a Puller.
type PullerConfig struct {
	Runtime   ImagePuller
	AgentType rlarkv1alpha1.AgentType
	// NodeName identifies the local node for Kubernetes node-selector matching.
	// Empty disables selector filtering (pull-all with a warning).
	NodeName string
	// LocalKube is the local Kubernetes client used to read node labels for the
	// Kubernetes path. May be nil for Docker/Raw agents.
	LocalKube kubernetes.Interface
	// ProgressPatcher patches node status with pull progress updates.
	// If nil, progress reporting is disabled.
	ProgressPatcher ProgressPatcher
	// ManagementNamespace is the namespace for management Node CRs.
	ManagementNamespace string
}

// taskState tracks, per Task, the last-processed ResourceVersion and the set of
// images already pulled. It guards against redundant pulls across informer
// resyncs and Task updates.
type taskState struct {
	rv     string
	pulled map[string]struct{}
	mu     sync.Mutex
}

type pullJob struct {
	taskUID types.UID
	image   string
}

// imageProgressState tracks the current progress of a single image pull.
type imageProgressState struct {
	image      string
	downloaded int64
	total      int64
	speed      float64
	status     string
	message    string
	updatedAt  time.Time
}

// Puller watches management Tasks and pre-pulls their referenced images into
// the node's container runtime. It is a best-effort side-effect optimization
// and never writes to Task status.
type Puller struct {
	log       logr.Logger
	runtime   ImagePuller
	agentType rlarkv1alpha1.AgentType
	nodeName  string
	localKube kubernetes.Interface

	mu     sync.Mutex
	states map[types.UID]*taskState

	jobs chan pullJob

	patcher ProgressPatcher
	mgmtNS  string

	progressMu  sync.Mutex
	progressMap map[string]*imageProgressState
}

// NewPuller constructs a Puller and registers informer event handlers. It MUST
// be called before the informer factory is started so no events are missed.
// Worker goroutines are started by Run.
func NewPuller(cfg PullerConfig, taskInformer cache.SharedIndexInformer, log logr.Logger) *Puller {
	p := &Puller{
		log:         log.WithName("imagePull"),
		runtime:     cfg.Runtime,
		agentType:   cfg.AgentType,
		nodeName:    cfg.NodeName,
		localKube:   cfg.LocalKube,
		states:      make(map[types.UID]*taskState),
		jobs:        make(chan pullJob, jobsBuffer),
		patcher:     cfg.ProgressPatcher,
		mgmtNS:      cfg.ManagementNamespace,
		progressMap: make(map[string]*imageProgressState),
	}
	if taskInformer != nil {
		_, _ = taskInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc:    p.onAdd,
			UpdateFunc: p.onUpdate,
			DeleteFunc: p.onDelete,
		})
	}
	return p
}

// Run starts the worker pool and blocks until ctx is cancelled. The actual
// work happens in informer callbacks (dispatching jobs) and workers (pulling).
func (p *Puller) Run(ctx context.Context) error {
	p.log.Info("starting image puller", "workers", numWorkers, "agentType", p.agentType, "nodeName", p.nodeName)
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.worker(ctx)
		}()
	}
	<-ctx.Done()
	wg.Wait()
	p.log.Info("image puller stopped")
	return ctx.Err()
}

func (p *Puller) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-p.jobs:
			p.processJob(ctx, job)
		}
	}
}

func (p *Puller) processJob(ctx context.Context, job pullJob) {
	log := p.log.WithValues("image", job.image, "taskUID", job.taskUID)

	// Best-effort existence check: skip the pull if already present (e.g. after
	// an agent restart where in-memory state was lost).
	if exists, err := p.runtime.ImageExists(ctx, job.image); err != nil {
		log.V(1).Info("image exists check failed, will attempt pull", "err", err.Error())
	} else if exists {
		log.V(1).Info("image already present locally, skipping pull")
		p.markPulled(job.taskUID, job.image)
		return
	}

	reporter := p.makeProgressReporter(ctx, job.image)
	if err := p.runtime.Pull(ctx, job.image, reporter); err != nil {
		log.Error(err, "failed to pull image")
		p.cleanupProgress(ctx, job.image)
		return
	}
	log.Info("image pulled")
	p.markPulled(job.taskUID, job.image)

	p.cleanupProgress(ctx, job.image)
}

// makeProgressReporter creates a ProgressReporter that stores progress
// updates and triggers node status patching when progress changes.
// It reports immediately on status transitions (completed/failed) and
// throttles ongoing progress updates to progressReportThrottle intervals.
func (p *Puller) makeProgressReporter(ctx context.Context, image string) ProgressReporter {
	var lastDownloaded int64
	var lastTotal int64
	var lastSpeed float64
	var lastStatus string
	var lastMessage string
	var lastReportTime time.Time

	return func(progress Progress) {
		p.progressMu.Lock()
		state, ok := p.progressMap[image]
		if !ok {
			state = &imageProgressState{image: image}
			p.progressMap[image] = state
		}
		state.downloaded = progress.Downloaded
		state.total = progress.Total
		state.speed = progress.Speed
		state.status = progress.Status
		state.message = progress.Message
		state.updatedAt = time.Now()

		isTerminal := progress.Status == "completed" || progress.Status == "failed"
		changed := progress.Downloaded != lastDownloaded ||
			progress.Total != lastTotal ||
			progress.Speed != lastSpeed ||
			progress.Status != lastStatus ||
			progress.Message != lastMessage

		throttled := time.Since(lastReportTime) >= progressReportThrottle

		if isTerminal || (changed && throttled) {
			lastDownloaded = progress.Downloaded
			lastTotal = progress.Total
			lastSpeed = progress.Speed
			lastStatus = progress.Status
			lastMessage = progress.Message
			lastReportTime = time.Now()

			snapshots := p.collectProgressSnapshotsLocked()
			p.progressMu.Unlock()
			// Always log progress locally so it is observable even when the
			// management server is unreachable (patcher nil / patch failures).
			p.logProgress(progress)
			p.reportProgress(ctx, snapshots)
		} else {
			p.progressMu.Unlock()
		}
	}
}

// cleanupProgress removes a completed/failed image from the progress map
// and sends a final status report. When the map becomes empty it patches the
// node status with an empty list so stale progress entries are cleared.
func (p *Puller) cleanupProgress(ctx context.Context, image string) {
	p.progressMu.Lock()
	delete(p.progressMap, image)
	snapshots := p.collectProgressSnapshotsLocked()
	p.progressMu.Unlock()
	p.reportProgress(ctx, snapshots)
}

// collectProgressSnapshotsLocked returns a snapshot of current progress states.
// Must be called with progressMu held.
func (p *Puller) collectProgressSnapshotsLocked() []imageProgressState {
	snapshots := make([]imageProgressState, 0, len(p.progressMap))
	for _, state := range p.progressMap {
		snapshots = append(snapshots, *state)
	}
	return snapshots
}

// logProgress writes a single image's pull progress to the agent logs. It is
// called whenever progress changes (throttled to progressReportThrottle) or on
// terminal status transitions, independent of whether the management server is
// reachable. This guarantees progress is always observable locally.
func (p *Puller) logProgress(progress Progress) {
	p.log.Info("image pull progress",
		"image", progress.Image,
		"status", progress.Status,
		"downloaded", progress.Downloaded,
		"total", progress.Total,
		"speed", progress.Speed,
		"message", progress.Message,
	)
}

// reportProgress patches the management Node status with current pull progress.
func (p *Puller) reportProgress(ctx context.Context, snapshots []imageProgressState) {
	if p.patcher == nil || p.nodeName == "" {
		return
	}

	progressList := make([]rlarkv1alpha1.PullProgress, 0, len(snapshots))
	for _, s := range snapshots {
		progressList = append(progressList, rlarkv1alpha1.PullProgress{
			Image:      s.image,
			Downloaded: s.downloaded,
			Total:      s.total,
			Speed:      s.speed,
			Status:     s.status,
			Message:    s.message,
		})
	}

	if err := p.patcher.PatchPullProgress(ctx, p.nodeName, p.mgmtNS, progressList); err != nil {
		p.log.V(1).Info("failed to patch node pull progress", "err", err.Error())
	}
}

// --- informer event handlers ---

func (p *Puller) onAdd(obj interface{}) {
	task, ok := obj.(*rlarkv1alpha1.Task)
	if !ok {
		return
	}
	p.handle(task)
}

func (p *Puller) onUpdate(oldObj, newObj interface{}) {
	oldTask, ok := oldObj.(*rlarkv1alpha1.Task)
	if !ok {
		return
	}
	newTask, ok := newObj.(*rlarkv1alpha1.Task)
	if !ok {
		return
	}
	if oldTask.ResourceVersion == newTask.ResourceVersion {
		return
	}
	p.handle(newTask)
}

func (p *Puller) onDelete(obj interface{}) {
	task, ok := obj.(*rlarkv1alpha1.Task)
	if !ok {
		return
	}
	p.mu.Lock()
	delete(p.states, task.UID)
	p.mu.Unlock()
}

// handle applies the filtering and dispatch logic for a Task event. It must
// not block on pulls: it only evaluates fast in-memory state and dispatches
// pull jobs to the worker pool.
func (p *Puller) handle(task *rlarkv1alpha1.Task) {
	log := p.log.WithValues("task", task.Name, "namespace", task.Namespace, "rv", task.ResourceVersion)

	if task.Spec.AgentType != p.agentType {
		return
	}

	images := utils.ExtractTaskImages(task)
	if len(images) == 0 {
		return
	}

	if p.agentType == rlarkv1alpha1.AgentTypeKubernetes {
		if p.nodeName == "" || p.localKube == nil {
			log.Info("node name unknown, pulling without node-selector filtering")
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), nodeLookupTimeout)
			matches, err := p.nodeMatches(ctx, task.Spec.NodeSelector)
			cancel()
			if err != nil {
				log.Error(err, "failed to evaluate node selector, pulling anyway")
			} else if !matches {
				log.V(1).Info("task node selector does not match this node, skipping")
				return
			}
		}
	}

	st := p.stateFor(task.UID)
	st.mu.Lock()
	var toPull []string
	for _, img := range images {
		if _, ok := st.pulled[img]; ok {
			continue
		}
		toPull = append(toPull, img)
	}
	if st.rv == task.ResourceVersion && len(toPull) == 0 {
		st.mu.Unlock()
		return
	}
	st.rv = task.ResourceVersion
	st.mu.Unlock()

	for _, img := range toPull {
		p.dispatch(pullJob{taskUID: task.UID, image: img})
	}
}

// dispatch enqueues a pull job without blocking. If the jobs buffer is full
// (pulls cannot keep up), the job is dropped with a warning; correctness is
// preserved because the kubelet/runtime pulls at Pod scheduling time.
func (p *Puller) dispatch(job pullJob) {
	select {
	case p.jobs <- job:
	default:
		p.log.V(1).Info("image pull queue full, dropping pull job (kubelet will pull at schedule time)", "image", job.image)
	}
}

func (p *Puller) stateFor(uid types.UID) *taskState {
	p.mu.Lock()
	defer p.mu.Unlock()
	st, ok := p.states[uid]
	if !ok {
		st = &taskState{pulled: make(map[string]struct{})}
		p.states[uid] = st
	}
	return st
}

func (p *Puller) markPulled(uid types.UID, image string) {
	st := p.stateFor(uid)
	st.mu.Lock()
	st.pulled[image] = struct{}{}
	st.mu.Unlock()
}

// nodeMatches reports whether the local node's labels satisfy the task's node
// selector. An empty selector matches all nodes.
func (p *Puller) nodeMatches(ctx context.Context, selector map[string]string) (bool, error) {
	node, err := p.localKube.CoreV1().Nodes().Get(ctx, p.nodeName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	return matchNodeSelector(node.Labels, selector), nil
}

// matchNodeSelector mirrors the semantics of applyNodeSelector in
// controllers/task/pull.go: values containing commas are treated as an "In" set
// (OR within a key), multiple keys are ANDed, and an empty selector matches.
func matchNodeSelector(nodeLabels, selector map[string]string) bool {
	if len(selector) == 0 {
		return true
	}
	for k, v := range selector {
		if strings.Contains(v, ",") {
			values := strings.Split(v, ",")
			matched := false
			for i := range values {
				values[i] = strings.TrimSpace(values[i])
				if nodeLabels[k] == values[i] {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
			continue
		}
		if nodeLabels[k] != strings.TrimSpace(v) {
			return false
		}
	}
	return true
}

// --- merge patch helper ---

// buildPullProgressMergePatch creates a JSON merge patch body for the
// pullProgress field on the status subresource. The "status" wrapper is
// required: the status subresource endpoint applies the merge patch to the
// whole object (only .status modifications are accepted), not to the .status
// sub-object directly. The caller must pass "status" as the subresource
// argument; a main-resource Patch would have its "status" field stripped
// by the API server when subresources.status is enabled on the CRD.
func buildPullProgressMergePatch(progresses []rlarkv1alpha1.PullProgress) ([]byte, error) {
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"pullProgress": progresses,
		},
	}
	return json.Marshal(patch)
}
