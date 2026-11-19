package node

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
)

const nodeOfflineSweepInterval = 30 * time.Second

// OfflineSweeper marks management Nodes offline when their local Kubernetes
// Node no longer has the rlark.io/enabled=true label. It only observes Nodes
// explicitly attributed to this cluster agent.
type OfflineSweeper struct {
	c        *Controller
	interval time.Duration
}

func NewOfflineSweeper(c *Controller, interval time.Duration) *OfflineSweeper {
	if interval <= 0 {
		interval = nodeOfflineSweepInterval
	}
	return &OfflineSweeper{c: c, interval: interval}
}

func (s *OfflineSweeper) Start(ctx context.Context) error {
	if err := s.Sweep(ctx); err != nil {
		log.FromContext(ctx).Error(err, "initial management Node offline sweep failed")
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.Sweep(ctx); err != nil {
				log.FromContext(ctx).Error(err, "management Node offline sweep failed")
			}
		}
	}
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
		var local corev1.Node
		err := s.c.LocalKubeClient.Get(ctx, client.ObjectKey{Name: node.Name}, &local)
		if err != nil && client.IgnoreNotFound(err) != nil {
			errs = append(errs, fmt.Errorf("get local Node %s: %w", node.Name, err))
			continue
		}
		if err == nil && local.Labels["rlark.io/enabled"] == "true" {
			continue
		}
		logger := log.FromContext(ctx).WithValues("node", client.ObjectKeyFromObject(node))
		logger.Info("local Node is no longer managed; waiting for the control-plane Lease reconciler to update connectivity")
	}
	return errors.Join(errs...)
}
