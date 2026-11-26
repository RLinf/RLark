package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	nodeAdmissionGVR = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "nodeadmissions"}
	nodeClassGVR     = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "nodeclasses"}
	siteNetworkGVR   = schema.GroupVersionResource{Group: "station.rlark.io", Version: "v1alpha1", Resource: "sitenetworks"}
)

const (
	maxNodeAdmissionRequestBytes = 16 * 1024
	defaultJoinCodeTTL           = 2 * time.Hour
	minJoinCodeTTL               = 10 * time.Minute
	maxJoinCodeTTL               = 7 * 24 * time.Hour
	joinCodeLength               = 12
)

// stationBootstrap is Station API as the Gateway sees it when it gives out a
// join code: where a new machine reaches it, and the digests of what it
// serves, read over the Gateway's own verified connection. The machine's
// command pins those digests before it trusts anything it fetches.
type stationBootstrap interface {
	// Address is host:port of Station API as a machine that joins reaches it.
	Address() string
	// TunnelAddress is host:port of Station API on the site network, as the
	// Gateway itself reaches it: where a machine whose tunnel is up before it
	// joins (a node the SiteNetwork declares with its key) reaches it.
	TunnelAddress() string
	// Digest is the SHA-256, in hex, of what Station API serves at path.
	Digest(ctx context.Context, path string) (string, error)
	// Open streams what Station API serves at path (the installer, which takes minutes to send), and its length.
	Open(ctx context.Context, path string) (io.ReadCloser, int64, error)
	// BundleDigest is the SHA-256, in hex, of the installer, as Station API says it (/v1/bootstrap/bundle.sha256).
	BundleDigest(ctx context.Context) (string, error)
}

type stationBootstrapClient struct {
	address, tunnel, origin string
	http                    *http.Client
}

func newStationBootstrapClient(bootstrapURL, origin, caFile string) (*stationBootstrapClient, error) {
	machines, err := url.Parse(bootstrapURL)
	if err != nil || machines.Scheme != "https" || machines.Port() == "" || machines.User != nil || (machines.Path != "" && machines.Path != "/") || machines.RawQuery != "" {
		return nil, errors.New("the Station API bootstrap URL must be https://host:port")
	}
	address := machines.Host
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("the Station API HTTPS origin is required to give out join codes")
	}
	client, err := stationHTTPClient(caFile)
	if err != nil {
		return nil, err
	}
	return &stationBootstrapClient{address: address, tunnel: u.Host, origin: strings.TrimRight(origin, "/"), http: client}, nil
}

// stationHTTPClient reaches Station API verified by the CA of caFile, and follows no redirect.
func stationHTTPClient(caFile string) (*http.Client, error) {
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return nil, errors.New("read Station API CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return nil, errors.New("invalid Station API CA")
	}
	return &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect forbidden") }}, nil
}

func (s *stationBootstrapClient) Address() string { return s.address }

func (s *stationBootstrapClient) TunnelAddress() string { return s.tunnel }

// get GETs path from Station API within timeout (0: until ctx ends), and answers its body when Station API answers 200.
func (s *stationBootstrapClient) get(ctx context.Context, path string, timeout time.Duration) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.origin+path, nil)
	if err != nil {
		return nil, err
	}
	client := *s.http
	client.Timeout = timeout
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		// The path may hold a code: it stays out of the error.
		return nil, fmt.Errorf("Station API answered %d", resp.StatusCode)
	}
	return resp, nil
}

func (s *stationBootstrapClient) Digest(ctx context.Context, path string) (string, error) {
	resp, err := s.get(ctx, path, s.http.Timeout)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, io.LimitReader(resp.Body, 1<<20)); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

// Open has no time limit of the client's: the installer is hundreds of MB; ctx ends the stream.
func (s *stationBootstrapClient) Open(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	resp, err := s.get(ctx, path, 0)
	if err != nil {
		return nil, 0, err
	}
	return resp.Body, resp.ContentLength, nil
}

// BundleDigest waits up to 2 minutes: Station API reads the whole installer (about 2 GB) for its first answer after
// the installer changes.
func (s *stationBootstrapClient) BundleDigest(ctx context.Context) (string, error) {
	resp, err := s.get(ctx, "/v1/bootstrap/bundle.sha256", 2*time.Minute)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

type issueNodeAdmissionRequest struct {
	ClusterID string `json:"cluster_id"`
	NodeName  string `json:"node_name"`
	// Site is the site the machine stands in when it is not its cluster's.
	Site string `json:"site,omitempty"`
	// Address pins the tunnel address; empty lets station-controller allocate one.
	Address string `json:"address,omitempty"`
	// Uplink is the gateway of a machine whose default route does not lead
	// out, where its site declares no leafUplink.
	Uplink string `json:"uplink,omitempty"`
	// NodeClass names the NodeClass of the machine; the node follows it.
	NodeClass  string                 `json:"node_class,omitempty"`
	Taints     []string               `json:"taints,omitempty"`
	Labels     map[string]string      `json:"labels,omitempty"`
	Machine    map[string]interface{} `json:"machine,omitempty"`
	TTLSeconds int64                  `json:"ttl_seconds,omitempty"`
}

type reissueNodeAdmissionRequest struct {
	TTLSeconds int64 `json:"ttl_seconds,omitempty"`
}

// nodeAdmission is one machine's admission as an operator reads it. The code
// is never in it: the Gateway answers it once, when it gives it out.
type nodeAdmission struct {
	ClusterID string `json:"cluster_id"`
	NodeName  string `json:"node_name"`
	// State is Pending (the code is open), Redeemed (a machine took it and is
	// joining), Joined, or Expired (also a code withdrawn before it was used).
	State      string            `json:"state"`
	Reason     string            `json:"reason,omitempty"`
	Site       string            `json:"site,omitempty"`
	Address    string            `json:"address,omitempty"`
	Uplink     string            `json:"uplink,omitempty"`
	NodeClass  string            `json:"node_class,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Taints     []string          `json:"taints,omitempty"`
	ExpiresAt  string            `json:"expires_at"`
	IssuedBy   string            `json:"issued_by,omitempty"`
	IssuedAt   string            `json:"issued_at,omitempty"`
	RedeemedAt string            `json:"redeemed_at,omitempty"`
	// PublicKey is the WireGuard public key of the machine that redeemed the code.
	PublicKey string `json:"public_key,omitempty"`
	// TunnelAddress is the address station-controller gave the machine.
	TunnelAddress string `json:"tunnel_address,omitempty"`
	// SSHLogin and SSHHostKey are there for a machine joined from the console
	// by SSH: user@host:port, and the host key it showed when it was logged in to.
	SSHLogin   string `json:"ssh_login,omitempty"`
	SSHHostKey string `json:"ssh_host_key,omitempty"`
	// SelfHealed is what RLark found of the machine's earlier attempt or
	// earlier node and put right by itself.
	SelfHealed string `json:"self_healed,omitempty"`
	// JoinTokenAt is when Station API gave the machine its token to join its cluster.
	JoinTokenAt string `json:"join_token_at,omitempty"`
	// Stuck says a machine redeemed the code longer ago than a join takes and is no node yet: Reason says where it stopped.
	Stuck bool `json:"stuck,omitempty"`
	// JoinPhase and JoinStage are where the SSH join of the node stands (its NodeJoin), when it was joined so.
	JoinPhase string `json:"join_phase,omitempty"`
	JoinStage string `json:"join_stage,omitempty"`
}

// joinStuckAfter is how long a redeemed code may take to become a node before its join is shown as stuck: an
// installer takes a few minutes, one that pulls its images through a slow uplink about ten.
const joinStuckAfter = 20 * time.Minute

// issuedNodeAdmission is the one answer that carries the code, and the line
// the operator runs as root on the machine.
type issuedNodeAdmission struct {
	Admission nodeAdmission `json:"admission"`
	Code      string        `json:"code"`
	Command   string        `json:"command"`
}

func nodeAdmissionView(object *unstructured.Unstructured, now time.Time) nodeAdmission {
	text := func(fields ...string) string {
		value, _, _ := unstructured.NestedString(object.Object, fields...)
		return value
	}
	view := nodeAdmission{
		ClusterID: text("spec", "clusterID"), NodeName: object.GetName(), Site: text("spec", "site"),
		Address: text("spec", "address"), Uplink: text("spec", "uplink", "gateway"), NodeClass: text("spec", "nodeClass"), ExpiresAt: text("spec", "expiresAt"),
		IssuedBy: text("spec", "issuedBy"), IssuedAt: text("spec", "issuedAt"), RedeemedAt: text("spec", "redeemedAt"), PublicKey: text("spec", "publicKey"),
		TunnelAddress: text("status", "address"), Reason: text("status", "reason"),
	}
	view.SSHLogin, view.SSHHostKey = object.GetAnnotations()[nodeAdmissionSSHLoginAnnotation], object.GetAnnotations()[nodeAdmissionSSHHostKeyAnnotation]
	view.SelfHealed, view.JoinTokenAt = object.GetAnnotations()[nodeAdmissionSelfHealedAnnotation], object.GetAnnotations()[nodeAdmissionJoinTokenAnnotation]
	view.Labels, _, _ = unstructured.NestedStringMap(object.Object, "spec", "labels")
	view.Taints, _, _ = unstructured.NestedStringSlice(object.Object, "spec", "taints")
	// The state of the code is read from spec, which the Gateway and Station
	// API write; station-controller's answer only says whether a redeemed
	// code has led to a node yet.
	expires, _ := time.Parse(time.RFC3339, view.ExpiresAt)
	observed, _, _ := unstructured.NestedInt64(object.Object, "status", "observedGeneration")
	switch {
	case view.PublicKey == "" && !now.Before(expires):
		view.State = "Expired"
	case view.PublicKey == "":
		view.State = "Pending"
	case observed == object.GetGeneration() && text("status", "phase") == "Joined":
		view.State = "Joined"
	default:
		view.State = "Redeemed"
	}
	if redeemed, err := time.Parse(time.RFC3339, view.RedeemedAt); err == nil && view.State == "Redeemed" {
		view.Stuck = now.Sub(redeemed) > joinStuckAfter
	}
	// Where the installer stopped on a machine joined by SSH (the Gateway follows its output).
	if failure := object.GetAnnotations()[nodeAdmissionInstallFailureAnnotation]; failure != "" && view.State != "Joined" {
		view.Reason = failure
	}
	return view
}

// decodeOneJSON reads a request body that is one JSON object of into's fields, at most limit bytes (or nothing,
// when emptyAllowed). On failure it has answered.
func decodeOneJSON(c *gin.Context, limit int64, into any, emptyAllowed bool) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, limit))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(into)
	if err == io.EOF && emptyAllowed {
		return true
	}
	var trailing any
	if err != nil || decoder.Decode(&trailing) != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "the request must be one JSON object of the documented fields"})
		return false
	}
	return true
}

func joinCodeTTL(seconds int64) (time.Duration, error) {
	if seconds == 0 {
		return defaultJoinCodeTTL, nil
	}
	ttl := time.Duration(seconds) * time.Second
	if ttl < minJoinCodeTTL || ttl > maxJoinCodeTTL {
		return 0, fmt.Errorf("ttl_seconds must be between %d and %d", int64(minJoinCodeTTL.Seconds()), int64(maxJoinCodeTTL.Seconds()))
	}
	return ttl, nil
}

func validateIssueNodeAdmission(request *issueNodeAdmissionRequest) error {
	if request.ClusterID == "" {
		return errors.New("cluster_id is required")
	}
	if len(validation.IsDNS1123Subdomain(request.NodeName)) > 0 {
		return errors.New("node_name must be a DNS subdomain")
	}
	if request.Site != "" && len(validation.IsDNS1123Label(request.Site)) > 0 {
		return errors.New("site must be a DNS label")
	}
	if request.NodeClass != "" && len(validation.IsDNS1123Label(request.NodeClass)) > 0 {
		return errors.New("node_class must be a DNS label")
	}
	for field, value := range map[string]string{"address": request.Address, "uplink": request.Uplink} {
		if ip := net.ParseIP(value); value != "" && (ip == nil || ip.To4() == nil) {
			return fmt.Errorf("%s must be an IPv4 address", field)
		}
	}
	if len(request.Labels) > 32 {
		return errors.New("at most 32 labels")
	}
	for key, value := range request.Labels {
		if len(validation.IsQualifiedName(key)) > 0 || len(validation.IsValidLabelValue(value)) > 0 {
			return fmt.Errorf("label %q is not a valid Kubernetes label", key)
		}
	}
	if len(request.Taints) > 8 {
		return errors.New("at most 8 taints")
	}
	for _, taint := range request.Taints {
		rest, effect, found := strings.Cut(taint, ":")
		key, value, _ := strings.Cut(rest, "=")
		if !found || (effect != "NoSchedule" && effect != "PreferNoSchedule" && effect != "NoExecute") ||
			len(validation.IsQualifiedName(key)) > 0 || len(validation.IsValidLabelValue(value)) > 0 {
			return fmt.Errorf("taint %q must be key[=value]:NoSchedule|PreferNoSchedule|NoExecute", taint)
		}
	}
	return nil
}

func newJoinCode() (string, string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	// 252 is the largest multiple of 36 that fits a byte: rejecting above it
	// keeps every character equally likely.
	code := make([]byte, 0, joinCodeLength)
	buffer := make([]byte, 2*joinCodeLength)
	for len(code) < joinCodeLength {
		if _, err := rand.Read(buffer); err != nil {
			return "", "", err
		}
		for _, b := range buffer {
			if b < 252 && len(code) < joinCodeLength {
				code = append(code, alphabet[int(b)%len(alphabet)])
			}
		}
	}
	digest := sha256.Sum256(code)
	return string(code), hex.EncodeToString(digest[:]), nil
}

// siteNetwork reads the one SiteNetwork; a management plane without one has no sites.
func (g *Gateway) siteNetwork(ctx context.Context) (*unstructured.Unstructured, error) {
	network, err := g.stationClient.Resource(siteNetworkGVR).Namespace("station-system").Get(ctx, "default", metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return &unstructured.Unstructured{Object: map[string]interface{}{}}, nil
	}
	return network, err
}

// leafUplinkOf returns the device a machine of this admission stands
// behind, when its site declares one: the SiteNetwork's entry for the node
// names the site, or else the admission, or else the site of its cluster.
func leafUplinkOf(network *unstructured.Unstructured, clusterID, nodeName, site string) string {
	nodes, _, _ := unstructured.NestedSlice(network.Object, "spec", "nodes")
	for _, raw := range nodes {
		entry, _ := raw.(map[string]interface{})
		if named, _ := entry["site"].(string); entry["clusterID"] == clusterID && entry["nodeName"] == nodeName && named != "" {
			site = named
		}
	}
	sites, _, _ := unstructured.NestedSlice(network.Object, "spec", "sites")
	for _, raw := range sites {
		entry, _ := raw.(map[string]interface{})
		clusters, _, _ := unstructured.NestedStringSlice(entry, "clusters")
		if site != "" && entry["name"] != site || site == "" && !slices.Contains(clusters, clusterID) {
			continue
		}
		if gateway, _, _ := unstructured.NestedString(entry, "leafUplink", "gateway"); gateway != "" {
			return gateway
		}
	}
	return ""
}

// stationAPIFor is where a machine reaches Station API to redeem its code: one the SiteNetwork declares with its key
// has its tunnel before it joins (a new site's machines, which do not reach Station API otherwise) and redeems through
// it; any other at the bootstrap address.
func (g *Gateway) stationAPIFor(network *unstructured.Unstructured, clusterID, nodeName string) string {
	nodes, _, _ := unstructured.NestedSlice(network.Object, "spec", "nodes")
	for _, raw := range nodes {
		entry, _ := raw.(map[string]interface{})
		if key, _ := entry["publicKey"].(string); entry["clusterID"] == clusterID && entry["nodeName"] == nodeName && key != "" {
			return g.stationBootstrap.TunnelAddress()
		}
	}
	return g.stationBootstrap.Address()
}

// joinCommand is the one line the operator runs on the machine. The machine
// has no CA of ours yet: the line fetches the CA unverified, checks it
// against the digest it carries, and verifies every connection after that
// with it. A machine of a class that stands behind a device of its own
// (NodeClass, uplink) on a site that says where that device is (leafUplink)
// first makes the device let it out, running the device's setup only if it
// is, byte for byte, the file Station API serves now.
func (g *Gateway) joinCommand(ctx context.Context, network *unstructured.Unstructured, clusterID, nodeName, site, class, code, caDigest string) (string, error) {
	address := g.stationAPIFor(network, clusterID, nodeName)
	trust := fmt.Sprintf(`c=$(mktemp) && curl --connect-timeout 15 -fsSk https://%s/v1/bootstrap/ca -o $c && echo "%s  $c" | sha256sum -c - && curl -fsS --cacert $c https://%s/b/%s`,
		address, caDigest, address, code)
	first := ""
	if gateway := leafUplinkOf(network, clusterID, nodeName, site); gateway != "" && class != "" {
		declared, err := g.stationClient.Resource(nodeClassGVR).Get(ctx, class, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("read NodeClass %s: %w", class, err)
		}
		login, _, _ := unstructured.NestedString(declared.Object, "spec", "access", "entry", "user")
		if _, declares, _ := unstructured.NestedMap(declared.Object, "spec", "uplink", "setup"); declares && login != "" {
			digest, err := g.stationBootstrap.Digest(ctx, "/b/"+code+"/uplink")
			if err != nil {
				return "", fmt.Errorf("read the uplink setup of NodeClass %s: %w", class, err)
			}
			first = fmt.Sprintf(`ssh -t %s@%s 'f=$(mktemp) %s/uplink -o $f && echo "%s  $f" | sha256sum -c - && sudo sh $f' && `, login, gateway, trust, digest)
		}
	}
	return first + trust + " | sudo sh", nil
}

// handleListNodeAdmissions answers GET /api/v1/station/node-admissions?cluster_id=:
// every admission of a cluster, newest code first.
func (g *Gateway) handleListNodeAdmissions(c *gin.Context) {
	clusterID, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return
	}
	list, err := g.stationClient.Resource(nodeAdmissionGVR).Namespace(namespace).List(c.Request.Context(), metav1.ListOptions{})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "NodeAdmission request failed"})
		return
	}
	joins, err := g.nodeJoinsOf(c.Request.Context(), clusterID)
	if err != nil {
		respondKubeError(c, "list node joins: "+err.Error(), err)
		return
	}
	now := time.Now()
	items := make([]nodeAdmission, 0, len(list.Items))
	for i := range list.Items {
		view := nodeAdmissionView(&list.Items[i], now)
		for _, j := range joins {
			if j.Spec.NodeName == view.NodeName {
				view.JoinPhase, view.JoinStage = string(j.Status.Phase), j.Status.Stage
			}
		}
		items = append(items, view)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].IssuedAt != items[j].IssuedAt {
			return items[i].IssuedAt > items[j].IssuedAt
		}
		return items[i].NodeName < items[j].NodeName
	})
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// handleIssueNodeAdmission answers POST /api/v1/station/node-admissions: it admits a
// machine as the named node and answers the code once. A node that has an
// admission keeps it, as what declares its tunnel address; a new code for it
// replaces what the admission declares and opens it again.
func (g *Gateway) handleIssueNodeAdmission(c *gin.Context) {
	var request issueNodeAdmissionRequest
	if !decodeOneJSON(c, maxNodeAdmissionRequestBytes, &request, false) {
		return
	}
	namespace, err := g.stationNamespace(c, request.ClusterID)
	if err != nil {
		return
	}
	issued, status, refused := g.admitNode(c.Request.Context(), c.GetString(authSubjectKey), namespace, &request, nil)
	if refused != nil {
		refused.respond(c)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(status, issued)
}

// The marks an admission carries of how its machine was joined: by SSH from
// the console (who logged in where, and the host key the machine showed the
// first time), and why the installer stopped there. A new code drops them.
const (
	nodeAdmissionSSHLoginAnnotation       = "station.rlark.io/ssh-login"
	nodeAdmissionSSHHostKeyAnnotation     = "station.rlark.io/ssh-host-key"
	nodeAdmissionInstallFailureAnnotation = "station.rlark.io/install-failure"
	// Station API and station-controller write these (stationidentity.SelfHealedAnnotation, JoinTokenIssuedAnnotation).
	nodeAdmissionSelfHealedAnnotation = "station.rlark.io/self-healed"
	nodeAdmissionJoinTokenAnnotation  = "station.rlark.io/join-token-issued-at"
)

// requestError is a request refused: its status, what it says, and for the
// console a reason it acts on and what the machine printed.
type requestError struct {
	status int
	reason string
	err    error
	output string
}

func refuse(status int, text string) *requestError {
	return &requestError{status: status, err: errors.New(text)}
}

func (e *requestError) Error() string { return e.err.Error() }

func (e *requestError) respond(c *gin.Context) {
	body := gin.H{"error": e.err.Error()}
	if e.reason != "" {
		body["reason"] = e.reason
	}
	if e.output != "" {
		body["output"] = e.output
	}
	c.JSON(e.status, body)
}

// admitNode writes the admission of a machine in the cluster's namespace and
// makes its code and its command: what the code tab and the SSH tab both do,
// and what any part of the Gateway that joins a machine calls.
func (g *Gateway) admitNode(ctx context.Context, actor, namespace string, request *issueNodeAdmissionRequest, marks map[string]string) (issuedNodeAdmission, int, *requestError) {
	ttl, err := joinCodeTTL(request.TTLSeconds)
	if err == nil {
		err = validateIssueNodeAdmission(request)
	}
	if err != nil {
		return issuedNodeAdmission{}, 0, &requestError{status: http.StatusBadRequest, err: err}
	}
	network, caDigest, code, codeHash, refused := g.newJoinCodeFor(ctx)
	if refused != nil {
		return issuedNodeAdmission{}, 0, refused
	}
	if request.Site != "" {
		declared := false
		sites, _, _ := unstructured.NestedSlice(network.Object, "spec", "sites")
		for _, raw := range sites {
			entry, _ := raw.(map[string]interface{})
			declared = declared || entry["name"] == request.Site
		}
		if !declared {
			return issuedNodeAdmission{}, 0, refuse(http.StatusBadRequest, "the SiteNetwork has no site "+request.Site)
		}
	}
	now := time.Now().UTC()
	spec := map[string]interface{}{
		"clusterID": request.ClusterID, "codeHash": codeHash,
		"expiresAt": now.Add(ttl).Format(time.RFC3339), "issuedBy": actor, "issuedAt": now.Format(time.RFC3339),
	}
	if request.Site != "" {
		spec["site"] = request.Site
	}
	if request.Address != "" {
		spec["address"] = request.Address
	}
	if request.Uplink != "" {
		spec["uplink"] = map[string]interface{}{"gateway": request.Uplink}
	}
	if request.NodeClass != "" {
		spec["nodeClass"] = request.NodeClass
	}
	if len(request.Taints) > 0 {
		taints := make([]interface{}, len(request.Taints))
		for i, taint := range request.Taints {
			taints[i] = taint
		}
		spec["taints"] = taints
	}
	if len(request.Labels) > 0 {
		labels := map[string]interface{}{}
		for key, value := range request.Labels {
			labels[key] = value
		}
		spec["labels"] = labels
	}
	if len(request.Machine) > 0 {
		spec["machine"] = request.Machine
	}
	resource := g.stationClient.Resource(nodeAdmissionGVR).Namespace(namespace)
	existing, err := resource.Get(ctx, request.NodeName, metav1.GetOptions{})
	var written *unstructured.Unstructured
	status := http.StatusOK
	switch {
	case apierrors.IsNotFound(err):
		status = http.StatusCreated
		admission := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "station.rlark.io/v1alpha1", "kind": "NodeAdmission",
			"metadata": map[string]interface{}{"name": request.NodeName, "namespace": namespace}, "spec": spec,
		}}
		admission.SetAnnotations(marks)
		written, err = resource.Create(ctx, admission, metav1.CreateOptions{})
	case err == nil:
		existing.Object["spec"] = spec
		dropJoinMarks(existing, marks)
		written, err = resource.Update(ctx, existing, metav1.UpdateOptions{})
	}
	if refused := nodeAdmissionWriteError(ctx, err); refused != nil {
		return issuedNodeAdmission{}, 0, refused
	}
	issued, refused := g.issuedNodeAdmission(ctx, network, written, code, caDigest, "issue")
	return issued, status, refused
}

// handleReissueNodeAdmission answers POST /api/v1/station/node-admissions/:name/reissue:
// a new code for what the admission declares; the earlier code stops working.
func (g *Gateway) handleReissueNodeAdmission(c *gin.Context) {
	var request reissueNodeAdmissionRequest
	if !decodeOneJSON(c, maxNodeAdmissionRequestBytes, &request, true) {
		return
	}
	ttl, err := joinCodeTTL(request.TTLSeconds)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	admission, resource, ok := g.existingNodeAdmission(c)
	if !ok {
		return
	}
	ctx := c.Request.Context()
	network, caDigest, code, codeHash, refused := g.newJoinCodeFor(ctx)
	if refused != nil {
		refused.respond(c)
		return
	}
	now := time.Now().UTC()
	spec, _, _ := unstructured.NestedMap(admission.Object, "spec")
	for _, field := range []string{"publicKey", "redeemedAt"} {
		delete(spec, field)
	}
	spec["codeHash"], spec["expiresAt"] = codeHash, now.Add(ttl).Format(time.RFC3339)
	spec["issuedBy"], spec["issuedAt"] = c.GetString(authSubjectKey), now.Format(time.RFC3339)
	admission.Object["spec"] = spec
	dropJoinMarks(admission, nil)
	written, err := resource.Update(ctx, admission, metav1.UpdateOptions{})
	if refused := nodeAdmissionWriteError(ctx, err); refused != nil {
		refused.respond(c)
		return
	}
	issued, refused := g.issuedNodeAdmission(ctx, network, written, code, caDigest, "reissue")
	if refused != nil {
		refused.respond(c)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, issued)
}

// handleRevokeNodeAdmission answers POST /api/v1/station/node-admissions/:name/revoke:
// the code stops working now: its expiry is set to now, which Station API and
// station-controller already treat as the end of the code. A code a machine
// has redeemed is that machine's; it is not withdrawn here.
func (g *Gateway) handleRevokeNodeAdmission(c *gin.Context) {
	admission, resource, ok := g.existingNodeAdmission(c)
	if !ok {
		return
	}
	now := time.Now().UTC()
	switch view := nodeAdmissionView(admission, now); view.State {
	case "Pending":
	case "Expired":
		c.JSON(http.StatusOK, gin.H{"admission": view})
		return
	default:
		c.JSON(http.StatusConflict, gin.H{"error": "a machine has redeemed the code; remove the node instead (a removal by name also clears a join that never made a node)"})
		return
	}
	_ = unstructured.SetNestedField(admission.Object, now.Format(time.RFC3339), "spec", "expiresAt")
	written, err := resource.Update(c.Request.Context(), admission, metav1.UpdateOptions{})
	if refused := nodeAdmissionWriteError(c.Request.Context(), err); refused != nil {
		refused.respond(c)
		return
	}
	view := nodeAdmissionView(written, now)
	log.FromContext(c.Request.Context()).WithName("audit").Info("node admission", "action", "revoke", "actor", c.GetString(authSubjectKey),
		"cluster", view.ClusterID, "node", view.NodeName)
	c.JSON(http.StatusOK, gin.H{"admission": view})
}

// newJoinCodeFor reads what a machine's command is made of (the SiteNetwork, the digest of Station API's
// bootstrap CA) and makes a code: nothing is admitted unless the command can be made.
func (g *Gateway) newJoinCodeFor(ctx context.Context) (network *unstructured.Unstructured, caDigest, code, codeHash string, refused *requestError) {
	network, err := g.siteNetwork(ctx)
	if err != nil {
		return nil, "", "", "", refuse(http.StatusBadGateway, "SiteNetwork request failed")
	}
	if caDigest, err = g.stationBootstrap.Digest(ctx, "/v1/bootstrap/ca"); err != nil {
		log.FromContext(ctx).Error(err, "read Station API's bootstrap CA")
		return nil, "", "", "", refuse(http.StatusBadGateway, "Station API does not serve its bootstrap CA")
	}
	if code, codeHash, err = newJoinCode(); err != nil {
		return nil, "", "", "", refuse(http.StatusInternalServerError, "cannot make a code")
	}
	return network, caDigest, code, codeHash, nil
}

// dropJoinMarks drops the marks of how the admission's machine was joined (a new code drops them) and sets marks.
func dropJoinMarks(admission *unstructured.Unstructured, marks map[string]string) {
	annotations := admission.GetAnnotations()
	for _, key := range []string{nodeAdmissionSSHLoginAnnotation, nodeAdmissionSSHHostKeyAnnotation, nodeAdmissionInstallFailureAnnotation, nodeAdmissionJoinTokenAnnotation, nodeAdmissionSelfHealedAnnotation} {
		delete(annotations, key)
	}
	if len(marks) > 0 && annotations == nil {
		annotations = map[string]string{}
	}
	maps.Copy(annotations, marks)
	admission.SetAnnotations(annotations)
}

func (g *Gateway) existingNodeAdmission(c *gin.Context) (*unstructured.Unstructured, dynamic.ResourceInterface, bool) {
	_, namespace, ok := g.clusterNamespace(c)
	if !ok {
		return nil, nil, false
	}
	resource := g.stationClient.Resource(nodeAdmissionGVR).Namespace(namespace)
	admission, err := resource.Get(c.Request.Context(), c.Param("name"), metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "node admission not found"})
		return nil, nil, false
	}
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "NodeAdmission request failed"})
		return nil, nil, false
	}
	return admission, resource, true
}

func nodeAdmissionWriteError(ctx context.Context, err error) *requestError {
	switch {
	case err == nil:
		return nil
	case apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err):
		return refuse(http.StatusConflict, "the admission changed meanwhile; try again")
	case apierrors.IsInvalid(err):
		return refuse(http.StatusBadRequest, "the management plane rejected the admission")
	default:
		log.FromContext(ctx).Error(err, "write NodeAdmission")
		return refuse(http.StatusBadGateway, "NodeAdmission request failed")
	}
}

// issuedNodeAdmission is the one answer that carries the code and its command.
func (g *Gateway) issuedNodeAdmission(ctx context.Context, network, written *unstructured.Unstructured, code, caDigest, action string) (issuedNodeAdmission, *requestError) {
	view := nodeAdmissionView(written, time.Now())
	command, err := g.joinCommand(ctx, network, view.ClusterID, view.NodeName, view.Site, view.NodeClass, code, caDigest)
	if err != nil {
		// The admission is written and its code is lost with this answer:
		// a new code replaces it.
		log.FromContext(ctx).Error(err, "make the join command", "cluster", view.ClusterID, "node", view.NodeName)
		return issuedNodeAdmission{}, refuse(http.StatusBadGateway, "the admission is written but its command cannot be made; give out a new code")
	}
	log.FromContext(ctx).WithName("audit").Info("node admission", "action", action, "actor", view.IssuedBy,
		"cluster", view.ClusterID, "node", view.NodeName, "expiresAt", view.ExpiresAt)
	return issuedNodeAdmission{Admission: view, Code: code, Command: command}, nil
}
