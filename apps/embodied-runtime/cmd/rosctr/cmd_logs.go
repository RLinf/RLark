package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

func logsCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs <robot-id>",
		Short: "Show recent logs from a robot's roslaunch process",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			tail, _ := cmd.Flags().GetInt32("tail")

			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.GetRobotLogs(ctx, &pb.GetRobotLogsRequest{
				RobotId: args[0],
				Tail:    tail,
			})
			if err != nil {
				return fmt.Errorf("get logs: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				for _, line := range resp.Lines {
					fmt.Println(line)
				}
			})
			return nil
		},
	}

	cmd.Flags().Int32("tail", 50, "Number of recent lines (0 = all)")
	cli.AddFormatFlag(cmd)
	return cmd
}
