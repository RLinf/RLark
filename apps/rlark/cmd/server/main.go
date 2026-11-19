package main

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"

	"github.com/rlinf/rlark/apps/rlark/pkg/server"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
)

func init() {
	gin.SetMode(gin.ReleaseMode)
}

func main() {
	config := server.DefaultConfig()
	cmd := &cobra.Command{
		Use:     "rlark-server",
		Short:   "Start the server application",
		Version: version.String(),
		RunE: func(cmd *cobra.Command, args []string) error {
			srv := server.NewServer(config)
			return srv.Run(cmd.Context())
		},
	}
	config.SetupFlags(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
