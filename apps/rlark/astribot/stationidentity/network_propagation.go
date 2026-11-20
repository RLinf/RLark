package stationidentity

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// How long a new node's key takes to reach its peers (astribot/station: station-controller recomputes every
// peer's document as soon as the SiteNetwork or an admission changes; an enrolled peer's station-agent asks for
// its document every 30 s, with a request timeout of 10 s; a peer's station-gateway applies the document it has
// every 10 s). Twice each, so that one missed round is not a failure.
const (
	PropagationKeyWindow = 2*(30+10)*time.Second + 2*10*time.Second
	// PropagationReportAge is the age of a peer's report beyond which it says nothing of now (station-controller's
	// linkReportAge).
	PropagationReportAge = 2 * time.Minute
	// PropagationLongest is how long a wait goes on that only peers without a fresh report hold up.
	PropagationLongest = 15 * time.Minute
)

// DeclaredKeys indexes the nodes the SiteNetwork declares with their key (a new site's machines, which have their
// tunnel before they redeem a code) by that key, as "<cluster>/<node>". It is read again as KeyIndex lists.
type DeclaredKeys struct {
	Client dynamic.Interface
	Now    func() time.Time

	mu    sync.Mutex
	names map[string]string
	read  time.Time
}

func (x *DeclaredKeys) Lookup(ctx context.Context, keys ...string) (map[string]string, error) {
	now := clock(x.Now)
	x.mu.Lock()
	defer x.mu.Unlock()
	age := now.Sub(x.read)
	for _, key := range keys {
		if x.names[key] == "" && age > 5*time.Second {
			age = time.Minute
		}
	}
	if age <= 30*time.Second {
		return x.names, nil
	}
	network, err := x.Client.Resource(siteNetworkGVR).Namespace("station-system").Get(ctx, "default", metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, err
	}
	x.names, x.read = map[string]string{}, now
	if network != nil {
		nodes, _, _ := unstructured.NestedSlice(network.Object, "spec", "nodes")
		for _, raw := range nodes {
			entry, _ := raw.(map[string]interface{})
			key, _ := entry["publicKey"].(string)
			cluster, _ := entry["clusterID"].(string)
			node, _ := entry["nodeName"].(string)
			if key != "" {
				x.names[key] = cluster + "/" + node
			}
		}
	}
	return x.names, nil
}

// memberName is how a node is named among its peers' observations: "<cluster>/<name>" of its enrollment or admission.
func memberName(object *unstructured.Unstructured) string {
	cluster, _, _ := unstructured.NestedString(object.Object, "spec", "clusterID")
	return cluster + "/" + object.GetName()
}

// PeerPropagation is what one peer a new node dials reports of the node: whether the peer's tunnel holds the node's
// key (its observation names the node, which it does only for a key in its kernel), and its last handshake with it.
type PeerPropagation struct {
	// Name is the peer as "<cluster>/<node>", or its key's beginning when no member holds the key.
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	// ObservedAt is when the peer last reported; nil, never.
	ObservedAt *time.Time `json:"observedAt,omitempty"`
	HasKey     bool       `json:"hasKey"`
	Handshake  *time.Time `json:"handshake,omitempty"`
}

// Propagation reads, for each peer the document of self ("<cluster>/<node>") dials, what the peer's latest
// observation says of self, and the longest keepalive of those peers.
func Propagation(ctx context.Context, client dynamic.Interface, enrolled *KeyIndex, self, document string) ([]PeerPropagation, time.Duration, error) {
	var config struct {
		Peers []struct {
			PublicKey        string `json:"publicKey"`
			Endpoint         string `json:"endpoint"`
			KeepaliveSeconds int    `json:"keepaliveSeconds"`
		} `json:"peers"`
	}
	if err := json.Unmarshal([]byte(document), &config); err != nil {
		return nil, 0, fmt.Errorf("the node's document: %w", err)
	}
	var keys []string
	keepalive := time.Duration(0)
	for _, peer := range config.Peers {
		if peer.Endpoint != "" {
			keys = append(keys, peer.PublicKey)
			keepalive = max(keepalive, time.Duration(peer.KeepaliveSeconds)*time.Second)
		}
	}
	members, err := enrolled.Lookup(ctx, keys...)
	if err != nil {
		return nil, 0, fmt.Errorf("list NetworkEnrollments: %w", err)
	}
	peers := make([]PeerPropagation, 0, len(keys))
	for _, key := range keys {
		peer := PeerPropagation{Name: "key " + key[:min(8, len(key))], PublicKey: key}
		if member := members[key]; member != nil {
			peer.Name = memberName(member)
			// The index is up to 30 s old; the peer's report is read now.
			observation, err := client.Resource(networkEnrollmentGVR).Namespace(member.GetNamespace()).Get(ctx, member.GetName(), metav1.GetOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				return nil, 0, fmt.Errorf("read the observation of %s: %w", peer.Name, err)
			}
			if err == nil {
				if reported, _, _ := unstructured.NestedString(observation.Object, "status", "observation", "reportedAt"); reported != "" {
					if at, err := time.Parse(time.RFC3339Nano, reported); err == nil {
						peer.ObservedAt = &at
					}
				}
				seen, _, _ := unstructured.NestedSlice(observation.Object, "status", "observation", "wireguard", "peers")
				for _, raw := range seen {
					entry, _ := raw.(map[string]interface{})
					if entry["name"] != self {
						continue
					}
					peer.HasKey = true
					if handshake, _ := entry["latestHandshake"].(string); handshake != "" {
						if at, err := time.Parse(time.RFC3339Nano, handshake); err == nil {
							peer.Handshake = &at
						}
					}
				}
			}
		}
		peers = append(peers, peer)
	}
	return peers, keepalive, nil
}

// PropagationState is what a wait remembers between rounds: when it started (the node's key was declared or
// redeemed) and when each peer was first seen holding the key.
type PropagationState struct {
	Since time.Time            `json:"since"`
	Keyed map[string]time.Time `json:"keyed,omitempty"`
}

// JudgePropagation takes one round of the peers' reports and the node's own handshakes (by the peer's key) into
// the state, and says where the wait stands; an error is a wait that the evidence says will not end: a peer whose
// fresh report still lacks the key after PropagationKeyWindow, peers that all hold it and do not shake hands for two
// keepalives and 15 s (UDP to endpoint does not reach the node), or, when only peers without a fresh report are
// left, PropagationLongest.
func JudgePropagation(state *PropagationState, peers []PeerPropagation, handshakes map[string]time.Time, keepalive time.Duration, endpoint string, now time.Time) (string, error) {
	if state.Keyed == nil {
		state.Keyed = map[string]time.Time{}
	}
	if keepalive == 0 {
		keepalive = 25 * time.Second
	}
	var keyed, missing, silent, shaken []string
	fresh := 0
	lastKeyed := state.Since
	for _, peer := range peers {
		reported := peer.ObservedAt != nil && now.Sub(*peer.ObservedAt) <= PropagationReportAge
		// The node's own handshake counts always; a peer's report of one only when the report is fresh and the
		// handshake is not older than the wait (an earlier machine of the same name).
		handshake, ok := handshakes[peer.PublicKey]
		if !ok && reported && peer.Handshake != nil && !peer.Handshake.Before(state.Since) {
			handshake, ok = *peer.Handshake, true
		}
		if ok && !handshake.IsZero() {
			shaken = append(shaken, peer.Name+" "+handshake.Local().Format("15:04:05"))
		}
		if !reported {
			silent = append(silent, peer.Name)
			continue
		}
		fresh++
		if !peer.HasKey {
			missing = append(missing, peer.Name)
			continue
		}
		keyed = append(keyed, peer.Name)
		if _, seen := state.Keyed[peer.Name]; !seen {
			state.Keyed[peer.Name] = now
		}
		if at := state.Keyed[peer.Name]; at.After(lastKeyed) {
			lastKeyed = at
		}
	}
	message := fmt.Sprintf("peer 拿到密钥 %d/%d", len(keyed), fresh)
	if len(missing) > 0 {
		message += "（等 " + strings.Join(missing, ", ") + "）"
	}
	if len(shaken) > 0 {
		message += "；握手：" + strings.Join(shaken, ", ")
	} else {
		message += "；还没有握手"
	}
	if len(silent) > 0 {
		message += "；无报告：" + strings.Join(silent, ", ")
	}
	switch {
	case len(missing) > 0 && now.Sub(state.Since) > PropagationKeyWindow:
		return message, fmt.Errorf("%s 在 %s 内没有取走新节点的文档（看它的 station-agent / station-gateway）：%s", strings.Join(missing, ", "), PropagationKeyWindow, message)
	case fresh > 0 && len(missing) == 0 && len(shaken) == 0 && now.Sub(lastKeyed) > 2*keepalive+15*time.Second:
		return message, fmt.Errorf("peer 已有密钥但没有握手：UDP %s 没到这台机器（安全组 / 转发）：%s", endpoint, message)
	case fresh == 0 && len(shaken) == 0 && now.Sub(state.Since) > PropagationLongest:
		return message, fmt.Errorf("%s 内没有一个 peer 有新的报告，也没有握手：%s", PropagationLongest, message)
	}
	return message, nil
}

type propagationRequest struct {
	documentRequest
	State PropagationState `json:"state"`
	// Handshakes are the node's own last handshakes, by the peer's key.
	Handshakes map[string]time.Time `json:"handshakes,omitempty"`
}

// ServePropagation answers a node that waits for its peers to take its key (station-gateway --await-peers), for its
// own identity: its peers' reports of it and the verdict of JudgePropagation, with the state it sends back next time.
func (d *NetworkDocuments) ServePropagation(w http.ResponseWriter, r *http.Request) {
	var request propagationRequest
	if !decodeBody(w, r, &request) || !d.proven(w, request.documentRequest) {
		return
	}
	holder, err := d.holder(r.Context(), request.PublicKey)
	if err != nil {
		log.Printf("network propagation: %v", err)
		writeError(w, http.StatusServiceUnavailable, "network document storage unavailable")
		return
	}
	if holder == nil {
		writeError(w, http.StatusNotFound, "no document for this key")
		return
	}
	document, _, _ := unstructured.NestedString(holder.Object, "status", "config")
	peers, keepalive, err := Propagation(r.Context(), d.Client, d.Enrolled, memberName(holder), document)
	if err != nil {
		log.Printf("network propagation: %v", err)
		writeError(w, http.StatusServiceUnavailable, "the peers' observations are unavailable")
		return
	}
	now := clock(d.Now)
	if request.State.Since.IsZero() || request.State.Since.After(now) {
		request.State.Since = now
	}
	message, verdict := JudgePropagation(&request.State, peers, request.Handshakes, keepalive, "", now)
	answer := map[string]any{"peers": peers, "state": request.State, "message": message}
	if verdict != nil {
		answer["failure"] = verdict.Error()
	}
	writeJSON(w, http.StatusOK, answer)
}
