package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/coordination/coordinator"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/evidence"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/host"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
)

// Controller manages resources.
type Controller struct {
	base.Controller
	execution *coordinator.Coordinator
	host      *host.Host
	status    host.StatusWriter
	triggers  *triggerBridge
	// logMu protects logState because controller-runtime may reconcile Tasks in
	// parallel through the same Controller instance.
	logMu      sync.Mutex
	logState   map[string]taskReconcileLogState
	evidenceMu sync.RWMutex
	evidence   map[trigger.Key]verifiedExecution
}

type verifiedExecution struct {
	taskUID    string
	generation int64
	revision   evidence.Revision
	roots      map[string]struct{}
}

// taskReconcileLogState records the last emitted in-progress summary for one
// management Task. It is an observability cache only and never affects retry
// or reconciliation decisions.
type taskReconcileLogState struct {
	signature string
	loggedAt  time.Time
}

// WithExecutionCoordinator installs the sole Runtime writer used by Task
// reconciliation. Legacy helpers remain only for non-declarative lifecycle
// compatibility such as restart cleanup and in-container SSH key updates.
func (c *Controller) WithExecutionCoordinator(execution *coordinator.Coordinator) *Controller {
	c.execution = execution
	return c
}

// WithHost installs the sole Task lifecycle and Status owner.
func (c *Controller) WithHost(taskHost *host.Host) *Controller {
	c.host = taskHost
	if taskHost != nil {
		c.status = taskHost.Status
	}
	return c
}

// shouldLogTaskReconcile reports whether a successful reconcile summary should
// be emitted. Only repeated in-progress results are suppressed; every other
// result clears cached state and is logged.
func (c *Controller) shouldLogTaskReconcile(key string, result host.Result, now time.Time) bool {
	if result.Retry.Class != host.RetryInProgress {
		c.clearTaskReconcileLog(key)
		return true
	}
	signature := string(result.Outcome) + "\x00" + string(result.Retry.Class) + "\x00" + result.Reason
	c.logMu.Lock()
	defer c.logMu.Unlock()
	if c.logState == nil {
		c.logState = make(map[string]taskReconcileLogState)
	}
	previous, found := c.logState[key]
	if found && previous.signature == signature && now.Sub(previous.loggedAt) < taskReconcileLogInterval {
		return false
	}
	c.logState[key] = taskReconcileLogState{signature: signature, loggedAt: now}
	return true
}

// clearTaskReconcileLog removes suppression state for one Task so a result
// following an error or a completed in-progress sequence is logged immediately.
func (c *Controller) clearTaskReconcileLog(key string) {
	c.logMu.Lock()
	delete(c.logState, key)
	c.logMu.Unlock()
}

var _ base.Reconciler = (*Controller)(nil)

// NewTaskController creates a new Controller.
func NewTaskController(bc base.Controller) *Controller {
	tc := &Controller{
		Controller: bc,
		triggers:   newTriggerBridge(),
	}
	tc.C = tc
	return tc
}

// SetupPullController installs the management Task watch and the shared typed
// trigger channel on one controller-runtime queue.
func (c *Controller) SetupPullController(mgr ctrl.Manager) error {
	if err := c.validateExecutionDependencies(); err != nil {
		return err
	}
	blder := ctrl.NewControllerManagedBy(mgr).
		Watches(&rlarkv1alpha1.Task{}, c.desiredTaskHandler()).
		WatchesRawSource(source.Channel(c.triggers.events, &handler.EnqueueRequestForObject{})).
		Named("task-pull")
	if c.PullMaxConcurrentReconciles > 0 {
		blder = blder.WithOptions(controller.Options{MaxConcurrentReconciles: c.PullMaxConcurrentReconciles})
	}
	return blder.Complete(c.AsPullReconciler())
}

func (c *Controller) validateExecutionDependencies() error {
	switch {
	case c.host == nil:
		return fmt.Errorf("task Host is not configured")
	case c.execution == nil:
		return fmt.Errorf("task execution coordinator is not configured")
	case c.status == nil:
		return fmt.Errorf("task Status writer is not configured")
	case c.ManagementClient == nil:
		return fmt.Errorf("task management client is not configured")
	case c.LocalKubeClient == nil:
		return fmt.Errorf("task local Kubernetes client is not configured")
	default:
		return nil
	}
}

func (c *Controller) desiredTaskHandler() handler.EventHandler {
	enqueue := func(object client.Object, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
		if object == nil {
			return
		}
		key := trigger.Key{Namespace: object.GetNamespace(), Name: object.GetName()}
		_ = c.triggers.accumulator.Merge(trigger.Trigger{Key: key, Kind: trigger.DesiredState, ObjectUID: string(object.GetUID()), Generation: object.GetGeneration()})
		queue.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: key.Namespace, Name: key.Name}})
	}
	return handler.Funcs{
		CreateFunc: func(_ context.Context, value event.CreateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(value.Object, queue)
		},
		UpdateFunc: func(_ context.Context, value event.UpdateEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(value.ObjectNew, queue)
		},
		DeleteFunc: func(_ context.Context, value event.DeleteEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(value.Object, queue)
		},
		GenericFunc: func(_ context.Context, value event.GenericEvent, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			enqueue(value.Object, queue)
		},
	}
}

// KubernetesResource is an exported method.
func (c *Controller) KubernetesResource() base.KubernetesResource {
	return base.KubernetesResource{
		Name: "task",
		Type: &rlarkv1alpha1.Task{},
	}
}

// AsPullReconciler is an exported method.
func (c *Controller) AsPullReconciler() base.KubernetesReconciler {
	return &pullReconciler{c: c}
}

// AsKubePushReconcilers is an exported method.
func (c *Controller) AsKubePushReconcilers() map[base.KubernetesResource]base.KubernetesReconciler {
	return map[base.KubernetesResource]base.KubernetesReconciler{
		{
			Name: "task-deployment",
			Type: &appsv1.Deployment{},
		}: &pushDeploymentReconciler{c: c},
		{
			Name: "task-daemonset",
			Type: &appsv1.DaemonSet{},
		}: &pushDaemonSetReconciler{c: c},
		{
			Name: "task-statefulset",
			Type: &appsv1.StatefulSet{},
		}: &pushStatefulSetReconciler{c: c},
	}
}

// AsDockerPushReconcilers is an exported method.
func (c *Controller) AsDockerPushReconcilers() map[base.DockerResource]base.DockerReconciler {
	return nil
}

// AsRawPushReconcilers is an exported method.
func (c *Controller) AsRawPushReconcilers() map[base.RawResource]base.RawReconciler {
	return nil
}
