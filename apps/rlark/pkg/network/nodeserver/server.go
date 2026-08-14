package nodeserver

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// PodCred holds pod credentials.
type PodCred interface {
	IP() string
	IPPrefixLength() int
}

// CredGetter retrieves values.
type CredGetter[C PodCred] func(ctx context.Context, pid int32) (C, error)

// DialGetter retrieves values.
type DialGetter[C PodCred] func(ctx context.Context, cred C, host string, query url.Values) (utils.Dial, error)

// HostsGetter retrieves values.
type HostsGetter[C PodCred] func(ctx context.Context, cred C) (map[string]string, error)

// NodeServer is a server.
type NodeServer[C PodCred] struct {
	config             Config
	getCred            CredGetter[C]
	getDial            DialGetter[C]
	getHosts           HostsGetter[C]
	marshalCred        func(C) (string, error)
	unmarshalCred      func(string) (C, error)
	localServiceDialer utils.Dial
}

// NewNodeServer creates a new NodeServer.
func NewNodeServer[C PodCred](
	config Config,
	getCred CredGetter[C], getDial DialGetter[C], getHosts HostsGetter[C],
	marshalCred func(C) (string, error), unmarshalCred func(string) (C, error),
) *NodeServer[C] {
	return &NodeServer[C]{
		config:        config,
		getCred:       getCred,
		getDial:       getDial,
		getHosts:      getHosts,
		marshalCred:   marshalCred,
		unmarshalCred: unmarshalCred,
	}
}

func (s *NodeServer[C]) startLocalService(ctx context.Context) error {
	l, d := utils.NetPipeWithBuffer(65536)
	s.localServiceDialer = d
	r := gin.New()
	r.Use(gin.Recovery())
	r.GET("/get_ip", s.handleGetIP)
	r.GET("/get_hosts", s.handleGetHosts)

	srv := http.Server{Handler: r}
	go func() {
		_ = srv.Serve(l)
	}()
	return nil
}

// Run runs the component.
func (s *NodeServer[C]) Run(ctx context.Context) error {
	logger := log.FromContext(ctx)
	if err := s.startLocalService(ctx); err != nil {
		return fmt.Errorf("start local service: %w", err)
	}
	l, err := s.config.Listen()
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()

	logger.Info("Node server listening", "address", s.config.UnixSocketAddress)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			conn, err := l.Accept()
			if err != nil {
				return err
			}
			pid, err := GetPeerProcess(conn)
			if err != nil {
				logger.Error(nil, "Failed to get peer process", "err", err)
				_ = conn.Close()
				continue
			}
			cred, err := s.getCred(ctx, pid)
			if err != nil {
				logger.Error(nil, "Failed to get credentials", "pid", pid, "err", err)
				_ = conn.Close()
				continue
			}
			go s.handleConnection(ctx, utils.NewWrapConn(conn), cred)
		}
	}
}

// handleConnection 处理来自本地进程的连接请求，读取目标地址并通过 dialer 连接到目标。
func (s *NodeServer[C]) handleConnection(ctx context.Context, conn *utils.WrapConn, cred C) {
	logger := log.FromContext(ctx)
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	defer func() { _ = conn.Close() }()

	network, host, port, query, err := utils.ReadTargetFromConn(conn)
	if err != nil {
		logger.Error(nil, "Failed to read target from connection", "err", err)
		return
	}
	logger.V(1).Info("Received connection", "network", network, "host", host, "port", port, "query", query.Encode())

	var conn2 net.Conn
	// 如果 host 为 0.0.0.0，则表示连接的是本地服务，此时使用 localServiceDialer 连接本地服务。
	// 否则，使用 getDial 获取到目标的 dialer，并连接到目标。
	if host == "0.0.0.0" {
		credName, err := s.marshalCred(cred)
		if err != nil {
			logger.Error(nil, "Failed to marshal credentials", "err", err)
			return
		}
		ctx = utils.WithRemoteAddr(ctx, &net.UnixAddr{
			Net:  "pod",
			Name: credName,
		})
		conn2, err = s.localServiceDialer(ctx)
		if err != nil {
			logger.Error(nil, "Failed to connect to local service", "err", err)
			return
		}
	} else {
		dial, err := s.getDial(ctx, cred, host, query)
		if err != nil {
			logger.Error(nil, "Failed to get target", "host", host, "err", err)
			return
		}
		conn2, err = dial(ctx)
		if err != nil {
			logger.Error(nil, "Failed to connect to target", "host", host, "port", port, "err", err)
			return
		}
		defer func() { _ = conn2.Close() }()

		targetUrl := &url.URL{
			Scheme:   network,
			Host:     net.JoinHostPort(host, port),
			RawQuery: query.Encode(),
		}
		proxyData := []byte(targetUrl.String() + "\n")
		if _, err := conn2.Write(proxyData); err != nil {
			logger.Error(nil, "Failed to write target to proxy", "target", targetUrl.String(), "err", err)
			return
		}
	}

	go func() {
		_, _ = io.Copy(conn2, conn)
	}()
	_, _ = io.Copy(conn, conn2)
}

func (s *NodeServer[C]) handleGetIP(ctx *gin.Context) {
	// 按照上面 Dial 的逻辑，这里 RemoteAddr 获取到的格式为 marshaledCred。
	cred, err := s.unmarshalCred(ctx.Request.RemoteAddr)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ip, ipNet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", cred.IP(), cred.IPPrefixLength()))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	prefixLength, _ := ipNet.Mask.Size()
	ctx.JSON(http.StatusOK, PodIPInfo{
		IP:           ip.String(),
		PrefixLength: prefixLength,
	})
}

func (s *NodeServer[C]) handleGetHosts(ctx *gin.Context) {
	// 按照上面 Dial 的逻辑，这里 RemoteAddr 获取到的格式为 marshaledCred。
	cred, err := s.unmarshalCred(ctx.Request.RemoteAddr)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	hosts, err := s.getHosts(ctx.Request.Context(), cred)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, hosts)
}
