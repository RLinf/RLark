package stationidentity

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/addons"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var nodeAdmissionGVR = schema.GroupVersionResource{Group: stationGroup, Version: stationVersion, Resource: "nodeadmissions"}
var nodeClassGVR = schema.GroupVersionResource{Group: stationGroup, Version: stationVersion, Resource: "nodeclasses"}
var addonGVR = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "addons"}
var rlarkNodeGVR = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: "nodes"}

// Bootstrap serves a machine that is not a node yet. Its only credential is
// the one-time code of a NodeAdmission: the code buys the installer, the
// machine's tunnel configuration and a short-lived token to join its cluster.
type Bootstrap struct {
	Management dynamic.Interface
	// Member returns a client of the registered cluster that may create a
	// bootstrap token in kube-system and read kube-public/cluster-info.
	Member func(ctx context.Context, clusterID string) (kubernetes.Interface, error)
	// CA is the PEM certificate the machine verifies this API with.
	CA []byte
	// BundleFile is the installer as it lies on a machine, a tar
	// (astribot/station/bootstrap/build.sh, OUT.bundle.tar).
	BundleFile string
	// AddonsDir holds what the site's addons publish, one directory each. A
	// NodeClass's uplink.setup names a file of a built-in addon or one in it.
	AddonsDir string
	Now       func() time.Time

	mu       sync.Mutex
	failures []time.Time

	// bundleDigest is the sha256 of BundleFile as it was at bundleStamp (its size and mtime).
	digestMu     sync.Mutex
	bundleDigest string
	bundleStamp  string
}

type redeemRequest struct {
	Code      string `json:"code"`
	PublicKey string `json:"public_key"`
}

type BootstrapJoin struct {
	APIServer  string `json:"api_server"`
	Token      string `json:"token"`
	CACertHash string `json:"ca_cert_hash"`
}

type BootstrapAnswer struct {
	State         string          `json:"state"`
	Reason        string          `json:"reason,omitempty"`
	ClusterID     string          `json:"cluster_id,omitempty"`
	NodeName      string          `json:"node_name,omitempty"`
	TunnelAddress string          `json:"tunnel_address,omitempty"`
	Config        json.RawMessage `json:"config,omitempty"`
	Join          *BootstrapJoin  `json:"join,omitempty"`
	Labels        string          `json:"labels,omitempty"`
	Taints        string          `json:"taints,omitempty"`
}

func (b *Bootstrap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/b/") && strings.HasSuffix(r.URL.Path, "/uplink"):
		b.uplink(w, r, strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/b/"), "/uplink"))
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/b/"):
		b.script(w, r, strings.TrimPrefix(r.URL.Path, "/b/"))
	case r.Method == http.MethodGet && r.URL.Path == "/v1/bootstrap/ca":
		// Public: an operator's command fetches it without verifying the
		// connection, compares it with the digest the command holds, and
		// verifies every connection after that with it. The CA outlives the
		// certificates it signs, so a command stays good when this API's
		// certificate is renewed.
		w.Header().Set("Content-Type", "application/x-pem-file")
		_, _ = w.Write(b.CA)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/bootstrap/bundle":
		// The archive is hundreds of megabytes; the server's write timeout is for API calls.
		_ = http.NewResponseController(w).SetWriteDeadline(clock(b.Now).Add(30 * time.Minute))
		w.Header().Set("Content-Type", "application/x-tar")
		http.ServeFile(w, r, b.BundleFile)
	case r.Method == http.MethodGet && r.URL.Path == "/v1/bootstrap/bundle.sha256":
		digest, err := b.BundleDigest()
		if err != nil {
			log.Printf("installer digest: %v", err)
			writeError(w, http.StatusServiceUnavailable, "the installer cannot be read")
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, digest+"\n")
	case r.Method == http.MethodPost && r.URL.Path == "/v1/bootstrap/redeem":
		b.redeem(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

// script is what the operator's one command pipes into a shell: it fetches
// the installer over a connection verified with the CA it carries and runs
// it, the same files RLark sends a machine it joins over SSH.
func (b *Bootstrap) script(w http.ResponseWriter, r *http.Request, code string) {
	if !validCode(code) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript")
	fmt.Fprintf(w, `#!/bin/sh
# RLark node bootstrap. Unpacks the installer and runs it once. The machine
# needs curl and tar; the installer brings everything else.
set -eu
[ "$(id -u)" = 0 ] || { echo "run with sudo" >&2; exit 1; }
B=/var/lib/rlark/bootstrap
mkdir -p $B
cat > $B/station-api-ca.crt <<'RLARK_CA'
%sRLARK_CA
want=$(curl -fsS --cacert $B/station-api-ca.crt https://%s/v1/bootstrap/bundle.sha256)
if [ "$(cat $B/bundle.sha256 2>/dev/null)" = "$want" ]; then
  echo "$(date +%%T) the installer of sha256 $want is here already"
else
  echo "$(date +%%T) loading the installer"
  bundle=$B/installer.bundle.tar
  attempt=0
  until curl -fSs -C - --cacert $B/station-api-ca.crt -o $bundle https://%s/v1/bootstrap/bundle; do
    attempt=$((attempt + 1))
    [ $attempt -lt 30 ] || { echo "the installer did not load in 30 attempts" >&2; exit 1; }
    echo "the installer's download broke off; it goes on in 10 s ($attempt/30)" >&2
    sleep 10
  done
  # A part left by an earlier installer makes another file: it goes, and the line is run again.
  echo "$want  $bundle" | sha256sum -c --status - || { rm -f $bundle; echo "the installer loaded is not of sha256 $want: run the line again" >&2; exit 1; }
  rm -f $B/bundle.sha256
  tar -xf $bundle -C $B
  echo "$want" > $B/bundle.sha256
  rm -f $bundle
fi
RLARK_SERVER=https://%s RLARK_CODE=%s RLARK_CA_FILE=$B/station-api-ca.crt exec $B/station-bootstrap
`, b.CA, r.Host, r.Host, r.Host, code)
}

// BundleDigest is the sha256 of the installer, computed again only when its file changes (it is about 2 GB).
func (b *Bootstrap) BundleDigest() (string, error) {
	b.digestMu.Lock()
	defer b.digestMu.Unlock()
	info, err := os.Stat(b.BundleFile)
	if err != nil {
		return "", err
	}
	stamp := fmt.Sprintf("%d %d", info.Size(), info.ModTime().UnixNano())
	if stamp == b.bundleStamp {
		return b.bundleDigest, nil
	}
	file, err := os.Open(b.BundleFile)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	b.bundleDigest, b.bundleStamp = hex.EncodeToString(sum.Sum(nil)), stamp
	return b.bundleDigest, nil
}

// uplink serves the device a new leaf stands behind (a robot's router): until
// that device lets the leaf out, the leaf reaches nothing, this API included,
// so the device fetches what makes it do so, with the leaf's code. The file
// is the one the machine's NodeClass declares (uplink.setup), published by an addon;
// this API does not know what is in it. The operator's command holds its
// digest and runs it only if it is that file.
func (b *Bootstrap) uplink(w http.ResponseWriter, r *http.Request, code string) {
	if b.throttled(false) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many wrong codes")
		return
	}
	setup, known, err := b.UplinkSetup(r.Context(), code)
	if err != nil {
		// The reason stays here: the caller is a machine nobody knows yet.
		log.Printf("bootstrap uplink: %v", err)
		writeError(w, http.StatusServiceUnavailable, "bootstrap storage unavailable")
		return
	}
	if setup == nil {
		if !known {
			b.throttled(true)
		}
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript")
	_, _ = w.Write(setup)
}

// UplinkSetup returns the uplink setup of the class of the machine the code
// admits (NodeClass, uplink), while the code can still be redeemed: once the machine has joined
// with it, the code opens nothing. known says whether the code is one.
func (b *Bootstrap) UplinkSetup(ctx context.Context, code string) (setup []byte, known bool, err error) {
	if !validCode(code) {
		return nil, false, nil
	}
	admission, _, err := b.admission(ctx, code)
	if err != nil || admission == nil {
		return nil, false, err
	}
	expires, _, _ := unstructured.NestedString(admission.Object, "spec", "expiresAt")
	redeemedBy, _, _ := unstructured.NestedString(admission.Object, "spec", "publicKey")
	if at, err := time.Parse(time.RFC3339, expires); err != nil || !clock(b.Now).Before(at) || redeemedBy != "" {
		return nil, true, nil
	}
	name, _, _ := unstructured.NestedString(admission.Object, "spec", "nodeClass")
	if name == "" {
		return nil, true, nil
	}
	class, err := b.Management.Resource(nodeClassGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, true, fmt.Errorf("read NodeClass %s: %w", name, err)
	}
	addon, _, _ := unstructured.NestedString(class.Object, "spec", "uplink", "setup", "addon")
	path, _, _ := unstructured.NestedString(class.Object, "spec", "uplink", "setup", "path")
	if addon == "" {
		return nil, true, nil
	}
	// A built-in addon's files are this program's own; a site's addon publishes in the directory.
	if setup, builtIn, err := addons.Published(addon, path); builtIn {
		if err != nil {
			return nil, true, fmt.Errorf("read the uplink setup %s of addon %s: %w", path, addon, err)
		}
		return setup, true, nil
	}
	// Within the directory, whatever the declaration says.
	root, err := os.OpenRoot(b.AddonsDir)
	if err != nil {
		return nil, true, fmt.Errorf("open the addons directory: %w", err)
	}
	defer root.Close()
	setup, err = root.ReadFile(filepath.Join(addon, path))
	if err != nil {
		return nil, true, fmt.Errorf("read the uplink setup %s of addon %s: %w", path, addon, err)
	}
	return setup, true, nil
}

// admission returns the NodeAdmission the code belongs to, nil if none,
// and every admission there is.
func (b *Bootstrap) admission(ctx context.Context, code string) (*unstructured.Unstructured, []unstructured.Unstructured, error) {
	digest := sha256.Sum256([]byte(code))
	hash := hex.EncodeToString(digest[:])
	list, err := b.Management.Resource(nodeAdmissionGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("list NodeAdmissions: %w", err)
	}
	var admission *unstructured.Unstructured
	for i := range list.Items {
		stored, _, _ := unstructured.NestedString(list.Items[i].Object, "spec", "codeHash")
		if subtle.ConstantTimeCompare([]byte(stored), []byte(hash)) == 1 {
			admission = &list.Items[i]
		}
	}
	return admission, list.Items, nil
}

// SelfHealedAnnotation is, on a NodeAdmission, what RLark found of an earlier
// attempt of the machine and cleared away by itself, said as the console
// shows it. station-controller writes it too.
const SelfHealedAnnotation = "station.rlark.io/self-healed"

// JoinTokenIssuedAnnotation is, on a NodeAdmission, when Station API last gave
// the machine that redeemed it a token to join its cluster with.
const JoinTokenIssuedAnnotation = "station.rlark.io/join-token-issued-at"

// reclaim deletes what a machine left of the joins it began with this key and
// never finished: the admissions it redeemed under other names that never led
// to a node. A machine keeps its key across attempts, so another code it
// redeems is the same machine starting over; until those go, the SiteNetwork
// has two members with one key and configures neither. An admission a node
// joined with, one of a node joining again, or one of a discovered machine
// (Serial, the Machine's own) is not an attempt left behind. It answers the
// names it cleared.
func (b *Bootstrap) reclaim(ctx context.Context, redeeming *unstructured.Unstructured, all []unstructured.Unstructured, publicKey string) ([]string, error) {
	var cleared []string
	for i := range all {
		other := &all[i]
		text := func(fields ...string) string {
			value, _, _ := unstructured.NestedString(other.Object, fields...)
			return value
		}
		if other.GetNamespace() == redeeming.GetNamespace() && other.GetName() == redeeming.GetName() || text("spec", "publicKey") != publicKey || text("spec", "serial") != "" ||
			text("status", "phase") == "Joined" || text("status", "rejoinsNodeUID") != "" {
			continue
		}
		uid := other.GetUID()
		err := b.Management.Resource(nodeAdmissionGVR).Namespace(other.GetNamespace()).Delete(ctx, other.GetName(), metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("clear the unfinished admission %s/%s of this machine: %w", other.GetNamespace(), other.GetName(), err)
		}
		cleared = append(cleared, text("spec", "clusterID")+"/"+other.GetName())
	}
	return cleared, nil
}

func validCode(code string) bool {
	if len(code) < 8 || len(code) > 32 {
		return false
	}
	for _, c := range code {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// throttled keeps guessing a code slow: ten wrong codes a minute, for everyone.
func (b *Bootstrap) throttled(failed bool) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	cutoff := clock(b.Now).Add(-time.Minute)
	for len(b.failures) > 0 && b.failures[0].Before(cutoff) {
		b.failures = b.failures[1:]
	}
	if failed {
		b.failures = append(b.failures, clock(b.Now))
	}
	return len(b.failures) > 10
}

func (b *Bootstrap) redeem(w http.ResponseWriter, r *http.Request) {
	var req redeemRequest
	if !decodeBody(w, r, &req) {
		return
	}
	if key, err := base64.StdEncoding.DecodeString(req.PublicKey); err != nil || len(key) != 32 || !validCode(req.Code) {
		writeError(w, http.StatusBadRequest, "code and a WireGuard public_key are required")
		return
	}
	if b.throttled(false) {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "too many wrong codes")
		return
	}
	answer, status, err := b.Redeem(r.Context(), req.Code, req.PublicKey)
	if err != nil {
		log.Printf("bootstrap redeem: %v", err)
		// What the machine's cluster refuses is said as it is: an administrator
		// fixes it and the machine runs its command again.
		var refusal *RedeemRefusal
		if errors.As(err, &refusal) {
			writeError(w, http.StatusServiceUnavailable, refusal.Reason)
			return
		}
		writeError(w, http.StatusServiceUnavailable, "bootstrap storage unavailable")
		return
	}
	if status == http.StatusNotFound {
		b.throttled(true)
	}
	writeJSON(w, status, answer)
}

// Redeem binds the code to the machine's key, and once station-controller has
// computed the machine's tunnel returns it with a fresh join token.
func (b *Bootstrap) Redeem(ctx context.Context, code, publicKey string) (BootstrapAnswer, int, error) {
	admission, all, err := b.admission(ctx, code)
	if err != nil {
		return BootstrapAnswer{}, 0, err
	}
	if admission == nil {
		return BootstrapAnswer{State: "rejected", Reason: "unknown code"}, http.StatusNotFound, nil
	}
	text := func(fields ...string) string {
		value, _, _ := unstructured.NestedString(admission.Object, fields...)
		return value
	}
	switch redeemedBy := text("spec", "publicKey"); {
	case redeemedBy == "":
		expires, err := time.Parse(time.RFC3339, text("spec", "expiresAt"))
		if err != nil || !clock(b.Now).Before(expires) {
			return BootstrapAnswer{State: "rejected", Reason: "the code has expired"}, http.StatusGone, nil
		}
		// The code is bound to the machine only once its cluster can give the
		// machine what it joins with: a refusal leaves the code as it was.
		if _, _, _, err := b.joinable(ctx, text("spec", "clusterID")); err != nil {
			return BootstrapAnswer{}, 0, err
		}
		cleared, err := b.reclaim(ctx, admission, all, publicKey)
		if err != nil {
			return BootstrapAnswer{}, 0, err
		}
		if len(cleared) > 0 {
			annotations := admission.GetAnnotations()
			if annotations == nil {
				annotations = map[string]string{}
			}
			annotations[SelfHealedAnnotation] = "这台机器上次以 " + strings.Join(cleared, "、") + " 入网没走完（同一个隧道公钥，从没成为节点）：那条入网记录已自动清掉，公钥和隧道地址归这次入网"
			admission.SetAnnotations(annotations)
		}
		_ = unstructured.SetNestedField(admission.Object, publicKey, "spec", "publicKey")
		_ = unstructured.SetNestedField(admission.Object, clock(b.Now).UTC().Format(time.RFC3339), "spec", "redeemedAt")
		if _, err := b.Management.Resource(nodeAdmissionGVR).Namespace(admission.GetNamespace()).Update(ctx, admission, metav1.UpdateOptions{}); err != nil && !apierrors.IsConflict(err) {
			return BootstrapAnswer{}, 0, fmt.Errorf("record the redeemed code: %w", err)
		}
		return BootstrapAnswer{State: "pending", Reason: "waiting for station-controller"}, http.StatusAccepted, nil
	case redeemedBy != publicKey:
		return BootstrapAnswer{State: "rejected", Reason: "the code was redeemed by another machine"}, http.StatusConflict, nil
	}
	// A status that has not seen this redemption yet is not an answer to it:
	// a code given to a node for joining again leaves the old "Joined" there
	// until station-controller has answered.
	observed, _, _ := unstructured.NestedInt64(admission.Object, "status", "observedGeneration")
	if observed != admission.GetGeneration() {
		return BootstrapAnswer{State: "pending", Reason: "waiting for station-controller"}, http.StatusAccepted, nil
	}
	if text("status", "phase") == "Joined" {
		return BootstrapAnswer{State: "rejected", Reason: "the node has already joined"}, http.StatusGone, nil
	}
	config := text("status", "config")
	if config == "" || text("status", "publicKey") != publicKey || !json.Valid([]byte(config)) {
		reason := text("status", "reason")
		if reason == "" {
			reason = "waiting for station-controller"
		}
		return BootstrapAnswer{State: "pending", Reason: reason}, http.StatusAccepted, nil
	}

	clusterID := text("spec", "clusterID")
	// A node of a site whose registry cache is still being filled would pull
	// through the uplink what the cache is there to spare it: it waits, and
	// is told what the cache's addon says. A cluster with no cache waits for none.
	// (A management plane that knows no Addons has no cache either.)
	addons, err := b.Management.Resource(addonGVR).Namespace(clusterID).List(ctx, metav1.ListOptions{})
	if apierrors.IsNotFound(err) {
		addons, err = &unstructured.UnstructuredList{}, nil
	}
	if err != nil {
		return BootstrapAnswer{}, 0, fmt.Errorf("list the Addons of %s: %w", clusterID, err)
	}
	for _, addon := range addons.Items {
		kind, _, _ := unstructured.NestedString(addon.Object, "spec", "addonName")
		role, _, _ := unstructured.NestedString(addon.Object, "spec", "values", "role")
		phase, _, _ := unstructured.NestedString(addon.Object, "status", "phase")
		seen, _, _ := unstructured.NestedInt64(addon.Object, "status", "observedGeneration")
		if kind == "site-registry" && role == "cache" && (phase != "Ready" || seen != addon.GetGeneration()) {
			message, _, _ := unstructured.NestedString(addon.Object, "status", "message")
			if message == "" || phase == "Ready" {
				message = "the site's registry cache " + addon.GetName() + " has not reported on its last change yet"
			}
			return BootstrapAnswer{State: "pending", Reason: message}, http.StatusAccepted, nil
		}
	}
	member, server, caHash, err := b.joinable(ctx, clusterID)
	if err != nil {
		return BootstrapAnswer{}, 0, err
	}
	join, err := newJoinToken(ctx, member, clusterID, admission.GetName(), caHash, clock(b.Now))
	if err != nil {
		return BootstrapAnswer{}, 0, err
	}
	// The machine joins where every node of the cluster reaches its API: what
	// cluster-info says, which kubeadm join goes on with after its discovery anyway.
	join.APIServer = server
	labels, _, _ := unstructured.NestedStringMap(admission.Object, "spec", "labels")
	// A machine that joins by a code is one RLark manages, like a machine
	// joined by a node operation: the cluster's agents report only such nodes.
	if labels == nil {
		labels = map[string]string{}
	}
	labels["rlark.io/enabled"], labels["rlark.io/cluster-id"] = "true", clusterID
	if text("spec", "machine", "category") == "Server" {
		labels["station.rlark.io/machine-type"] = "server"
	}
	// A machine installed anew under a name the management plane knows has a
	// management Node already, with the labels declared for it before: the
	// agent would put those back over what this admission declares.
	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"labels": labels}})
	// The admission stands in the cluster's agent namespace, where its management Nodes are.
	if _, err := b.Management.Resource(rlarkNodeGVR).Namespace(admission.GetNamespace()).Patch(ctx, admission.GetName(), types.MergePatchType, patch, metav1.PatchOptions{}); err != nil && !apierrors.IsNotFound(err) {
		return BootstrapAnswer{}, 0, fmt.Errorf("declare the labels on management Node %s/%s: %w", admission.GetNamespace(), admission.GetName(), err)
	}
	// Where the join stands, for the admission to say where it stopped: the
	// machine has what it joins its cluster with from now on.
	if annotations := admission.GetAnnotations(); annotations[JoinTokenIssuedAnnotation] < text("spec", "redeemedAt") {
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[JoinTokenIssuedAnnotation] = clock(b.Now).UTC().Format(time.RFC3339)
		admission.SetAnnotations(annotations)
		if _, err := b.Management.Resource(nodeAdmissionGVR).Namespace(admission.GetNamespace()).Update(ctx, admission, metav1.UpdateOptions{}); err != nil && !apierrors.IsConflict(err) {
			return BootstrapAnswer{}, 0, fmt.Errorf("record that the machine has its join token: %w", err)
		}
	}
	pairs := make([]string, 0, len(labels))
	for key, value := range labels {
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	taints, _, _ := unstructured.NestedStringSlice(admission.Object, "spec", "taints")
	return BootstrapAnswer{
		State: "ready", ClusterID: clusterID, NodeName: admission.GetName(), TunnelAddress: text("status", "address"),
		Config: json.RawMessage(config), Join: &join, Labels: strings.Join(pairs, ","), Taints: strings.Join(taints, ","),
	}, http.StatusOK, nil
}

// RedeemRefusal is a redemption the machine's cluster refuses, said as an
// administrator acts on it. One before the code is bound to the machine
// leaves the code as it was; one after it, the same machine (the same key)
// redeems again once it is fixed.
type RedeemRefusal struct{ Reason string }

func (r *RedeemRefusal) Error() string { return r.Reason }

// memberRefusal says what the cluster refused Station API, and which part of
// astribot/station/deploy/member-rbac.yaml lets it when it is permission.
func memberRefusal(clusterID, what, role string, err error) *RedeemRefusal {
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		return &RedeemRefusal{fmt.Sprintf("%s被拒：集群 %s 缺 %s 权限（astribot/station/deploy/member-rbac.yaml）", what, clusterID, role)}
	}
	return &RedeemRefusal{fmt.Sprintf("%s失败：集群 %s：%v", what, clusterID, err)}
}

// joinable says whether the machine's cluster can give it what it joins
// with: where its API is and the CA it pins (kube-public/cluster-info, host:port
// of its server), and a join token.
func (b *Bootstrap) joinable(ctx context.Context, clusterID string) (member kubernetes.Interface, server, caHash string, err error) {
	member, err = b.Member(ctx, clusterID)
	if err != nil {
		return nil, "", "", &RedeemRefusal{fmt.Sprintf("连不上集群 %s：%v", clusterID, err)}
	}
	info, err := member.CoreV1().ConfigMaps("kube-public").Get(ctx, "cluster-info", metav1.GetOptions{})
	if err != nil {
		return nil, "", "", memberRefusal(clusterID, "读取集群信息（kube-public/cluster-info）", "station-api-cluster-info", err)
	}
	kubeconfig, err := clientcmd.Load([]byte(info.Data["kubeconfig"]))
	if err != nil || len(kubeconfig.Clusters) != 1 {
		return nil, "", "", &RedeemRefusal{"集群 " + clusterID + " 的 kube-public/cluster-info 里没有唯一的集群"}
	}
	for _, cluster := range kubeconfig.Clusters {
		endpoint, err := url.Parse(cluster.Server)
		if err != nil || endpoint.Host == "" {
			return nil, "", "", &RedeemRefusal{"集群 " + clusterID + " 的 kube-public/cluster-info 里没有 API 地址"}
		}
		server = endpoint.Host
		block, _ := pem.Decode(cluster.CertificateAuthorityData)
		if block == nil {
			return nil, "", "", &RedeemRefusal{"集群 " + clusterID + " 的 kube-public/cluster-info 里没有 CA 证书"}
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, "", "", &RedeemRefusal{fmt.Sprintf("集群 %s 的 CA 证书读不出：%v", clusterID, err)}
		}
		sum := sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
		caHash = "sha256:" + hex.EncodeToString(sum[:])
	}
	review, err := member.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{ResourceAttributes: &authorizationv1.ResourceAttributes{Namespace: "kube-system", Verb: "create", Resource: "secrets"}},
	}, metav1.CreateOptions{})
	if err != nil {
		return nil, "", "", memberRefusal(clusterID, "核对建入网令牌的权限", "station-api-join-token", err)
	}
	if !review.Status.Allowed {
		return nil, "", "", &RedeemRefusal{fmt.Sprintf("建入网令牌被拒：集群 %s 缺 station-api-join-token 权限（astribot/station/deploy/member-rbac.yaml）", clusterID)}
	}
	return member, server, caHash, nil
}

// newJoinToken creates a kubeadm bootstrap token that lives half an hour,
// with the hash of the cluster's CA kubeadm join pins.
func newJoinToken(ctx context.Context, member kubernetes.Interface, clusterID, nodeName, caHash string, now time.Time) (BootstrapJoin, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	random := make([]byte, 22)
	if _, err := rand.Read(random); err != nil {
		return BootstrapJoin{}, err
	}
	for i := range random {
		random[i] = alphabet[int(random[i])%len(alphabet)]
	}
	id, secret := string(random[:6]), string(random[6:])
	_, err := member.CoreV1().Secrets("kube-system").Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "bootstrap-token-" + id, Namespace: "kube-system"},
		Type:       corev1.SecretTypeBootstrapToken,
		StringData: map[string]string{
			"token-id": id, "token-secret": secret, "expiration": now.Add(30 * time.Minute).UTC().Format(time.RFC3339),
			"usage-bootstrap-authentication": "true", "usage-bootstrap-signing": "true",
			"auth-extra-groups": "system:bootstrappers:kubeadm:default-node-token",
			"description":       "RLark node admission of " + nodeName,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return BootstrapJoin{}, memberRefusal(clusterID, "建入网令牌", "station-api-join-token", err)
	}
	return BootstrapJoin{Token: id + "." + secret, CACertHash: caHash}, nil
}
