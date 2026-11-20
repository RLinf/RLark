// Package rlarkctl holds our rlarkctl commands: release and station.
package rlarkctl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/rlinf/rlark/apps/rlark/astribot/release"
	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
)

// Commands are our rlarkctl commands, added to its root.
func Commands() []*cobra.Command {
	return []*cobra.Command{releaseCommand(), stationCommand()}
}

// releaseCommand is what makes a release and brings an environment to it.
func releaseCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "release", Short: "Make a release and bring an environment to it."}
	cmd.AddCommand(applyCommand(), candidateCommand(), sourceCommand(), credentialCommand(), copyCommand(), archiveCommand(), kubectlCommand())
	return cmd
}

// clients are the typed and dynamic clients of a cluster.
func clients(config *rest.Config) (kubernetes.Interface, dynamic.Interface, error) {
	core, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(config)
	return core, dynamicClient, err
}

// managementClients are the management plane's clients as the environment's kubeconfig (the release's identity)
// reaches it.
func managementClients(environment release.Environment) (kubernetes.Interface, dynamic.Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", environment.Management.Kubeconfig)
	if err != nil {
		return nil, nil, err
	}
	return clients(config)
}

// site reaches the environment as its file says, from the management machine.
func site(ctx context.Context, environment release.Environment) (*release.Site, error) {
	core, management, err := managementClients(environment)
	if err != nil {
		return nil, err
	}
	// The Packages of Addons are published from the catalog the Gateway is given.
	addons.Registry.SetDirectory(environment.AddonCatalog)
	site := &release.Site{
		Environment: environment, Management: management, Core: core,
		Members: map[string]kubernetes.Interface{}, MemberDefinitions: map[string]dynamic.Interface{},
		Plane: release.Compose{Project: environment.ControlPlane.Project, File: environment.ControlPlane.Compose, Env: environment.ControlPlane.Env},
	}
	// The clusters are the management plane's, each by the release's credential for it.
	configs, err := release.MemberConfigs(ctx, management, core)
	if err != nil {
		return nil, err
	}
	for name, config := range configs {
		if site.Members[name], site.MemberDefinitions[name], err = clients(config); err != nil {
			return nil, err
		}
	}
	if _, found := site.Members[environment.Station.Cluster]; !found {
		return nil, fmt.Errorf("station.cluster %q is not a cluster of the management plane", environment.Station.Cluster)
	}
	// The registry the release is kept in, by the account the management plane holds for it, and the site's own.
	if site.Registry, err = release.NewRegistryClient(ctx, environment.Registry.Name, environment.Registry.Source, core); err != nil {
		return nil, err
	}
	if site.SiteRegistry, err = release.NewRegistryClient(ctx, environment.Registry.Name, environment.Registry.Site, core); err != nil {
		return nil, err
	}
	site.Installer = &release.StationBootstrap{Dir: environment.Station.BootstrapDir, URL: environment.Station.BootstrapURL, Owner: release.BootstrapUser, Registry: site.Registry}
	return site, nil
}

func applyCommand() *cobra.Command {
	var environmentPath, releasePath, treePath, toPath string
	var back, dryRun bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Bring an environment to its release, stage by stage, reading back each one; stop at the first that does not settle.",
		Long: `Bring an environment to the release its file names (or, with --to, to a state a run recorded before it
changed anything), walking the stages of stages.yaml in order: what differs, then the reading back of it and the
stage's checks, until they hold or the stage times out. It stops at the first stage that does not settle and prints
the command that takes the site back. --dry-run prints what each stage would change and does nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			environment, rel, err := release.LoadEnvironment(environmentPath)
			if err != nil {
				return err
			}
			if releasePath != "" {
				if rel, err = release.LoadRelease(releasePath); err != nil {
					return err
				}
			}
			run := &release.Run{
				Back: back, DryRun: dryRun, Timeout: timeout, Poll: 5 * time.Second, Out: cmd.OutOrStdout(),
				RollbackCommand: func(before string) string {
					return fmt.Sprintf("rlarkctl release apply -e %s --to %s --back", environmentPath, before)
				},
			}
			if run.Site, err = site(cmd.Context(), environment); err != nil {
				return err
			}
			if toPath != "" {
				data, err := os.ReadFile(toPath)
				if err != nil {
					return err
				}
				if err := json.Unmarshal(data, &run.Desired); err != nil {
					return fmt.Errorf("%s: %w", toPath, err)
				}
				run.Name = "to-" + strings.TrimSuffix(filepath.Base(toPath), ".json")
			} else {
				installed, err := run.Site.Installed(cmd.Context())
				if err != nil {
					return err
				}
				run.Desired = environment.Desired(rel, installed)
				// What the site's registry is to hold: every image of the release and the installer's base.
				run.Images = append(slices.Sorted(maps.Values(rel.Images)), environment.Installer.Base)
				run.Name = rel.Name
				if !back {
					tree, err := release.ReadTree(treePath)
					if err != nil {
						return err
					}
					if tree.Commit != rel.Source.RLark {
						return fmt.Errorf("%s is the tree of %s; release %s is built from %s", treePath, tree.Commit, rel.Name, rel.Source.RLark)
					}
					run.Tree = &tree
					run.Desired.Compose = environment.Compose(tree)
					if run.Desired.Record, err = release.CurrentRelease(environment, rel, tree); err != nil {
						return err
					}
				}
			}
			cmd.SilenceUsage = true
			return run.Apply(cmd.Context())
		},
	}
	cmd.Flags().StringVarP(&environmentPath, "environment", "e", "", "The environment file (astribot/release/environments/<name>.yaml)")
	cmd.Flags().StringVarP(&releasePath, "release", "r", "", "A release file in place of the one the environment names (a release just built, astribot/release)")
	cmd.Flags().StringVar(&treePath, "tree", "", "git archive of the release's RLark commit, whose definitions (CRDs, management RBAC) the run applies")
	cmd.Flags().StringVar(&toPath, "to", "", "A state a run recorded before it changed anything: the run takes the site back to it")
	cmd.Flags().BoolVar(&back, "back", false, "Walk the stages in reverse (the server before the agents), as taking a release back does")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print what each stage would change and change nothing")
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "How long a stage may take to settle (stages.yaml's timeoutFactor times this)")
	_ = cmd.MarkFlagRequired("environment")
	cmd.MarkFlagsMutuallyExclusive("to", "tree")
	cmd.MarkFlagsMutuallyExclusive("to", "release")
	return cmd
}

func credentialCommand() *cobra.Command {
	var environmentPath, cluster, kubeconfig, treePath string
	cmd := &cobra.Command{
		Use:   "credential",
		Short: "Give the release its credential for a member cluster the Gateway did not make (Station's own).",
		Long: `Write into the member cluster what the release may do there (the tree's astribot/release/member-rbac.yaml: the
ServiceAccount rlark-system/rlark-release, its token, its roles), as the cluster's administrator (--kubeconfig), and
keep its token in the management plane as the cluster's credential (rlark-release-member-credentials/release-<cluster>):
its server where the administrator reaches the API: at the address the SiteNetwork's names give the API's name
(cp.<…>.rlark.internal), where they declare it, verified as that name. The Gateway does the same for a cluster it makes.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			environment, _, err := release.LoadEnvironment(environmentPath)
			if err != nil {
				return err
			}
			tree, err := release.ReadTree(treePath)
			if err != nil {
				return err
			}
			managementCore, managementDynamic, err := managementClients(environment)
			if err != nil {
				return err
			}
			administrator, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
			if err != nil {
				return err
			}
			member, memberDynamic, err := clients(administrator)
			if err != nil {
				return err
			}
			server, serverName, err := release.ReachedAs(cmd.Context(), managementDynamic, administrator)
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true
			if err := release.IssueMemberCredential(cmd.Context(), member, memberDynamic, managementCore, cluster, server, serverName, []byte(tree.Files["release-member-rbac.yaml"])); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "cluster %s: the release's credential is %s/%s, its server %s (verified as %q)\n", cluster, release.CredentialNamespace, release.CredentialName(cluster), server, serverName)
			return nil
		},
	}
	cmd.Flags().StringVarP(&environmentPath, "environment", "e", "", "The environment file: the management plane, as the command's identity")
	cmd.Flags().StringVar(&cluster, "cluster", "", "The member cluster, as the management plane names it")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "The member cluster's administrator kubeconfig")
	cmd.Flags().StringVar(&treePath, "tree", "", "git archive of the RLark commit whose astribot/release/member-rbac.yaml is written")
	for _, required := range []string{"environment", "cluster", "kubeconfig", "tree"} {
		_ = cmd.MarkFlagRequired(required)
	}
	return cmd
}

func candidateCommand() *cobra.Command {
	var basePath, treePath, name, fleet string
	var images []string
	cmd := &cobra.Command{
		Use:   "candidate",
		Short: "Write the release of a tree: a base release with what the tree pins, the images built from it and its Addon versions.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			base, err := release.LoadRelease(basePath)
			if err != nil {
				return err
			}
			tree, err := release.ReadTree(treePath)
			if err != nil {
				return err
			}
			built := map[string]string{}
			for _, image := range images {
				component, reference, _ := strings.Cut(image, "=")
				built[component] = reference
			}
			candidate, err := release.Candidate(base, tree, name, fleet, built)
			if err != nil {
				return err
			}
			path, err := candidate.Write()
			if err != nil {
				return err
			}
			// Read back as apply reads it: every component by its digest, every image its Addons name.
			if _, err := release.LoadRelease(path); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
	cmd.Flags().StringVar(&basePath, "base", "", "The release the candidate is made from (the live environment's)")
	cmd.Flags().StringVar(&treePath, "tree", "", "git archive of the candidate's RLark commit")
	cmd.Flags().StringVar(&name, "name", "", "The candidate's name; it is written as <name>.yaml beside the base")
	cmd.Flags().StringVar(&fleet, "fleet", "", "The Fleet commit the candidate's Fleet images are built from")
	cmd.Flags().StringArrayVar(&images, "image", nil, "<component>=<image by digest>, built from the candidate's commits")
	for _, flag := range []string{"base", "tree", "name"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}

// registryAccess is a registry as flags name it: where this machine reaches it, the CA it is trusted by and the image
// registry credential of the management plane whose account it takes, read by the identity of a kubeconfig.
type registryAccess struct {
	release.RegistryAccess
	management string
}

func (a *registryAccess) flags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&a.Address, "registry-address", "", "Where this machine reaches the registry (http://… or https://…)")
	cmd.Flags().StringVar(&a.CA, "registry-ca", "", "The CA it is trusted by")
	cmd.Flags().StringVar(&a.Credential, "registry-credential", "", "The image registry credential of the management plane (ir-…) whose account it takes")
	cmd.Flags().StringVar(&a.management, "management-kubeconfig", "", "With --registry-credential: the kubeconfig of the identity that reads it (the release's)")
	_ = cmd.MarkFlagRequired("registry-address")
	cmd.MarkFlagsRequiredTogether("registry-credential", "management-kubeconfig")
}

func (a *registryAccess) client(ctx context.Context) (*release.RegistryClient, error) {
	var core kubernetes.Interface
	if a.Credential != "" {
		config, err := clientcmd.BuildConfigFromFlags("", a.management)
		if err != nil {
			return nil, err
		}
		if core, err = kubernetes.NewForConfig(config); err != nil {
			return nil, err
		}
	}
	return release.NewRegistryClient(ctx, "", a.RegistryAccess, core)
}

func sourceCommand() *cobra.Command {
	var access registryAccess
	var repository, branch string
	push := &cobra.Command{
		Use:   "push",
		Short: "Push the tree of a branch's head (a git archive of the commit, on standard input) to the registry.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			archive, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}
			client, err := access.client(cmd.Context())
			if err != nil {
				return err
			}
			commit, err := client.PushSource(cmd.Context(), repository, branch, archive)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "pushed %s:%s (%s, %d bytes)\n", repository, commit, branch, len(archive))
			return nil
		},
	}
	access.flags(push)
	push.Flags().StringVar(&repository, "repository", "", "The repository the trees go to (fleet/source)")
	push.Flags().StringVar(&branch, "branch", "", "The branch whose head the tree is")
	for _, flag := range []string{"repository", "branch"} {
		_ = push.MarkFlagRequired(flag)
	}
	cmd := &cobra.Command{
		Use:   "source",
		Short: "The trees pushed for a build Job (Fleet's): a commit's tree as an artifact of the registry, tagged by the commit.",
	}
	cmd.AddCommand(push)
	return cmd
}

func copyCommand() *cobra.Command {
	var environmentPath, to string
	cmd := &cobra.Command{
		Use:   "copy -e ENVIRONMENT --to source|site IMAGE...",
		Short: "Copy images from one of the environment's registries into the other and print each by its digest.",
		Long: `Copy each IMAGE (REPOSITORY@sha256:… or REPOSITORY:TAG, the registry's name before it or not) from one of the
environment's registries (registry.source, registry.site) into the other, as the management machine reaches them:
the manifests and layers the other lacks, streamed; an image it holds by the digest is not sent again, an image by
its tag is tagged so there too. The images of a release built into the site's registry go to the source (--to
source), which the clusters that do not pull from the site's read; an image built elsewhere comes by its tag the
other way (--to site), which finds its digest.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, images []string) error {
			if to != "source" && to != "site" {
				return fmt.Errorf("--to is source or site, not %q", to)
			}
			environment, _, err := release.LoadEnvironment(environmentPath)
			if err != nil {
				return err
			}
			core, _, err := managementClients(environment)
			if err != nil {
				return err
			}
			name := environment.Registry.Name
			from, err := release.NewRegistryClient(cmd.Context(), name, environment.Registry.Source, core)
			if err != nil {
				return err
			}
			into, err := release.NewRegistryClient(cmd.Context(), name, environment.Registry.Site, core)
			if err != nil {
				return err
			}
			if to == "source" {
				from, into = into, from
			}
			cmd.SilenceUsage = true
			for _, image := range images {
				repository, reference, byDigest := strings.Cut(strings.TrimPrefix(image, name+"/"), "@")
				tag := ""
				if at := strings.LastIndex(repository, ":"); !byDigest && at > strings.LastIndex(repository, "/") {
					repository, reference, tag = repository[:at], repository[at+1:], repository[at+1:]
				}
				digest, err := from.Copy(cmd.Context(), into, repository, reference, repository, tag)
				if err != nil {
					return fmt.Errorf("%s: %w", image, err)
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s/%s@%s\n", name, repository, digest)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&environmentPath, "environment", "e", "", "The environment file whose registries these are")
	cmd.Flags().StringVar(&to, "to", "", "The registry the images go into: source or site")
	_ = cmd.MarkFlagRequired("environment")
	_ = cmd.MarkFlagRequired("to")
	return cmd
}

func archiveCommand() *cobra.Command {
	var source registryAccess
	var treePath, platform, output string
	cmd := &cobra.Command{
		Use:   "archive --tree TREE --platform OS/ARCH --output FILE",
		Short: "Write the images every node has from its installer (the tree's pinned.yaml installer) as an archive a node imports.",
		Long: `Write, as an OCI image layout in a tar, the images of the platform that the tree's astribot/release/pinned.yaml names
under installer, each by the name a Pod runs it by (registry name, repository and digest): the installer of the tree
carries one archive per platform (Dockerfile.installer), and a node it makes imports its platform's, so it has them
before it pulls anything. An image that is not of the platform is not in the archive; the ones that are are printed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			tree, err := release.ReadTree(treePath)
			if err != nil {
				return err
			}
			client, err := source.client(cmd.Context())
			if err != nil {
				return err
			}
			var images []string
			for _, component := range tree.Pinned.Installer {
				images = append(images, tree.Pinned.Images[component])
			}
			file, err := os.Create(output)
			if err != nil {
				return err
			}
			cmd.SilenceUsage = true
			held, err := client.Archive(cmd.Context(), file, images, platform)
			if closed := file.Close(); err == nil {
				err = closed
			}
			if err != nil {
				_ = os.Remove(output)
				return err
			}
			for _, image := range held {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), image)
			}
			return nil
		},
	}
	source.flags(cmd)
	cmd.Flags().StringVar(&treePath, "tree", "", "git archive of the RLark commit whose installer it is")
	cmd.Flags().StringVar(&platform, "platform", "", "The nodes' platform (linux/amd64, linux/arm64)")
	cmd.Flags().StringVar(&output, "output", "", "The archive to write")
	for _, flag := range []string{"tree", "platform", "output"} {
		_ = cmd.MarkFlagRequired(flag)
	}
	return cmd
}

func kubectlCommand() *cobra.Command {
	var environmentPath, cluster string
	cmd := &cobra.Command{
		Use:   "kubectl --cluster CLUSTER -- ARGS...",
		Short: "Run kubectl in a member cluster as its reader: what the cluster runs, read, and nothing written.",
		Long: `Run kubectl with ARGS in a member cluster as the cluster's reader, rlark-system/rlark-reader
(astribot/release/member-rbac.yaml, which every release writes): Kubernetes' view of every namespace (workloads, their
logs and events, no Secret) and the cluster's nodes and storage. The release's credential in the management plane
reaches the cluster; the reader's token is read there and given to kubectl in a file of its own, removed when kubectl
ends. No administrator's kubeconfig is kept on the machine for it.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			environment, _, err := release.LoadEnvironment(environmentPath)
			if err != nil {
				return err
			}
			core, _, err := managementClients(environment)
			if err != nil {
				return err
			}
			kubeconfig, err := release.ReaderKubeconfig(cmd.Context(), core, cluster)
			if err != nil {
				return err
			}
			file, err := os.CreateTemp("", "rlark-reader-*.kubeconfig")
			if err != nil {
				return err
			}
			defer os.Remove(file.Name())
			if _, err := file.Write(kubeconfig); err != nil {
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			cmd.SilenceUsage = true
			kubectl := exec.CommandContext(cmd.Context(), "kubectl", append([]string{"--kubeconfig", file.Name()}, args...)...)
			kubectl.Stdin, kubectl.Stdout, kubectl.Stderr = cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr()
			return kubectl.Run()
		},
	}
	cmd.Flags().StringVarP(&environmentPath, "environment", "e", "", "The environment file (astribot/release/environments/<name>.yaml)")
	cmd.Flags().StringVar(&cluster, "cluster", "", "The member cluster, by its name in the management plane")
	_ = cmd.MarkFlagRequired("environment")
	_ = cmd.MarkFlagRequired("cluster")
	return cmd
}
