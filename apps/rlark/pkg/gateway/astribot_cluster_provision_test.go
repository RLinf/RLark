package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	clientset "github.com/rlinf/rlark/api/kubeclients/clientset/versioned/fake"
	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/release"
	"github.com/rlinf/rlark/apps/rlark/astribot/stationmember"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// fakeMachine answers each script by the first line of it that a test names.
type fakeMachine struct {
	answers map[string]string
	ran     []string
	stdin   []string
	closed  bool
}

func (m *fakeMachine) Run(_ context.Context, script string, stdin io.Reader, line func(string)) (string, error) {
	m.ran = append(m.ran, script)
	if stdin != nil {
		data, _ := io.ReadAll(stdin)
		m.stdin = append(m.stdin, string(data))
	}
	for marker, answer := range m.answers {
		if strings.Contains(script, marker) {
			if line != nil {
				for _, l := range strings.Split(answer, "\n") {
					line(l)
				}
			}
			return answer, nil
		}
	}
	return "", errors.New("no answer")
}

func (m *fakeMachine) Close() error { m.closed = true; return nil }

func provisionTestNetwork(declared ...map[string]interface{}) *unstructured.Unstructured {
	status := []interface{}{}
	for _, d := range declared {
		status = append(status, d)
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "station.rlark.io/v1alpha1", "kind": "SiteNetwork",
		"metadata": map[string]interface{}{"name": "default", "namespace": "station-system"},
		"spec": map[string]interface{}{
			"pool":  "10.254.240.0/24",
			"sites": []interface{}{map[string]interface{}{"name": "office", "clusters": []interface{}{"rlark-local-dev-gpu"}, "mesh": true}},
			"names": []interface{}{
				map[string]interface{}{"name": "rlark-server", "address": "10.254.240.1"},
				map[string]interface{}{"name": originName, "address": "10.0.2.57"},
				map[string]interface{}{"name": "cp.rlark-robot-s1u-01.rlark.internal", "address": "10.254.240.2"},
			},
			"nodes": []interface{}{map[string]interface{}{"clusterID": "rlark-ksyun-gpu", "nodeName": "10-0-2-57", "role": "Hub", "address": "10.254.240.3",
				"advertise": []interface{}{map[string]interface{}{"cidr": "10.0.2.0/24"}}}},
		},
		"status": map[string]interface{}{
			"declared": status,
		},
	}}
}

func provisionTestRecord() *corev1.ConfigMap {
	images, _ := json.Marshal(map[string]string{
		"cluster-agent": "registry.rlark.internal:5443/rlark/agent@sha256:aaaa", "tools": "registry.rlark.internal:5443/rlark/agent@sha256:bbbb",
		"node-agent": "registry.rlark.internal:5443/rlark/agent@sha256:aaaa", "station-agent": "registry.rlark.internal:5443/rlark/station-agent@sha256:cccc",
		"station-wireguard": "registry.rlark.internal:5443/rlark/station-wireguard@sha256:dddd",
	})
	addons, _ := json.Marshal(map[string]releaseAddon{"registry-trust": {Version: "v0.2.3"}})
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: release.RecordName, Namespace: release.RecordNamespace},
		Data: map[string]string{
			"release": "live-test", "registry": "registry.rlark.internal:5443", "images.json": string(images), "addons.json": string(addons),
			releaseFileMemberRBAC: "kind: Namespace", releaseFileOperationRBAC: "kind: ServiceAccount", releaseFileStationAgent: "kind: DaemonSet", releaseFileReleaseMemberRBAC: "kind: ServiceAccount",
		},
	}
}

func provisionGateway(t *testing.T, network *unstructured.Unstructured, objects ...runtime.Object) *Gateway {
	t.Helper()
	g := &Gateway{
		kubeClient: clientset.NewSimpleClientset(objects...),
		rawClient:  kubefake.NewSimpleClientset(provisionTestRecord()),
		astribotState: astribotState{stationClient: dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{siteNetworkGVR: "SiteNetworkList", nodeAdmissionGVR: "NodeAdmissionList", networkEnrollmentGVR: "NetworkEnrollmentList"}, network,
			// A robot that joined by a code holds its tunnel address on its admission, not in the SiteNetwork.
			&unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
				"metadata": map[string]interface{}{"name": "s1u-01-x86", "namespace": "rlark-local-dev-gpu"}, "status": map[string]interface{}{"address": "10.254.240.4"}}})},
	}
	// The Gateway's own namespace is not the record's (live runs it in default).
	g.config.KubeClientConfig.Namespace = "default"
	g.config.ServerAddress = "https://rlark-server:8443"
	return g
}

func TestChooseSubnetsTakesTheFirstPairFreeOfEveryNetwork(t *testing.T) {
	taken := []netip.Prefix{netip.MustParsePrefix("10.200.0.0/16"), netip.MustParsePrefix("10.203.4.0/24"), netip.MustParsePrefix("10.0.0.0/8")}
	if _, _, err := chooseSubnets(taken); err == nil {
		t.Fatal("10.0.0.0/8 covers every candidate; want an error")
	}
	pod, service, err := chooseSubnets(taken[:2])
	if err != nil || pod != "10.204.0.0/16" || service != "10.205.0.0/16" {
		t.Fatalf("subnets = %s %s %v, want 10.204.0.0/16 10.205.0.0/16 (10.200 and 10.203 are taken)", pod, service, err)
	}
}

func TestAllocateTakesTheLowestFreeAddressAfterTheManagementMachine(t *testing.T) {
	network := provisionNetwork{provisionTestNetwork()}
	address, err := network.allocate([]string{"10.254.240.4"})
	if err != nil || address != "10.254.240.5" {
		t.Fatalf("address = %s %v, want 10.254.240.5 (.1 is the management machine; .2 a name, .3 a node, .4 another provision)", address, err)
	}
}

func TestPrecheckReadsTheMachineAndRefusesWhatIsMissing(t *testing.T) {
	g := provisionGateway(t, provisionTestNetwork())
	machine := &fakeMachine{answers: map[string]string{"echo \"hostname=": strings.Join([]string{
		"hostname=bd-gpu-1", "os=Ubuntu 22.04.5 LTS", "kernel=6.8.0", "systemd=yes", "tar=yes",
		"gpu=NVIDIA RTX PRO 6000", "gpu=NVIDIA RTX PRO 6000", "diskfree=" + "800000000000", "address=192.168.0.10",
		"route=192.168.0.0/24", "route=192.168.0.10/24", "route=127.0.0.1/8",
	}, "\n")}}
	run := &provisionRun{g: g, machine: machine}
	p := &rlarkv1alpha1.ClusterProvision{}
	readback, err := run.precheck(context.Background(), p)
	if err == nil || !strings.Contains(err.Error(), "wireguard") {
		t.Fatalf("err = %v, want the missing wireguard module named", err)
	}
	facts := p.Status.Facts
	if facts.NodeName != "bd-gpu-1" || facts.PrivateAddress != "192.168.0.10" || len(facts.GPUs) != 1 || facts.GPUs[0].Count != 2 || !facts.CacheVolume {
		t.Fatalf("facts = %+v", facts)
	}
	if strings.Join(facts.Routes, ",") != "192.168.0.0/24" {
		t.Fatalf("routes = %v, want the machine's own network once, without loopback", facts.Routes)
	}
	if !strings.Contains(readback, "2 × NVIDIA RTX PRO 6000") {
		t.Fatalf("readback = %q", readback)
	}
}

func TestBundleStreamsStationAPIsInstallerAndChecksWhatTheMachineHolds(t *testing.T) {
	g := provisionGateway(t, provisionTestNetwork())
	g.stationBootstrap = &fakeStationBootstrap{}
	key := strings.Repeat("k", 43) + "="
	ca := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	machine := &fakeMachine{answers: map[string]string{"installer.tar.new": "sha256=" + sha256Hex(fakeInstaller) + "\npublickey=" + key + "\nca=" + base64Std(ca)}}
	run := &provisionRun{g: g, machine: machine}
	p := &rlarkv1alpha1.ClusterProvision{}
	if _, err := run.bundle(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if machine.stdin[0] != fakeInstaller || p.Status.Facts.PublicKey != key || p.Status.Facts.RegistryCA != ca {
		t.Fatalf("stdin %q facts %+v", machine.stdin, p.Status.Facts)
	}
	if !strings.Contains(machine.ran[0], "certs.d/registry.rlark.internal:5443/ca.crt") {
		t.Fatal("the registry's CA is read from the installer by the release's registry")
	}
	if !strings.Contains(machine.ran[0], `[ "$sum" = "`+sha256Hex(fakeInstaller)+`" ]`) || !strings.Contains(machine.ran[0], "echo \"$sum\" > $D/bundle.sha256") {
		t.Fatal("the machine checks what it received against Station API's sha256 and marks the installer with it")
	}
	// A machine that holds Station API's installer already is not sent it again: it only makes its key.
	machine.ran, machine.stdin = nil, nil
	machine.answers["install -m 755 $D/bin/station-gateway"] = "publickey=" + key + "\nca=" + base64Std(ca)
	readback, err := run.bundle(context.Background(), p)
	if err != nil || len(machine.stdin) != 0 || strings.Contains(machine.ran[0], "installer.tar.new") || !strings.Contains(readback, "not sent again") {
		t.Fatalf("a machine with the installer: %v %q %q", err, machine.ran, readback)
	}
	delete(machine.answers, "install -m 755 $D/bin/station-gateway")
	p.Status.Facts.BundleSHA256 = ""
	machine.answers["installer.tar.new"] = "sha256=0000\npublickey=" + key + "\nca=" + base64Std(ca)
	if _, err := run.bundle(context.Background(), p); err == nil || !strings.Contains(err.Error(), "sha256 0000") {
		t.Fatalf("err = %v, want the installer's mismatch", err)
	}
}

func TestDeclareAddsTheSiteItsNamesAndTheHubKeepingTheRest(t *testing.T) {
	g := provisionGateway(t, provisionTestNetwork(map[string]interface{}{"clusterID": "rlark-baidu-gpu", "nodeName": "bd-gpu-1", "config": "{}"}))
	run := &provisionRun{g: g}
	p := &rlarkv1alpha1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{Name: "rlark-baidu-gpu"},
		Spec:       rlarkv1alpha1.ClusterProvisionSpec{Site: "baidu", Machine: rlarkv1alpha1.ClusterProvisionMachine{Address: "1.2.3.4", Port: 8023, User: "root", TunnelEndpoint: "1.2.3.4:8100"}},
		Status: rlarkv1alpha1.ClusterProvisionStatus{Facts: rlarkv1alpha1.ClusterProvisionFacts{
			NodeName: "bd-gpu-1", PrivateAddress: "192.168.0.10", PublicKey: "KEY", Routes: []string{"10.200.0.0/16"},
		}},
	}
	readback, err := run.declare(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	facts := p.Status.Facts
	if facts.TunnelAddress != "10.254.240.5" || facts.ControlPlaneName != "cp.rlark-baidu-gpu.rlark.internal" || facts.PodSubnet != "10.202.0.0/16" || facts.ServiceSubnet != "10.203.0.0/16" {
		t.Fatalf("facts = %+v (the admission's 10.254.240.4 and the machine's own 10.200.0.0/16 are avoided)", facts)
	}
	if !strings.Contains(readback, "status.declared") {
		t.Fatalf("readback = %q", readback)
	}
	network, _ := g.provisionNetwork(context.Background())
	if network.name(facts.ControlPlaneName) != "10.254.240.5" || network.name("rlark-server") != "10.254.240.1" || network.siteOf("rlark-baidu-gpu") != "baidu" || network.siteOf("rlark-local-dev-gpu") != "office" {
		t.Fatalf("names and sites = %v", network.object.Object["spec"])
	}
	sites := network.list("spec", "sites")
	if sites[1]["mesh"] != true {
		t.Fatalf("the new site %v is not a mesh: its later hubs would not reach the control plane on its first hub's tunnel", sites[1])
	}
	if names, _, _ := unstructured.NestedSlice(sites[1], "names"); len(names) != 1 || names[0].(map[string]interface{})["address"] != "10.0.2.57" {
		t.Fatalf("the new site's registry name = %v, want origin's address", names)
	}
	nodes := network.list("spec", "nodes")
	hub := nodes[1]
	// Behind one address its cloud forwards UDP 8100 to it: it listens there, other sites dial the address's 8100,
	// its own site its private address.
	endpoints := hub["endpoints"].([]interface{})
	if hub["nodeAddress"] != "192.168.0.10" || hub["publicKey"] != "KEY" || hub["role"] != "Hub" || hub["listenPort"] != int64(8100) || len(endpoints) != 2 ||
		endpoints[0].(map[string]interface{})["address"] != "1.2.3.4:8100" || endpoints[0].(map[string]interface{})["scope"] != "Public" ||
		endpoints[1].(map[string]interface{})["address"] != "192.168.0.10:8100" || endpoints[1].(map[string]interface{})["scope"] != "Site" {
		t.Fatalf("hub = %v", hub)
	}
	// Done again, nothing is added twice.
	if _, err := run.declare(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	network, _ = g.provisionNetwork(context.Background())
	if len(network.list("spec", "nodes")) != 2 || len(network.list("spec", "sites")) != 2 || len(network.list("spec", "names")) != 4 {
		t.Fatalf("declared twice: %v", network.object.Object["spec"])
	}
}

func TestDeclareRefusesASiteThatIsTaken(t *testing.T) {
	g := provisionGateway(t, provisionTestNetwork())
	p := &rlarkv1alpha1.ClusterProvision{ObjectMeta: metav1.ObjectMeta{Name: "rlark-office-gpu"}, Spec: rlarkv1alpha1.ClusterProvisionSpec{Site: "office"},
		Status: rlarkv1alpha1.ClusterProvisionStatus{Facts: rlarkv1alpha1.ClusterProvisionFacts{NodeName: "x", PodSubnet: "10.202.0.0/16"}}}
	if _, err := (&provisionRun{g: g}).declare(context.Background(), p); err == nil || !strings.Contains(err.Error(), "already has a site office") {
		t.Fatalf("err = %v", err)
	}
}

// withProvisionStages replaces the stages' work for a test.
func withProvisionStages(t *testing.T, stages map[string]func(*provisionRun, context.Context, *rlarkv1alpha1.ClusterProvision) (string, error)) {
	t.Helper()
	saved := provisionStageFuncs
	provisionStageFuncs = stages
	t.Cleanup(func() { provisionStageFuncs = saved })
}

func provisionCall(t *testing.T, g *Gateway, method, path, name, body string) (int, map[string]any) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set(authSubjectKey, "alice"); c.Next() })
	router.GET("/cluster-provisions/:name", g.handleGetClusterProvision)
	router.POST("/cluster-provisions", g.handleCreateClusterProvision)
	router.POST("/cluster-provisions/:name/retry", g.handleRetryClusterProvision)
	router.POST("/cluster-provisions/:name/clear", g.handleClearClusterProvision)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(method, path, bytes.NewBufferString(body)))
	var out map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &out)
	return recorder.Code, out
}

func waitProvision(t *testing.T, g *Gateway, name string) *rlarkv1alpha1.ClusterProvision {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		g.provisions.mu.Lock()
		running := g.provisions.running[name]
		g.provisions.mu.Unlock()
		if !running {
			p, err := g.kubeClient.RlinfV1alpha1().ClusterProvisions().Get(context.Background(), name, metav1.GetOptions{})
			if err != nil {
				t.Fatal(err)
			}
			return p
		}
	}
	t.Fatal("the making did not end")
	return nil
}

func TestAMakingStopsAtTheStageThatFailsAndARetryGoesOnFromIt(t *testing.T) {
	g := provisionGateway(t, provisionTestNetwork())
	var ran []string
	var passwords []string
	failAt := stageControlPlane
	stages := map[string]func(*provisionRun, context.Context, *rlarkv1alpha1.ClusterProvision) (string, error){}
	for _, name := range provisionStages {
		stages[name] = func(r *provisionRun, _ context.Context, p *rlarkv1alpha1.ClusterProvision) (string, error) {
			ran = append(ran, name)
			passwords = append(passwords, string(r.password))
			if name == stageDeclare {
				p.Status.Facts.TunnelAddress = "10.254.240.5"
			}
			if name == failAt {
				return "half", errors.New("kubeadm init failed: port 6443 is in use")
			}
			return name + " read back", nil
		}
	}
	withProvisionStages(t, stages)

	for _, refused := range []string{
		`{"site":"baidu","address":"1.2.3.4","user":"root","password":"s3cret","tunnel_endpoint":"1.2.3.4"}`,
		`{"site":"baidu","address":"gpu-1.example","user":"root","password":"s3cret"}`,
	} {
		if code, body := provisionCall(t, g, http.MethodPost, "/cluster-provisions", "", refused); code != http.StatusBadRequest {
			t.Fatalf("create %s = %d %v, want 400: a tunnel endpoint is an IPv4 ip:port", refused, code, body)
		}
	}
	code, body := provisionCall(t, g, http.MethodPost, "/cluster-provisions", "", `{"site":"baidu","address":"1.2.3.4","user":"root","password":"s3cret"}`)
	if code != http.StatusCreated {
		t.Fatalf("create = %d %v", code, body)
	}
	p := waitProvision(t, g, "rlark-baidu-gpu")
	if p.Spec.Machine.TunnelEndpoint != "1.2.3.4:51820" {
		t.Fatalf("tunnel endpoint %q, want the address at 51820", p.Spec.Machine.TunnelEndpoint)
	}
	if p.Status.Phase != rlarkv1alpha1.ClusterProvisionFailed || p.Status.Stage != stageControlPlane || p.Status.Facts.TunnelAddress != "10.254.240.5" {
		t.Fatalf("status = %+v", p.Status)
	}
	stage := p.Status.Stages[3]
	if stage.State != rlarkv1alpha1.ClusterProvisionStageFailed || stage.Message != "kubeadm init failed: port 6443 is in use" || stage.Readback != "half" || p.Status.Stages[2].State != rlarkv1alpha1.ClusterProvisionStageDone || p.Status.Stages[4].State != rlarkv1alpha1.ClusterProvisionStagePending {
		t.Fatalf("stages = %+v", p.Status.Stages)
	}
	stored, _ := json.Marshal(p)
	if strings.Contains(string(stored), "s3cret") || p.Spec.RequestedBy != "alice" {
		t.Fatalf("the password is in the object, or the requester is not: %s", stored)
	}

	if code, _ := provisionCall(t, g, http.MethodPost, "/cluster-provisions/rlark-baidu-gpu/retry", "", `{}`); code != http.StatusBadRequest {
		t.Fatalf("a retry of a stage that logs in without the password = %d, want 400", code)
	}
	failAt, ran, passwords = "", nil, nil
	if code, body := provisionCall(t, g, http.MethodPost, "/cluster-provisions/rlark-baidu-gpu/retry", "", `{"password":"s3cret","user":"ops"}`); code != http.StatusAccepted {
		t.Fatalf("retry = %d %v", code, body)
	}
	p = waitProvision(t, g, "rlark-baidu-gpu")
	if p.Spec.Machine.User != "ops" {
		t.Fatalf("the retry's account is %q, want ops", p.Spec.Machine.User)
	}
	if p.Status.Phase != rlarkv1alpha1.ClusterProvisionSucceeded || strings.Join(ran, ",") != "control-plane,agent,station,addons,accept" || passwords[0] != "s3cret" {
		t.Fatalf("phase %s ran %v", p.Status.Phase, ran)
	}
	for _, stage := range p.Status.Stages {
		if stage.State != rlarkv1alpha1.ClusterProvisionStageDone || stage.Readback != stage.Name+" read back" {
			t.Fatalf("stage %+v", stage)
		}
	}
	if code, _ := provisionCall(t, g, http.MethodPost, "/cluster-provisions/rlark-baidu-gpu/retry", "", `{"password":"x"}`); code != http.StatusConflict {
		t.Fatalf("a retry of a finished making = %d, want 409", code)
	}
}

func TestCreateRefusesATakenSiteOrClusterAndAMissingRelease(t *testing.T) {
	g := provisionGateway(t, provisionTestNetwork(), &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "rlark-ksyun-gpu"}})
	for body, want := range map[string]int{
		`{"site":"office","address":"1.2.3.4","user":"root","password":"p"}`:                  http.StatusConflict,
		`{"site":"ksyun","address":"1.2.3.4","user":"root","password":"p"}`:                   http.StatusConflict,
		`{"site":"baidu","cluster":"baidu","address":"1.2.3.4","user":"root","password":"p"}`: http.StatusBadRequest,
		`{"site":"baidu","address":"1.2.3.4","user":"root"}`:                                  http.StatusBadRequest,
		`{"site":"Baidu","address":"1.2.3.4","user":"root","password":"p"}`:                   http.StatusBadRequest,
		`{"site":"baidu","address":"root@1.2.3.4","user":"root","password":"p"}`:              http.StatusBadRequest,
	} {
		if code, out := provisionCall(t, g, http.MethodPost, "/cluster-provisions", "", body); code != want {
			t.Errorf("%s = %d %v, want %d", body, code, out, want)
		}
	}
	_ = g.rawClient.CoreV1().ConfigMaps(release.RecordNamespace).Delete(context.Background(), release.RecordName, metav1.DeleteOptions{})
	if code, out := provisionCall(t, g, http.MethodPost, "/cluster-provisions", "", `{"site":"baidu","address":"1.2.3.4","user":"root","password":"p"}`); code != http.StatusServiceUnavailable || !strings.Contains(out["message"].(string), "release record") {
		t.Fatalf("without a release record = %d %v", code, out)
	}
}

func TestAMakingRunningWhenTheGatewayStartsIsMarkedStopped(t *testing.T) {
	status := newProvisionStatus()
	setStage(&status.Stages, &status.Stage, stageAgent, rlarkv1alpha1.ClusterProvisionStageRunning, "", "")
	g := provisionGateway(t, provisionTestNetwork(), &rlarkv1alpha1.ClusterProvision{ObjectMeta: metav1.ObjectMeta{Name: "rlark-baidu-gpu"}, Status: status})
	if err := g.interruptProvisions(context.Background()); err != nil {
		t.Fatal(err)
	}
	p, _ := g.kubeClient.RlinfV1alpha1().ClusterProvisions().Get(context.Background(), "rlark-baidu-gpu", metav1.GetOptions{})
	if p.Status.Phase != rlarkv1alpha1.ClusterProvisionFailed || p.Status.Stage != stageAgent || !strings.Contains(p.Status.Stages[4].Message, "Gateway restarted") {
		t.Fatalf("status = %+v", p.Status)
	}
}

func TestStationMemberSecretNameIsStationControllers(t *testing.T) {
	// The name station-controller looks a cluster's credentials up by (base32 of the first 10 bytes of its SHA-256).
	if got := stationmember.SecretName("rlark-baidu-gpu"); got != "station-member-accwi2jhtyczaq3d" {
		t.Fatalf("name = %s", got)
	}
}

func TestControlPlaneScriptGivesTheInstallerTheChoicesAndTheCacheVolume(t *testing.T) {
	facts := rlarkv1alpha1.ClusterProvisionFacts{NodeName: "bd-gpu-1", ControlPlaneName: "cp.rlark-baidu-gpu.rlark.internal", PodSubnet: "10.202.0.0/16", ServiceSubnet: "10.203.0.0/16"}
	script := controlPlaneScript(facts)
	if !strings.Contains(script, "CONTROL_PLANE_NAME=cp.rlark-baidu-gpu.rlark.internal POD_SUBNET=10.202.0.0/16 SERVICE_SUBNET=10.203.0.0/16 NODE_NAME=bd-gpu-1 WORKLOADS=yes ./station-bootstrap control-plane") || strings.Contains(script, "NODE_VOLUMES") {
		t.Fatal(script)
	}
	facts.CacheVolume = true
	if !strings.Contains(controlPlaneScript(facts), "NODE_VOLUMES="+provisionCacheVolume) {
		t.Fatal("a machine with room for the cache gets its volume")
	}
}

func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func base64Std(text string) string { return base64.StdEncoding.EncodeToString([]byte(text)) }

// A making that stopped half way holds the site, the cluster's name and the machine, as nb260706069's half join held
// its key (11-16): a new making of the name is refused and says what is left. Clearing it puts the machine back
// (the installer's own undo, the tunnel), deletes the Cluster it listed and the credentials, takes its declarations
// out of the SiteNetwork keeping every other, and deletes the record; the name can then be made again.
func TestAMakingThatStoppedIsClearedAndItsNameMadeAgain(t *testing.T) {
	network := provisionTestNetwork()
	spec := network.Object["spec"].(map[string]interface{})
	spec["sites"] = append(spec["sites"].([]interface{}), map[string]interface{}{"name": "baidu", "clusters": []interface{}{"rlark-baidu-gpu"}})
	spec["names"] = append(spec["names"].([]interface{}), map[string]interface{}{"name": "cp.rlark-baidu-gpu.rlark.internal", "address": "10.254.240.6"})
	spec["nodes"] = append(spec["nodes"].([]interface{}), map[string]interface{}{"clusterID": "rlark-baidu-gpu", "nodeName": "gpu-1", "role": "Hub", "address": "10.254.240.6", "publicKey": "key"})
	stopped := &rlarkv1alpha1.ClusterProvision{
		ObjectMeta: metav1.ObjectMeta{Name: "rlark-baidu-gpu"},
		Spec:       rlarkv1alpha1.ClusterProvisionSpec{Site: "baidu", Machine: rlarkv1alpha1.ClusterProvisionMachine{Address: "1.2.3.4", Port: 22, User: "root"}},
		Status: rlarkv1alpha1.ClusterProvisionStatus{Phase: rlarkv1alpha1.ClusterProvisionFailed, Stage: stageAgent, HostKey: "SHA256:first",
			Facts: rlarkv1alpha1.ClusterProvisionFacts{NodeName: "gpu-1", TunnelAddress: "10.254.240.6"},
			Stages: []rlarkv1alpha1.ClusterProvisionStage{{Name: stagePrecheck, State: rlarkv1alpha1.ClusterProvisionStageDone}, {Name: stageBundle, State: rlarkv1alpha1.ClusterProvisionStageDone},
				{Name: stageAgent, State: rlarkv1alpha1.ClusterProvisionStageFailed}}},
	}
	g := provisionGateway(t, network, stopped, &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "rlark-baidu-gpu"}})
	ctx := context.Background()
	_, _ = g.rawClient.CoreV1().Secrets(stationMemberCredentialNamespace).Create(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: stationmember.SecretName("rlark-baidu-gpu"), Namespace: stationMemberCredentialNamespace}}, metav1.CreateOptions{})
	for _, object := range []*unstructured.Unstructured{
		{Object: map[string]interface{}{"apiVersion": "station.rlark.io/v1alpha1", "kind": "NetworkEnrollment", "metadata": map[string]interface{}{"name": "gpu-1", "namespace": "rlark-baidu-gpu"},
			"spec": map[string]interface{}{"nodeUID": "earlier"}}},
		{Object: map[string]interface{}{"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission", "metadata": map[string]interface{}{"name": "gpu-2", "namespace": "rlark-baidu-gpu"},
			"spec": map[string]interface{}{"publicKey": "second"}}},
	} {
		resource := networkEnrollmentGVR
		if object.GetKind() == "NodeAdmission" {
			resource = nodeAdmissionGVR
		}
		if _, err := g.stationClient.Resource(resource).Namespace("rlark-baidu-gpu").Create(ctx, object, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	machine := &fakeMachine{answers: map[string]string{"control-plane undo": "12:00:00 the control plane is removed\nDone.\ncleared=yes"}}
	var shown string
	g.provisionDial = func(_ context.Context, _ rlarkv1alpha1.ClusterProvisionMachine, _ []byte, hostKey string) (provisionMachine, string, error) {
		shown = hostKey
		return machine, hostKey, nil
	}

	if code, out := provisionCall(t, g, http.MethodPost, "/cluster-provisions", "", `{"site":"baidu","address":"1.2.3.4","user":"root","password":"p"}`); code != http.StatusConflict || !strings.Contains(out["message"].(string), "clear it") {
		t.Fatalf("a new making over one that is left = %d %v", code, out)
	}
	if code, _ := provisionCall(t, g, http.MethodPost, "/cluster-provisions/rlark-baidu-gpu/clear", "", `{}`); code != http.StatusBadRequest {
		t.Fatalf("clearing a making that reached the machine, without its password = %d, want 400", code)
	}
	code, out := provisionCall(t, g, http.MethodPost, "/cluster-provisions/rlark-baidu-gpu/clear", "", `{"password":"p"}`)
	if code != http.StatusOK || len(out["steps"].([]interface{})) != 6 {
		t.Fatalf("clear = %d %v", code, out)
	}
	if shown != "SHA256:first" || len(machine.ran) != 1 || !machine.closed {
		t.Fatalf("the machine: host key %q, scripts %d, closed %v", shown, len(machine.ran), machine.closed)
	}
	if _, err := g.kubeClient.RlinfV1alpha1().Clusters().Get(ctx, "rlark-baidu-gpu", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("the Cluster the making listed: %v", err)
	}
	if _, err := g.rawClient.CoreV1().Secrets(stationMemberCredentialNamespace).Get(ctx, stationmember.SecretName("rlark-baidu-gpu"), metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("Station's member credential: %v", err)
	}
	if left, _ := g.stationClient.Resource(networkEnrollmentGVR).Namespace("rlark-baidu-gpu").List(ctx, metav1.ListOptions{}); len(left.Items) != 0 {
		t.Fatalf("the cluster's site node records must go: %d", len(left.Items))
	}
	if left, _ := g.stationClient.Resource(nodeAdmissionGVR).Namespace("rlark-baidu-gpu").List(ctx, metav1.ListOptions{}); len(left.Items) != 0 {
		t.Fatalf("the cluster's admissions must go: %d", len(left.Items))
	}
	if _, err := g.stationClient.Resource(nodeAdmissionGVR).Namespace("rlark-local-dev-gpu").Get(ctx, "s1u-01-x86", metav1.GetOptions{}); err != nil {
		t.Fatalf("another cluster's admission must stay: %v", err)
	}
	after, err := g.provisionNetwork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.hasSite("baidu") || after.name("cp.rlark-baidu-gpu.rlark.internal") != "" || len(after.nodes()) != 1 || !after.hasSite("office") || after.name(originName) == "" {
		t.Fatalf("the SiteNetwork after clearing: %v", after.object.Object["spec"])
	}
	if _, err := g.kubeClient.RlinfV1alpha1().ClusterProvisions().Get(ctx, "rlark-baidu-gpu", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
		t.Fatalf("the record of the making: %v", err)
	}
}

// A cluster that was made and is listed is not cleared from under it; a machine that is gone is not logged in to.
func TestClearingLeavesAListedClusterAndAMachineThatIsGone(t *testing.T) {
	made := &rlarkv1alpha1.ClusterProvision{ObjectMeta: metav1.ObjectMeta{Name: "rlark-baidu-gpu"}, Spec: rlarkv1alpha1.ClusterProvisionSpec{Site: "baidu"},
		Status: rlarkv1alpha1.ClusterProvisionStatus{Phase: rlarkv1alpha1.ClusterProvisionSucceeded, Stages: []rlarkv1alpha1.ClusterProvisionStage{{Name: stageBundle, State: rlarkv1alpha1.ClusterProvisionStageDone}}}}
	g := provisionGateway(t, provisionTestNetwork(), made, &rlarkv1alpha1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "rlark-baidu-gpu"}})
	g.provisionDial = func(context.Context, rlarkv1alpha1.ClusterProvisionMachine, []byte, string) (provisionMachine, string, error) {
		t.Fatal("a machine that is gone is not logged in to")
		return nil, "", nil
	}
	if code, out := provisionCall(t, g, http.MethodPost, "/cluster-provisions/rlark-baidu-gpu/clear", "", `{"machine_gone":true}`); code != http.StatusConflict {
		t.Fatalf("clearing a listed cluster's making = %d %v, want 409", code, out)
	}
	_ = g.kubeClient.RlinfV1alpha1().Clusters().Delete(context.Background(), "rlark-baidu-gpu", metav1.DeleteOptions{})
	if code, out := provisionCall(t, g, http.MethodPost, "/cluster-provisions/rlark-baidu-gpu/clear", "", `{"machine_gone":true}`); code != http.StatusOK {
		t.Fatalf("clearing what a deleted cluster's making left = %d %v", code, out)
	}
}
