package main

import (
	"cmp"
	"log"
	"os"

	"github.com/spf13/cobra"
)

// defaultSocketPath picks the Unix socket path for the ros-controller or
// ros2-controller gRPC server. RLINF_EMBODIED_ROS_SOCKET_PATH (ROS 1) takes
// priority; if unset, falls back to RLINF_EMBODIED_ROS2_SOCKET_PATH (ROS 2);
// if both are unset, defaults to the ROS 1 socket path. An explicit
// --socket-path flag always wins.
var defaultSocketPath = cmp.Or(
	os.Getenv("RLINF_EMBODIED_ROS_SOCKET_PATH"),
	os.Getenv("RLINF_EMBODIED_ROS2_SOCKET_PATH"),
	"/var/run/rlark/ros-ctrl.sock",
)

func main() {
	var socketPath string

	root := &cobra.Command{
		Use:   "rosctr",
		Short: "ROS Controller CLI — control robot nodes (ROS 1 or ROS 2)",
	}

	root.PersistentFlags().StringVar(&socketPath, "socket-path", defaultSocketPath,
		"Unix socket path of the ros-controller or ros2-controller gRPC server")

	root.AddCommand(
		listCmd(socketPath),
		statusCmd(socketPath),
		startCmd(socketPath),
		stopCmd(socketPath),
		switchCmd(socketPath),
		resetCmd(socketPath),
		modesCmd(socketPath),
		logsCmd(socketPath),
		envCmd(socketPath),
		packagesCmd(socketPath),
		pkgCmd(socketPath),
	)

	if err := root.Execute(); err != nil {
		log.Fatalf("error: %v", err)
	}
}
