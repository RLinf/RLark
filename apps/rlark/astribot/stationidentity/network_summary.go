package stationidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
)

// NodeNetworkSummary contains only the interface and addresses needed by a
// node listing. It does not expose Station records or peer details.
type NodeNetworkSummary struct {
	State      string     `json:"state"`
	Interface  string     `json:"interface,omitempty"`
	Addresses  []string   `json:"addresses,omitempty"`
	ReportedAt *time.Time `json:"reportedAt,omitempty"`
}

type currentNodeReader interface {
	CurrentNodeUID(context.Context, string) (string, error)
}

type NodeNetworkSummaryReader struct {
	Registry   ClusterRegistry
	Client     dynamic.Interface
	Namespaces ObservationNamespaceResolver
	Now        func() time.Time
}

// Read answers from the report of the Node that is in the cluster now: a
// record of another Node UID, or of a removed node, is no report of it.
func (s *NodeNetworkSummaryReader) Read(ctx context.Context, clusterID, nodeName string) (NodeNetworkSummary, error) {
	reader, err := s.Registry.Reader(ctx, clusterID)
	if err != nil {
		return NodeNetworkSummary{}, err
	}
	current, ok := reader.(currentNodeReader)
	if !ok {
		return NodeNetworkSummary{}, ErrIdentityUnavailable
	}
	nodeUID, err := current.CurrentNodeUID(ctx, nodeName)
	if err != nil {
		return NodeNetworkSummary{}, err
	}
	namespace, err := s.Namespaces.ResolveObservationNamespace(ctx, clusterID)
	if err != nil {
		return NodeNetworkSummary{}, err
	}
	enrollment, err := s.Client.Resource(networkEnrollmentGVR).Namespace(namespace).Get(ctx, nodeName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return NodeNetworkSummary{State: "unreported"}, nil
	}
	if err != nil {
		return NodeNetworkSummary{}, fmt.Errorf("read NetworkEnrollment: %w", err)
	}
	observation, _, _ := unstructured.NestedMap(enrollment.Object, "status", "observation")
	recorded, _, _ := unstructured.NestedString(enrollment.Object, "spec", "nodeUID")
	retired, _, _ := unstructured.NestedBool(enrollment.Object, "spec", "retired")
	if retired || recorded != nodeUID || observation["nodeUID"] != nodeUID {
		return NodeNetworkSummary{State: "unreported"}, nil
	}
	reported, _ := observation["reportedAt"].(string)
	reportedAt, err := time.Parse(time.RFC3339Nano, reported)
	if err != nil {
		return NodeNetworkSummary{}, errors.New("invalid observation receipt time")
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	if reportedAt.After(now.Add(5 * time.Second)) {
		return NodeNetworkSummary{}, errors.New("observation receipt time is in the future")
	}
	raw, found := observation["wireguard"].(map[string]interface{})
	if !found {
		return NodeNetworkSummary{State: "unreported", ReportedAt: &reportedAt}, nil
	}
	// The summary shows no peers; they are stored under the names the server
	// gave them, not as the node reported them.
	delete(raw, "peers")
	data, err := json.Marshal(raw)
	if err != nil || validateWireguardObservation(data, now) != nil {
		return NodeNetworkSummary{}, errors.New("invalid stored WireGuard observation")
	}
	var wg wireguardObservation
	if err := json.Unmarshal(data, &wg); err != nil {
		return NodeNetworkSummary{}, err
	}
	state := "current"
	if now.Sub(reportedAt) > 90*time.Second {
		state = "stale"
	}
	return NodeNetworkSummary{State: state, Interface: wg.Interface, Addresses: wg.Addresses, ReportedAt: &reportedAt}, nil
}
