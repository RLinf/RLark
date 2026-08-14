package main

import (
	"cmp"
	"log"
	"os"

	"github.com/spf13/cobra"
)

var defaultSocketPath = cmp.Or(os.Getenv("RLINF_EMBODIED_CAMERA_SOCKET_PATH"), "/var/run/rlark/camera-ctrl.sock")

func main() {
	var socketPath string

	root := &cobra.Command{
		Use:   "camctr",
		Short: "Camera Controller CLI — control camera devices via camera-ctrl.sock",
	}

	root.PersistentFlags().StringVar(&socketPath, "socket-path", defaultSocketPath,
		"Unix socket path of the camera-controller gRPC server")

	root.AddCommand(
		listCmd(socketPath),
		infoCmd(socketPath),
		openCmd(socketPath),
		closeCmd(socketPath),
		frameCmd(socketPath),
		framesCmd(socketPath),
		watchCmd(socketPath),
	)

	if err := root.Execute(); err != nil {
		log.Fatalf("error: %v", err)
	}
}
