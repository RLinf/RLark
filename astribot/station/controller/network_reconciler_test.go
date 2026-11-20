package controller

import (
	"context"
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	"github.com/rlinf/rlark/astribot/station/network"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

var testNetworkKey = types.NamespacedName{Namespace: "station-system", Name: "default"}

func testSiteNetwork() *stationv1alpha1.SiteNetwork {
	return &stationv1alpha1.SiteNetwork{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNetworkKey.Namespace, Name: testNetworkKey.Name, Generation: 1},
		Spec: stationv1alpha1.SiteNetworkSpec{
			Mode: stationv1alpha1.SiteNetworkModeTunnel, Pool: "10.254.240.0/24", MTU: 1380, Interface: "st-wg0", ListenPort: 51820, KeepaliveSeconds: 25,
			Sites: []stationv1alpha1.SiteNetworkSite{
				{Name: "office", Clusters: []string{"dev"}, Mesh: true},
				{Name: "lab", Clusters: []string{"robot-01", "robot-02"}, Reaches: []string{"office"}},
			},
			Nodes: []stationv1alpha1.SiteNetworkNode{
				{ClusterID: "dev", NodeName: "cailiang", Role: "Hub", Relay: true, Address: "10.254.240.1",
					Endpoints: []stationv1alpha1.SiteNetworkEndpoint{{Address: "10.11.14.128:51820", Scope: "Site"}}},
				{ClusterID: "dev", NodeName: "alexfu", Role: "Hub", Address: "10.254.240.2", Endpoints: []stationv1alpha1.SiteNetworkEndpoint{{Scope: "Site"}}},
				{ClusterID: "robot-01", NodeName: "s1u-01-x86", Address: "10.254.240.3"},
				{ClusterID: "robot-02", NodeName: "s1u-02-x86", Address: "10.254.240.4"},
			},
		},
	}
}

// testMember returns the NetworkEnrollment of one node.
func testMember(cluster, node, key string, addresses ...string) []client.Object {
	return []client.Object{&stationv1alpha1.NetworkEnrollment{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-" + cluster, Name: node, Generation: 1},
		Spec:       stationv1alpha1.NetworkEnrollmentSpec{ClusterID: cluster, NodeUID: "node-" + node, PublicKey: key, Addresses: addresses},
	}}
}

func newNetworkTest(t *testing.T, objects ...client.Object) (*NetworkReconciler, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := stationv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&stationv1alpha1.SiteNetwork{}, &stationv1alpha1.NetworkEnrollment{}, &stationv1alpha1.NodeAdmission{}).
		WithObjects(objects...).Build()
	reconciler := NewNetworkReconciler(c, c, testNetworkKey, func() time.Time { return time.Unix(1_800_000_000, 0) })
	return reconciler, c
}

func reconcileNetwork(t *testing.T, r *NetworkReconciler) {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: testNetworkKey}); err != nil {
		t.Fatal(err)
	}
}

func enrollmentStatus(t *testing.T, c client.Client, cluster, node string) stationv1alpha1.NetworkEnrollmentStatus {
	t.Helper()
	var enrollment stationv1alpha1.NetworkEnrollment
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns-" + cluster, Name: node}, &enrollment); err != nil {
		t.Fatal(err)
	}
	return enrollment.Status
}

func configOf(t *testing.T, status stationv1alpha1.NetworkEnrollmentStatus) network.NodeConfig {
	t.Helper()
	var config network.NodeConfig
	if err := json.Unmarshal([]byte(status.Config), &config); err != nil {
		t.Fatalf("config %q: %v", status.Config, err)
	}
	return config
}

func TestNetworkEnrollsNodesAtTheirDeclaredAddresses(t *testing.T) {
	objects := []client.Object{testSiteNetwork()}
	objects = append(objects, testMember("dev", "cailiang", "key-dev", "10.11.14.128")...)
	objects = append(objects, testMember("dev", "alexfu", "key-3090", "10.11.13.192")...)
	objects = append(objects, testMember("robot-01", "s1u-01-x86", "key-robot-1", "192.168.0.10")...)
	r, c := newNetworkTest(t, objects...)
	reconcileNetwork(t, r)

	dev := enrollmentStatus(t, c, "dev", "cailiang")
	desk := enrollmentStatus(t, c, "dev", "alexfu")
	robot := enrollmentStatus(t, c, "robot-01", "s1u-01-x86")
	if dev.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || dev.Address != "10.254.240.1" || dev.Role != "Hub" || dev.Site != "office" {
		t.Fatalf("declared hub: %+v", dev)
	}
	if desk.Address != "10.254.240.2" || robot.Address != "10.254.240.3" || robot.Role != "Leaf" || robot.Site != "lab" {
		t.Fatalf("declared %s and %s, robot %+v", desk.Address, robot.Address, robot)
	}
	robotConfig := configOf(t, robot)
	if len(robotConfig.Peers) != 2 || robotConfig.Peers[0].Endpoint != "10.11.14.128:51820" || robotConfig.Peers[1].Endpoint != "10.11.13.192:51820" {
		t.Fatalf("the robot dials both hubs, the second at the address it reported: %+v", robotConfig.Peers)
	}
	if robotConfig.Peers[0].AllowedIPs[1] != "10.254.240.0/24" {
		t.Fatalf("the relay carries the pool: %+v", robotConfig.Peers[0])
	}

	// A second robot joins: one new object, no edit anywhere else.
	for _, object := range testMember("robot-02", "s1u-02-x86", "key-robot-2", "192.168.0.10") {
		if err := c.Create(context.Background(), object); err != nil {
			t.Fatal(err)
		}
	}
	reconcileNetwork(t, r)
	second := enrollmentStatus(t, c, "robot-02", "s1u-02-x86")
	if second.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || second.Address != "10.254.240.4" {
		t.Fatalf("second robot: %+v", second)
	}
	if got := configOf(t, enrollmentStatus(t, c, "dev", "cailiang")); len(got.Peers) != 3 {
		t.Fatalf("the hub learns the new robot: %+v", got.Peers)
	}
	if unchanged := enrollmentStatus(t, c, "robot-01", "s1u-01-x86"); unchanged.ConfigHash != robot.ConfigHash {
		t.Fatal("a leaf's configuration must not change when another leaf joins")
	}
	var siteNetwork stationv1alpha1.SiteNetwork
	if err := c.Get(context.Background(), testNetworkKey, &siteNetwork); err != nil {
		t.Fatal(err)
	}
	if siteNetwork.Status.Members != 4 || len(siteNetwork.Status.Unreachable) != 0 {
		t.Fatalf("status %+v", siteNetwork.Status)
	}

	// The DHCP address of the 3090 changes; the robot follows it.
	var enrollment stationv1alpha1.NetworkEnrollment
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns-dev", Name: "alexfu"}, &enrollment); err != nil {
		t.Fatal(err)
	}
	enrollment.Spec.Addresses = []string{"10.11.13.77"}
	if err := c.Update(context.Background(), &enrollment); err != nil {
		t.Fatal(err)
	}
	reconcileNetwork(t, r)
	if got := configOf(t, enrollmentStatus(t, c, "robot-01", "s1u-01-x86")); got.Peers[1].Endpoint != "10.11.13.77:51820" {
		t.Fatalf("endpoint did not follow the reported address: %+v", got.Peers[1])
	}
}

// A node in Conflict, a removed one and one with no address source have no
// place in the tunnel; the answer leaves the report and the node's state alone.
func TestNetworkRejectsAConflictingARetiredAndAnAddresslessNode(t *testing.T) {
	objects := []client.Object{testSiteNetwork()}
	objects = append(objects, testMember("dev", "cailiang", "key-dev", "10.11.14.128")...)
	objects = append(objects, testMember("robot-01", "s1u-01-x86", "key-robot-1")...)
	objects = append(objects, testMember("robot-02", "s1u-02-x86", "key-robot-2")...)
	objects = append(objects, testMember("robot-02", "undeclared", "key-robot-3")...)
	r, c := newNetworkTest(t, objects...)
	reconcileNetwork(t, r)
	if got := enrollmentStatus(t, c, "robot-01", "s1u-01-x86"); got.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || got.Address != "10.254.240.3" {
		t.Fatalf("first robot: %+v", got)
	}
	if got := enrollmentStatus(t, c, "robot-02", "undeclared"); got.Phase != stationv1alpha1.NetworkEnrollmentRejected || !strings.Contains(got.Reason, "node admission") {
		t.Fatalf("a node with no address source: %+v", got)
	}

	var enrollment stationv1alpha1.NetworkEnrollment
	key := types.NamespacedName{Namespace: "ns-robot-01", Name: "s1u-01-x86"}
	if err := c.Get(context.Background(), key, &enrollment); err != nil {
		t.Fatal(err)
	}
	enrollment.Status.Node = &stationv1alpha1.NetworkEnrollmentNode{State: stationv1alpha1.NodeStateConflict}
	enrollment.Status.Observation = &stationv1alpha1.NodeObservation{NodeUID: "node-reinstalled"}
	if err := c.Status().Update(context.Background(), &enrollment); err != nil {
		t.Fatal(err)
	}
	reconcileNetwork(t, r)
	conflict := enrollmentStatus(t, c, "robot-01", "s1u-01-x86")
	if conflict.Phase != stationv1alpha1.NetworkEnrollmentRejected || conflict.Config != "" || conflict.Node.State != stationv1alpha1.NodeStateConflict || conflict.Observation.NodeUID != "node-reinstalled" {
		t.Fatalf("a node in Conflict must be rejected, its report and state kept: %+v", conflict)
	}
	if got := configOf(t, enrollmentStatus(t, c, "dev", "cailiang")); len(got.Peers) != 1 {
		t.Fatalf("its key must leave every peer list: %+v", got.Peers)
	}

	// The record passes to the reinstalled node, which enrolls with a new key at the same address.
	if err := c.Get(context.Background(), key, &enrollment); err != nil {
		t.Fatal(err)
	}
	enrollment.Status.Node.State = stationv1alpha1.NodeStateBound
	if err := c.Status().Update(context.Background(), &enrollment); err != nil {
		t.Fatal(err)
	}
	enrollment.Spec.NodeUID, enrollment.Spec.PublicKey = "node-reinstalled", "key-robot-1-new"
	if err := c.Update(context.Background(), &enrollment); err != nil {
		t.Fatal(err)
	}
	reconcileNetwork(t, r)
	if again := enrollmentStatus(t, c, "robot-01", "s1u-01-x86"); again.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || again.Address != "10.254.240.3" || again.PublicKey != "key-robot-1-new" {
		t.Fatalf("the node the record passed to: %+v", again)
	}

	if err := c.Get(context.Background(), key, &enrollment); err != nil {
		t.Fatal(err)
	}
	enrollment.Spec.Retired = true
	if err := c.Update(context.Background(), &enrollment); err != nil {
		t.Fatal(err)
	}
	reconcileNetwork(t, r)
	if retired := enrollmentStatus(t, c, "robot-01", "s1u-01-x86"); retired.Phase != stationv1alpha1.NetworkEnrollmentRejected || !strings.Contains(retired.Reason, "removed") {
		t.Fatalf("a removed node: %+v", retired)
	}
}

func TestNetworkWithoutSiteOrKey(t *testing.T) {
	objects := []client.Object{testSiteNetwork()}
	objects = append(objects, testMember("elsewhere", "stranger", "key-x")...)
	objects = append(objects, testMember("robot-01", "s1u-01-x86", "")...)
	r, c := newNetworkTest(t, objects...)
	reconcileNetwork(t, r)
	if got := enrollmentStatus(t, c, "elsewhere", "stranger"); got.Phase != stationv1alpha1.NetworkEnrollmentRejected || !strings.Contains(got.Reason, "not in any site") {
		t.Fatalf("unknown cluster: %+v", got)
	}
	if got := enrollmentStatus(t, c, "robot-01", "s1u-01-x86"); got.Phase != stationv1alpha1.NetworkEnrollmentPending {
		t.Fatalf("no key yet: %+v", got)
	}
}

// With a routed private line the registry stays and the tunnel goes.
func TestNetworkRoutedModeKeepsRegistryWithoutTunnel(t *testing.T) {
	siteNetwork := testSiteNetwork()
	siteNetwork.Spec.Mode = stationv1alpha1.SiteNetworkModeRouted
	objects := []client.Object{siteNetwork}
	objects = append(objects, testMember("robot-01", "s1u-01-x86", "", "10.20.0.31")...)
	r, c := newNetworkTest(t, objects...)
	reconcileNetwork(t, r)
	got := enrollmentStatus(t, c, "robot-01", "s1u-01-x86")
	if got.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || got.Address != "10.20.0.31" || got.Config != "" || got.TunnelAddress != "" {
		t.Fatalf("routed member: %+v", got)
	}
}

// The declaration shipped for the live site must enroll all six nodes of
// 2026-11-12 with the addresses and peers they have today.
func TestLiveSiteDeclarationEnrollsTheSixNodes(t *testing.T) {
	data, err := os.ReadFile("testdata/site-network.live-20261112.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var siteNetwork stationv1alpha1.SiteNetwork
	if err := yaml.UnmarshalStrict(data, &siteNetwork); err != nil {
		t.Fatal(err)
	}
	objects := []client.Object{&siteNetwork}
	live := []struct {
		cluster, node, key, reported, address string
		peers                                 int
	}{
		{"rlark-local-dev-gpu", "cailiang", "DwjZS7W8Yab4SZN62rtBuvKzrzWwoa6nHwObl1Ck2XM=", "10.11.14.128", "10.254.240.1", 5},
		{"rlark-ksyun-gpu", "10-0-2-57", "ixGRrgUmo9UMpnCg00v3DPbRT2NEqvmm6eI/lfzpEEc=", "10.0.2.57", "10.0.2.57", 4},
		{"rlark-robot-s1u-01", "s1u-01-x86", "YxGnfRDiE+94DLi/fCMeVe3zl73UTudA9apw8/BWjXs=", "192.168.31.250", "10.254.240.3", 5},
		{"rlark-ksyun-gpu", "10-0-2-196", "5Go4sNJOoyAwxL5PXdMYnVHRJJQWkNzdBRUKDWzDzS0=", "10.0.2.196", "10.0.2.196", 4},
		{"rlark-local-dev-gpu", "alexfu", "Rpk1Y6lpJXIGOdNcgLMYs0J6mBG+67d85HVJs+upaR4=", "10.11.13.192", "10.254.240.5", 5},
		{"rlark-local-dev-gpu", "wendychen", "uaC5QIh01DyQvTqCObM2dl4hcMIedM5lPYIhkrcQjQQ=", "10.11.14.135", "10.254.240.6", 5},
	}
	for _, node := range live {
		objects = append(objects, testMember(node.cluster, node.node, node.key, node.reported)...)
	}
	r, c := newNetworkTest(t, objects...)
	reconcileNetwork(t, r)
	for _, node := range live {
		status := enrollmentStatus(t, c, node.cluster, node.node)
		if status.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || status.Address != node.address || len(status.Problems) != 0 {
			t.Errorf("%s: %+v", node.node, status)
			continue
		}
		if config := configOf(t, status); len(config.Peers) != node.peers {
			t.Errorf("%s has %d peers, want %d", node.node, len(config.Peers), node.peers)
		}
	}
	if err := c.Get(context.Background(), testNetworkKey, &siteNetwork); err != nil {
		t.Fatal(err)
	}
	if len(siteNetwork.Status.Unreachable) != 0 || siteNetwork.Status.Members != 6 {
		t.Fatalf("status %+v", siteNetwork.Status)
	}
}

// A cloud node cannot reach Station API before its tunnel is up: declaring its
// key puts it into its peers' configurations first, and its own enrollment
// takes over once it arrives.
func TestNetworkAdmitsDeclaredKeyBeforeEnrollment(t *testing.T) {
	siteNetwork := testSiteNetwork()
	siteNetwork.Spec.Sites = append(siteNetwork.Spec.Sites, stationv1alpha1.SiteNetworkSite{Name: "ksyun", Clusters: []string{"cloud"}})
	siteNetwork.Spec.Nodes = append(siteNetwork.Spec.Nodes, stationv1alpha1.SiteNetworkNode{
		ClusterID: "cloud", NodeName: "h100-3", Role: "Hub", Address: "10.254.240.9", NodeAddress: "10.0.2.9", PublicKey: "key-cloud",
		Endpoints: []stationv1alpha1.SiteNetworkEndpoint{{Address: "36.103.180.169:51822", Scope: "Public"}},
	})
	objects := append([]client.Object{siteNetwork}, testMember("dev", "cailiang", "key-dev", "10.11.14.128")...)
	r, c := newNetworkTest(t, objects...)
	reconcileNetwork(t, r)
	entry := configOf(t, enrollmentStatus(t, c, "dev", "cailiang"))
	if len(entry.Peers) != 1 || entry.Peers[0].PublicKey != "key-cloud" || entry.Peers[0].Endpoint != "36.103.180.169:51822" || entry.Peers[0].AllowedIPs[1] != "10.0.2.9/32" {
		t.Fatalf("the site entry must dial the declared node: %+v", entry.Peers)
	}
	// The declared node's own document is the network's, published for the machine that cannot ask for it yet. The
	// entry's endpoint is of its own site: the entry dials the declared node's public one, not the other way round.
	var declared stationv1alpha1.SiteNetwork
	if err := c.Get(context.Background(), testNetworkKey, &declared); err != nil || len(declared.Status.Declared) != 1 {
		t.Fatalf("declared documents: %+v, %v", declared.Status.Declared, err)
	}
	own := declared.Status.Declared[0]
	var first network.NodeConfig
	if err := json.Unmarshal([]byte(own.Config), &first); err != nil || own.ClusterID != "cloud" || own.NodeName != "h100-3" || own.ConfigHash == "" ||
		first.Address != "10.254.240.9/32" || len(first.Peers) != 1 || first.Peers[0].PublicKey != "key-dev" || first.Peers[0].Endpoint != "" {
		t.Fatalf("the declared node's first document: %+v %+v, %v", own, first, err)
	}

	// The node enrolls through the tunnel with a rotated key: its enrollment wins.
	for _, object := range testMember("cloud", "h100-3", "key-cloud-rotated", "10.0.2.9") {
		if err := c.Create(context.Background(), object); err != nil {
			t.Fatal(err)
		}
	}
	reconcileNetwork(t, r)
	cloud := enrollmentStatus(t, c, "cloud", "h100-3")
	if cloud.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || cloud.TunnelAddress != "10.254.240.9" || cloud.Address != "10.0.2.9" {
		t.Fatalf("enrolled cloud node: %+v", cloud)
	}
	if entry = configOf(t, enrollmentStatus(t, c, "dev", "cailiang")); len(entry.Peers) != 1 || entry.Peers[0].PublicKey != "key-cloud-rotated" {
		t.Fatalf("the enrolled key replaces the declared one: %+v", entry.Peers)
	}
	var updated stationv1alpha1.SiteNetwork
	if err := c.Get(context.Background(), testNetworkKey, &updated); err != nil || updated.Status.Members != 2 || updated.Status.Declared != nil {
		t.Fatalf("members %d, declared %+v, %v", updated.Status.Members, updated.Status.Declared, err)
	}
}

// A machine admitted by a one-time code gets its address before it has
// redeemed the code, its tunnel once it has, and keeps both when the node
// enrolls under its own identity. A machine that replaces a node of the same
// name pushes the old machine's key out the moment it redeems.
func TestNetworkAdmitsAMachineByCode(t *testing.T) {
	keyHub, keyOld, keyNew := strings.Repeat("A", 43)+"=", strings.Repeat("B", 43)+"=", strings.Repeat("C", 43)+"="
	objects := append(testMember("dev", "cailiang", keyHub, "10.11.14.128"), testMember("dev", "robot", keyOld, "192.168.0.10")...)
	admission := &stationv1alpha1.NodeAdmission{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-dev", Name: "robot", Generation: 1},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "dev", CodeHash: strings.Repeat("0", 64),
			ExpiresAt: metav1.NewTime(time.Unix(1_800_000_600, 0)),
			Uplink:    &stationv1alpha1.SiteNetworkUplink{Gateway: "192.168.0.11"}},
	}
	r, c := newNetworkTest(t, append(objects, testSiteNetwork(), admission)...)
	admissionStatus := func() stationv1alpha1.NodeAdmissionStatus {
		t.Helper()
		reconcileNetwork(t, r)
		var got stationv1alpha1.NodeAdmission
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns-dev", Name: "robot"}, &got); err != nil {
			t.Fatal(err)
		}
		return got.Status
	}
	hubPeers := func() map[string]bool {
		keys := map[string]bool{}
		for _, peer := range configOf(t, enrollmentStatus(t, c, "dev", "cailiang")).Peers {
			keys[peer.PublicKey] = true
		}
		return keys
	}

	pending := admissionStatus()
	old := enrollmentStatus(t, c, "dev", "robot")
	if pending.Phase != stationv1alpha1.NodeAdmissionPending || pending.Address != old.TunnelAddress || pending.Config != "" ||
		old.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || !hubPeers()[keyOld] {
		t.Fatalf("an unredeemed admission must leave the old machine in place and take over its address: %+v, old %+v", pending, old)
	}

	var stored stationv1alpha1.NodeAdmission
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns-dev", Name: "robot"}, &stored); err != nil {
		t.Fatal(err)
	}
	stored.Spec.PublicKey = keyNew
	if err := c.Update(context.Background(), &stored); err != nil {
		t.Fatal(err)
	}
	redeemed := admissionStatus()
	var config network.NodeConfig
	if err := json.Unmarshal([]byte(redeemed.Config), &config); err != nil {
		t.Fatalf("a redeemed admission must carry a configuration: %+v: %v", redeemed, err)
	}
	if redeemed.Phase != stationv1alpha1.NodeAdmissionRedeemed || redeemed.Address != pending.Address || config.Address != pending.Address+"/32" ||
		config.ListenPort != 0 || len(config.UplinkRoutes) == 0 || len(config.Peers) != 1 || config.Peers[0].PublicKey != keyHub {
		t.Fatalf("redeemed admission: %+v config %+v", redeemed, config)
	}
	if peers := hubPeers(); !peers[keyNew] || peers[keyOld] || enrollmentStatus(t, c, "dev", "robot").Phase != stationv1alpha1.NetworkEnrollmentRejected {
		t.Fatalf("the hub must know the admitted machine and drop the replaced one: %v", peers)
	}

	// The node joins: its record passes to the new Node and it enrolls with the same key.
	var enrollment stationv1alpha1.NetworkEnrollment
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns-dev", Name: "robot"}, &enrollment); err != nil {
		t.Fatal(err)
	}
	enrollment.Spec.NodeUID, enrollment.Spec.PublicKey = "node-robot-new", keyNew
	if err := c.Update(context.Background(), &enrollment); err != nil {
		t.Fatal(err)
	}
	joined := admissionStatus()
	own := enrollmentStatus(t, c, "dev", "robot")
	if joined.Phase != stationv1alpha1.NodeAdmissionJoined || joined.Config != "" || own.Phase != stationv1alpha1.NetworkEnrollmentEnrolled ||
		own.TunnelAddress != pending.Address || configOf(t, own).ListenPort != 0 || !hubPeers()[keyNew] {
		t.Fatalf("joined: admission %+v enrollment %+v", joined, own)
	}
}

// A machine that is a node of one cluster and redeems an admission into
// another keeps its key and its address: the enrollment it leaves behind
// gives way without anyone deleting it, and its peers keep one entry for it.
func TestNetworkMovesAMachineToAnotherCluster(t *testing.T) {
	keyHub, keyRobot := strings.Repeat("A", 43)+"=", strings.Repeat("B", 43)+"="
	siteNetwork := testSiteNetwork()
	siteNetwork.Spec.Nodes = append(siteNetwork.Spec.Nodes, stationv1alpha1.SiteNetworkNode{ClusterID: "robot-01", NodeName: "x86", Role: "Leaf", Address: "10.254.240.7"})
	objects := append(testMember("dev", "cailiang", keyHub, "10.11.14.128"), testMember("robot-01", "x86", keyRobot, "192.168.0.10")...)
	admission := &stationv1alpha1.NodeAdmission{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-dev", Name: "x86", Generation: 1},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "dev", CodeHash: strings.Repeat("0", 64),
			ExpiresAt: metav1.NewTime(time.Unix(1_800_000_600, 0)), Address: "10.254.240.7"},
	}
	r, c := newNetworkTest(t, append(objects, siteNetwork, admission)...)
	reconcileNetwork(t, r)
	if old := enrollmentStatus(t, c, "robot-01", "x86"); old.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || old.TunnelAddress != "10.254.240.7" {
		t.Fatalf("before the move the machine is enrolled where it is: %+v", old)
	}

	var stored stationv1alpha1.NodeAdmission
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns-dev", Name: "x86"}, &stored); err != nil {
		t.Fatal(err)
	}
	stored.Spec.PublicKey = keyRobot
	if err := c.Update(context.Background(), &stored); err != nil {
		t.Fatal(err)
	}
	reconcileNetwork(t, r)
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns-dev", Name: "x86"}, &stored); err != nil {
		t.Fatal(err)
	}
	old := enrollmentStatus(t, c, "robot-01", "x86")
	hub := configOf(t, enrollmentStatus(t, c, "dev", "cailiang"))
	entries := 0
	for _, peer := range hub.Peers {
		if peer.PublicKey == keyRobot {
			entries++
			if len(peer.AllowedIPs) == 0 || peer.AllowedIPs[0] != "10.254.240.7/32" {
				t.Fatalf("the hub's entry for the moved machine: %+v", peer)
			}
		}
	}
	if stored.Status.Phase != stationv1alpha1.NodeAdmissionRedeemed || stored.Status.Address != "10.254.240.7" || stored.Status.Config == "" ||
		old.Phase != stationv1alpha1.NetworkEnrollmentRejected || !strings.Contains(old.Reason, "moved to dev/x86") || entries != 1 {
		t.Fatalf("after the move: admission %+v, old enrollment %+v, hub entries %d", stored.Status, old, entries)
	}
	var network stationv1alpha1.SiteNetwork
	if err := c.Get(context.Background(), testNetworkKey, &network); err != nil {
		t.Fatal(err)
	}
	for _, condition := range network.Status.Conditions {
		if condition.Type == "Ready" && condition.Status != metav1.ConditionTrue {
			t.Fatalf("the SiteNetwork must stay computable during the move: %+v", condition)
		}
	}
}

// A cluster gets a new control plane and its nodes join it again, each as
// the node it is, with the key it has: the admission answers with the
// node's configuration, the node stays among its peers until its record
// has passed to its new Node, and the admission is Joined from there. So for a node that joined by a code and for one the
// SiteNetwork declares; and a second time with a new code.
func TestNetworkLetsANodeJoinItsClustersNewControlPlane(t *testing.T) {
	keyHub, keyRobot, keyListed := strings.Repeat("A", 43)+"=", strings.Repeat("B", 43)+"=", strings.Repeat("C", 43)+"="
	siteNetwork := testSiteNetwork()
	siteNetwork.Spec.Nodes = append(siteNetwork.Spec.Nodes, stationv1alpha1.SiteNetworkNode{ClusterID: "robot-01", NodeName: "listed", Address: "10.254.240.7"})
	objects := append(testMember("dev", "cailiang", keyHub, "10.11.14.128"), testMember("robot-01", "robot", keyRobot, "192.168.0.10")...)
	objects = append(objects, testMember("robot-01", "listed", keyListed, "192.168.0.10")...)
	redeemedAt := metav1.NewTime(time.Unix(1_799_990_000, 0))
	first := &stationv1alpha1.NodeAdmission{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-robot-01", Name: "robot", Generation: 2},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robot-01", CodeHash: strings.Repeat("0", 64),
			ExpiresAt: metav1.NewTime(time.Unix(1_799_990_600, 0)), PublicKey: keyRobot, RedeemedAt: &redeemedAt},
		Status: stationv1alpha1.NodeAdmissionStatus{ObservedGeneration: 2, Phase: stationv1alpha1.NodeAdmissionJoined, Address: "10.254.240.4", RedeemedAt: &redeemedAt},
	}
	r, c := newNetworkTest(t, append(objects, siteNetwork, first)...)
	ctx := context.Background()
	admission := func(node string) stationv1alpha1.NodeAdmission {
		t.Helper()
		reconcileNetwork(t, r)
		var got stationv1alpha1.NodeAdmission
		if err := c.Get(ctx, types.NamespacedName{Namespace: "ns-robot-01", Name: node}, &got); err != nil {
			t.Fatal(err)
		}
		return got
	}
	hubKnows := func(key string) bool {
		for _, peer := range configOf(t, enrollmentStatus(t, c, "dev", "cailiang")).Peers {
			if peer.PublicKey == key {
				return true
			}
		}
		return false
	}
	if got := admission("robot").Status; got.Phase != stationv1alpha1.NodeAdmissionJoined || got.RejoinsNodeUID != "" || enrollmentStatus(t, c, "robot-01", "robot").TunnelAddress != "10.254.240.4" {
		t.Fatalf("an admission joined before must stay as it is: %+v", got)
	}

	for _, node := range []struct{ name, key, address string }{{"robot", keyRobot, "10.254.240.4"}, {"listed", keyListed, "10.254.240.7"}} {
		// A new code for the node (admit-node.sh): the last redemption is cleared.
		renewed := &stationv1alpha1.NodeAdmission{
			ObjectMeta: metav1.ObjectMeta{Namespace: "ns-robot-01", Name: node.name},
			Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robot-01", CodeHash: strings.Repeat("1", 64),
				ExpiresAt: metav1.NewTime(time.Unix(1_800_000_600, 0))},
		}
		if node.name == "robot" {
			stored := admission("robot")
			stored.Spec = renewed.Spec
			if err := c.Update(ctx, &stored); err != nil {
				t.Fatal(err)
			}
		} else if err := c.Create(ctx, renewed); err != nil {
			t.Fatal(err)
		}
		if got := admission(node.name).Status; got.Phase != stationv1alpha1.NodeAdmissionPending || got.Address != node.address {
			t.Fatalf("%s: a new code opens the admission at the node's address: %+v", node.name, got)
		}
		stored := admission(node.name)
		now := metav1.NewTime(time.Unix(1_800_000_000, 0))
		stored.Spec.PublicKey, stored.Spec.RedeemedAt = node.key, &now
		if err := c.Update(ctx, &stored); err != nil {
			t.Fatal(err)
		}
		redeemed := admission(node.name).Status
		own := enrollmentStatus(t, c, "robot-01", node.name)
		if redeemed.Phase != stationv1alpha1.NodeAdmissionRedeemed || redeemed.RejoinsNodeUID != "node-"+node.name || redeemed.Config == "" ||
			redeemed.Config != own.Config || redeemed.PublicKey != node.key || own.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || own.TunnelAddress != node.address {
			t.Fatalf("%s: the admission must answer with the node's own configuration: %+v, enrollment %+v", node.name, redeemed, own)
		}

		// The node is registered by the new control plane under a new UID and its record passes to it.
		var enrollment stationv1alpha1.NetworkEnrollment
		if err := c.Get(ctx, types.NamespacedName{Namespace: "ns-robot-01", Name: node.name}, &enrollment); err != nil {
			t.Fatal(err)
		}
		enrollment.Spec.NodeUID = "node-" + node.name + "-again"
		if err := c.Update(ctx, &enrollment); err != nil {
			t.Fatal(err)
		}
		joined := admission(node.name).Status
		own = enrollmentStatus(t, c, "robot-01", node.name)
		if joined.Phase != stationv1alpha1.NodeAdmissionJoined || joined.Config != "" || joined.RejoinsNodeUID != "" ||
			own.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || own.TunnelAddress != node.address || !hubKnows(node.key) {
			t.Fatalf("%s: joined again: admission %+v enrollment %+v", node.name, joined, own)
		}
		if again := admission(node.name).Status; again.Phase != stationv1alpha1.NodeAdmissionJoined {
			t.Fatalf("%s: Joined must stay: %+v", node.name, again)
		}
	}
}

// A leaf that drops off cannot say why; its hubs' reports tell requests that
// arrive unanswered from silence, and the leaf's status is written only when
// that account changes.
func TestNetworkSaysWhatTheHubsSeeOfALeaf(t *testing.T) {
	objects := []client.Object{testSiteNetwork()}
	objects = append(objects, testMember("dev", "cailiang", "key-dev", "10.11.14.128")...)
	objects = append(objects, testMember("dev", "alexfu", "key-3090", "10.11.13.192")...)
	objects = append(objects, testMember("robot-01", "s1u-01-x86", "key-robot-1", "192.168.0.10")...)
	r, c := newNetworkTest(t, objects...)
	clock := time.Unix(1_800_000_000, 0)
	r.now = func() time.Time { return clock }
	// Thirty seconds pass, the named hubs report, and the network is reconciled.
	step := func(handshake time.Time, received int64, hubs ...string) (stationv1alpha1.NetworkEnrollmentStatus, bool) {
		t.Helper()
		clock = clock.Add(30 * time.Second)
		for _, hub := range hubs {
			var reporting stationv1alpha1.NetworkEnrollment
			if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns-dev", Name: hub}, &reporting); err != nil {
				t.Fatal(err)
			}
			seen := metav1.NewTime(handshake)
			reporting.Status.Observation = &stationv1alpha1.NodeObservation{ReportedAt: metav1.NewTime(clock.Add(-time.Second)), NodeUID: "node-" + hub,
				Wireguard: &stationv1alpha1.WireguardObservation{Interface: "st-wg0", Peers: []stationv1alpha1.WireguardPeerObservation{
					{Name: "robot-01/s1u-01-x86", LatestHandshake: &seen, ReceivedBytes: received}}}}
			if err := c.Status().Update(context.Background(), &reporting); err != nil {
				t.Fatal(err)
			}
		}
		var before, after stationv1alpha1.NetworkEnrollment
		key := types.NamespacedName{Namespace: "ns-robot-01", Name: "s1u-01-x86"}
		if err := c.Get(context.Background(), key, &before); err != nil {
			t.Fatal(err)
		}
		result, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: testNetworkKey})
		if err != nil || result.RequeueAfter != 30*time.Second {
			t.Fatalf("reconcile = %+v, %v", result, err)
		}
		if err := c.Get(context.Background(), key, &after); err != nil {
			t.Fatal(err)
		}
		return after.Status, before.ResourceVersion != after.ResourceVersion
	}
	states := func(status stationv1alpha1.NetworkEnrollmentStatus) string {
		text := ""
		for _, link := range status.Links {
			text += link.Hub + "=" + string(link.State) + " "
		}
		return text
	}
	both := []string{"cailiang", "alexfu"}

	status, _ := step(clock, 1000, both...)
	connectedSince := clock
	if states(status) != "dev/cailiang=Connected dev/alexfu=Connected " || status.LinkSummary != "" || !status.Links[0].Since.Time.Equal(connectedSince) {
		t.Fatalf("connected: %+v %q", status.Links, status.LinkSummary)
	}
	if hub := enrollmentStatus(t, c, "dev", "cailiang"); hub.Links != nil || hub.LinkSummary != "" {
		t.Fatalf("a hub has links: %+v", hub.Links)
	}
	handshake := clock
	if status, written := step(handshake, 2000, both...); written || status.Links[0].ObservedAt != nil {
		t.Fatalf("a connected leaf was written again: %+v", status.Links)
	}

	// The handshake grows old while the leaf's requests keep arriving. The
	// first such report only leaves the counter to compare the next with.
	clock = handshake.Add(3 * time.Minute)
	status, written := step(handshake, 2148, both...)
	if !written || states(status) != "dev/cailiang=Connected dev/alexfu=Connected " || status.Links[0].ReceivedBytes != 2148 || status.Links[0].ObservedAt == nil {
		t.Fatalf("first stale report: %+v", status.Links)
	}
	status, _ = step(handshake, 2296, both...)
	unansweredSince := clock
	if states(status) != "dev/cailiang=Unanswered dev/alexfu=Unanswered " || !status.Links[0].Since.Time.Equal(unansweredSince) || !status.Links[0].LatestHandshake.Time.Equal(handshake) ||
		status.LinkSummary != "this node's requests arrive at dev/cailiang, dev/alexfu and no session forms: the answers do not get back to it" {
		t.Fatalf("unanswered: %+v %q", status.Links, status.LinkSummary)
	}
	if status, _ = step(handshake, 2444, both...); states(status) != "dev/cailiang=Unanswered dev/alexfu=Unanswered " || !status.Links[0].Since.Time.Equal(unansweredSince) || status.Links[0].ReceivedBytes != 2444 {
		t.Fatalf("still unanswered: %+v", status.Links)
	}
	// No hub has reported since: there is nothing new to say.
	if _, written := step(handshake, 0); written {
		t.Fatal("the same reports were written again")
	}

	// One hub stops hearing the leaf, then both.
	status, _ = step(handshake, 2444, "cailiang")
	if states(status) != "dev/cailiang=Silent dev/alexfu=Unanswered " || !status.Links[1].Since.Time.Equal(unansweredSince) ||
		status.LinkSummary != "this node's requests arrive at dev/alexfu and no session forms: the answers do not get back to it; nothing arrives from this node at dev/cailiang" {
		t.Fatalf("one hub silent: %+v %q", status.Links, status.LinkSummary)
	}
	status, _ = step(handshake, 2444, both...)
	silentSince := clock
	if states(status) != "dev/cailiang=Silent dev/alexfu=Silent " || !status.Links[1].Since.Time.Equal(silentSince) || status.Links[0].Since.Time.Equal(silentSince) ||
		status.LinkSummary != "nothing arrives from this node at any hub it dials since "+silentSince.UTC().Format(time.RFC3339) {
		t.Fatalf("silent: %+v %q", status.Links, status.LinkSummary)
	}
	for range 3 {
		if _, written := step(handshake, 2444, both...); written {
			t.Fatal("a silent leaf was written again")
		}
	}

	// A hub that stops reporting says nothing about the leaf.
	for range 4 {
		status, _ = step(handshake, 2444, "cailiang")
	}
	if states(status) != "dev/cailiang=Silent dev/alexfu=Unknown " || status.Links[1].ObservedAt != nil ||
		status.LinkSummary != "nothing arrives from this node at dev/cailiang; no current report of this node from dev/alexfu" {
		t.Fatalf("hub without a report: %+v %q", status.Links, status.LinkSummary)
	}

	status, _ = step(clock, 5000, both...)
	if states(status) != "dev/cailiang=Connected dev/alexfu=Connected " || status.LinkSummary != "" || !status.Links[0].Since.Time.Equal(clock) ||
		status.Links[0].LatestHandshake != nil || status.Links[0].ObservedAt != nil || status.Links[0].ReceivedBytes != 0 {
		t.Fatalf("connected again: %+v %q", status.Links, status.LinkSummary)
	}
	if _, written := step(clock, 6000, both...); written {
		t.Fatal("a connected leaf was written again")
	}
}

func TestNetworkPutsANodeInTheSiteItNames(t *testing.T) {
	siteNetwork := testSiteNetwork()
	siteNetwork.Spec.Sites[0].DNS = []string{"10.11.0.2"}
	siteNetwork.Spec.Sites[1].DNS = []string{"192.168.50.1"}
	siteNetwork.Spec.Nodes = append(siteNetwork.Spec.Nodes,
		stationv1alpha1.SiteNetworkNode{ClusterID: "robot-01", NodeName: "by-entry", Site: "office"},
		stationv1alpha1.SiteNetworkNode{ClusterID: "robot-01", NodeName: "astray", Site: "nowhere"})
	objects := []client.Object{siteNetwork,
		// The entry's site comes before the admission's.
		&stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-robot-01", Name: "by-entry"},
			Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robot-01", Site: "lab", ExpiresAt: metav1.NewTime(time.Unix(1_800_003_600, 0))}},
		&stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-robot-01", Name: "by-admission"},
			Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robot-01", Site: "office", ExpiresAt: metav1.NewTime(time.Unix(1_800_003_600, 0))}},
		&stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-robot-01", Name: "at-home"},
			Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robot-01", ExpiresAt: metav1.NewTime(time.Unix(1_800_003_600, 0))}},
		// A machine not yet a node, admitted into a site nothing declares, before and after it redeemed its code.
		&stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-robot-01", Name: "new-astray"},
			Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robot-01", Site: "nowhere", ExpiresAt: metav1.NewTime(time.Unix(1_800_003_600, 0))}},
		&stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: "ns-robot-01", Name: "new-astray-redeemed"},
			Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "robot-01", Site: "nowhere", PublicKey: "key-new-astray", ExpiresAt: metav1.NewTime(time.Unix(1_800_003_600, 0))}},
	}
	objects = append(objects, testMember("dev", "cailiang", "key-dev", "10.11.14.128")...)
	for _, node := range []string{"at-home", "by-entry", "by-admission", "astray"} {
		objects = append(objects, testMember("robot-01", node, "key-"+node, "192.168.0.10")...)
	}
	r, c := newNetworkTest(t, objects...)
	reconcileNetwork(t, r)

	for node, site := range map[string]string{"at-home": "lab", "by-entry": "office", "by-admission": "office"} {
		status := enrollmentStatus(t, c, "robot-01", node)
		dns := map[string]string{"lab": "192.168.50.1", "office": "10.11.0.2"}[site]
		if status.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || status.Site != site || configOf(t, status).DNS[0] != dns {
			t.Fatalf("%s is in %s with that site's name servers: %+v", node, site, status)
		}
	}
	astray := enrollmentStatus(t, c, "robot-01", "astray")
	if astray.Phase != stationv1alpha1.NetworkEnrollmentRejected || !strings.Contains(astray.Reason, "nowhere") {
		t.Fatalf("a node of a site nothing declares: %+v", astray)
	}
	for name, phase := range map[string]stationv1alpha1.NodeAdmissionPhase{"new-astray": stationv1alpha1.NodeAdmissionPending, "new-astray-redeemed": stationv1alpha1.NodeAdmissionRedeemed} {
		var admission stationv1alpha1.NodeAdmission
		if err := c.Get(context.Background(), types.NamespacedName{Namespace: "ns-robot-01", Name: name}, &admission); err != nil {
			t.Fatal(err)
		}
		if admission.Status.Phase != phase || !strings.Contains(admission.Status.Reason, "nowhere") || admission.Status.Config != "" {
			t.Fatalf("the admission %s into a site nothing declares: %+v", name, admission.Status)
		}
	}
}

func TestNetworkSaysWhichHubRelaysForALeaf(t *testing.T) {
	siteNetwork := testSiteNetwork()
	siteNetwork.Spec.Nodes[1].Relay = true
	objects := []client.Object{siteNetwork}
	objects = append(objects, testMember("dev", "cailiang", "key-dev", "10.11.14.128")...)
	objects = append(objects, testMember("dev", "alexfu", "key-3090", "10.11.13.192")...)
	objects = append(objects, testMember("robot-01", "s1u-01-x86", "key-robot-1", "192.168.0.10")...)
	r, c := newNetworkTest(t, objects...)
	reconcileNetwork(t, r)
	if robot := enrollmentStatus(t, c, "robot-01", "s1u-01-x86"); robot.Relay != "dev/cailiang" {
		t.Fatalf("the first relay the leaf can dial: %+v", robot)
	}

	// The site names another: one value of the declaration.
	var declared stationv1alpha1.SiteNetwork
	if err := c.Get(context.Background(), testNetworkKey, &declared); err != nil {
		t.Fatal(err)
	}
	declared.Spec.Sites[1].Relay = "dev/alexfu"
	if err := c.Update(context.Background(), &declared); err != nil {
		t.Fatal(err)
	}
	reconcileNetwork(t, r)
	robot := enrollmentStatus(t, c, "robot-01", "s1u-01-x86")
	if robot.Relay != "dev/alexfu" || robot.Phase != stationv1alpha1.NetworkEnrollmentEnrolled || len(robot.Problems) != 0 {
		t.Fatalf("the relay the site names: %+v", robot)
	}
}

func TestNetworkTellsTheHubsOfADiscoverySiteWhatToLookForAndSaysWhenOneGoesSilent(t *testing.T) {
	ctx := context.Background()
	declared := testSiteNetwork()
	declared.Spec.Sites[1].Discovery = &stationv1alpha1.SiteNetworkDiscovery{Enabled: true, Ranges: []string{"192.168.31.0/24"}, IntervalSeconds: 30,
		From: stationv1alpha1.SiteNetworkDiscoverySource{ClusterID: "dev", NodeName: "cailiang"}}
	declared.Spec.Nodes = append(declared.Spec.Nodes, stationv1alpha1.SiteNetworkNode{ClusterID: "robot-01", NodeName: "cp", Role: "Hub", Address: "10.254.240.5",
		Endpoints: []stationv1alpha1.SiteNetworkEndpoint{{Address: "10.11.13.192:51820", Scope: "Site"}}})
	factory := "SHA256:" + strings.Repeat("f", 43)
	class := &stationv1alpha1.NodeClass{ObjectMeta: metav1.ObjectMeta{Name: "s1u"}, Spec: stationv1alpha1.NodeClassSpec{
		Discovery: &stationv1alpha1.NodeClassDiscovery{HostKeyFingerprints: []string{factory}},
		Access:    &stationv1alpha1.NodeClassAccess{Entry: stationv1alpha1.NodeClassEntry{Port: 22, User: "orin", SerialFile: "/serial"}}}}
	objects := []client.Object{declared, class}
	objects = append(objects, testMember("dev", "cailiang", "key-dev", "10.11.14.128")...)
	objects = append(objects, testMember("robot-01", "cp", "key-cp", "10.11.13.192")...)
	objects = append(objects, testMember("robot-01", "s1u-01-x86", "key-robot-1", "192.168.0.10")...)
	r, c := newNetworkTest(t, objects...)
	stale := func() *metav1.Condition {
		t.Helper()
		reconcileNetwork(t, r)
		var stored stationv1alpha1.SiteNetwork
		if err := c.Get(ctx, testNetworkKey, &stored); err != nil {
			t.Fatal(err)
		}
		for i := range stored.Status.Conditions {
			if stored.Status.Conditions[i].Type == "DiscoveryStale" {
				return &stored.Status.Conditions[i]
			}
		}
		return nil
	}
	stale() // the hub enrolls

	hub := enrollmentStatus(t, c, "robot-01", "cp")
	want := &network.Discovery{Ranges: []string{"192.168.31.0/24"}, IntervalSeconds: 30, Ports: []int{22}, HostKeys: []string{factory}, RelayPort: network.DiscoveryRelayPort, RelayFrom: "10.254.240.1"}
	if got := configOf(t, hub).Discovery; got == nil || got.RelayFrom != want.RelayFrom || got.RelayPort != want.RelayPort || got.HostKeys[0] != factory || got.Ports[0] != 22 || got.Ranges[0] != want.Ranges[0] || got.IntervalSeconds != 30 {
		t.Fatalf("the document of the lab's hub: %+v", got)
	}
	if configOf(t, enrollmentStatus(t, c, "robot-01", "s1u-01-x86")).Discovery != nil || configOf(t, enrollmentStatus(t, c, "dev", "cailiang")).Discovery != nil {
		t.Fatal("a leaf of the site or a hub of another site is told to look")
	}

	// A machine of the site that joined shows a key of its own from then on: the site's hubs look for it too, so
	// that it is seen where its next lease puts it. A machine of another site is that site's.
	rotated, elsewhere := "SHA256:"+strings.Repeat("r", 43), "SHA256:"+strings.Repeat("e", 43)
	for name, at := range map[string][2]string{"lab-48b02d5c9a11": {"lab", rotated}, "office-48b02d5c9a12": {"office", elsewhere}, "lab-48b02d5c9a13": {"lab", factory}} {
		machine := &stationv1alpha1.Machine{ObjectMeta: metav1.ObjectMeta{Namespace: testNetworkKey.Namespace, Name: name},
			Spec:   stationv1alpha1.MachineSpec{Site: at[0], NodeClass: "s1u", Source: "Discovery"},
			Status: stationv1alpha1.MachineStatus{Entry: stationv1alpha1.MachineEntry{HostKeyFingerprint: at[1]}}}
		if err := c.Create(ctx, machine); err != nil {
			t.Fatal(err)
		}
	}
	stale()
	if got := configOf(t, enrollmentStatus(t, c, "robot-01", "cp")).Discovery; got == nil || !slices.Equal(got.HostKeys, []string{factory, rotated}) {
		t.Fatalf("what the lab's hub looks for with a joined machine: %+v", got)
	}

	// A hub that has not reported yet, one that reports, one that has stopped.
	if condition := stale(); condition == nil || condition.Status != metav1.ConditionTrue || !strings.Contains(condition.Message, "site lab: hub robot-01/cp") || !strings.Contains(condition.Message, "never") {
		t.Fatalf("a hub that never reported: %+v", condition)
	}
	report := func(ago time.Duration) {
		t.Helper()
		var enrollment stationv1alpha1.NetworkEnrollment
		if err := c.Get(ctx, types.NamespacedName{Namespace: "ns-robot-01", Name: "cp"}, &enrollment); err != nil {
			t.Fatal(err)
		}
		enrollment.Annotations = map[string]string{DiscoveryReportedAnnotation: time.Unix(1_800_000_000, 0).Add(-ago).UTC().Format(time.RFC3339)}
		if err := c.Update(ctx, &enrollment); err != nil {
			t.Fatal(err)
		}
	}
	report(90 * time.Second)
	if condition := stale(); condition == nil || condition.Status != metav1.ConditionFalse {
		t.Fatalf("a hub that reported three intervals ago: %+v", condition)
	}
	report(91 * time.Second)
	if condition := stale(); condition == nil || condition.Status != metav1.ConditionTrue || !strings.Contains(condition.Message, "last at 2027-01-15T07:58:29Z") {
		t.Fatalf("a hub silent for more than three intervals: %+v", condition)
	}

	// Without a site that looks, the network says nothing of looking.
	var stored stationv1alpha1.SiteNetwork
	_ = c.Get(ctx, testNetworkKey, &stored)
	stored.Spec.Sites[1].Discovery = nil
	_ = c.Update(ctx, &stored)
	if condition := stale(); condition != nil {
		t.Fatalf("no site declares discovery: %+v", condition)
	}
}

// A node declared at an address another node holds by its admission is left out, and the network says so: the
// machine that has the address keeps it (staging 11-17: a new cluster's first hub declared at a robot's address took
// the robot's tunnel).
func TestNetworkLeavesOutADeclaredNodeAtAnAddressAnAdmissionHolds(t *testing.T) {
	siteNetwork := testSiteNetwork()
	siteNetwork.Spec.Sites = append(siteNetwork.Spec.Sites, stationv1alpha1.SiteNetworkSite{Name: "ksyun", Clusters: []string{"cloud"}})
	siteNetwork.Spec.Nodes = append(siteNetwork.Spec.Nodes, stationv1alpha1.SiteNetworkNode{
		ClusterID: "cloud", NodeName: "cloud-1", Role: "Hub", Address: "10.254.240.9", NodeAddress: "10.0.2.9", PublicKey: "key-cloud",
		Endpoints: []stationv1alpha1.SiteNetworkEndpoint{{Address: "36.103.180.169:51820", Scope: "Public"}},
	})
	admission := &stationv1alpha1.NodeAdmission{
		ObjectMeta: metav1.ObjectMeta{Namespace: "ns-dev", Name: "robot", Generation: 1},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "dev", CodeHash: strings.Repeat("0", 64), Address: "10.254.240.9",
			ExpiresAt: metav1.NewTime(time.Unix(1_800_000_600, 0))},
	}
	objects := append([]client.Object{siteNetwork, admission}, testMember("dev", "cailiang", "key-dev", "10.11.14.128")...)
	r, c := newNetworkTest(t, objects...)
	reconcileNetwork(t, r)
	if entry := configOf(t, enrollmentStatus(t, c, "dev", "cailiang")); slices.ContainsFunc(entry.Peers, func(p network.Peer) bool { return p.PublicKey == "key-cloud" }) {
		t.Fatalf("the declared node takes part at a held address: %+v", entry.Peers)
	}
	var got stationv1alpha1.SiteNetwork
	if err := c.Get(context.Background(), testNetworkKey, &got); err != nil {
		t.Fatal(err)
	}
	ready := apimeta.FindStatusCondition(got.Status.Conditions, "Ready")
	if ready == nil || ready.Status != metav1.ConditionFalse || ready.Reason != "AddressTaken" || !strings.Contains(ready.Message, "cloud/cloud-1 is declared at 10.254.240.9, which is dev/robot's") || got.Status.Declared != nil {
		t.Fatalf("ready %+v, declared %+v", ready, got.Status.Declared)
	}
}

// A redeemed admission says where its join stands, step by step.
func TestARedeemedAdmissionSaysWhereItsJoinStands(t *testing.T) {
	now := time.Now().UTC()
	redeemed := metav1.NewTime(now.Add(-5 * time.Minute))
	admission := &stationv1alpha1.NodeAdmission{ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: "nb260706069"},
		Spec: stationv1alpha1.NodeAdmissionSpec{ClusterID: "cluster-a", PublicKey: "key", RedeemedAt: &redeemed}}
	if step := joinStep(admission, nil); !strings.Contains(step, "还没回来取入网令牌") {
		t.Fatalf("before the token: %q", step)
	}
	admission.Annotations = map[string]string{JoinTokenIssuedAnnotation: now.Add(-4 * time.Minute).Format(time.RFC3339)}
	enrollment := &stationv1alpha1.NetworkEnrollment{Spec: stationv1alpha1.NetworkEnrollmentSpec{NodeUID: "old-uid"}, Status: stationv1alpha1.NetworkEnrollmentStatus{
		Observation: &stationv1alpha1.NodeObservation{NodeUID: "old-uid", ReportedAt: metav1.NewTime(now.Add(-time.Hour))},
		Node:        &stationv1alpha1.NetworkEnrollmentNode{Conditions: []metav1.Condition{{Type: "IdentityMatched", Status: metav1.ConditionFalse, Message: "delete this node record"}}}}}
	if step := joinStep(admission, enrollment); !strings.Contains(step, "kubeadm join") {
		t.Fatalf("token given, no report since: %q", step)
	}
	enrollment.Status.Observation = &stationv1alpha1.NodeObservation{NodeUID: "new-uid", ReportedAt: metav1.NewTime(now)}
	if step := joinStep(admission, enrollment); !strings.Contains(step, "以前的节点上") || !strings.Contains(step, "delete this node record") {
		t.Fatalf("reported, the record still of the gone node: %q", step)
	}
	enrollment.Spec.NodeUID, enrollment.Spec.Retired = "new-uid", true
	if step := joinStep(admission, enrollment); !strings.Contains(step, "已被移除") {
		t.Fatalf("reported, the record's own node removed: %q", step)
	}
	enrollment.Spec.Retired = false
	enrollment.Status.Phase, enrollment.Status.Reason = stationv1alpha1.NetworkEnrollmentRejected, "the machine moved"
	if step := joinStep(admission, enrollment); !strings.Contains(step, "隧道登记还没通过：the machine moved") {
		t.Fatalf("reported, enrollment rejected: %q", step)
	}
}
