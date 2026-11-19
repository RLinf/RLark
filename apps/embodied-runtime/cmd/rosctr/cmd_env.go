package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

// envVars is the structured output for --format json/yaml. Only the field
// relevant to the detected ROS version is populated; the other is left
// empty (zero value).
type envVars struct {
	ROSMASTERURI string `json:"ROS_MASTER_URI,omitempty" yaml:"ROS_MASTER_URI,omitempty"`
	ROSDOMAINID  int32  `json:"ROS_DOMAIN_ID,omitempty" yaml:"ROS_DOMAIN_ID,omitempty"`
}

func envCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env <robot-id>",
		Short: "Print ROS env vars for a robot, for sourcing into the shell",
		Long: `Print ROS connection env vars for the specified robot, for sourcing.

Auto-detects ROS version from the controller response:
  - ROS 1 (ros_master_uri non-empty): prints  export ROS_MASTER_URI=<uri>
  - ROS 2 (ros_domain_id non-zero):   prints  export ROS_DOMAIN_ID=<id>

Usage:
  . <(rosctr --socket-path /var/run/rlark/ros-ctrl.sock env franka-0)     # ROS 1
  . <(rosctr --socket-path /var/run/rlark/ros2-ctrl.sock env franka-0)    # ROS 2

For ROS 1, if you also need ROS_IP (for bidirectional communication), set
it to your local machine's IP before sourcing:
  export ROS_IP=$(hostname -I | awk '{print $1}')
  . <(rosctr env franka-0)

For ROS 2, set the RMW implementation before sourcing if you need a
non-default DDS:
  export RMW_IMPLEMENTATION=rmw_cyclonedds_cpp
  . <(rosctr --socket-path /var/run/rlark/ros2-ctrl.sock env franka-0)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.GetRobotStatus(ctx, &pb.GetRobotStatusRequest{RobotId: args[0]})
			if err != nil {
				return fmt.Errorf("get robot status: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, envVars{
				ROSMASTERURI: resp.RosMasterUri,
				ROSDOMAINID:  resp.RosDomainId,
			}, func() {
				if resp.RosMasterUri != "" {
					fmt.Printf("export ROS_MASTER_URI=%s\n", resp.RosMasterUri)
				}
				if resp.RosDomainId != 0 {
					fmt.Printf("export ROS_DOMAIN_ID=%d\n", resp.RosDomainId)
				}
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
