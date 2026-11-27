package idlerelay

import (
	"net"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// Command is rlark-tools' idle-relay.
func Command() *cobra.Command {
	var listen string
	var idle time.Duration
	var allow []string
	var device string
	cmd := &cobra.Command{
		Use:   "idle-relay",
		Short: "HTTP CONNECT proxy that closes a tunnel silent for longer than --idle",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if device != "" {
				address, err := InterfaceAddress(device)
				if err != nil {
					return err
				}
				_, port, err := net.SplitHostPort(listen)
				if err != nil {
					return err
				}
				listen = net.JoinHostPort(address, port)
			}
			ln, err := net.Listen("tcp", listen)
			if err != nil {
				return err
			}
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			return Serve(ctx, ln, idle, allow)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:3128", "Address to accept CONNECT requests on")
	cmd.Flags().DurationVar(&idle, "idle", time.Minute, "Close a tunnel when nothing has moved in either direction for this long")
	cmd.Flags().StringVar(&device, "interface", "", "Listen on this network interface's IPv4 address, with --listen's port")
	cmd.Flags().StringSliceVar(&allow, "allow", nil, "Targets a tunnel may go to: host names, or .domain for a domain and the names under it (default: any)")
	return cmd
}
