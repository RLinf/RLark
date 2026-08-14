package sync

import (
	"fmt"

	"github.com/spf13/pflag"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Config holds configuration for the persistencer.
type Config struct {
	// Workers is the number of concurrent workers for syncing.
	Workers int
}

// DefaultConfig returns the default persistencer configuration.
func DefaultConfig() Config {
	return Config{
		Workers: 5,
	}
}

// SetupFlags sets the upFlags.
func (c *Config) SetupFlags(fs *pflag.FlagSet) {
	fs.IntVar(&c.Workers, "sync-workers", c.Workers, "Number of concurrent workers for syncing")
}

// Validate validates the configuration.
func (c Config) Validate() error {
	if c.Workers <= 0 {
		return fmt.Errorf("sync workers must be positive")
	}
	return nil
}

// ToControllerOptions is an exported method.
func (c Config) ToControllerOptions() controller.TypedOptions[reconcile.Request] {
	return controller.TypedOptions[reconcile.Request]{
		MaxConcurrentReconciles: c.Workers,
	}
}
