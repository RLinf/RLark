package controllermanager

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/domain"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/job"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/node"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/replication"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/sync"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/task"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/workflow"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// Reconciler reconciles resources.
type Reconciler interface {
	SetupWithManager(ctrl.Manager) error
}

var (
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rlarkv1alpha1.AddToScheme(scheme))
}

// New creates a new instance.
func New(config Config) (manager.Manager, error) {
	logger := log.GetLogger()
	ctrl.SetLogger(logger)
	if err := config.ControllerConcurrency.Validate(); err != nil {
		return nil, fmt.Errorf("validate controller concurrency: %w", err)
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
	replicationReconciler, err := replication.New(restConfig, mgr.GetClient())
	if err != nil {
		return nil, fmt.Errorf("create replication controller: %w", err)
	}

	reconcilers := []Reconciler{
		replicationReconciler,
		&job.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Job,
		},
		&task.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Task,
		},
		&workflow.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Workflow,
		},
		&node.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Node,
		},
		&domain.Reconciler{
			Client:                  mgr.GetClient(),
			Scheme:                  scheme,
			MaxConcurrentReconciles: config.ControllerConcurrency.Domain,

			KubeClientConfig: config.KubeClientConfig,
			ServerAddress:    config.ServerAddress,
		},
	}

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
		reconcilers = append(reconcilers,
			sync.NewJobReconciler(config.ControllerConcurrency.JobSync, mgr.GetClient(), database.DB),
			sync.NewTaskReconciler(config.ControllerConcurrency.TaskSync, mgr.GetClient(), database.DB),
			sync.NewWorkflowReconciler(config.ControllerConcurrency.WorkflowSync, mgr.GetClient(), database.DB),
			sync.NewNodeReconciler(config.ControllerConcurrency.NodeSync, mgr.GetClient(), database.DB),
		)
	} else {
		logger.Error(nil, "RLark controller manager is running without persistent storage.")
	}

	for _, r := range reconcilers {
		if err := r.SetupWithManager(mgr); err != nil {
			return nil, fmt.Errorf("setup controller: %w", err)
		}
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return nil, fmt.Errorf("setup health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
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
