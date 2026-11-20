package stationidentity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testRegistry struct {
	readers map[string]IdentityReader
	err     error
}

func (r testRegistry) Reader(_ context.Context, id string) (IdentityReader, error) {
	if r.err != nil {
		return nil, r.err
	}
	reader := r.readers[id]
	if reader == nil {
		return nil, ErrClusterNotRegistered
	}
	return reader, nil
}

type testObservationSink struct {
	items    []Observation
	evidence []Evidence
	err      error
}

func (s *testObservationSink) Store(_ context.Context, e Evidence, o Observation) error {
	if s.err != nil {
		return s.err
	}
	s.evidence = append(s.evidence, e)
	s.items = append(s.items, o)
	return nil
}

func apiRequest(handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// testAgent is the station-agent every report states.
var testAgent = `"agent":{"digest":"` + strings.Repeat("cd", 32) + `"}`

func TestHTTPObservationAuthenticatesAndReturnsGenericFailures(t *testing.T) {
	reader := &fakeReader{evidence: validEvidence()}
	sink := &testObservationSink{}
	now := time.Unix(2000, 0)
	api := &API{Registry: testRegistry{readers: map[string]IdentityReader{"c": reader}}, Observations: sink, Now: func() time.Time { return now }}
	body := `{"cluster_id":"c","observation":{` + testAgent + `,"systemUUID":"uuid","wireguard":{"interface":"st-wg0"},"probes":[{"target":"nas:445","ok":true}]}}`
	if w := apiRequest(api, http.MethodPost, "/v1/observations", body, ""); w.Code != 401 {
		t.Fatalf("missing auth status=%d", w.Code)
	}
	if w := apiRequest(api, http.MethodPost, "/v1/observations", body, "token"); w.Code != 202 {
		t.Fatalf("valid status=%d body=%s", w.Code, w.Body.String())
	}
	if sink.evidence[0].NodeUID != "n" || sink.items[0].SystemUUID != "uuid" {
		t.Fatal("stored observation was not attributed to validated subject")
	}
	if w := apiRequest(api, http.MethodPost, "/v1/observations", `{"cluster_id":"c","unexpected":true}`, "token"); w.Code != 400 {
		t.Fatalf("unknown field status=%d", w.Code)
	}
	for _, invalid := range []string{
		`{"cluster_id":"c","observation":{` + testAgent + `,"wireguard":{}}}`,
		`{"cluster_id":"c","observation":{` + testAgent + `,"wireguard":{"interface":"st-wg0","privateKey":"secret"}}}`,
		`{"cluster_id":"c","observation":{` + testAgent + `,"wireguard":{"interface":"st-wg0","publicKey":"not-a-wireguard-key"}}}`,
		`{"cluster_id":"c","observation":{` + testAgent + `,"wireguard":{"interface":"st-wg0","peers":[{"name":"peer","receivedBytes":-1}]}}}`,
		`{"cluster_id":"c","observation":{` + testAgent + `,"wireguard":{"interface":"st-wg0","addresses":["10.0.0.1/32","10.0.0.1/32"]}}}`,
	} {
		if w := apiRequest(api, http.MethodPost, "/v1/observations", invalid, "token"); w.Code != http.StatusBadRequest {
			t.Fatalf("invalid observation accepted: status=%d body=%s", w.Code, w.Body.String())
		}
	}
	for _, malformed := range []string{body + ` {}`, strings.Repeat(" ", 1<<20) + body} {
		if w := apiRequest(api, http.MethodPost, "/v1/observations", malformed, "token"); w.Code != http.StatusBadRequest {
			t.Fatalf("malformed or oversized request status=%d", w.Code)
		}
	}
	badType := httptest.NewRequest(http.MethodPost, "/v1/observations", strings.NewReader(body))
	badType.Header.Set("Content-Type", "text/plain")
	badType.Header.Set("Authorization", "Bearer token")
	badTypeResponse := httptest.NewRecorder()
	api.ServeHTTP(badTypeResponse, badType)
	if badTypeResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("content type status=%d", badTypeResponse.Code)
	}
	api.Registry = testRegistry{err: errors.New("secret cluster configuration")}
	w := apiRequest(api, http.MethodPost, "/v1/observations", body, "token")
	if w.Code != 503 || strings.Contains(w.Body.String(), "secret") {
		t.Fatalf("registry failure leaked or wrong status: %d %s", w.Code, w.Body.String())
	}
	api.Registry = testRegistry{readers: map[string]IdentityReader{"c": reader}}
	reader.err = ErrIdentityUnavailable
	if w := apiRequest(api, http.MethodPost, "/v1/observations", body, "token"); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("identity dependency failure status=%d body=%s", w.Code, w.Body.String())
	}
	reader.err = errors.New("invalid bearer token")
	if w := apiRequest(api, http.MethodPost, "/v1/observations", body, "token"); w.Code != http.StatusUnauthorized {
		t.Fatalf("invalid identity status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestHTTPReadinessIsSeparateFromLiveness(t *testing.T) {
	api := &API{}
	if got := apiRequest(api, http.MethodGet, "/healthz", "", ""); got.Code != http.StatusOK {
		t.Fatalf("health status=%d", got.Code)
	}
	if got := apiRequest(api, http.MethodGet, "/readyz", "", ""); got.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured readiness status=%d", got.Code)
	}
	api.Ready = func(context.Context) error { return errors.New("private dependency detail") }
	if got := apiRequest(api, http.MethodGet, "/readyz", "", ""); got.Code != http.StatusServiceUnavailable || strings.Contains(got.Body.String(), "private dependency detail") {
		t.Fatalf("dependency failure leaked or wrong readiness status: %d %s", got.Code, got.Body.String())
	}
	api.Ready = func(context.Context) error { return nil }
	if got := apiRequest(api, http.MethodGet, "/readyz", "", ""); got.Code != http.StatusOK {
		t.Fatalf("ready status=%d", got.Code)
	}
}

func TestHTTPObservationRateLimitRunsAfterIdentityAndBeforePersistence(t *testing.T) {
	reader := &fakeReader{evidence: validEvidence()}
	sink := &testObservationSink{}
	limiter := &NodeLimiter{Rate: 1, Burst: 1}
	now := time.Unix(5000, 0)
	api := &API{
		Registry:     testRegistry{readers: map[string]IdentityReader{"c": reader}},
		Observations: sink, ObservationLimiter: limiter, Now: func() time.Time { return now },
	}
	body := `{"cluster_id":"c","observation":{` + testAgent + `}}`
	if got := apiRequest(api, http.MethodPost, "/v1/observations", body, "token"); got.Code != http.StatusAccepted {
		t.Fatalf("initial observation status=%d %s", got.Code, got.Body.String())
	}
	limited := apiRequest(api, http.MethodPost, "/v1/observations", body, "token")
	if limited.Code != http.StatusTooManyRequests || limited.Header().Get("Retry-After") != "1" {
		t.Fatalf("limited response=%d retry-after=%q", limited.Code, limited.Header().Get("Retry-After"))
	}
	if len(sink.items) != 1 {
		t.Fatalf("rate-limited observation persisted; count=%d", len(sink.items))
	}
	now = now.Add(time.Second)
	if got := apiRequest(api, http.MethodPost, "/v1/observations", body, "token"); got.Code != http.StatusAccepted {
		t.Fatalf("observation was not allowed after refill: %d %s", got.Code, got.Body.String())
	}
}

func TestWireguardPeerObservationValidation(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	key, other := "bGVhZi1rZXktbGVhZi1rZXktbGVhZi1rZXktbGVhZi0=", "bW92ZWQta2V5LW1vdmVkLWtleS1tb3ZlZC1rZXktbW8="
	many := make([]string, wireguardObservationPeers+1)
	for i := range many {
		many[i] = `{"publicKey":"` + base64.StdEncoding.EncodeToString(fmt.Appendf(nil, "%032d", i)) + `"}`
	}
	tests := []struct {
		name, peers string
		valid       bool
	}{
		{"no peers", ``, true},
		{"key, handshake and counters", `{"publicKey":"` + key + `","latestHandshake":"2026-11-14T12:00:50Z","receivedBytes":148,"sentBytes":92},{"publicKey":"` + other + `"}`, true},
		{"as many as allowed", strings.Join(many[1:], ","), true},
		{"too many", strings.Join(many, ","), false},
		{"a name the node chose", `{"publicKey":"` + key + `","name":"robot-01/s1u-01-x86"}`, false},
		{"no key", `{"receivedBytes":148}`, false},
		{"not a key", `{"publicKey":"c2hvcnQ="}`, false},
		{"the same key twice", `{"publicKey":"` + key + `"},{"publicKey":"` + key + `"}`, false},
		{"negative received", `{"publicKey":"` + key + `","receivedBytes":-1}`, false},
		{"negative sent", `{"publicKey":"` + key + `","sentBytes":-1}`, false},
		{"handshake in the future", `{"publicKey":"` + key + `","latestHandshake":"2026-11-14T12:01:01Z"}`, false},
		{"an endpoint", `{"publicKey":"` + key + `","endpoint":"203.0.113.7:40000"}`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw := `{"interface":"st-wg0"}`
			if tc.peers != "" {
				raw = `{"interface":"st-wg0","peers":[` + tc.peers + `]}`
			}
			if err := validateObservation(Observation{Wireguard: json.RawMessage(raw), Agent: AgentProgram{Digest: strings.Repeat("cd", 32)}}, now); (err == nil) != tc.valid {
				t.Fatalf("error = %v, want valid = %v", err, tc.valid)
			}
		})
	}
}

func TestProgramObservationValidation(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	for name, tc := range map[string]struct {
		observation Observation
		valid       bool
	}{
		"an agent without a gateway":     {Observation{Agent: AgentProgram{Digest: digest}}, true},
		"a gateway and an agent":         {Observation{Gateway: &GatewayProgram{DocumentLevel: 1, Digest: digest}, Agent: AgentProgram{Digest: digest}}, true},
		"no agent":                       {Observation{}, false},
		"a gateway without a level":      {Observation{Gateway: &GatewayProgram{Digest: digest}, Agent: AgentProgram{Digest: digest}}, false},
		"a gateway without a digest":     {Observation{Gateway: &GatewayProgram{DocumentLevel: 1}, Agent: AgentProgram{Digest: digest}}, false},
		"a gateway digest that is a tag": {Observation{Gateway: &GatewayProgram{DocumentLevel: 1, Digest: "v0.4.0"}, Agent: AgentProgram{Digest: digest}}, false},
		"a digest in upper case":         {Observation{Agent: AgentProgram{Digest: strings.ToUpper(digest)}}, false},
		"a digest with a line after it":  {Observation{Agent: AgentProgram{Digest: digest + "\n"}}, false},
	} {
		if err := validateObservation(tc.observation, time.Now()); (err == nil) != tc.valid {
			t.Errorf("%s: error = %v, want valid = %v", name, err, tc.valid)
		}
	}
	api := &API{Registry: testRegistry{readers: map[string]IdentityReader{"c": &fakeReader{evidence: validEvidence()}}}, Observations: &testObservationSink{}}
	if w := apiRequest(api, http.MethodPost, "/v1/observations", `{"cluster_id":"c","observation":{"gateway":{"documentLevel":1,"digest":"`+digest+`","image":"x"}}}`, "token"); w.Code != http.StatusBadRequest {
		t.Fatalf("a field the gateway does not state was accepted: %d", w.Code)
	}
}

type fakeReader struct {
	evidence Evidence
	err      error
}

func (f *fakeReader) Authenticate(context.Context, string) (Evidence, error) {
	return f.evidence, f.err
}

func validEvidence() Evidence {
	return Evidence{ClusterID: "c", NodeName: "node", NodeUID: "n"}
}
