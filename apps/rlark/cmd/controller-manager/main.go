package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/controllermanager"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
)

func main() {
	config := controllermanager.DefaultConfig()
	cmd := &cobra.Command{
		Use:     "rlark-controller-manager",
		Short:   "Start the rlark controller manager",
		Version: version.String(),
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := controllermanager.New(config)
			if err != nil {
				return err
			}
			return mgr.Start(cmd.Context())
		},
	}
	config.SetupFlags(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
