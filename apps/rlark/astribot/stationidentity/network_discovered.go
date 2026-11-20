package stationidentity

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var machineGVR = schema.GroupVersionResource{Group: stationGroup, Version: stationVersion, Resource: "machines"}
var siteNetworkGVR = schema.GroupVersionResource{Group: stationGroup, Version: stationVersion, Resource: "sitenetworks"}

// discoveryRelayPort is DiscoveryRelayPort of astribot/station/network:
// where a hub relays connections to the hosts it found.
const discoveryRelayPort = 18022

// discoveryReportedAnnotation is DiscoveryReportedAnnotation of
// astribot/station/controller: on a hub's NetworkEnrollment, when the hub
// last reported what it found.
const discoveryReportedAnnotation = "station.rlark.io/discovery-reported-at"

// entryHostKeyAnnotation is EntryHostKeyAnnotation of
// astribot/station/controller: on a NodeAdmission, the host key its machine's
// entry device was given when it joined.
const entryHostKeyAnnotation = "rlark.io/entry-host-key"

var (
	hardwareAddressPattern = regexp.MustCompile(`^[0-9a-f]{2}(:[0-9a-f]{2}){5}$`)
	hostKeyPattern         = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)
)

type discoveredRequest struct {
	documentRequest
	Hosts []discoveredHost `json:"hosts"`
}

// discoveredHost is a host a hub saw show a host key it looks for: the
// factory's of a machine that has not joined, or the one a machine's entry
// device was given when it joined (astribot/station-gateway).
type discoveredHost struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
	MAC     string `json:"mac"`
	HostKey string `json:"host_key"`
}

// ServeDiscovered takes what a hub of a site that declares discovery found on
// the site's network, for the hub's own identity, and keeps a Machine for
// every host whose key a NodeClass declares as its factory's. A host that
// shows the key its Machine is known by is that machine, wherever its lease
// has put it. It writes what was seen and where; what the machine is,
// station-controller reads.
func (d *NetworkDocuments) ServeDiscovered(w http.ResponseWriter, r *http.Request) {
	var request discoveredRequest
	if !decodeBody(w, r, &request) || !d.proven(w, request.documentRequest) {
		return
	}
	if len(request.Hosts) > 256 {
		writeError(w, http.StatusBadRequest, "at most 256 hosts")
		return
	}
	status, err := d.discovered(r.Context(), request.PublicKey, request.Hosts)
	if err != nil {
		log.Printf("discovered machines: %v", err)
		writeError(w, http.StatusServiceUnavailable, "machine storage unavailable")
		return
	}
	if status != http.StatusOK {
		writeError(w, status, "this node does not look for machines")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"hosts": len(request.Hosts)})
}

func (d *NetworkDocuments) discovered(ctx context.Context, key string, hosts []discoveredHost) (int, error) {
	enrolled, err := d.Enrolled.Lookup(ctx, key)
	if err != nil {
		return 0, err
	}
	hub := enrolled[key]
	if hub == nil {
		return http.StatusForbidden, nil
	}
	text := func(object *unstructured.Unstructured, fields ...string) string {
		value, _, _ := unstructured.NestedString(object.Object, fields...)
		return value
	}
	site, tunnel := text(hub, "status", "site"), text(hub, "status", "tunnelAddress")
	network, err := d.Client.Resource(siteNetworkGVR).Namespace("station-system").Get(ctx, "default", metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("read the SiteNetwork: %w", err)
	}
	declared := false
	var interval time.Duration
	sites, _, _ := unstructured.NestedSlice(network.Object, "spec", "sites")
	for _, entry := range sites {
		entry, _ := entry.(map[string]interface{})
		enabled, _, _ := unstructured.NestedBool(entry, "discovery", "enabled")
		if entry["name"] == site && enabled {
			declared = true
			seconds, _, _ := unstructured.NestedInt64(entry, "discovery", "intervalSeconds")
			interval = time.Duration(seconds) * time.Second
		}
	}
	if !declared || text(hub, "status", "role") != "Hub" || tunnel == "" {
		return http.StatusForbidden, nil
	}
	now := clock(d.Now)
	// When the hub last reported, on its enrollment: station-controller says
	// on the SiteNetwork when a hub has gone silent (DiscoveryStale).
	enrollments := d.Client.Resource(networkEnrollmentGVR).Namespace(hub.GetNamespace())
	current, err := enrollments.Get(ctx, hub.GetName(), metav1.GetOptions{})
	if err != nil {
		return 0, fmt.Errorf("read the hub's NetworkEnrollment: %w", err)
	}
	annotations := current.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[discoveryReportedAnnotation] = now.UTC().Format(time.RFC3339)
	current.SetAnnotations(annotations)
	if _, err := enrollments.Update(ctx, current, metav1.UpdateOptions{}); err != nil && !apierrors.IsConflict(err) {
		return 0, fmt.Errorf("record the hub's report: %w", err)
	}
	classes, err := d.Client.Resource(nodeClassGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("list NodeClasses: %w", err)
	}
	classOf := map[string]string{}
	for i := range classes.Items {
		keys, _, _ := unstructured.NestedStringSlice(classes.Items[i].Object, "spec", "discovery", "hostKeyFingerprints")
		for _, fingerprint := range keys {
			classOf[fingerprint] = classes.Items[i].GetName()
		}
	}
	// The key an entry device was given when its machine joined, by the
	// machine's cluster and node: after that the machine is known by it, not by
	// a key a report says it shows (station-controller looks at it).
	admissions, err := d.Client.Resource(nodeAdmissionGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("list NodeAdmissions: %w", err)
	}
	ownKey := map[[2]string]string{}
	for i := range admissions.Items {
		clusterID := text(&admissions.Items[i], "spec", "clusterID")
		ownKey[[2]string{clusterID, admissions.Items[i].GetName()}] = admissions.Items[i].GetAnnotations()[entryHostKeyAnnotation]
	}
	machines := d.Client.Resource(machineGVR).Namespace("station-system")
	for _, host := range hosts {
		class := classOf[host.HostKey]
		if ip := net.ParseIP(host.Address); ip == nil || ip.To4() == nil || !hardwareAddressPattern.MatchString(host.MAC) || !hostKeyPattern.MatchString(host.HostKey) {
			continue
		}
		name := site + "-" + strings.ReplaceAll(host.MAC, ":", "")
		machine, err := machines.Get(ctx, name, metav1.GetOptions{})
		// A key that is no class's factory key says which machine it is only
		// where that machine is known by it.
		if class == "" && (apierrors.IsNotFound(err) || err == nil && text(machine, "status", "entry", "hostKeyFingerprint") != host.HostKey) {
			continue
		}
		via := net.JoinHostPort(tunnel, strconv.Itoa(discoveryRelayPort))
		// Hubs on one network all see its machines. A machine stays with the
		// hub that has it until that hub has not seen it for three intervals,
		// when station-controller would call a silent hub stale; it is not
		// handed between them and looked at anew on every report.
		if err == nil && text(machine, "status", "entry", "via") != via {
			if seen, err := time.Parse(time.RFC3339, text(machine, "status", "entry", "lastSeen")); err == nil && now.Sub(seen) <= 3*interval {
				continue
			}
		}
		if apierrors.IsNotFound(err) {
			machine, err = machines.Create(ctx, &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": stationGroup + "/" + stationVersion, "kind": "Machine",
				"metadata": map[string]interface{}{"name": name, "namespace": "station-system"},
				"spec":     map[string]interface{}{"site": site, "nodeClass": class, "source": "Discovery"},
			}}, metav1.CreateOptions{})
		}
		if err != nil {
			return 0, fmt.Errorf("keep Machine %s: %w", name, err)
		}
		hostKey := host.HostKey
		// A machine that shows its class's factory key is written with it:
		// station-controller looks at it by that key, and a machine set up anew
		// is joined again.
		if key := ownKey[[2]string{text(machine, "status", "clusterID"), text(machine, "status", "nodeName")}]; key != "" && class == "" {
			hostKey = key
		}
		entry := map[string]interface{}{"address": host.Address, "mac": host.MAC, "hostKeyFingerprint": hostKey,
			"via": via, "lastSeen": now.UTC().Format(time.RFC3339)}
		// A machine that shows up changed is looked at anew.
		for _, field := range []string{"address", "hostKeyFingerprint", "via"} {
			if text(machine, "status", "entry", field) != entry[field] {
				unstructured.RemoveNestedField(machine.Object, "status", "inspectedAt")
			}
		}
		_ = unstructured.SetNestedMap(machine.Object, entry, "status", "entry")
		if _, err := machines.UpdateStatus(ctx, machine, metav1.UpdateOptions{}); err != nil && !apierrors.IsConflict(err) {
			return 0, fmt.Errorf("write what was seen of Machine %s: %w", name, err)
		}
	}
	return http.StatusOK, nil
}
