package stationidentity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// The cluster CA of the test: any certificate does, the hash is of its public key.
const testClusterInfo = `apiVersion: v1
kind: Config
clusters:
- name: ""
  cluster:
    server: https://10.11.14.128:16443
    certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJlVENDQVIrZ0F3SUJBZ0lVQ1FQRkhWbHlkZ1ZMTHhWSUcwc1B5OVQwN2Nnd0NnWUlLb1pJemowRUF3SXcKRWpFUU1BNEdBMVVFQXd3SGRHVnpkQzFqWVRBZUZ3MHlOakV4TVRJd09UTTFNelphRncwek5qRXhNRGt3T1RNMQpNelphTUJJeEVEQU9CZ05WQkFNTUIzUmxjM1F0WTJFd1dUQVRCZ2NxaGtqT1BRSUJCZ2dxaGtqT1BRTUJCd05DCkFBUjdleVkyd0dYSkoyYkRBdzI2WGpwSlFENm1GVGh0YkFTYUpBTFBWVCtaR3JNMHBpaVg1Y05UeWN5SDlyNkMKajY0VnArUVlhaGVrZm1nazhtRmlaeDE3bzFNd1VUQWRCZ05WSFE0RUZnUVU5NklpaU5MNjBUN1RzQ0xIQ0FRRwozRXAveWlVd0h3WURWUjBqQkJnd0ZvQVU5NklpaU5MNjBUN1RzQ0xIQ0FRRzNFcC95aVV3RHdZRFZSMFRBUUgvCkJBVXdBd0VCL3pBS0JnZ3Foa2pPUFFRREFnTklBREJGQWlBeTlwQzlKMjFycHVKZDdUaC9zUXhUZjlxMkV4NGYKVlJSd25Gc3FqbFAxeHdJaEFJQ1k4eUZLK0x6eVhQSnp1cmJBelk4SDNVVVMxaWJuVWxULzZZbGw4MGVvCi0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K
`

// joinableCluster is a member cluster with its cluster-info, which lets
// Station API create join tokens or not.
func joinableCluster(tokens bool) *kubefake.Clientset {
	member := kubefake.NewSimpleClientset(&corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "kube-public", Name: "cluster-info"}, Data: map[string]string{"kubeconfig": testClusterInfo}})
	member.PrependReactor("create", "selfsubjectaccessreviews", func(action k8stesting.Action) (bool, runtime.Object, error) {
		review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
		review.Status.Allowed = tokens
		return true, review, nil
	})
	return member
}

// A cluster that refuses Station API what a machine joins with refuses the
// redemption before the code is bound: the code stays open, the refusal says
// which permission the cluster lacks, and once it has it the code is redeemed.
func TestARefusedClusterLeavesTheCodeOpenAndSaysWhy(t *testing.T) {
	code, key := "k7m2p9xq", strings.Repeat("A", 43)+"="
	digest := sha256.Sum256([]byte(code))
	now := time.Unix(1_800_000_000, 0)
	management := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{nodeAdmissionGVR: "NodeAdmissionList", addonGVR: "AddonList"},
		&unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
			"metadata": map[string]interface{}{"name": "cailiang-desktop", "namespace": "agent-office"},
			"spec": map[string]interface{}{"clusterID": "office", "codeHash": hex.EncodeToString(digest[:]),
				"expiresAt": now.Add(time.Hour).UTC().Format(time.RFC3339)},
		}})
	unreadable := joinableCluster(true)
	unreadable.PrependReactor("get", "configmaps", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, "cluster-info", errors.New("RBAC"))
	})
	member := kubernetes.Interface(unreadable)
	b := &Bootstrap{Management: management, Now: func() time.Time { return now },
		Member: func(context.Context, string) (kubernetes.Interface, error) { return member, nil }}
	open := func() bool {
		stored, _ := management.Resource(nodeAdmissionGVR).Namespace("agent-office").Get(context.Background(), "cailiang-desktop", metav1.GetOptions{})
		_, bound, _ := unstructured.NestedString(stored.Object, "spec", "publicKey")
		return !bound
	}

	for want, cluster := range map[string]kubernetes.Interface{"读取集群信息（kube-public/cluster-info）被拒：集群 office 缺 station-api-cluster-info 权限": unreadable, "建入网令牌被拒：集群 office 缺 station-api-join-token 权限": joinableCluster(false)} {
		member = cluster
		recorder, request := httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/bootstrap/redeem", strings.NewReader(`{"code":"`+code+`","public_key":"`+key+`"}`))
		request.Header.Set("Content-Type", "application/json")
		b.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), want) || !open() {
			t.Fatalf("want %q, the code open: %d %s, open %v", want, recorder.Code, recorder.Body.String(), open())
		}
	}
	member = joinableCluster(true)
	if answer, status, err := b.Redeem(context.Background(), code, key); err != nil || status != http.StatusAccepted || answer.State != "pending" || open() {
		t.Fatalf("once the cluster lets it: %+v %d %v", answer, status, err)
	}
}

func TestBootstrapRedeemsACodeOnceForOneKey(t *testing.T) {
	code, key, other := "k7m2p9xq", strings.Repeat("A", 43)+"=", strings.Repeat("B", 43)+"="
	digest := sha256.Sum256([]byte(code))
	now := time.Unix(1_800_000_000, 0)
	admission := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
		"metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "rlark-local-dev-gpu"},
		"spec": map[string]interface{}{
			"clusterID": "rlark-local-dev-gpu", "codeHash": hex.EncodeToString(digest[:]),
			"expiresAt": now.Add(time.Hour).UTC().Format(time.RFC3339),
			"labels":    map[string]interface{}{"rlark.io/robot-id": "s1u-01", "rlark.io/node-category": "robot"},
			"taints":    []interface{}{"rlark.io/node-category=robot:NoSchedule"},
		},
	}}
	// The machine is installed anew: the management plane knows a node of its name.
	known := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rlinf.io/v1alpha1", "kind": "Node",
		"metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "rlark-local-dev-gpu",
			"labels": map[string]interface{}{"rlark.io/robot-id": "before", "rlark.io/robot-model": "s1u"}},
	}}
	management := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{nodeAdmissionGVR: "NodeAdmissionList", addonGVR: "AddonList"}, admission, known)
	member := joinableCluster(true)
	b := &Bootstrap{Management: management, Now: func() time.Time { return now },
		Member: func(context.Context, string) (kubernetes.Interface, error) { return member, nil }}
	ctx := context.Background()

	if answer, status, err := b.Redeem(ctx, "wrongcode", key); err != nil || status != http.StatusNotFound || answer.State != "rejected" {
		t.Fatalf("unknown code: %+v %d %v", answer, status, err)
	}
	if answer, status, err := b.Redeem(ctx, code, key); err != nil || status != http.StatusAccepted || answer.State != "pending" {
		t.Fatalf("first redemption: %+v %d %v", answer, status, err)
	}
	if answer, status, err := b.Redeem(ctx, code, other); err != nil || status != http.StatusConflict {
		t.Fatalf("a second machine must not redeem the code: %+v %d %v", answer, status, err)
	}
	if answer, status, err := b.Redeem(ctx, code, key); err != nil || status != http.StatusAccepted || answer.Join != nil {
		t.Fatalf("no join token before the tunnel is computed: %+v %d %v", answer, status, err)
	}

	resource := management.Resource(nodeAdmissionGVR).Namespace("rlark-local-dev-gpu")
	stored, err := resource.Get(ctx, "s1u-01-x86", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stored.Object["status"] = map[string]interface{}{"phase": "Redeemed", "address": "10.254.240.3", "publicKey": key, "config": `{"address":"10.254.240.3/32"}`}
	if _, err := resource.Update(ctx, stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	// The site's registry cache is being filled: the machine waits and is told
	// what the cache says, until the cache is Ready for its last change. A
	// registry that is no cache is not waited for.
	addon := func(name, role, phase, message string, generation, seen int64) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "rlinf.io/v1alpha1", "kind": "Addon",
			"metadata": map[string]interface{}{"name": name, "namespace": "rlark-local-dev-gpu", "generation": generation},
			"spec":     map[string]interface{}{"addonName": "site-registry", "values": map[string]interface{}{"role": role}},
			"status":   map[string]interface{}{"phase": phase, "message": message, "observedGeneration": seen},
		}}
	}
	addons := management.Resource(addonGVR).Namespace("rlark-local-dev-gpu")
	_, _ = addons.Create(ctx, addon("origin", "registry", "Failed", "站点缓存不可用: no", 1, 1), metav1.CreateOptions{})
	_, _ = addons.Create(ctx, addon("cache", "cache", "Installing", "站点缓存预热中: pass 1", 1, 1), metav1.CreateOptions{})
	if answer, status, err := b.Redeem(ctx, code, key); err != nil || status != http.StatusAccepted || answer.Reason != "站点缓存预热中: pass 1" || answer.Join != nil {
		t.Fatalf("while the cache is filled: %+v %d %v", answer, status, err)
	}
	_, _ = addons.Update(ctx, addon("cache", "cache", "Ready", "", 2, 1), metav1.UpdateOptions{})
	if answer, status, err := b.Redeem(ctx, code, key); err != nil || status != http.StatusAccepted || !strings.Contains(answer.Reason, "has not reported on its last change") {
		t.Fatalf("a Ready that is older than the cache's last change: %+v %d %v", answer, status, err)
	}
	_, _ = addons.Update(ctx, addon("cache", "cache", "Ready", "", 2, 2), metav1.UpdateOptions{})
	answer, status, err := b.Redeem(ctx, code, key)
	if err != nil || status != http.StatusOK || answer.State != "ready" || answer.NodeName != "s1u-01-x86" || answer.TunnelAddress != "10.254.240.3" ||
		answer.Join.APIServer != "10.11.14.128:16443" || len(answer.Join.Token) != 23 || !strings.HasPrefix(answer.Join.CACertHash, "sha256:") ||
		answer.Labels != "rlark.io/cluster-id=rlark-local-dev-gpu,rlark.io/enabled=true,rlark.io/node-category=robot,rlark.io/robot-id=s1u-01" || answer.Taints != "rlark.io/node-category=robot:NoSchedule" {
		t.Fatalf("ready answer: %+v %d %v", answer, status, err)
	}
	if stored, _ := resource.Get(ctx, "s1u-01-x86", metav1.GetOptions{}); stored.GetAnnotations()[JoinTokenIssuedAnnotation] != now.UTC().Format(time.RFC3339) {
		t.Fatalf("the admission does not say the machine has its join token: %v", stored.GetAnnotations())
	}
	node, err := management.Resource(rlarkNodeGVR).Namespace("rlark-local-dev-gpu").Get(ctx, "s1u-01-x86", metav1.GetOptions{})
	if want := map[string]string{"rlark.io/robot-id": "s1u-01", "rlark.io/node-category": "robot", "rlark.io/robot-model": "s1u", "rlark.io/enabled": "true", "rlark.io/cluster-id": "rlark-local-dev-gpu"}; err != nil || !reflect.DeepEqual(node.GetLabels(), want) {
		t.Fatalf("labels of the management Node after the admission: %v %v", node.GetLabels(), err)
	}
	secrets, err := member.CoreV1().Secrets("kube-system").List(ctx, metav1.ListOptions{})
	if err != nil || len(secrets.Items) != 1 || secrets.Items[0].Type != corev1.SecretTypeBootstrapToken ||
		secrets.Items[0].StringData["expiration"] != now.Add(30*time.Minute).UTC().Format(time.RFC3339) {
		t.Fatalf("join token secret: %+v %v", secrets.Items, err)
	}

	// A node that joins again got a new code for its admission: "Joined" from
	// before is no answer until station-controller has seen this redemption.
	stored, _ = resource.Get(ctx, "s1u-01-x86", metav1.GetOptions{})
	stored.SetGeneration(5)
	stored.Object["status"] = map[string]interface{}{"phase": "Joined", "address": "10.254.240.3", "observedGeneration": int64(4)}
	if _, err := resource.Update(ctx, stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if answer, status, err := b.Redeem(ctx, code, key); err != nil || status != http.StatusAccepted || answer.State != "pending" {
		t.Fatalf("a status older than the redemption: %+v %d %v", answer, status, err)
	}
	_ = unstructured.SetNestedField(stored.Object, int64(5), "status", "observedGeneration")
	if _, err := resource.Update(ctx, stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if answer, status, err := b.Redeem(ctx, code, key); err != nil || status != http.StatusGone {
		t.Fatalf("joined: %+v %d %v", answer, status, err)
	}

	// After the time is up an unredeemed code is dead.
	stored, _ = resource.Get(ctx, "s1u-01-x86", metav1.GetOptions{})
	unstructured.RemoveNestedField(stored.Object, "spec", "publicKey")
	if _, err := resource.Update(ctx, stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if answer, status, err := b.Redeem(ctx, code, key); err != nil || status != http.StatusGone {
		t.Fatalf("expired code: %+v %d %v", answer, status, err)
	}
}

func TestBootstrapServesTheClassUplinkSetupForAnUnredeemedCode(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	admission := func(name, cluster, class, code string) *unstructured.Unstructured {
		digest := sha256.Sum256([]byte(code))
		spec := map[string]interface{}{"clusterID": cluster, "codeHash": hex.EncodeToString(digest[:]),
			"expiresAt": now.Add(time.Hour).UTC().Format(time.RFC3339)}
		if class != "" {
			spec["nodeClass"] = class
		}
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
			"metadata": map[string]interface{}{"name": name, "namespace": cluster}, "spec": spec,
		}}
	}
	class := func(name string, spec map[string]interface{}) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeClass", "metadata": map[string]interface{}{"name": name}, "spec": spec,
		}}
	}
	uplink := func(path string) map[string]interface{} {
		return map[string]interface{}{"uplink": map[string]interface{}{"setup": map[string]interface{}{"addon": "site-robot", "path": path}}}
	}
	addons := t.TempDir()
	if err := os.MkdirAll(addons+"/addons/site-robot/bootstrap", 0755); err != nil {
		t.Fatal(err)
	}
	setup := "#!/bin/sh\necho the uplink\n"
	_ = os.WriteFile(addons+"/addons/site-robot/bootstrap/orin-uplink.sh", []byte(setup), 0644)
	_ = os.WriteFile(addons+"/secret", []byte("not an addon's"), 0644)
	management := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{nodeAdmissionGVR: "NodeAdmissionList"},
		admission("s1u-03-x86", "robot-s1u", "s1u", "robotcode"), admission("gpu-9", "office-gpu", "", "officecode"),
		admission("gpu-8", "robot-s1u", "gpu", "gpucode"), admission("out", "escape", "escape", "escapecode"), admission("s1u-04-x86", "robot-s1u", "built-in", "builtincode"),
		class("s1u", uplink("bootstrap/orin-uplink.sh")), class("gpu", map[string]interface{}{}), class("escape", uplink("../../secret")),
		class("built-in", map[string]interface{}{"uplink": map[string]interface{}{"setup": map[string]interface{}{"addon": "s1u-robot", "path": "bootstrap/orin-uplink.sh"}}}))
	b := &Bootstrap{Management: management, AddonsDir: addons + "/addons", CA: []byte("the CA\n"), Now: func() time.Time { return now }}
	get := func(path string) (int, string) {
		recorder := httptest.NewRecorder()
		b.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		return recorder.Code, recorder.Body.String()
	}

	if status, body := get("/v1/bootstrap/ca"); status != http.StatusOK || body != "the CA\n" {
		t.Fatalf("the CA, byte for byte: %d %q", status, body)
	}
	if status, body := get("/b/robotcode/uplink"); status != http.StatusOK || body != setup {
		t.Fatalf("the setup of the robot's class: %d %q", status, body)
	}
	// A built-in addon's file is this program's own: no directory holds it.
	if status, body := get("/b/builtincode/uplink"); status != http.StatusOK || !strings.Contains(body, "s1u-orin-uplink") {
		t.Fatalf("the setup a built-in addon publishes: %d %q", status, body)
	}
	// A machine of no class, or of a class that declares none: the operator's command is the one line.
	for _, code := range []string{"officecode", "gpucode"} {
		if status, body := get("/b/" + code + "/uplink"); status != http.StatusNotFound || strings.Contains(body, "uplink") {
			t.Fatalf("a machine without a setup (%s): %d %q", code, status, body)
		}
	}
	if status, body := get("/b/escapecode/uplink"); status != http.StatusServiceUnavailable || strings.Contains(body, "not an addon's") {
		t.Fatalf("a declaration that points out of the addons directory: %d %q", status, body)
	}
	// The installer is loaded only when the machine does not hold the one of Station API's sha256, resumed when the
	// download breaks off, checked by its sha256 before it is unpacked, and not kept beside what it unpacks to.
	if status, body := get("/b/robotcode"); status != http.StatusOK ||
		!strings.Contains(body, "RLARK_CODE=robotcode") ||
		!strings.Contains(body, `want=$(curl -fsS --cacert $B/station-api-ca.crt https://example.com/v1/bootstrap/bundle.sha256)`) ||
		!strings.Contains(body, `if [ "$(cat $B/bundle.sha256 2>/dev/null)" = "$want" ]`) ||
		!strings.Contains(body, "curl -fSs -C -") ||
		!strings.Contains(body, `echo "$want  $bundle" | sha256sum -c --status -`) ||
		!strings.Contains(body, `echo "$want" > $B/bundle.sha256`) ||
		!strings.Contains(body, "rm -f $bundle\nfi") ||
		strings.Contains(body, "| tar -xf -") {
		t.Fatalf("the installer script beside it: %d %q", status, body)
	}

	// Redeemed by the leaf, the code opens nothing more; nor does it after its time.
	resource := management.Resource(nodeAdmissionGVR).Namespace("robot-s1u")
	stored, _ := resource.Get(context.Background(), "s1u-03-x86", metav1.GetOptions{})
	_ = unstructured.SetNestedField(stored.Object, strings.Repeat("A", 43)+"=", "spec", "publicKey")
	if _, err := resource.Update(context.Background(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	if status, _ := get("/b/robotcode/uplink"); status != http.StatusNotFound {
		t.Fatalf("a redeemed code: %d", status)
	}
	unstructured.RemoveNestedField(stored.Object, "spec", "publicKey")
	if _, err := resource.Update(context.Background(), stored, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if status, _ := get("/b/robotcode/uplink"); status != http.StatusNotFound {
		t.Fatalf("an expired code: %d", status)
	}

	// Guessing is as slow here as at the redemption, and known codes do not count.
	for range 10 {
		if status, _ := get("/b/guessguess/uplink"); status != http.StatusNotFound {
			t.Fatalf("an unknown code: %d", status)
		}
	}
	if status, _ := get("/b/guessguess/uplink"); status != http.StatusNotFound {
		t.Fatalf("the eleventh unknown code is still answered, and counted: %d", status)
	}
	if status, _ := get("/b/officecode/uplink"); status != http.StatusTooManyRequests {
		t.Fatalf("after eleven wrong codes in a minute: %d", status)
	}
}

// A machine that redeemed a code, never became a node and runs a new code
// (under another name, as nb260706069 did after cailiang-desktop) is the same
// machine starting over: it keeps its key. What it left of the first attempt
// is cleared when it redeems the second, and the new admission says so. An
// admission a node joined with, or one of a discovered machine, stays.
func TestRedeemClearsWhatTheSameMachineLeftOfAnUnfinishedJoin(t *testing.T) {
	code, key := "k7m2p9xq", strings.Repeat("A", 43)+"="
	digest := sha256.Sum256([]byte(code))
	now := time.Unix(1_800_000_000, 0)
	admission := func(name string, spec, status map[string]interface{}) *unstructured.Unstructured {
		spec["clusterID"] = "office"
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
			"metadata": map[string]interface{}{"name": name, "namespace": "agent-office"}, "spec": spec, "status": status,
		}}
	}
	management := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{nodeAdmissionGVR: "NodeAdmissionList", addonGVR: "AddonList"},
		admission("nb260706069", map[string]interface{}{"codeHash": hex.EncodeToString(digest[:]), "expiresAt": now.Add(time.Hour).UTC().Format(time.RFC3339)}, map[string]interface{}{}),
		admission("cailiang-desktop", map[string]interface{}{"codeHash": "x", "publicKey": key}, map[string]interface{}{"phase": "Redeemed", "address": "10.254.240.8"}),
		admission("joined-elsewhere", map[string]interface{}{"codeHash": "y", "publicKey": key}, map[string]interface{}{"phase": "Joined"}),
		admission("s1u-07-x86", map[string]interface{}{"codeHash": "z", "publicKey": key, "serial": "1423"}, map[string]interface{}{"phase": "Redeemed"}),
		admission("other-machine", map[string]interface{}{"codeHash": "w", "publicKey": strings.Repeat("B", 43) + "="}, map[string]interface{}{"phase": "Redeemed"}),
	)
	member := kubernetes.Interface(joinableCluster(true))
	b := &Bootstrap{Management: management, Now: func() time.Time { return now },
		Member: func(context.Context, string) (kubernetes.Interface, error) { return member, nil }}
	ctx := context.Background()
	if answer, status, err := b.Redeem(ctx, code, key); err != nil || status != http.StatusAccepted {
		t.Fatalf("redemption: %+v %d %v", answer, status, err)
	}
	resource := management.Resource(nodeAdmissionGVR).Namespace("agent-office")
	if _, err := resource.Get(ctx, "cailiang-desktop", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("the unfinished admission of the same machine must be cleared: %v", err)
	}
	for _, name := range []string{"joined-elsewhere", "s1u-07-x86", "other-machine"} {
		if _, err := resource.Get(ctx, name, metav1.GetOptions{}); err != nil {
			t.Fatalf("%s must stay: %v", name, err)
		}
	}
	redeemed, _ := resource.Get(ctx, "nb260706069", metav1.GetOptions{})
	if key, _, _ := unstructured.NestedString(redeemed.Object, "spec", "publicKey"); key == "" || !strings.Contains(redeemed.GetAnnotations()[SelfHealedAnnotation], "office/cailiang-desktop") {
		t.Fatalf("the redeemed admission must hold the key and say what was cleared: %v", redeemed.GetAnnotations())
	}
}

func TestBootstrapServesTheInstallersSHA256AndReadsTheFileAgainOnlyWhenItChanges(t *testing.T) {
	bundle := filepath.Join(t.TempDir(), "bundle.tar")
	if err := os.WriteFile(bundle, []byte("installer one"), 0644); err != nil {
		t.Fatal(err)
	}
	b := &Bootstrap{BundleFile: bundle}
	get := func() (int, string) {
		recorder := httptest.NewRecorder()
		b.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/bootstrap/bundle.sha256", nil))
		return recorder.Code, recorder.Body.String()
	}
	one := sha256.Sum256([]byte("installer one"))
	if status, body := get(); status != http.StatusOK || body != hex.EncodeToString(one[:])+"\n" {
		t.Fatalf("%d %q", status, body)
	}
	// The digest is kept for the file as it was: the same size and time give the same answer without reading it.
	stamp := b.bundleStamp
	b.bundleDigest = "kept"
	if _, body := get(); body != "kept\n" || b.bundleStamp != stamp {
		t.Fatalf("read again: %q", body)
	}
	later := time.Now().Add(time.Minute)
	if err := os.WriteFile(bundle, []byte("installer two!"), 0644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(bundle, later, later)
	two := sha256.Sum256([]byte("installer two!"))
	if _, body := get(); body != hex.EncodeToString(two[:])+"\n" {
		t.Fatalf("a new installer: %q", body)
	}
}
