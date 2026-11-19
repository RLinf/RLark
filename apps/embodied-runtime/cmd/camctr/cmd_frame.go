package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"github.com/spf13/cobra"
)

func frameCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "frame <camera-id> <output-file>",
		Short: "Capture a single frame from a camera and save to a file",
		Long: `Capture the latest frame from an open camera.
The camera must already be open (use 'camctr open' first).

The frame is returned in the encoding the camera was opened with
(see 'camctr open --encoding').`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			timeout, _ := cmd.Flags().GetInt32("timeout")

			req := &pb.CaptureFrameRequest{
				CameraId: args[0],
				Timeout:  &timeout,
			}

			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := client.CaptureFrame(ctx, req)
			if err != nil {
				return fmt.Errorf("capture frame: %w", err)
			}

			if err := os.WriteFile(args[1], resp.Data, 0644); err != nil {
				return fmt.Errorf("write %s: %w", args[1], err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				fmt.Printf("Frame captured: %s (%dx%d, %s, %d bytes)\n",
					args[1], resp.Width, resp.Height, resp.Encoding, len(resp.Data))
			})
			return nil
		},
	}

	cmd.Flags().Int32("timeout", 5, "Max seconds to wait for a frame")
	cli.AddFormatFlag(cmd)
	return cmd
}
