package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

func statusCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status <robot-id>",
		Short: "Show status of a specific robot",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.GetRobotStatus(ctx, &pb.GetRobotStatusRequest{RobotId: args[0]})
			if err != nil {
				return fmt.Errorf("get status: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				fmt.Printf("Robot ID:  %s\n", resp.RobotId)
				fmt.Printf("Mode:      %s\n", resp.Mode)
				fmt.Printf("State:     %s\n", stateStr(resp.State))
				if resp.RosMasterUri != "" {
					fmt.Printf("ROS:       %s\n", resp.RosMasterUri)
				}
				if resp.RosDomainId != 0 {
					fmt.Printf("DOMAIN_ID: %d\n", resp.RosDomainId)
				}
				if m := resp.CurrentMode; m != nil {
					fmt.Printf("Package:   %s\n", m.Package)
					fmt.Printf("Launch:    %s\n", m.LaunchFile)
					if len(m.Args) > 0 {
						fmt.Printf("Args:      %s\n", formatMap(m.Args))
					}
					if len(m.Env) > 0 {
						fmt.Printf("Env:       %s\n", formatMap(m.Env))
					}
				}
				if len(resp.Params) > 0 {
					fmt.Printf("Params:    %s\n", formatMap(resp.Params))
				}
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
