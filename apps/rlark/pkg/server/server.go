// Package server provides the RLark control plane server, including SSH bastion host,
// REST API gateway, and terminal proxy functionality.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"golang.org/x/sync/errgroup"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	"github.com/rlinf/rlark/api/kubeclients/informers/externalversions"
	listerv1alpha1 "github.com/rlinf/rlark/api/kubeclients/listers/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/db"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/server/caches"
	"github.com/rlinf/rlark/apps/rlark/pkg/server/reverseproxy"
)

// Server is the main struct for the server application, encapsulating all components and dependencies.
type Server struct {
	// config holds the server configuration parameters.
	config Config

	restConfig  *rest.Config
	kubeClient  kubernetes.Interface
	rlarkClient versioned.Interface
	kubeHandler http.Handler

	// DB Client and Stores
	// may be nil if DBConfigPath is not provided, should be checked before use
	dbClient     *db.DB
	rcStore      *db.RevokedCertificateStore
	rcCache      *gocache.Cache
	userKeyStore *db.SSHUserKeyStore

	// resource informers/listers/cache
	podInformer cache.SharedIndexInformer
	podLister   listerv1alpha1.PodLister
	podCache    *caches.PodCache

	tlsCA *cert.Data
	tls   cert.Data
	ca    []cert.Data

	dialerFactory         *reverseproxy.DialerFactory
	defaultProxyTransport http.RoundTripper
	defaultPeerTransport  http.RoundTripper

	// health scope variables
	peerBroadcasted bool // 第一次广播完成后，才认为服务已经准备好
}

// NewServer creates a new Server instance with the provided configuration.
func NewServer(config Config) *Server {
	s := &Server{
		config: config,

		rcCache: gocache.New(5*time.Minute, 10*time.Minute),

		ca: make([]cert.Data, 0),

		dialerFactory: reverseproxy.NewDialerFactory(),
	}
	s.defaultProxyTransport = &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dial, address, err := s.GetDial(ctx, "default", addr, nil)
			if err != nil {
				return nil, fmt.Errorf("get dial for address %s: %w", addr, err)
			}
			return dial(ctx, network, address)
		},
	}
	return s
}

// Run runs the component.
func (s *Server) Run(ctx context.Context) error {
	if err := s.init(ctx); err != nil {
		return err
	}

	var eg errgroup.Group
	eg.Go(func() error {
		return s.runBroadcaster(ctx)
	})
	eg.Go(func() error {
		return s.runPeerTunnel(ctx)
	})
	eg.Go(func() error {
		return s.runHTTPSServer(ctx)
	})
	eg.Go(func() error {
		return s.runSSHServer(ctx)
	})
	eg.Go(func() error {
		return s.runUnsafeHTTPServer(ctx)
	})

	if err := eg.Wait(); err != nil {
		return fmt.Errorf("server run error: %w", err)
	}
	return nil
}

func (s *Server) init(ctx context.Context) error {
	// 初始化服务器

	// 1. 连接 Kubernetes API
	if err := s.initKubeClient(ctx); err != nil {
		return err
	}

	// 2. 连接数据库
	if err := s.initDatabase(ctx); err != nil {
		return err
	}

	// 3. 通过 Kubernetes Lease 获取操作权，进行数据初始化
	id := fmt.Sprintf("%s-%d", common.Hostname("node"), os.Getpid())
	rl, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		s.config.KubeClientConfig.DefaultNamespace(),
		"rlark-server-init-lock",
		s.kubeClient.CoreV1(),
		s.kubeClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: id,
		},
	)
	if err != nil {
		return fmt.Errorf("create resource lock: %w", err)
	}
	initServerDataErrorCh := make(chan error)
	defer close(initServerDataErrorCh)
	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: time.Second * 5,
		RenewDeadline: time.Second * 2,
		RetryPeriod:   time.Second * 1,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				initServerDataErrorCh <- s.initServerData(ctx)
				<-ctx.Done()
			},
			OnStoppedLeading: func() {},
			OnNewLeader:      func(identity string) {},
		},
	})
	if err != nil {
		return fmt.Errorf("create leader elector: %w", err)
	}

	leCtx, leCancel := context.WithCancel(ctx)
	go le.Run(leCtx)

	err = <-initServerDataErrorCh
	leCancel()
	if err != nil {
		return err
	}

	// 4. 初始化本实例的一些信息
	if err := s.initSelfInstance(ctx); err != nil {
		return err
	}

	return nil
}

func (s *Server) initKubeClient(ctx context.Context) error {
	var err error
	s.restConfig, err = s.config.KubeClientConfig.BuildRestConfig()
	if err != nil {
		return fmt.Errorf("build Kubernetes client config: %w", err)
	}
	s.kubeClient, err = kubernetes.NewForConfig(s.restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes client: %w", err)
	}
	s.rlarkClient, err = versioned.NewForConfig(s.restConfig)
	if err != nil {
		return fmt.Errorf("create RLark client: %w", err)
	}
	kubeProxy, err := NewKubeProxy(s.restConfig)
	if err != nil {
		return fmt.Errorf("create Kubernetes proxy: %w", err)
	}
	s.kubeHandler = kubeProxy.GetHandler()

	// init listers
	factory := externalversions.NewSharedInformerFactory(s.rlarkClient, 30*time.Minute)
	s.podInformer = factory.Rlinf().V1alpha1().Pods().Informer()
	s.podLister = factory.Rlinf().V1alpha1().Pods().Lister()
	s.podCache = caches.NewPodCache(s.podInformer)
	factory.Start(ctx.Done())
	return nil
}

func (s *Server) initDatabase(ctx context.Context) error {
	if s.config.DBConfigPath == "" {
		logger := log.FromContext(ctx)
		logger.Error(nil, "RLark server is running without persistent storage.")
		return nil
	}
	var err error
	s.dbClient, err = db.OpenFromFileConfig(s.config.DBConfigPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	s.rcStore = db.NewRevokedCertificateStore(s.dbClient.DB)
	s.userKeyStore = db.NewSSHUserKeyStore(s.dbClient.DB)
	return nil
}

func (s *Server) initServerData(ctx context.Context) error {
	// 假设已经获取了操作权，开始进行数据初始化

	// 1. 检查 Kubernetes 是否安装了必要的 CRD，以及 CRD 的版本是否正确。
	// TODO

	// 2. 检查 Kubernetes 中保存的各个配置，如果没有则初始化。
	if err := s.loadTLSCA(ctx); err != nil {
		return fmt.Errorf("load TLS CA: %w", err)
	}
	if err := s.loadTLSConfig(ctx); err != nil {
		return fmt.Errorf("load TLS config: %w", err)
	}
	if err := s.initCAConfigs(ctx); err != nil {
		return fmt.Errorf("initialize CA configs: %w", err)
	}
	if err := s.signAdminCert(ctx); err != nil {
		return fmt.Errorf("sign admin certificate: %w", err)
	}

	// 3. 检查数据库中的表结构和索引，如果不正确则进行迁移。
	if s.dbClient != nil {
		if err := s.dbClient.Migrate(ctx); err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
	}

	return nil
}

func (s *Server) initSelfInstance(ctx context.Context) error {
	if err := s.initPeerTransport(ctx); err != nil {
		return fmt.Errorf("initialize peer transport: %w", err)
	}

	// wait for informers to sync
	for {
		if !s.podInformer.HasSynced() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		break
	}

	return nil
}
