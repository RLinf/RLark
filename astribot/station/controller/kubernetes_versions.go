package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Annotations on each RLark Node, read from its member cluster; the console's 集群管理 shows them per cluster.
const (
	// KubernetesVersionAnnotation is the version the member's API server runs (gitVersion).
	KubernetesVersionAnnotation = "station.rlark.io/kubernetes-version"
	// KubeletVersionAnnotation is the version of the kubelet the member Node of the same name reports.
	KubeletVersionAnnotation = "station.rlark.io/kubelet-version"
	// KubernetesVersionErrorAnnotation says why the member's last read failed; the versions are then the ones read before.
	KubernetesVersionErrorAnnotation = "station.rlark.io/kubernetes-version-error"
)

// KubernetesVersionReporter keeps on the management plane the Kubernetes version each member cluster runs and
// each of its nodes' kubelets, read from the members themselves, so an upgrade shows without anyone writing it.
type KubernetesVersionReporter struct {
	Reader  client.Reader
	Writer  client.Writer
	Members NodeMemberClientSource
	Every   time.Duration
}

// Start reports now and then every r.Every until ctx ends; it runs on the leader only.
func (r *KubernetesVersionReporter) Start(ctx context.Context) error {
	log := ctrl.Log.WithName("kubernetes-versions")
	ticker := time.NewTicker(r.Every)
	defer ticker.Stop()
	for {
		if err := r.Report(ctx); err != nil {
			log.Error(err, "report member Kubernetes versions")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Report reads every registered cluster's versions once.
func (r *KubernetesVersionReporter) Report(ctx context.Context) error {
	clusters := &unstructured.UnstructuredList{}
	clusters.SetGroupVersionKind(clusterListGVK)
	if err := r.Reader.List(ctx, clusters); err != nil {
		return fmt.Errorf("list RLark Clusters: %w", err)
	}
	var errs []error
	for i := range clusters.Items {
		if err := r.reportCluster(ctx, &clusters.Items[i]); err != nil {
			errs = append(errs, fmt.Errorf("cluster %s: %w", clusters.Items[i].GetName(), err))
		}
	}
	return errors.Join(errs...)
}

func (r *KubernetesVersionReporter) reportCluster(ctx context.Context, cluster *unstructured.Unstructured) error {
	namespace, _, _ := unstructured.NestedString(cluster.Object, "spec", "agentNamespace")
	nodes := &unstructured.UnstructuredList{}
	nodes.SetGroupVersionKind(rlarkNodeGVK.GroupVersion().WithKind("NodeList"))
	if err := r.Reader.List(ctx, nodes, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("list RLark Nodes: %w", err)
	}
	version, kubelets, reason, readErr := r.readMember(ctx, cluster.GetName())
	var errs []error
	for i := range nodes.Items {
		values := map[string]string{KubernetesVersionErrorAnnotation: reason}
		if readErr == nil {
			values[KubernetesVersionAnnotation] = version
			if kubelet, ok := kubelets[nodes.Items[i].GetName()]; ok {
				values[KubeletVersionAnnotation] = kubelet
			}
		}
		errs = append(errs, r.annotate(ctx, &nodes.Items[i], values))
	}
	return errors.Join(append(errs, readErr)...)
}

// readMember returns the member API server's version and each member Node's kubelet version, or a short fixed
// reason for the page and the error behind it.
func (r *KubernetesVersionReporter) readMember(ctx context.Context, name string) (string, map[string]string, string, error) {
	member, err := r.Members.Client(ctx, name)
	if err != nil {
		return "", nil, "member unreachable", err
	}
	version, err := member.Discovery().ServerVersion()
	if err != nil {
		return "", nil, "API server version unreadable", fmt.Errorf("read the member API server's version: %w", err)
	}
	memberNodes, err := member.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", nil, "member Nodes unreadable", fmt.Errorf("list member Nodes: %w", err)
	}
	kubelets := make(map[string]string, len(memberNodes.Items))
	for _, node := range memberNodes.Items {
		kubelets[node.Name] = node.Status.NodeInfo.KubeletVersion
	}
	return version.GitVersion, kubelets, "", nil
}

// annotate writes the values that differ from object's annotations in one merge patch; "" removes an annotation.
func (r *KubernetesVersionReporter) annotate(ctx context.Context, object *unstructured.Unstructured, values map[string]string) error {
	current := object.GetAnnotations()
	changes := map[string]any{}
	for key, value := range values {
		existing, ok := current[key]
		switch {
		case value == "" && ok:
			changes[key] = nil
		case value != "" && existing != value:
			changes[key] = value
		}
	}
	if len(changes) == 0 {
		return nil
	}
	patch, _ := json.Marshal(map[string]any{"metadata": map[string]any{"annotations": changes}})
	if err := r.Writer.Patch(ctx, object, client.RawPatch(types.MergePatchType, patch)); err != nil {
		return fmt.Errorf("annotate Node %s: %w", object.GetName(), err)
	}
	return nil
}
