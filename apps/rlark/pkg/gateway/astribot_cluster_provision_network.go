package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/yaml"
)

// originName is the global name of the registry every site's registry is a copy of: a new site pulls from it
// through the tunnel until it has a cache of its own (site-registry, role=cache).
const originName = "origin.rlark.internal"

// siteNetworkNode is what a cluster's provision reads of a node the SiteNetwork declares.
type siteNetworkNode struct {
	ClusterID string   `json:"clusterID"`
	NodeName  string   `json:"nodeName"`
	Address   string   `json:"address"`
	Advertise []string `json:"-"`
}

// provisionNetwork is the one SiteNetwork, read for a cluster's provision. It is changed as the object it is, so
// that whatever else it holds stays as it is.
type provisionNetwork struct{ object *unstructured.Unstructured }

func (g *Gateway) provisionNetwork(ctx context.Context) (provisionNetwork, error) {
	object, err := g.siteNetwork(ctx)
	if err != nil {
		return provisionNetwork{}, fmt.Errorf("read the SiteNetwork: %w", err)
	}
	if object.GetName() == "" {
		return provisionNetwork{}, errors.New("there is no SiteNetwork (station-system/default): a new site is declared in it")
	}
	return provisionNetwork{object}, nil
}

func (n provisionNetwork) list(path ...string) []map[string]interface{} {
	raw, _, _ := unstructured.NestedSlice(n.object.Object, path...)
	out := make([]map[string]interface{}, 0, len(raw))
	for _, item := range raw {
		if entry, ok := item.(map[string]interface{}); ok {
			out = append(out, entry)
		}
	}
	return out
}

func (n provisionNetwork) nodes() []siteNetworkNode {
	var out []siteNetworkNode
	for _, entry := range n.list("spec", "nodes") {
		node := siteNetworkNode{}
		node.ClusterID, _ = entry["clusterID"].(string)
		node.NodeName, _ = entry["nodeName"].(string)
		node.Address, _ = entry["address"].(string)
		advertise, _, _ := unstructured.NestedSlice(entry, "advertise")
		for _, raw := range advertise {
			if a, ok := raw.(map[string]interface{}); ok {
				if cidr, _ := a["cidr"].(string); cidr != "" {
					node.Advertise = append(node.Advertise, cidr)
				}
			}
		}
		out = append(out, node)
	}
	return out
}

// name is the address the global names give name, if any.
func (n provisionNetwork) name(name string) string {
	for _, entry := range n.list("spec", "names") {
		if entry["name"] == name {
			address, _ := entry["address"].(string)
			return address
		}
	}
	return ""
}

// siteOf is the site that has cluster, if any.
func (n provisionNetwork) siteOf(cluster string) string {
	for _, entry := range n.list("spec", "sites") {
		clusters, _, _ := unstructured.NestedStringSlice(entry, "clusters")
		if slices.Contains(clusters, cluster) {
			name, _ := entry["name"].(string)
			return name
		}
	}
	return ""
}

func (n provisionNetwork) hasSite(site string) bool {
	return slices.ContainsFunc(n.list("spec", "sites"), func(entry map[string]interface{}) bool { return entry["name"] == site })
}

// declared is the first tunnel document station-controller published for a node declared with its key, until
// the node is enrolled.
func (n provisionNetwork) declared(cluster, node string) string {
	for _, entry := range n.list("status", "declared") {
		if entry["clusterID"] == cluster && entry["nodeName"] == node {
			config, _ := entry["config"].(string)
			return config
		}
	}
	return ""
}

// allocate is the lowest free tunnel address of the pool that neither the SiteNetwork nor others (the node
// admissions', other makings') hold. The pool's first address is the management machine's.
func (n provisionNetwork) allocate(others []string) (string, error) {
	pool, _, _ := unstructured.NestedString(n.object.Object, "spec", "pool")
	prefix, err := netip.ParsePrefix(pool)
	if err != nil {
		return "", fmt.Errorf("the SiteNetwork's pool %q: %w", pool, err)
	}
	taken := map[string]bool{}
	for _, node := range n.nodes() {
		taken[node.Address] = true
	}
	for _, entry := range n.list("spec", "names") {
		address, _ := entry["address"].(string)
		taken[address] = true
	}
	for _, address := range others {
		taken[address] = true
	}
	prefix = prefix.Masked()
	address := prefix.Addr().Next().Next()
	for ; prefix.Contains(address) && prefix.Contains(address.Next()); address = address.Next() {
		if !taken[address.String()] {
			return address.String(), nil
		}
	}
	return "", fmt.Errorf("the SiteNetwork's pool %s has no free address", pool)
}

// admissionAddresses are the tunnel addresses the node admissions hold (pinned, or allocated by station-controller):
// a machine that joined by a code has its address there, not in the SiteNetwork.
func (g *Gateway) admissionAddresses(ctx context.Context) ([]string, error) {
	admissions, err := g.stationClient.Resource(nodeAdmissionGVR).Namespace("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("list node admissions: %w", err)
	}
	var out []string
	for _, admission := range admissions.Items {
		for _, path := range [][]string{{"spec", "address"}, {"status", "address"}} {
			if address, _, _ := unstructured.NestedString(admission.Object, path...); address != "" {
				out = append(out, address)
			}
		}
	}
	return out, nil
}

// chooseSubnets is the first pair of /16 Pod and Service subnets, from 10.200.0.0 on, that overlaps none of taken.
func chooseSubnets(taken []netip.Prefix) (string, string, error) {
	for second := 200; second+1 < 256; second += 2 {
		pod := netip.PrefixFrom(netip.AddrFrom4([4]byte{10, byte(second), 0, 0}), 16)
		service := netip.PrefixFrom(netip.AddrFrom4([4]byte{10, byte(second + 1), 0, 0}), 16)
		if !slices.ContainsFunc(taken, pod.Overlaps) && !slices.ContainsFunc(taken, service.Overlaps) {
			return pod.String(), service.String(), nil
		}
	}
	return "", "", errors.New("no pair of /16 in 10.200.0.0–10.255.255.255 is free of every cluster's and the machine's networks")
}

// clusterNetworks are the Pod and Service networks of a cluster, as its API shows them: kubeadm's configuration
// where there is one, each node's Pod CIDR, and the /16 around the API's own Service address.
func (g *Gateway) clusterNetworks(ctx context.Context, cluster string) ([]netip.Prefix, error) {
	agent := provisionAgentID(cluster)
	get := func(path string, into any) (bool, error) {
		resp, err := g.proxyKubeRequest(ctx, http.MethodGet, agent, path, nil)
		if err != nil {
			return false, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return false, nil
		}
		if resp.StatusCode != http.StatusOK {
			return false, fmt.Errorf("%s answered %d", path, resp.StatusCode)
		}
		return true, json.NewDecoder(resp.Body).Decode(into)
	}
	var out []netip.Prefix
	add := func(cidr string) {
		if prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr)); err == nil && prefix.Addr().Is4() {
			out = append(out, prefix.Masked())
		}
	}
	var kubeadm struct {
		Data map[string]string `json:"data"`
	}
	if found, err := get("api/v1/namespaces/kube-system/configmaps/kubeadm-config", &kubeadm); err != nil {
		return nil, err
	} else if found {
		var config struct {
			Networking struct {
				PodSubnet     string `json:"podSubnet"`
				ServiceSubnet string `json:"serviceSubnet"`
			} `json:"networking"`
		}
		if err := yaml.Unmarshal([]byte(kubeadm.Data["ClusterConfiguration"]), &config); err == nil {
			for _, cidr := range strings.Split(config.Networking.PodSubnet+","+config.Networking.ServiceSubnet, ",") {
				add(cidr)
			}
		}
	}
	var nodes struct {
		Items []struct {
			Spec struct {
				PodCIDRs []string `json:"podCIDRs"`
			} `json:"spec"`
		} `json:"items"`
	}
	if _, err := get("api/v1/nodes", &nodes); err != nil {
		return nil, err
	}
	for _, node := range nodes.Items {
		for _, cidr := range node.Spec.PodCIDRs {
			add(cidr)
		}
	}
	var service struct {
		Spec struct {
			ClusterIP string `json:"clusterIP"`
		} `json:"spec"`
	}
	if _, err := get("api/v1/namespaces/default/services/kubernetes", &service); err != nil {
		return nil, err
	}
	if address, err := netip.ParseAddr(service.Spec.ClusterIP); err == nil && address.Is4() {
		out = append(out, netip.PrefixFrom(address, 16).Masked())
	}
	return out, nil
}

func (r *provisionRun) declare(ctx context.Context, p *rlarkv1alpha1.ClusterProvision) (string, error) {
	facts := &p.Status.Facts
	release, err := r.g.currentRelease(ctx)
	if err != nil {
		return "", err
	}
	facts.Release = release.Name
	registryHost, _, err := net.SplitHostPort(release.Registry)
	if err != nil {
		return "", fmt.Errorf("the release's registry %q is not host:port", release.Registry)
	}
	server, err := url.Parse(r.g.config.ServerAddress)
	if err != nil || server.Hostname() == "" {
		return "", fmt.Errorf("the Gateway's --server-address %q names no host", r.g.config.ServerAddress)
	}
	others, err := r.g.kubeClient.RlinfV1alpha1().ClusterProvisions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list cluster provisions: %w", err)
	}
	otherAddresses, err := r.g.admissionAddresses(ctx)
	if err != nil {
		return "", err
	}
	var taken []netip.Prefix
	for _, other := range others.Items {
		if other.Name == p.Name {
			continue
		}
		otherAddresses = append(otherAddresses, other.Status.Facts.TunnelAddress)
		for _, cidr := range []string{other.Status.Facts.PodSubnet, other.Status.Facts.ServiceSubnet} {
			if prefix, err := netip.ParsePrefix(cidr); err == nil {
				taken = append(taken, prefix)
			}
		}
	}
	var unread []string
	if facts.PodSubnet == "" {
		clusters, err := r.g.kubeClient.RlinfV1alpha1().Clusters().List(ctx, metav1.ListOptions{})
		if err != nil {
			return "", fmt.Errorf("list clusters: %w", err)
		}
		for _, cluster := range clusters.Items {
			if cluster.Name == p.Name {
				continue
			}
			if cluster.Status.Phase != rlarkv1alpha1.ClusterOnline && cluster.Status.Phase != rlarkv1alpha1.ClusterDowngraded {
				unread = append(unread, cluster.Name)
				continue
			}
			networks, err := r.g.clusterNetworks(ctx, cluster.Name)
			if err != nil {
				return "", fmt.Errorf("read the networks of %s: %w", cluster.Name, err)
			}
			taken = append(taken, networks...)
		}
	}

	var readback string
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		network, err := r.g.provisionNetwork(ctx)
		if err != nil {
			return err
		}
		if site := network.siteOf(p.Name); site != "" && site != p.Spec.Site {
			return fmt.Errorf("the SiteNetwork already has %s in site %s", p.Name, site)
		}
		if network.siteOf(p.Name) == "" && network.hasSite(p.Spec.Site) {
			return fmt.Errorf("the SiteNetwork already has a site %s", p.Spec.Site)
		}
		origin := network.name(originName)
		if origin == "" {
			return fmt.Errorf("the SiteNetwork's names have no %s: a new site pulls its images from it", originName)
		}
		if network.name(server.Hostname()) == "" {
			return fmt.Errorf("the SiteNetwork's names have no %s: a new cluster's agent reaches RLark by it", server.Hostname())
		}
		if facts.TunnelAddress == "" {
			for _, node := range network.nodes() {
				if node.ClusterID == p.Name && node.NodeName == facts.NodeName {
					facts.TunnelAddress = node.Address
				}
			}
		}
		if facts.TunnelAddress == "" {
			if facts.TunnelAddress, err = network.allocate(otherAddresses); err != nil {
				return err
			}
		}
		if facts.PodSubnet == "" {
			networkTaken := slices.Clone(taken)
			if pool, _, _ := unstructured.NestedString(network.object.Object, "spec", "pool"); pool != "" {
				if prefix, err := netip.ParsePrefix(pool); err == nil {
					networkTaken = append(networkTaken, prefix)
				}
			}
			for _, node := range network.nodes() {
				for _, cidr := range node.Advertise {
					if prefix, err := netip.ParsePrefix(cidr); err == nil {
						networkTaken = append(networkTaken, prefix)
					}
				}
			}
			for _, route := range facts.Routes {
				if prefix, err := netip.ParsePrefix(route); err == nil {
					networkTaken = append(networkTaken, prefix)
				}
			}
			if facts.PodSubnet, facts.ServiceSubnet, err = chooseSubnets(networkTaken); err != nil {
				return err
			}
		}
		facts.ControlPlaneName = "cp." + p.Name + ".rlark.internal"
		if address := network.name(facts.ControlPlaneName); address != "" && address != facts.TunnelAddress {
			return fmt.Errorf("the SiteNetwork's names give %s the address %s, not this cluster's control plane's", facts.ControlPlaneName, address)
		}
		object := network.object
		names, _, _ := unstructured.NestedSlice(object.Object, "spec", "names")
		if network.name(facts.ControlPlaneName) == "" {
			names = append(names, map[string]interface{}{"name": facts.ControlPlaneName, "address": facts.TunnelAddress})
		}
		sites, _, _ := unstructured.NestedSlice(object.Object, "spec", "sites")
		if !network.hasSite(p.Spec.Site) {
			// Mesh: the site's later hubs reach its control plane, which is on its first hub's tunnel address.
			sites = append(sites, map[string]interface{}{
				"name": p.Spec.Site, "clusters": []interface{}{p.Name}, "mesh": true,
				"names": []interface{}{map[string]interface{}{"name": registryHost, "address": origin}},
			})
		}
		nodes, _, _ := unstructured.NestedSlice(object.Object, "spec", "nodes")
		nodes = withHub(nodes, hubEntry(p.Name, facts.NodeName, facts.TunnelAddress, facts.PrivateAddress, facts.PublicKey, p.Spec.Machine.TunnelEndpoint))
		for path, value := range map[string][]interface{}{"names": names, "sites": sites, "nodes": nodes} {
			if err := unstructured.SetNestedSlice(object.Object, value, "spec", path); err != nil {
				return err
			}
		}
		if _, err := r.g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Update(ctx, object, metav1.UpdateOptions{}); err != nil {
			return err
		}
		readback = fmt.Sprintf("site %s; %s → %s; node %s hub at %s, endpoint %s; %s → %s; Pod %s, Service %s",
			p.Spec.Site, facts.ControlPlaneName, facts.TunnelAddress, facts.NodeName, facts.TunnelAddress, p.Spec.Machine.TunnelEndpoint, registryHost, origin, facts.PodSubnet, facts.ServiceSubnet)
		return nil
	})
	if err != nil {
		return readback, err
	}
	if len(unread) > 0 {
		readback += "; not read (offline): " + strings.Join(unread, ", ")
	}
	if _, err := r.g.declaredDocument(ctx, p.Name, facts.NodeName); err != nil {
		return readback, err
	}
	return readback + "; status.declared holds its tunnel document", nil
}

// hubEntry is the SiteNetwork's declaration of a hub by its key: a machine that reaches Station API only through its
// tunnel. It listens on its tunnel endpoint's port; other sites dial the endpoint, and the machines of its own site
// its private address at that port (a cloud's machines behind one address do not reach each other at it).
func hubEntry(cluster, node, tunnelAddress, privateAddress, publicKey, endpoint string) map[string]interface{} {
	_, port, _ := net.SplitHostPort(endpoint)
	listen, _ := strconv.ParseInt(port, 10, 32)
	return map[string]interface{}{
		"clusterID": cluster, "nodeName": node, "role": "Hub", "address": tunnelAddress,
		"nodeAddress": privateAddress, "publicKey": publicKey, "listenPort": listen,
		"endpoints": []interface{}{
			map[string]interface{}{"address": endpoint, "scope": "Public"},
			map[string]interface{}{"address": net.JoinHostPort(privateAddress, port), "scope": "Site"},
		},
	}
}

// validTunnelEndpoint is an IPv4 ip:port with a port, as a SiteNetwork endpoint is.
func validTunnelEndpoint(endpoint string) bool {
	parsed, err := netip.ParseAddrPort(endpoint)
	return err == nil && parsed.Addr().Is4() && parsed.Port() != 0
}

// withHub is nodes with the hub's entry in place of an earlier one of the same node.
func withHub(nodes []interface{}, hub map[string]interface{}) []interface{} {
	i := slices.IndexFunc(nodes, func(raw interface{}) bool {
		entry, _ := raw.(map[string]interface{})
		return entry["clusterID"] == hub["clusterID"] && entry["nodeName"] == hub["nodeName"]
	})
	if i < 0 {
		return append(nodes, hub)
	}
	nodes[i] = hub
	return nodes
}

// declareHub declares a machine of a cluster's site as a hub by its key, at a tunnel address of its own (the one it
// has, or the lowest free), and answers that address and the SiteNetwork's generation that holds the declaration.
func (g *Gateway) declareHub(ctx context.Context, cluster, node, privateAddress, publicKey, endpoint string) (string, int64, error) {
	admitted, err := g.admissionAddresses(ctx)
	if err != nil {
		return "", 0, err
	}
	address := ""
	generation := int64(0)
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		network, err := g.provisionNetwork(ctx)
		if err != nil {
			return err
		}
		if network.siteOf(cluster) == "" {
			return fmt.Errorf("no site of the SiteNetwork has the cluster %s", cluster)
		}
		address = ""
		for _, entry := range network.nodes() {
			if entry.ClusterID == cluster && entry.NodeName == node {
				address = entry.Address
			}
		}
		if address == "" {
			if address, err = network.allocate(admitted); err != nil {
				return err
			}
		}
		nodes, _, _ := unstructured.NestedSlice(network.object.Object, "spec", "nodes")
		if err := unstructured.SetNestedSlice(network.object.Object, withHub(nodes, hubEntry(cluster, node, address, privateAddress, publicKey, endpoint)), "spec", "nodes"); err != nil {
			return err
		}
		written, err := g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Update(ctx, network.object, metav1.UpdateOptions{})
		if err == nil {
			generation = written.GetGeneration()
		}
		return err
	})
	return address, generation, err
}

// declaredDocument waits for the first tunnel document station-controller publishes for a node declared by its key.
func (g *Gateway) declaredDocument(ctx context.Context, cluster, node string) (string, error) {
	for deadline := time.Now().Add(2 * time.Minute); time.Now().Before(deadline); time.Sleep(3 * time.Second) {
		network, err := g.provisionNetwork(ctx)
		if err != nil {
			return "", err
		}
		if document := network.declared(cluster, node); document != "" {
			return document, nil
		}
	}
	return "", fmt.Errorf("station-controller has published no tunnel document for %s in 2 min (SiteNetwork status.declared)", node)
}
