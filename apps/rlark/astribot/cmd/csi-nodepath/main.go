// Command csi-nodepath runs the CSI driver of RLark's host-path storages
// (apps/rlark/astribot/nodepath): the same binary is the controller plugin beside
// csi-provisioner and, with --node, the node plugin on every node, which also
// probes the node's storages (labels and events).
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/rlinf/rlark/apps/rlark/astribot/nodepath"
	"github.com/rlinf/rlark/apps/rlark/pkg/version"
)

func main() {
	var endpoint, nodeID, hostRoot, stateDir string
	var node bool
	var probeInterval time.Duration
	cmd := &cobra.Command{
		Use:     "csi-nodepath",
		Short:   "CSI driver that bind-mounts a directory the node already has into Pods",
		Version: version.String(),
		RunE: func(*cobra.Command, []string) error {
			socket, found := strings.CutPrefix(endpoint, "unix://")
			if !found {
				return fmt.Errorf("--endpoint must be unix://<path>, got %q", endpoint)
			}
			if err := os.Remove(socket); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove stale socket: %w", err)
			}
			listener, err := net.Listen("unix", socket)
			if err != nil {
				return fmt.Errorf("listen on %s: %w", socket, err)
			}
			published := &nodepath.PublishedStore{Dir: stateDir}
			driver := &nodepath.Driver{NodeID: nodeID, Version: version.String(), HostRoot: hostRoot, Mounter: nodepath.LinuxMounter{},
				Mounts: nodepath.ReadMountInfo, Published: published}
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
			defer stop()
			if node {
				config, err := rest.InClusterConfig()
				if err != nil {
					return fmt.Errorf("in-cluster config: %w", err)
				}
				client, err := kubernetes.NewForConfig(config)
				if err != nil {
					return fmt.Errorf("kubernetes client: %w", err)
				}
				prober := &nodepath.Prober{Client: client, Node: nodeID, HostRoot: hostRoot, Mounts: nodepath.ReadMountInfo, Published: published, Now: time.Now}
				go prober.Run(ctx, probeInterval, func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) })
			}
			server := grpc.NewServer()
			csi.RegisterIdentityServer(server, driver)
			csi.RegisterControllerServer(server, driver)
			csi.RegisterNodeServer(server, driver)
			go func() { <-ctx.Done(); server.GracefulStop() }()
			return server.Serve(listener)
		},
	}
	cmd.Flags().StringVar(&endpoint, "endpoint", "unix:///csi/csi.sock", "CSI endpoint")
	cmd.Flags().StringVar(&nodeID, "node-id", os.Getenv("NODE_NAME"), "this node's name (defaults to $NODE_NAME)")
	cmd.Flags().StringVar(&hostRoot, "host-root", "/host", "where the node's root file system is mounted in this container")
	cmd.Flags().StringVar(&stateDir, "state-dir", "/csi/published", "where the node plugin records the volumes it bound (on the node, outliving the plugin)")
	cmd.Flags().BoolVar(&node, "node", false, "run as the node plugin: probe the node's storages, label the node and report remounts")
	cmd.Flags().DurationVar(&probeInterval, "probe-interval", 30*time.Second, "how often the node plugin probes the node's storages")
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
