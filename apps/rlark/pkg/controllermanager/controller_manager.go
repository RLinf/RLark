package controllermanager

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/archive"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/cluster"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/controller"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/domain"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/job"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/node"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/replication"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/sweep"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/sync"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/task"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/upgrade"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/workflow"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	archivestore "github.com/rlinf/rlark/apps/rlark/pkg/persistence"
)

// Reconciler reconciles resources.
type Reconciler interface {
	SetupWithManager(ctrl.Manager) error
}

var (
	scheme = runtime.NewScheme()
)

// cacheSyncProbeTimeout bounds how long a single readiness probe waits for the
// informer cache. It is short on purpose: the probe only has to answer "synced
// yet?", and the kubelet will ask again.
const cacheSyncProbeTimeout = 2 * time.Second

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rlarkv1alpha1.AddToScheme(scheme))
}

// New creates a new instance.
func New(config Config) (manager.Manager, error) {
	logger := log.GetLogger()
	ctrl.SetLogger(logger)
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("validate controller manager config: %w", err)
	}

	restConfig, err := config.KubeClientConfig.BuildRestConfig()
	if err != nil {
		return nil, fmt.Errorf("build Kubernetes client config: %w", err)
	}
	options, err := managerOptions(config, restConfig)
	if err != nil {
		return nil, err
	}
	mgr, err := ctrl.NewManager(restConfig, options)
	if err != nil {
		return nil, fmt.Errorf("create manager: %w", err)
	}
	// Every reconcile pass in this process is classified and recorded by the
	// same shell, so the collectors are registered once here and shared. The
	// EventRecorder is per controller because its name becomes the event
	// source, which is how an operator tells which controller spoke.
	reconcileMetrics, err := controller.NewMetrics(ctrlmetrics.Registry)
	if err != nil {
		return nil, fmt.Errorf("register controller metrics: %w", err)
	}
	shellFor := func(name string) controller.Shell {
		// GetEventRecorderFor is deprecated in favour of the events/v1 style
		// recorder, whose Eventf additionally requires a related object and an
		// action verb. Both recorders share one broadcaster and still write
		// core/v1 Events, so switching is an event-semantics change rather than
		// a correctness fix; it is deliberately kept out of this change.
		return controller.NewShell(reconcileMetrics, mgr.GetEventRecorderFor(name)) //nolint:staticcheck
	}

	replicationReconciler, err := replication.New(restConfig, mgr.GetClient())
	if err != nil {
		return nil, fmt.Errorf("create replication controller: %w", err)
	}
	replicationReconciler.Shell = shellFor("resource-replication-controller")

	reconcilers := []Reconciler{
		replicationReconciler,
		&job.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Job,
			Shell:                   shellFor("job-controller"),
		},
		&task.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Task,
			Shell:                   shellFor("task-controller"),
			Resync:                  config.TaskFreshness,
		},
		&workflow.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Workflow,
			Shell:                   shellFor("workflow-controller"),
		},
		&node.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Node,
			LeaseTimeout:            config.NodeHeartbeatTimeout,
			LeaseGracePeriod:        config.NodeHeartbeatGracePeriod,
			Shell:                   shellFor("node-controller"),
		},
		&cluster.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Cluster,
			Shell:                   shellFor("cluster-controller"),
		},
		&domain.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Domain,
			Shell:                   shellFor("domain-controller"),

			KubeClientConfig: config.KubeClientConfig,
			ServerAddress:    config.ServerAddress,
		},
		&upgrade.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: 1, // Only one instance to aggregate status
		},
	}

	// The sweeper reads through the API server rather than the informer cache:
	// it lists in pages, and the cache reader rejects a List that carries a
	// continue token. Writes still go through the cached client so the delete
	// and the mark are observed by the same controllers as any other write.
	orphanSweeper, err := sweep.New(mgr.GetAPIReader(), mgr.GetClient(), shellFor("orphan-sweeper"), config.OrphanSweep,
		domain.OrphanPeerRule(),
		job.OrphanTaskRule(),
	)
	if err != nil {
		return nil, fmt.Errorf("create orphan sweeper: %w", err)
	}
	reconcilers = append(reconcilers, orphanSweeper)

	// 初始化持久化后端：优先 PG，否则用 Secret（测试模式）
	var backend archivestore.Backend

	if config.DBConfigPath != "" {
		dbConfig := db.DefaultConfig()
		data, err := os.ReadFile(config.DBConfigPath)
		if err != nil {
			return nil, fmt.Errorf("read database config file: %w", err)
		}
		if err := db.UnmarshalConfig(data, &dbConfig); err != nil {
			return nil, fmt.Errorf("unmarshal database config: %w", err)
		}
		database, err := db.Open(dbConfig)
		if err != nil {
			return nil, fmt.Errorf("open database: %w", err)
		}
		ctx := context.Background()
		if err := database.Migrate(ctx); err != nil {
			return nil, fmt.Errorf("run database migrations: %w", err)
		}
		logger.Info("database connected and migrated")
		backend = archivestore.NewPostgresBackend(database.DB)
	} else if config.EnableSecretBackend {
		// Secret Backend：测试/CI 模式，用 K8s Secret 存归档快照。
		backend = archivestore.NewSecretBackend(mgr.GetClient(), config.KubeClientConfig.DefaultNamespace())
		logger.Info("using Secret backend for persistence (test mode)")
	} else {
		// Running without a persistence backend is a supported configuration,
		// not a failure: it only means the sync and archive controllers stay
		// off. Logging it at error level made every such deployment look broken
		// and trained operators to ignore real errors.
		logger.Info("no persistence backend configured; snapshot sync and archive controllers are disabled")
	}

	// Start the sync and archive controllers only when a backend exists.
	if backend != nil {
		reconcilers = append(reconcilers,
			sync.NewJobReconciler(config.ControllerConcurrency.JobSync, mgr.GetClient(), backend, shellFor("job-sync-controller")),
			sync.NewTaskReconciler(config.ControllerConcurrency.TaskSync, mgr.GetClient(), backend, shellFor("task-sync-controller")),
			sync.NewWorkflowReconciler(config.ControllerConcurrency.WorkflowSync, mgr.GetClient(), backend, shellFor("workflow-sync-controller")),
			sync.NewNodeReconciler(config.ControllerConcurrency.NodeSync, mgr.GetClient(), backend, shellFor("node-sync-controller")),
		)

		for _, res := range archive.DefaultRegistry.Resources() {
			reconcilers = append(reconcilers, archive.NewReconciler(mgr.GetClient(), scheme, res,
				config.ControllerConcurrency.Archive, config.ArchiveTTL, shellFor(res.Kind()+"-archive-controller")),
			)
		}
	}

	for _, r := range reconcilers {
		if err := r.SetupWithManager(mgr); err != nil {
			return nil, fmt.Errorf("setup controller: %w", err)
		}
	}

	// Liveness deliberately stays a pure process-liveness signal. Making it
	// depend on the API server or the database would turn a dependency outage
	// into a restart loop, which removes the only component that could recover
	// once the dependency comes back.
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("setup health check: %w", err)
	}
	// Readiness, by contrast, must be false while the informer cache is still
	// filling: a reconcile that runs against a partially populated cache sees a
	// child or a peer as absent and can act on that false conclusion.
	if err := mgr.AddReadyzCheck("informer-cache-sync", func(req *http.Request) error {
		ctx, cancel := context.WithTimeout(req.Context(), cacheSyncProbeTimeout)
		defer cancel()
		if !mgr.GetCache().WaitForCacheSync(ctx) {
			return fmt.Errorf("informer cache has not synced")
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("setup ready check: %w", err)
	}

	return mgr, nil
}

func managerOptions(config Config, restConfig *rest.Config) (ctrl.Options, error) {
	options := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: config.MetricsBindAddress},
		HealthProbeBindAddress: config.ProbeBindAddress,
		LeaderElection:         config.LeaderElection.Enabled,
	}
	if !config.LeaderElection.Enabled {
		return options, nil
	}

	leNamespace, leName, err := config.LeaderElection.NamespaceAndName(config.KubeClientConfig.DefaultNamespace())
	if err != nil {
		return ctrl.Options{}, fmt.Errorf("resolve leader election key: %w", err)
	}
	if err := config.LeaderElection.Validate(config.KubeClientConfig.DefaultNamespace()); err != nil {
		return ctrl.Options{}, fmt.Errorf("validate leader election config: %w", err)
	}
	options.LeaderElectionID = leName
	options.LeaderElectionNamespace = leNamespace
	if config.LeaderElection.Identity != "" {
		clientset, err := kubernetes.NewForConfig(restConfig)
		if err != nil {
			return ctrl.Options{}, fmt.Errorf("create leader election client: %w", err)
		}
		lock, err := config.LeaderElection.ResourceLock(clientset, config.KubeClientConfig.DefaultNamespace())
		if err != nil {
			return ctrl.Options{}, fmt.Errorf("create leader election lock: %w", err)
		}
		options.LeaderElectionResourceLockInterface = lock
	}
	if config.LeaderElection.LeaseDuration > 0 {
		options.LeaseDuration = &config.LeaderElection.LeaseDuration
		options.RenewDeadline = &config.LeaderElection.RenewDeadline
		options.RetryPeriod = &config.LeaderElection.RetryPeriod
	}
	return options, nil
}
