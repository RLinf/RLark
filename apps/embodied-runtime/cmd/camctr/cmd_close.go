package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"github.com/spf13/cobra"
)

func closeCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "close <camera-id>",
		Short: "Close (stop) a camera",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.CloseCamera(ctx, &pb.CloseCameraRequest{
				CameraId: args[0],
			})
			if err != nil {
				return fmt.Errorf("close camera: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				fmt.Printf("Camera %s → %s\n", resp.CameraId, stateStr(resp.State))
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
