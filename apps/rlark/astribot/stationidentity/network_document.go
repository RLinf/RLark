package stationidentity

import (
	"context"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// KeyIndex is the objects of one kind by the WireGuard public key in their
// status, as last listed. Every node asks every 30 s, so the objects are
// listed once for all of them: again after 30 s, or after 5 s when a key is
// asked for that the list does not have, which is how a node that has just
// enrolled looks.
type KeyIndex struct {
	Client   dynamic.Interface
	Resource schema.GroupVersionResource
	// Keep says which objects speak for their key (an enrollment that was
	// rejected leaves its key behind).
	Keep func(*unstructured.Unstructured) bool
	Now  func() time.Time

	mu      sync.Mutex
	objects map[string]*unstructured.Unstructured
	listed  time.Time
}

// EnrolledKeys indexes the NetworkEnrollments that are Enrolled.
func EnrolledKeys(client dynamic.Interface) *KeyIndex {
	return &KeyIndex{Client: client, Resource: networkEnrollmentGVR, Keep: func(enrollment *unstructured.Unstructured) bool {
		phase, _, _ := unstructured.NestedString(enrollment.Object, "status", "phase")
		return phase == "Enrolled"
	}}
}

// AdmittedKeys indexes the NodeAdmissions that hold a document: those of
// machines that have redeemed their code and have not enrolled yet.
func AdmittedKeys(client dynamic.Interface) *KeyIndex {
	return &KeyIndex{Client: client, Resource: nodeAdmissionGVR, Keep: func(admission *unstructured.Unstructured) bool {
		config, _, _ := unstructured.NestedString(admission.Object, "status", "config")
		return config != ""
	}}
}

// Lookup returns the objects by key. The map is not written to again.
func (x *KeyIndex) Lookup(ctx context.Context, keys ...string) (map[string]*unstructured.Unstructured, error) {
	now := clock(x.Now)
	x.mu.Lock()
	defer x.mu.Unlock()
	age := now.Sub(x.listed)
	for _, key := range keys {
		if x.objects[key] == nil && age > 5*time.Second {
			age = time.Minute
		}
	}
	if age > 30*time.Second {
		list, err := x.Client.Resource(x.Resource).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, err
		}
		x.objects, x.listed = map[string]*unstructured.Unstructured{}, now
		for i := range list.Items {
			if key, _, _ := unstructured.NestedString(list.Items[i].Object, "status", "publicKey"); x.Keep(&list.Items[i]) {
				x.objects[key] = &list.Items[i]
			}
		}
	}
	return x.objects, nil
}

// NetworkDocuments serves a node its own configuration document for the
// node's own identity, its WireGuard key, with no Pod and no cluster in
// between: a node whose kubelet looks for its control plane where it no
// longer is can start no agent Pod, and the document that says where the
// control plane is now was what only that Pod could fetch.
//
// The node proves it holds the private key without anything being stored for
// it on either side. It names its public key and is given a challenge and a
// public key of the server's made for that challenge alone; its proof is an
// HMAC over the challenge under the X25519 secret of its private key and the
// server's public one. The server keeps no challenge: the private half of
// its key is derived from a secret of this process, the challenge and the
// node's key, so it is made again when the proof arrives. A challenge is
// given for any key, so the answer does not say which keys are nodes; a
// process started since gives another, and the node asks again.
type NetworkDocuments struct {
	Client   dynamic.Interface
	Enrolled *KeyIndex
	Admitted *KeyIndex
	Now      func() time.Time

	secret [32]byte
}

func NewNetworkDocuments(client dynamic.Interface, enrolled *KeyIndex) (*NetworkDocuments, error) {
	d := &NetworkDocuments{Client: client, Enrolled: enrolled, Admitted: AdmittedKeys(client)}
	_, err := rand.Read(d.secret[:])
	return d, err
}

const (
	documentChallengeLife = time.Minute
	// DocumentProofLabel begins what the node's proof is an HMAC of; the
	// challenge follows it.
	DocumentProofLabel = "rlark station network document v1"
)

type documentRequest struct {
	PublicKey string `json:"public_key"`
	Challenge string `json:"challenge,omitempty"`
	Proof     string `json:"proof,omitempty"`
}

// serverKey is the server's key for one challenge of one node.
func (d *NetworkDocuments) serverKey(challenge, publicKey []byte) *ecdh.PrivateKey {
	// Neither call fails for these lengths.
	seed, _ := hkdf.Key(sha256.New, d.secret[:], nil, "rlark station network document challenge"+string(challenge)+string(publicKey), 32)
	key, _ := ecdh.X25519().NewPrivateKey(seed)
	return key
}

// proven says whether the request proves its public key; if not, it has
// answered the request, with a challenge.
func (d *NetworkDocuments) proven(w http.ResponseWriter, request documentRequest) bool {
	publicKey, err := base64.StdEncoding.DecodeString(request.PublicKey)
	if err != nil || len(publicKey) != 32 {
		writeError(w, http.StatusBadRequest, "public_key must be a WireGuard public key")
		return false
	}
	now := clock(d.Now)
	challenge, _ := base64.StdEncoding.DecodeString(request.Challenge)
	proof, _ := base64.StdEncoding.DecodeString(request.Proof)
	proven := false
	if len(challenge) == 24 && now.Sub(time.Unix(int64(binary.BigEndian.Uint64(challenge)), 0)) <= documentChallengeLife {
		node, _ := ecdh.X25519().NewPublicKey(publicKey)
		// A key of low order has no secret with anyone: it proves nothing.
		if shared, err := d.serverKey(challenge, publicKey).ECDH(node); err == nil {
			mac := hmac.New(sha256.New, shared)
			mac.Write([]byte(DocumentProofLabel))
			mac.Write(challenge)
			proven = hmac.Equal(mac.Sum(nil), proof)
		}
	}
	if !proven {
		// Also the answer to a proof that is wrong, too old or for the
		// challenge of another process: the node starts over with this one.
		challenge = binary.BigEndian.AppendUint64(nil, uint64(now.Unix()))
		challenge = append(challenge, make([]byte, 16)...)
		if _, err := rand.Read(challenge[8:]); err != nil {
			writeError(w, http.StatusServiceUnavailable, "no challenge can be made")
			return false
		}
		writeJSON(w, http.StatusUnauthorized, map[string]string{
			"challenge":  base64.StdEncoding.EncodeToString(challenge),
			"server_key": base64.StdEncoding.EncodeToString(d.serverKey(challenge, publicKey).PublicKey().Bytes()),
		})
		return false
	}
	return true
}

func (d *NetworkDocuments) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var request documentRequest
	if !decodeBody(w, r, &request) || !d.proven(w, request) {
		return
	}
	config, hash, err := d.document(r.Context(), request.PublicKey)
	if err != nil {
		log.Printf("network document: %v", err)
		writeError(w, http.StatusServiceUnavailable, "network document storage unavailable")
		return
	}
	if config == "" {
		writeError(w, http.StatusNotFound, "no document for this key")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"config": json.RawMessage(config), "config_hash": hash})
}

// document is the document of the node that holds the key (holder).
func (d *NetworkDocuments) document(ctx context.Context, key string) (config, hash string, err error) {
	holder, err := d.holder(ctx, key)
	if holder == nil || err != nil {
		return "", "", err
	}
	config, _, _ = unstructured.NestedString(holder.Object, "status", "config")
	hash, _, _ = unstructured.NestedString(holder.Object, "status", "configHash")
	if config != "" && !json.Valid([]byte(config)) {
		return "", "", errors.New("the stored document of " + holder.GetNamespace() + "/" + holder.GetName() + " is not JSON")
	}
	if config == "" {
		return config, hash, nil
	}
	return d.withGatewayRelayToken(ctx, holder, config, hash)
}

// withGatewayRelayToken keeps the token outside Kubernetes status. Only an
// enrolled Hub proving possession of its WireGuard key receives the token.
func (d *NetworkDocuments) withGatewayRelayToken(ctx context.Context, holder *unstructured.Unstructured, config, hash string) (string, string, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(config), &payload); err != nil {
		return "", "", errors.New("stored network document is not an object")
	}
	var discovery map[string]json.RawMessage
	if raw := payload["discovery"]; len(raw) == 0 || json.Unmarshal(raw, &discovery) != nil {
		return config, hash, nil
	}
	// includeUnclassified is part of the level-3 wire contract and is always
	// emitted there, even when false. Its presence keeps Release A documents
	// at level 2 while Station API can enrich Release B Hub responses.
	if _, levelThree := discovery["includeUnclassified"]; !levelThree {
		return config, hash, nil
	}
	delete(discovery, "relayToken")
	role, _, _ := unstructured.NestedString(holder.Object, "status", "role")
	site, _, _ := unstructured.NestedString(holder.Object, "status", "site")
	if holder.GetKind() == "NetworkEnrollment" && role == "Hub" && site != "" {
		secrets := schema.GroupVersionResource{Version: "v1", Resource: "secrets"}
		secret, err := d.Client.Resource(secrets).Namespace("default").Get(ctx, "station-relay-token-"+site, metav1.GetOptions{})
		if err == nil {
			encoded, _, _ := unstructured.NestedString(secret.Object, "data", "token")
			token, decodeErr := base64.StdEncoding.DecodeString(encoded)
			random, tokenErr := base64.StdEncoding.DecodeString(string(token))
			if decodeErr == nil && tokenErr == nil && len(random) == 32 && base64.StdEncoding.EncodeToString(random) == string(token) {
				discovery["relayToken"], _ = json.Marshal(string(token))
			}
		} else if !apierrors.IsNotFound(err) && !apierrors.IsForbidden(err) {
			return "", "", errors.New("could not read the Hub's relay token")
		}
	}
	if len(discovery) == 0 {
		delete(payload, "discovery")
	} else {
		payload["discovery"], _ = json.Marshal(discovery)
	}
	data, _ := json.Marshal(payload)
	digest := sha256.Sum256(data)
	return string(data), hex.EncodeToString(digest[:]), nil
}

// holder is the object whose document is the node's that holds the key: its enrollment, read now (the index only
// says which enrollment it is), or else the admission the machine redeemed and has not yet enrolled from; nil, none.
func (d *NetworkDocuments) holder(ctx context.Context, key string) (*unstructured.Unstructured, error) {
	enrolled, err := d.Enrolled.Lookup(ctx, key)
	if err != nil {
		return nil, err
	}
	holder := enrolled[key]
	if holder != nil {
		holder, err = d.Client.Resource(networkEnrollmentGVR).Namespace(holder.GetNamespace()).Get(ctx, holder.GetName(), metav1.GetOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return nil, err
		}
	}
	if holder == nil || err != nil || !d.Enrolled.Keep(holder) {
		admitted, err := d.Admitted.Lookup(ctx, key)
		if err != nil {
			return nil, err
		}
		if holder = admitted[key]; holder == nil {
			return nil, nil
		}
	}
	// The key may have left the object since it was listed.
	if held, _, _ := unstructured.NestedString(holder.Object, "status", "publicKey"); held != key {
		return nil, nil
	}
	return holder, nil
}
