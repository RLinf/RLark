// Package stationidentity is Station API (cmd/station-api) and the station agent that reports to it
// (cmd/station-agent).
package stationidentity

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var (
	ErrClusterNotRegistered = errors.New("cluster is not registered")
	ErrIdentityUnavailable  = errors.New("identity verification unavailable")
	ErrIdentityDenied       = errors.New("identity denied")
)

// IdentityReader resolves fresh, server-side evidence from a registered cluster.
type IdentityReader interface {
	Authenticate(context.Context, string) (Evidence, error)
}

// ClusterRegistry resolves a caller-supplied routing hint only to a trusted,
// server-owned registration. Implementations must not fetch arbitrary URLs.
type ClusterRegistry interface {
	Reader(context.Context, string) (IdentityReader, error)
}

type Observation struct {
	BootTime  *time.Time      `json:"bootTime,omitempty"`
	Wireguard json.RawMessage `json:"wireguard,omitempty"`
	Probes    []Probe         `json:"probes,omitempty"`
	// Gateway is what the node's station-gateway states of itself in its
	// observation file (none while it is not reconciling), Agent the
	// station-agent that sends the report.
	Gateway *GatewayProgram `json:"gateway,omitempty"`
	Agent   AgentProgram    `json:"agent"`
}

// GatewayProgram is a station-gateway by the sha256 of its file and the
// newest configuration document it understands.
type GatewayProgram struct {
	DocumentLevel int32  `json:"documentLevel"`
	Digest        string `json:"digest"`
}

type AgentProgram struct {
	Digest string `json:"digest"`
}

var programDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Probe struct {
	Target    string `json:"target"`
	Via       string `json:"via,omitempty"`
	OK        bool   `json:"ok"`
	RTTMillis *int64 `json:"rttMs,omitempty"`
}

type ObservationSink interface {
	Store(context.Context, Evidence, Observation) error
}

type API struct {
	Registry           ClusterRegistry
	Observations       ObservationSink
	ObservationLimiter *NodeLimiter
	Ready              func(context.Context) error
	Now                func() time.Time
	NetworkSummary     *NodeNetworkSummaryReader
	Enrollments        NetworkEnroller
	EnrollmentLimiter  *NodeLimiter
	// Bootstrap, when set, admits machines that are not nodes yet by a one-time code.
	Bootstrap *Bootstrap
	// Documents, when set, gives a node its document for its WireGuard key.
	Documents         *NetworkDocuments
	GatewayCredential func() ([]byte, error)
}

type observationRequest struct {
	ClusterID   string      `json:"cluster_id"`
	Observation Observation `json:"observation"`
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	switch route := r.Method + " " + r.URL.Path; {
	case route == "GET /healthz":
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	case route == "GET /readyz":
		if a.Ready == nil || a.Ready(r.Context()) != nil {
			writeError(w, http.StatusServiceUnavailable, "station API dependencies are unavailable")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	case a.Bootstrap != nil && (strings.HasPrefix(r.URL.Path, "/b/") || strings.HasPrefix(r.URL.Path, "/v1/bootstrap/")):
		a.Bootstrap.ServeHTTP(w, r)
	// For the node's WireGuard key, with no token: its own document, how far a new node's key has reached its
	// peers, and what a hub found on its site's network.
	case route == "POST /v1/network/document":
		a.Documents.ServeHTTP(w, r)
	case route == "POST /v1/network/propagation":
		a.Documents.ServePropagation(w, r)
	case route == "POST /v1/network/discovery-scan":
		a.Documents.ServeDiscoveryScan(w, r)
	case route == "POST /v1/network/discovered":
		a.Documents.ServeDiscovered(w, r)
	case r.Method != http.MethodPost:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	default:
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch {
		case token == "":
			writeError(w, http.StatusUnauthorized, "bearer token required")
		case r.URL.Path == "/v1/internal/node-network":
			a.nodeNetwork(w, r, token)
		case r.URL.Path == "/v1/observations":
			a.observe(w, r, token)
		case r.URL.Path == "/v1/network/enrollments":
			a.enroll(w, r, token)
		default:
			writeError(w, http.StatusNotFound, "not found")
		}
	}
}

func (a *API) nodeNetwork(w http.ResponseWriter, r *http.Request, token string) {
	if a.NetworkSummary == nil || a.GatewayCredential == nil {
		writeError(w, http.StatusServiceUnavailable, "network summary is not configured")
		return
	}
	credential, err := a.GatewayCredential()
	if err != nil || len(credential) < 32 || len(credential) > 4096 {
		writeError(w, http.StatusServiceUnavailable, "network summary credential is unavailable")
		return
	}
	expected := sha256.Sum256(credential)
	provided := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
		writeError(w, http.StatusUnauthorized, "invalid service credential")
		return
	}
	var request struct {
		ClusterID string `json:"cluster_id"`
		NodeName  string `json:"node_name"`
	}
	if !decodeBody(w, r, &request) {
		return
	}
	if request.ClusterID == "" || len(request.ClusterID) > 128 || request.NodeName == "" || len(request.NodeName) > 253 {
		writeError(w, http.StatusBadRequest, "cluster_id and node_name are required")
		return
	}
	summary, err := a.NetworkSummary.Read(r.Context(), request.ClusterID, request.NodeName)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "network summary is unavailable")
		return
	}
	writeJSON(w, http.StatusOK, summary)
}

func (a *API) authenticate(ctx context.Context, clusterID, token string) (Evidence, error) {
	if clusterID == "" {
		return Evidence{}, ErrIdentityDenied
	}
	reader, err := a.Registry.Reader(ctx, clusterID)
	if err != nil {
		if errors.Is(err, ErrClusterNotRegistered) {
			return Evidence{}, ErrIdentityDenied
		}
		return Evidence{}, ErrIdentityUnavailable
	}
	e, err := reader.Authenticate(ctx, token)
	if err != nil {
		if errors.Is(err, ErrIdentityUnavailable) {
			return Evidence{}, ErrIdentityUnavailable
		}
		return Evidence{}, ErrIdentityDenied
	}
	// The reader validated the evidence (Authenticate).
	if e.ClusterID != clusterID {
		return Evidence{}, ErrIdentityDenied
	}
	return e, nil
}

func (a *API) observe(w http.ResponseWriter, r *http.Request, token string) {
	var req observationRequest
	if !decodeBody(w, r, &req) {
		return
	}
	e, err := a.authenticate(r.Context(), req.ClusterID, token)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if err := validateObservation(req.Observation, clock(a.Now)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if a.ObservationLimiter != nil && !a.ObservationLimiter.Allow(e, clock(a.Now)) {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "observation rate limit exceeded")
		return
	}
	if err := a.Observations.Store(r.Context(), e, req.Observation); err != nil {
		writeError(w, http.StatusServiceUnavailable, "observation storage unavailable")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "node_uid": e.NodeUID, "reported_at": clock(a.Now).UTC()})
}

// clock is the time by now, the clock a test set, or else the system's.
func clock(now func() time.Time) time.Time {
	if now != nil {
		return now()
	}
	return time.Now()
}

func validateObservation(o Observation, now time.Time) error {
	if len(o.Probes) > 256 || len(o.Wireguard) > wireguardObservationBytes {
		return errors.New("observation exceeds limits")
	}
	for _, probe := range o.Probes {
		if probe.Target == "" || len(probe.Target) > 256 || len(probe.Via) > 256 || (probe.RTTMillis != nil && (*probe.RTTMillis < 0 || *probe.RTTMillis > 600000)) {
			return errors.New("invalid probe")
		}
	}
	if len(o.Wireguard) > 0 {
		if err := validateWireguardObservation(o.Wireguard, now); err != nil {
			return err
		}
	}
	if o.Gateway != nil && (o.Gateway.DocumentLevel < 1 || !programDigestPattern.MatchString(o.Gateway.Digest)) {
		return errors.New("invalid gateway program")
	}
	if !programDigestPattern.MatchString(o.Agent.Digest) {
		return errors.New("invalid agent program")
	}
	return nil
}

// A hub of a hundred leaves reports a hundred peers, about 140 bytes each.
const (
	wireguardObservationBytes = 256 * 1024
	wireguardObservationPeers = 1024
)

// wireguardObservation is what a node reports of its tunnel. It tells its
// peers by public key and names none: the name a peer is stored under is the
// member the server finds enrolled with that key.
type wireguardObservation struct {
	Interface string   `json:"interface"`
	PublicKey string   `json:"publicKey,omitempty"`
	Addresses []string `json:"addresses,omitempty"`
	Peers     []struct {
		PublicKey       string     `json:"publicKey"`
		LatestHandshake *time.Time `json:"latestHandshake,omitempty"`
		ReceivedBytes   int64      `json:"receivedBytes,omitempty"`
		SentBytes       int64      `json:"sentBytes,omitempty"`
	} `json:"peers,omitempty"`
}

// A handshake carries the node's clock, which may run ahead of the server's;
// one further ahead than a minute is not a time the node observed.
func validateWireguardObservation(raw json.RawMessage, now time.Time) error {
	var observation wireguardObservation
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&observation); err != nil {
		return errors.New("invalid WireGuard observation")
	}
	if observation.Interface == "" || len(observation.Interface) > 15 || len(observation.Addresses) > 16 || len(observation.Peers) > wireguardObservationPeers {
		return errors.New("invalid WireGuard observation")
	}
	if observation.PublicKey != "" {
		decoded, err := base64.StdEncoding.DecodeString(observation.PublicKey)
		if err != nil || len(decoded) != 32 || base64.StdEncoding.EncodeToString(decoded) != observation.PublicKey {
			return errors.New("invalid WireGuard public key")
		}
	}
	addresses := make(map[string]struct{}, len(observation.Addresses))
	for _, address := range observation.Addresses {
		if _, _, err := net.ParseCIDR(address); err != nil || len(address) > 64 {
			return errors.New("invalid WireGuard address")
		}
		if _, exists := addresses[address]; exists {
			return errors.New("duplicate WireGuard address")
		}
		addresses[address] = struct{}{}
	}
	peers := make(map[string]struct{}, len(observation.Peers))
	for _, peer := range observation.Peers {
		key, err := base64.StdEncoding.DecodeString(peer.PublicKey)
		if _, exists := peers[peer.PublicKey]; exists || err != nil || len(key) != 32 || peer.ReceivedBytes < 0 || peer.SentBytes < 0 ||
			peer.LatestHandshake != nil && peer.LatestHandshake.After(now.Add(time.Minute)) {
			return errors.New("invalid WireGuard peer observation")
		}
		peers[peer.PublicKey] = struct{}{}
	}
	return nil
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "application/json required")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
func writeAuthError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrIdentityUnavailable) {
		writeError(w, http.StatusServiceUnavailable, "identity verification unavailable")
		return
	}
	writeError(w, http.StatusUnauthorized, "identity rejected")
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
