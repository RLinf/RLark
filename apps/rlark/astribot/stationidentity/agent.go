package stationidentity

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

type AgentConfig struct {
	APIURL                   string
	ClusterID                string
	TokenFile                string
	CAFile                   string
	Interval                 time.Duration
	RequestLimit             time.Duration
	ProbeAddress             string
	WireguardObservationFile string
	// NetworkConfigFile enables network enrollment: it is where the agent
	// stores the configuration computed for the node. NetworkIdentityFile is
	// where the gateway container publishes its public key; without it the
	// node enrolls its address only, for a network that runs no tunnel.
	NetworkIdentityFile string
	NetworkConfigFile   string
}

// pendingEnrollmentRetry is how soon a node asks again while its enrollment is
// being answered. It stays above the observation rate limit of one per second.
const pendingEnrollmentRetry = 5 * time.Second

type Agent struct {
	config               AgentConfig
	client               *http.Client
	read                 func(string) ([]byte, error)
	post                 func(ctx context.Context, path string, body []byte) (int, []byte, error)
	now                  func() time.Time
	mu                   sync.RWMutex
	lastSuccessfulReport time.Time
	observe              func() (Observation, error)
	dial                 func(context.Context, string) error
	program              AgentProgram
}

func NewAgent(config AgentConfig) (*Agent, error) {
	agent, err := newAgent(config, os.ReadFile, time.Now)
	if err != nil {
		return nil, err
	}
	// The file this process was started from: read once, it cannot change.
	self, err := os.Open("/proc/self/exe")
	if err != nil {
		return nil, err
	}
	defer self.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, self); err != nil {
		return nil, err
	}
	agent.program = AgentProgram{Digest: hex.EncodeToString(sum.Sum(nil))}
	return agent, nil
}

func newAgent(config AgentConfig, read func(string) ([]byte, error), now func() time.Time) (*Agent, error) {
	if config.ProbeAddress == "" {
		config.ProbeAddress = ":8081"
	}
	// The Pod's token goes to Station API over verified TLS only.
	if parsed, err := url.Parse(config.APIURL); err != nil || parsed.Scheme != "https" || parsed.Host == "" || config.ClusterID == "" || config.TokenFile == "" {
		return nil, errors.New("an HTTPS Station API URL, the cluster ID and the projected token path are required")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	if config.CAFile != "" {
		pem, err := read(config.CAFile)
		if err != nil {
			return nil, errors.New("read Station API CA bundle")
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("Station API CA bundle contains no certificate")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   config.RequestLimit,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("Station API redirects are not allowed")
		},
	}
	agent := &Agent{config: config, client: client, read: read, now: now, observe: collectSystemObservation}
	agent.post = agent.send
	agent.dial = func(ctx context.Context, address string) error {
		connection, err := (&net.Dialer{}).DialContext(ctx, "tcp4", address)
		if err == nil {
			_ = connection.Close()
		}
		return err
	}
	return agent, nil
}

func (a *Agent) Run(ctx context.Context) error {
	probeServer := &http.Server{
		Addr: a.config.ProbeAddress, Handler: a.probeHandler(),
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second,
		WriteTimeout: 3 * time.Second, IdleTimeout: 10 * time.Second,
	}
	listener, err := net.Listen("tcp", a.config.ProbeAddress)
	if err != nil {
		return fmt.Errorf("listen for Station agent health probes: %w", err)
	}
	go func() { _ = probeServer.Serve(listener) }()
	defer probeServer.Close()
	backoff, enrollment, reported := time.Second, "", ""
	maxBackoff := min(a.config.Interval, 30*time.Second)
	for {
		// A refused report is tried again sooner, and does not keep the node
		// from asking for its network: its tunnel document must not hang on
		// what station-api makes of an observation.
		wait := a.config.Interval
		if err := a.Report(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if err.Error() != reported {
				reported = err.Error()
				fmt.Fprintln(os.Stderr, "observation:", reported)
			}
			wait = backoff
			backoff = min(backoff*2, maxBackoff)
		} else {
			backoff = time.Second
			if reported != "" {
				reported = ""
				fmt.Fprintln(os.Stderr, "observation: accepted")
			}
		}
		if a.config.NetworkConfigFile != "" {
			state, err := a.Enroll(ctx)
			if err != nil {
				state = err.Error()
			}
			if state != enrollment {
				enrollment = state
				fmt.Fprintln(os.Stderr, "network enrollment:", state)
			}
			// The controller answers a new request within seconds; do not
			// make a joining node wait a whole interval for it.
			if state == "pending" {
				wait = min(wait, pendingEnrollmentRetry)
			}
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (a *Agent) Report(ctx context.Context) error {
	observation, err := a.observe()
	if err != nil {
		return fmt.Errorf("collect Station observation: %w", err)
	}
	observation.Agent = a.program
	if a.config.WireguardObservationFile != "" {
		observation.Wireguard, observation.Gateway = readWireguardSnapshot(a.config.WireguardObservationFile, a.now())
	}
	if a.config.NetworkConfigFile != "" {
		observation.Probes = a.probePeers(ctx)
	}
	body, err := json.Marshal(observationRequest{ClusterID: a.config.ClusterID, Observation: observation})
	if err != nil {
		return err
	}
	status, _, err := a.post(ctx, "/v1/observations", body)
	if err != nil {
		return err
	}
	if status != http.StatusAccepted {
		return fmt.Errorf("Station API rejected observation with HTTP %d", status)
	}
	a.mu.Lock()
	a.lastSuccessfulReport = a.now().UTC()
	a.mu.Unlock()
	return nil
}

// A missing, malformed or stale snapshot (one without the gateway's statement
// of itself among them) clears the previous observation while leaving the
// identity heartbeat independent of the network container. What the gateway
// states of itself goes with the snapshot: a gateway that is not reconciling
// is not known to understand anything.
func readWireguardSnapshot(path string, now time.Time) (json.RawMessage, *GatewayProgram) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, wireguardObservationBytes+1))
	if err != nil || len(data) > wireguardObservationBytes {
		return nil, nil
	}
	var snapshot struct {
		ObservedAt time.Time       `json:"observedAt"`
		Gateway    *GatewayProgram `json:"gateway"`
		wireguardObservation
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if dec.Decode(&snapshot) != nil {
		return nil, nil
	}
	var extra any
	if dec.Decode(&extra) != io.EOF || snapshot.Gateway == nil || snapshot.ObservedAt.IsZero() || snapshot.ObservedAt.After(now.Add(5*time.Second)) || now.Sub(snapshot.ObservedAt) > 30*time.Second {
		return nil, nil
	}
	raw, err := json.Marshal(snapshot.wireguardObservation)
	if err != nil || validateWireguardObservation(raw, now) != nil {
		return nil, nil
	}
	return raw, snapshot.Gateway
}

func collectSystemObservation() (Observation, error) {
	data, err := os.Open("/proc/stat")
	if err != nil {
		return Observation{}, err
	}
	defer func() { _ = data.Close() }()
	bootTime, err := parseBootTime(data)
	if err != nil {
		return Observation{}, err
	}
	return Observation{BootTime: &bootTime}, nil
}

func parseBootTime(reader io.Reader) (time.Time, error) {
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || fields[0] != "btime" {
			continue
		}
		if len(fields) != 2 {
			return time.Time{}, errors.New("invalid /proc/stat btime field")
		}
		seconds, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || seconds <= 0 {
			return time.Time{}, errors.New("invalid /proc/stat boot time")
		}
		return time.Unix(seconds, 0).UTC(), nil
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, err
	}
	return time.Time{}, errors.New("/proc/stat has no btime field")
}

func (a *Agent) probeHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.RLock()
		last := a.lastSuccessfulReport
		a.mu.RUnlock()
		if last.IsZero() || a.now().Sub(last) > 2*a.config.Interval+a.config.RequestLimit {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

// send posts body to Station API as the node's Pod, with its projected token as it is now (it rotates), and
// answers the status and what came back.
func (a *Agent) send(ctx context.Context, path string, body []byte) (int, []byte, error) {
	token, err := a.read(a.config.TokenFile)
	if err != nil {
		return 0, nil, errors.New("read projected Station token")
	}
	requestCtx, cancel := context.WithTimeout(ctx, a.config.RequestLimit)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, strings.TrimRight(a.config.APIURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
	response, err := a.client.Do(request)
	if err != nil {
		return 0, nil, fmt.Errorf("send to Station API: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	return response.StatusCode, data, err
}
