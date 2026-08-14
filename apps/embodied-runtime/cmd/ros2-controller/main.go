package main

import (
	"log"
	"os"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/netmac"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/ros2controller"
	"github.com/spf13/cobra"
)

func main() {
	var (
		configPath string
		socketPath string
		httpAddr   string
		podIP      = os.Getenv("POD_IP")
	)

	cmd := &cobra.Command{
		Use:   "ros2-controller",
		Short: "ROS 2 Controller — manages robot nodes via ros2 launch",
		Long: `ros2-controller is the robot node lifecycle manager for the embodied-runtime
DaemonSet. It assigns each robot a unique ROS_DOMAIN_ID for DDS isolation,
then exposes a gRPC server on a Unix socket that the device plugin calls to
start/stop robot nodes and switch control modes.

Unlike the ROS 1 controller, there is no central master (roscore). Each
robot gets its own ROS_DOMAIN_ID so multiple robots in the same container
do not discover each other's topics/services.

Optionally, an HTTP/JSON gateway (--http-addr) mirrors every gRPC RPC over
REST under /v1/ and serves a per-robot reverse proxy under
/v1/robots/{robot_id}/proxy/*, so non-gRPC clients (curl, browsers, fetch)
can list robots, start/stop/switch modes, read logs, and reach each
robot's web service.

Networking:
  - ros2 launch runs in the container's network namespace
  - MACVLAN interfaces are created at startup for robot network access
  - ROS_DOMAIN_ID is injected into each ros2 launch process for DDS isolation
  - Optional RMW/DDS env vars can be configured per-mode`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileCfg, err := ros2controller.LoadConfigFile(configPath)
			if err != nil {
				return err
			}

			for i := range fileCfg.MACVLANs {
				netmac.EnrichMACVLANConfig(&fileCfg.MACVLANs[i], "[ros2-controller]")
				if err := netmac.ValidateMACVLANConfig(fileCfg.MACVLANs[i]); err != nil {
					return err
				}
			}

			cfg := ros2controller.ServerConfig{
				PodIP:                 podIP,
				HTTPAddr:              fileCfg.HTTPAddr,
				MACVLANs:              fileCfg.MACVLANs,
				Types:                 fileCfg.Types,
				Robots:                fileCfg.Robots,
				AllowedLaunchPackages: fileCfg.AllowedLaunchPackages,
			}

			if cmd.Flags().Changed("socket-path") {
				cfg.SocketPath = socketPath
			}
			if cfg.SocketPath == "" {
				cfg.SocketPath = ros2controller.DefaultServerConfig().SocketPath
			}
			if cmd.Flags().Changed("http-addr") {
				cfg.HTTPAddr = httpAddr
			}

			server := ros2controller.NewServer(cfg)
			return server.Run()
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "/etc/rlinf/ros2-controller.yaml",
		"Path to the YAML config file (macvlans, types, robots)")

	cmd.Flags().StringVar(&socketPath, "socket-path",
		ros2controller.DefaultServerConfig().SocketPath,
		"Unix socket path for the gRPC server (overridden only when set; otherwise the config file's value wins)")

	cmd.Flags().StringVar(&httpAddr, "http-addr", "",
		"Optional TCP address (host:port or :port) for the HTTP/JSON gateway; empty disables HTTP (overridden only when set; otherwise the config file's value wins)")

	cmd.Flags().StringVar(&podIP, "pod-ip", podIP,
		"Pod IP on the container network (default: $POD_IP)")

	if err := cmd.Execute(); err != nil {
		log.Fatalf("[ros2-controller] fatal: %v", err)
	}
}
