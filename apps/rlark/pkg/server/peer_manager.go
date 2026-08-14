package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"

	"github.com/rlinf/rlark/apps/rlark/pkg/auth/cert"
)

// Variables used by the package.
var (
	ServerPeerPrefix = "rlark-server-peer-"
)

func (s *Server) initPeerTransport(ctx context.Context) error {
	if len(s.ca) == 0 {
		return fmt.Errorf("no CA available for peer transport")
	}
	signType, meta, err := s.parseSignRequest(&SignRequest{Role: "peer"})
	if err != nil {
		return fmt.Errorf("parse sign request: %w", err)
	}
	certData, err := cert.Sign(&s.ca[0], signType, meta)
	if err != nil {
		return fmt.Errorf("sign peer certificate: %w", err)
	}
	clientCert, err := tls.X509KeyPair(certData.CertPEM, certData.KeyPEM)
	if err != nil {
		return fmt.Errorf("create client certificate: %w", err)
	}

	var caCertPool *x509.CertPool
	if s.tlsCA != nil {
		caCertPool = x509.NewCertPool()
		caCertPool.AddCert(s.tlsCA.Cert)
	}

	s.defaultPeerTransport = &http.Transport{
		TLSClientConfig: &tls.Config{
			Certificates:       []tls.Certificate{clientCert},
			RootCAs:            caCertPool,
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true,
		},
	}
	return nil
}

func (s *Server) runBroadcaster(ctx context.Context) error {
	// 向 Kubernetes 集群通过 Lease 广播服务器的存在，并且检查其他服务器的信息
	// 用于构建 Peer-to-Peer 的服务器集群，确保高可用和负载均衡。

	id := ServerPeerPrefix + common.Hostname("node")
	ip := common.PodIP("localhost") + "/" + s.dialerFactory.GetPeerID() + "/" + s.dialerFactory.GetPeerToken()
	rl, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		s.config.KubeClientConfig.DefaultNamespace(),
		id,
		s.kubeClient.CoreV1(),
		s.kubeClient.CoordinationV1(),
		resourcelock.ResourceLockConfig{
			Identity: ip,
		},
	)
	if err != nil {
		return fmt.Errorf("create resource lock: %w", err)
	}

	le, err := leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          rl,
		LeaseDuration: time.Second * 30,
		RenewDeadline: time.Second * 10,
		RetryPeriod:   time.Second * 5,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(ctx context.Context) {
				s.peerBroadcasted = true
				<-ctx.Done()
			},
			OnStoppedLeading: func() {},
			OnNewLeader:      func(identity string) {},
		},
	})
	if err != nil {
		return fmt.Errorf("create leader elector: %w", err)
	}

	le.Run(ctx)
	return nil
}

func (s *Server) runPeerTunnel(ctx context.Context) error {
	logger := log.FromContext(ctx)
	var mu sync.Mutex
	leaseMap := make(map[string]string) // leaseName -> leaseIdentity(ip/peerID/peerToken)

	myLeaseName := ServerPeerPrefix + common.Hostname("node")

	setLease := func(name, identity string) {
		mu.Lock()
		defer mu.Unlock()
		if name == myLeaseName {
			return
		}
		if prev, ok := leaseMap[name]; ok && prev == identity {
			return // unchanged
		}
		fields := strings.Split(identity, "/")
		if len(fields) != 3 {
			logger.Error(nil, "Invalid lease identity", "identity", identity)
			return
		}
		ip, peerID, peerToken := fields[0], fields[1], fields[2]
		if ip == "" {
			return
		}
		url := fmt.Sprintf("ws://127.0.0.1:%d/api/peer/%s", s.config.UnsafeHTTPPort, ip)
		// identity 变更（例如对端重启后 peerID 更新）时，先移除旧 peer 条目，
		// 避免旧 peer.start goroutine 与 s.peers 条目永久泄漏。
		if prev, ok := leaseMap[name]; ok {
			if prevFields := strings.Split(prev, "/"); len(prevFields) == 3 {
				if oldPeerID := prevFields[1]; oldPeerID != peerID {
					s.dialerFactory.RemovePeer(oldPeerID)
					logger.Info("Removed stale peer", "peerID", oldPeerID, "lease", name)
				}
			}
		}
		s.dialerFactory.AddPeer(url, peerID, peerToken)
		leaseMap[name] = identity
		metrics.SetPeerConnections(len(leaseMap))
		logger.Info("Added peer", "peerID", peerID, "lease", name, "ip", ip)
	}
	unsetLease := func(name string) {
		mu.Lock()
		defer mu.Unlock()
		if identity, ok := leaseMap[name]; ok {
			fields := strings.Split(identity, "/")
			if len(fields) == 3 {
				peerID := fields[1]
				s.dialerFactory.RemovePeer(peerID)
				logger.Info("Removed peer", "peerID", peerID, "lease", name)
			}
			delete(leaseMap, name)
			metrics.SetPeerConnections(len(leaseMap))
		}
	}

	client := s.kubeClient.CoordinationV1().Leases(s.config.KubeClientConfig.DefaultNamespace())

	// 通过 Kubernetes Lease 发现其他服务器实例并建立 Peer-to-Peer 连接。
	// 每台服务器在 runBroadcaster 中创建名为 rlark-server-peer-{HOSTNAME} 的 Lease，
	// HolderIdentity 格式为 {POD_IP}/{peerID}/{peerToken}。
	// 注意：失去 HolderIdentity 时，不直接删除 Peer，防止短暂的网络波动导致 Peer 大量波动。
	// 只有当 Lease 长时间没有 HolderIdentity 时，才删除 Lease，以触发 Peer 的删除。
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// 列出所有 Lease
		leaseList, err := client.List(ctx, metav1.ListOptions{})
		if err != nil {
			logger.Error(nil, "Failed to list leases", "err", err)
			// 等待一段时间后重试，避免频繁请求 Kubernetes API 导致压力过大。
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
			continue
		}
		leaseNames := make(map[string]struct{})
		for i := range leaseList.Items {
			lease := &leaseList.Items[i]
			if !strings.HasPrefix(lease.Name, ServerPeerPrefix) {
				continue
			}
			// 如果过期太长时间，删除该 Lease，避免列表中充斥大量过期的 Lease。
			if lease.Spec.RenewTime != nil && time.Since(lease.Spec.RenewTime.Time) > time.Minute*30 {
				if err := client.Delete(ctx, lease.Name, metav1.DeleteOptions{}); err != nil {
					logger.Error(nil, "Failed to delete expired lease", "lease", lease.Name, "err", err)
				}
				continue
			}
			// 从列表中将符合条件的 Lease 添加到 Peer 列表中。
			// 这里进行检查：如果 Lease 持续没有 HolderIdentity，说明对应的服务器可能已经不可用，可以将该 Lease 删除。
			leaseNames[lease.Name] = struct{}{}
			if lease.Spec.HolderIdentity != nil {
				setLease(lease.Name, *lease.Spec.HolderIdentity)
			}
		}
		// 释放已经过期的 Lease
		for name := range leaseMap {
			if _, ok := leaseNames[name]; !ok {
				unsetLease(name)
			}
		}

		// 监听 Lease 的变化，及时更新 Peer 列表。
		// 这里限制监听的最长时间，超出时间时可以重新运行该循环。
		watcher, err := client.Watch(ctx, metav1.ListOptions{
			ResourceVersion: leaseList.ResourceVersion,
		})
		if err != nil {
			logger.Error(nil, "Failed to watch leases", "err", err)
			// 等待一段时间后重试，避免频繁请求 Kubernetes API 导致压力过大。
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Second):
			}
			continue
		}
		if err := watchLeases(ctx, watcher, setLease, unsetLease); err != nil {
			logger.Error(nil, "Lease watch ended, re-listing", "err", err)
		}
	}
}

func watchLeases(
	ctx context.Context,
	watcher watch.Interface,
	setLease func(name, identity string),
	unsetLease func(name string),
) error {
	logger := log.FromContext(ctx)
	// 设置每次 Watch 的最长时间，超过这个时间后无论如何都要重新 List 和 Watch，避免长时间的 Watch 导致状态不同步。
	ctx, cancel := context.WithTimeout(ctx, time.Minute*30)
	defer cancel()

	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}

			lease, ok := event.Object.(*coordinationv1.Lease)
			if !ok {
				continue
			}
			if !strings.HasPrefix(lease.Name, ServerPeerPrefix) {
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				if lease.Spec.HolderIdentity != nil {
					setLease(lease.Name, *lease.Spec.HolderIdentity)
				}
			case watch.Deleted:
				unsetLease(lease.Name)
			case watch.Bookmark:
				// no-op, just keep the watch alive
			case watch.Error:
				logger.Error(nil, "Lease watch error event", "event", event.Object)
				return fmt.Errorf("watch error event")
			}
		}
	}
}
