package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"slices"
	"strings"
	"time"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
)

const (
	// acceleratorLabel marks a node with NVIDIA GPUs; the nvidia-gpu Addon selects it.
	acceleratorLabel = "rlark.io/accelerator"
	// siteRegistrySecret is RLark's secret object the site registries pull from their upstream with.
	siteRegistrySecret = "site-registry"
)

// provisionAddon is one Addon of a new cluster: the release's version and values, with the values that are the
// new site's own.
type provisionAddon struct {
	name  string
	sited map[string]string
}

// installProvisionAddon installs an Addon at the release's version, or brings an installed one to it, and waits
// until it is Ready.
func (g *Gateway) installProvisionAddon(ctx context.Context, cluster string, release provisionRelease, addon provisionAddon) (string, error) {
	brought, found := release.Addons[addon.name]
	if !found {
		return "", fmt.Errorf("the release %s has no Addon %s", release.Name, addon.name)
	}
	values := maps.Clone(brought.Values)
	if values == nil {
		values = map[string]string{}
	}
	maps.Copy(values, addon.sited)
	meta, _, err := g.publishAddonPackage(ctx, cluster, addon.name, brought.Version)
	if err != nil {
		return "", fmt.Errorf("publish %s %s: %w", addon.name, brought.Version, err)
	}
	if missing := addons.MissingRequired(meta, values); len(missing) > 0 {
		return "", fmt.Errorf("the release %s states no %s of Addon %s", release.Name, strings.Join(missing, ", "), addon.name)
	}
	client := g.kubeClient.RlinfV1alpha1().Addons(cluster)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		current, err := client.Get(ctx, addon.name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			_, err = client.Create(ctx, &rlarkv1alpha1.Addon{
				ObjectMeta: metav1.ObjectMeta{Name: addon.name, Namespace: cluster},
				Spec:       rlarkv1alpha1.AddonSpec{AddonName: addon.name, Version: meta.Version, Values: values},
			}, metav1.CreateOptions{})
			return err
		}
		if err != nil {
			return err
		}
		if current.Spec.Version == meta.Version && maps.Equal(current.Spec.Values, values) {
			return nil
		}
		current.Spec.Version, current.Spec.Values = meta.Version, values
		_, err = client.Update(ctx, current, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return "", fmt.Errorf("install %s: %w", addon.name, err)
	}
	phase, message := "", ""
	for deadline := time.Now().Add(10 * time.Minute); time.Now().Before(deadline); time.Sleep(5 * time.Second) {
		current, err := client.Get(ctx, addon.name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}
		phase, message = string(current.Status.Phase), current.Status.Message
		if current.Status.ObservedGeneration >= current.Generation && current.Status.Phase == rlarkv1alpha1.AddonPhaseReady {
			return fmt.Sprintf("%s %s Ready", addon.name, meta.Version), nil
		}
	}
	return "", fmt.Errorf("Addon %s is %s after 10 min: %s", addon.name, phase, message)
}

func (r *provisionRun) addons(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (string, error) {
	release, err := r.g.currentRelease(ctx)
	if err != nil {
		return "", err
	}
	facts := p.Status.Facts
	var readback []string
	registry := []provisionAddon{
		// registry-trust's own images are the release's; the machine trusts the registry from its installer already.
		{name: "registry-trust", sited: map[string]string{"hosts": release.Registry, "caCert": facts.RegistryCA,
			"image": release.Images["registry-trust"], "kubectlImage": release.Images["registry-trust-kubectl"]}},
		{name: "csi-driver-rclone"},
	}
	if len(facts.GPUs) > 0 {
		patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"labels": map[string]string{acceleratorLabel: "nvidia"}}})
		if _, err := r.g.kubeClient.RlinfV1alpha1().Nodes(p.Name).Patch(ctx, facts.NodeName, types.MergePatchType, patch, metav1.PatchOptions{}); err != nil {
			return "", fmt.Errorf("label node %s %s=nvidia: %w", facts.NodeName, acceleratorLabel, err)
		}
		registry = append(registry, provisionAddon{name: "nvidia-gpu", sited: map[string]string{"nodeSelector": acceleratorLabel + "=nvidia"}})
	}
	for _, addon := range registry {
		done, err := r.g.installProvisionAddon(ctx, p.Name, release, addon)
		if err != nil {
			return strings.Join(readback, "; "), err
		}
		readback = append(readback, done)
	}
	if len(facts.GPUs) > 0 {
		want := int64(0)
		for _, gpu := range facts.GPUs {
			want += int64(gpu.Count)
		}
		got, err := r.g.nodeGPUs(ctx, p.Name, facts.NodeName, want)
		readback = append(readback, fmt.Sprintf("%s nvidia.com/gpu %d", facts.NodeName, got))
		if err != nil {
			return strings.Join(readback, "; "), err
		}
	}
	if facts.CacheVolume {
		done, err := r.siteCache(ctx, p, release)
		if err != nil {
			return strings.Join(readback, "; "), err
		}
		readback = append(readback, done)
	}
	storages, err := r.g.addClusterToSiteStorage(ctx, p.Name)
	if err != nil {
		return strings.Join(readback, "; "), err
	}
	readback = append(readback, storages)
	return strings.Join(readback, "; "), nil
}

// nodeGPUs waits until the node offers the GPUs the machine has.
func (g *Gateway) nodeGPUs(ctx context.Context, cluster, name string, want int64) (int64, error) {
	var got int64
	for deadline := time.Now().Add(5 * time.Minute); time.Now().Before(deadline); time.Sleep(5 * time.Second) {
		node, err := g.kubeClient.RlinfV1alpha1().Nodes(cluster).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return 0, err
		}
		if quantity, found := node.Status.Capacity["nvidia.com/gpu"]; found {
			got = quantity.Value()
		}
		if got == want {
			return got, nil
		}
	}
	return got, fmt.Errorf("node %s offers %d nvidia.com/gpu after 5 min; the machine has %d", name, got, want)
}

// siteCache makes the new site pull through a cache of its own on the first machine: the cache pulls from the
// origin registry through the tunnel once, and the site's registry name points to it once it is Ready.
func (r *provisionRun) siteCache(ctx context.Context, p *rlarkv1alpha1.ClusterProvision, release provisionRelease) (string, error) {
	facts := p.Status.Facts
	if err := r.g.distributeJobSecret(ctx, siteRegistrySecret, p.Name); err != nil {
		return "", err
	}
	_, port, err := net.SplitHostPort(release.Registry)
	if err != nil {
		return "", err
	}
	done, err := r.g.installProvisionAddon(ctx, p.Name, release, provisionAddon{name: "site-registry", sited: map[string]string{
		"role": "cache", "listen": net.JoinHostPort(facts.PrivateAddress, port),
		"nodeSelector": "kubernetes.io/hostname=" + facts.NodeName,
		"upstream":     "https://" + net.JoinHostPort(originName, port), "caCert": facts.RegistryCA,
		"secret": siteRegistrySecret,
		// Each site's own (no release states them): a new site's first cache pulls them from the origin, not through itself.
		"image": release.Images["site-registry"], "relayImage": release.Images["site-registry-relay"],
	}})
	if err != nil {
		return "", err
	}
	host, _, _ := net.SplitHostPort(release.Registry)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		network, err := r.g.provisionNetwork(ctx)
		if err != nil {
			return err
		}
		sites, _, _ := unstructured.NestedSlice(network.object.Object, "spec", "sites")
		for _, raw := range sites {
			site, _ := raw.(map[string]interface{})
			if site["name"] != p.Spec.Site {
				continue
			}
			names, _, _ := unstructured.NestedSlice(site, "names")
			for _, rawName := range names {
				if name, _ := rawName.(map[string]interface{}); name["name"] == host {
					name["address"] = facts.PrivateAddress
				}
			}
			site["names"] = names
		}
		if err := unstructured.SetNestedSlice(network.object.Object, sites, "spec", "sites"); err != nil {
			return err
		}
		_, err = r.g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Update(ctx, network.object, metav1.UpdateOptions{})
		return err
	})
	if err != nil {
		return done, fmt.Errorf("point %s of site %s to its cache: %w", host, p.Spec.Site, err)
	}
	return fmt.Sprintf("%s; %s of site %s → %s", done, host, p.Spec.Site, facts.PrivateAddress), nil
}

// distributeJobSecret delivers one of RLark's secret objects to one more cluster, its values as they are.
func (g *Gateway) distributeJobSecret(ctx context.Context, name, cluster string) error {
	current, err := g.rawClient.CoreV1().Secrets(g.managementNamespace()).Get(ctx, jobSecretReplicationName(name), metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("RLark's secret object %s: %w", name, err)
	}
	var stored storedJobSecret
	if err := json.Unmarshal(current.Data["secret"], &stored); err != nil {
		return fmt.Errorf("RLark's secret object %s: %w", name, err)
	}
	if slices.Contains(stored.Clusters, cluster) {
		return nil
	}
	values, err := jobSecretValues(current)
	if err != nil {
		return err
	}
	return putJobSecret(ctx, g.rawClient, g.managementNamespace(), current.Annotations[tenant.CreatedByAnnotation],
		jobSecretRequest{Name: name, Description: stored.Description, Clusters: append(stored.Clusters, cluster), Data: values}, true)
}

// addClusterToSiteStorage gives the new cluster every S3 storage every other cluster has. A storage a cloud's
// clusters mount at an endpoint of their own network is mounted at the Gateway's own endpoint, which is reachable
// from anywhere.
func (g *Gateway) addClusterToSiteStorage(ctx context.Context, cluster string) (string, error) {
	clusters, err := g.kubeClient.RlinfV1alpha1().Clusters().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", err
	}
	var others []string
	for _, c := range clusters.Items {
		if c.Name != cluster {
			others = append(others, c.Name)
		}
	}
	secrets, err := listStorageClassSecrets(ctx, g.rawClient, g.managementNamespace())
	if err != nil {
		return "", fmt.Errorf("list storages: %w", err)
	}
	var added []string
	for i := range secrets {
		_, stored, err := storageClassFromSecret(&secrets[i])
		if err != nil || stored.Type != "" && stored.Type != storageTypeS3 {
			continue
		}
		if slices.Contains(stored.Clusters, cluster) {
			added = append(added, stored.Name)
			continue
		}
		if !slices.ContainsFunc(others, func(other string) bool { return !slices.Contains(stored.Clusters, other) }) {
			stored.Clusters = append(stored.Clusters, cluster)
			if stored.MountEndpoint != "" && stored.MountEndpoint != stored.Endpoint {
				if stored.ClusterMountEndpoints == nil {
					stored.ClusterMountEndpoints = map[string]string{}
				}
				stored.ClusterMountEndpoints[cluster] = stored.Endpoint
			}
			if _, err := g.updateStorageClassByName(ctx, stored.Name, stored); err != nil {
				return "", fmt.Errorf("give storage %s to %s: %w", stored.Name, cluster, err)
			}
			added = append(added, stored.Name)
		}
	}
	if len(added) == 0 {
		return "no storage every cluster has", nil
	}
	return "storages " + strings.Join(added, ", "), nil
}

// accept runs a small Job on the new cluster's node and reads what it printed: the GPUs it was given, or its
// host's name. Its image is the release's cluster-check. The Job is deleted once its output is read.
func (r *provisionRun) accept(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (string, error) {
	release, err := r.g.currentRelease(ctx)
	if err != nil {
		return "", err
	}
	image := release.Images["cluster-check"]
	if image == "" {
		return "", fmt.Errorf("the release %s states no image cluster-check (a shell, and the C library NVIDIA's tools need) to check a new cluster with", release.Name)
	}
	facts := p.Status.Facts
	name := "check-" + provisionAgentID(p.Name)
	jobs := r.g.kubeClient.RlinfV1alpha1().Jobs()
	if err := jobs.Delete(ctx, name, metav1.DeleteOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return "", err
	}
	run, want := "hostname; sleep infinity", facts.NodeName
	container := corev1.Container{Name: "main", Image: image, ImagePullPolicy: corev1.PullIfNotPresent}
	if len(facts.GPUs) > 0 {
		run, want = "nvidia-smi -L; sleep infinity", "GPU 0: "
		container.Resources.Limits = corev1.ResourceList{"nvidia.com/gpu": resource.MustParse("1")}
	}
	parameters, _ := json.Marshal(map[string]any{"run": map[string]string{"inline": run}})
	job := &rlarkv1alpha1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: rlarkv1alpha1.JobSpec{
			// A tag value is at most 10 characters (the Gateway's Job API).
			Tags: []rlarkv1alpha1.JobTag{{Key: "purpose", Values: []string{"check"}}},
			Tasks: []rlarkv1alpha1.JobTaskTemplate{{Name: "check", TaskSpec: rlarkv1alpha1.TaskSpec{
				Role:      "Worker",
				Placement: rlarkv1alpha1.TaskPlacementSpec{NodeSelector: map[string]string{"kubernetes.io/hostname": facts.NodeName}},
				Runtime:   rlarkv1alpha1.TaskRuntimeSpec{Kind: "Kubernetes"},
				Workload: rlarkv1alpha1.TaskWorkloadSpec{
					Kind: "rlinf.io/ray", Version: "v1", Parameters: apiextensionsv1.JSON{Raw: parameters},
					// A replicated carrier states its parameters; without them the cluster's agent cannot decode the Task.
					Carrier:  rlarkv1alpha1.TaskWorkloadCarrier{Type: "rlinf.io/kubernetes-deployment", Version: "v1", Parameters: apiextensionsv1.JSON{Raw: []byte(`{"replicas":1}`)}},
					Template: &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{container}}},
				},
			}}},
		},
	}
	tenant.SetIdentity(job, tenant.DefaultID, p.Spec.RequestedBy)
	if _, err := jobs.Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("submit the check Job: %w", err)
	}
	defer func() { _ = jobs.Delete(context.WithoutCancel(ctx), name, metav1.DeleteOptions{}) }()
	output := ""
	for deadline := time.Now().Add(10 * time.Minute); time.Now().Before(deadline); time.Sleep(5 * time.Second) {
		if output = r.g.jobOutput(ctx, name); strings.Contains(output, want) {
			return fmt.Sprintf("Job %s on %s printed %q; deleted", name, facts.NodeName, lastLine(output, want)), nil
		}
	}
	if output == "" {
		job, _ := jobs.Get(ctx, name, metav1.GetOptions{})
		if job != nil {
			output = "the Job is " + string(job.Status.Phase)
			for _, task := range job.Status.Tasks {
				output += "; " + task.Name + " " + string(task.Phase) + " " + task.Message
			}
		}
	}
	return "", fmt.Errorf("the check Job printed no %q in 10 min: %s", want, strings.TrimSpace(output))
}

// jobOutput is what the Pods of a Job have printed so far.
func (g *Gateway) jobOutput(ctx context.Context, job string) string {
	tasks, err := g.kubeClient.RlinfV1alpha1().Tasks(metav1.NamespaceAll).List(ctx, metav1.ListOptions{LabelSelector: "rlinf.io/job=" + job})
	if err != nil {
		return ""
	}
	var out strings.Builder
	for _, task := range tasks.Items {
		pods, err := g.kubeClient.RlinfV1alpha1().Pods(task.Namespace).List(ctx, metav1.ListOptions{LabelSelector: rlarkv1alpha1.PodLabelTaskName + "=" + task.Name})
		if err != nil {
			continue
		}
		for _, pod := range pods.Items {
			logs, err := g.fetchPodLogs(ctx, provisionAgentID(task.Namespace), pod.Spec.PodNamespace, pod.Spec.PodName)
			if err == nil {
				out.WriteString(logs)
			}
		}
	}
	return out.String()
}
