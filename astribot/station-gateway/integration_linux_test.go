//go:build linux

package main

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Run only in a disposable network namespace with CAP_NET_ADMIN.
func TestKernelReconciliation(t *testing.T) {
	if os.Getenv("STATION_NETNS_TEST") != "1" {
		t.Skip("requires disposable privileged network namespace")
	}
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	d, err := parse([]byte(`{"interface":"wg-st-test","address":"10.253.0.1/32","listenPort":51820,"mtu":1420,"routes":["10.253.0.2/32"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = apply(d, key, false); err != nil {
		t.Fatal(err)
	}
	if err = apply(d, key, false); err != nil {
		t.Fatalf("restart reconcile: %v", err)
	}
	changed, err := parse([]byte(`{"interface":"wg-st-test","address":"10.253.0.9/32","listenPort":51821,"mtu":1420}`))
	if err != nil {
		t.Fatal(err)
	}
	if err = apply(changed, key, false); err == nil {
		t.Fatal("accepted address migration on existing interface")
	}
	linkBefore, err := netlink.LinkByName("wg-st-test")
	if err != nil {
		t.Fatal(err)
	}
	addresses, err := netlink.AddrList(linkBefore, netlink.FAMILY_V4)
	if err != nil || len(addresses) != 1 || addresses[0].IPNet.String() != "10.253.0.1/32" {
		t.Fatalf("address changed after rejected migration: %v %v", addresses, err)
	}
	if err = apply(d, key, false); err != nil {
		t.Fatalf("recover original configuration: %v", err)
	}
	uplink := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "station-uplink"}}
	if err = netlink.LinkAdd(uplink); err != nil {
		t.Fatal(err)
	}
	addr, _ := netlink.ParseAddr("192.0.2.10/24")
	if err = netlink.AddrAdd(uplink, addr); err != nil {
		t.Fatal(err)
	}
	if err = netlink.LinkSetUp(uplink); err != nil {
		t.Fatal(err)
	}
	d.UplinkRoutes = []uplinkRoute{{Destination: "198.51.100.1/32", Gateway: "192.0.2.1"}}
	if err = apply(d, key, false); err != nil {
		t.Fatalf("add uplink: %v", err)
	}
	if err = apply(d, key, false); err != nil {
		t.Fatalf("reconcile uplink: %v", err)
	}
	d.UplinkRoutes[0].Gateway = "203.0.113.1"
	if err = apply(d, key, false); err == nil {
		t.Fatal("accepted a gateway in no network of the node")
	}
	d.UplinkRoutes[0].Gateway = "192.0.2.1"
	_, uplinkDst, _ := net.ParseCIDR("198.51.100.1/32")
	conflict := netlink.Route{LinkIndex: uplink.Attrs().Index, Dst: uplinkDst, Gw: net.ParseIP("192.0.2.1"), Src: net.ParseIP("192.0.2.10"), Protocol: 99, Priority: routePriority, Table: 254}
	if err = netlink.RouteReplace(&conflict); err != nil {
		t.Fatal(err)
	}
	if err = apply(d, key, false); err == nil {
		t.Fatal("accepted foreign uplink route")
	}
	d.UplinkRoutes = nil
	link, err := netlink.LinkByName("wg-st-test")
	if err != nil {
		t.Fatal(err)
	}
	_, dst, _ := net.ParseCIDR("10.253.0.3/32")
	other := netlink.Route{LinkIndex: link.Attrs().Index, Dst: dst, Protocol: routeProtocol, Priority: routePriority, Table: 254, Type: 1}
	if err = netlink.RouteAdd(&other); err != nil {
		t.Fatal(err)
	}
	if err = apply(d, key, false); err == nil {
		t.Fatal("accepted ambiguous pre-existing route")
	}
	routes, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: dst}, netlink.RT_FILTER_DST)
	if err != nil || len(routes) != 1 {
		t.Fatalf("removed external route: %v %v", routes, err)
	}
	foreign := &netlink.GenericLink{LinkAttrs: netlink.LinkAttrs{Name: "wg-foreign"}, LinkType: "wireguard"}
	if err = netlink.LinkAdd(foreign); err != nil {
		t.Fatal(err)
	}
	d.Interface = "wg-foreign"
	if err = apply(d, key, false); err == nil {
		t.Fatal("adopted unowned interface")
	}
}

func TestKernelStatus(t *testing.T) {
	if os.Getenv("STATION_NETNS_TEST") != "1" {
		t.Skip("requires disposable privileged network namespace")
	}
	private, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	peerKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	d, err := parse([]byte(`{"interface":"wg-status","address":"10.253.5.1/32","listenPort":51822,"mtu":1420,"peers":[{"publicKey":"` + peerKey.PublicKey().String() + `","allowedIPs":["10.253.5.2/32"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := apply(d, private, false); err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "observation.json")
	configData, err := json.Marshal(d.config)
	if err != nil {
		t.Fatal(err)
	}
	if err := publishObservation(snapshot, configData, time.Now(), strings.Repeat("ab", 32)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var observation struct {
		Interface string   `json:"interface"`
		PublicKey string   `json:"publicKey"`
		Addresses []string `json:"addresses"`
	}
	if err := json.Unmarshal(data, &observation); err != nil {
		t.Fatal(err)
	}
	if observation.Interface != "wg-status" || observation.PublicKey != private.PublicKey().String() || len(observation.Addresses) == 0 || observation.Addresses[0] != "10.253.5.1/32" {
		t.Fatalf("unexpected snapshot: %s", data)
	}
	if !bytes.Contains(data, []byte(`"gateway":{"documentLevel":4,"digest":"`+strings.Repeat("ab", 32)+`"}`)) {
		t.Fatalf("snapshot does not state the program: %s", data)
	}
	if bytes.Contains(data, []byte(private.String())) || !bytes.Contains(data, []byte(`"peers":[{"publicKey":"`+peerKey.PublicKey().String()+`"}]`)) {
		t.Fatalf("snapshot leaks the private key or misses the peer: %s", data)
	}
	client, err := wgctrl.New()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.Device("wg-absent"); err == nil {
		t.Fatal("missing device accepted")
	}
	device, err := client.Device("wg-status")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := writeStatus(&out, device); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Interface string       `json:"interface"`
		PublicKey string       `json:"publicKey"`
		Peers     []peerStatus `json:"peers"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Interface != "wg-status" || got.PublicKey != private.PublicKey().String() || len(got.Peers) != 1 || got.Peers[0].PublicKey != peerKey.PublicKey().String() || got.Peers[0].LastHandshake != nil || got.Peers[0].ReceiveBytes != 0 || got.Peers[0].TransmitBytes != 0 {
		t.Fatalf("unexpected kernel status: %+v", got)
	}
}

// An enrolled node gains and loses peers without touching the others, and
// with pruning it removes the routes of a node that left.
func TestKernelIncrementalPeersAndPrunedRoutes(t *testing.T) {
	if os.Getenv("STATION_NETNS_TEST") != "1" {
		t.Skip("requires disposable privileged network namespace")
	}
	key, _ := wgtypes.GeneratePrivateKey()
	first, _ := wgtypes.GeneratePrivateKey()
	second, _ := wgtypes.GeneratePrivateKey()
	peer := func(k wgtypes.Key, address string) string {
		return `{"publicKey":"` + k.PublicKey().String() + `","allowedIPs":["` + address + `/32"],"endpoint":"192.0.2.9:51820","keepaliveSeconds":25}`
	}
	document := func(peers, routes string) *desired {
		d, err := parse([]byte(`{"interface":"wg-st-incr","address":"10.253.1.1/32","listenPort":51830,"mtu":1380,"peers":[` + peers + `],"routes":[` + routes + `]}`))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	client, err := wgctrl.New()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	one := document(peer(first, "10.253.1.2"), `"10.253.1.2/32"`)
	if err := apply(one, key, true); err != nil {
		t.Fatal(err)
	}
	both := document(peer(first, "10.253.1.2")+","+peer(second, "10.253.1.3"), `"10.253.1.2/32","10.253.1.3/32"`)
	device, _ := client.Device("wg-st-incr")
	if config, changed := deviceConfig(both, key, device); !changed || len(config.Peers) != 1 || config.Peers[0].PublicKey != second.PublicKey() || config.PrivateKey != nil || config.ReplacePeers {
		t.Fatalf("joining node must be the only change: %+v", config)
	}
	if err := apply(both, key, true); err != nil {
		t.Fatal(err)
	}
	if device, _ = client.Device("wg-st-incr"); len(device.Peers) != 2 {
		t.Fatalf("peers after join: %d", len(device.Peers))
	}
	if _, changed := deviceConfig(both, key, device); changed {
		t.Fatal("reconciling the same document must not touch the device")
	}
	if err := apply(one, key, false); err == nil {
		t.Fatal("without pruning a dropped route stops reconciliation")
	}
	if err := apply(one, key, true); err != nil {
		t.Fatal(err)
	}
	device, _ = client.Device("wg-st-incr")
	link, _ := netlink.LinkByName("wg-st-incr")
	routes, err := netlink.RouteList(link, netlink.FAMILY_V4)
	if err != nil || len(device.Peers) != 1 || device.Peers[0].PublicKey != first.PublicKey() || len(routes) != 1 || routes[0].Dst.String() != "10.253.1.2/32" {
		t.Fatalf("after the node left: peers %d, routes %v, %v", len(device.Peers), routes, err)
	}
}

// A node with no port of its own gets one from the kernel, and another when a
// peer it dials has stayed away.
func TestKernelTakesAnotherPort(t *testing.T) {
	if os.Getenv("STATION_NETNS_TEST") != "1" {
		t.Skip("requires disposable privileged network namespace")
	}
	key, _ := wgtypes.GeneratePrivateKey()
	hub, _ := wgtypes.GeneratePrivateKey()
	d, err := parse([]byte(`{"interface":"wg-st-port","address":"10.253.0.3/32","listenPort":0,"mtu":1420,"peers":[{"publicKey":"` + hub.PublicKey().String() + `","endpoint":"192.0.2.1:51820","allowedIPs":["10.253.0.1/32"],"keepaliveSeconds":25}]}`))
	if err != nil {
		t.Fatal(err)
	}
	c, err := wgctrl.New()
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	port := func() int {
		if err := apply(d, key, false); err != nil {
			t.Fatal(err)
		}
		device, err := c.Device(d.Interface)
		if err != nil {
			t.Fatal(err)
		}
		return device.ListenPort
	}
	dialling = redial{}
	first := port()
	if again := port(); first == 0 || again != first {
		t.Fatalf("ports %d then %d: the kernel's port must stay while the wait lasts", first, again)
	}
	dialling.since = time.Now().Add(-time.Minute)
	if second := port(); second == 0 || second == first {
		t.Fatalf("port %d after %d: a peer that stays away must cost a new port", second, first)
	}
}

// The uplink follows the document: the interface and the source are the
// node's own in the gateway's network unless the document names others, a
// route this program installed is replaced when they change and removed when
// the document drops it, and another origin's route is left alone.
func TestKernelUplinkFollowsTheDocument(t *testing.T) {
	if os.Getenv("STATION_NETNS_TEST") != "1" {
		t.Skip("requires disposable privileged network namespace")
	}
	lan := &netlink.Dummy{LinkAttrs: netlink.LinkAttrs{Name: "robot-lan"}}
	if err := netlink.LinkAdd(lan); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"192.168.0.10/24", "192.168.31.251/32"} {
		addr, _ := netlink.ParseAddr(address)
		if err := netlink.AddrAdd(lan, addr); err != nil {
			t.Fatal(err)
		}
	}
	if err := netlink.LinkSetUp(lan); err != nil {
		t.Fatal(err)
	}
	_, hub, _ := net.ParseCIDR("10.11.13.192/32")
	_, other, _ := net.ParseCIDR("10.11.14.128/32")
	routes := func(dst *net.IPNet) []netlink.Route {
		found, err := netlink.RouteListFiltered(netlink.FAMILY_V4, &netlink.Route{Dst: dst, Table: 254}, netlink.RT_FILTER_DST|netlink.RT_FILTER_TABLE)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	// A route this program installed earlier with another source (as a robot had: an address of the site's network).
	for _, dst := range []*net.IPNet{hub, other} {
		earlier := netlink.Route{LinkIndex: lan.Attrs().Index, Dst: dst, Gw: net.ParseIP("192.168.0.11"), Src: net.ParseIP("192.168.31.251"), Protocol: routeProtocol, Priority: routePriority, Table: 254}
		if err := netlink.RouteAdd(&earlier); err != nil {
			t.Fatal(err)
		}
	}
	// Another origin's route at its own metric stays, and is the one in use while it is there.
	foreign := netlink.Route{LinkIndex: lan.Attrs().Index, Dst: hub, Gw: net.ParseIP("192.168.0.11"), Src: net.ParseIP("192.168.31.251"), Protocol: 4, Priority: 100, Table: 254}
	if err := netlink.RouteAdd(&foreign); err != nil {
		t.Fatal(err)
	}
	// The site's declaration: the gateway alone.
	if err := applyUplink([]uplinkRoute{{Destination: hub.String(), Gateway: "192.168.0.11"}}, true); err != nil {
		t.Fatal(err)
	}
	found := routes(hub)
	if len(found) != 2 {
		t.Fatalf("routes to the hub: %+v", found)
	}
	for _, r := range found {
		want := "192.168.31.251"
		if r.Protocol == routeProtocol {
			want = "192.168.0.10"
		}
		if !r.Src.Equal(net.ParseIP(want)) || r.LinkIndex != lan.Attrs().Index {
			t.Fatalf("route %+v: source must be %s", r, want)
		}
	}
	if left := routes(other); len(left) != 0 {
		t.Fatalf("a route the document dropped stayed: %+v", left)
	}
	if err := applyUplink([]uplinkRoute{{Destination: hub.String(), Gateway: "192.168.1.1"}}, true); err == nil {
		t.Fatal("accepted a gateway in no network of the node")
	}
}
