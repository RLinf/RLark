package main

import (
	"context"
	"fmt"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

func switchCmd(socketPath string) *cobra.Command {
	var mf modeFlags

	cmd := &cobra.Command{
		Use:   "switch <robot-id> [mode]",
		Short: "Switch a running robot to a different control mode",
		Long: `Switch a running robot to a different control mode.

Examples:
  rosctr switch franka-0 joint                      # preset mode
  rosctr switch franka-0 joint --arg extra=val      # preset + extra args
  rosctr switch franka-0 --package pkg --launch-file f.launch  # custom mode`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			robotID := args[0]
			mode := ""
			if len(args) > 1 {
				mode = args[1]
			}

			cfg, err := resolveModeConfig(cmd, mode, &mf)
			if err != nil {
				return err
			}

			req := &pb.SwitchModeRequest{
				RobotId:    robotID,
				Mode:       mode,
				ModeConfig: cfg,
			}

			resp, err := client.SwitchMode(ctx, req)
			if err != nil {
				return fmt.Errorf("switch mode: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				fmt.Printf("Robot %s switched to mode %s → %s\n", resp.RobotId, resp.Mode, stateStr(resp.State))
			})
			return nil
		},
	}

	addModeFlags(cmd, &mf)
	cli.AddFormatFlag(cmd)

	return cmd
}
