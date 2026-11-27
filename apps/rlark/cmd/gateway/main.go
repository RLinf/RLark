package main

import (
	"fmt"
	"os"
	"strconv"

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
			for _, setting := range []struct {
				env, flag string
				target    *int
			}{
				{"RLARK_CAMERA_MAX_LIVE_STREAMS", "camera-max-live-streams", &config.CameraMaxLiveStreams},
				{"RLARK_CAMERA_MAX_LIVE_STREAMS_PER_USER", "camera-max-live-streams-per-user", &config.CameraMaxLiveStreamsPerUser},
			} {
				if !cmd.Flags().Changed(setting.flag) {
					if raw, ok := os.LookupEnv(setting.env); ok {
						value, err := strconv.Atoi(raw)
						if err != nil || value < 0 {
							return fmt.Errorf("parse %s: expected a non-negative integer", setting.env)
						}
						*setting.target = value
					}
				}
			}
			if !cmd.Flags().Changed("disable-native-login") {
				if value, ok := os.LookupEnv("RLARK_DISABLE_NATIVE_LOGIN"); ok {
					disabled, err := strconv.ParseBool(value)
					if err != nil {
						return fmt.Errorf("parse RLARK_DISABLE_NATIVE_LOGIN: %w", err)
					}
					config.DisableNativeLogin = disabled
				}
			}
			gw := gateway.NewGateway(config)
			return gw.Run(cmd.Context())
		},
	}
	config.SetupFlags(cmd.Flags())

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
