package container

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	gocache "github.com/patrickmn/go-cache"
	listerv1alpha1 "github.com/rlinf/rlark/api/kubeclients/listers/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/utils"
)

// containerNetworkCred 是容器网络凭证，记录 Pod 所属 Domain 的网络信息，
// 用于在 NodeServer 中标识连接发起方并做路由决策。
type containerNetworkCred struct {
	// containerID        string
	PodID              string `json:"pid,omitempty"`
	DomainID           string `json:"did,omitempty"`
	DomainIP           string `json:"dip,omitempty"`
	DomainPrefixLength int    `json:"dpl,omitempty"`
}

// IP is an exported method.
func (c containerNetworkCred) IP() string {
	return c.DomainIP
}

// IPPrefixLength is an exported method.
func (c containerNetworkCred) IPPrefixLength() int {
	return c.DomainPrefixLength
}

// MarshalContainerNetworkCred 将容器网络凭证序列化为 JSON 后做 base64 编码。
func MarshalContainerNetworkCred(cred *containerNetworkCred) (string, error) {
	b, err := json.Marshal(cred)
	if err != nil {
		return "", fmt.Errorf("marshal container network cred: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// UnmarshalContainerNetworkCred 将 base64 编码的字符串解码为 JSON 并反序列
// 化为 containerNetworkCred。
func UnmarshalContainerNetworkCred(s string) (*containerNetworkCred, error) {
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode container network cred: %w", err)
	}
	var cred containerNetworkCred
	if err := json.Unmarshal(data, &cred); err != nil {
		return nil, fmt.Errorf("unmarshal container network cred: %w", err)
	}
	return &cred, nil
}

// containerNetworkAdapter 基于 K8s lister 提供容器网络能力：
// 根据 PID 查询网络凭证、根据凭证构造到目标 Pod 的拨号器，以及查询 Domain 内的 hosts 映射。
type containerNetworkAdapter struct {
	globalNamespace     string
	domainPeerLister    listerv1alpha1.DomainPeerLister
	managementPodLister listerv1alpha1.PodLister
	sshAddr             string
	sshDialer           *SSHDialer

	enableSameClusterDirect  bool
	enableCrossClusterDirect bool

	kubeletDir  string
	podUIDCache *gocache.Cache
}

// NewContainerNetworkAdapter creates a new ContainerNetworkAdapter.
func NewContainerNetworkAdapter(
	globalNamespace string,
	domainPeerLister listerv1alpha1.DomainPeerLister,
	managementPodLister listerv1alpha1.PodLister,
	sshAddr string,
	sshHostKey string,
	enableSameClusterDirect bool,
	enableCrossClusterDirect bool,
	kubeletDir string,
) *containerNetworkAdapter {
	hostKeyCallback := makeHostKeyCallback(sshHostKey)
	return &containerNetworkAdapter{
		globalNamespace:     globalNamespace,
		domainPeerLister:    domainPeerLister,
		managementPodLister: managementPodLister,
		sshAddr:             sshAddr,
		sshDialer: NewSSHDialer(SSHDialerConfig{
			HostKeyCallback: hostKeyCallback,
		}),
		enableSameClusterDirect:  enableSameClusterDirect,
		enableCrossClusterDirect: enableCrossClusterDirect,
		kubeletDir:               kubeletDir,
		podUIDCache:              gocache.New(5*time.Minute, 10*time.Minute),
	}
}

// makeHostKeyCallback 解析 SSH 主机公钥字符串，返回对应的 HostKeyCallback。
// 空字符串时使用 InsecureIgnoreHostKey（仅开发环境）。
func makeHostKeyCallback(sshHostKey string) ssh.HostKeyCallback {
	if sshHostKey == "" {
		return ssh.InsecureIgnoreHostKey()
	}
	pk, _, _, _, err := ssh.ParseAuthorizedKey([]byte(sshHostKey))
	if err != nil {
		// 解析失败时 fallback 到 insecure，避免服务完全无法启动
		return ssh.InsecureIgnoreHostKey()
	}
	return ssh.FixedHostKey(pk)
}

func (a *containerNetworkAdapter) getPodUIDFromSource(source string) string {
	// source 即为从进程中解析出来的 Pod UID
	if a.kubeletDir == "" {
		return source
	}
	if cached, found := a.podUIDCache.Get(source); found {
		if podUID, ok := cached.(string); ok {
			return podUID
		}
	}
	podUID := readPodUIDFromKubeAPIAccess(a.kubeletDir, source)
	if podUID == "" {
		return source
	}
	a.podUIDCache.Set(source, podUID, gocache.DefaultExpiration)
	return podUID
}

// GetContainerNetworkCred returns the containerNetworkCred.
func (a *containerNetworkAdapter) GetContainerNetworkCred(ctx context.Context, pid int32) (*containerNetworkCred, error) {
	// 根据 pid 获取进程所属容器/pod 的信息，获取其所属的 domain，以及 domain 的网络信息
	sourceType, source, err := a.getProcessNetworkInfo(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("get process network info: %w", err)
	}

	switch sourceType {
	case "pod":
		uid := a.getPodUIDFromSource(source)
		domain, err := a.getPodDomainByPodUID(ctx, uid)
		if err != nil {
			return nil, fmt.Errorf("get pod domain by pod UID: %w", err)
		}
		dpeer, err := a.domainPeerLister.DomainPeers(a.globalNamespace).Get(domain)
		if err != nil {
			return nil, fmt.Errorf("get domain peer: %w", err)
		}
		ret := containerNetworkCred{
			PodID:              uid,
			DomainID:           dpeer.Name,
			DomainPrefixLength: dpeer.Spec.PrefixLen,
		}
		for _, pod := range dpeer.Spec.Pods {
			if pod.UID == uid {
				ret.DomainIP = pod.IP
				break
			}
		}
		if ret.DomainIP == "" {
			return nil, fmt.Errorf("pod %s not found in domain peer %s", uid, dpeer.Name)
		}
		return &ret, nil

	default:
		return nil, fmt.Errorf("unsupported source type: %s", sourceType)
	}
}

// getPodDomainByPodUID 通过 Pod UID（即 Pod 在控制面的名称）查询其所属的 Domain 名称。
func (a *containerNetworkAdapter) getPodDomainByPodUID(ctx context.Context, podUID string) (string, error) {
	pod, err := a.managementPodLister.Pods(a.globalNamespace).Get(podUID) // pod UID 即为 pod 在控制面的名称
	if err != nil {
		return "", err
	}
	return pod.Spec.Domain, nil
}

// GetContainerNetworkDial returns the containerNetworkDial.
func (a *containerNetworkAdapter) GetContainerNetworkDial(ctx context.Context, cred *containerNetworkCred, host string, query url.Values) (utils.Dial, error) {
	logger := log.FromContext(ctx)

	dpeer, err := a.domainPeerLister.DomainPeers(a.globalNamespace).Get(cred.DomainID)
	if err != nil {
		return nil, fmt.Errorf("get domain peer: %w", err)
	}
	var targetPod v1alpha1.DomainPodInfo
	for _, pod := range dpeer.Spec.Pods {
		if pod.Name == host || pod.IP == host {
			targetPod = pod
			break
		}
	}
	if targetPod.Node == "" || targetPod.IP == "" {
		return func(ctx context.Context) (net.Conn, error) {
			return nil, fmt.Errorf("server not found")
		}, nil
	}
	// 1. 如果在同一个集群内，可以直接通过节点访问
	if targetPod.GlobalNamespace == a.globalNamespace && a.enableSameClusterDirect {
		logger.V(1).Info("Target pod is in the same cluster, using direct node access", "targetPod", targetPod.LocalIP)
		return func(ctx context.Context) (net.Conn, error) {
			var dialer net.Dialer
			// 直接通过目标 Pod 的 LocalIP 访问其 5700 端口（proxy 端口）
			return dialer.DialContext(ctx, "tcp", net.JoinHostPort(targetPod.LocalIP, "5700"))
		}, nil
	}

	// 2. 如果不在同一个集群内，但是可以直接访问到目标节点，也可以直接通过节点访问
	if a.enableCrossClusterDirect {
		_ = "TODO"
	}

	// 3. 不在同一个集群内，且无法直接访问到目标节点，则通过控制面代理进行访问
	if agentID, ok := strings.CutPrefix(targetPod.GlobalNamespace, apis.RLarkAgentNamespacePrefix); ok {
		target := fmt.Sprintf("%s.%s.%s.agent-node:5700", targetPod.LocalIP, targetPod.Node, agentID)
		logger.V(1).Info("Target pod is in a different cluster, using control plane proxy", "targetPod", target)
		return func(ctx context.Context) (net.Conn, error) {
			return a.sshDialer.DialContext(ctx, cred.DomainID, a.sshAddr, dpeer.Spec.Cert, dpeer.Spec.Key, target)
		}, nil
	}

	return nil, fmt.Errorf("container network dialer for domain %s not implemented", cred.DomainID)
}

// GetContainerNetworkDomainHosts returns the domain hosts for the given container network credentials.
func (a *containerNetworkAdapter) GetContainerNetworkDomainHosts(ctx context.Context, cred *containerNetworkCred) (map[string]string, error) {
	dpeer, err := a.domainPeerLister.DomainPeers(a.globalNamespace).Get(cred.DomainID)
	if err != nil {
		return nil, fmt.Errorf("get domain peer: %w", err)
	}
	hosts := make(map[string]string)
	for _, pod := range dpeer.Spec.Pods {
		hosts[fmt.Sprintf("%s.%s", pod.Name, common.DomainSuffix)] = pod.IP
	}
	return hosts, nil
}
