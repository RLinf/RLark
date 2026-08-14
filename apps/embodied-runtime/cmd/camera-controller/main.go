package main

import (
	"log"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/cameracontroller"
	"github.com/spf13/cobra"
)

func main() {
	var (
		configPath string
		socketPath string
		httpAddr   string
	)

	cmd := &cobra.Command{
		Use:   "camera-controller",
		Short: "Camera Controller — manages camera devices",
		Long: `camera-controller is the camera device lifecycle manager for the
embodied-runtime DaemonSet. It exposes a gRPC server on a Unix socket
that user containers call to open/close cameras and capture frames.

Cameras are configured statically in a YAML config file and registered
at startup. The server supports multiple cameras of different types.

Optionally, an HTTP/JSON gateway (--http-addr) mirrors every gRPC RPC
over REST under /v1/, so non-gRPC clients (curl, browsers, fetch) can
list cameras, open/close them, capture frames, and watch streams.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 1. Load config file (Cameras, HTTPAddr).
			fileCfg, err := cameracontroller.LoadConfigFile(configPath)
			if err != nil {
				return err
			}

			// 2. Enrich device-specific config — V4L2 capabilities (pixel
			// formats, resolutions, framerates) by opening the device and
			// issuing V4L2 ioctls. Best-effort, mirroring the ros-controller's
			// MACVLAN enrichment; the camera-controller runs with host device
			// access, so this step lives here rather than in the device plugin.
			for i := range fileCfg.Cameras {
				cameracontroller.EnrichV4L2Config(&fileCfg.Cameras[i])
			}

			// 3. Assemble the runtime ServerConfig from the file config.
			// SocketPath is CLI-only (not in the file); HTTPAddr and Cameras
			// come from the file and may be overridden by flags below.
			cfg := cameracontroller.ServerConfig{
				Cameras:  fileCfg.Cameras,
				HTTPAddr: fileCfg.HTTPAddr,
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
				cfg.SocketPath = cameracontroller.DefaultServerConfig().SocketPath
			}
			if cmd.Flags().Changed("http-addr") {
				cfg.HTTPAddr = httpAddr
			}

			// 5. Build and run.
			server, err := cameracontroller.NewServer(cfg)
			if err != nil {
				return err
			}
			return server.Run()
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "/etc/rlinf/camera-controller.yaml",
		"Path to the YAML config file (cameras)")

	cmd.Flags().StringVar(&socketPath, "socket-path",
		cameracontroller.DefaultServerConfig().SocketPath,
		"Unix socket path for the gRPC server (overridden only when set; otherwise the config file's value wins)")

	cmd.Flags().StringVar(&httpAddr, "http-addr", "",
		"Optional TCP address (host:port or :port) for the HTTP/JSON gateway; empty disables HTTP (overridden only when set; otherwise the config file's value wins)")

	if err := cmd.Execute(); err != nil {
		log.Fatalf("[camera-controller] fatal: %v", err)
	}
}
