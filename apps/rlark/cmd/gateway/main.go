package main

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/gateway"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
)

func init() {
	gin.SetMode(gin.ReleaseMode)
}

func main() {
	config := gateway.DefaultConfig()
	cmd := &cobra.Command{
		Use:     "rlark-gateway",
		Short:   "Start the rlark API gateway",
		Version: version.String(),
		RunE: func(cmd *cobra.Command, args []string) error {
			gw := gateway.NewGateway(config)
			return gw.Run(cmd.Context())
		},
	}
	config.SetupFlags(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
