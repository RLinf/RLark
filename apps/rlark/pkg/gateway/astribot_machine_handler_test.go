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

func handlerMachine(name, phase, problem, serial string) *unstructured.Unstructured {
	status := map[string]interface{}{"phase": phase, "serial": serial,
		"entry": map[string]interface{}{"address": "192.168.31.56", "mac": "48:b0:2d:5c:9a:11", "lastSeen": "2027-01-15T08:00:" + name[len(name)-2:] + "Z"}}
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
		"spec": map[string]interface{}{"labels": map[string]interface{}{"rlark.io/robot-model": "s1u", "rlark.io/robot-body": "s1u"}},
	}}
	earlier := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission", "metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "agent-a"},
		"spec": map[string]interface{}{"clusterID": "cluster-a"},
	}}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		stationClusterGVR: "ClusterList", nodeAdmissionGVR: "NodeAdmissionList", machineGVR: "MachineList", nodeClassGVR: "NodeClassList",
	}, stationHandlerCluster("cluster-a", "agent-a"), class, earlier,
		handlerMachine("lab-01", "Discovered", "", "1794925030610"), handlerMachine("lab-02", "Discovered", "CredentialRejected", "1794925029733"),
		handlerMachine("lab-03", "Admitted", "JoinBlocked", "1794925030999"), handlerMachine("lab-04", "Joined", "", "1794925031000"))
	g := &Gateway{astribotState: astribotState{stationClient: client}}

	status, body := nodeAdmissionCall(t, g.handleListMachines, "GET", "/api/v1/station/machines?phase=Discovered", "", "")
	var listed struct {
		Items   []machine               `json:"items"`
		Classes map[string]machineClass `json:"classes"`
	}
	_ = json.Unmarshal(body, &listed)
	if status != 200 || len(listed.Items) != 2 || listed.Items[0].Name != "lab-02" || listed.Items[0].Admissible || !listed.Items[1].Admissible ||
		listed.Items[1].Serial != "1794925030610" || listed.Classes["s1u"].Labels["rlark.io/robot-body"] != "s1u" {
		t.Fatalf("list %d: %s", status, body)
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
