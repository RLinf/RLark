package stationidentity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var networkEnrollmentGVR = schema.GroupVersionResource{Group: stationGroup, Version: stationVersion, Resource: "networkenrollments"}

// NetworkEnrollmentRequest is what a node reports to join the site network.
// The private key never leaves the node; the public key is not a credential:
// the request is bound to the node by its verified Pod token.
type NetworkEnrollmentRequest struct {
	PublicKey      string
	Addresses      []string
	ObservedSource string
}

// NetworkEnrollmentAnswer is the controller's current answer. Config is the
// exact document station-gateway reconciles and ConfigHash its SHA-256.
type NetworkEnrollmentAnswer struct {
	State         string          `json:"state"`
	Reason        string          `json:"reason,omitempty"`
	Address       string          `json:"address,omitempty"`
	TunnelAddress string          `json:"tunnel_address,omitempty"`
	Config        json.RawMessage `json:"config,omitempty"`
	ConfigHash    string          `json:"config_hash,omitempty"`
}

type NetworkEnroller interface {
	Enroll(context.Context, Evidence, NetworkEnrollmentRequest) (NetworkEnrollmentAnswer, error)
}

type enrollmentRequest struct {
	ClusterID string   `json:"cluster_id"`
	PublicKey string   `json:"public_key"`
	Addresses []string `json:"addresses"`
}

func (a *API) enroll(w http.ResponseWriter, r *http.Request, token string) {
	var req enrollmentRequest
	if !decodeBody(w, r, &req) {
		return
	}
	e, err := a.authenticate(r.Context(), req.ClusterID, token)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	if key, err := base64.StdEncoding.DecodeString(req.PublicKey); req.PublicKey != "" && (err != nil || len(key) != 32) {
		writeError(w, http.StatusBadRequest, "public_key must be a WireGuard public key")
		return
	}
	if len(req.Addresses) > 8 {
		writeError(w, http.StatusBadRequest, "too many addresses")
		return
	}
	for _, address := range req.Addresses {
		if ip := net.ParseIP(address); ip == nil || ip.To4() == nil {
			writeError(w, http.StatusBadRequest, "addresses must be IPv4 addresses")
			return
		}
	}
	if a.EnrollmentLimiter != nil && !a.EnrollmentLimiter.Allow(e, a.now()) {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "enrollment rate limit exceeded")
		return
	}
	source, _, _ := net.SplitHostPort(r.RemoteAddr)
	answer, err := a.Enrollments.Enroll(r.Context(), e, NetworkEnrollmentRequest{PublicKey: req.PublicKey, Addresses: req.Addresses, ObservedSource: source})
	if errors.Is(err, ErrNodeNotRecorded) {
		// The node's report makes or passes its record; the agent retries.
		writeError(w, http.StatusConflict, "pending: the node record is not of this Node yet")
		return
	}
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "network enrollment storage unavailable")
		return
	}
	writeJSON(w, http.StatusOK, answer)
}

// ErrNodeNotRecorded says the node's NetworkEnrollment is not there yet (its
// first report creates it), is of another Node UID, or is of a removed node.
var ErrNodeNotRecorded = errors.New("the node record is not of this Node")

// KubernetesNetworkEnroller writes the node's key and addresses into its
// NetworkEnrollment and returns the status station-controller wrote for
// exactly that request. Whose the record is, is not its to change.
type KubernetesNetworkEnroller struct {
	Client     dynamic.Interface
	Namespaces ObservationNamespaceResolver
}

func (s *KubernetesNetworkEnroller) Enroll(ctx context.Context, evidence Evidence, request NetworkEnrollmentRequest) (NetworkEnrollmentAnswer, error) {
	namespace, err := s.Namespaces.ResolveObservationNamespace(ctx, evidence.ClusterID)
	if err != nil {
		return NetworkEnrollmentAnswer{}, fmt.Errorf("resolve enrollment namespace: %w", err)
	}
	resource := s.Client.Resource(networkEnrollmentGVR).Namespace(namespace)
	current, err := resource.Get(ctx, evidence.NodeName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return NetworkEnrollmentAnswer{}, ErrNodeNotRecorded
	}
	if err != nil {
		return NetworkEnrollmentAnswer{}, fmt.Errorf("read NetworkEnrollment: %w", err)
	}
	spec, _, _ := unstructured.NestedMap(current.Object, "spec")
	if retired, _ := spec["retired"].(bool); retired || spec["nodeUID"] != evidence.NodeUID {
		return NetworkEnrollmentAnswer{}, ErrNodeNotRecorded
	}
	addresses := make([]interface{}, len(request.Addresses))
	for i, address := range request.Addresses {
		addresses[i] = address
	}
	pending := NetworkEnrollmentAnswer{State: "pending", Reason: "waiting for station-controller"}
	requested := map[string]interface{}{"publicKey": request.PublicKey, "observedSource": request.ObservedSource, "addresses": addresses}
	if enrollmentRequestText(spec) != enrollmentRequestText(requested) {
		// The resourceVersion makes a concurrent writer lose instead of
		// silently replacing a newer report.
		for field, value := range requested {
			spec[field] = value
		}
		current.Object["spec"] = spec
		if _, err := resource.Update(ctx, current, metav1.UpdateOptions{}); err != nil && !apierrors.IsConflict(err) {
			return NetworkEnrollmentAnswer{}, fmt.Errorf("update NetworkEnrollment: %w", err)
		}
		return pending, nil
	}
	status, _, _ := unstructured.NestedMap(current.Object, "status")
	observed, _, _ := unstructured.NestedInt64(status, "observedGeneration")
	if observed != current.GetGeneration() || status["publicKey"] != request.PublicKey && request.PublicKey != "" {
		return pending, nil
	}
	text := func(key string) string { value, _ := status[key].(string); return value }
	answer := NetworkEnrollmentAnswer{Reason: text("reason"), Address: text("address"), TunnelAddress: text("tunnelAddress")}
	switch text("phase") {
	case "Enrolled":
		answer.State = "enrolled"
		if config := text("config"); config != "" {
			if !json.Valid([]byte(config)) {
				return NetworkEnrollmentAnswer{}, errors.New("stored network configuration is not JSON")
			}
			answer.Config, answer.ConfigHash = json.RawMessage(config), text("configHash")
		}
	case "Rejected":
		answer.State = "rejected"
	default:
		answer.State = "pending"
	}
	return answer, nil
}

// enrollmentRequestText is what a request writes of a spec, whatever the API
// server did to empty fields.
func enrollmentRequestText(spec map[string]interface{}) string {
	key, _ := spec["publicKey"].(string)
	source, _ := spec["observedSource"].(string)
	addresses, _ := spec["addresses"].([]interface{})
	return key + "|" + source + "|" + fmt.Sprint(addresses...)
}
