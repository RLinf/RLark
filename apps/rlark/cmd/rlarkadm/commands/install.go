package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm"
)

// InstallCommand installs the components.
func InstallCommand() *cobra.Command {
	var installConf string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install a control or data plane from a deploy config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if installConf == "" {
				return fmt.Errorf("--install-conf/-f is required")
			}

			cfg, err := rlarkadm.LoadDeployConfig(installConf)
			if err != nil {
				return err
			}

			if level, _ := cmd.Flags().GetString("log-level"); level != "" {
				log.InitLogger(level)
			}

			return rlarkadm.Install(cfg)
		},
	}

	cmd.Flags().StringVarP(&installConf, "install-conf", "f", "", "Path to the install config file")

	return cmd
}
