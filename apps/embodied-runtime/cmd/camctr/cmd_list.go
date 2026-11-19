package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/cameracontroller/v1"
	"github.com/spf13/cobra"
)

func listCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all managed cameras",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.ListCameras(ctx, &pb.ListCamerasRequest{})
			if err != nil {
				return fmt.Errorf("list cameras: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				if len(resp.Cameras) == 0 {
					fmt.Println("No cameras registered.")
					return
				}

				w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
				_, _ = fmt.Fprintln(w, "CAMERA ID\tNAME\tTYPE\tSERIAL\tRESOLUTION\tFPS\tPIXEL\tDEPTH\tSTATE")
				_, _ = fmt.Fprintln(w, "---------\t----\t----\t------\t----------\t---\t-----\t-----\t-----")
				for _, c := range resp.Cameras {
					depth := "no"
					if c.EnableDepth {
						depth = "yes"
					}
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%dx%d\t%d\t%s\t%s\t%s\n",
						c.CameraId, c.Name, c.CameraType, c.SerialNumber,
						c.Width, c.Height, c.Fps, c.PixelFormat, depth, stateStr(c.State))
				}
				_ = w.Flush()
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
