package main

import (
	"log"
	"os"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/netmac"
	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/roscontroller"
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
		Use:   "ros-controller",
		Short: "ROS Controller — manages robot nodes via roslaunch",
		Long: `ros-controller is the robot node lifecycle manager for the embodied-runtime
DaemonSet. It starts a per-robot ROS Core (roscore), then exposes a gRPC
server on a Unix socket that the device plugin calls to start/stop robot
nodes and switch control modes.

Each robot gets its own roscore on a unique port (starting from 11311),
enabling multiple robots to run in the same container without conflicts.

Optionally, an HTTP/JSON gateway (--http-addr) mirrors every gRPC RPC over
REST under /v1/ and serves a per-robot reverse proxy under
/v1/robots/{robot_id}/proxy/*, so non-gRPC clients (curl, browsers, fetch)
can list robots, start/stop/switch modes, read logs, and reach each
robot's web service.

Networking:
  - roscore runs on the container network
  - MACVLAN interfaces are created at startup for robot network access
  - roslaunch runs directly in the container (no nsenter)
  - ROS_MASTER_URI and ROS_IP are injected into each roslaunch process`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. Load config file (MACVLANs, Types, Robots, HTTPAddr).
			fileCfg, err := roscontroller.LoadConfigFile(configPath)
			if err != nil {
				return err
			}

			// 2. Enrich device-specific config — MACVLAN placeholders:
			// auto-detect the host NIC when host_nic is blank, and pick an
			// unused IP when the configured ip is the network address (e.g.
			// "172.16.0.0/24"). Best-effort, mirroring the camera V4L2
			// enrichment.
			for i := range fileCfg.MACVLANs {
				netmac.EnrichMACVLANConfig(&fileCfg.MACVLANs[i], "[ros-controller]")
				if err := netmac.ValidateMACVLANConfig(fileCfg.MACVLANs[i]); err != nil {
					return err
				}
			}

			// 3. Assemble the runtime ServerConfig from the file config.
			// SocketPath and PodIP are CLI/runtime-only (not in the file);
			// HTTPAddr and the device fields come from the file and may be
			// overridden by flags below.
			cfg := roscontroller.ServerConfig{
				PodIP:                 podIP,
				HTTPAddr:              fileCfg.HTTPAddr,
				MACVLANs:              fileCfg.MACVLANs,
				Types:                 fileCfg.Types,
				Robots:                fileCfg.Robots,
				AllowedLaunchPackages: fileCfg.AllowedLaunchPackages,
			}

			// 4. CLI flags override the config file only when explicitly set;
			// otherwise the file's values win (so the device-plugin can set
			// http_addr in the mounted config file and have it honored in both
			// local and pod mode). The package default is the last resort for
			// socket_path.
			if cmd.Flags().Changed("socket-path") {
				cfg.SocketPath = socketPath
			}
			if cfg.SocketPath == "" {
				cfg.SocketPath = roscontroller.DefaultServerConfig().SocketPath
			}
			if cmd.Flags().Changed("http-addr") {
				cfg.HTTPAddr = httpAddr
			}

			// 5. Build and run.
			server := roscontroller.NewServer(cfg)
			return server.Run()
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "/etc/rlinf/ros-controller.yaml",
		"Path to the YAML config file (macvlans, types, robots)")

	cmd.Flags().StringVar(&socketPath, "socket-path",
		roscontroller.DefaultServerConfig().SocketPath,
		"Unix socket path for the gRPC server (overridden only when set; otherwise the config file's value wins)")

	cmd.Flags().StringVar(&httpAddr, "http-addr", "",
		"Optional TCP address (host:port or :port) for the HTTP/JSON gateway; empty disables HTTP (overridden only when set; otherwise the config file's value wins)")

	cmd.Flags().StringVar(&podIP, "pod-ip", podIP,
		"Pod IP on the container network (default: $POD_IP) — used as ROS_IP for robot nodes")

	if err := cmd.Execute(); err != nil {
		log.Fatalf("[ros-controller] fatal: %v", err)
	}
}
