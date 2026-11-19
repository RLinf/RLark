package stationidentity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
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

type currentNodeBindingReader interface {
	CurrentNodeBinding(context.Context, string) (Binding, error)
}

type NodeNetworkSummaryReader struct {
	Registry   ClusterRegistry
	Client     dynamic.Interface
	Namespaces ObservationNamespaceResolver
	Now        func() time.Time
}

// Read fails closed when any current identity or ownership evidence is absent.
func (s *NodeNetworkSummaryReader) Read(ctx context.Context, clusterID, nodeName string) (NodeNetworkSummary, error) {
	reader, err := s.Registry.Reader(ctx, clusterID)
	if err != nil {
		return NodeNetworkSummary{}, err
	}
	current, ok := reader.(currentNodeBindingReader)
	if !ok {
		return NodeNetworkSummary{}, ErrIdentityUnavailable
	}
	binding, err := current.CurrentNodeBinding(ctx, nodeName)
	if errors.Is(err, ErrStationBindingNotFound) {
		return NodeNetworkSummary{State: "unreported"}, nil
	}
	if err != nil {
		return NodeNetworkSummary{}, err
	}
	namespace, err := s.Namespaces.ResolveObservationNamespace(ctx, clusterID)
	if err != nil {
		return NodeNetworkSummary{}, err
	}
	observation, err := s.Client.Resource(stationObservationGVR).Namespace(namespace).Get(ctx, nodeName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return NodeNetworkSummary{State: "unreported"}, nil
	}
	if err != nil {
		return NodeNetworkSummary{}, fmt.Errorf("read Station observation: %w", err)
	}
	annotations := observation.GetAnnotations()
	uid, _, uidErr := unstructured.NestedString(observation.Object, "status", "nodeUID")
	if uidErr != nil || uid != binding.NodeUID || annotations[observationClusterIDKey] != clusterID || annotations[observationNodeUIDKey] != binding.NodeUID || annotations[observationStationUIDKey] != binding.StationUID || annotations[observationBindingVersionKey] != strconv.FormatInt(binding.Generation, 10) {
		return NodeNetworkSummary{}, ErrObservationOwnershipConflict
	}
	reported, found, err := unstructured.NestedString(observation.Object, "status", "reportedAt")
	if err != nil || !found {
		return NodeNetworkSummary{}, errors.New("observation has no receipt time")
	}
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
	raw, found, err := unstructured.NestedMap(observation.Object, "status", "wireguard")
	if err != nil {
		return NodeNetworkSummary{}, err
	}
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
