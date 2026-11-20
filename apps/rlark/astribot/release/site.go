package release

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// The objects of a member cluster the release deploys by itself: what a
// cluster is in RLark with (rlarkadm's agent) and Station's own.
const (
	agentNamespace    = "rlark-system"
	nodeAgentDaemon   = "rlark-agent-node"
	stationNamespace  = "station-system"
	stationAPI        = "station-api"
	stationController = "station-controller"
	// rolloutAnnotation hands a DaemonSet's rollout to the cluster agent.
	rolloutAnnotation = "rlark.io/rollout"
)

// StationDaemonSets are Station's DaemonSets in every member cluster;
// stationContainers which image each container of them runs.
var (
	StationDaemonSets = []string{"station-agent", "station-agent-network", "station-agent-host"}
	stationContainers = map[string]func(ClusterState) string{
		"station-agent": func(c ClusterState) string { return c.StationAgent },
		"wireguard":     func(c ClusterState) string { return c.StationWireguard },
		// The host's gateway program, which the wireguard image carries (station-agent-host).
		"gateway-program": func(c ClusterState) string { return c.StationWireguard },
	}
)

// ControlPlane is the management machine's containers: RLark's server,
// controller-manager, gateway and console, one compose service each.
type ControlPlane interface {
	// Images is service -> the image its running container was started from.
	Images(ctx context.Context) (map[string]string, error)
	// Configs is service -> the hash of the definition its running container was made from.
	Configs(ctx context.Context) (map[string]string, error)
	// Compose is the compose file and its env file but the services' images, as the machine has them.
	Compose(ctx context.Context) (*ComposeFile, error)
	// Hashes is service -> the hash of its definition by compose and images, as the machine's compose makes it.
	Hashes(ctx context.Context, compose ComposeFile, images map[string]string) (map[string]string, error)
	// Write writes compose's file and values; the services' images in the env file stay, and nothing is started.
	Write(ctx context.Context, compose ComposeFile) error
	// Set starts the service anew with image; the others are not touched.
	Set(ctx context.Context, service, image string) error
	// Pull brings an image onto the machine.
	Pull(ctx context.Context, image string) error
}

// Site is an environment as the command reaches it.
type Site struct {
	Environment Environment
	// Management is the management plane as the command's own identity in kcp; Core the same, typed, for
	// the Addon Packages (ConfigMaps) it publishes.
	Management dynamic.Interface
	Core       kubernetes.Interface
	Members    map[string]kubernetes.Interface
	// MemberDefinitions are the same clusters, by the same kubeconfigs, for
	// the definitions the release writes into each (server-side apply).
	MemberDefinitions map[string]dynamic.Interface
	Plane             ControlPlane
	// Registry is the registry the release is kept in (environment registry.source); SiteRegistry the site's own
	// (registry.site).
	Registry, SiteRegistry *RegistryClient
	// Installer is Station API's machine, where the installer is laid down.
	Installer InstallerHost
}

// The management plane's objects, as far as the command reads them.
type (
	managedCluster struct {
		Spec struct {
			AgentNamespace string `json:"agentNamespace"`
			NodeAgents     struct {
				Image string `json:"image"`
			} `json:"nodeAgents"`
		} `json:"spec"`
	}
	managedAddon struct {
		Metadata struct {
			Generation int64 `json:"generation"`
		} `json:"metadata"`
		Spec struct {
			AddonName string            `json:"addonName"`
			Version   string            `json:"version"`
			Values    map[string]string `json:"values"`
		} `json:"spec"`
		Status struct {
			Phase              string `json:"phase"`
			Message            string `json:"message"`
			ObservedGeneration int64  `json:"observedGeneration"`
		} `json:"status"`
	}
)

func (s *Site) cluster(ctx context.Context, name string) (managedCluster, error) {
	object, err := s.Management.Resource(clusterGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return managedCluster{}, err
	}
	var cluster managedCluster
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, &cluster)
	return cluster, err
}

func (s *Site) addons(ctx context.Context, namespace string) (map[string]managedAddon, error) {
	return list[managedAddon](ctx, s.Management.Resource(addonGVR).Namespace(namespace))
}

// Installed is every cluster of the site -> the Addons installed in it.
func (s *Site) Installed(ctx context.Context) (map[string][]string, error) {
	installed := map[string][]string{}
	for name := range s.Members {
		cluster, err := s.cluster(ctx, name)
		if err != nil {
			return nil, err
		}
		addons, err := s.addons(ctx, cluster.Spec.AgentNamespace)
		if err != nil {
			return nil, err
		}
		installed[name] = slices.Sorted(maps.Keys(addons))
	}
	return installed, nil
}

// Read is what the site runs now.
func (s *Site) Read(ctx context.Context) (State, error) {
	state := State{Clusters: map[string]ClusterState{}}
	var err error
	if state.ControlPlane, err = s.Plane.Images(ctx); err != nil {
		return State{}, err
	}
	if state.ControlPlaneConfig, err = s.Plane.Configs(ctx); err != nil {
		return State{}, err
	}
	if state.Compose, err = s.Plane.Compose(ctx); err != nil {
		return State{}, err
	}
	station := s.Members[s.Environment.Station.Cluster]
	for name, into := range map[string]*string{stationAPI: &state.StationAPI, stationController: &state.StationController} {
		deployment, err := station.AppsV1().Deployments(stationNamespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return State{}, err
		}
		*into = deployment.Spec.Template.Spec.Containers[0].Image
		if name == stationAPI {
			state.Installer = deployment.Annotations[installerAnnotation]
		}
	}
	if state.Record, err = readRecord(ctx, s.Core); err != nil {
		return State{}, err
	}

	for name, member := range s.Members {
		agent, at, err := clusterAgent(ctx, member)
		if err != nil {
			return State{}, fmt.Errorf("cluster %s: %w", name, err)
		}
		container := agent.Spec.Template.Spec.Containers[at]
		current := ClusterState{Agent: container.Image, AgentDeployment: agent.Name, Addons: map[string]AddonState{}}
		for i, arg := range container.Args {
			if tools, found := strings.CutPrefix(arg, "--image="); found {
				current.Tools = tools
			} else if arg == "--image" && i+1 < len(container.Args) {
				current.Tools = container.Args[i+1]
			}
		}

		// Station's DaemonSets as the cluster has them, their images and the accounts they pull with, each read as
		// one only while they agree.
		pulls := map[string]bool{}
		for _, daemonSetName := range StationDaemonSets {
			daemonSet, err := member.AppsV1().DaemonSets(stationNamespace).Get(ctx, daemonSetName, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				continue
			}
			if err != nil {
				return State{}, fmt.Errorf("cluster %s: %w", name, err)
			}
			for _, container := range daemonSet.Spec.Template.Spec.Containers {
				switch container.Name {
				case "station-agent":
					current.StationAgent = agreeing(current.StationAgent, container.Image)
				case "wireguard", "gateway-program":
					current.StationWireguard = agreeing(current.StationWireguard, container.Image)
				}
			}
			var names []string
			for _, pull := range daemonSet.Spec.Template.Spec.ImagePullSecrets {
				names = append(names, pull.Name)
			}
			pulls[strings.Join(names, ",")] = true
		}
		current.StationPullSecret = strings.Join(slices.Sorted(maps.Keys(pulls)), " | ")

		cluster, err := s.cluster(ctx, name)
		if err != nil {
			return State{}, err
		}
		current.NodeAgent = cluster.Spec.NodeAgents.Image
		installed, err := s.addons(ctx, cluster.Spec.AgentNamespace)
		if err != nil {
			return State{}, err
		}
		for addonName, addon := range installed {
			current.Addons[addonName] = AddonState{Version: addon.Spec.Version, Values: addon.Spec.Values}
		}
		state.Clusters[name] = current
	}
	return state, nil
}

// Pods is every Pod of the member clusters: uid -> "<cluster> <namespace>/<owner>",
// the owner being the Deployment, DaemonSet or StatefulSet the Pod is of. A Job's
// Pods are not among them.
func (s *Site) Pods(ctx context.Context) (map[string]string, error) {
	pods := map[string]string{}
	for name, member := range s.Members {
		list, err := member.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("cluster %s: %w", name, err)
		}
		for _, pod := range list.Items {
			// A Job's Pod (a CronJob's through its Job) is made on the Job's own time, never by a release.
			if len(pod.OwnerReferences) > 0 && pod.OwnerReferences[0].Kind == "Job" {
				continue
			}
			pods[string(pod.UID)] = name + " " + pod.Namespace + "/" + ownerOf(pod)
		}
	}
	return pods, nil
}

func ownerOf(pod corev1.Pod) string {
	if len(pod.OwnerReferences) == 0 {
		return pod.Name
	}
	owner := pod.OwnerReferences[0]
	if owner.Kind == "ReplicaSet" {
		// <deployment>-<pod-template-hash>
		if at := strings.LastIndex(owner.Name, "-"); at > 0 {
			return owner.Name[:at]
		}
	}
	return owner.Name
}

func agreeing(read, image string) string {
	switch {
	case read == "":
		return image
	case read == image:
		return read
	}
	return read + " | " + image
}

// clusterAgent is the cluster's agent, whatever it was named when it was installed: the Deployment of
// rlark-system whose container runs the agent in its cluster mode (--mode=cluster), and that container's index.
func clusterAgent(ctx context.Context, member kubernetes.Interface) (*appsv1.Deployment, int, error) {
	deployments, err := member.AppsV1().Deployments(agentNamespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, 0, err
	}
	var found []*appsv1.Deployment
	at := 0
	for i := range deployments.Items {
		for c, container := range deployments.Items[i].Spec.Template.Spec.Containers {
			if runsInClusterMode(slices.Concat(container.Command, container.Args)) {
				found, at = append(found, &deployments.Items[i]), c
			}
		}
	}
	if len(found) != 1 {
		return nil, 0, fmt.Errorf("%d Deployments of %s run the agent with --mode=cluster; a cluster has one", len(found), agentNamespace)
	}
	return found[0], at, nil
}

func runsInClusterMode(args []string) bool {
	for i, arg := range args {
		if arg == "--mode=cluster" || arg == "--mode" && i+1 < len(args) && args[i+1] == "cluster" {
			return true
		}
	}
	return false
}

// stationDaemonSet is the DaemonSet name of the tree's astribot/station/deploy/station-agent.yaml, rendered with the
// cluster's images and the site's pull account.
func stationDaemonSet(file, name string, want ClusterState) (*appsv1.DaemonSet, error) {
	rendered := strings.NewReplacer("REPLACE_RLARK_IMAGE", want.StationAgent, "REPLACE_PRIVATE_WIREGUARD_IMAGE", want.StationWireguard,
		"REPLACE_IMAGE_PULL_SECRET", want.StationPullSecret).Replace(file)
	for _, document := range bytes.Split([]byte(rendered), []byte("\n---")) {
		var daemonSet appsv1.DaemonSet
		if err := yaml.Unmarshal(document, &daemonSet); err != nil {
			return nil, fmt.Errorf("station-agent.yaml: %w", err)
		}
		if daemonSet.Kind == "DaemonSet" && daemonSet.Name == name {
			return &daemonSet, nil
		}
	}
	return nil, fmt.Errorf("station-agent.yaml has no DaemonSet %s", name)
}
