package nodeserver

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/pflag"
)

// Config holds configuration options.
type Config struct {
	UnixSocketAddress string
	DrainTimeout      time.Duration
}

// DefaultConfig returns the default config.
func DefaultConfig() Config {
	return Config{
		UnixSocketAddress: "/var/run/rlark/nodeserver.sock",
		DrainTimeout:      30 * time.Minute,
	}
}

// SetupFlags sets the upFlags.
func (c *Config) SetupFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.UnixSocketAddress, "nodeserver-unix-socket", c.UnixSocketAddress, "Unix socket address for node server")
	fs.DurationVar(&c.DrainTimeout, "nodeserver-drain-timeout", c.DrainTimeout, "Maximum time to drain active node server connections during shutdown")
}

// Listen creates a unique instance socket without publishing it at the stable
// path. Call Publish once the server's dependencies are ready.
func (c *Config) Listen() (net.Listener, string, error) {
	stableDir := filepath.Dir(c.UnixSocketAddress)
	instanceFile, err := os.CreateTemp(stableDir, ".nodeserver-*.sock")
	if err != nil {
		return nil, "", fmt.Errorf("reserve instance socket path: %w", err)
	}
	instanceAddress := instanceFile.Name()
	if err := instanceFile.Close(); err != nil {
		_ = os.Remove(instanceAddress)
		return nil, "", fmt.Errorf("close reserved instance socket: %w", err)
	}
	if err := os.Remove(instanceAddress); err != nil {
		return nil, "", fmt.Errorf("prepare instance socket: %w", err)
	}
	l, err := net.Listen("unix", instanceAddress)
	if err != nil {
		return nil, "", err
	}
	return l, instanceAddress, nil
}

// Publish atomically switches the stable socket path to instanceAddress.
func (c *Config) Publish(instanceAddress string) error {
	stableDir := filepath.Dir(c.UnixSocketAddress)
	instanceDir := filepath.Dir(instanceAddress)
	if stableDir != instanceDir {
		return fmt.Errorf("stable and instance Unix sockets must be in the same directory")
	}
	tempLink, err := os.CreateTemp(stableDir, ".nodeserver-socket-*")
	if err != nil {
		return fmt.Errorf("create temporary socket link: %w", err)
	}
	tempPath := tempLink.Name()
	if err := tempLink.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("close temporary socket link: %w", err)
	}
	if err := os.Remove(tempPath); err != nil {
		return fmt.Errorf("prepare temporary socket link: %w", err)
	}
	defer func() { _ = os.Remove(tempPath) }()

	if err := os.Symlink(filepath.Base(instanceAddress), tempPath); err != nil {
		return fmt.Errorf("create temporary socket link: %w", err)
	}
	if err := os.Rename(tempPath, c.UnixSocketAddress); err != nil {
		return fmt.Errorf("publish instance socket: %w", err)
	}
	return nil
}

func (c *Config) CleanupSocket(instanceAddress string) {
	_ = os.Remove(instanceAddress)
}
