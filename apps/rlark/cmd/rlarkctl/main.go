package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkctl"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
)

func main() {
	cmd := &cobra.Command{
		Use:     "rlark-server-cli",
		Short:   "A CLI tool for managing the rlark server",
		Version: version.String(),
	}

	cmd.AddCommand(rlarkctl.SignCommand())
	cmd.AddCommand(rlarkctl.RevokeCommand())
	cmd.AddCommand(rlarkctl.ProxyCurlCommand())
	rlarkctl.SetupPersistentFlags(cmd.PersistentFlags())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
