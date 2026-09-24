package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/rlinf/rlark/apps/rlark/pkg/sshd"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
	"github.com/spf13/cobra"
)

func main() {
	cmd := &cobra.Command{
		Use:     "rlark-tools",
		Short:   "Utilities for RLark workloads",
		Version: version.String(),
	}
	cmd.AddCommand(newSSHDCommand())

	if err := cmd.Execute(); err != nil {
		os.Exit(2)
	}
}

func newSSHDCommand() *cobra.Command {
	var port string
	var shell string
	cmd := &cobra.Command{
		Use:   "sshd",
		Short: "Start the in-pod SSH server",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := &sshd.Server{Port: port, Shell: shell}
			go func() {
				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				<-sigCh
				os.Exit(0)
			}()
			return srv.ListenAndServe()
		},
	}
	cmd.Flags().StringVar(&port, "port", "22", "SSH listen port")
	cmd.Flags().StringVar(&shell, "shell", "", "Shell binary path (default: /bin/bash)")
	return cmd
}
