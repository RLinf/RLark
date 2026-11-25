package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const (
	nodeOfflineSweepInterval = 30 * time.Second
	// nodeOfflineSweepJitter spreads the wait between sweeps so that Agents
	// restarted together (upgrade, rolling restart) do not stay phase-aligned
	// and produce synchronized management API List spikes. Each wait is
	// extended by a random fraction of the interval, so a sweep never runs
	// more often than nodeOfflineSweepInterval.
	nodeOfflineSweepJitter = 0.2
	// nodeMissingConfirmationWindow is how long a management Node CR may be
	// observed without a matching local Kubernetes Node before it is deleted.
	// A local Node can disappear temporarily while it is re-created with the
	// same name, so one NotFound observation is not enough evidence.
	nodeMissingConfirmationWindow = 2 * time.Minute
	// nodeUnmanagedConfirmationWindow is shorter because removing the
	// rlark.io/enabled label is an explicit unmanage signal. One full sweep
	// interval still protects against a transient label loss.
	nodeUnmanagedConfirmationWindow = nodeOfflineSweepInterval
	// nodeStaleSinceAnnotation records when the sweeper first observed the CR
	// as stale. The push reconciler drops it when the Node is managed again.
	nodeStaleSinceAnnotation = "rlark.io/stale-since"
)

// OfflineSweeper deletes management Node CRs whose local Kubernetes Node no
// longer carries rlark.io/enabled=true. It scans RLark Node CRs that belong
// to this agent (matched by rlark.io/agent-type label) and reconciles the
// control-plane state against what the local Kubernetes API currently reports.
//
// The pushNodeReconciler never deletes Node CRs — once a Node has been
// registered it stays registered until OfflineSweeper observes that the
// local Node is no longer managed. Without this sweep a user can unmanage a
// node by removing the label and the control plane will never clean up the
// stale CR, so it continues to appear in the managed list indefinitely.
//
// Deletion is intentionally conservative: only CRs this Agent has reported at
// least once are eligible, the stale condition must persist for a confirmation
// window, and finalizers are never cleared. Finalizers belong to the
// controllers that added them; a CR that stays in Terminating while an owner
// is unavailable is an explicit operator signal, not something this Agent may
// bypass.
type OfflineSweeper struct {
	c              *Controller
	interval       time.Duration
	missingGrace   time.Duration
	unmanagedGrace time.Duration
	now            func() time.Time
}

func NewOfflineSweeper(c *Controller, interval time.Duration) *OfflineSweeper {
	if interval <= 0 {
		interval = nodeOfflineSweepInterval
	}
	return &OfflineSweeper{
		c:              c,
		interval:       interval,
		missingGrace:   nodeMissingConfirmationWindow,
		unmanagedGrace: nodeUnmanagedConfirmationWindow,
		now:            time.Now,
	}
}

func (s *OfflineSweeper) Start(ctx context.Context) error {
	// Sweep once on start, then re-sweep on a jittered interval. The jitter
	// keeps Agents that restarted together from converging on the same phase
	// after the initial run.
	wait.JitterUntilWithContext(ctx, func(ctx context.Context) {
		if err := s.Sweep(ctx); err != nil {
			log.FromContext(ctx).Error(err, "management Node offline sweep failed")
		}
	}, s.interval, nodeOfflineSweepJitter, true)
	return nil
}

func (s *OfflineSweeper) NeedLeaderElection() bool { return true }

func (s *OfflineSweeper) Sweep(ctx context.Context) error {
	var nodes rlarkv1alpha1.NodeList
	opts := &client.ListOptions{Namespace: s.c.ManagementNamespace}
	client.MatchingLabels{rlarkv1alpha1.LabelAgentType: s.c.AgentType}.ApplyToList(opts)
	if err := s.c.ManagementClient.List(ctx, &nodes, opts); err != nil {
		return err
	}

	var errs []error
	for i := range nodes.Items {
		node := &nodes.Items[i]
		logger := log.FromContext(ctx).WithValues("node", client.ObjectKeyFromObject(node))

		if !node.DeletionTimestamp.IsZero() {
			// Deletion is already in progress. Finalizers are owned by other
			// controllers and this sweeper must not force them.
			continue
		}
		if !s.agentOwns(node) {
			logger.V(1).Info("skipping management Node CR without an agent-reported Kubernetes node UID")
			continue
		}

		var local corev1.Node
		err := s.c.LocalKubeClient.Get(ctx, client.ObjectKey{Name: node.Name}, &local)
		switch {
		case err == nil:
			if local.Labels[nodeEnabledLabel] == "true" {
				s.clearStaleMarker(ctx, logger, node)
				continue
			}
			// Local Node still exists but is no longer managed — delete the CR
			// after the unmanage confirmation window so the control plane stops
			// reporting it as a registered node.
			s.requestDeletion(ctx, logger, node, s.unmanagedGrace, "local Kubernetes Node is no longer managed")
		case client.IgnoreNotFound(err) == nil:
			// Local K8s Node no longer exists — delete the management CR after
			// the missing confirmation window. The local Node may be recreated
			// with the same name, so a single NotFound is not conclusive.
			s.requestDeletion(ctx, logger, node, s.missingGrace, "local Kubernetes Node no longer exists")
		default:
			errs = append(errs, fmt.Errorf("get local Node %s: %w", node.Name, err))
		}
	}
	return errors.Join(errs...)
}

// requestDeletion deletes a stale management Node CR only after the same stale
// condition has been observed for the full confirmation window. The deletion
// is re-validated against current management and local state so a stale list
// snapshot cannot delete a Node that recovered in between.
func (s *OfflineSweeper) requestDeletion(ctx context.Context, logger logr.Logger, node *rlarkv1alpha1.Node, grace time.Duration, reason string) {
	now := s.now()
	if raw := node.Annotations[nodeStaleSinceAnnotation]; raw != "" {
		staleSince, err := time.Parse(time.RFC3339Nano, raw)
		if err == nil {
			if now.Sub(staleSince) < grace {
				return
			}
			current, stale, err := s.currentlyStale(ctx, node)
			if err != nil {
				logger.Error(err, "re-validate stale management Node CR")
				return
			}
			if !stale {
				return
			}
			s.deleteNodeCR(ctx, logger, current, reason)
			return
		}
	}
	s.markStale(ctx, logger, node, now, reason)
}

func (s *OfflineSweeper) markStale(ctx context.Context, logger logr.Logger, node *rlarkv1alpha1.Node, now time.Time, reason string) {
	original := node.DeepCopy()
	if node.Annotations == nil {
		node.Annotations = map[string]string{}
	}
	node.Annotations[nodeStaleSinceAnnotation] = now.UTC().Format(time.RFC3339Nano)
	if err := s.c.ManagementClient.Patch(ctx, node, client.MergeFrom(original)); err != nil {
		logger.Error(err, "mark management Node CR as stale")
		return
	}
	logger.V(1).Info("observed stale management Node CR; waiting for confirmation window", "reason", reason)
}

func (s *OfflineSweeper) clearStaleMarker(ctx context.Context, logger logr.Logger, node *rlarkv1alpha1.Node) {
	if node.Annotations[nodeStaleSinceAnnotation] == "" {
		return
	}
	original := node.DeepCopy()
	delete(node.Annotations, nodeStaleSinceAnnotation)
	if err := s.c.ManagementClient.Patch(ctx, node, client.MergeFrom(original)); err != nil {
		logger.Error(err, "clear stale marker on management Node CR")
	}
}

// currentlyStale re-reads the CR and confirms that both the identity and the
// stale condition are unchanged. A CR that was recreated, or a local Node that
// became managed again, cancels the pending deletion.
func (s *OfflineSweeper) currentlyStale(ctx context.Context, observed *rlarkv1alpha1.Node) (*rlarkv1alpha1.Node, bool, error) {
	var current rlarkv1alpha1.Node
	if err := s.c.ManagementClient.Get(ctx, client.ObjectKeyFromObject(observed), &current); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, false, nil
		}
		return nil, false, err
	}
	if current.UID != observed.UID || current.Annotations[nodeStaleSinceAnnotation] != observed.Annotations[nodeStaleSinceAnnotation] {
		return &current, false, nil
	}
	if !s.agentOwns(&current) {
		return &current, false, nil
	}

	var local corev1.Node
	err := s.c.LocalKubeClient.Get(ctx, client.ObjectKey{Name: current.Name}, &local)
	if err == nil {
		return &current, local.Labels[nodeEnabledLabel] != "true", nil
	}
	if client.IgnoreNotFound(err) == nil {
		return &current, true, nil
	}
	return &current, false, err
}

// deleteNodeCR requests deletion of the management Node CR. Finalizers belong
// to the controllers that added them and this Agent never clears them. A
// normal Delete lets each owner run its cleanup contract; a CR that stays in
// Terminating is surfaced to operators instead of being silently force-deleted.
func (s *OfflineSweeper) deleteNodeCR(ctx context.Context, logger logr.Logger, node *rlarkv1alpha1.Node, reason string) {
	if err := s.c.ManagementClient.Delete(ctx, node); err != nil {
		if client.IgnoreNotFound(err) != nil {
			logger.Error(err, "failed to delete stale management Node CR")
		}
		return
	}
	logger.Info("requested deletion of stale management Node CR", "reason", reason, "finalizers", node.Finalizers)
}

// agentOwns reports whether this Agent has successfully reported the Node at
// least once. KubernetesNodeUID is written only by the push reconciler through
// the status subresource, so a non-empty value proves the CR is an Agent
// observation rather than management-created registration intent that this
// Agent must not delete.
func (s *OfflineSweeper) agentOwns(node *rlarkv1alpha1.Node) bool {
	return node.Status.KubernetesNodeUID != ""
}
