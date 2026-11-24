package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestStatusOmitsSecretsAndShowsHandshake(t *testing.T) {
	private, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	peer, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 11, 1, 2, 3, 0, time.UTC)
	device := &wgtypes.Device{Name: "st-wg0", PrivateKey: private, PublicKey: private.PublicKey(), Peers: []wgtypes.Peer{
		{PublicKey: peer.PublicKey(), PresharedKey: peer, LastHandshakeTime: now, ReceiveBytes: 42, TransmitBytes: 84},
		{PublicKey: private.PublicKey()},
	}}
	var out bytes.Buffer
	if err := writeStatus(&out, device); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), private.String()) || strings.Contains(out.String(), peer.String()) {
		t.Fatal("status leaked a secret")
	}
	var result struct {
		Interface string       `json:"interface"`
		Peers     []peerStatus `json:"peers"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Interface != "st-wg0" || len(result.Peers) != 2 || result.Peers[0].LastHandshake == nil || !result.Peers[0].LastHandshake.Equal(now) || result.Peers[0].ReceiveBytes != 42 || result.Peers[0].TransmitBytes != 84 || result.Peers[1].LastHandshake != nil {
		t.Fatalf("unexpected status: %+v", result)
	}
}

func TestObservedPeersCarryKeyHandshakeAndCountersOnly(t *testing.T) {
	private, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	preshared, err := wgtypes.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 14, 1, 2, 3, 0, time.UTC)
	device := &wgtypes.Device{PrivateKey: private, Peers: []wgtypes.Peer{
		{PublicKey: private.PublicKey(), PresharedKey: preshared, Endpoint: &net.UDPAddr{IP: net.IPv4(203, 0, 113, 7), Port: 40000},
			AllowedIPs: []net.IPNet{{IP: net.IPv4(10, 254, 240, 9), Mask: net.CIDRMask(32, 32)}}, LastHandshakeTime: now, ReceiveBytes: 148, TransmitBytes: 92},
		{PublicKey: preshared.PublicKey()},
	}}
	data, err := json.Marshal(observedPeers(device))
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"publicKey":"` + private.PublicKey().String() + `","latestHandshake":"2026-11-14T01:02:03Z","receivedBytes":148,"sentBytes":92},{"publicKey":"` + preshared.PublicKey().String() + `"}]`
	if string(data) != want {
		t.Fatalf("peers = %s, want %s", data, want)
	}
}

// station-agent refuses a file with a field it does not know, so this is the
// file its test reads (TestAgentForwardsWhatTheGatewayStates in
// apps/rlark/astribot/stationidentity).
func TestObservationStatesTheProgram(t *testing.T) {
	observed := observation{ObservedAt: time.Date(2026, 11, 14, 1, 2, 3, 0, time.UTC), Interface: "st-wg0", Addresses: []string{"10.254.240.9/32"}, Peers: []peerObservation{}}
	observed.Gateway.DocumentLevel, observed.Gateway.Digest = documentLevel, strings.Repeat("ab", 32)
	data, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"observedAt":"2026-11-14T01:02:03Z","gateway":{"documentLevel":3,"digest":"` + strings.Repeat("ab", 32) + `"},"interface":"st-wg0","publicKey":"","addresses":["10.254.240.9/32"],"peers":[]}`
	if string(data) != want {
		t.Fatalf("observation = %s, want %s", data, want)
	}
}

func TestDigestOfTheRunningProgram(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	got, err := digest("/proc/self/exe")
	if sum := sha256.Sum256(content); err != nil || got != hex.EncodeToString(sum[:]) {
		t.Fatalf("digest = %s, %v", got, err)
	}
}

// The document's fields at each level. A field that is added goes in a new
// line here, with documentLevel raised in the same commit.
func TestDocumentLevelNamesTheDocument(t *testing.T) {
	levels := []string{
		1: "address dns interface listenPort mtu names names.address names.name nodeUID peers peers.allowedIPs peers.endpoint peers.keepaliveSeconds peers.nodeUID peers.publicKey publicKey routes uplinkRoutes uplinkRoutes.destination uplinkRoutes.gateway",
		2: "address discovery discovery.hostKeys discovery.intervalSeconds discovery.ports discovery.ranges discovery.relayFrom discovery.relayPort dns interface listenPort mtu names names.address names.name nodeUID peers peers.allowedIPs peers.endpoint peers.keepaliveSeconds peers.nodeUID peers.publicKey publicKey routes uplinkRoutes uplinkRoutes.destination uplinkRoutes.gateway",
		3: "address discovery discovery.hostKeys discovery.intervalSeconds discovery.ports discovery.ranges discovery.relayFrom discovery.relayPort dns forwards forwards.destination forwards.sources interface listenPort mtu names names.address names.name nodeUID peers peers.allowedIPs peers.endpoint peers.keepaliveSeconds peers.nodeUID peers.publicKey publicKey routes uplinkRoutes uplinkRoutes.destination uplinkRoutes.gateway",
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
	walk("", reflect.TypeOf(config{}))
	sort.Strings(fields)
	if len(levels)-1 != documentLevel || strings.Join(fields, " ") != levels[documentLevel] {
		t.Fatalf("documentLevel %d does not name the document: %s", documentLevel, strings.Join(fields, " "))
	}
}
