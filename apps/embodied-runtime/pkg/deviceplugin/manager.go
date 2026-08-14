package deviceplugin

import "context"

// ControllerManager defines the lifecycle interface for a controller
// subprocess. Implementations may launch the controller locally (exec),
// remotely (pod), or via any other mechanism.
//
// Every method takes a context.Context to support cancellation, timeouts,
// and tracing for remote operations.
type ControllerManager interface {
	// ApplyConfig delivers the generated YAML configuration to the
	// controller. For local process managers this writes to a file;
	// for pod-based managers this may mount a ConfigMap or pass
	// the data via environment variables.
	ApplyConfig(ctx context.Context, config []byte) error

	// Maintain checks whether the latest applied config differs from the
	// currently running configuration. If it does, the controller is
	// restarted. Returns true when a restart was triggered.
	Maintain(ctx context.Context) (bool, error)

	// Start launches the controller and blocks until it's ready.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the controller.
	Stop(ctx context.Context)

	// IsRunning returns whether the controller is alive.
	IsRunning(ctx context.Context) bool
}
