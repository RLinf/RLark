package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// awaitKeyWindow is how long the peers' agents and gateways take, twice over, to take a new node's key
// (apps/rlark/astribot/stationidentity PropagationKeyWindow); without Station API's answer a wait ends after it and two
// keepalives and 15 s more.
const awaitKeyWindow = 2*(30+10)*time.Second + 2*10*time.Second

// propagation is Station API's answer to a node that waits for its peers (/v1/network/propagation).
type propagation struct {
	State   json.RawMessage `json:"state"`
	Message string          `json:"message"`
	Failure string          `json:"failure"`
}

// awaiter waits until a peer the node dials answers its tunnel: it ends when one has shaken hands, and fails when
// Station API's verdict on the peers' reports says none will.
type awaiter struct {
	handshakes func() (map[string]time.Time, error)
	ask        func(state json.RawMessage, handshakes map[string]time.Time) (propagation, error)
	say        func(string)
	now        func() time.Time
	sleep      func(time.Duration)
}

func (a awaiter) run(dialled map[string]bool, keepalive time.Duration) error {
	if len(dialled) == 0 {
		return nil
	}
	start := a.now()
	fallback := start.Add(awaitKeyWindow + 2*keepalive + 15*time.Second)
	var state json.RawMessage
	said := time.Time{}
	for {
		handshakes, err := a.handshakes()
		if err != nil {
			return err
		}
		mine := map[string]time.Time{}
		for key, at := range handshakes {
			if dialled[key] && !at.IsZero() {
				a.say(fmt.Sprintf("a peer answered through the tunnel (handshake %s)", at.Local().Format("15:04:05")))
				return nil
			}
			mine[key] = at
		}
		answer, err := a.ask(state, mine)
		switch {
		case err != nil && a.now().After(fallback):
			return fmt.Errorf("no peer this machine dials has answered through the tunnel after %s, and Station API cannot say why: %v", a.now().Sub(start).Round(time.Second), err)
		case err != nil:
			answer.Message = "Station API does not answer (" + err.Error() + "); waiting for a handshake"
		case answer.Failure != "":
			return errors.New(answer.Failure)
		default:
			state = answer.State
		}
		if a.now().Sub(said) >= 30*time.Second {
			said = a.now()
			a.say(answer.Message)
		}
		a.sleep(5 * time.Second)
	}
}

// awaitPeers is --await-peers: the node of the document at configPath waits for the peers it dials.
func awaitPeers(configPath string, key wgtypes.Key) error {
	from := sources{static: configPath}
	_, d, _, err := from.read()
	if err != nil {
		return err
	}
	dialled := map[string]bool{}
	keepalive := 25 * time.Second
	for _, p := range d.Peers {
		if p.Endpoint != "" {
			dialled[p.PublicKey] = true
			keepalive = max(keepalive, time.Duration(p.KeepaliveSeconds)*time.Second)
		}
	}
	client, err := wgctrl.New()
	if err != nil {
		return err
	}
	defer client.Close()
	directory := filepath.Dir(configPath)
	a := awaiter{
		handshakes: func() (map[string]time.Time, error) {
			device, err := client.Device(d.Interface)
			if err != nil {
				return nil, fmt.Errorf("read wireguard device: %w", err)
			}
			out := map[string]time.Time{}
			for _, p := range device.Peers {
				out[p.PublicKey.String()] = p.LastHandshakeTime
			}
			return out, nil
		},
		ask: func(state json.RawMessage, handshakes map[string]time.Time) (propagation, error) {
			url, ca, err := stationAPIOf(directory)
			if err != nil {
				return propagation{}, err
			}
			request := map[string]any{"handshakes": handshakes}
			if state != nil {
				request["state"] = state
			}
			status, data, err := askStationAPI(url, ca, key, "/v1/network/propagation", request)
			if err != nil {
				return propagation{}, err
			}
			if status != http.StatusOK {
				return propagation{}, fmt.Errorf("Station API at %s answered %d", url, status)
			}
			var answer propagation
			return answer, json.Unmarshal(data, &answer)
		},
		say:   func(text string) { fmt.Println(time.Now().Format("15:04:05") + " " + text) },
		now:   time.Now,
		sleep: time.Sleep,
	}
	return a.run(dialled, keepalive)
}
