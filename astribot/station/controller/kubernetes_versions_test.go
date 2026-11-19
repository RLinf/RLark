package controller

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

type memberClients map[string]kubernetes.Interface

func (m memberClients) Client(_ context.Context, cluster string) (kubernetes.Interface, error) {
	if member, ok := m[cluster]; ok {
		return member, nil
	}
	return nil, errors.New("member unreachable")
}

func TestKubernetesVersionReporterAnnotatesNodes(t *testing.T) {
	cluster := func(name string) *unstructured.Unstructured {
		c := &unstructured.Unstructured{}
		c.SetGroupVersionKind(clusterGVK)
		c.SetName(name)
		_ = unstructured.SetNestedField(c.Object, name, "spec", "agentNamespace")
		return c
	}
	node := func(namespace, name string, annotations map[string]string) *unstructured.Unstructured {
		n := &unstructured.Unstructured{}
		n.SetGroupVersionKind(rlarkNodeGVK)
		n.SetNamespace(namespace)
		n.SetName(name)
		n.SetAnnotations(annotations)
		return n
	}
	memberNode := func(name, kubelet string) *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name}, Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KubeletVersion: kubelet}}}
	}
	member := func(gitVersion string, nodes ...runtime.Object) kubernetes.Interface {
		m := kubefake.NewSimpleClientset(nodes...)
		m.Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: gitVersion}
		return m
	}
	annotations := func(ctx context.Context, c client.Client, namespace, name string) map[string]string {
		got := node(namespace, name, nil)
		if err := c.Get(ctx, client.ObjectKeyFromObject(got), got); err != nil {
			t.Fatal(err)
		}
		return got.GetAnnotations()
	}
	// The robot cluster was upgraded; one robot's kubelet is still the old one and one RLark Node has no member Node.
	// The cloud cluster cannot be reached: it keeps what was read before and says so.
	old := map[string]string{KubernetesVersionAnnotation: "v1.33.13", KubeletVersionAnnotation: "v1.33.13"}
	patches := 0
	management := fake.NewClientBuilder().WithScheme(runtime.NewScheme()).WithObjects(
		cluster("rlark-robot-s1u-01"), cluster("rlark-ksyun-gpu"),
		node("rlark-robot-s1u-01", "alexfu", old), node("rlark-robot-s1u-01", "s1u-01-x86", old), node("rlark-robot-s1u-01", "s1u-03-x86", nil),
		node("rlark-ksyun-gpu", "10-0-2-57", old),
	).WithInterceptorFuncs(interceptor.Funcs{Patch: func(ctx context.Context, c client.WithWatch, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
		patches++
		return c.Patch(ctx, obj, patch, opts...)
	}}).Build()
	members := memberClients{"rlark-robot-s1u-01": member("v1.36.6", memberNode("alexfu", "v1.36.6"), memberNode("s1u-01-x86", "v1.33.13"))}
	r := &KubernetesVersionReporter{Reader: management, Writer: management, Members: members}
	ctx := context.Background()
	if err := r.Report(ctx); err == nil {
		t.Fatal("an unreachable member is reported as an error")
	}
	if err := r.Report(ctx); err == nil || patches != 4 {
		t.Fatalf("the second pass writes nothing new: %d patches, %v", patches, err)
	}
	for name, want := range map[string]map[string]string{
		"alexfu":     {KubernetesVersionAnnotation: "v1.36.6", KubeletVersionAnnotation: "v1.36.6"},
		"s1u-01-x86": {KubernetesVersionAnnotation: "v1.36.6", KubeletVersionAnnotation: "v1.33.13"},
		"s1u-03-x86": {KubernetesVersionAnnotation: "v1.36.6"},
	} {
		if got := annotations(ctx, management, "rlark-robot-s1u-01", name); !reflect.DeepEqual(got, want) {
			t.Fatalf("node %s: %v", name, got)
		}
	}
	want := map[string]string{KubernetesVersionAnnotation: "v1.33.13", KubeletVersionAnnotation: "v1.33.13", KubernetesVersionErrorAnnotation: "member unreachable"}
	if got := annotations(ctx, management, "rlark-ksyun-gpu", "10-0-2-57"); !reflect.DeepEqual(got, want) {
		t.Fatalf("unreachable node: %v", got)
	}

	members["rlark-ksyun-gpu"] = member("v1.36.6", memberNode("10-0-2-57", "v1.36.6"))
	if err := r.Report(ctx); err != nil {
		t.Fatal(err)
	}
	want = map[string]string{KubernetesVersionAnnotation: "v1.36.6", KubeletVersionAnnotation: "v1.36.6"}
	if got := annotations(ctx, management, "rlark-ksyun-gpu", "10-0-2-57"); !reflect.DeepEqual(got, want) {
		t.Fatalf("reachable again: %v", got)
	}
}
