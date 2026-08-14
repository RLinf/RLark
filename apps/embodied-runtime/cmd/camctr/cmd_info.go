package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"github.com/spf13/cobra"
)

func infoCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <camera-id>",
		Short: "Show detailed information about a specific camera",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.GetCameraInfo(ctx, &pb.GetCameraInfoRequest{
				CameraId: args[0],
			})
			if err != nil {
				return fmt.Errorf("get camera info: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				c := resp.Camera
				fmt.Printf("Camera ID:    %s\n", c.CameraId)
				fmt.Printf("Name:         %s\n", c.Name)
				fmt.Printf("Type:         %s\n", c.CameraType)
				fmt.Printf("Serial:       %s\n", c.SerialNumber)
				fmt.Printf("Resolution:   %dx%d\n", c.Width, c.Height)
				fmt.Printf("FPS:          %d\n", c.Fps)
				fmt.Printf("Depth:        %v\n", c.EnableDepth)
				if c.PixelFormat != "" {
					fmt.Printf("Pixel Format: %s\n", c.PixelFormat)
				}
				if res := c.GetSupportedResolutions(); len(res) > 0 {
					fmt.Printf("Supported Res: %s\n", strings.Join(res, ", "))
				}
				if fps := c.GetSupportedFps(); len(fps) > 0 {
					fmt.Printf("Supported FPS: %s\n", joinInt32(fps))
				}
				fmt.Printf("State:        %s\n", stateStr(c.State))
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
