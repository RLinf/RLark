package agent

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/inuse"
	"github.com/rlinf/rlark/apps/rlark/astribot/nodeagent"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/imagepull"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

// firstSyncTimeout is how long the node agent's first watches may take, as
// long as a controller's may in the cluster agent (controller-runtime's CacheSyncTimeout).
const firstSyncTimeout = 2 * time.Minute

// runtimeSocket is the container runtime socket images are pre-pulled through, and whether they are pre-pulled.
// One DaemonSet serves Nodes of different runtimes: without --containerd-socket each node Agent takes the socket
// its own Node records, reached through the host /run the DaemonSet mounts.
func (n *nodeAgent) runtimeSocket(ctx context.Context, agentType rlarkv1alpha1.AgentType) (string, bool, error) {
	logger := log.FromContext(ctx).WithName("nodeAgent")
	runtime := imagepull.RuntimeFor(agentType)
	pull := n.a.config.ImagePullEnabled && runtime != imagepull.RuntimeNone
	socket := n.a.config.ContainerdSocket
	if !pull || runtime != imagepull.RuntimeContainerd || socket != "" {
		return socket, pull, nil
	}
	// On a Node that has just joined the agent can start before the Pod network reaches the
	// API server, so the first reads may fail.
	var node *corev1.Node
	if err := wait.PollUntilContextCancel(ctx, 2*time.Second, true, func(ctx context.Context) (bool, error) {
		var err error
		node, err = n.a.localKubeClient.CoreV1().Nodes().Get(ctx, n.a.config.NodeName, metav1.GetOptions{})
		if err != nil {
			logger.Error(err, "get own Node for its runtime socket; retrying", "node", n.a.config.NodeName)
		}
		return err == nil, nil
	}); err != nil {
		return "", false, fmt.Errorf("get Node %s for its runtime socket: %w", n.a.config.NodeName, err)
	}
	socket, err := nodeagent.HostRunSocket(node)
	if err != nil {
		logger.Error(err, "image pre-pull disabled")
		return "", false, nil
	}
	return socket, true, nil
}

// waitForFirstSync waits for the node agent's first watches, which then says it has them. A node agent whose
// first watches do not sync is not left waiting with its tunnel up: it ends, and its DaemonSet starts it anew.
func (n *nodeAgent) waitForFirstSync(ctx context.Context, informers []cache.SharedIndexInformer) error {
	syncContext, stopWaiting := context.WithTimeout(ctx, firstSyncTimeout)
	defer stopWaiting()
	if err := waitForCacheSync(syncContext, informers); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("the first watches of the management plane did not sync in %s", firstSyncTimeout)
	}
	n.a.ready.set(partNodeWatches, true)
	return nil
}

// startAstribot returns the group the node agent's parts run in: the node agent is all of them, and when one stops,
// the others are stopped and the agent ends. Our part is the machine's in-use directory, reported on its Kubernetes
// Node for what decides in the cluster (updates of per-node Tasks that wait for an idle node). The host's /run is
// the DaemonSet's mount.
func (n *nodeAgent) startAstribot(ctx context.Context) (*together, context.Context) {
	parts, ctx := newTogether(ctx)
	if n.a.localKubeClient != nil {
		reporter := &inuse.Reporter{Directory: nodeagent.HostRun + "/rlark/in-use", NodeName: n.a.config.NodeName, Nodes: n.a.localKubeClient.CoreV1().Nodes(), Interval: inuse.Interval}
		parts.Go(func() error { return reporter.Run(ctx) })
	}
	return parts, ctx
}
