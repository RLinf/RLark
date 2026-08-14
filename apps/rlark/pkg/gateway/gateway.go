package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"

	versioned "github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	"github.com/rlinf/rlark/api/kubeclients/informers/externalversions"
	listerv1alpha1 "github.com/rlinf/rlark/api/kubeclients/listers/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/gateway/storage"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// Gateway provides HTTP handlers for resource CRUD APIs.
type Gateway struct {
	config Config

	kubeClient versioned.Interface
	rawClient  kubernetes.Interface

	// podLister is a cached view of rlark Pod CRs, backed by a shared
	// informer. It is used by the TensorBoard proxy handler to look up
	// the data-plane pod name for a task without hitting the API server
	// on every request.
	podLister listerv1alpha1.PodLister

	dbClient     *db.DB // may be nil if DBConfigPath is not provided, should be checked before use
	rcStore      *db.RevokedCertificateStore
	userKeyStore *db.SSHUserKeyStore

	stores    map[string]*db.ResourceStore
	accessors map[string]*resourceAccessor

	storageClients   map[string]*storage.Client
	storageClientsMu sync.RWMutex

	serverTransport *http.Transport
}

// NewGateway creates a Gateway with database-backed stores for read operations
// and a Kubernetes typed client for write operations. If database is nil, read operations
// fall back to the Kubernetes API server.
func NewGateway(config Config) *Gateway {
	return &Gateway{
		config:         config,
		storageClients: make(map[string]*storage.Client),
	}
}

// Run runs the component.
func (g *Gateway) Run(ctx context.Context) error {
	if err := g.init(ctx); err != nil {
		return err
	}

	var eg errgroup.Group

	eg.Go(func() error {
		return g.runHTTPServer(ctx)
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("gateway run error: %w", err)
	}
	return nil
}

func (g *Gateway) init(ctx context.Context) error {
	logger := log.FromContext(ctx)
	// init Kubernetes client
	restConfig, err := g.config.KubeClientConfig.BuildRestConfig()
	if err != nil {
		return fmt.Errorf("build Kubernetes rest config: %w", err)
	}
	g.kubeClient, err = versioned.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}
	g.rawClient, err = kubernetes.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("create raw Kubernetes client: %w", err)
	}

	// init pod informer/lister for cached Pod lookups (used by the
	// TensorBoard proxy handler).
	factory := externalversions.NewSharedInformerFactory(g.kubeClient, 30*time.Minute)
	g.podLister = factory.Rlinf().V1alpha1().Pods().Lister()
	factory.StartWithContext(ctx)
	synced := factory.WaitForCacheSyncWithContext(ctx)
	if err := synced.AsError(); err != nil {
		return fmt.Errorf("wait for pod informer cache sync: %w", err)
	}

	// init accessors
	g.accessors = registerAccessors(g.kubeClient)

	// init DB (optional)
	if g.config.DBConfigPath != "" {
		g.dbClient, err = db.OpenFromFileConfig(g.config.DBConfigPath)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		g.rcStore = db.NewRevokedCertificateStore(g.dbClient.DB)
		g.userKeyStore = db.NewSSHUserKeyStore(g.dbClient.DB)
		g.stores = make(map[string]*db.ResourceStore)
		g.stores["nodes"] = db.NewNodeStore(g.dbClient.DB)
		g.stores["workflows"] = db.NewWorkflowStore(g.dbClient.DB)
		g.stores["jobs"] = db.NewJobStore(g.dbClient.DB)
		g.stores["tasks"] = db.NewTaskStore(g.dbClient.DB)
	} else {
		logger.Error(nil, "RLark gateway is running without persistent storage.")
	}

	adminCertPEM, adminKeyPEM, caCertPEM, err := g.getKCPAdminCerts()
	if err != nil {
		return fmt.Errorf("get admin certs: %w", err)
	}

	cert, err := tls.X509KeyPair(adminCertPEM, adminKeyPEM)
	if err != nil {
		return fmt.Errorf("load admin keypair: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caCertPEM) {
		return fmt.Errorf("failed to parse CA cert")
	}

	g.serverTransport = &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			RootCAs:      caPool,
		},
	}

	return nil
}

func (g *Gateway) runHTTPServer(ctx context.Context) error {
	logger := log.FromContext(ctx)
	r := gin.Default()
	r.Use(MetricsMiddleware())
	g.RegisterRoutes(r)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	server := http.Server{
		Addr:    g.config.Address,
		Handler: r,
	}

	l, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", server.Addr, err)
	}

	go func() {
		<-ctx.Done()
		logger.Info("Shutting down unsafe HTTP server", "address", g.config.Address)
		if err := server.Shutdown(context.Background()); err != nil {
			logger.Error(nil, "Unsafe HTTP server shutdown error", "err", err)
		}
	}()

	logger.Info("Starting unsafe HTTP server", "address", g.config.Address)
	return server.Serve(l)
}
