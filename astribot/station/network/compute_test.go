package network

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"
)

const (
	keyDev   = "DwjZS7W8Yab4SZN62rtBuvKzrzWwoa6nHwObl1Ck2XM="
	keyCloud = "ixGRrgUmo9UMpnCg00v3DPbRT2NEqvmm6eI/lfzpEEc="
	keyRobot = "YxGnfRDiE+94DLi/fCMeVe3zl73UTudA9apw8/BWjXs="
	keyTrain = "5Go4sNJOoyAwxL5PXdMYnVHRJJQWkNzdBRUKDWzDzS0="
	key3090  = "Rpk1Y6lpJXIGOdNcgLMYs0J6mBG+67d85HVJs+upaR4="
	keyDesk  = "uaC5QIh01DyQvTqCObM2dl4hcMIedM5lPYIhkrcQjQQ="
)

// liveTopology is the site of 2026-11-12 16:10 (six nodes) written as a declaration.
func liveTopology() (Topology, []Member) {
	topology := Topology{
		Pool: "10.254.240.0/24", MTU: 1380, Interface: "st-wg0", ListenPort: 51820, KeepaliveSeconds: 25,
		Sites: []Site{{Name: "office", Mesh: true}, {Name: "lab", Reaches: []string{"office"}}, {Name: "ksyun"}},
	}
	members := []Member{
		{ID: "rlark-local-dev-gpu/cailiang", Site: "office", Role: RoleHub, PublicKey: keyDev, Address: "10.254.240.1",
			Endpoints: []Endpoint{{Address: "10.11.14.128:51820", Scope: ScopeSite}},
			Advertise: []Advertisement{{CIDR: "10.11.5.12/32", Sites: []string{"ksyun"}}}},
		{ID: "rlark-ksyun-gpu/10-0-2-57", Site: "ksyun", Role: RoleHub, PublicKey: keyCloud, Address: "10.254.240.2", NodeAddress: "10.0.2.57",
			Endpoints: []Endpoint{{Address: "36.103.180.169:51820", Scope: ScopePublic}},
			Advertise: []Advertisement{{CIDR: "172.27.0.0/24", Sites: []string{"office"}}}},
		{ID: "rlark-robot-s1u-01/s1u-01-x86", Site: "lab", Role: RoleLeaf, PublicKey: keyRobot, Address: "10.254.240.3",
			Uplink: &Uplink{Gateway: "192.168.0.11"}},
		{ID: "rlark-ksyun-gpu/10-0-2-196", Site: "ksyun", Role: RoleHub, PublicKey: keyTrain, Address: "10.254.240.4", NodeAddress: "10.0.2.196",
			Interface: "st-hub0", ListenPort: 51821,
			Endpoints: []Endpoint{{Address: "36.103.180.169:51821", Scope: ScopePublic}},
			Advertise: []Advertisement{{CIDR: "172.27.1.0/24", Sites: []string{"office"}}}},
		{ID: "rlark-local-dev-gpu/alexfu", Site: "office", Role: RoleHub, PublicKey: key3090, Address: "10.254.240.5",
			Endpoints: []Endpoint{{Address: "10.11.13.192:51820", Scope: ScopeSite}}},
		{ID: "rlark-local-dev-gpu/wendychen", Site: "office", Role: RoleHub, PublicKey: keyDesk, Address: "10.254.240.6",
			Endpoints: []Endpoint{{Address: "10.11.14.135:51820", Scope: ScopeSite}}},
	}
	return topology, members
}

// collectionSite is the full site over the public internet: 96 robots behind
// their own NAT, one workstation that relays between them, three cloud nodes.
func collectionSite(robots int) (Topology, []Member) {
	topology := Topology{
		Pool: "10.254.240.0/22", MTU: 1380, Interface: "st-wg0", ListenPort: 51820, KeepaliveSeconds: 25,
		Sites: []Site{{Name: "collection"}, {Name: "ksyun"}},
	}
	members := []Member{{
		ID: "site/workstation", Site: "collection", Role: RoleHub, Relay: true, PublicKey: "workstation", Address: "10.254.240.1",
		Endpoints: []Endpoint{{Address: "10.20.0.1:51820", Scope: ScopeSite}},
	}}
	for i := 0; i < 3; i++ {
		members = append(members, Member{
			ID: fmt.Sprintf("ksyun/h100-%d", i), Site: "ksyun", Role: RoleHub, PublicKey: fmt.Sprintf("cloud-%d", i),
			Address: fmt.Sprintf("10.254.240.%d", 2+i), NodeAddress: fmt.Sprintf("10.0.2.%d", 10+i),
			Endpoints: []Endpoint{{Address: fmt.Sprintf("36.103.180.169:%d", 51820+i), Scope: ScopePublic}},
		})
	}
	for i := 0; i < robots; i++ {
		members = append(members, Member{
			ID: fmt.Sprintf("robots/s1u-%02d-x86", i+1), Site: "collection", Role: RoleLeaf, PublicKey: fmt.Sprintf("robot-%02d", i+1),
			Address: fmt.Sprintf("10.254.241.%d", i+1),
			Uplink:  &Uplink{Gateway: "192.168.0.11"},
		})
	}
	return topology, members
}

func TestComputeCollectionSite(t *testing.T) {
	topology, members := collectionSite(96)
	result, err := Compute(topology, members)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Problems) != 0 || len(result.Unreachable) != 0 {
		t.Fatalf("problems %v, unreachable %v", result.Problems, result.Unreachable[:min(5, len(result.Unreachable))])
	}
	links, largest := 0, 0
	for id, config := range result.Configs {
		links += len(config.Peers)
		data, _ := json.Marshal(config)
		largest = max(largest, len(data))
		want := map[string]int{"site/workstation": 99, "ksyun/h100-0": 97}[id]
		if want == 0 {
			want = 4
			if config.Interface == "" {
				t.Fatal("empty interface")
			}
		}
		if id == "ksyun/h100-1" || id == "ksyun/h100-2" {
			want = 97
		}
		if len(config.Peers) != want {
			t.Errorf("%s has %d peers, want %d", id, len(config.Peers), want)
		}
	}
	// 96 robots x (workstation + 3 cloud) + workstation x 3 cloud; a full mesh
	// of the same 100 nodes would be 4950.
	if links/2 != 387 {
		t.Errorf("%d tunnels, want 387", links/2)
	}
	if largest > 16*1024 {
		t.Errorf("largest configuration is %d bytes", largest)
	}
	robot := result.Configs["robots/s1u-01-x86"]
	if robot.Peers[0].AllowedIPs[1] != "10.254.240.0/22" || len(robot.UplinkRoutes) != 2 {
		t.Errorf("robot does not relay through the workstation: %+v", robot)
	}
}

func TestComputeReportsUnreachablePairs(t *testing.T) {
	topology, members := collectionSite(2)
	members[0].Relay = false
	result, err := Compute(topology, members)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"robots/s1u-01-x86 -> robots/s1u-02-x86", "robots/s1u-02-x86 -> robots/s1u-01-x86"}
	if !reflect.DeepEqual(result.Unreachable, want) {
		t.Fatalf("unreachable %v, want %v", result.Unreachable, want)
	}
}

func TestComputeSkipsInvalidMembers(t *testing.T) {
	topology, members := liveTopology()
	members[4].PublicKey = keyDev
	members[2].Address = "10.254.241.3"
	members = append(members, Member{ID: "x/leaf-with-endpoint", Site: "lab", Role: RoleLeaf, PublicKey: "k", Address: "10.254.240.9",
		Endpoints: []Endpoint{{Address: "1.2.3.4:5", Scope: ScopePublic}}})
	result, err := Compute(topology, members)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"rlark-local-dev-gpu/alexfu", "rlark-robot-s1u-01/s1u-01-x86", "x/leaf-with-endpoint"} {
		if _, ok := result.Configs[id]; ok || len(result.Problems[id]) == 0 {
			t.Errorf("%s was not skipped with a problem: %v", id, result.Problems[id])
		}
	}
	if len(result.Configs["rlark-local-dev-gpu/cailiang"].Peers) != 3 {
		t.Errorf("valid members must still be configured: %+v", result.Configs["rlark-local-dev-gpu/cailiang"])
	}
}

func TestComputeDropsPrefixThatHidesAnEndpoint(t *testing.T) {
	topology, members := liveTopology()
	members[0].Advertise = append(members[0].Advertise, Advertisement{CIDR: "10.11.12.0/22"})
	result, err := Compute(topology, members)
	if err != nil {
		t.Fatal(err)
	}
	robot := result.Configs["rlark-robot-s1u-01/s1u-01-x86"]
	if contains(robot.Routes, "10.11.12.0/22") || len(result.Problems["rlark-robot-s1u-01/s1u-01-x86"]) != 1 {
		t.Fatalf("the robot dials 10.11.14.128 and must not route it into the tunnel: %v %v", robot.Routes, result.Problems)
	}
	if !contains(result.Configs["rlark-ksyun-gpu/10-0-2-57"].Routes, "10.11.12.0/22") {
		t.Fatal("a node that does not dial into the prefix keeps it")
	}
}

func TestAllocate(t *testing.T) {
	for _, test := range []struct {
		pool  string
		taken []string
		want  string
	}{
		{"10.254.240.0/24", nil, "10.254.240.1"},
		{"10.254.240.0/24", []string{"10.254.240.1", "10.254.240.2", "10.254.240.3", "10.254.240.5"}, "10.254.240.4"},
		{"10.254.240.0/30", []string{"10.254.240.1"}, "10.254.240.2"},
	} {
		got, err := Allocate(test.pool, test.taken)
		if err != nil || got != test.want {
			t.Errorf("Allocate(%s, %v) = %s, %v; want %s", test.pool, test.taken, got, err, test.want)
		}
	}
	if _, err := Allocate("10.254.240.0/30", []string{"10.254.240.1", "10.254.240.2"}); err == nil {
		t.Error("an exhausted pool must fail")
	}
}

// The document Compute emits is the one station-gateway parses. The same file
// is read by astribot/station-gateway's TestParsesComputedDocument, so the
// two modules cannot drift apart. Regenerate with UPDATE_GOLDEN=1.
func TestComputedDocumentIsTheGatewayContract(t *testing.T) {
	topology, members := liveTopology()
	result, err := Compute(topology, members)
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(result.Configs["rlark-robot-s1u-01/s1u-01-x86"])
	if err != nil {
		t.Fatal(err)
	}
	const golden = "../../station-gateway/testdata/computed-s1u-01-x86.json"
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, got, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil || string(want) != string(got) {
		t.Fatalf("computed document differs from %s (%v):\n%s", golden, err, got)
	}
}

// The site can instead send its robots through the workstation only: one peer
// per robot, and the workstation carries the cloud addresses for them.
func TestComputeLeavesThroughTheRelayOnly(t *testing.T) {
	topology, members := collectionSite(96)
	topology.Sites[0].LeavesViaRelay = true
	result, err := Compute(topology, members)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Problems) != 0 || len(result.Unreachable) != 0 {
		t.Fatalf("problems %v, unreachable %v", result.Problems, result.Unreachable[:min(5, len(result.Unreachable))])
	}
	links := 0
	for _, config := range result.Configs {
		links += len(config.Peers)
	}
	if links/2 != 99 { // 96 robots and 3 cloud nodes, each with the workstation
		t.Errorf("%d tunnels, want 99", links/2)
	}
	robot := result.Configs["robots/s1u-01-x86"]
	want := []string{"10.254.240.1/32", "10.254.240.0/22", "10.0.2.10/32", "10.0.2.11/32", "10.0.2.12/32"}
	if len(robot.Peers) != 1 || !reflect.DeepEqual(robot.Peers[0].AllowedIPs, want) || len(robot.UplinkRoutes) != 1 {
		t.Errorf("robot: %+v", robot)
	}
	cloud := result.Configs["ksyun/h100-0"]
	if len(cloud.Peers) != 1 || len(cloud.Peers[0].AllowedIPs) != 97 {
		t.Errorf("a cloud node answers the robots through the workstation: %d peers, %d prefixes", len(cloud.Peers), len(cloud.Peers[0].AllowedIPs))
	}

	members[0].Relay = false
	result, _ = Compute(topology, members)
	if len(result.Problems["robots/s1u-01-x86"]) != 1 || len(result.Configs["robots/s1u-01-x86"].Peers) != 0 {
		t.Errorf("a leaf without a relay is reported and peers with nobody: %v", result.Problems["robots/s1u-01-x86"])
	}
}

func TestNamesReachEveryMemberAndASitesOwnItsMembers(t *testing.T) {
	everywhere := []HostName{{Name: "cp.robots.rlark.internal", Address: "10.254.240.5"}}
	lab := []HostName{{Name: "registry.rlark.internal", Address: "10.11.14.128"}}
	topology := Topology{Pool: "10.254.240.0/24", MTU: 1380, Interface: "st-wg0", ListenPort: 51820, KeepaliveSeconds: 25, Names: everywhere, Sites: []Site{{Name: "lab", Names: lab}, {Name: "cloud"}}}
	result, err := Compute(topology, []Member{
		{ID: "robots/workstation", Site: "lab", Role: RoleHub, PublicKey: "a", Address: "10.254.240.5", Relay: true, Endpoints: []Endpoint{{Address: "192.168.31.55:51820", Scope: ScopeSite}}},
		{ID: "robots/robot", Site: "lab", Role: RoleLeaf, PublicKey: "b", Address: "10.254.240.7"},
		{ID: "cloud/gpu", Site: "cloud", Role: RoleHub, PublicKey: "c", Address: "10.254.240.2", Endpoints: []Endpoint{{Address: "1.2.3.4:51820", Scope: ScopePublic}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	both := append(append([]HostName{}, everywhere...), lab...)
	if !reflect.DeepEqual(result.Configs["robots/robot"].Names, both) || !reflect.DeepEqual(result.Configs["robots/workstation"].Names, both) || !reflect.DeepEqual(result.Configs["cloud/gpu"].Names, everywhere) {
		t.Fatalf("names: %+v", result.Configs)
	}
}

// A site declares the uplink of its leaves once: every leaf that has none of
// its own gets it, without a source (the leaf dials from its own address in
// the gateway's network), and a hub of the site does not.
func TestLeavesOfASiteShareItsUplink(t *testing.T) {
	topology, members := liveTopology()
	topology.Sites[1].LeafUplink = &Uplink{Gateway: "192.168.0.11"}
	members[2].Uplink = nil
	members = append(members,
		Member{ID: "rlark-robot-s1u-01/s1u-02-x86", Site: "lab", Role: RoleLeaf, PublicKey: strings.Repeat("E", 43) + "=", Address: "10.254.240.7"},
		Member{ID: "rlark-robot-s1u-01/own", Site: "lab", Role: RoleLeaf, PublicKey: strings.Repeat("F", 43) + "=", Address: "10.254.240.8",
			Uplink: &Uplink{Gateway: "10.9.9.1"}},
		Member{ID: "rlark-robot-s1u-01/hub", Site: "lab", Role: RoleHub, PublicKey: strings.Repeat("G", 43) + "=", Address: "10.254.240.9",
			Endpoints: []Endpoint{{Address: "10.11.13.200:51820", Scope: ScopeSite}}})
	result, err := Compute(topology, members)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"rlark-robot-s1u-01/s1u-01-x86", "rlark-robot-s1u-01/s1u-02-x86"} {
		routes := result.Configs[id].UplinkRoutes
		if len(routes) == 0 || len(result.Problems[id]) != 0 {
			t.Fatalf("%s: routes %+v problems %v", id, routes, result.Problems[id])
		}
		for _, r := range routes {
			if r.Gateway != "192.168.0.11" {
				t.Fatalf("%s: %+v", id, r)
			}
		}
	}
	if own := result.Configs["rlark-robot-s1u-01/own"].UplinkRoutes; len(own) == 0 || own[0].Gateway != "10.9.9.1" {
		t.Fatalf("a leaf's own uplink: %+v, problems %v", own, result.Problems["rlark-robot-s1u-01/own"])
	}
	if hub := result.Configs["rlark-robot-s1u-01/hub"].UplinkRoutes; len(hub) != 0 {
		t.Fatalf("a hub took the leaves' uplink: %+v", hub)
	}
}

// A site names the hub that relays for its leaves. Changing it changes, in a
// leaf's configuration, which peer carries the pool, and nothing else: the
// leaf's own sessions with the hubs it dials stay.
func TestASiteNamesItsRelay(t *testing.T) {
	topology := Topology{Pool: "10.254.240.0/24", MTU: 1380, Interface: "st-wg0", ListenPort: 51820, KeepaliveSeconds: 25, Sites: []Site{{Name: "field", Mesh: true}}}
	members := []Member{
		{ID: "robots/old", Site: "field", Role: RoleHub, Relay: true, PublicKey: "a", Address: "10.254.240.5", Endpoints: []Endpoint{{Address: "192.168.31.55:51820", Scope: ScopeSite}}},
		{ID: "robots/new", Site: "field", Role: RoleHub, Relay: true, PublicKey: "b", Address: "10.254.240.6", Endpoints: []Endpoint{{Address: "192.168.31.56:51820", Scope: ScopeSite}}},
		{ID: "robots/plain", Site: "field", Role: RoleHub, PublicKey: "c", Address: "10.254.240.4", Endpoints: []Endpoint{{Address: "192.168.31.54:51820", Scope: ScopeSite}}},
		{ID: "robots/robot-1", Site: "field", Role: RoleLeaf, PublicKey: "d", Address: "10.254.240.7"},
		{ID: "robots/robot-2", Site: "field", Role: RoleLeaf, PublicKey: "e", Address: "10.254.240.8"},
	}
	compute := func(relay string) Result {
		topology.Sites[0].Relay = relay
		result, err := Compute(topology, members)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	carrier := func(config NodeConfig) string {
		for _, peer := range config.Peers {
			if slices.Contains(peer.AllowedIPs, "10.254.240.0/24") {
				return peer.PublicKey
			}
		}
		return ""
	}
	before, after := compute(""), compute("robots/new")
	if before.Relays["robots/robot-1"] != "robots/old" || carrier(before.Configs["robots/robot-1"]) != "a" {
		t.Fatalf("without a named relay, the first of the site: %+v", before.Relays)
	}
	if after.Relays["robots/robot-1"] != "robots/new" || after.Relays["robots/robot-2"] != "robots/new" || carrier(after.Configs["robots/robot-1"]) != "b" {
		t.Fatalf("the named relay: %+v", after.Relays)
	}
	if len(after.Unreachable) != 0 || len(after.Problems) != 0 {
		t.Fatalf("every member reaches every other through the named relay: %v %v", after.Unreachable, after.Problems)
	}
	endpoints := func(config NodeConfig) (list []string) {
		for _, peer := range config.Peers {
			list = append(list, peer.PublicKey+"@"+peer.Endpoint)
		}
		return list
	}
	if !reflect.DeepEqual(endpoints(before.Configs["robots/robot-1"]), endpoints(after.Configs["robots/robot-1"])) {
		t.Fatalf("the leaf dials the same hubs before and after: %v, %v", endpoints(before.Configs["robots/robot-1"]), endpoints(after.Configs["robots/robot-1"]))
	}
	// A hub that does not relay, or a node there is none of, is not replaced by another.
	for _, wrong := range []string{"robots/plain", "robots/gone"} {
		result := compute(wrong)
		if _, has := result.Relays["robots/robot-1"]; has || len(result.Problems["robots/robot-1"]) != 1 || !strings.Contains(result.Problems["robots/robot-1"][0], wrong) {
			t.Fatalf("relay %s: %+v %v", wrong, result.Relays, result.Problems)
		}
	}
}

// The fields of NodeConfig at each level. A field that is added goes in a new
// line here, with DocumentLevel raised in the same commit.
func TestDocumentLevelNamesTheDocument(t *testing.T) {
	levels := []string{
		1: "address dns interface listenPort mtu names names.address names.name peers peers.allowedIPs peers.endpoint peers.keepaliveSeconds peers.publicKey routes uplinkRoutes uplinkRoutes.destination uplinkRoutes.gateway",
		2: "address discovery discovery.hostKeys discovery.intervalSeconds discovery.ports discovery.ranges discovery.relayFrom discovery.relayPort dns interface listenPort mtu names names.address names.name peers peers.allowedIPs peers.endpoint peers.keepaliveSeconds peers.publicKey routes uplinkRoutes uplinkRoutes.destination uplinkRoutes.gateway",
	}
	var fields []string
	var walk func(prefix string, of reflect.Type)
	walk = func(prefix string, of reflect.Type) {
		for i := range of.NumField() {
			name, _, _ := strings.Cut(of.Field(i).Tag.Get("json"), ",")
			fields = append(fields, prefix+name)
			if element := of.Field(i).Type; (element.Kind() == reflect.Slice || element.Kind() == reflect.Pointer) && element.Elem().Kind() == reflect.Struct {
				walk(prefix+name+".", element.Elem())
			}
		}
	}
	walk("", reflect.TypeOf(NodeConfig{}))
	sort.Strings(fields)
	if len(levels)-1 != DocumentLevel || strings.Join(fields, " ") != levels[DocumentLevel] {
		t.Fatalf("DocumentLevel %d does not name the document: %s", DocumentLevel, strings.Join(fields, " "))
	}
}

func TestOnlyTheHubsOfASiteThatDeclaresDiscoveryLook(t *testing.T) {
	looked := &Discovery{Ranges: []string{"192.168.31.0/24"}, IntervalSeconds: 30, Ports: []int{22}, HostKeys: []string{"SHA256:factory"}, RelayPort: DiscoveryRelayPort}
	key := func(n byte) string { return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{n}, 32)) }
	topology := Topology{Pool: "10.254.240.0/24", MTU: 1380, Interface: "st-wg0", ListenPort: 51820, KeepaliveSeconds: 25,
		Sites: []Site{{Name: "office"}, {Name: "lab", Reaches: []string{"office"}, Discovery: looked, DiscoveryFrom: "office/hub"}}}
	members := []Member{
		{ID: "office/hub", Site: "office", Role: RoleHub, PublicKey: key(1), Address: "10.254.240.1", Endpoints: []Endpoint{{Address: "10.11.14.128:51820", Scope: ScopeSite}}},
		{ID: "robots/cp", Site: "lab", Role: RoleHub, Relay: true, PublicKey: key(2), Address: "10.254.240.5", Endpoints: []Endpoint{{Address: "10.11.13.192:51820", Scope: ScopeSite}}},
		{ID: "robots/s1u-01-x86", Site: "lab", Role: RoleLeaf, PublicKey: key(3), Address: "10.254.240.3"},
	}
	result, err := Compute(topology, members)
	if err != nil || len(result.Problems) != 0 {
		t.Fatalf("compute: %v %v", err, result.Problems)
	}
	want := *looked
	want.RelayFrom = "10.254.240.1"
	if !reflect.DeepEqual(result.Configs["robots/cp"].Discovery, &want) || result.Configs["robots/s1u-01-x86"].Discovery != nil || result.Configs["office/hub"].Discovery != nil {
		t.Fatalf("discovery of the lab's hub %+v, its leaf %+v, the office's hub %+v", result.Configs["robots/cp"].Discovery, result.Configs["robots/s1u-01-x86"].Discovery, result.Configs["office/hub"].Discovery)
	}
	// A site whose discovery is from a node the network does not have: its hub is told nothing, and says why.
	topology.Sites[1].DiscoveryFrom = "office/gone"
	result, err = Compute(topology, members)
	if err != nil || result.Configs["robots/cp"].Discovery != nil || len(result.Problems["robots/cp"]) != 1 || !strings.Contains(result.Problems["robots/cp"][0], "office/gone") {
		t.Fatalf("discovery from a node that is no member: %v %+v %v", err, result.Configs["robots/cp"].Discovery, result.Problems)
	}
}
