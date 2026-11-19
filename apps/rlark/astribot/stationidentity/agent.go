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

func (c AgentConfig) Validate() error {
	parsed, err := url.Parse(c.APIURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
		return errors.New("Station API URL must be an HTTPS origin or path")
	}
	if c.ClusterID == "" || len(c.ClusterID) > 128 || c.TokenFile == "" {
		return errors.New("cluster ID and projected token path are required")
	}
	if c.Interval < 5*time.Second || c.Interval > 10*time.Minute || c.RequestLimit < time.Second || c.RequestLimit > time.Minute {
		return errors.New("agent interval or request timeout is outside the supported range")
	}
	if c.ProbeAddress != "" {
		_, port, err := net.SplitHostPort(c.ProbeAddress)
		n, portErr := strconv.Atoi(port)
		if err != nil || portErr != nil || n < 1 || n > 65535 {
			return errors.New("agent probe address must contain a valid TCP port")
		}
	}
	if c.NetworkIdentityFile != "" && c.NetworkConfigFile == "" {
		return errors.New("a network identity file requires a network configuration file")
	}
	return nil
}

// pendingEnrollmentRetry is how soon a node asks again while its enrollment is
// being answered. It stays above the observation rate limit of one per second.
const pendingEnrollmentRetry = 5 * time.Second

type Agent struct {
	config               AgentConfig
	client               *http.Client
	read                 func(string) ([]byte, error)
	post                 func(context.Context, string, []byte, string) error
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
	if err := config.Validate(); err != nil {
		return nil, err
	}
	transport := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: config.RequestLimit, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:   config.RequestLimit,
		ResponseHeaderTimeout: config.RequestLimit,
		IdleConnTimeout:       60 * time.Second,
		MaxIdleConns:          16,
	}
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
	agent.post = agent.postObservation
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
	probeErr := make(chan error, 1)
	go func() { probeErr <- probeServer.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = probeServer.Shutdown(shutdownCtx)
	}()
	backoff, enrollment, reported := time.Second, "", ""
	maxBackoff := min(a.config.Interval, 30*time.Second)
	for {
		select {
		case err := <-probeErr:
			if !errors.Is(err, http.ErrServerClosed) {
				return errors.New("Station agent probe listener stopped")
			}
		default:
		}
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
	tokenBytes, err := a.read(a.config.TokenFile)
	if err != nil {
		return errors.New("read projected Station token")
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" || strings.ContainsAny(token, "\r\n \t") {
		return errors.New("projected Station token is invalid")
	}
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
		return errors.New("encode Station observation")
	}
	if err := a.post(ctx, strings.TrimRight(a.config.APIURL, "/")+"/v1/observations", body, token); err != nil {
		return err
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

func (a *Agent) postObservation(ctx context.Context, endpoint string, body []byte, token string) error {
	requestCtx, cancel := context.WithTimeout(ctx, a.config.RequestLimit)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("build Station API request")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("send Station observation: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Station API rejected observation with HTTP %d", response.StatusCode)
	}
	return nil
}
