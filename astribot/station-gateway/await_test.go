package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type awaitClock struct{ now time.Time }

func (c *awaitClock) awaiter(handshakes func() map[string]time.Time, ask func(json.RawMessage) (propagation, error), said *[]string) awaiter {
	return awaiter{
		handshakes: func() (map[string]time.Time, error) { return handshakes(), nil },
		ask: func(state json.RawMessage, _ map[string]time.Time) (propagation, error) {
			return ask(state)
		},
		say:   func(text string) { *said = append(*said, text) },
		now:   func() time.Time { return c.now },
		sleep: func(d time.Duration) { c.now = c.now.Add(d) },
	}
}

// The wait ends at the first handshake with a peer the node dials, carries Station API's state from round to round,
// and fails with Station API's verdict.
func TestAwaitPeersEndsAtAHandshakeAndFailsOnStationAPIsVerdict(t *testing.T) {
	clock := &awaitClock{now: time.Unix(1_800_000_000, 0)}
	start := clock.now
	var said, states []string
	a := clock.awaiter(func() map[string]time.Time {
		if clock.now.Sub(start) >= 20*time.Second {
			return map[string]time.Time{"other": clock.now, "hub": clock.now}
		}
		return map[string]time.Time{"other": clock.now, "hub": {}}
	}, func(state json.RawMessage) (propagation, error) {
		states = append(states, string(state))
		return propagation{State: json.RawMessage(`{"since":"x"}`), Message: "peer 拿到密钥 1/2"}, nil
	}, &said)
	if err := a.run(map[string]bool{"hub": true}, 25*time.Second); err != nil {
		t.Fatal(err)
	}
	if len(states) != 4 || states[0] != "" || states[1] != `{"since":"x"}` || said[0] != "peer 拿到密钥 1/2" || !strings.Contains(said[len(said)-1], "a peer answered") {
		t.Fatalf("states %q said %q", states, said)
	}

	a = clock.awaiter(func() map[string]time.Time { return nil }, func(json.RawMessage) (propagation, error) {
		return propagation{Failure: "baidu/gpu-1 没有取走新节点的文档"}, nil
	}, &said)
	if err := a.run(map[string]bool{"hub": true}, 25*time.Second); err == nil || !strings.Contains(err.Error(), "没有取走") {
		t.Fatalf("err %v", err)
	}
	// A document that dials no peer has nothing to wait for.
	if err := a.run(nil, 0); err != nil {
		t.Fatal(err)
	}
}

// Without Station API the wait ends at the time the propagation takes at most: the key window, two keepalives, 15 s.
func TestAwaitPeersWithoutStationAPIEndsAtTheLongestPropagation(t *testing.T) {
	clock := &awaitClock{now: time.Unix(1_800_000_000, 0)}
	start := clock.now
	var said []string
	a := clock.awaiter(func() map[string]time.Time { return nil }, func(json.RawMessage) (propagation, error) {
		return propagation{}, errors.New("Station API is not reached")
	}, &said)
	err := a.run(map[string]bool{"hub": true}, 25*time.Second)
	if err == nil || !strings.Contains(err.Error(), "cannot say why") {
		t.Fatalf("err %v", err)
	}
	if waited := clock.now.Sub(start); waited < awaitKeyWindow+65*time.Second || waited > awaitKeyWindow+75*time.Second {
		t.Fatalf("waited %s", waited)
	}
}
