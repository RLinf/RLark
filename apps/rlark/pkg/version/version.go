// Package version provides build-time version information.
// These values are injected via ldflags at build time.
package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the semantic version (e.g. "v0.1.0"). Set via -ldflags.
	Version = "dev"
	// Commit is the git commit SHA. Set via -ldflags.
	Commit = "unknown"
	// BuildTime is the build timestamp in UTC. Set via -ldflags.
	BuildTime = "unknown"
)

// String returns the full version string.
func String() string {
	return fmt.Sprintf("%s (commit: %s, built: %s, go: %s)", Version, Commit, BuildTime, runtime.Version())
}
