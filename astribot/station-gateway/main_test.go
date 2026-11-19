package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestConfigValidation(t *testing.T) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	valid := fmt.Sprintf(`{"interface":"wg-site","address":"10.253.0.1/32","listenPort":51820,"mtu":1420,"peers":[{"publicKey":%q,"allowedIPs":["10.253.0.2/32"]}],"routes":["10.253.0.2/32"]}`, key.PublicKey().String())
	if _, err := parse([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	tests := []string{`{}`, `{"interface":"wg-site","address":"bad","listenPort":51820,"mtu":1420}`, `{"interface":"wg-site","address":"10.253.0.1/32","listenPort":51820,"mtu":1420,"routes":["0.0.0.0/0"]}`, `{"interface":"wg-site","address":"10.253.0.1/32","listenPort":51820,"mtu":1420,"unknown":1}`}
	for _, input := range tests {
		if _, err := parse([]byte(input)); err == nil {
			t.Errorf("accepted invalid config %s", input)
		}
	}
}

func TestUnchangedPeerKeepsLearnedEndpoint(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	peerKey, _ := wgtypes.GeneratePrivateKey()
	_, prefix, _ := net.ParseCIDR("10.254.240.3/32")
	keep := time.Duration(0)
	d := &desired{config: config{ListenPort: 51820}, peers: []wgtypes.PeerConfig{{PublicKey: peerKey.PublicKey(), AllowedIPs: []net.IPNet{*prefix}, PersistentKeepaliveInterval: &keep}}}
	endpoint := &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 1234}
	current := &wgtypes.Device{PrivateKey: key, ListenPort: 51820, Peers: []wgtypes.Peer{{PublicKey: peerKey.PublicKey(), AllowedIPs: []net.IPNet{*prefix}, Endpoint: endpoint}}}
	cfg, changed := deviceConfig(d, key, current)
	if changed || len(cfg.Peers) != 0 {
		t.Fatal("an unchanged peer must not be sent to the kernel, which keeps its learned endpoint")
	}
	d.ListenPort = 51821
	cfg, changed = deviceConfig(d, key, current)
	if !changed || cfg.ListenPort == nil || len(cfg.Peers) != 0 || cfg.PrivateKey != nil || cfg.ReplacePeers {
		t.Fatal("a changed device must not replace its peers")
	}
}

func TestPeerChangesAreIncremental(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	keep := 25 * time.Second
	var peers []wgtypes.PeerConfig
	var current []wgtypes.Peer
	for i := 0; i < 4; i++ {
		peerKey, _ := wgtypes.GeneratePrivateKey()
		_, prefix, _ := net.ParseCIDR(fmt.Sprintf("10.254.240.%d/32", i+2))
		peers = append(peers, wgtypes.PeerConfig{PublicKey: peerKey.PublicKey(), AllowedIPs: []net.IPNet{*prefix}, PersistentKeepaliveInterval: &keep})
		current = append(current, wgtypes.Peer{PublicKey: peerKey.PublicKey(), AllowedIPs: []net.IPNet{*prefix}, PersistentKeepaliveInterval: keep})
	}
	// The kernel has peers 0..2; the document drops 0, widens 1 and adds 3.
	_, pool, _ := net.ParseCIDR("10.254.240.0/24")
	peers[1].AllowedIPs = append(peers[1].AllowedIPs, *pool)
	d := &desired{config: config{ListenPort: 51820}, peers: peers[1:]}
	cfg, changed := deviceConfig(d, key, &wgtypes.Device{PrivateKey: key, ListenPort: 51820, Peers: current[:3]})
	if !changed || cfg.ReplacePeers || cfg.PrivateKey != nil || cfg.ListenPort != nil || len(cfg.Peers) != 3 {
		t.Fatalf("unexpected device update: %+v", cfg)
	}
	if cfg.Peers[0].PublicKey != peers[1].PublicKey || len(cfg.Peers[0].AllowedIPs) != 2 || cfg.Peers[1].PublicKey != peers[3].PublicKey || !cfg.Peers[2].Remove || cfg.Peers[2].PublicKey != peers[0].PublicKey {
		t.Fatalf("expected one update, one addition and one removal; peer 2 untouched: %+v", cfg.Peers)
	}
}

func TestConfigurationSourcesPreferComputedThenLastApplied(t *testing.T) {
	dir := t.TempDir()
	document := func(address string) []byte {
		return []byte(`{"interface":"st-wg0","address":"` + address + `/32","listenPort":51820,"mtu":1380}`)
	}
	from := sources{static: filepath.Join(dir, "static.json"), computed: filepath.Join(dir, "computed.json"), lastApplied: filepath.Join(dir, "last-applied.json")}
	if _, _, _, err := from.read(); err == nil {
		t.Fatal("no file is not a configuration")
	}
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(from.static, document("10.254.240.5"))
	if _, d, origin, err := from.read(); err != nil || origin != "static" || d.Address != "10.254.240.5/32" {
		t.Fatalf("before enrollment the static file applies: %s %v", origin, err)
	}
	write(from.lastApplied, document("10.254.240.6"))
	if _, _, origin, _ := from.read(); origin != "last-applied" {
		t.Fatalf("after a restart without Station API: %s", origin)
	}
	write(from.computed, document("10.254.240.7"))
	if _, d, origin, _ := from.read(); origin != "computed" || d.Address != "10.254.240.7/32" {
		t.Fatalf("enrolled node: %s", origin)
	}
	write(from.computed, []byte(`{"interface":"st-wg0","routes":["0.0.0.0/0"]}`))
	if _, _, origin, _ := from.read(); origin != "last-applied" {
		t.Fatalf("an invalid computed document must not be applied: %s", origin)
	}
	// Without enrollment only the static file is ever read.
	if _, _, origin, _ := (sources{static: from.static}).read(); origin != "static" {
		t.Fatalf("static-only deployment: %s", origin)
	}
}

func TestPublishIdentityHoldsOnlyThePublicKey(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	path := filepath.Join(t.TempDir(), "identity.json")
	if err := publishIdentity(path, key); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != `{"publicKey":"`+key.PublicKey().String()+`"}` {
		t.Fatalf("identity file %s", data)
	}
}
func TestKeyPersists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "identity", "private.key")
	a, err := keyFile(p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := keyFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("identity changed on restart")
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("unexpected key mode %s", fi.Mode())
	}
}

func TestInvalidKeyIsNotReplaced(t *testing.T) {
	p := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(p, []byte("corrupt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := keyFile(p); err == nil {
		t.Fatal("accepted corrupt identity")
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "corrupt\n" {
		t.Fatal("replaced existing identity")
	}
}
func TestDuplicatePeerRejected(t *testing.T) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	p := fmt.Sprintf(`{"publicKey":%q,"allowedIPs":["10.253.0.2/32"]}`, key.PublicKey().String())
	data := fmt.Sprintf(`{"interface":"wg-site","address":"10.253.0.1/32","listenPort":51820,"mtu":1420,"peers":[%s,%s]}`, p, p)
	if _, err = parse([]byte(data)); err == nil {
		t.Fatal("accepted duplicate peer")
	}
}

func TestNodeUIDOwnershipMetadataIsOptionalAndValidated(t *testing.T) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	valid := fmt.Sprintf(`{"interface":"wg-site","nodeUID":"node-uid","publicKey":%q,"address":"10.253.0.1/32","listenPort":51820,"mtu":1420,"peers":[{"publicKey":%q,"nodeUID":"peer-uid","allowedIPs":["10.253.0.2/32"]}]}`, key.PublicKey().String(), key.PublicKey().String())
	if _, err := parse([]byte(valid)); err != nil {
		t.Fatalf("valid ownership metadata rejected: %v", err)
	}
	invalid := strings.Replace(valid, key.PublicKey().String(), "not-a-key", 1)
	if _, err := parse([]byte(invalid)); err == nil {
		t.Fatal("invalid interface public key accepted")
	}
	legacy := `{"interface":"wg-site","address":"10.253.0.1/32","listenPort":51820,"mtu":1420}`
	if _, err := parse([]byte(legacy)); err != nil {
		t.Fatalf("legacy config without ownership metadata rejected: %v", err)
	}
	desired, err := parse([]byte(fmt.Sprintf(`{"interface":"wg-site","nodeUID":"node-uid","publicKey":%q,"address":"10.253.0.1/32","listenPort":51820,"mtu":1420}`, key.PublicKey().String())))
	if err != nil {
		t.Fatal(err)
	}
	other, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := apply(desired, other, false); err == nil || !strings.Contains(err.Error(), "does not match the host key") {
		t.Fatalf("mismatched configured public key: %v", err)
	}
}

func TestTrailingJSONRejected(t *testing.T) {
	data := []byte(`{"interface":"wg-site","address":"10.253.0.1/32","listenPort":51820,"mtu":1420} {}`)
	if _, err := parse(data); err == nil {
		t.Fatal("accepted trailing JSON object")
	}
}

func TestUplinkRouteValidation(t *testing.T) {
	base := `{"interface":"wg-site","address":"10.254.240.1/32","listenPort":51820,"mtu":1420,"uplinkRoutes":[%s]}`
	if _, err := parse([]byte(fmt.Sprintf(base, `{"destination":"10.11.14.128/32","gateway":"192.168.0.11"}`))); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		`{"destination":"10.11.0.0/16","gateway":"192.168.0.11"}`,
		`{"destination":"10.11.14.128/32","gateway":"bad"}`,
		`{"destination":"10.11.14.128/32","gateway":"192.168.0.11","source":"192.168.31.250"}`,
	} {
		if _, err := parse([]byte(fmt.Sprintf(base, bad))); err == nil {
			t.Errorf("accepted invalid uplink %s", bad)
		}
	}
}

// testdata/computed-s1u-01-x86.json is written by astribot/station's
// network package (TestComputedDocumentIsTheGatewayContract).
func TestParsesComputedDocument(t *testing.T) {
	data, err := os.ReadFile("testdata/computed-s1u-01-x86.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if d.Interface != "st-wg0" || len(d.peers) != 5 || len(d.routes) != 7 || len(d.UplinkRoutes) != 4 || d.peers[0].Endpoint.String() != "10.11.14.128:51820" || *d.peers[0].PersistentKeepaliveInterval != 25*time.Second {
		t.Fatalf("computed document parsed as %+v", d)
	}
}

func TestRedialTakesAnotherPortOnlyWhileADialledPeerStaysAway(t *testing.T) {
	hub, _ := wgtypes.GeneratePrivateKey()
	leaf, _ := wgtypes.GeneratePrivateKey()
	endpoint := &net.UDPAddr{IP: net.ParseIP("192.0.2.1"), Port: 51820}
	d := &desired{peers: []wgtypes.PeerConfig{{PublicKey: hub.PublicKey(), Endpoint: endpoint}, {PublicKey: leaf.PublicKey()}}}
	now := time.Unix(1_800_000_000, 0)
	device := func(hubHandshake time.Time) *wgtypes.Device {
		return &wgtypes.Device{Peers: []wgtypes.Peer{{PublicKey: hub.PublicKey(), LastHandshakeTime: hubHandshake}, {PublicKey: leaf.PublicKey()}}}
	}
	var r redial
	// A peer that is not dialled never counts, and neither does an answering one.
	if r.due(d, device(now.Add(-time.Minute)), now) || r.due(d, device(now.Add(59*time.Minute)), now.Add(time.Hour)) {
		t.Fatal("an answering peer asked for another port")
	}
	r = redial{}
	var taken []time.Duration
	for at := time.Duration(0); at <= 2*time.Hour; at += 10 * time.Second {
		if r.due(d, device(time.Time{}), now.Add(at)) {
			taken = append(taken, at)
		}
	}
	want := []time.Duration{30 * time.Second, 90 * time.Second, 210 * time.Second, 450 * time.Second, 930 * time.Second, 1890 * time.Second, 2850 * time.Second}
	if len(taken) < len(want) || !reflect.DeepEqual(taken[:len(want)], want) {
		t.Fatalf("ports were taken at %v, want %v first", taken, want)
	}
	// Once the peer answers the count starts again.
	if r.due(d, device(now.Add(2*time.Hour)), now.Add(2*time.Hour)) || r.wait != redialFirst {
		t.Fatalf("an answer did not reset the wait: %v", r.wait)
	}
}

func TestListenPortZeroLeavesThePortToTheKernel(t *testing.T) {
	key, _ := wgtypes.GeneratePrivateKey()
	d, err := parse([]byte(`{"interface":"st-wg0","address":"10.254.240.3/32","listenPort":0,"mtu":1380}`))
	if err != nil {
		t.Fatal(err)
	}
	if cfg, changed := deviceConfig(d, key, &wgtypes.Device{PrivateKey: key, ListenPort: 40123}); changed || cfg.ListenPort != nil {
		t.Fatalf("the kernel's port was replaced: %+v", cfg)
	}
}
