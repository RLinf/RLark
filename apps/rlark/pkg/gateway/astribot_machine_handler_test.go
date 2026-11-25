package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// handlerMachine is a Machine RLark has read: its entry device let it in and the computer behind it answered.
func handlerMachine(name, phase, problem, serial string) *unstructured.Unstructured {
	status := map[string]interface{}{"phase": phase, "serial": serial, "inspectedAt": "2027-01-15T07:58:00Z", "nodeHostKeyFingerprint": "SHA256:x86",
		"entry": map[string]interface{}{"address": "192.168.31.56", "port": int64(22), "hostKeyFingerprint": "SHA256:entry", "hostKeyAlgorithm": "ssh-ed25519", "mac": "48:b0:2d:5c:9a:11", "lastSeen": "2027-01-15T08:00:" + name[len(name)-2:] + "Z"}}
	if problem != "" {
		status["problem"] = problem
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "Machine", "metadata": map[string]interface{}{"name": name, "namespace": "station-system"},
		"spec": map[string]interface{}{"site": "lab", "nodeClass": "s1u", "source": "Discovery"}, "status": status,
	}}
}

func TestMachinesAreListedAndAdmittedEachForItself(t *testing.T) {
	gin.SetMode(gin.TestMode)
	class := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeClass", "metadata": map[string]interface{}{"name": "s1u"},
		"spec": map[string]interface{}{"kind": "Robot", "model": "s1u", "discovery": map[string]interface{}{"hostKeyFingerprints": []interface{}{"SHA256:entry"}}, "labels": map[string]interface{}{"rlark.io/robot-model": "s1u", "rlark.io/robot-body": "s1u"}},
	}}
	earlier := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission", "metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "agent-a"},
		"spec": map[string]interface{}{"clusterID": "cluster-a"},
	}}
	// The join of lab-03, stopped after its first stage.
	blocked := handlerMachine("lab-03", "Admitted", "JoinBlocked", "1794925030999")
	blocked.SetGeneration(2)
	_ = unstructured.SetNestedField(blocked.Object, "join-s1u-02-x86-2", "status", "operation")
	_ = unstructured.SetNestedField(blocked.Object, map[string]interface{}{"clusterID": "cluster-a", "requestedBy": "alice", "requestedAt": "2027-01-15T07:59:00Z"}, "spec", "admit")
	join := func(name, generation, phase, stage string) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeOperation",
			"metadata": map[string]interface{}{"name": name, "namespace": "agent-a", "labels": map[string]interface{}{machineOperationLabel: "lab-03"},
				"annotations": map[string]interface{}{machineRequestAnnotation: generation}},
			"status": map[string]interface{}{"phase": phase, "stage": stage},
		}}
	}
	// An earlier request's join, which made a node removed since, is not this request's.
	blockedJoin, spentJoin := join("join-s1u-02-x86-2", "2", "Blocked", "Preflight"), join("join-s1u-02-x86-1", "1", "Succeeded", "RotateEntryHostKey")
	// lab-05 was seen again since it was read last: it is read anew before it can be admitted.
	unread := handlerMachine("lab-05", "Discovered", "", "1794925031001")
	unstructured.RemoveNestedField(unread.Object, "status", "inspectedAt")
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		stationClusterGVR: "ClusterList", nodeAdmissionGVR: "NodeAdmissionList", machineGVR: "MachineList", nodeClassGVR: "NodeClassList", nodeOperationGVR: "NodeOperationList",
	}, stationHandlerCluster("cluster-a", "agent-a"), class, earlier, blockedJoin, spentJoin,
		handlerMachine("lab-01", "Discovered", "", "1794925030610"), handlerMachine("lab-02", "Discovered", "CredentialRejected", "1794925029733"),
		blocked, handlerMachine("lab-04", "Joined", "", "1794925031000"), unread)
	g := &Gateway{astribotState: astribotState{stationClient: client}}

	status, body := nodeAdmissionCall(t, g.handleListMachines, "GET", "/api/v1/station/machines?phase=Discovered", "", "")
	var listed struct {
		Items   []machine               `json:"items"`
		Classes map[string]machineClass `json:"classes"`
	}
	_ = json.Unmarshal(body, &listed)
	if status != 200 || len(listed.Items) != 3 || listed.Items[0].Name != "lab-05" || listed.Items[0].Admissible || listed.Items[1].Name != "lab-02" || listed.Items[1].Admissible ||
		!listed.Items[2].Admissible || listed.Items[2].Serial != "1794925030610" || listed.Items[2].NodeHostKeyFingerprint != "SHA256:x86" ||
		listed.Classes["s1u"].Labels["rlark.io/robot-body"] != "s1u" {
		t.Fatalf("list %d: %s", status, body)
	}
	// An admitted machine is listed with where its join stands.
	status, body = nodeAdmissionCall(t, g.handleListMachines, "GET", "/api/v1/station/machines?phase=Admitted", "", "")
	_ = json.Unmarshal(body, &listed)
	if status != 200 || len(listed.Items) != 1 || listed.Items[0].AdmitClusterID != "cluster-a" || listed.Items[0].Join == nil ||
		*listed.Items[0].Join != (machineJoin{Phase: "Blocked", Stage: "Preflight"}) {
		t.Fatalf("an admitted machine's join %d: %s", status, body)
	}

	status, body = nodeAdmissionCall(t, g.handleAdmitMachines, "POST", "/api/v1/station/machines/admit", "",
		`{"cluster_id":"cluster-a","machines":["lab-01","lab-02","lab-03","lab-04","lab-99"],"labels":{"rlark.io/robot-body":"s1u_observer"}}`)
	var answer struct {
		Machines []admittedMachine `json:"machines"`
	}
	_ = json.Unmarshal(body, &answer)
	admitted := ""
	for _, one := range answer.Machines {
		if one.Admitted {
			admitted += one.Name + " "
		}
	}
	if status != 200 || len(answer.Machines) != 5 || admitted != "lab-01 lab-03 " || !strings.Contains(answer.Machines[4].Error, "no such machine") {
		t.Fatalf("admit %d: %s", status, body)
	}
	stored, _ := client.Resource(machineGVR).Namespace("station-system").Get(context.Background(), "lab-01", metav1.GetOptions{})
	admit, _, _ := unstructured.NestedMap(stored.Object, "spec", "admit")
	labels, _, _ := unstructured.NestedStringMap(stored.Object, "spec", "admit", "labels")
	if admit["clusterID"] != "cluster-a" || admit["requestedBy"] != "alice" || admit["requestedAt"] == nil || labels["rlark.io/robot-body"] != "s1u_observer" {
		t.Fatalf("what was written on the Machine: %v", admit)
	}
	refused, _ := client.Resource(machineGVR).Namespace("station-system").Get(context.Background(), "lab-02", metav1.GetOptions{})
	if _, written, _ := unstructured.NestedMap(refused.Object, "spec", "admit"); written {
		t.Fatal("a machine that refuses its class's account was admitted")
	}

	// Until station-controller has acted on it, lab-01 is being joined: it is listed so and not admitted twice.
	status, body = nodeAdmissionCall(t, g.handleListMachines, "GET", "/api/v1/station/machines?phase=Discovered", "", "")
	_ = json.Unmarshal(body, &listed)
	if status != 200 || listed.Items[2].Name != "lab-01" || !listed.Items[2].Joining || listed.Items[2].Admissible {
		t.Fatalf("a machine whose admission is being acted on %d: %s", status, body)
	}
	status, body = nodeAdmissionCall(t, g.handleAdmitMachines, "POST", "/api/v1/station/machines/admit", "", `{"cluster_id":"cluster-a","machines":["lab-01"]}`)
	_ = json.Unmarshal(body, &answer)
	if status != 200 || answer.Machines[0].Admitted || !strings.Contains(answer.Machines[0].Error, "being joined to cluster cluster-a already, as alice asked at") {
		t.Fatalf("the same machine admitted twice: %d %s", status, body)
	}

	// A label the class does not declare is nobody's to set here.
	status, body = nodeAdmissionCall(t, g.handleAdmitMachines, "POST", "/api/v1/station/machines/admit", "",
		`{"cluster_id":"cluster-a","machines":["lab-03"],"labels":{"rlark.io/tenant":"other"}}`)
	_ = json.Unmarshal(body, &answer)
	if status != 200 || answer.Machines[0].Admitted || !strings.Contains(answer.Machines[0].Error, "rlark.io/tenant") {
		t.Fatalf("a label of no class: %d %s", status, body)
	}
	if status, _ := nodeAdmissionCall(t, g.handleAdmitMachines, "POST", "/api/v1/station/machines/admit", "", `{"cluster_id":"cluster-a","machines":[]}`); status != 400 {
		t.Fatalf("no machines: %d", status)
	}
}
