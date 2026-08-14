package main

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/cmd/rlarkadm/commands"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
)

func main() {
	root := &cobra.Command{
		Use:     "rlarkadm",
		Short:   "Rlark administration tool",
		Version: version.String(),
	}
	root.PersistentFlags().String("log-level", "info", "log level (debug/info/warn/error)")

	root.AddCommand(commands.InstallCommand())
	root.AddCommand(commands.UninstallCommand())

	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
