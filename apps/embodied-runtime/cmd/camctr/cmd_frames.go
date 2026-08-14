package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"github.com/spf13/cobra"
)

func framesCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "frames <camera-id> [<camera-id> ...] <output-dir>",
		Short: "Capture frames from multiple cameras in a single request",
		Long: `Capture the latest frame from several open cameras at once.

Each requested camera is read concurrently in one RPC, so the round-trip
latency is bounded by the slowest camera. This is intended for use cases
such as fetching an RGB frame and its depth map together where splitting
the capture across multiple requests would hurt real-time performance.

Each camera must already be open (use 'camctr open' first). Frames are
returned in the encoding each camera was opened with.

The last argument is the output target:
  <dir>     one file per camera is written there, named <camera-id>.<ext>
            (e.g. camera-0.jpeg). The directory is created if needed.
  -         no files are written; per-camera status is printed to stdout
            and errors to stderr. Useful with -o json for scripting.

Cameras that fail capture are reported on stderr and skip the file write,
but do not abort the command.

Examples:
  camctr frames wrist-rgb wrist-depth /tmp/frames
  camctr frames wrist-rgb wrist-depth - -o json
  camctr open wrist-rgb --encoding jpeg   # open cameras first`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			timeout, _ := cmd.Flags().GetInt32("timeout")

			outputDir := args[len(args)-1]
			cameraIDs := args[:len(args)-1]
			noFiles := outputDir == "-"

			req := &pb.CaptureFramesRequest{
				CameraIds: cameraIDs,
				Timeout:   &timeout,
			}

			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := client.CaptureFrames(ctx, req)
			if err != nil {
				return fmt.Errorf("capture frames: %w", err)
			}

			if !noFiles {
				if err := os.MkdirAll(outputDir, 0755); err != nil {
					return fmt.Errorf("create output dir %s: %w", outputDir, err)
				}
			}

			for _, f := range resp.Frames {
				if f.GetErrorCode() != 0 {
					fmt.Fprintf(os.Stderr, "  %s: ERROR (code=%d): %s\n",
						f.GetCameraId(), f.GetErrorCode(), f.GetError())
					continue
				}
				if noFiles {
					fmt.Printf("  %s: %dx%d, %s, %d bytes, ts=%d\n",
						f.GetCameraId(), f.GetWidth(), f.GetHeight(),
						f.GetEncoding(), len(f.GetData()), f.GetTimestampNs())
					continue
				}
				ext := "." + extForEncoding(f.GetEncoding())
				out := filepath.Join(outputDir, f.GetCameraId()+ext)
				if err := os.WriteFile(out, f.GetData(), 0644); err != nil {
					fmt.Fprintf(os.Stderr, "  %s: write %s: %v\n",
						f.GetCameraId(), out, err)
					continue
				}
				fmt.Printf("  %s: %s (%dx%d, %s, %d bytes)\n",
					f.GetCameraId(), out, f.GetWidth(), f.GetHeight(),
					f.GetEncoding(), len(f.GetData()))
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {})
			return nil
		},
	}

	cmd.Flags().Int32("timeout", 5, "Max seconds to wait for a frame (per camera)")
	cli.AddFormatFlag(cmd)
	return cmd
}
