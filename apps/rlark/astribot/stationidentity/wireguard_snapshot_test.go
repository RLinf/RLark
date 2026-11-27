package stationidentity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWireguardSnapshotFreshnessAndValidation(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "wireguard.json")
	if got, _ := readWireguardSnapshot(path, now); got != nil {
		t.Fatal("missing snapshot was accepted")
	}
	tests := []struct {
		name, body string
		valid      bool
	}{
		{"fresh with public key", fmt.Sprintf(`{"observedAt":%q,"interface":"st-wg0","publicKey":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","addresses":["10.254.240.1/32"]}`, now.Format(time.RFC3339)), true},
		{"legacy fresh", fmt.Sprintf(`{"observedAt":%q,"interface":"st-wg0","addresses":["10.254.240.1/32"]}`, now.Format(time.RFC3339)), true},
		{"invalid public key", fmt.Sprintf(`{"observedAt":%q,"interface":"st-wg0","publicKey":"bad","addresses":[]}`, now.Format(time.RFC3339)), false},
		{"stale", fmt.Sprintf(`{"observedAt":%q,"interface":"st-wg0","addresses":["10.254.240.1/32"]}`, now.Add(-31*time.Second).Format(time.RFC3339)), false},
		{"future", fmt.Sprintf(`{"observedAt":%q,"interface":"st-wg0","addresses":[]}`, now.Add(6*time.Second).Format(time.RFC3339)), false},
		{"invalid address", fmt.Sprintf(`{"observedAt":%q,"interface":"st-wg0","addresses":["bad"]}`, now.Format(time.RFC3339)), false},
		{"unknown field", fmt.Sprintf(`{"observedAt":%q,"interface":"st-wg0","addresses":[],"privateKey":"secret"}`, now.Format(time.RFC3339)), false},
		{"peers", fmt.Sprintf(`{"observedAt":%q,"interface":"st-wg0","addresses":[],"peers":[{"publicKey":"bGVhZi1rZXktbGVhZi1rZXktbGVhZi1rZXktbGVhZi0=","latestHandshake":%q,"receivedBytes":148,"sentBytes":92}]}`, now.Format(time.RFC3339), now.Add(-time.Minute).Format(time.RFC3339)), true},
		{"peer endpoint", fmt.Sprintf(`{"observedAt":%q,"interface":"st-wg0","addresses":[],"peers":[{"publicKey":"bGVhZi1rZXktbGVhZi1rZXktbGVhZi1rZXktbGVhZi0=","endpoint":"203.0.113.7:40000"}]}`, now.Format(time.RFC3339)), false},
		{"oversized", string(make([]byte, wireguardObservationBytes+1)), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Every file states the gateway that wrote it.
			body := strings.Replace(tc.body, `"interface"`, `"gateway":{"documentLevel":1,"digest":"`+strings.Repeat("ab", 32)+`"},"interface"`, 1)
			if err := os.WriteFile(path, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			got, _ := readWireguardSnapshot(path, now)
			if (got != nil) != tc.valid {
				t.Fatalf("accepted = %v, want %v", got != nil, tc.valid)
			}
			if tc.name == "peers" && !strings.Contains(string(got), `"peers":[{"publicKey":"bGVhZi1rZXktbGVhZi1rZXktbGVhZi1rZXktbGVhZi0=","latestHandshake":"2026-11-11T11:59:00Z","receivedBytes":148,"sentBytes":92}]`) {
				t.Fatalf("peers were not carried: %s", got)
			}
		})
	}
}

// The file is the one station-gateway writes (TestObservationStatesTheProgram
// in astribot/station-gateway).
func TestAgentForwardsWhatTheGatewayStates(t *testing.T) {
	now := time.Date(2026, 11, 14, 1, 2, 10, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "wireguard.json")
	agent, err := newAgent(AgentConfig{APIURL: "https://station-api", ClusterID: "cluster", TokenFile: "token", Interval: 30 * time.Second, RequestLimit: 5 * time.Second, WireguardObservationFile: path},
		func(string) ([]byte, error) { return []byte("token"), nil }, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	agent.observe = func() (Observation, error) { return Observation{}, nil }
	agent.program = AgentProgram{Digest: strings.Repeat("cd", 32)}
	var sent string
	agent.post = func(_ context.Context, _ string, body []byte) (int, []byte, error) {
		sent = string(body)
		var request observationRequest
		if err := json.Unmarshal(body, &request); err != nil {
			return 0, nil, err
		}
		return http.StatusAccepted, nil, validateObservation(request.Observation, now)
	}
	for name, c := range map[string]struct{ file, want string }{
		"a gateway that states its program": {
			`{"observedAt":"2026-11-14T01:02:03Z","gateway":{"documentLevel":1,"digest":"` + strings.Repeat("ab", 32) + `"},"interface":"st-wg0","publicKey":"","addresses":["10.254.240.9/32"],"peers":[]}`,
			`{"cluster_id":"cluster","observation":{"wireguard":{"interface":"st-wg0","addresses":["10.254.240.9/32"]},"gateway":{"documentLevel":1,"digest":"` + strings.Repeat("ab", 32) + `"},"agent":{"digest":"` + strings.Repeat("cd", 32) + `"}}}`,
		},
		"a gateway that stopped reconciling": {
			`{"observedAt":"2026-11-14T01:01:03Z","gateway":{"documentLevel":1,"digest":"` + strings.Repeat("ab", 32) + `"},"interface":"st-wg0","publicKey":"","addresses":[],"peers":[]}`,
			`{"cluster_id":"cluster","observation":{"agent":{"digest":"` + strings.Repeat("cd", 32) + `"}}}`,
		},
	} {
		if err := os.WriteFile(path, []byte(c.file), 0600); err != nil {
			t.Fatal(err)
		}
		if err := agent.Report(context.Background()); err != nil || sent != c.want {
			t.Errorf("%s: %v\n got %s\nwant %s", name, err, sent, c.want)
		}
	}
}

func TestAgentStatesTheFileItWasStartedFrom(t *testing.T) {
	agent, err := NewAgent(AgentConfig{APIURL: "https://station-api", ClusterID: "cluster", TokenFile: "token", Interval: 30 * time.Second, RequestLimit: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(content); agent.program.Digest != hex.EncodeToString(sum[:]) {
		t.Fatalf("digest = %s", agent.program.Digest)
	}
}
