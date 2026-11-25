package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
	"github.com/rlinf/rlark/apps/rlark/pkg/rlarkadm"
)

// UpgradeCommand upgrades the components.
func UpgradeCommand() *cobra.Command {
	var upgradeConf string

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade a control or data plane from a deploy config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if upgradeConf == "" {
				return fmt.Errorf("--upgrade-conf/-f is required")
			}

			cfg, err := rlarkadm.LoadDeployConfig(upgradeConf)
			if err != nil {
				return err
			}
			if err := cfg.ValidateForUpgrade(); err != nil {
				return err
			}

			if level, _ := cmd.Flags().GetString("log-level"); level != "" {
				log.InitLogger(level)
			}

			return rlarkadm.Upgrade(cfg)
		},
	}

	cmd.Flags().StringVarP(&upgradeConf, "upgrade-conf", "f", "", "Path to the upgrade config file (same format as install)")

	return cmd
}
