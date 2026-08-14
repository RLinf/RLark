// Command devinit is the init-container tool that performs device
// initialization for a pod by requesting setup from the device plugin's
// init service.
//
// A pod that needs node-local device setup mounts the device plugin's
// RunDir (via the device resource), then runs `devinit setup` in an init
// container. The CLI dials the service Unix socket (discovered from
// RLINF_EMBODIED_DEVINIT_SOCKET_PATH or --socket-path) and calls Setup.
// The service reads this process's PID from the socket peer credentials
// (SO_PEERCRED) and applies the node's configured device setup to this
// container's namespace. If the pod uses hostNetwork the service skips the
// request and the CLI exits 0.
//
// Usage:
//
//	devinit setup [--socket-path /var/run/rlark/devinit.sock]
//	devinit setup --timeout 30s
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	devicepb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/device/v1"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// defaultSocketPath mirrors the device plugin's default init-service socket.
// RLINF_EMBODIED_DEVINIT_SOCKET_PATH (injected by Allocate) takes priority
// so a pod never needs to hard-code the path.
func defaultSocketPath() string {
	if v := os.Getenv("RLINF_EMBODIED_DEVINIT_SOCKET_PATH"); v != "" {
		return v
	}
	return "/var/run/rlark/devinit.sock"
}

func main() {
	var (
		socketPath string
		timeout    time.Duration
	)

	root := &cobra.Command{
		Use:   "devinit",
		Short: "Device initialization tool — request node-local device setup for this pod",
	}

	setup := &cobra.Command{
		Use:           "setup",
		Short:         "Apply the node's configured device setup to this container",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(socketPath, timeout)
		},
	}

	setup.Flags().StringVar(&socketPath, "socket-path", defaultSocketPath(),
		"Unix socket path of the device plugin init service")
	setup.Flags().DurationVar(&timeout, "timeout", 30*time.Second,
		"Timeout for the setup RPC")

	root.AddCommand(setup)
	if err := root.Execute(); err != nil {
		log.Fatalf("[devinit] error: %v", err)
	}
}

func runSetup(socketPath string, timeout time.Duration) error {
	info, statErr := os.Stat(socketPath)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			log.Printf("[devinit] socket %s does not exist — skipping setup", socketPath)
			return nil
		}
		log.Printf("[devinit] socket %s unavailable (%v) — skipping setup", socketPath, statErr)
		return nil
	}
	if info.Mode()&os.ModeSocket == 0 {
		log.Printf("[devinit] %s exists but is not a Unix socket — skipping setup", socketPath)
		return nil
	}

	log.Printf("[devinit] dialing %s", socketPath)

	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial %s: %w", socketPath, err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resp, err := devicepb.NewDeviceServiceClient(conn).Setup(ctx, &devicepb.SetupRequest{})
	if err != nil {
		return fmt.Errorf("setup: %w", err)
	}

	if resp.HostNetwork {
		log.Printf("[devinit] host namespace detected — skipped: %v", resp.Skipped)
		return nil
	}
	if len(resp.Created) == 0 {
		log.Printf("[devinit] nothing to set up (no entries configured on this node)")
		return nil
	}
	log.Printf("[devinit] set up %d device resource(s): %v",
		len(resp.Created), resp.Created)
	return nil
}
