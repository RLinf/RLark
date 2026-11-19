package controller

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	clusterGVK     = schema.GroupVersionKind{Group: "rlinf.io", Version: "v1alpha1", Kind: "Cluster"}
	clusterListGVK = schema.GroupVersionKind{Group: "rlinf.io", Version: "v1alpha1", Kind: "ClusterList"}
	rlarkNodeGVK   = schema.GroupVersionKind{Group: "rlinf.io", Version: "v1alpha1", Kind: "Node"}
)

// ManagementClusterResolver maps exactly one Cluster.spec.agentNamespace to its
// immutable Cluster object name. Duplicate claims fail closed.
type ManagementClusterResolver struct{ Reader client.Reader }

func (r ManagementClusterResolver) ResolveNamespace(ctx context.Context, namespace string) (ClusterRegistration, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(clusterListGVK)
	if err := r.Reader.List(ctx, list); err != nil {
		return ClusterRegistration{}, fmt.Errorf("list RLark Clusters: %w", err)
	}
	var registration ClusterRegistration
	for i := range list.Items {
		item := &list.Items[i]
		agentNamespace, found, err := unstructured.NestedString(item.Object, "spec", "agentNamespace")
		if err != nil {
			return ClusterRegistration{}, fmt.Errorf("read Cluster %q agentNamespace: %w", item.GetName(), err)
		}
		if !found || agentNamespace != namespace {
			continue
		}
		if registration.ClusterID != "" {
			return ClusterRegistration{}, fmt.Errorf("multiple RLark Clusters claim agent namespace %q", namespace)
		}
		registration = ClusterRegistration{ClusterID: item.GetName(), Namespace: agentNamespace}
	}
	if registration.ClusterID == "" {
		return ClusterRegistration{}, ErrInvalidRegistration
	}
	return registration, nil
}

// ClusterIDs lists the registered member-cluster identities after verifying
// each has a unique, non-empty agent namespace.
func (r ManagementClusterResolver) ClusterIDs(ctx context.Context) ([]string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(clusterListGVK)
	if err := r.Reader.List(ctx, list); err != nil {
		return nil, fmt.Errorf("list RLark Clusters: %w", err)
	}
	ids := make([]string, 0, len(list.Items))
	namespaces := make(map[string]string, len(list.Items))
	for i := range list.Items {
		item := &list.Items[i]
		namespace, found, err := unstructured.NestedString(item.Object, "spec", "agentNamespace")
		if err != nil || !found || namespace == "" {
			return nil, fmt.Errorf("Cluster %q has invalid agentNamespace", item.GetName())
		}
		if prior := namespaces[namespace]; prior != "" {
			return nil, fmt.Errorf("Clusters %q and %q share agent namespace %q", prior, item.GetName(), namespace)
		}
		namespaces[namespace] = item.GetName()
		ids = append(ids, item.GetName())
	}
	return ids, nil
}

// ManagementNodeReadiness reads a namespaced RLark Node without trusting labels
// for Cluster membership; the caller supplies the already-verified Cluster scope.
type ManagementNodeReadiness struct{ Reader client.Reader }

func (r ManagementNodeReadiness) Ready(ctx context.Context, _ string, namespace, nodeName string) (bool, error) {
	node := &unstructured.Unstructured{}
	node.SetGroupVersionKind(rlarkNodeGVK)
	if err := r.Reader.Get(ctx, client.ObjectKey{Namespace: namespace, Name: nodeName}, node); apierrors.IsNotFound(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("read RLark Node %s/%s: %w", namespace, nodeName, err)
	}
	conditions, found, err := unstructured.NestedSlice(node.Object, "status", "conditions")
	if err != nil {
		return false, fmt.Errorf("read RLark Node conditions: %w", err)
	}
	if !found {
		return false, nil
	}
	for _, raw := range conditions {
		condition, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		typeValue, _, _ := unstructured.NestedString(condition, "type")
		statusValue, _, _ := unstructured.NestedString(condition, "status")
		if typeValue == "Ready" {
			return statusValue == "True", nil
		}
	}
	return false, nil
}

// Namespace returns the management namespace of a registered cluster.
func (r ManagementClusterResolver) Namespace(ctx context.Context, clusterID string) (string, error) {
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(clusterListGVK)
	if err := r.Reader.List(ctx, list); err != nil {
		return "", fmt.Errorf("list RLark Clusters: %w", err)
	}
	for i := range list.Items {
		if namespace, _, _ := unstructured.NestedString(list.Items[i].Object, "spec", "agentNamespace"); list.Items[i].GetName() == clusterID && namespace != "" {
			return namespace, nil
		}
	}
	return "", ErrInvalidRegistration
}
