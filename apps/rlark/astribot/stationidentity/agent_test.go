package stationidentity

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAgentReportsProbePortConflictBeforeReporting(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	config := AgentConfig{APIURL: "https://station-api", ClusterID: "cluster", TokenFile: "token", Interval: 30 * time.Second, RequestLimit: time.Second, ProbeAddress: listener.Addr().String()}
	agent, err := newAgent(config, func(string) ([]byte, error) { return []byte("token"), nil }, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.Run(context.Background()); err == nil || !strings.Contains(err.Error(), "listen for Station agent health probes") {
		t.Fatalf("expected probe listener conflict, got %v", err)
	}
}

func TestAgentReportUsesRotatingPodTokenAndVerifiedTLS(t *testing.T) {
	var token = "first-token"
	var calls int
	bootTime := time.Date(2026, time.November, 11, 0, 0, 0, 0, time.UTC)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/observations" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Errorf("Authorization = %q", got)
		}
		var request observationRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode observation: %v", err)
		}
		if request.ClusterID != "cluster-a" || len(request.Observation.Roles) != 0 || request.Observation.BootTime == nil || !request.Observation.BootTime.Equal(bootTime) {
			t.Errorf("request = %#v", request)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	// The TLS test server certificate is self-signed; encode it as the trusted CA.
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	config := AgentConfig{APIURL: server.URL, ClusterID: "cluster-a", TokenFile: "token", CAFile: "ca", Interval: 30 * time.Second, RequestLimit: 2 * time.Second}
	agent, err := newAgent(config, func(path string) ([]byte, error) {
		switch path {
		case "token":
			return []byte(token + "\n"), nil
		case "ca":
			return certPEM, nil
		default:
			return nil, errors.New("unexpected file")
		}
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	agent.observe = func() (Observation, error) {
		return Observation{BootTime: &bootTime}, nil
	}
	if err := agent.Report(context.Background()); err != nil {
		t.Fatal(err)
	}
	token = "rotated-token"
	if err := agent.Report(context.Background()); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("request count=%d", calls)
	}
}

func TestParseBootTime(t *testing.T) {
	got, err := parseBootTime(strings.NewReader("cpu  1 2 3 4\nbtime 1794355200\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Unix(1794355200, 0).UTC(); !got.Equal(want) {
		t.Fatalf("boot time=%s, want %s", got, want)
	}
	for _, input := range []string{"cpu 1 2\n", "btime nope\n", "btime 0\n", "btime 1 trailing\n"} {
		if _, err := parseBootTime(strings.NewReader(input)); err == nil {
			t.Errorf("accepted invalid proc stat %q", input)
		}
	}
}

func TestAgentDoesNotReportWhenObservationCollectionFails(t *testing.T) {
	agent, err := newAgent(AgentConfig{APIURL: "https://station-api", ClusterID: "cluster", TokenFile: "token", Interval: 30 * time.Second, RequestLimit: 5 * time.Second}, func(string) ([]byte, error) { return []byte("token"), nil }, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	agent.observe = func() (Observation, error) { return Observation{}, errors.New("proc unavailable") }
	agent.post = func(context.Context, string, []byte, string) error {
		called = true
		return nil
	}
	if err := agent.Report(context.Background()); err == nil {
		t.Fatal("observation collection failure was ignored")
	}
	if called {
		t.Fatal("reported an incomplete observation")
	}
}

func TestAgentRejectsInvalidConfigAndResponse(t *testing.T) {
	base := AgentConfig{APIURL: "http://station-api", ClusterID: "cluster", TokenFile: "token", Interval: 30 * time.Second, RequestLimit: 5 * time.Second}
	if _, err := newAgent(base, func(string) ([]byte, error) { return []byte("token"), nil }, time.Now); err == nil {
		t.Fatal("accepted non-HTTPS endpoint")
	}
	base.APIURL = "https://station-api"
	base.ProbeAddress = "invalid-port"
	if _, err := newAgent(base, func(string) ([]byte, error) { return []byte("token"), nil }, time.Now); err == nil {
		t.Fatal("accepted invalid probe address")
	}
	base.ProbeAddress = ":18082"
	agent, err := newAgent(base, func(string) ([]byte, error) { return []byte("token"), nil }, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	agent.post = func(context.Context, string, []byte, string) error { return errors.New("down") }
	if err := agent.Report(context.Background()); err == nil {
		t.Fatal("accepted failed API report")
	}
	if _, err := newAgent(AgentConfig{APIURL: "https://station-api", ClusterID: "cluster", TokenFile: "token", Interval: time.Second, RequestLimit: time.Second}, func(string) ([]byte, error) { return nil, nil }, time.Now); err == nil {
		t.Fatal("accepted too-short observation interval")
	}
}

func TestAgentDoesNotFollowRedirectsWithProjectedToken(t *testing.T) {
	var redirected int
	redirectTarget := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirected++
	}))
	defer redirectTarget.Close()
	redirector := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, httptest.NewRequest(http.MethodPost, redirectTarget.URL, nil), redirectTarget.URL, http.StatusTemporaryRedirect)
	}))
	defer redirector.Close()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: redirector.Certificate().Raw})
	agent, err := newAgent(AgentConfig{APIURL: redirector.URL, ClusterID: "cluster", TokenFile: "token", CAFile: "ca", Interval: 30 * time.Second, RequestLimit: 2 * time.Second}, func(path string) ([]byte, error) {
		if path == "token" {
			return []byte("pod-token"), nil
		}
		return ca, nil
	}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if err := agent.Report(context.Background()); err == nil {
		t.Fatal("accepted Station API redirect")
	}
	if redirected != 0 {
		t.Fatalf("redirect target received %d requests", redirected)
	}
}

func TestAgentProbeReadinessTracksRecentSuccessfulReport(t *testing.T) {
	now := time.Unix(100, 0)
	agent, err := newAgent(AgentConfig{APIURL: "https://station-api", ClusterID: "cluster", TokenFile: "token", Interval: 30 * time.Second, RequestLimit: 5 * time.Second}, func(string) ([]byte, error) { return []byte("token"), nil }, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	response := httptest.NewRecorder()
	agent.probeHandler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unreported readiness=%d", response.Code)
	}
	agent.mu.Lock()
	agent.lastSuccessfulReport = now
	agent.mu.Unlock()
	response = httptest.NewRecorder()
	agent.probeHandler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("fresh readiness=%d", response.Code)
	}
	now = now.Add(2*agent.config.Interval + agent.config.RequestLimit + time.Second)
	response = httptest.NewRecorder()
	agent.probeHandler().ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("stale readiness=%d", response.Code)
	}
}
