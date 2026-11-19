package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"

	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	"github.com/rlinf/rlark/api/kubeclients/informers/externalversions"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/inuse"
	"github.com/rlinf/rlark/apps/rlark/astribot/nodeagent"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/container"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/imagepull"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/nodeevents"
	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/network/nodeserver"
)

// node agent
// 节点级别的 Agent，主要负责节点和控制面之间的通信和管理.
type nodeAgent struct {
	a *Agent
}

// nodeProgressPatcher implements imagepull.ProgressPatcher using the
// management versioned client to patch node status via merge patch.
type nodeProgressPatcher struct {
	client versioned.Interface
}

// newNodeProgressPatcher creates a progress patcher backed by the management client.
func newNodeProgressPatcher(client versioned.Interface) *nodeProgressPatcher {
	return &nodeProgressPatcher{client: client}
}

// PatchPullProgress implements the imagepull.ProgressPatcher interface.
// It sends a JSON merge patch to the status subresource to update the
// pullProgress field on the Node status. The "status" subresource MUST be
// specified: nodes.rlinf.io enables subresources.status, so a main-resource
// Patch on {"status":...} would be silently stripped by the API server.
// The body keeps the "status" wrapper because the status subresource
// applies the merge patch to the whole object (only .status modifications
// are accepted), not to the .status sub-object directly.
func (p *nodeProgressPatcher) PatchPullProgress(ctx context.Context, nodeName, namespace string, progresses []rlarkv1alpha1.PullProgress) error {
	patchBytes, err := buildPullProgressPatch(progresses)
	if err != nil {
		return err
	}
	_, err = p.client.RlinfV1alpha1().Nodes(namespace).Patch(
		ctx, nodeName, types.MergePatchType, patchBytes, metav1.PatchOptions{}, "status",
	)
	return err
}

// buildPullProgressPatch creates a JSON merge patch body for the pullProgress
// field on the status subresource. The "status" wrapper is required because
// the status subresource endpoint applies the patch to the whole object and
// only permits .status modifications.
func buildPullProgressPatch(progresses []rlarkv1alpha1.PullProgress) ([]byte, error) {
	patch := map[string]interface{}{
		"status": map[string]interface{}{
			"pullProgress": progresses,
		},
	}
	return json.Marshal(patch)
}

// nodeEventsPatcher implements nodeevents.Patcher using the management
// versioned client to patch Node.status.events via the status subresource.
// It mirrors nodeProgressPatcher so events reporting rides the same
// transport/auth as pull-progress reporting.
type nodeEventsPatcher struct {
	client versioned.Interface
}

// newNodeEventsPatcher creates an events patcher backed by the management client.
func newNodeEventsPatcher(client versioned.Interface) *nodeEventsPatcher {
	return &nodeEventsPatcher{client: client}
}

// PatchNodeEvents implements the nodeevents.Patcher interface. It sends a
// JSON merge patch to the status subresource to update the events field on
// the Node status. The "status" subresource MUST be specified: like
// pullProgress, a main-resource Patch would be silently stripped by the
// API server because nodes.rlinf.io enables subresources.status.
func (p *nodeEventsPatcher) PatchNodeEvents(ctx context.Context, nodeName, namespace string, events []rlarkv1alpha1.NodeEvent) error {
	patchBytes, err := nodeevents.BuildEventsMergePatch(events)
	if err != nil {
		return err
	}
	_, err = p.client.RlinfV1alpha1().Nodes(namespace).Patch(
		ctx, nodeName, types.MergePatchType, patchBytes, metav1.PatchOptions{}, "status",
	)
	return err
}

// Run runs the component.
func (n *nodeAgent) Run(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("nodeAgent")

	factory := externalversions.NewSharedInformerFactoryWithOptions(
		n.a.managementClient,
		30*time.Minute,
		externalversions.WithNamespace(n.a.config.ClientConfig.ServerNamespace),
	)
	domainPeerInformer := factory.Rlinf().V1alpha1().DomainPeers().Informer()
	domainPeerLister := factory.Rlinf().V1alpha1().DomainPeers().Lister()
	managementPodInformer := factory.Rlinf().V1alpha1().Pods().Informer()
	managementPodLister := factory.Rlinf().V1alpha1().Pods().Lister()

	// Set up the image pre-puller. Event handlers must be registered before
	// factory.Start so the initial list is not missed.
	var taskInformer cache.SharedIndexInformer
	var imgPuller *imagepull.Puller
	agentType := rlarkv1alpha1.AgentType(n.a.config.AgentType)
	runtime := imagepull.RuntimeFor(agentType)
	pull := n.a.config.ImagePullEnabled && runtime != imagepull.RuntimeNone
	socket := n.a.config.ContainerdSocket
	if pull && runtime == imagepull.RuntimeContainerd && socket == "" {
		// One DaemonSet serves Nodes of different runtimes: each node Agent takes the socket its own
		// Node records, reached through the host /run the DaemonSet mounts.
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
			return fmt.Errorf("get Node %s for its runtime socket: %w", n.a.config.NodeName, err)
		}
		var err error
		if socket, err = nodeagent.HostRunSocket(node); err != nil {
			logger.Error(err, "image pre-pull disabled")
			pull = false
		}
	}
	if pull {
		rt := imagepull.NewImagePuller(
			runtime,
			socket,
			n.a.config.ContainerdNamespace,
			logger,
		)
		if rt == nil {
			// NewImagePuller returns nil when the container runtime client
			// cannot be created (e.g. containerd socket unavailable); skip
			// the puller so workers never dereference a nil runtime.
			logger.Info("image pre-pull disabled: container runtime client unavailable", "agentType", agentType)
		} else {
			taskInformer = factory.Rlinf().V1alpha1().Tasks().Informer()
			patcher := newNodeProgressPatcher(n.a.managementClient)
			imgPuller = imagepull.NewPuller(imagepull.PullerConfig{
				Runtime:             rt,
				AgentType:           agentType,
				NodeName:            n.a.config.NodeName,
				LocalKube:           n.a.localKubeClient,
				ProgressPatcher:     patcher,
				ManagementNamespace: n.a.config.ClientConfig.ServerNamespace,
			}, taskInformer, logger)
			logger.Info("image pre-pull enabled", "agentType", agentType, "nodeName", n.a.config.NodeName, "socket", socket)
		}
	} else {
		logger.Info("image pre-pull disabled", "agentType", agentType, "enabled", n.a.config.ImagePullEnabled)
	}

	factory.Start(ctx.Done())

	// Wait for caches to sync before serving.
	informers := []cache.InformerSynced{domainPeerInformer.HasSynced, managementPodInformer.HasSynced}
	if taskInformer != nil {
		informers = append(informers, taskInformer.HasSynced)
	}
	// A node agent whose first watches do not sync is not left waiting with
	// its tunnel up: it ends, and its DaemonSet starts it anew.
	syncContext, stopWaiting := context.WithTimeout(ctx, firstSyncTimeout)
	synced := cache.WaitForCacheSync(syncContext.Done(), informers...)
	stopWaiting()
	if !synced {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("the first watches of the management plane did not sync in %s", firstSyncTimeout)
	}
	n.a.readiness.set(partNodeWatches, true)

	networkAdapter := container.NewContainerNetworkAdapter(
		n.a.config.ClientConfig.ServerNamespace,
		domainPeerLister,
		managementPodLister,
		n.a.config.RLarkServerSSHAddress,
		n.a.config.RLarkServerSSHHostKey,
		n.a.config.SSHMaxConnectionsPerDomain,
		n.a.config.EnableSameClusterDirect,
		n.a.config.EnableCrossClusterDirect,
		n.a.config.KubeletDir,
	)
	defer func() { _ = networkAdapter.Close() }()
	nodeserver := nodeserver.NewNodeServer(
		n.a.config.NodeServerConfig,
		networkAdapter.GetContainerNetworkCred,
		networkAdapter.GetContainerNetworkDial,
		networkAdapter.GetContainerNetworkDomainHosts,
		container.MarshalContainerNetworkCred,
		container.UnmarshalContainerNetworkCred,
	)

	// The node agent is all of these: when one stops, the others are stopped and the agent ends.
	parts := []func(context.Context) error{func(ctx context.Context) error {
		return nodeserver.Run(ctx, n.a)
	}}
	if imgPuller != nil {
		parts = append(parts, imgPuller.Run)
	}
	// node events watcher: surfaces DiskPressure and other Warning events
	// (plus Pulling/Scheduling Normal events) to Node.status.events so the
	// control plane can show them on Pending Tasks. Best-effort: the watcher
	// logs and skips on misconfiguration rather than failing node-agent
	// startup, mirroring the image-puller behavior.
	eventsWatcher := nodeevents.NewWatcher(nodeevents.Config{
		NodeName:  n.a.config.NodeName,
		Namespace: n.a.config.ClientConfig.ServerNamespace,
		LocalKube: n.a.localKubeClient,
		Patcher:   newNodeEventsPatcher(n.a.managementClient),
	}, logger)
	parts = append(parts, eventsWatcher.Run)
	// The machine's in-use directory, reported on its Kubernetes Node for what
	// decides in the cluster (updates of per-node Tasks that wait for an idle
	// node). The host's /run is the DaemonSet's mount.
	if n.a.localKubeClient != nil {
		reporter := &inuse.Reporter{Directory: nodeagent.HostRun + "/rlark/in-use", NodeName: n.a.config.NodeName, Nodes: n.a.localKubeClient.CoreV1().Nodes(), Interval: inuse.Interval}
		parts = append(parts, reporter.Run)
	}
	return startTogether(ctx, parts...)
}
