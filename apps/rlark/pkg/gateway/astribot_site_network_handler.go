package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strings"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/rlinf/rlark/apps/rlark/pkg/log"
)

type siteDiscoveryFrom struct {
	ClusterID string `json:"clusterID"`
	NodeName  string `json:"nodeName"`
}

type siteDiscovery struct {
	Enabled             bool              `json:"enabled"`
	Ranges              []string          `json:"ranges"`
	IntervalSeconds     int64             `json:"intervalSeconds"`
	From                siteDiscoveryFrom `json:"from"`
	IncludeUnclassified bool              `json:"includeUnclassified,omitempty"`
	SSHPorts            []int64           `json:"sshPorts,omitempty"`
}

type siteNetworkSite struct {
	Name      string         `json:"name"`
	Clusters  []string       `json:"clusters"`
	Discovery *siteDiscovery `json:"discovery"`
}

type siteNetworkView struct {
	ResourceVersion string            `json:"resourceVersion"`
	Generation      int64             `json:"generation"`
	Sites           []siteNetworkSite `json:"sites"`
	Members         []map[string]any  `json:"members"`
	NodeClasses     []map[string]any  `json:"nodeClasses"`
	Status          map[string]any    `json:"status"`
}

func siteNetworkDiscovery(network *unstructured.Unstructured, site string) *siteDiscovery {
	sites, _, _ := unstructured.NestedSlice(network.Object, "spec", "sites")
	for _, raw := range sites {
		entry, _ := raw.(map[string]any)
		if entry["name"] != site {
			continue
		}
		discovery, ok := entry["discovery"].(map[string]any)
		if !ok || discovery == nil {
			return nil
		}
		view := &siteDiscovery{}
		view.Enabled, _, _ = unstructured.NestedBool(discovery, "enabled")
		view.Ranges, _, _ = unstructured.NestedStringSlice(discovery, "ranges")
		view.IntervalSeconds, _, _ = unstructured.NestedInt64(discovery, "intervalSeconds")
		view.IncludeUnclassified, _, _ = unstructured.NestedBool(discovery, "includeUnclassified")
		ports, _, _ := unstructured.NestedSlice(discovery, "sshPorts")
		for _, rawPort := range ports {
			if port, ok := rawPort.(int64); ok {
				view.SSHPorts = append(view.SSHPorts, port)
			}
			if port, ok := rawPort.(int); ok {
				view.SSHPorts = append(view.SSHPorts, int64(port))
			}
		}
		view.From.ClusterID, _, _ = unstructured.NestedString(discovery, "from", "clusterID")
		view.From.NodeName, _, _ = unstructured.NestedString(discovery, "from", "nodeName")
		return view
	}
	return nil
}

func siteNetworkViewOf(network *unstructured.Unstructured, classes *unstructured.UnstructuredList) siteNetworkView {
	view := siteNetworkView{
		ResourceVersion: network.GetResourceVersion(),
		Generation:      network.GetGeneration(),
		Sites:           []siteNetworkSite{},
		Members:         []map[string]any{},
		NodeClasses:     []map[string]any{},
		Status:          map[string]any{},
	}
	sites, _, _ := unstructured.NestedSlice(network.Object, "spec", "sites")
	for _, raw := range sites {
		entry, _ := raw.(map[string]any)
		name, _ := entry["name"].(string)
		clusters, _, _ := unstructured.NestedStringSlice(entry, "clusters")
		view.Sites = append(view.Sites, siteNetworkSite{Name: name, Clusters: clusters, Discovery: siteNetworkDiscovery(network, name)})
	}
	clusterSites := map[string]string{}
	for _, site := range view.Sites {
		for _, cluster := range site.Clusters {
			clusterSites[cluster] = site.Name
		}
	}
	nodes, _, _ := unstructured.NestedSlice(network.Object, "spec", "nodes")
	for _, raw := range nodes {
		entry, _ := raw.(map[string]any)
		member := map[string]any{}
		for _, field := range []string{"clusterID", "nodeName", "role", "address", "site"} {
			if value, ok := entry[field]; ok {
				member[field] = value
			}
		}
		if member["site"] == nil {
			member["site"] = clusterSites[fmt.Sprint(entry["clusterID"])]
		}
		view.Members = append(view.Members, member)
	}
	for i := range classes.Items {
		class := &classes.Items[i]
		access, hasAccess, _ := unstructured.NestedMap(class.Object, "spec", "access")
		discovery, hasDiscovery, _ := unstructured.NestedMap(class.Object, "spec", "discovery")
		if !hasAccess || !hasDiscovery {
			continue
		}
		item := map[string]any{"name": class.GetName(), "hasAccess": true, "hasDiscovery": true}
		if port, found, _ := unstructured.NestedInt64(access, "entry", "port"); found {
			item["entryPort"] = port
		}
		fingerprints, _, _ := unstructured.NestedStringSlice(discovery, "hostKeyFingerprints")
		if fingerprints == nil {
			fingerprints = []string{}
		}
		item["hostKeyFingerprints"] = fingerprints
		view.NodeClasses = append(view.NodeClasses, item)
	}
	if status, found, _ := unstructured.NestedMap(network.Object, "status"); found {
		encoded, _ := json.Marshal(status)
		_ = json.Unmarshal(encoded, &view.Status)
		if view.Status == nil {
			view.Status = map[string]any{}
		}
		if declared, ok := view.Status["declared"].([]any); ok {
			for _, raw := range declared {
				entry, _ := raw.(map[string]any)
				delete(entry, "config")
			}
		}
	}
	// A SiteNetwork without status still answers with the arrays the console
	// expects, so a fresh site network does not read as a broken response.
	if _, ok := view.Status["conditions"]; !ok {
		view.Status["conditions"] = []any{}
	}
	if _, ok := view.Status["unreachable"]; !ok {
		view.Status["unreachable"] = []any{}
	}
	return view
}

func (g *Gateway) handleGetSiteNetwork(c *gin.Context) {
	network, err := g.siteNetwork(c.Request.Context())
	if err != nil {
		respondKubeError(c, "SiteNetwork request failed", err)
		return
	}
	if network.GetName() == "" {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "SiteNetwork not found")
		return
	}
	classes, err := g.stationClient.Resource(nodeClassGVR).List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		respondKubeError(c, "NodeClass request failed", err)
		return
	}
	c.JSON(http.StatusOK, siteNetworkViewOf(network, classes))
}

type updateSiteDiscoveryRequest struct {
	ResourceVersion string          `json:"resourceVersion"`
	Discovery       json.RawMessage `json:"discovery"`
}

func validateSiteDiscovery(discovery *siteDiscovery) error {
	if discovery == nil {
		return nil
	}
	if discovery.Ranges == nil || len(discovery.Ranges) < 1 || len(discovery.Ranges) > 8 {
		return fmt.Errorf("ranges must be 1 to 8 IPv4 prefixes of /22 or shorter")
	}
	seen := make(map[string]struct{}, len(discovery.Ranges))
	for i, raw := range discovery.Ranges {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil || !prefix.Addr().Is4() || prefix.Bits() < 22 {
			return fmt.Errorf("ranges must be 1 to 8 IPv4 prefixes of /22 or shorter")
		}
		canonical := prefix.Masked().String()
		if _, exists := seen[canonical]; exists {
			return fmt.Errorf("ranges must be 1 to 8 IPv4 prefixes of /22 or shorter")
		}
		seen[canonical] = struct{}{}
		discovery.Ranges[i] = canonical
	}
	if discovery.IntervalSeconds < 10 || discovery.IntervalSeconds > 3600 {
		return fmt.Errorf("intervalSeconds must be between 10 and 3600")
	}
	if len(discovery.From.ClusterID) == 0 || len(discovery.From.ClusterID) > 128 ||
		len(discovery.From.NodeName) == 0 || len(discovery.From.NodeName) > 253 {
		return fmt.Errorf("from must name a Hub of the network")
	}
	if len(discovery.SSHPorts) > 4 {
		return fmt.Errorf("sshPorts must have at most 4 ports")
	}
	seenPorts := map[int64]bool{}
	for _, port := range discovery.SSHPorts {
		if port < 1 || port > 65535 || seenPorts[port] {
			return fmt.Errorf("sshPorts must contain unique ports between 1 and 65535")
		}
		seenPorts[port] = true
	}
	return nil
}

func (g *Gateway) handleUpdateSiteDiscovery(c *gin.Context) {
	var request updateSiteDiscoveryRequest
	if !decodeOneJSON(c, maxNodeAdmissionRequestBytes, &request, false) {
		return
	}
	if request.ResourceVersion == "" {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "resourceVersion is required")
		return
	}
	if len(request.Discovery) == 0 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "discovery is required")
		return
	}
	site := c.Param("site")
	if len(validation.IsDNS1123Label(site)) != 0 || len(site) > 63 {
		respondError(c, http.StatusBadRequest, CodeInvalidRequest, "site must be a DNS label of at most 63 characters")
		return
	}
	var discovery *siteDiscovery
	if len(request.Discovery) != 0 && strings.TrimSpace(string(request.Discovery)) != "null" {
		var value struct {
			Enabled             *bool              `json:"enabled"`
			Ranges              []string           `json:"ranges"`
			IntervalSeconds     *int64             `json:"intervalSeconds"`
			From                *siteDiscoveryFrom `json:"from"`
			IncludeUnclassified bool               `json:"includeUnclassified,omitempty"`
			SSHPorts            []int64            `json:"sshPorts,omitempty"`
		}
		decoder := json.NewDecoder(strings.NewReader(string(request.Discovery)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequestBody, "the request must be one JSON object of the documented fields")
			return
		}
		if value.Enabled == nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "discovery.enabled is required")
			return
		}
		interval := int64(30)
		if value.IntervalSeconds != nil {
			interval = *value.IntervalSeconds
		}
		from := siteDiscoveryFrom{}
		if value.From != nil {
			from = *value.From
		}
		if value.From == nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, "from must name a Hub of the network")
			return
		}
		discovery = &siteDiscovery{Enabled: *value.Enabled, Ranges: value.Ranges, IntervalSeconds: interval, From: from, IncludeUnclassified: value.IncludeUnclassified, SSHPorts: value.SSHPorts}
		if err := validateSiteDiscovery(discovery); err != nil {
			respondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
			return
		}
	}

	ctx := c.Request.Context()
	network, err := g.siteNetwork(ctx)
	if err != nil {
		respondKubeError(c, "SiteNetwork request failed", err)
		return
	}
	if network.GetName() == "" {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "SiteNetwork not found")
		return
	}
	if network.GetResourceVersion() != request.ResourceVersion {
		respondError(c, http.StatusConflict, CodeResourceConflict, "the SiteNetwork changed meanwhile; reload and try again")
		return
	}
	sites, _, _ := unstructured.NestedSlice(network.Object, "spec", "sites")
	found := false
	var previous *siteDiscovery
	for i, raw := range sites {
		entry, _ := raw.(map[string]any)
		if entry["name"] != site {
			continue
		}
		found = true
		previous = siteNetworkDiscovery(network, site)
		if discovery == nil {
			delete(entry, "discovery")
		} else {
			value := map[string]any{
				"enabled": discovery.Enabled, "ranges": stringInterfaces(discovery.Ranges),
				"intervalSeconds":     discovery.IntervalSeconds,
				"from":                map[string]any{"clusterID": discovery.From.ClusterID, "nodeName": discovery.From.NodeName},
				"includeUnclassified": discovery.IncludeUnclassified,
			}
			if len(discovery.SSHPorts) > 0 {
				ports := make([]interface{}, len(discovery.SSHPorts))
				for i, port := range discovery.SSHPorts {
					ports[i] = port
				}
				value["sshPorts"] = ports
			}
			if err := validateSiteDiscoveryMember(network, discovery.From); err != nil {
				respondError(c, http.StatusBadRequest, CodeInvalidRequest, err.Error())
				return
			}
			if _, err := g.stationClient.Resource(stationClusterGVR).Get(ctx, discovery.From.ClusterID, metav1.GetOptions{}); err != nil {
				if apierrors.IsNotFound(err) {
					respondError(c, http.StatusBadRequest, CodeInvalidRequest, "from must name a Hub of the network")
				} else {
					respondKubeError(c, "Cluster request failed", err)
				}
				return
			}
			entry["discovery"] = value
		}
		sites[i] = entry
		break
	}
	if !found {
		respondError(c, http.StatusNotFound, CodeResourceNotFound, "site not found")
		return
	}
	if err := unstructured.SetNestedSlice(network.Object, sites, "spec", "sites"); err != nil {
		respondError(c, http.StatusInternalServerError, CodeInternalError, "failed to update SiteNetwork")
		return
	}
	classes, err := g.stationClient.Resource(nodeClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		respondKubeError(c, "NodeClass request failed", err)
		return
	}
	updated, err := g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Update(ctx, network, metav1.UpdateOptions{})
	if err != nil {
		if apierrors.IsConflict(err) {
			respondError(c, http.StatusConflict, CodeResourceConflict, "the SiteNetwork changed meanwhile; reload and try again")
			return
		}
		respondKubeError(c, "SiteNetwork update failed", err)
		return
	}
	log.FromContext(ctx).WithName("audit").Info("site network discovery", "action", "update", "actor", c.GetString(authSubjectKey),
		"site", site, "beforeEnabled", discoveryEnabled(previous), "beforeRanges", discoveryRanges(previous),
		"beforeIntervalSeconds", discoveryInterval(previous), "beforeFrom", discoveryFrom(previous),
		"enabled", discovery != nil && discovery.Enabled, "ranges", discoveryRanges(discovery),
		"intervalSeconds", discoveryInterval(discovery), "from", discoveryFrom(discovery),
		"resourceVersion", request.ResourceVersion, "newResourceVersion", updated.GetResourceVersion())
	c.JSON(http.StatusOK, siteNetworkViewOf(updated, classes))
}

func validateSiteDiscoveryMember(network *unstructured.Unstructured, from siteDiscoveryFrom) error {
	nodes, _, _ := unstructured.NestedSlice(network.Object, "spec", "nodes")
	for _, raw := range nodes {
		entry, _ := raw.(map[string]any)
		if entry["clusterID"] == from.ClusterID && entry["nodeName"] == from.NodeName && entry["role"] == "Hub" {
			return nil
		}
	}
	return fmt.Errorf("from must name a Hub of the network")
}

func stringInterfaces(values []string) []any {
	result := make([]any, len(values))
	for i := range values {
		result[i] = values[i]
	}
	return result
}

func discoveryRanges(discovery *siteDiscovery) []string {
	if discovery == nil {
		return nil
	}
	return discovery.Ranges
}

func discoveryEnabled(discovery *siteDiscovery) bool {
	return discovery != nil && discovery.Enabled
}

func discoveryInterval(discovery *siteDiscovery) int64 {
	if discovery == nil {
		return 0
	}
	return discovery.IntervalSeconds
}

func discoveryFrom(discovery *siteDiscovery) string {
	if discovery == nil {
		return ""
	}
	return discovery.From.ClusterID + "/" + discovery.From.NodeName
}
