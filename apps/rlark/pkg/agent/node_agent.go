package agent

import (
	"context"
	"encoding/json"
	"time"

	"golang.org/x/sync/errgroup"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	"github.com/rlinf/rlark/api/kubeclients/clientset/versioned"
	"github.com/rlinf/rlark/api/kubeclients/informers/externalversions"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
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
	if n.a.config.ImagePullEnabled && imagepull.RuntimeFor(agentType) != imagepull.RuntimeNone {
		rt := imagepull.NewImagePuller(
			imagepull.RuntimeFor(agentType),
			n.a.config.ContainerdSocket,
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
			logger.Info("image pre-pull enabled", "agentType", agentType, "nodeName", n.a.config.NodeName)
		}
	} else {
		logger.Info("image pre-pull disabled", "agentType", agentType, "enabled", n.a.config.ImagePullEnabled)
	}

	factory.Start(ctx.Done())

	// Wait for caches to sync before serving.
	informers := []cache.SharedIndexInformer{domainPeerInformer, managementPodInformer}
	if taskInformer != nil {
		informers = append(informers, taskInformer)
	}
	if err := waitForCacheSync(ctx, informers); err != nil {
		return err
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

	var eg errgroup.Group
	eg.Go(func() error {
		return nodeserver.Run(ctx)
	})
	if imgPuller != nil {
		eg.Go(func() error {
			return imgPuller.Run(ctx)
		})
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
	eg.Go(func() error {
		return eventsWatcher.Run(ctx)
	})
	return eg.Wait()
}

// waitForCacheSync blocks until all informers have synced or ctx is cancelled.
func waitForCacheSync(ctx context.Context, informers []cache.SharedIndexInformer) error {
	for {
		allSynced := true
		for _, inf := range informers {
			if !inf.HasSynced() {
				allSynced = false
				break
			}
		}
		if allSynced {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
