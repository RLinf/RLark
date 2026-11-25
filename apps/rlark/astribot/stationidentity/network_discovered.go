package stationidentity

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"reflect"
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
	Address          string `json:"address"`
	Port             int    `json:"port"`
	MAC              string `json:"mac"`
	HostKey          string `json:"host_key"`
	HostKeyAlgorithm string `json:"host_key_algorithm,omitempty"`
	Banner           string `json:"ssh_banner,omitempty"`
	Unclassified     bool   `json:"unclassified,omitempty"`
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
		message := "this node does not look for machines"
		if status == http.StatusTooManyRequests {
			message = "the site has reached its unclassified Machine limit"
		}
		writeError(w, status, message)
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
	includeUnclassified := false
	var interval time.Duration
	sites, _, _ := unstructured.NestedSlice(network.Object, "spec", "sites")
	for _, entry := range sites {
		entry, _ := entry.(map[string]interface{})
		enabled, _, _ := unstructured.NestedBool(entry, "discovery", "enabled")
		if entry["name"] == site && enabled {
			declared = true
			includeUnclassified, _, _ = unstructured.NestedBool(entry, "discovery", "includeUnclassified")
			seconds, _, _ := unstructured.NestedInt64(entry, "discovery", "intervalSeconds")
			interval = time.Duration(seconds) * time.Second
		}
	}
	if !declared || text(hub, "status", "role") != "Hub" || tunnel == "" {
		return http.StatusForbidden, nil
	}
	// Exclude every enrolled member address we can authoritatively observe in
	// the Hub's management namespace. A discovery report is evidence, not a
	// request to turn an existing member into a candidate.
	enrolledAddresses := map[string]struct{}{}
	knownEnrollments, err := d.Client.Resource(networkEnrollmentGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("list enrolled members: %w", err)
	}
	for i := range knownEnrollments.Items {
		if text(&knownEnrollments.Items[i], "status", "site") != site || text(&knownEnrollments.Items[i], "status", "phase") != "Enrolled" {
			continue
		}
		addresses, _, _ := unstructured.NestedStringSlice(knownEnrollments.Items[i].Object, "spec", "addresses")
		for _, address := range addresses {
			enrolledAddresses[address] = struct{}{}
		}
		if address, _, _ := unstructured.NestedString(knownEnrollments.Items[i].Object, "status", "address"); address != "" {
			enrolledAddresses[address] = struct{}{}
		}
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
	knownMachineKeys := map[string]struct{}{}
	for i := range admissions.Items {
		clusterID := text(&admissions.Items[i], "spec", "clusterID")
		key := admissions.Items[i].GetAnnotations()[entryHostKeyAnnotation]
		ownKey[[2]string{clusterID, admissions.Items[i].GetName()}] = key
		if key != "" {
			knownMachineKeys[key] = struct{}{}
		}
	}
	machines := d.Client.Resource(machineGVR).Namespace("station-system")
	existingMachines, err := machines.List(ctx, metav1.ListOptions{})
	if err != nil {
		return 0, fmt.Errorf("list Machines: %w", err)
	}
	unclassifiedCount := 0
	atCapacity := false
	reported := make(map[string]bool, len(hosts))
	for i := range existingMachines.Items {
		candidate := &existingMachines.Items[i]
		candidateSite := text(candidate, "spec", "site")
		candidateClass := text(candidate, "spec", "nodeClass")
		candidatePhase := text(candidate, "status", "phase")
		admit, _, _ := unstructured.NestedMap(candidate.Object, "spec", "admit")
		if candidateSite == site && candidateClass == "" && len(admit) == 0 && (candidatePhase == "" || candidatePhase == "Discovered") {
			unclassifiedCount++
		}
	}
	for _, host := range hosts {
		class := classOf[host.HostKey]
		ip := net.ParseIP(host.Address)
		mac, macErr := net.ParseMAC(host.MAC)
		if ip == nil || ip.To4() == nil || macErr != nil || len(mac) != 6 || mac[0]&1 != 0 || !hardwareAddressPattern.MatchString(strings.ToLower(host.MAC)) ||
			!hostKeyPattern.MatchString(host.HostKey) || host.Port < 1 || host.Port > 65535 || len(host.Banner) > 200 || len(host.HostKeyAlgorithm) > 64 {
			continue
		}
		name := site + "-" + strings.ReplaceAll(strings.ToLower(host.MAC), ":", "")
		if reported[name] {
			continue
		}
		machine, err := machines.Get(ctx, name, metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return 0, fmt.Errorf("read Machine %s: %w", name, err)
		}
		// A machine's entry key is continuity evidence after join, never a
		// class match. Do not rediscover such a key as a generic candidate.
		_, isKnownNodeKey := knownMachineKeys[host.HostKey]
		isSameKnownMachine := err == nil && text(machine, "status", "entry", "hostKeyFingerprint") == host.HostKey
		if class == "" && !isSameKnownMachine {
			_, enrolled := enrolledAddresses[host.Address]
			if !includeUnclassified || !host.Unclassified || isKnownNodeKey || enrolled || !privateDiscoveryIP(ip) || !validDiscoverySSH(host) {
				continue
			}
		}
		if err == nil && (text(machine, "spec", "site") != site || text(machine, "spec", "source") != "Discovery") {
			continue
		}
		if class == "" && !isSameKnownMachine && apierrors.IsNotFound(err) && unclassifiedCount >= 512 {
			atCapacity = true
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
		created := false
		if apierrors.IsNotFound(err) {
			machineSpec := map[string]interface{}{"site": site, "source": "Discovery"}
			if class != "" {
				machineSpec["nodeClass"] = class
			}
			machine, err = machines.Create(ctx, &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": stationGroup + "/" + stationVersion, "kind": "Machine",
				"metadata": map[string]interface{}{"name": name, "namespace": "station-system"},
				"spec":     machineSpec,
			}}, metav1.CreateOptions{})
			created = err == nil
			if apierrors.IsAlreadyExists(err) {
				machine, err = machines.Get(ctx, name, metav1.GetOptions{})
			}
		}
		if err != nil {
			return 0, fmt.Errorf("keep Machine %s: %w", name, err)
		}
		if class != "" && text(machine, "spec", "nodeClass") == "" && (text(machine, "status", "phase") == "" || text(machine, "status", "phase") == "Discovered") {
			if admit, _, _ := unstructured.NestedMap(machine.Object, "spec", "admit"); len(admit) == 0 {
				_ = unstructured.SetNestedField(machine.Object, class, "spec", "nodeClass")
				if machine, err = machines.Update(ctx, machine, metav1.UpdateOptions{}); err != nil {
					return 0, fmt.Errorf("classify unadmitted Machine %s: %w", name, err)
				}
			}
		}
		hostKey := host.HostKey
		// A machine that shows its class's factory key is written with it:
		// station-controller looks at it by that key, and a machine set up anew
		// is joined again.
		if key := ownKey[[2]string{text(machine, "status", "clusterID"), text(machine, "status", "nodeName")}]; key != "" && class == "" {
			hostKey = key
		}
		entry := map[string]interface{}{"address": host.Address, "mac": strings.ToLower(host.MAC), "hostKeyFingerprint": hostKey,
			"port": int64(host.Port), "hostKeyAlgorithm": host.HostKeyAlgorithm, "banner": host.Banner,
			"via": via, "lastSeen": now.UTC().Format(time.RFC3339)}
		// A machine that shows up changed is looked at anew.
		for _, field := range []string{"address", "port", "hostKeyFingerprint", "hostKeyAlgorithm", "via"} {
			if previous, _, _ := unstructured.NestedFieldNoCopy(machine.Object, "status", "entry", field); !reflect.DeepEqual(previous, entry[field]) {
				unstructured.RemoveNestedField(machine.Object, "status", "inspectedAt")
			}
		}
		_ = unstructured.SetNestedMap(machine.Object, entry, "status", "entry")
		if created && class == "" {
			unclassifiedCount++
		}
		if _, err := machines.UpdateStatus(ctx, machine, metav1.UpdateOptions{}); err != nil && !apierrors.IsConflict(err) {
			return 0, fmt.Errorf("write what was seen of Machine %s: %w", name, err)
		}
		reported[name] = true
	}
	if atCapacity {
		return http.StatusTooManyRequests, nil
	}
	return http.StatusOK, nil
}

// Keep the API-side checks aligned with the Hub's private IPv4 discovery.
func privateDiscoveryIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}

func validDiscoverySSH(host discoveredHost) bool {
	switch host.HostKeyAlgorithm {
	case "ssh-ed25519", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "ssh-rsa":
		return strings.HasPrefix(host.Banner, "SSH-") && !strings.ContainsAny(host.Banner, "\r\n")
	default:
		return false
	}
}
