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
	"k8s.io/client-go/kubernetes/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
)

func TestTheNodePasswordsOfAClassAreEnteredAndNeverAnswered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()
	class := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeClass", "metadata": map[string]interface{}{"name": "s1u"},
		"spec": map[string]interface{}{"access": map[string]interface{}{
			"entry": map[string]interface{}{"password": map[string]interface{}{"secret": "orin-access", "key": "password"}},
			"node":  map[string]interface{}{"password": map[string]interface{}{"secret": "orin-access", "key": "x86_password"}}}},
	}}
	g := &Gateway{config: Config{KubeClientConfig: configs.KubernetesClientConfig{Namespace: "management"}}, rawClient: fake.NewSimpleClientset(),
		astribotState: astribotState{stationClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{machineGVR: "MachineList", nodeClassGVR: "NodeClassList", nodeOperationGVR: "NodeOperationList"},
			class, handlerMachine("lab-01", "Discovered", "AccessUnconfigured", ""))}}
	const path = "/api/v1/station/node-classes/s1u/access"

	// The secret the class names is not there yet: nothing can be entered into it.
	if status, body := nodeAdmissionCall(t, g.handleSetNodeClassAccess, "PUT", path, "s1u", `{"node_password":"x86-secret"}`); status != 409 {
		t.Fatalf("a password for a secret that does not exist: %d %s", status, body)
	}
	if err := putJobSecret(ctx, g.rawClient, "management", "bob", jobSecretRequest{Name: "orin-access", Description: "the Orins", Clusters: []string{"robots"},
		Data: map[string]string{"password": "orin-secret", "ssh_key": "a key"}}, false); err != nil {
		t.Fatal(err)
	}

	var access nodeClassAccess
	status, body := nodeAdmissionCall(t, g.handleGetNodeClassAccess, "GET", path, "s1u", "")
	_ = json.Unmarshal(body, &access)
	if status != 200 || !access.Entry.Configured || access.Node.Configured || access.Node.Secret != "orin-access" || access.Node.Key != "x86_password" || access.Sudo != nil {
		t.Fatalf("before anything is entered: %d %s", status, body)
	}
	// The list says the same of the class, and the machine cannot be admitted.
	status, body = nodeAdmissionCall(t, g.handleListMachines, "GET", "/api/v1/station/machines", "", "")
	var listed struct {
		Items   []machine               `json:"items"`
		Classes map[string]machineClass `json:"classes"`
	}
	_ = json.Unmarshal(body, &listed)
	if status != 200 || listed.Items[0].Admissible || listed.Classes["s1u"].Access == nil || listed.Classes["s1u"].Access.Node.Configured {
		t.Fatalf("list before anything is entered: %d %s", status, body)
	}

	for _, refused := range []string{`{}`, `{"node_password":""}`, `{"node_password":"two\nlines"}`, `{"node_password":"` + strings.Repeat("x", 257) + `"}`, `{"node_password":"a","password":"b"}`} {
		if status, body := nodeAdmissionCall(t, g.handleSetNodeClassAccess, "PUT", path, "s1u", refused); status != 400 {
			t.Fatalf("%s: %d %s", refused, status, body)
		}
	}

	status, body = nodeAdmissionCall(t, g.handleSetNodeClassAccess, "PUT", path, "s1u", `{"node_password":"x86-secret","sudo_password":"sudo-secret"}`)
	access = nodeClassAccess{}
	_ = json.Unmarshal(body, &access)
	if status != 200 || !access.Node.Configured || access.Node.SetBy != "alice" || access.Node.SetAt == "" ||
		access.Sudo == nil || !access.Sudo.Configured || access.Sudo.Key != "x86_password_sudo" || access.Entry.SetBy != "" {
		t.Fatalf("entered: %d %s", status, body)
	}
	status, read := nodeAdmissionCall(t, g.handleGetNodeClassAccess, "GET", path, "s1u", "")
	for _, answer := range [][]byte{body, read} {
		for _, value := range []string{"x86-secret", "sudo-secret", "orin-secret", "a key"} {
			if strings.Contains(string(answer), value) {
				t.Fatalf("an answer carries a value: %s", answer)
			}
		}
	}
	if status != 200 {
		t.Fatalf("read after entering: %d %s", status, read)
	}

	// The values are in the secret the class names, beside what was there; the class names the sudo password.
	declaration, err := g.rawClient.CoreV1().Secrets("management").Get(ctx, jobSecretReplicationName("orin-access"), metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := readJobSecret(declaration)
	if err != nil || len(stored.Data) != 4 || stored.Data["password"] != "orin-secret" || stored.Data["ssh_key"] != "a key" || stored.Data["x86_password"] != "x86-secret" ||
		stored.Data["x86_password_sudo"] != "sudo-secret" || stored.Description != "the Orins" || len(stored.Clusters) != 1 || stored.Clusters[0] != "robots" {
		t.Fatalf("the secret after entering: %v, description %q, clusters %v, %d values", err, stored.Description, stored.Clusters, len(stored.Data))
	}
	written, _ := g.stationClient.Resource(nodeClassGVR).Get(ctx, "s1u", metav1.GetOptions{})
	if sudo, _, _ := unstructured.NestedStringMap(written.Object, "spec", "access", "node", "sudoPassword"); sudo["secret"] != "orin-access" || sudo["key"] != "x86_password_sudo" ||
		!strings.Contains(written.GetAnnotations()[nodeClassPasswordsSetAnnotation], `"by":"alice"`) {
		t.Fatalf("the NodeClass after entering: %v", written.Object)
	}

	// Entered again without a sudo password: the node's password changes, the sudo password stays.
	if status, body := nodeAdmissionCall(t, g.handleSetNodeClassAccess, "PUT", path, "s1u", `{"node_password":"another"}`); status != 200 {
		t.Fatalf("entered again: %d %s", status, body)
	}
	declaration, _ = g.rawClient.CoreV1().Secrets("management").Get(ctx, jobSecretReplicationName("orin-access"), metav1.GetOptions{})
	if stored, _ := readJobSecret(declaration); stored.Data["x86_password"] != "another" || stored.Data["x86_password_sudo"] != "sudo-secret" {
		t.Fatal("entering the node's password again changed the sudo password or kept the old one")
	}
	if status, _ := nodeAdmissionCall(t, g.handleGetNodeClassAccess, "GET", "/api/v1/station/node-classes/none/access", "none", ""); status != 404 {
		t.Fatalf("a class that is not there: %d", status)
	}
}
