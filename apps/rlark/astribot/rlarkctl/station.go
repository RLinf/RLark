package rlarkctl

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rlinf/rlark/apps/rlark/astribot/release"
	"github.com/rlinf/rlark/apps/rlark/astribot/stationmember"
)

// stationCommand registers a cluster the Gateway did not make with the site's Station: the cluster Station itself
// runs in, which is there before Station is. A cluster made by "new cluster" is registered by the Gateway, with the
// same code (stationmember).
func stationCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "station", Short: "A member cluster of the site's Station."}
	var management, memberPath, tree, caPath string
	var r stationmember.Registration
	register := &cobra.Command{
		Use:   "register",
		Short: "Register a member cluster with Station: its agents and how the management plane reaches its API.",
		Long: `Install Station's part into the member cluster (--member-kubeconfig, its administrator's) from the release's
tree (--tree: astribot/station/deploy), and store in kcp (--kubeconfig) how station-api and station-controller reach
its API: at the address the SiteNetwork's names give the name the administrator reaches it by. Run again, it changes
nothing that is as it should be.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			memberConfig, err := clientcmd.BuildConfigFromFlags("", memberPath)
			if err != nil {
				return err
			}
			managementConfig, err := clientcmd.BuildConfigFromFlags("", management)
			if err != nil {
				return err
			}
			managementClient, err := kubernetes.NewForConfig(managementConfig)
			if err != nil {
				return err
			}
			managementDynamic, err := dynamic.NewForConfig(managementConfig)
			if err != nil {
				return err
			}
			// The member is reached, and its credentials point, at the address the site's names give its API's name.
			if memberConfig.Host, r.APIName, err = release.ReachedAs(cmd.Context(), managementDynamic, memberConfig); err != nil {
				return err
			}
			memberConfig.TLSClientConfig.ServerName = r.APIName
			if r.StationAPICA, err = os.ReadFile(caPath); err != nil {
				return err
			}
			deploy := filepath.Join(tree, "astribot/station/deploy")
			for file, into := range map[string]*string{"member-rbac.yaml": &r.Manifests.MemberRBAC, "node-operation-member-rbac.yaml": &r.Manifests.OperationRBAC, "station-agent.yaml": &r.Manifests.StationAgent} {
				data, err := os.ReadFile(filepath.Join(deploy, file))
				if err != nil {
					return err
				}
				*into = string(data)
			}
			m, err := stationmember.New(memberConfig)
			if err != nil {
				return err
			}
			if err := stationmember.Register(cmd.Context(), m, managementClient, r); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s is a member of Station; its credentials are %s\n", r.Cluster, stationmember.SecretName(r.Cluster))
			return nil
		},
	}
	flags := register.Flags()
	flags.StringVar(&management, "kubeconfig", "", "kcp, as an identity that writes Station's member credentials")
	flags.StringVar(&memberPath, "member-kubeconfig", "", "The member cluster's administrator")
	flags.StringVar(&tree, "tree", "", "The release's tree")
	flags.StringVar(&caPath, "station-api-ca", "", "Station API's CA")
	flags.StringVar(&r.Cluster, "cluster", "", "The cluster's name")
	flags.StringVar(&r.StationAPIURL, "station-api-url", "", "Station API as the cluster's agents reach it (https://host:port)")
	flags.StringVar(&r.AgentImage, "agent-image", "", "The station agent's image")
	flags.StringVar(&r.WireguardImage, "wireguard-image", "", "The station wireguard image")
	flags.StringVar(&r.PullSecret, "pull-secret", "", "The image registry credential RLark delivers into station-system (ir-…)")
	for _, name := range []string{"kubeconfig", "member-kubeconfig", "tree", "station-api-ca", "cluster", "station-api-url", "agent-image", "wireguard-image", "pull-secret"} {
		_ = register.MarkFlagRequired(name)
	}
	cmd.AddCommand(register)
	return cmd
}
