package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
)

func main() {
	config := agent.DefaultConfig()
	cmd := &cobra.Command{
		Use:     "rlark-agent",
		Short:   "Start the agent application",
		Version: version.String(),
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := agent.NewAgent(config)
			return srv.Run(cmd.Context())
		},
	}
	config.SetupFlags(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
