package controllermanager

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/domain"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/job"
	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager/node"
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

	restConfig, err := config.KubeClientConfig.BuildRestConfig()
	if err != nil {
		return nil, fmt.Errorf("build Kubernetes client config: %w", err)
	}
	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: config.MetricsBindAddress},
		HealthProbeBindAddress: config.ProbeBindAddress,
		LeaderElection:         config.LeaderElection,
		LeaderElectionID:       config.LeaderElectionID,
	})
	if err != nil {
		return nil, fmt.Errorf("create manager: %w", err)
	}

	reconcilers := []Reconciler{
		&job.Reconciler{
			Client: mgr.GetClient(),
			Scheme: scheme,
		},
		&task.Reconciler{
			Client: mgr.GetClient(),
			Scheme: scheme,
		},
		&workflow.Reconciler{
			Client: mgr.GetClient(),
			Scheme: scheme,
		},
		&node.Reconciler{
			Client: mgr.GetClient(),
			Scheme: scheme,
		},
		&domain.Reconciler{
			Client: mgr.GetClient(),
			Scheme: scheme,

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
			sync.NewJobReconciler(config.SyncConfig, mgr.GetClient(), database.DB),
			sync.NewTaskReconciler(config.SyncConfig, mgr.GetClient(), database.DB),
			sync.NewWorkflowReconciler(config.SyncConfig, mgr.GetClient(), database.DB),
			sync.NewNodeReconciler(config.SyncConfig, mgr.GetClient(), database.DB),
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
