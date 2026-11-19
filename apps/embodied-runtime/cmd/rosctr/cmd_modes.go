package main

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cli"
	pb "github.com/rlinf/rlark/sdks/embodied-runtime-go/gen/roscontroller/v1"
	"github.com/spf13/cobra"
)

func modesCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "modes <robot-id>",
		Short: "List available control modes for a robot with full details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.ListModes(ctx, &pb.ListModesRequest{RobotId: args[0]})
			if err != nil {
				return fmt.Errorf("list modes: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				if len(resp.Modes) == 0 {
					fmt.Printf("Robot %s has no modes configured.\n", resp.RobotId)
					return
				}

				w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
				_, _ = fmt.Fprintf(w, "MODE\tPACKAGE\tLAUNCH FILE\tARGS\tARG_FROM\tPASSTHROUGH\tENV\n")
				_, _ = fmt.Fprintf(w, "----\t-------\t-----------\t----\t--------\t-----------\t---\n")
				for _, m := range resp.Modes {
					argsStr := formatMap(m.Args)
					argFromStr := formatMap(m.ArgFrom)
					passthroughStr := ""
					if m.PassthroughRobotArgs {
						passthroughStr = "true"
					}
					envStr := formatMap(m.Env)
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", m.Name, m.Package, m.LaunchFile, argsStr, argFromStr, passthroughStr, envStr)
				}
				_ = w.Flush()
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
