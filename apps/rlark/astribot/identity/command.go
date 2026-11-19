package identity

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Command gives the management plane's clients identities of their own.
func Command() *cobra.Command {
	var o identityOptions
	cmd := &cobra.Command{
		Use:   "identity",
		Short: "Give each client of the management plane (kcp) an identity of its own",
		Long: `Each client of the management plane gets a ServiceAccount in kcp's root workspace, the permissions it
needs and a token kcp signs with its service-account key. Unlike the administrator's credential, which kcp
writes anew at every start, the tokens stay valid when kcp restarts.

The credentials go where the components read them:
  --output-dir DIR          <DIR>/<component>/{kubeconfig,token}, for components started by docker compose
  --cluster-kubeconfig F    Secrets rlark-<component>-kcp in --namespace of that cluster (rlarkadm's Kubernetes mode)
  --station-kubeconfig F    Station's Secrets station-management-kubeconfig(-controller) in station-system of that cluster
  --station                 Station's credentials too, into --output-dir (Station's API and controller run by docker)

A component reads its token from the file beside its kubeconfig and reads it again within a minute when it
changes, so a rotation needs no restart.`,
	}
	o.flags(cmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "issue",
		Short: "Make every identity and write its credential (again); a token already issued is kept",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return o.run(cmd.Context(), func(ctx context.Context, kcp kubernetes.Interface, c Component, server string, ca []byte, store Store) error {
				cred, err := Issue(ctx, kcp, c, server, ca)
				if err != nil {
					return err
				}
				if err := store.Put(ctx, c, cred); err != nil {
					return err
				}
				fmt.Printf("%s: token %s, in %s\n", c.Name, cred.Secret, store.Where(c))
				return nil
			})
		},
	})
	var grace time.Duration
	rotate := &cobra.Command{
		Use:   "rotate",
		Short: "Give every identity a new token, then retire the old ones once the components have read the new one",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Every component gets its new token first; the old ones are retired together, once.
			type rotated struct {
				kcp  kubernetes.Interface
				c    Component
				keep string
			}
			var all []rotated
			ctx := cmd.Context()
			if err := o.run(ctx, func(ctx context.Context, kcp kubernetes.Interface, c Component, server string, ca []byte, store Store) error {
				cred, err := Rotate(ctx, kcp, c, server, ca)
				if err != nil {
					return err
				}
				if err := store.Put(ctx, c, cred); err != nil {
					return err
				}
				fmt.Printf("%s: new token %s, in %s\n", c.Name, cred.Secret, store.Where(c))
				all = append(all, rotated{kcp, c, cred.Secret})
				return nil
			}); err != nil {
				return err
			}
			fmt.Printf("the old tokens are retired in %s\n", grace)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(grace):
			}
			for _, r := range all {
				retired, err := Retire(ctx, r.kcp, r.c, r.keep)
				if err != nil {
					return fmt.Errorf("%s: %w", r.c.Name, err)
				}
				fmt.Printf("%s: retired %v\n", r.c.Name, retired)
			}
			return nil
		},
	}
	// A Pod sees a changed Secret within about a minute, and a client reads its token file again every minute.
	rotate.Flags().DurationVar(&grace, "grace", 3*time.Minute, "How long the old tokens stay valid after the new one is written")
	cmd.AddCommand(rotate)
	return cmd
}

type identityOptions struct {
	kcpKubeconfig     string
	kcpServer         string
	outputDir         string
	clusterKubeconfig string
	namespace         string
	stationKubeconfig string
	station           bool
	only              []string
}

func (o *identityOptions) flags(cmd *cobra.Command) {
	f := cmd.PersistentFlags()
	f.StringVar(&o.kcpKubeconfig, "kcp-kubeconfig", "", "kcp's administrator kubeconfig, as kcp wrote it at this start (read once, given to no component)")
	f.StringVar(&o.kcpServer, "kcp-server", "", "kcp's address as the components reach it, e.g. https://kcp:6443 (default: the administrator kubeconfig's)")
	f.StringVar(&o.outputDir, "output-dir", "", "Write the RLark components' credentials under this directory")
	f.StringVar(&o.clusterKubeconfig, "cluster-kubeconfig", "", "Write the RLark components' credentials as Secrets into this cluster")
	f.StringVar(&o.namespace, "namespace", "rlark-system", "Namespace of the Secrets written with --cluster-kubeconfig")
	f.StringVar(&o.stationKubeconfig, "station-kubeconfig", "", "Write Station's credentials as Secrets into this cluster (the one Station's API and controller run in)")
	f.BoolVar(&o.station, "station", false, "Also write Station's credentials into --output-dir")
	f.StringSliceVar(&o.only, "component", nil, "Only these components (default: all that have somewhere to go)")
}

type identityAction func(ctx context.Context, kcp kubernetes.Interface, c Component, server string, ca []byte, store Store) error

func (o *identityOptions) run(ctx context.Context, act identityAction) error {
	if o.kcpKubeconfig == "" {
		return fmt.Errorf("--kcp-kubeconfig is required")
	}
	if (o.outputDir == "") == (o.clusterKubeconfig == "") && o.stationKubeconfig == "" {
		return fmt.Errorf("give one of --output-dir and --cluster-kubeconfig, or --station-kubeconfig")
	}
	if o.station && o.outputDir == "" {
		return fmt.Errorf("--station writes into --output-dir; for a cluster give --station-kubeconfig")
	}
	if o.outputDir != "" && o.clusterKubeconfig != "" {
		return fmt.Errorf("--output-dir and --cluster-kubeconfig are two places for the same credentials; give one")
	}
	admin, err := clientcmd.LoadFromFile(o.kcpKubeconfig)
	if err != nil {
		return err
	}
	// The root workspace: kcp's administrator kubeconfig names its context "root".
	config, err := clientcmd.NewNonInteractiveClientConfig(*admin, RootContext, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
	if err != nil {
		return fmt.Errorf("context %q of %s: %w", RootContext, o.kcpKubeconfig, err)
	}
	server, err := Server(config.Host, o.kcpServer)
	if err != nil {
		return err
	}
	kcp, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	type target struct {
		components []Component
		store      Store
	}
	var targets []target
	switch {
	case o.outputDir != "":
		components := RLark()
		if o.station {
			components = append(components, Station()...)
		}
		targets = append(targets, target{components, Dir(o.outputDir)})
	case o.clusterKubeconfig != "":
		cluster, err := clusterClient(o.clusterKubeconfig)
		if err != nil {
			return err
		}
		targets = append(targets, target{RLark(), Secrets{Client: cluster, Namespace: o.namespace, Name: RLarkSecretName}})
	}
	if o.stationKubeconfig != "" {
		cluster, err := clusterClient(o.stationKubeconfig)
		if err != nil {
			return err
		}
		targets = append(targets, target{Station(), Secrets{Client: cluster, Namespace: StationNamespace, Name: StationSecretName}})
	}
	for _, t := range targets {
		for _, c := range t.components {
			if len(o.only) > 0 && !slices.Contains(o.only, c.Name) {
				continue
			}
			if err := act(ctx, kcp, c, server, config.CAData, t.store); err != nil {
				return fmt.Errorf("%s: %w", c.Name, err)
			}
		}
	}
	return nil
}

func clusterClient(path string) (kubernetes.Interface, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewForConfig(config)
}
