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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const (
	stationObservationGroup      = "station.rlark.io"
	stationObservationVersion    = "v1alpha1"
	stationObservationResource   = "stationobservations"
	stationResource              = "stations"
	rlarkClusterResource         = "clusters"
	observationClusterIDKey      = "station.rlark.io/cluster-id"
	observationNodeUIDKey        = "station.rlark.io/node-uid"
	observationStationUIDKey     = "station.rlark.io/station-uid"
	observationBindingVersionKey = "station.rlark.io/binding-version"
	observationRetries           = 6
)

var (
	stationObservationGVR           = schema.GroupVersionResource{Group: stationObservationGroup, Version: stationObservationVersion, Resource: stationObservationResource}
	stationGVR                      = schema.GroupVersionResource{Group: stationObservationGroup, Version: stationObservationVersion, Resource: stationResource}
	rlarkClusterGVR                 = schema.GroupVersionResource{Group: "rlinf.io", Version: "v1alpha1", Resource: rlarkClusterResource}
	ErrObservationOwnershipConflict = errors.New("StationObservation ownership conflict")
)

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

// KubernetesObservationSink stores only the latest validated snapshot. It
// uses the uncached dynamic API client and writes through the status subresource.
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
		return fmt.Errorf("resolve Station observation namespace: %w", err)
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
		return fmt.Errorf("encode Station observation status: %w", err)
	}
	resource := s.Client.Resource(stationObservationGVR).Namespace(namespace)
	for attempt := 0; attempt < observationRetries; attempt++ {
		current, getErr := resource.Get(ctx, evidence.NodeName, metav1.GetOptions{})
		if apierrors.IsNotFound(getErr) {
			annotations := map[string]interface{}{
				observationClusterIDKey: evidence.ClusterID,
				observationNodeUIDKey:   evidence.NodeUID,
			}
			if evidence.StationUID != "" {
				annotations[observationStationUIDKey] = evidence.StationUID
				annotations[observationBindingVersionKey] = strconv.FormatInt(evidence.Generation, 10)
			}
			candidate := &unstructured.Unstructured{Object: map[string]interface{}{
				"apiVersion": stationObservationGroup + "/" + stationObservationVersion,
				"kind":       "StationObservation",
				"metadata": map[string]interface{}{
					"name": evidence.NodeName, "namespace": namespace,
					"annotations": annotations,
				},
				"spec": map[string]interface{}{},
			}}
			if _, createErr := resource.Create(ctx, candidate, metav1.CreateOptions{}); apierrors.IsAlreadyExists(createErr) || apierrors.IsConflict(createErr) {
				continue
			} else if createErr != nil {
				return fmt.Errorf("create StationObservation: %w", createErr)
			}
			continue
		}
		if getErr != nil {
			return fmt.Errorf("read StationObservation: %w", getErr)
		}
		annotations := current.GetAnnotations()
		if annotations[observationClusterIDKey] == evidence.ClusterID && annotations[observationNodeUIDKey] != evidence.NodeUID && evidence.StationUID == "" {
			// The member API server has just vouched for this Node under this
			// name and it is bound to no Station, so the Node the stored
			// snapshot belongs to is gone. Its snapshot goes with it; nothing
			// of it is carried over. This is what lets a machine that replaced
			// a node be observed at all: its Station then shows Conflict and
			// can be rebound. A bound identity with another Node UID is not
			// this case: it is a verified rebind or a conflict, decided below.
			uid := current.GetUID()
			if err := resource.Delete(ctx, evidence.NodeName, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) && !apierrors.IsConflict(err) {
				return fmt.Errorf("replace StationObservation of a previous Node: %w", err)
			}
			continue
		}
		storedStationUID := annotations[observationStationUIDKey]
		storedNodeUID := annotations[observationNodeUIDKey]
		storedBindingVersion, versionErr := strconv.ParseInt(annotations[observationBindingVersionKey], 10, 64)
		reboundIdentity := evidence.StationUID != "" && storedStationUID == evidence.StationUID && storedNodeUID != evidence.NodeUID && versionErr == nil && storedBindingVersion > 0 && evidence.Generation > storedBindingVersion
		if annotations[observationClusterIDKey] != evidence.ClusterID || storedNodeUID != evidence.NodeUID && !reboundIdentity || storedStationUID != "" && storedStationUID != evidence.StationUID || evidence.StationUID == "" && storedStationUID != "" {
			return ErrObservationOwnershipConflict
		}
		metadataPatch := map[string]interface{}{}
		if reboundIdentity {
			oldStatus, found, nestedErr := unstructured.NestedMap(current.Object, "status")
			if nestedErr != nil {
				return fmt.Errorf("read previous observation status: %w", nestedErr)
			}
			hasOldSnapshot := false
			if found {
				for _, field := range []string{"systemUUID", "bootTime", "roles", "probes", "wireguard", "gateway", "pressures"} {
					if _, exists := oldStatus[field]; exists {
						hasOldSnapshot = true
						break
					}
				}
			}
			if hasOldSnapshot {
				clearStatus, marshalErr := json.Marshal(map[string]interface{}{
					"metadata": map[string]interface{}{"resourceVersion": current.GetResourceVersion()},
					// nodeUID, reportedAt and agent are required by the CRD schema. Keep
					// their old values until the fresh snapshot replaces them.
					"status": map[string]interface{}{"systemUUID": nil, "bootTime": nil, "roles": nil, "probes": nil, "wireguard": nil, "gateway": nil, "pressures": nil},
				})
				if marshalErr != nil {
					return marshalErr
				}
				if _, patchErr := resource.Patch(ctx, evidence.NodeName, types.MergePatchType, clearStatus, metav1.PatchOptions{}, "status"); apierrors.IsConflict(patchErr) {
					continue
				} else if patchErr != nil {
					return fmt.Errorf("clear previous Node identity observation: %w", patchErr)
				}
				continue
			}
			metadataPatch[observationNodeUIDKey] = evidence.NodeUID
		}
		if evidence.StationUID != "" && storedStationUID == "" {
			metadataPatch[observationStationUIDKey] = evidence.StationUID
		}
		if evidence.StationUID != "" && annotations[observationBindingVersionKey] != strconv.FormatInt(evidence.Generation, 10) {
			// Do not make an older snapshot appear to belong to the new binding
			// while the metadata and status subresources are updated separately.
			if _, found, nestedErr := unstructured.NestedFieldNoCopy(current.Object, "status", "wireguard"); nestedErr != nil {
				return fmt.Errorf("read previous WireGuard observation: %w", nestedErr)
			} else if found {
				clear, marshalErr := json.Marshal(map[string]interface{}{"metadata": map[string]interface{}{"resourceVersion": current.GetResourceVersion()}, "status": map[string]interface{}{"wireguard": nil}})
				if marshalErr != nil {
					return marshalErr
				}
				if _, patchErr := resource.Patch(ctx, evidence.NodeName, types.MergePatchType, clear, metav1.PatchOptions{}, "status"); apierrors.IsConflict(patchErr) {
					continue
				} else if patchErr != nil {
					return fmt.Errorf("clear previous binding observation: %w", patchErr)
				}
				continue
			}
			metadataPatch[observationBindingVersionKey] = strconv.FormatInt(evidence.Generation, 10)
		}
		if len(metadataPatch) > 0 {
			metadataBody, marshalErr := json.Marshal(map[string]interface{}{
				"metadata": map[string]interface{}{
					"resourceVersion": current.GetResourceVersion(),
					"annotations":     metadataPatch,
				},
			})
			if marshalErr != nil {
				return fmt.Errorf("encode StationObservation ownership patch: %w", marshalErr)
			}
			if _, patchErr := resource.Patch(ctx, evidence.NodeName, types.MergePatchType, metadataBody, metav1.PatchOptions{}); apierrors.IsConflict(patchErr) {
				continue
			} else if patchErr != nil {
				return fmt.Errorf("patch StationObservation ownership metadata: %w", patchErr)
			}
			continue
		}
		patch, err := json.Marshal(map[string]interface{}{
			"metadata": map[string]interface{}{"resourceVersion": current.GetResourceVersion()},
			"status":   status,
		})
		if err != nil {
			return fmt.Errorf("encode StationObservation patch: %w", err)
		}
		_, err = resource.Patch(ctx, evidence.NodeName, types.MergePatchType, patch, metav1.PatchOptions{}, "status")
		if apierrors.IsConflict(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("patch StationObservation status: %w", err)
		}
		return nil
	}
	return errors.New("StationObservation remained contended after bounded retries")
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
