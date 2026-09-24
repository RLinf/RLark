package configs

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/pflag"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// LeaderElectionConfig configures a Kubernetes Lease-based leader election.
type LeaderElectionConfig struct {
	Enabled       bool
	Key           string
	Identity      string
	LeaseDuration time.Duration
	RenewDeadline time.Duration
	RetryPeriod   time.Duration
}

// ResourceLock creates the Lease lock described by the configuration.
func (c LeaderElectionConfig) ResourceLock(client kubernetes.Interface, defaultNamespace string) (resourcelock.Interface, error) {
	if err := c.Validate(defaultNamespace); err != nil {
		return nil, err
	}
	if c.Identity == "" {
		return nil, fmt.Errorf("leader election identity is required")
	}
	namespace, name, _ := c.NamespaceAndName(defaultNamespace)
	lock, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		namespace,
		name,
		client.CoreV1(),
		client.CoordinationV1(),
		resourcelock.ResourceLockConfig{Identity: c.Identity},
	)
	if err != nil {
		return nil, fmt.Errorf("create leader election resource lock: %w", err)
	}
	return lock, nil
}

// DefaultLeaderElectionConfig returns the standard leader election timings.
func DefaultLeaderElectionConfig() LeaderElectionConfig {
	return LeaderElectionConfig{
		LeaseDuration: 30 * time.Second,
		RenewDeadline: 10 * time.Second,
		RetryPeriod:   5 * time.Second,
	}
}

// SetupFlags registers leader election flags with an optional prefix.
func (c *LeaderElectionConfig) SetupFlags(fs *pflag.FlagSet, prefix string) {
	prefix = strings.TrimSuffix(prefix, "-")
	if prefix != "" {
		prefix += "-"
	}
	fs.BoolVar(&c.Enabled, prefix+"leader-election", c.Enabled, "Enable leader election")
	fs.StringVar(&c.Key, prefix+"leader-election-key", c.Key, "Leader election lock key (name or namespace/name)")
	fs.StringVar(&c.Identity, prefix+"leader-election-id", c.Identity, "Unique identity for this leader election participant")
}

// NamespaceAndName resolves a key in name or namespace/name form.
func (c LeaderElectionConfig) NamespaceAndName(defaultNamespace string) (string, string, error) {
	parts := strings.Split(c.Key, "/")
	switch len(parts) {
	case 1:
		if parts[0] == "" {
			return "", "", fmt.Errorf("leader election key name is required")
		}
		return defaultNamespace, parts[0], nil
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", "", fmt.Errorf("leader election key must be name or namespace/name, got %q", c.Key)
		}
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("leader election key must be name or namespace/name, got %q", c.Key)
	}
}

// Validate validates the lock key and any explicitly configured timings.
func (c LeaderElectionConfig) Validate(defaultNamespace string) error {
	if _, _, err := c.NamespaceAndName(defaultNamespace); err != nil {
		return err
	}
	durations := []time.Duration{c.LeaseDuration, c.RenewDeadline, c.RetryPeriod}
	configured := 0
	for _, duration := range durations {
		if duration < 0 {
			return fmt.Errorf("leader election durations must not be negative")
		}
		if duration > 0 {
			configured++
		}
	}
	if configured != 0 && configured != len(durations) {
		return fmt.Errorf("leader election timings must be configured together")
	}
	if configured > 0 && c.LeaseDuration <= c.RenewDeadline {
		return fmt.Errorf("leader election lease duration must be greater than renew deadline")
	}
	if configured > 0 && c.RenewDeadline <= c.RetryPeriod {
		return fmt.Errorf("leader election renew deadline must be greater than retry period")
	}
	return nil
}

// Build creates a client-go leader election configuration using a Lease lock.
func (c LeaderElectionConfig) Build(client kubernetes.Interface, defaultNamespace string, callbacks leaderelection.LeaderCallbacks) (leaderelection.LeaderElectionConfig, error) {
	if err := c.Validate(defaultNamespace); err != nil {
		return leaderelection.LeaderElectionConfig{}, err
	}
	if c.LeaseDuration == 0 {
		return leaderelection.LeaderElectionConfig{}, fmt.Errorf("leader election timings are required")
	}
	lock, err := c.ResourceLock(client, defaultNamespace)
	if err != nil {
		return leaderelection.LeaderElectionConfig{}, err
	}
	return leaderelection.LeaderElectionConfig{
		Lock:          lock,
		LeaseDuration: c.LeaseDuration,
		RenewDeadline: c.RenewDeadline,
		RetryPeriod:   c.RetryPeriod,
		Callbacks:     callbacks,
	}, nil
}
