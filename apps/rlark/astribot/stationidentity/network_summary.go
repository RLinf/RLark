package stationidentity

import (
	"context"
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
	// The summary shows no peers.
	iface, found, _ := unstructured.NestedString(observation, "wireguard", "interface")
	if !found {
		return NodeNetworkSummary{State: "unreported", ReportedAt: &reportedAt}, nil
	}
	addresses, _, _ := unstructured.NestedStringSlice(observation, "wireguard", "addresses")
	state := "current"
	if clock(s.Now).Sub(reportedAt) > 90*time.Second {
		state = "stale"
	}
	return NodeNetworkSummary{State: state, Interface: iface, Addresses: addresses, ReportedAt: &reportedAt}, nil
}
