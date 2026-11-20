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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const (
	stationGroup         = "station.rlark.io"
	stationVersion       = "v1alpha1"
	rlarkClusterResource = "clusters"
)

var rlarkClusterGVR = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: rlarkClusterResource}

// ObservationNamespaceResolver maps a trusted RLark cluster ID to its
// management-plane agentNamespace. Implementations must reject ambiguous scope.
type ObservationNamespaceResolver interface {
	ResolveObservationNamespace(context.Context, string) (string, error)
}

// KubernetesClusterNamespaceResolver reads the current RLark Cluster objects
// on every call; labels are not used to establish membership.
type KubernetesClusterNamespaceResolver struct{ Client dynamic.Interface }

func (r KubernetesClusterNamespaceResolver) ResolveObservationNamespace(ctx context.Context, clusterID string) (string, error) {
	if clusterID == "" {
		return "", ErrClusterNotRegistered
	}
	clusters, err := r.Client.Resource(rlarkClusterGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("list registered RLark Clusters: %w", err)
	}
	var namespace string
	for i := range clusters.Items {
		cluster := &clusters.Items[i]
		agentNamespace, found, err := unstructured.NestedString(cluster.Object, "spec", "agentNamespace")
		if err != nil {
			return "", fmt.Errorf("read Cluster %q agentNamespace: %w", cluster.GetName(), err)
		}
		if cluster.GetName() != clusterID || !found || agentNamespace == "" {
			continue
		}
		if namespace != "" {
			return "", fmt.Errorf("cluster ID %q is ambiguous", clusterID)
		}
		namespace = agentNamespace
	}
	if namespace == "" {
		return "", ErrClusterNotRegistered
	}
	for i := range clusters.Items {
		other := &clusters.Items[i]
		if other.GetName() == clusterID {
			continue
		}
		otherNamespace, found, err := unstructured.NestedString(other.Object, "spec", "agentNamespace")
		if err != nil {
			return "", fmt.Errorf("read Cluster %q agentNamespace: %w", other.GetName(), err)
		}
		if found && otherNamespace == namespace {
			return "", fmt.Errorf("multiple RLark Clusters claim agent namespace %q", namespace)
		}
	}
	return namespace, nil
}

// KubernetesObservationSink stores the latest validated report of a node in
// status.observation of its NetworkEnrollment, which the node's first report
// creates. It writes nothing else of the status: the rest is station-controller's.
type KubernetesObservationSink struct {
	Client     dynamic.Interface
	Namespaces ObservationNamespaceResolver
	Now        func() time.Time

	// Enrolled is the enrollment each public key is enrolled with; Admitted the admission a machine that has not
	// enrolled yet redeemed with it; Declared the node the SiteNetwork declares with it.
	Enrolled *KeyIndex
	Admitted *KeyIndex
	Declared *DeclaredKeys
}

func (s *KubernetesObservationSink) Store(ctx context.Context, evidence Evidence, observation Observation) error {
	namespace, err := s.Namespaces.ResolveObservationNamespace(ctx, evidence.ClusterID)
	if err != nil {
		return fmt.Errorf("resolve observation namespace: %w", err)
	}
	if namespace == "" || evidence.NodeName == "" || evidence.NodeUID == "" {
		return errors.New("verified observation has incomplete namespace or identity")
	}
	// A node tells its peers by public key. What is stored is the member each
	// key is enrolled as, or the node a redeemed admission or the SiteNetwork
	// gives it to before it enrolls (that a peer reports such a key says its
	// tunnel has taken the new node's document); another key is not stored.
	var wireguard wireguardObservation
	if len(observation.Wireguard) > 0 {
		if err := json.Unmarshal(observation.Wireguard, &wireguard); err != nil {
			return errors.New("wireguard observation must be a JSON object")
		}
	}
	members := map[string]string{}
	if len(wireguard.Peers) > 0 {
		keys := make([]string, len(wireguard.Peers))
		for i, peer := range wireguard.Peers {
			keys[i] = peer.PublicKey
		}
		enrolled, err := s.Enrolled.Lookup(ctx, keys...)
		if err != nil {
			return fmt.Errorf("list NetworkEnrollments: %w", err)
		}
		admitted, err := s.Admitted.Lookup(ctx, keys...)
		if err != nil {
			return fmt.Errorf("list NodeAdmissions: %w", err)
		}
		declared, err := s.Declared.Lookup(ctx, keys...)
		if err != nil {
			return fmt.Errorf("read the SiteNetwork: %w", err)
		}
		for _, key := range keys {
			switch {
			case enrolled[key] != nil:
				members[key] = memberName(enrolled[key])
			case admitted[key] != nil:
				members[key] = memberName(admitted[key])
			case declared[key] != "":
				members[key] = declared[key]
			}
		}
	}
	status, err := observationStatus(evidence, observation, members, s.now())
	if err != nil {
		return fmt.Errorf("encode observation: %w", err)
	}
	patch, err := json.Marshal(map[string]interface{}{"status": map[string]interface{}{"observation": status}})
	if err != nil {
		return fmt.Errorf("encode the observation patch: %w", err)
	}
	resource := s.Client.Resource(networkEnrollmentGVR).Namespace(namespace)
	_, err = resource.Patch(ctx, evidence.NodeName, types.MergePatchType, patch, metav1.PatchOptions{}, "status")
	if apierrors.IsNotFound(err) {
		// The node's first report: the record is of the Node that was verified.
		_, err = resource.Create(ctx, &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": stationGroup + "/" + stationVersion, "kind": "NetworkEnrollment",
			"metadata": map[string]interface{}{"name": evidence.NodeName, "namespace": namespace},
			"spec":     map[string]interface{}{"clusterID": evidence.ClusterID, "nodeUID": evidence.NodeUID, "systemUUID": observation.SystemUUID},
		}}, metav1.CreateOptions{})
		if err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create NetworkEnrollment: %w", err)
		}
		_, err = resource.Patch(ctx, evidence.NodeName, types.MergePatchType, patch, metav1.PatchOptions{}, "status")
	}
	if err != nil {
		return fmt.Errorf("patch the observation of NetworkEnrollment: %w", err)
	}
	return nil
}

func (s *KubernetesObservationSink) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func observationStatus(evidence Evidence, observation Observation, members map[string]string, receivedAt time.Time) (map[string]interface{}, error) {
	// Written as a list even when empty: that is what tells a Node under no
	// pressure from a report stored before pressures were.
	pressures := make([]interface{}, 0, len(evidence.NodePressures))
	for _, pressure := range evidence.NodePressures {
		pressures = append(pressures, pressure)
	}
	status := map[string]interface{}{
		"reportedAt": receivedAt.UTC().Format(time.RFC3339Nano),
		"nodeUID":    evidence.NodeUID,
		"systemUUID": observation.SystemUUID,
		"bootTime":   nil,
		"wireguard":  nil,
		"roles":      []interface{}{},
		"probes":     []interface{}{},
		"gateway":    nil,
		"agent":      map[string]interface{}{"digest": observation.Agent.Digest},
		"pressures":  pressures,
	}
	if observation.Gateway != nil {
		status["gateway"] = map[string]interface{}{"documentLevel": int64(observation.Gateway.DocumentLevel), "digest": observation.Gateway.Digest}
	}
	if observation.BootTime != nil {
		status["bootTime"] = observation.BootTime.UTC().Format(time.RFC3339Nano)
	}
	if len(observation.Roles) > 0 {
		roles := make([]interface{}, len(observation.Roles))
		for i, role := range observation.Roles {
			roles[i] = role
		}
		status["roles"] = roles
	}
	if len(observation.Probes) > 0 {
		probes, err := toUnstructuredValue(observation.Probes)
		if err != nil {
			return nil, err
		}
		status["probes"] = probes
	}
	if len(observation.Wireguard) > 0 {
		var wireguard map[string]interface{}
		if err := json.Unmarshal(observation.Wireguard, &wireguard); err != nil || wireguard == nil {
			return nil, errors.New("wireguard observation must be a JSON object")
		}
		// The list is always written: a merge patch keeps what it does not name.
		reported, _ := wireguard["peers"].([]interface{})
		peers := make([]interface{}, 0, len(reported))
		for _, entry := range reported {
			peer := entry.(map[string]interface{})
			if name := members[peer["publicKey"].(string)]; name != "" {
				delete(peer, "publicKey")
				peer["name"] = name
				peers = append(peers, peer)
			}
		}
		wireguard["peers"] = peers
		status["wireguard"] = wireguard
	}
	return status, nil
}

func toUnstructuredValue(value any) (interface{}, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var converted interface{}
	if err := json.Unmarshal(data, &converted); err != nil {
		return nil, err
	}
	return converted, nil
}
