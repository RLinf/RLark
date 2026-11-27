package stationidentity

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestAHubsDiscoveredHostsBecomeMachinesOfTheClassWhoseKeyTheyShow(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	hub, neighbour, leaf := newDocumentNode(t), newDocumentNode(t), newDocumentNode(t)
	factory := "SHA256:" + strings.Repeat("f", 43)
	enrollment := func(name string, node documentNode, role string) *unstructured.Unstructured {
		holder := documentHolder("NetworkEnrollment", "robots", name, "Enrolled", node.public(), `{}`)
		_ = unstructured.SetNestedField(holder.Object, "lab", "status", "site")
		_ = unstructured.SetNestedField(holder.Object, role, "status", "role")
		_ = unstructured.SetNestedField(holder.Object, "10.254.240.5", "status", "tunnelAddress")
		return holder
	}
	network := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "SiteNetwork", "metadata": map[string]interface{}{"name": "default", "namespace": "station-system"},
		"spec": map[string]interface{}{"sites": []interface{}{
			map[string]interface{}{"name": "lab", "discovery": map[string]interface{}{"enabled": true, "ranges": []interface{}{"192.168.31.0/24"}, "intervalSeconds": int64(30)}}}},
	}}
	class := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeClass", "metadata": map[string]interface{}{"name": "s1u"},
		"spec": map[string]interface{}{"discovery": map[string]interface{}{"hostKeyFingerprints": []interface{}{factory}}},
	}}
	second := enrollment("alexfu", neighbour, "Hub")
	_ = unstructured.SetNestedField(second.Object, "10.254.240.6", "status", "tunnelAddress")
	api, client := testNetworkDocuments(t, &now, enrollment("cp", hub, "Hub"), second, enrollment("s1u-01-x86", leaf, "Leaf"), network, class)
	report := func(node documentNode, hosts string) int {
		t.Helper()
		request := func(challenge, proof string) (int, documentAnswer) {
			response := apiRequest(api, http.MethodPost, "/v1/network/discovered",
				`{"public_key":"`+node.public()+`","challenge":"`+challenge+`","proof":"`+proof+`","hosts":`+hosts+`}`, "")
			var answer documentAnswer
			_ = json.Unmarshal(response.Body.Bytes(), &answer)
			return response.Code, answer
		}
		status, challenged := request("", "")
		if status != http.StatusUnauthorized {
			t.Fatalf("what was found is taken without a proof of the hub's key: %d", status)
		}
		status, _ = request(challenged.Challenge, node.prove(challenged))
		return status
	}
	hosts := `[{"address":"192.168.31.56","port":22,"mac":"48:b0:2d:5c:9a:11","host_key":"` + factory + `"},
		{"address":"192.168.31.9","port":22,"mac":"48:b0:2d:00:00:01","host_key":"SHA256:` + strings.Repeat("o", 43) + `"}]`
	if status := report(hub, hosts); status != http.StatusOK {
		t.Fatalf("the hub's report: %d", status)
	}
	machines := client.Resource(machineGVR).Namespace("station-system")
	list, _ := machines.List(context.Background(), metav1.ListOptions{})
	if len(list.Items) != 1 || list.Items[0].GetName() != "lab-48b02d5c9a11" {
		t.Fatalf("one Machine, of the host with the class's key: %v", list.Items)
	}
	machine := &list.Items[0]
	spec, _, _ := unstructured.NestedStringMap(machine.Object, "spec")
	entry, _, _ := unstructured.NestedMap(machine.Object, "status", "entry")
	if spec["site"] != "lab" || spec["nodeClass"] != "s1u" || spec["source"] != "Discovery" || entry["address"] != "192.168.31.56" || entry["via"] != "10.254.240.5:18022" ||
		entry["hostKeyFingerprint"] != factory || entry["lastSeen"] != now.UTC().Format(time.RFC3339) {
		t.Fatalf("the Machine: %v %v", spec, entry)
	}

	reporting, _ := client.Resource(networkEnrollmentGVR).Namespace("robots").Get(context.Background(), "cp", metav1.GetOptions{})
	if reporting.GetAnnotations()[discoveryReportedAnnotation] != now.UTC().Format(time.RFC3339) {
		t.Fatalf("when the hub reported, on its enrollment: %v", reporting.GetAnnotations())
	}

	// Seen again where it was, it stays as it was read; at another address, it is read anew.
	_ = unstructured.SetNestedField(machine.Object, "2027-01-15T08:00:00Z", "status", "inspectedAt")
	_ = unstructured.SetNestedField(machine.Object, "1794925030610", "status", "serial")
	if _, err := machines.UpdateStatus(context.Background(), machine, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Second)
	_ = report(hub, hosts)
	machine, _ = machines.Get(context.Background(), "lab-48b02d5c9a11", metav1.GetOptions{})
	if _, inspected, _ := unstructured.NestedString(machine.Object, "status", "inspectedAt"); !inspected {
		t.Fatal("a machine seen again where it was is read anew")
	}
	_ = report(hub, strings.Replace(hosts, "192.168.31.56", "192.168.31.77", 1))
	machine, _ = machines.Get(context.Background(), "lab-48b02d5c9a11", metav1.GetOptions{})
	_, inspected, _ := unstructured.NestedString(machine.Object, "status", "inspectedAt")
	if serial, _, _ := unstructured.NestedString(machine.Object, "status", "serial"); inspected || serial != "1794925030610" {
		t.Fatalf("a machine at another address: inspected %v, serial %q", inspected, serial)
	}

	// Joined, its entry device was given a key of its own (station-controller writes it on the Machine). Shown by
	// that key after its lease changed, it is that machine at its new address; a key nobody knows it by says
	// nothing, and neither does its key on another device.
	rotated := "SHA256:" + strings.Repeat("r", 43)
	_ = unstructured.SetNestedField(machine.Object, rotated, "status", "entry", "hostKeyFingerprint")
	_ = unstructured.SetNestedField(machine.Object, "Joined", "status", "phase")
	_ = unstructured.SetNestedField(machine.Object, "robots", "status", "clusterID")
	_ = unstructured.SetNestedField(machine.Object, "s1u-01-x86", "status", "nodeName")
	if _, err := machines.UpdateStatus(context.Background(), machine, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Second)
	_ = report(hub, `[{"address":"192.168.31.140","port":22,"mac":"48:b0:2d:5c:9a:11","host_key":"`+rotated+`"},
		{"address":"192.168.31.141","port":22,"mac":"48:b0:2d:00:00:02","host_key":"`+rotated+`"}]`)
	machine, _ = machines.Get(context.Background(), "lab-48b02d5c9a11", metav1.GetOptions{})
	entry, _, _ = unstructured.NestedMap(machine.Object, "status", "entry")
	if entry["address"] != "192.168.31.140" || entry["hostKeyFingerprint"] != rotated || entry["lastSeen"] != now.UTC().Format(time.RFC3339) {
		t.Fatalf("a joined machine whose lease changed: %v", entry)
	}
	_ = report(hub, `[{"address":"192.168.31.150","port":22,"mac":"48:b0:2d:5c:9a:11","host_key":"SHA256:`+strings.Repeat("x", 43)+`"}]`)
	machine, _ = machines.Get(context.Background(), "lab-48b02d5c9a11", metav1.GetOptions{})
	if address, _, _ := unstructured.NestedString(machine.Object, "status", "entry", "address"); address != "192.168.31.140" {
		t.Fatalf("a host with the machine's hardware address and another key moved the Machine to %s", address)
	}
	if list, _ := machines.List(context.Background(), metav1.ListOptions{}); len(list.Items) != 1 {
		t.Fatalf("Machines after a known key on another device: %d", len(list.Items))
	}
	// The key its NodeAdmission records is the one it is known by. A report of the factory's key is written, for
	// station-controller to look at the machine by it (one set up anew shows it), and does not drop it.
	admission := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
		"metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "agents-robots", "annotations": map[string]interface{}{entryHostKeyAnnotation: rotated}},
		"spec":     map[string]interface{}{"clusterID": "robots"},
	}}
	if _, err := client.Resource(nodeAdmissionGVR).Namespace("agents-robots").Create(context.Background(), admission, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	_ = report(hub, `[{"address":"192.168.31.140","port":22,"mac":"48:b0:2d:5c:9a:11","host_key":"`+factory+`"}]`)
	machine, _ = machines.Get(context.Background(), "lab-48b02d5c9a11", metav1.GetOptions{})
	entry, _, _ = unstructured.NestedMap(machine.Object, "status", "entry")
	if entry["hostKeyFingerprint"] != factory || entry["lastSeen"] != now.UTC().Format(time.RFC3339) {
		t.Fatalf("a joined machine reported with the factory's key: %v", entry)
	}
	_ = unstructured.SetNestedField(machine.Object, rotated, "status", "entry", "hostKeyFingerprint")
	if _, err := machines.UpdateStatus(context.Background(), machine, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}

	// Another hub on the same network sees it too: it stays with the hub that has it, and is not looked at anew,
	// until that hub has not seen it for three intervals.
	_ = unstructured.SetNestedField(machine.Object, "2027-01-15T08:00:00Z", "status", "inspectedAt")
	if _, err := machines.UpdateStatus(context.Background(), machine, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	seenThere := `[{"address":"192.168.31.140","port":22,"mac":"48:b0:2d:5c:9a:11","host_key":"` + rotated + `"}]`
	now = now.Add(30 * time.Second)
	_ = report(neighbour, seenThere)
	machine, _ = machines.Get(context.Background(), "lab-48b02d5c9a11", metav1.GetOptions{})
	_, inspected, _ = unstructured.NestedString(machine.Object, "status", "inspectedAt")
	if via, _, _ := unstructured.NestedString(machine.Object, "status", "entry", "via"); via != "10.254.240.5:18022" || !inspected {
		t.Fatalf("a machine its hub still sees, reported by another: via %s, inspected %v", via, inspected)
	}
	now = now.Add(61 * time.Second)
	_ = report(neighbour, seenThere)
	machine, _ = machines.Get(context.Background(), "lab-48b02d5c9a11", metav1.GetOptions{})
	_, inspected, _ = unstructured.NestedString(machine.Object, "status", "inspectedAt")
	if via, _, _ := unstructured.NestedString(machine.Object, "status", "entry", "via"); via != "10.254.240.6:18022" || inspected {
		t.Fatalf("a machine its hub has not seen for three intervals, reported by another: via %s, inspected %v", via, inspected)
	}

	// A leaf looks for nothing, and says nothing of its site's network.
	if status := report(leaf, strings.Replace(hosts, "9a:11", "9a:22", 1)); status != http.StatusForbidden {
		t.Fatalf("a leaf's report: %d", status)
	}
	if list, _ := machines.List(context.Background(), metav1.ListOptions{}); len(list.Items) != 1 {
		t.Fatalf("Machines after a leaf's report: %d", len(list.Items))
	}
}

func TestOptedInHubCreatesCredentialFreeUnclassifiedMachine(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	hubNode := newDocumentNode(t)
	hub := documentHolder("NetworkEnrollment", "robots", "cp", "Enrolled", hubNode.public(), `{}`)
	_ = unstructured.SetNestedField(hub.Object, "lab", "status", "site")
	_ = unstructured.SetNestedField(hub.Object, "Hub", "status", "role")
	_ = unstructured.SetNestedField(hub.Object, "10.254.240.5", "status", "tunnelAddress")
	network := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "station.rlark.io/v1alpha1", "kind": "SiteNetwork",
		"metadata": map[string]interface{}{"name": "default", "namespace": "station-system"}, "spec": map[string]interface{}{"sites": []interface{}{map[string]interface{}{
			"name": "lab", "discovery": map[string]interface{}{"enabled": true, "includeUnclassified": true, "ranges": []interface{}{"192.168.31.0/24"}, "intervalSeconds": int64(30)}}}}}}
	api, client := testNetworkDocuments(t, &now, hub, network)
	hosts := `[{"address":"192.168.31.56","port":2222,"mac":"48:b0:2d:5c:9a:11","host_key":"SHA256:` + strings.Repeat("f", 43) + `","host_key_algorithm":"ssh-ed25519","ssh_banner":"SSH-2.0-test","unclassified":true},` +
		`{"address":"192.168.31.56","port":2222,"mac":"48:b0:2d:5c:9a:11","host_key":"SHA256:` + strings.Repeat("f", 43) + `","host_key_algorithm":"ssh-ed25519","ssh_banner":"SSH-2.0-test","unclassified":true}]`
	request := func(challenge, proof string) int {
		body := `{"public_key":"` + hubNode.public() + `","challenge":"` + challenge + `","proof":"` + proof + `","hosts":` + hosts + `}`
		return apiRequest(api, http.MethodPost, "/v1/network/discovered", body, "").Code
	}
	// Ask for the proof challenge using the discovery endpoint itself.
	body := `{"public_key":"` + hubNode.public() + `","hosts":[]}`
	response := apiRequest(api, http.MethodPost, "/v1/network/discovered", body, "")
	var challenged documentAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &challenged); err != nil || response.Code != http.StatusUnauthorized {
		t.Fatalf("challenge %d %s %v", response.Code, response.Body, err)
	}
	if code := request(challenged.Challenge, hubNode.prove(challenged)); code != http.StatusOK {
		t.Fatalf("report status %d", code)
	}
	list, err := client.Resource(machineGVR).Namespace("station-system").List(context.Background(), metav1.ListOptions{})
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("candidate list %v %v", list.Items, err)
	}
	item := &list.Items[0]
	if nodeClass, found, _ := unstructured.NestedString(item.Object, "spec", "nodeClass"); found || nodeClass != "" {
		t.Fatalf("unclassified candidate has NodeClass %q", nodeClass)
	}
	entry, _, _ := unstructured.NestedMap(item.Object, "status", "entry")
	if entry["port"] != int64(2222) || entry["hostKeyAlgorithm"] != "ssh-ed25519" || entry["banner"] != "SSH-2.0-test" {
		t.Fatalf("SSH evidence missing: %v", entry)
	}
	if spec, _, _ := unstructured.NestedMap(item.Object, "spec"); spec["access"] != nil {
		t.Fatalf("candidate acquired credentials: %v", spec)
	}
}
