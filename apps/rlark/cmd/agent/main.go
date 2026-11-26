package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/agent"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
)

func newAgentCommand() *cobra.Command {
	config := agent.DefaultConfig()
	var configPath string
	cmd := &cobra.Command{
		Use:          "rlark-agent",
		Short:        "Start the agent application",
		Version:      version.String(),
		SilenceUsage: true,
		Args:         noAgentArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			effective, err := agent.LoadStartupConfig(configPath, cmd.Flags())
			if err != nil {
				return err
			}
			srv := agent.NewAgent(effective.Config)
			return srv.Run(cmd.Context())
		},
	}
	config.SetupFlags(cmd.PersistentFlags())
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &agent.InputError{Message: "invalid agent CLI options; see --help for names and types", Cause: err}
	})
	cmd.PersistentFlags().StringVar(&configPath, "config", "", "Startup-only v1 YAML config file (explicit CLI overrides options)")
	cmd.AddCommand(newCheckConfigCommand(&configPath), newPrintConfigCommand(&configPath))
	return cmd
}

func noAgentArgs(_ *cobra.Command, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("agent does not accept positional arguments")
	}
	return nil
}

func newCheckConfigCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "check-config",
		Short: "Validate effective local configuration without connecting or starting workers",
		Args:  noAgentArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := agent.LoadStartupConfig(*configPath, cmd.Flags())
			return err
		},
	}
}

func newPrintConfigCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "print-config",
		Short: "Print redacted effective options and sources after validation",
		Args:  noAgentArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			effective, err := agent.LoadStartupConfig(*configPath, cmd.Flags())
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				Version string            `json:"version"`
				Options map[string]string `json:"options"`
				Sources map[string]string `json:"sources"`
			}{"v1", effective.Options, effective.Sources})
		},
	}
}

func main() {
	cmd := newAgentCommand()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cmd.ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
