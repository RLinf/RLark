package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"github.com/spf13/cobra"
)

func openCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "open <camera-id>",
		Short: "Open (start) a camera for frame capture",
		Long: `Start frame capture on a camera device. Optional flags can override
the default resolution and frame rate for this capture session.

Encodings (frame-mode — each message is one complete, independently
decodable still image):
  jpeg   JPEG-compressed frames (default)
  png    PNG lossless-compressed frames
  bmp    BMP uncompressed frames
  tiff   TIFF lossless-compressed frames

Encodings (bitstream-mode — concatenate chunks for a valid stream):
  h264   H.264 Annex B elementary stream
  h265   H.265 (HEVC) Annex B elementary stream

The driver uses ffmpeg to capture and transcode. If the camera does not
support the requested encoding directly, ffmpeg converts automatically
(e.g. a YUYV-only camera produces JPEG output when encoding=jpeg).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			width, _ := cmd.Flags().GetInt32("width")
			height, _ := cmd.Flags().GetInt32("height")
			fps, _ := cmd.Flags().GetInt32("fps")
			encoding, _ := cmd.Flags().GetString("encoding")

			req := &pb.OpenCameraRequest{CameraId: args[0]}
			req.Width = &width
			req.Height = &height
			req.Fps = &fps
			if encoding != "jpeg" {
				req.Encoding = &encoding
			}

			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.OpenCamera(ctx, req)
			if err != nil {
				return fmt.Errorf("open camera: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				fmt.Printf("Camera %s → %s (encoding=%s)\n", resp.CameraId, stateStr(resp.State), resp.Encoding)
			})
			return nil
		},
	}

	cmd.Flags().Int32("width", 0, "Override capture width (0 = use default)")
	cmd.Flags().Int32("height", 0, "Override capture height (0 = use default)")
	cmd.Flags().Int32("fps", 0, "Override capture FPS (0 = use default)")
	cmd.Flags().String("encoding", "jpeg", "Encoding: jpeg, png, bmp, tiff, h264, or h265")
	cli.AddFormatFlag(cmd)
	return cmd
}
