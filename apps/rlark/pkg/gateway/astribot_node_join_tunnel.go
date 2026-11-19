package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/astribot/stationidentity"
)

// tunnelAnswerWithin is how long Station API may stay silent through a tunnel whose peers have shaken hands.
const tunnelAnswerWithin = 2 * time.Minute

// joinClock is the time the tunnel's wait judges by.
var joinClock = time.Now

// tunnel starts the machine's own tunnel and waits for it on the evidence (awaitTunnel).
func (r *nodeJoinRun) tunnel(ctx context.Context, j *rlarkv1alpha1.NodeJoin) (string, error) {
	if err := routeSkip(j.Status.Facts.Route); err != nil {
		return "", err
	}
	machine, err := r.needMachine(ctx, j)
	if err != nil {
		return "", err
	}
	network, err := r.g.provisionNetwork(ctx)
	if err != nil {
		return "", err
	}
	document := network.declared(j.Spec.ClusterID, j.Spec.NodeName)
	if document == "" {
		return "", fmt.Errorf("SiteNetwork 的 status.declared 里没有 %s 的隧道文档：station-controller 还没接受这条声明", j.Spec.NodeName)
	}
	endpoint, err := joinTunnelEndpoint(j)
	if err != nil {
		return "", err
	}
	out, err := machine.Run(ctx, tunnelScript, strings.NewReader(document), nil)
	if err != nil {
		return "", err
	}
	started := lastLine(out, "tunnel service started")
	since, declared := timingOf(j.Status.Facts, "status.declared")
	if !declared {
		since = joinClock()
	}
	readback, err := r.g.awaitTunnel(ctx, machine, j.Spec.ClusterID+"/"+j.Spec.NodeName, document, endpoint, since, &j.Status.Facts,
		func(message string) { r.line(ctx, stageTunnel, message) })
	return started + "；" + readback, err
}

// awaitTunnel waits until Station API answers the machine self ("<cluster>/<node>") at its tunnel address, which is
// what a code's line redeems through and what the machine's control plane is reached by. Each round it reads the
// machine's handshakes and, for every peer the machine's document dials, whether the peer's latest report names the
// machine (its tunnel has taken the machine's key); the packets of the probe make the machine's kernel shake hands
// again, which it stops doing 90 s after an unanswered try. It fails on what stationidentity.JudgePropagation reads
// from that evidence, and after PropagationLongest and tunnelAnswerWithin from since whatever it reads; it records
// when each step was seen in facts.Timings and says where the wait stands.
func (g *Gateway) awaitTunnel(ctx context.Context, machine provisionMachine, self, document, endpoint string, since time.Time,
	facts *rlarkv1alpha1.NodeJoinFacts, say func(string)) (string, error) {
	address := g.stationBootstrap.TunnelAddress()
	state := stationidentity.PropagationState{Since: since}
	enrolled := stationidentity.EnrolledKeys(g.stationClient)
	probe := `/opt/rlark/bin/station-gateway --config /etc/rlark/station/bootstrap.json --status 2>/dev/null | sed 's/^/status=/'
curl --connect-timeout 5 -sk -o /dev/null -w 'code=%{http_code}\n' https://` + address + `/v1/bootstrap/ca || echo code=000
`
	var shaken time.Time
	for {
		out, err := machine.Run(ctx, probe, nil, nil)
		if err != nil {
			return "", err
		}
		values := provisionValues(out)
		var status struct {
			Peers []struct {
				PublicKey     string     `json:"publicKey"`
				LastHandshake *time.Time `json:"lastHandshake"`
			} `json:"peers"`
		}
		_ = json.Unmarshal([]byte(values["status"]), &status)
		handshakes := map[string]time.Time{}
		for _, peer := range status.Peers {
			if peer.LastHandshake != nil {
				handshakes[peer.PublicKey] = *peer.LastHandshake
			}
		}
		peers, keepalive, err := stationidentity.Propagation(ctx, g.stationClient, enrolled, self, document)
		if err != nil {
			return "", err
		}
		now := joinClock()
		for _, peer := range peers {
			if peer.HasKey {
				addTiming(facts, peer.Name+" 拿到密钥", now)
			}
			if at, ok := handshakes[peer.PublicKey]; ok {
				addTiming(facts, peer.Name+" 首次握手", at)
				if shaken.IsZero() {
					shaken = now
				}
			}
		}
		message, verdict := stationidentity.JudgePropagation(&state, peers, handshakes, keepalive, endpoint, now)
		if values["code"] == "200" {
			addTiming(facts, "Station API 经隧道应答", now)
			return fmt.Sprintf("%s；Station API %s 经隧道应答 200；%s；用时 %s", message, address, describeTimings(facts.Timings), now.Sub(since).Round(time.Second)), nil
		}
		if verdict != nil {
			return describeTimings(facts.Timings), verdict
		}
		if !shaken.IsZero() && now.Sub(shaken) > tunnelAnswerWithin {
			return describeTimings(facts.Timings), fmt.Errorf("隧道已经握手，但 Station API %s 经隧道 %s 内没有应答：%s", address, tunnelAnswerWithin, message)
		}
		// The longest the evidence allows, whatever it says: the wait does not hold its stage Running.
		if now.Sub(since) > stationidentity.PropagationLongest+tunnelAnswerWithin {
			return describeTimings(facts.Timings), fmt.Errorf("Station API %s 经隧道 %s 内没有应答：%s", address, stationidentity.PropagationLongest+tunnelAnswerWithin, message)
		}
		say(message)
		time.Sleep(joinWaitStep)
	}
}

// describeTimings is the join's moments as one line.
func describeTimings(timings []rlarkv1alpha1.NodeJoinTiming) string {
	parts := make([]string, 0, len(timings))
	for _, t := range timings {
		parts = append(parts, t.Event+" "+t.At.Local().Format("15:04:05"))
	}
	return strings.Join(parts, "，")
}
