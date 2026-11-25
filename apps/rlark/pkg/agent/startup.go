package agent

import (
	"context"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sync"
)

const startupPreflightTimeout = 30 * time.Second

func tunnelRoles(config Config) []string {
	if config.Mode == "node" {
		return []string{"node-agent:" + config.NodeName}
	}
	roles := []string{""}
	if config.HasNode() {
		roles = append(roles, "node-agent:"+config.NodeName)
	}
	return roles
}

type startupRole interface {
	Build(context.Context) error
	Run(context.Context) error
}

// buildRoles completes every selected role before any role starts. The node
// role is not coupled to cluster leadership or a manager's runnable gating.
func buildRoles(ctx context.Context, config Config, cluster, node startupRole) ([]func(context.Context) error, error) {
	var runs []func(context.Context) error
	if config.Mode == "cluster" {
		if cluster == nil {
			return nil, fmt.Errorf("cluster role is missing")
		}
		if err := cluster.Build(ctx); err != nil {
			return nil, fmt.Errorf("build cluster role: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		runs = append(runs, cluster.Run)
	}
	if config.HasNode() {
		if node == nil {
			return nil, fmt.Errorf("node role is missing")
		}
		if err := node.Build(ctx); err != nil {
			return nil, fmt.Errorf("build node role: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		runs = append(runs, node.Run)
	}
	return runs, nil
}

// preflight uses only reads. Local checks precede registration, management
// checks follow the bootstrap tunnel's registration handshake. It
// checks the actual role dependencies; it is not an authorization guarantee for
// all future operations or proof that replicas point to the same backend.
func (a *Agent) preflight(ctx context.Context) error {
	if err := a.localPreflight(ctx); err != nil {
		return err
	}
	return a.managementPreflight(ctx)
}

func (a *Agent) localPreflight(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, startupPreflightTimeout)
	defer cancel()
	if a.config.ClientConfig.ServerNamespace == "" {
		return fmt.Errorf("certificate-derived management scope is required")
	}
	reads := []func(context.Context) error{func(ctx context.Context) error {
		return a.localKubeClient.Discovery().RESTClient().Get().AbsPath("/version").Do(ctx).Error()
	}}
	if a.config.HasNode() {
		reads = append(reads, func(ctx context.Context) error {
			_, err := a.localKubeClient.CoreV1().Nodes().Get(ctx, a.config.NodeName, metav1.GetOptions{})
			return err
		})
	}
	for _, read := range reads {
		if err := read(ctx); err != nil {
			return &InputError{Message: "local agent dependency preflight failed", Cause: err}
		}
	}
	return nil
}

func (a *Agent) managementPreflight(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, startupPreflightTimeout)
	defer cancel()
	var reads []func(context.Context) error
	scope := a.config.ClientConfig.ServerNamespace
	options := metav1.ListOptions{Limit: 1}
	if a.config.Mode == "cluster" {
		reads = append(reads, func(ctx context.Context) error {
			_, err := a.managementClient.RlinfV1alpha1().Tasks(scope).List(ctx, options)
			return err
		}, func(ctx context.Context) error {
			_, err := a.managementClient.RlinfV1alpha1().ClusterUpgradeJobs(scope).List(ctx, options)
			return err
		})
	}
	if a.config.HasNode() {
		reads = append(reads,
			func(ctx context.Context) error {
				_, err := a.managementClient.RlinfV1alpha1().DomainPeers(scope).List(ctx, options)
				return err
			},
			func(ctx context.Context) error {
				_, err := a.managementClient.RlinfV1alpha1().Pods(scope).List(ctx, options)
				return err
			},
		)
		if a.config.ImagePullEnabled {
			reads = append(reads, func(ctx context.Context) error {
				_, err := a.managementClient.RlinfV1alpha1().Tasks(scope).List(ctx, options)
				return err
			})
		}
	}
	for _, read := range reads {
		if err := waitManagementAccess(ctx, read); err != nil {
			return &InputError{Message: "agent dependency preflight failed; verify API access, local node and certificate scope", Cause: err}
		}
	}
	return nil
}

func waitManagementAccess(ctx context.Context, read func(context.Context) error) error {
	for {
		err := read(ctx)
		if err == nil {
			return nil
		}
		if !apierrors.IsForbidden(err) && !apierrors.IsNotFound(err) && !apierrors.IsServiceUnavailable(err) && !apierrors.IsTimeout(err) && !apierrors.IsServerTimeout(err) {
			return err
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(ctx.Err(), err)
		case <-timer.C:
		}
	}
}

// runWithRegistration keeps the bootstrap tunnel supervised throughout startup
// and execution. No business worker starts before registration and preparation.
func runWithRegistration(ctx context.Context, tunnel func(context.Context, func()) error, prepare func(context.Context) ([]func(context.Context) error, error)) error {
	ready := make(chan struct{})
	var once sync.Once
	return supervise(ctx,
		func(ctx context.Context) error { return tunnel(ctx, func() { once.Do(func() { close(ready) }) }) },
		func(ctx context.Context) error {
			waitCtx, cancel := context.WithTimeout(ctx, startupPreflightTimeout)
			defer cancel()
			select {
			case <-waitCtx.Done():
				return fmt.Errorf("wait for agent registration: %w", waitCtx.Err())
			case <-ready:
			}
			runs, err := prepare(ctx)
			if err != nil {
				return err
			}
			return supervise(ctx, runs...)
		},
	)
}
