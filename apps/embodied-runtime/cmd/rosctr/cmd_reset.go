package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

func resetCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reset <robot-id>",
		Short: "Reset a robot — stop process, restart middleware, clear state",
		Long: `Reset a robot node.

Stops the launch process, restarts the ROS middleware (roscore for ROS 1;
the launch process for ROS 2 — there is no master to restart), and
resets the robot state back to STOPPED. Useful for recovering from
error states without re-registering the robot.

Example:
  rosctr reset franka-0
  rosctr --socket-path /var/run/rlark/ros2-ctrl.sock reset franka-0`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			req := &pb.ResetRobotRequest{RobotId: args[0]}

			resp, err := client.ResetRobot(ctx, req)
			if err != nil {
				return fmt.Errorf("reset robot: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				if resp.RosMasterUri != "" {
					fmt.Printf("Robot %s reset → %s (ROS_MASTER_URI=%s)\n",
						resp.RobotId, stateStr(resp.State), resp.RosMasterUri)
				} else {
					fmt.Printf("Robot %s reset → %s (ROS_DOMAIN_ID=%d)\n",
						resp.RobotId, stateStr(resp.State), resp.RosDomainId)
				}
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
