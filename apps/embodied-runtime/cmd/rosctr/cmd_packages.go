package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

func packagesCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "packages",
		Short: "List available ROS packages on the server",
		Long: `List ROS packages available on the server, filtered by the
allowed launch packages whitelist configured in the controller
(ros-controller or ros2-controller, depending on --socket-path).

Use -o json or -o yaml to get machine-readable output.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.ListPackages(ctx, &pb.ListPackagesRequest{})
			if err != nil {
				return fmt.Errorf("list packages: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				if len(resp.Packages) == 0 {
					fmt.Println("(no packages available)")
					return
				}
				for _, pkg := range resp.Packages {
					fmt.Println(pkg)
				}
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
