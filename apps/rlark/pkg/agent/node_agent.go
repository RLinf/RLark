package agent

import (
	"context"
	"time"

	"github.com/rlinf/rlark/api/kubeclients/informers/externalversions"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/container"
	"github.com/rlinf/rlark/apps/rlark/pkg/network/nodeserver"
)

// node agent
// 节点级别的 Agent，主要负责节点和控制面之间的通信和管理.
type nodeAgent struct {
	a *Agent
}

// Run runs the component.
func (n *nodeAgent) Run(ctx context.Context) error {
	factory := externalversions.NewSharedInformerFactoryWithOptions(
		n.a.managementClient,
		30*time.Minute,
		externalversions.WithNamespace(n.a.config.ClientConfig.ServerNamespace),
	)
	domainPeerInformer := factory.Rlinf().V1alpha1().DomainPeers().Informer()
	domainPeerLister := factory.Rlinf().V1alpha1().DomainPeers().Lister()
	managementPodInformer := factory.Rlinf().V1alpha1().Pods().Informer()
	managementPodLister := factory.Rlinf().V1alpha1().Pods().Lister()
	factory.Start(ctx.Done())

	for {
		if !domainPeerInformer.HasSynced() || !managementPodInformer.HasSynced() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}
		break
	}

	networkAdapter := container.NewContainerNetworkAdapter(
		n.a.config.ClientConfig.ServerNamespace,
		domainPeerLister,
		managementPodLister,
		n.a.config.RLarkServerSSHAddress,
		n.a.config.RLarkServerSSHHostKey,
		n.a.config.EnableSameClusterDirect,
		n.a.config.EnableCrossClusterDirect,
		n.a.config.KubeletDir,
	)
	nodeserver := nodeserver.NewNodeServer(
		n.a.config.NodeServerConfig,
		networkAdapter.GetContainerNetworkCred,
		networkAdapter.GetContainerNetworkDial,
		networkAdapter.GetContainerNetworkDomainHosts,
		container.MarshalContainerNetworkCred,
		container.UnmarshalContainerNetworkCred,
	)

	return nodeserver.Run(ctx)
}
