package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// peerObservation is what a hub knows of a node that has dropped off: whether
// a session formed and whether anything still arrives.
type peerObservation struct {
	PublicKey       string     `json:"publicKey"`
	LatestHandshake *time.Time `json:"latestHandshake,omitempty"`
	ReceivedBytes   int64      `json:"receivedBytes,omitempty"`
	SentBytes       int64      `json:"sentBytes,omitempty"`
}

func observedPeers(device *wgtypes.Device) []peerObservation {
	peers := make([]peerObservation, 0, len(device.Peers))
	for _, peer := range device.Peers {
		observed := peerObservation{PublicKey: peer.PublicKey.String(), ReceivedBytes: peer.ReceiveBytes, SentBytes: peer.TransmitBytes}
		if !peer.LastHandshakeTime.IsZero() {
			handshake := peer.LastHandshakeTime.UTC()
			observed.LatestHandshake = &handshake
		}
		peers = append(peers, observed)
	}
	return peers
}

// observation is the file station-agent forwards. Gateway is this program:
// the sha256 of its file, which is what the keeper and an image digest tell
// builds apart by, and the document level it was compiled with.
type observation struct {
	ObservedAt time.Time `json:"observedAt"`
	Gateway    struct {
		DocumentLevel int    `json:"documentLevel"`
		Digest        string `json:"digest"`
	} `json:"gateway"`
	Interface string            `json:"interface"`
	PublicKey string            `json:"publicKey"`
	Addresses []string          `json:"addresses"`
	Peers     []peerObservation `json:"peers"`
}

// publishObservation writes this program's digest and document level, the
// interface, its public key, its addresses and, for every peer,
// its public key, the time of its last handshake and the bytes received from
// and sent to it. It never includes the device's keys, a preshared key, an
// endpoint or allowed addresses.
func publishObservation(path string, data []byte, now time.Time, program string) error {
	d, err := parse(data)
	if err != nil {
		return err
	}
	link, err := netlink.LinkByName(d.Interface)
	if err != nil {
		return err
	}
	if link.Type() != "wireguard" || link.Attrs().Alias != owner {
		return fmt.Errorf("interface is not owned by station gateway")
	}
	deviceClient, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("open wireguard device: %w", err)
	}
	device, err := deviceClient.Device(d.Interface)
	closeErr := deviceClient.Close()
	if err != nil {
		return fmt.Errorf("read wireguard public key: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("close wireguard device client: %w", closeErr)
	}
	addresses, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return err
	}
	values := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if address.IPNet != nil {
			values = append(values, address.IPNet.String())
		}
	}
	sort.Strings(values)
	observed := observation{ObservedAt: now.UTC(), Interface: d.Interface, PublicKey: device.PublicKey.String(), Addresses: values, Peers: observedPeers(device)}
	observed.Gateway.DocumentLevel, observed.Gateway.Digest = documentLevel, program
	content, err := json.Marshal(observed)
	if err != nil {
		return err
	}
	return replaceFile(path, bytes.NewReader(content), 0644)
}

type peerStatus struct {
	PublicKey     string     `json:"publicKey"`
	LastHandshake *time.Time `json:"lastHandshake"`
	ReceiveBytes  int64      `json:"receiveBytes"`
	TransmitBytes int64      `json:"transmitBytes"`
}

// writeStatus deliberately projects only non-secret operational fields.
func writeStatus(w io.Writer, device *wgtypes.Device) error {
	peers := make([]peerStatus, 0, len(device.Peers))
	for _, peer := range device.Peers {
		var handshake *time.Time
		if !peer.LastHandshakeTime.IsZero() {
			value := peer.LastHandshakeTime.UTC()
			handshake = &value
		}
		peers = append(peers, peerStatus{
			PublicKey: peer.PublicKey.String(), LastHandshake: handshake,
			ReceiveBytes: peer.ReceiveBytes, TransmitBytes: peer.TransmitBytes,
		})
	}
	return json.NewEncoder(w).Encode(struct {
		Interface string       `json:"interface"`
		PublicKey string       `json:"publicKey"`
		Peers     []peerStatus `json:"peers"`
	}{device.Name, device.PublicKey.String(), peers})
}
