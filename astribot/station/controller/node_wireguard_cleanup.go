package controller

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	stationv1alpha1 "github.com/rlinf/rlark/astribot/station/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stationWireGuardNamespace = "station-system"
	stationWireGuardConfigMap = "station-wireguard-config"
	stationNetworkLabel       = "station.rlark.io/network"
)

var ErrWireGuardPeerOwnershipUnknown = errors.New("WireGuard peer ownership could not be verified")

// NodeWireGuardPeerCleaner validates ownership before drain and removes only
// peer/config entries tied to the exact member Node UID.
type NodeWireGuardPeerCleaner interface {
	Check(context.Context, *stationv1alpha1.NodeOperation) error
	Remove(context.Context, *stationv1alpha1.NodeOperation) (bool, error)
}

type NodeOperationClusterLister interface {
	ClusterIDs(context.Context) ([]string, error)
}

type NodeWireGuardEvidence struct {
	PublicKey string
	Addresses []string
}

type NodeWireGuardEvidenceSource interface {
	Evidence(context.Context, string, string, string) (NodeWireGuardEvidence, error)
}

type ManagementNodeWireGuardEvidence struct {
	Reader client.Reader
	Now    func() time.Time
}

func (s ManagementNodeWireGuardEvidence) Evidence(ctx context.Context, namespace, nodeName, nodeUID string) (NodeWireGuardEvidence, error) {
	observation := &stationv1alpha1.StationObservation{}
	if err := s.Reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: nodeName}, observation); apierrors.IsNotFound(err) {
		return NodeWireGuardEvidence{}, nil
	} else if err != nil {
		return NodeWireGuardEvidence{}, fmt.Errorf("read Station observation for WireGuard ownership: %w", err)
	}
	if observation.Status.NodeUID != nodeUID || observation.Status.Wireguard == nil || !validWireGuardPublicKey(observation.Status.Wireguard.PublicKey) {
		return NodeWireGuardEvidence{}, nil
	}
	now := time.Now()
	if s.Now != nil {
		now = s.Now()
	}
	reported := observation.Status.ReportedAt.Time
	if reported.IsZero() || reported.After(now.Add(5*time.Second)) || now.Sub(reported) > 90*time.Second {
		return NodeWireGuardEvidence{}, nil
	}
	return NodeWireGuardEvidence{PublicKey: observation.Status.Wireguard.PublicKey, Addresses: append([]string(nil), observation.Status.Wireguard.Addresses...)}, nil
}

type ConfigMapNodeWireGuardPeerCleaner struct {
	Members      NodeMemberClientSource
	Clusters     NodeOperationClusterLister
	Observations NodeWireGuardEvidenceSource
}

func (c ConfigMapNodeWireGuardPeerCleaner) Check(ctx context.Context, op *stationv1alpha1.NodeOperation) error {
	// A name its cluster has no Node of has no configuration of a Node to prove or remove.
	if op.Spec.NodeUID == "" {
		return nil
	}
	_, node, err := c.readNode(ctx, op)
	if err != nil {
		return err
	}
	evidence, err := c.evidence(ctx, op)
	if err != nil {
		return err
	}
	configs, err := c.readAllConfigMaps(ctx, op)
	if err != nil {
		return err
	}
	found := false
	targetConfigFound := false
	for _, config := range configs {
		if config.clusterID == op.Spec.ClusterID && config.object.Data[op.Spec.NodeName+".json"] != "" {
			targetConfigFound = true
		}
		matched, err := inspectNodeWireGuardConfigForCluster(config.object, op.Spec.NodeName, op.Spec.NodeUID, evidence, config.clusterID == op.Spec.ClusterID)
		if err != nil {
			return err
		}
		if matched {
			found = true
		}
	}
	if node.Labels[stationNetworkLabel] == "enabled" && (!targetConfigFound || !found || evidence.PublicKey == "" && !ownedNodeConfigFound(configs, op.Spec.NodeName, op.Spec.NodeUID)) {
		return ErrWireGuardPeerOwnershipUnknown
	}
	return nil
}

func (c ConfigMapNodeWireGuardPeerCleaner) Remove(ctx context.Context, op *stationv1alpha1.NodeOperation) (bool, error) {
	if op.Spec.NodeUID == "" {
		return true, nil
	}
	member, node, err := c.readNode(ctx, op)
	if err != nil {
		return false, err
	}
	evidence, err := c.evidence(ctx, op)
	if err != nil {
		return false, err
	}
	configs, err := c.readAllConfigMaps(ctx, op)
	if err != nil {
		return false, err
	}
	found := false
	for _, config := range configs {
		changed, matched, err := removeNodeWireGuardConfigForCluster(config.object, op.Spec.NodeName, op.Spec.NodeUID, evidence, config.clusterID == op.Spec.ClusterID)
		if err != nil {
			return false, err
		}
		found = found || matched
		if changed {
			if _, err := config.client.CoreV1().ConfigMaps(config.object.Namespace).Update(ctx, config.object, metav1.UpdateOptions{}); err != nil {
				return false, fmt.Errorf("update WireGuard configuration in cluster %q: %w", config.clusterID, err)
			}
		}
	}
	if node.Labels[stationNetworkLabel] == "enabled" && !found && !ownedNodeConfigFound(configs, op.Spec.NodeName, op.Spec.NodeUID) {
		return false, ErrWireGuardPeerOwnershipUnknown
	}
	remaining, err := c.readAllConfigMaps(ctx, op)
	if err != nil {
		return false, err
	}
	for _, config := range remaining {
		remains, err := inspectNodeWireGuardConfigForCluster(config.object, op.Spec.NodeName, op.Spec.NodeUID, evidence, config.clusterID == op.Spec.ClusterID)
		if err != nil {
			return false, err
		}
		if remains || config.clusterID == op.Spec.ClusterID && config.object.Data[op.Spec.NodeName+".json"] != "" {
			return false, errors.New("WireGuard peer cleanup is not yet observed across all registered clusters")
		}
	}
	if err := c.disableNodeNetwork(ctx, member, op); err != nil {
		return false, err
	}
	pods, err := member.CoreV1().Pods(stationWireGuardNamespace).List(ctx, metav1.ListOptions{FieldSelector: "spec.nodeName=" + op.Spec.NodeName, LabelSelector: "app=station-agent-network"})
	if err != nil {
		return false, fmt.Errorf("verify WireGuard DaemonSet pod stopped: %w", err)
	}
	if len(pods.Items) > 0 {
		// No kubelet stops the pod on a node that no longer reports.
		if !nodeReady(node) {
			now := int64(0)
			for i := range pods.Items {
				pod := &pods.Items[i]
				if err := member.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{GracePeriodSeconds: &now, Preconditions: &metav1.Preconditions{UID: &pod.UID}}); err != nil && !apierrors.IsNotFound(err) {
					return false, fmt.Errorf("delete WireGuard DaemonSet pod %s: %w", pod.Name, err)
				}
			}
		}
		return false, nil
	}
	return true, nil
}

func (c ConfigMapNodeWireGuardPeerCleaner) disableNodeNetwork(ctx context.Context, member kubernetes.Interface, op *stationv1alpha1.NodeOperation) error {
	nodes := member.CoreV1().Nodes()
	node, err := nodes.Get(ctx, op.Spec.NodeName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return ErrNodeIdentityChanged
	}
	if err != nil {
		return fmt.Errorf("read Node before disabling WireGuard DaemonSet: %w", err)
	}
	if string(node.UID) != op.Spec.NodeUID || node.DeletionTimestamp != nil {
		return ErrNodeIdentityChanged
	}
	// Only an enabled node runs the network DaemonSet: a node whose tunnel is
	// a host service (station.rlark.io/network=host, the installer's) has none.
	if node.Labels[stationNetworkLabel] != "enabled" {
		return nil
	}
	updated := node.DeepCopy()
	delete(updated.Labels, stationNetworkLabel)
	if _, err := nodes.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("disable WireGuard DaemonSet on Node: %w", err)
	}
	return nil
}

func (c ConfigMapNodeWireGuardPeerCleaner) evidence(ctx context.Context, op *stationv1alpha1.NodeOperation) (NodeWireGuardEvidence, error) {
	if c.Observations == nil {
		return NodeWireGuardEvidence{}, nil
	}
	return c.Observations.Evidence(ctx, op.Namespace, op.Spec.NodeName, op.Spec.NodeUID)
}

func (c ConfigMapNodeWireGuardPeerCleaner) readNode(ctx context.Context, op *stationv1alpha1.NodeOperation) (kubernetes.Interface, *corev1.Node, error) {
	member, err := c.Members.Client(ctx, op.Spec.ClusterID)
	if err != nil {
		return nil, nil, fmt.Errorf("connect member cluster: %w", err)
	}
	node, err := member.CoreV1().Nodes().Get(ctx, op.Spec.NodeName, metav1.GetOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("read member node for peer cleanup: %w", err)
	}
	if string(node.UID) != op.Spec.NodeUID || node.DeletionTimestamp != nil {
		return nil, nil, ErrNodeIdentityChanged
	}
	return member, node, nil
}

type nodeWireGuardConfigMap struct {
	clusterID string
	client    kubernetes.Interface
	object    *corev1.ConfigMap
}

func (c ConfigMapNodeWireGuardPeerCleaner) readAllConfigMaps(ctx context.Context, op *stationv1alpha1.NodeOperation) ([]nodeWireGuardConfigMap, error) {
	clusterIDs, err := c.Clusters.ClusterIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list registered clusters for WireGuard cleanup: %w", err)
	}
	foundTarget := false
	configs := make([]nodeWireGuardConfigMap, 0, len(clusterIDs))
	for _, clusterID := range clusterIDs {
		member, err := c.Members.Client(ctx, clusterID)
		if err != nil {
			return nil, fmt.Errorf("connect member cluster %q for WireGuard cleanup: %w", clusterID, err)
		}
		foundTarget = foundTarget || clusterID == op.Spec.ClusterID
		configMap, err := member.CoreV1().ConfigMaps(stationWireGuardNamespace).Get(ctx, stationWireGuardConfigMap, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read WireGuard config in cluster %q: %w", clusterID, err)
		}
		configs = append(configs, nodeWireGuardConfigMap{clusterID: clusterID, client: member, object: configMap})
	}
	if !foundTarget {
		return nil, errors.New("target cluster is absent from the registered Cluster inventory")
	}
	return configs, nil
}

func ownedNodeConfigFound(configs []nodeWireGuardConfigMap, nodeName, nodeUID string) bool {
	for _, configMap := range configs {
		doc, found, err := parseOwnedNodeDocument(configMap.object, nodeName, nodeUID, NodeWireGuardEvidence{})
		if err == nil && found && doc.ownerUID == nodeUID {
			return true
		}
	}
	return false
}

type nodeWireGuardDocument struct {
	ownerUID  string
	publicKey string
	address   string
	peers     []map[string]json.RawMessage
	routes    []string
}

func inspectNodeWireGuardConfigForCluster(cm *corev1.ConfigMap, nodeName, nodeUID string, evidence NodeWireGuardEvidence, includeNodeConfig bool) (bool, error) {
	var ownedDoc nodeWireGuardDocument
	var found bool
	var err error
	if includeNodeConfig {
		ownedDoc, found, err = parseOwnedNodeDocument(cm, nodeName, nodeUID, evidence)
	}
	if err != nil {
		return found, err
	}
	evidence = mergeNodeWireGuardEvidence(evidence, ownedDoc)
	for key, data := range cm.Data {
		doc, err := parseNodeWireGuardDocument(data)
		if err != nil {
			return false, fmt.Errorf("parse WireGuard config %q: %w", key, err)
		}
		for _, peer := range doc.peers {
			if peer == nil {
				return false, fmt.Errorf("peer entry in %q is not an object", key)
			}
			owner, err := rawString(peer["nodeUID"])
			if err != nil {
				return false, fmt.Errorf("read peer Node UID in %q: %w", key, err)
			}
			publicKey, err := rawString(peer["publicKey"])
			if err != nil {
				return false, fmt.Errorf("read peer public key in %q: %w", key, err)
			}
			if owner == nodeUID || peerMatchesEvidence(peer, publicKey, evidence) {
				if owner != "" && owner != nodeUID {
					return false, fmt.Errorf("%w: peer identity conflicts with the target Node UID", ErrWireGuardPeerOwnershipUnknown)
				}
				return true, nil
			}
		}
	}
	return found, nil
}

func parseOwnedNodeDocument(cm *corev1.ConfigMap, nodeName, nodeUID string, evidence NodeWireGuardEvidence) (nodeWireGuardDocument, bool, error) {
	key := nodeName + ".json"
	raw, exists := cm.Data[key]
	if !exists {
		return nodeWireGuardDocument{}, false, nil
	}
	doc, err := parseNodeWireGuardDocument(raw)
	if err != nil {
		return nodeWireGuardDocument{}, false, fmt.Errorf("parse node WireGuard config %q: %w", key, err)
	}
	if doc.ownerUID != "" && doc.ownerUID != nodeUID {
		return nodeWireGuardDocument{}, false, fmt.Errorf("%w: node config UID differs", ErrWireGuardPeerOwnershipUnknown)
	}
	if doc.publicKey != "" {
		if !validWireGuardPublicKey(doc.publicKey) {
			return nodeWireGuardDocument{}, false, errors.New("node WireGuard config has an invalid public key")
		}
	}
	if doc.address != "" {
		if _, _, err := net.ParseCIDR(doc.address); err != nil {
			return nodeWireGuardDocument{}, false, errors.New("node WireGuard config has an invalid address")
		}
	}
	if doc.ownerUID == "" {
		publicKeyMatches := doc.publicKey == "" || doc.publicKey == evidence.PublicKey
		if evidence.PublicKey == "" || !publicKeyMatches || !slices.Contains(evidence.Addresses, doc.address) {
			return nodeWireGuardDocument{}, false, ErrWireGuardPeerOwnershipUnknown
		}
	}
	return doc, true, nil
}

func parseNodeWireGuardDocument(data string) (nodeWireGuardDocument, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &object); err != nil || object == nil {
		return nodeWireGuardDocument{}, errors.New("configuration is not a JSON object")
	}
	owner, err := rawString(object["nodeUID"])
	if err != nil {
		return nodeWireGuardDocument{}, errors.New("nodeUID is invalid")
	}
	publicKey, err := rawString(object["publicKey"])
	if err != nil {
		return nodeWireGuardDocument{}, errors.New("publicKey is invalid")
	}
	address, err := rawString(object["address"])
	if err != nil {
		return nodeWireGuardDocument{}, errors.New("address is invalid")
	}
	var peers []map[string]json.RawMessage
	if raw := object["peers"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &peers); err != nil {
			return nodeWireGuardDocument{}, errors.New("peers is invalid")
		}
	}
	var routes []string
	if raw := object["routes"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &routes); err != nil {
			return nodeWireGuardDocument{}, errors.New("routes is invalid")
		}
	}
	return nodeWireGuardDocument{ownerUID: owner, publicKey: publicKey, address: address, peers: peers, routes: routes}, nil
}

func removeNodeWireGuardConfigForCluster(cm *corev1.ConfigMap, nodeName, nodeUID string, evidence NodeWireGuardEvidence, includeNodeConfig bool) (bool, bool, error) {
	var doc nodeWireGuardDocument
	var ownsNodeConfig bool
	var err error
	if includeNodeConfig {
		doc, ownsNodeConfig, err = parseOwnedNodeDocument(cm, nodeName, nodeUID, evidence)
	}
	if err != nil {
		return false, false, err
	}
	evidence = mergeNodeWireGuardEvidence(evidence, doc)
	found := ownsNodeConfig
	for key, raw := range cm.Data {
		parsed, err := parseNodeWireGuardDocument(raw)
		if err != nil {
			return false, found, fmt.Errorf("parse WireGuard config %q: %w", key, err)
		}
		remove := make([]bool, len(parsed.peers))
		removedIPs := map[string]struct{}{}
		remainingIPs := map[string]struct{}{}
		for i, peer := range parsed.peers {
			if peer == nil {
				return false, found, fmt.Errorf("peer entry in %q is not an object", key)
			}
			owner, err := rawString(peer["nodeUID"])
			if err != nil {
				return false, found, fmt.Errorf("read peer Node UID in %q: %w", key, err)
			}
			publicKey, err := rawString(peer["publicKey"])
			if err != nil {
				return false, found, fmt.Errorf("read peer public key in %q: %w", key, err)
			}
			var allowed []string
			if rawAllowed := peer["allowedIPs"]; len(rawAllowed) > 0 {
				if err := json.Unmarshal(rawAllowed, &allowed); err != nil {
					return false, found, fmt.Errorf("read allowedIPs in %q: %w", key, err)
				}
			}
			addressMatch := false
			for _, ip := range allowed {
				if slices.Contains(evidence.Addresses, ip) {
					addressMatch = true
				}
			}
			match := owner == nodeUID || addressMatch || (evidence.PublicKey != "" && publicKey == evidence.PublicKey)
			if match && owner != "" && owner != nodeUID {
				return false, found, fmt.Errorf("%w: peer identity conflicts with the target Node UID", ErrWireGuardPeerOwnershipUnknown)
			}
			if match {
				found = true
				remove[i] = true
				for _, ip := range allowed {
					removedIPs[ip] = struct{}{}
				}
			} else {
				for _, ip := range allowed {
					remainingIPs[ip] = struct{}{}
				}
			}
		}
		if !slices.Contains(remove, true) {
			continue
		}
		kept := make([]map[string]json.RawMessage, 0, len(parsed.peers))
		for i, peer := range parsed.peers {
			if !remove[i] {
				kept = append(kept, peer)
			}
		}
		encodedPeers, err := json.Marshal(kept)
		if err != nil {
			return false, found, fmt.Errorf("encode peers in %q: %w", key, err)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &object); err != nil {
			return false, found, fmt.Errorf("re-read WireGuard config %q: %w", key, err)
		}
		object["peers"] = encodedPeers
		remainingRoutes := make([]string, 0, len(parsed.routes))
		for _, route := range parsed.routes {
			_, removed := removedIPs[route]
			_, stillUsed := remainingIPs[route]
			if !removed || stillUsed {
				remainingRoutes = append(remainingRoutes, route)
			}
		}
		encodedRoutes, err := json.Marshal(remainingRoutes)
		if err != nil {
			return false, found, fmt.Errorf("encode routes in %q: %w", key, err)
		}
		object["routes"] = encodedRoutes
		updated, err := json.Marshal(object)
		if err != nil {
			return false, found, fmt.Errorf("encode WireGuard config %q: %w", key, err)
		}
		cm.Data[key] = string(updated)
	}
	if ownsNodeConfig && includeNodeConfig {
		delete(cm.Data, nodeName+".json")
	}
	return found, found, nil
}

func peerMatchesEvidence(peer map[string]json.RawMessage, publicKey string, evidence NodeWireGuardEvidence) bool {
	if evidence.PublicKey != "" && publicKey == evidence.PublicKey {
		return true
	}
	var allowed []string
	if err := json.Unmarshal(peer["allowedIPs"], &allowed); err != nil {
		return false
	}
	for _, prefix := range allowed {
		if slices.Contains(evidence.Addresses, prefix) {
			return true
		}
	}
	return false
}

func mergeNodeWireGuardEvidence(evidence NodeWireGuardEvidence, doc nodeWireGuardDocument) NodeWireGuardEvidence {
	if evidence.PublicKey == "" {
		evidence.PublicKey = doc.publicKey
	}
	if len(evidence.Addresses) == 0 && doc.address != "" {
		evidence.Addresses = []string{doc.address}
	}
	return evidence
}

func rawString(value json.RawMessage) (string, error) {
	if len(value) == 0 || string(value) == "null" {
		return "", nil
	}
	var result string
	if err := json.Unmarshal(value, &result); err != nil {
		return "", err
	}
	return result, nil
}

func validWireGuardPublicKey(value string) bool {
	decoded, err := base64.StdEncoding.DecodeString(value)
	return err == nil && len(decoded) == 32 && base64.StdEncoding.EncodeToString(decoded) == value
}
