package main

import (
	"context"
	"fmt"
	"io"
	"os"

	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"github.com/spf13/cobra"
)

func watchCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch <camera-id>",
		Short: "Stream frames or video from an open camera",
		Long: `Stream frames or video from an open camera.

The stream is delivered in the encoding the camera was opened with
(see 'camctr open --encoding'):

  jpeg / png / bmp / tiff   each message is one complete, independently
                            decodable still-image frame
  h264 / h265               each message is a chunk of the Annex B
                            elementary stream; concatenate all chunks in
                            order to produce a valid bitstream

Output rules:
  --save-dir set              → save each chunk/frame to a file (no stdout output)
  stdout is a terminal (TTY)  → print metadata on stderr only.
                                Use --output-to-terminal to force raw bytes to stdout.
  stdout is piped/redirected  → write raw bytes to stdout (metadata on stderr).

Examples:
  camctr open wrist-cam --encoding h264   # open the camera in h264 first
  camctr watch wrist-cam                   # metadata on stderr
  camctr watch wrist-cam --save-dir /tmp/frames   # save as files
  camctr watch wrist-cam > /tmp/stream.h264        # save the bitstream
  camctr watch wrist-cam | ffplay -i -             # pipe to ffplay`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			saveDir, _ := cmd.Flags().GetString("save-dir")
			outputToTerminal, _ := cmd.Flags().GetBool("output-to-terminal")

			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			req := &pb.WatchFramesRequest{
				CameraId: args[0],
			}

			stream, err := client.WatchFrames(ctx, req)
			if err != nil {
				return fmt.Errorf("watch frames: %w", err)
			}

			// Determine output mode.
			//   saveDir set        → save to files
			//   stdout is terminal → require --output-to-terminal for raw data
			//   stdout is pipe     → raw bytes to stdout
			writeStdout := saveDir == "" && (!isTerminal(os.Stdout) || outputToTerminal)

			if !writeStdout && saveDir == "" {
				fmt.Fprintln(os.Stderr,
					"stdout is a terminal. Use --output-to-terminal to force raw data,"+
						" or pipe to a file/command.")
			}

			for {
				frame, err := stream.Recv()
				if err != nil {
					return fmt.Errorf("stream recv: %w", err)
				}

				// Metadata always goes to stderr.
				fmt.Fprintf(os.Stderr, "frame %d: %dx%d %s (%d bytes) ts=%d keyframe=%v\n",
					frame.Sequence, frame.Width, frame.Height,
					frame.Encoding, len(frame.Data), frame.TimestampNs, frame.Keyframe)

				switch {
				case saveDir != "":
					ext := extForEncoding(frame.Encoding)
					fname := fmt.Sprintf("%s/%s_%08d.%s", saveDir, frame.Encoding, frame.Sequence, ext)
					if err := os.WriteFile(fname, frame.Data, 0644); err != nil {
						return fmt.Errorf("write %s: %w", fname, err)
					}

				case writeStdout:
					if _, err := os.Stdout.Write(frame.Data); err != nil {
						return fmt.Errorf("write stdout: %w", err)
					}
				}
			}
		},
	}

	cmd.Flags().String("save-dir", "", "Directory to save frames/chunks as files")
	cmd.Flags().Bool("output-to-terminal", false, "Allow raw bytes to stdout when stdout is a terminal")
	return cmd
}

// isTerminal returns true if w is a character device (terminal).
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	stat, err := f.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// extForEncoding returns the file extension for a given encoding.
func extForEncoding(enc string) string {
	switch enc {
	case "h264":
		return "h264"
	case "h265":
		return "h265"
	case "jpeg":
		return "jpg"
	case "png":
		return "png"
	case "bmp":
		return "bmp"
	case "tiff":
		return "tif"
	default:
		return "bin"
	}
}
