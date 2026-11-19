package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

func stopCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stop <robot-id>",
		Short: "Stop a running robot node",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			resp, err := client.StopRobot(ctx, &pb.StopRobotRequest{RobotId: args[0]})
			if err != nil {
				return fmt.Errorf("stop robot: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				fmt.Printf("Robot %s stopped\n", resp.RobotId)
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
