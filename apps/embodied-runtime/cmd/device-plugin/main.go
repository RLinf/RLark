package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/rlinf/rlark/apps/embodied-runtime/pkg/deviceplugin"
	"github.com/spf13/cobra"
)

func main() {
	var (
		configPath     string
		wh             deviceplugin.WebhookConfig
		webhookEnabled bool
	)

	cmd := &cobra.Command{
		Use:   "device-plugin",
		Short: "Kubernetes Device Plugin for robot and camera devices",
		Long: `device-plugin is a Kubernetes Device Plugin that exposes robot and
camera devices as a schedulable resource (rlinf.io/device). It runs as part
of a DaemonSet and coordinates with the ROS Core, Robot Node Controller,
and Camera Controller to provide secure, isolated access to task pods.

Architecture:
  - Registers with kubelet as a device plugin
  - Detects robot hardware on the host (NICs, ROS installation)
  - Generates ros-controller, ros2-controller, and camera-controller configuration
  - Starts and manages the ros-controller, ros2-controller, and camera-controller
    (as local subprocesses or Kubernetes Pods, per manager_mode)
  - Injects ROS environment variables and Unix socket mounts into pods
  - Optionally runs a mutating webhook that auto-injects the devinit init
    container into pods requesting the resource when host_macvlans is
    configured (see --webhook flags)

Configuration is loaded from a YAML file (--config). When omitted, built-in
defaults are used. See examples/device-plugin-config.yaml for a template.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var cfg deviceplugin.PluginConfig
			if configPath != "" {
				c, err := deviceplugin.LoadConfig(configPath)
				if err != nil {
					return err
				}
				cfg = c
			} else {
				cfg = deviceplugin.DefaultPluginConfig()
			}
			wh.Enabled = webhookEnabled
			return run(cfg, wh)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "",
		"Path to the plugin configuration YAML file (omit to use built-in defaults)")

	// Webhook flags. These are deployment concerns (which Service fronts the
	// webhook, which MutatingWebhookConfiguration to manage), so they live on
	// the CLI rather than in the node-level plugin config. The webhook only
	// starts when --webhook is set AND the config declares host_macvlans.
	cmd.Flags().BoolVar(&webhookEnabled, "webhook", false,
		"Enable the mutating webhook that auto-injects the devinit init container (requires host_macvlans in the config)")
	cmd.Flags().StringVar(&wh.Addr, "webhook-addr", "",
		"HTTPS listen address for the webhook (default \":9443\")")
	cmd.Flags().StringVar(&wh.Path, "webhook-path", "",
		"Admission endpoint path served by the webhook (default \"/mutate\")")
	cmd.Flags().StringVar(&wh.MutatingWebhookConfigName, "webhook-mutating-config", "",
		"Name of the MutatingWebhookConfiguration whose caBundle is auto-managed (required when --webhook)")
	cmd.Flags().StringVar(&wh.ServiceName, "webhook-service-name", "",
		"Name of the Kubernetes Service fronting the webhook (forms the serving cert DNS SAN; required when --webhook)")
	cmd.Flags().StringVar(&wh.Namespace, "webhook-service-namespace", "",
		"Namespace of the Kubernetes Service fronting the webhook (required when --webhook)")
	cmd.Flags().StringVar(&wh.CASecretName, "webhook-ca-secret-name", "",
		"Name of the Secret persisting the webhook CA cert+key (empty = in-memory, regenerated each start)")
	cmd.Flags().StringVar(&wh.CASecretNamespace, "webhook-ca-secret-namespace", "",
		"Namespace of the CA Secret (required when --webhook-ca-secret-name is set)")
	cmd.Flags().StringVar(&wh.DevinitImage, "webhook-devinit-image", "",
		"Image for the injected init container; must contain devinit at /usr/local/bin/devinit (default: auto-discovered device-plugin image)")

	if err := cmd.Execute(); err != nil {
		log.Fatalf("[device-plugin] fatal: %v", err)
	}
}

func run(cfg deviceplugin.PluginConfig, wh deviceplugin.WebhookConfig) error {
	log.Println("[device-plugin] starting...")
	log.Printf("[device-plugin] camera_manager_mode=%s ros_manager_mode=%s ros2_manager_mode=%s",
		cfg.Camera.ManagerMode, cfg.ROS.ManagerMode, cfg.ROS2.ManagerMode)
	log.Printf("[device-plugin] resource=%s socket=%s",
		cfg.EffectiveResourceName(), cfg.EffectiveSocketPath())
	if wh.Enabled {
		log.Printf("[device-plugin] webhook enabled: config=%q service=%s/%s addr=%s",
			wh.MutatingWebhookConfigName, wh.Namespace, wh.ServiceName, wh.EffectiveAddr())
	} else {
		log.Println("[device-plugin] webhook disabled")
	}

	// Ensure the socket directory exists.
	if err := deviceplugin.EnsureSocketDir(); err != nil {
		return err
	}

	// Create the plugin. This also detects devices, generates the
	// ros-controller, ros2-controller, and camera-controller configs, and
	// starts the controllers (local subprocesses or pods, per *_manager_mode).
	// When the webhook is enabled, a Kubernetes clientset is also created so
	// the webhook can manage its caBundle and auto-discover the device-plugin
	// image (used as the default devinit init container image).
	plugin := deviceplugin.NewPlugin(cfg, wh)

	// Create the gRPC server. When the webhook is enabled and host_macvlans
	// is configured, a mutating webhook server is also constructed here and
	// started alongside the gRPC server.
	server := deviceplugin.NewServer(plugin, cfg.EffectiveSocketPath())

	// Register with kubelet in the background.
	if !cfg.SkipRegister {
		go func() {
			if err := server.RegisterWithKubelet(); err != nil {
				log.Printf("[device-plugin] WARNING: kubelet registration failed: %v", err)
				log.Printf("[device-plugin] kubelet may not be running — plugin will retry on restart")
			}
		}()
	} else {
		log.Println("[device-plugin] kubelet registration skipped")
	}

	// Start the gRPC server (blocks until signal).
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("[device-plugin] gRPC server error: %v", err)
		}
	}()

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh

	log.Printf("[device-plugin] received %v, shutting down...", sig)
	server.Stop()
	log.Println("[device-plugin] stopped")

	return nil
}
