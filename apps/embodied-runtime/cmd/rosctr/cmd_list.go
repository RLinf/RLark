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

func listCmd(socketPath string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all managed robots and their status",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, conn := newClient(socketPath)
			defer func() { _ = conn.Close() }()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.ListRobots(ctx, &pb.ListRobotsRequest{})
			if err != nil {
				return fmt.Errorf("list robots: %w", err)
			}

			format := cli.FormatFromCmd(cmd)
			cli.Print(format, resp, func() {
				if len(resp.Robots) == 0 {
					fmt.Println("No robots registered.")
					return
				}

				w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
				_, _ = fmt.Fprintln(w, "ROBOT ID\tMODE\tPACKAGE\tLAUNCH FILE\tROS URI / DOMAIN\tSTATE")
				_, _ = fmt.Fprintln(w, "---------\t----\t-------\t-----------\t----------------\t-----")
				for _, r := range resp.Robots {
					pkg, launch, rosInfo := "", "", ""
					if m := r.CurrentMode; m != nil {
						pkg = m.Package
						launch = m.LaunchFile
					}
					if r.RosMasterUri != "" {
						rosInfo = r.RosMasterUri
					} else if r.RosDomainId != 0 {
						rosInfo = fmt.Sprintf("domain:%d", r.RosDomainId)
					}
					_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", r.RobotId, r.Mode, pkg, launch, rosInfo, stateStr(r.State))
				}
				_ = w.Flush()
			})
			return nil
		},
	}

	cli.AddFormatFlag(cmd)
	return cmd
}
