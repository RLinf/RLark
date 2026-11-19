package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/network/sidecar"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
)

func main() {
	config := sidecar.DefaultConfig()
	cmd := &cobra.Command{
		Use:     "rlark-network-sidecar",
		Short:   "Start the sidecar proxy for pod traffic interception",
		Version: version.String(),
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := sidecar.NewSidecar(config)
			return srv.Run(cmd.Context())
		},
	}
	config.SetupFlags(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
