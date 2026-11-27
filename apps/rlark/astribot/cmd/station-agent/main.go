package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rlinf/rlark/apps/rlark/astribot/stationidentity"
	"github.com/spf13/cobra"
)

func main() {
	var config stationidentity.AgentConfig
	cmd := &cobra.Command{
		Use:   "station-agent",
		Short: "Report pod-bound Station identity observations",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()
			agent, err := stationidentity.NewAgent(config)
			if err != nil {
				return err
			}
			return agent.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&config.APIURL, "api-url", os.Getenv("STATION_API_URL"), "Trusted Station API HTTPS origin")
	cmd.Flags().StringVar(&config.ClusterID, "cluster-id", os.Getenv("STATION_CLUSTER_ID"), "Registered RLark Cluster object name")
	cmd.Flags().StringVar(&config.TokenFile, "token-file", "/var/run/secrets/station/token", "Projected Pod-bound ServiceAccount token path")
	cmd.Flags().StringVar(&config.CAFile, "ca-file", os.Getenv("STATION_API_CA_FILE"), "Optional PEM CA bundle mounted by Kubernetes Secret")
	cmd.Flags().DurationVar(&config.Interval, "interval", 30*time.Second, "Observation interval")
	cmd.Flags().DurationVar(&config.RequestLimit, "request-timeout", 10*time.Second, "Maximum time for one API request")
	cmd.Flags().StringVar(&config.ProbeAddress, "probe-address", ":8081", "Health and readiness listener address")
	cmd.Flags().StringVar(&config.WireguardObservationFile, "wireguard-observation-file", "", "Non-secret WireGuard observation snapshot of the node's gateway")
	cmd.Flags().StringVar(&config.NetworkIdentityFile, "network-identity-file", "", "Public key file published by the node's gateway")
	cmd.Flags().StringVar(&config.NetworkConfigFile, "network-config-file", "", "Where to store the configuration computed by Station API")
	for _, flag := range []string{"wireguard-observation-file", "network-identity-file", "network-config-file"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "station-agent failed")
		os.Exit(1)
	}
}
