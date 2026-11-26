package kubernetes

import (
	"context"
	"errors"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	k8sptr "k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

func observationOwner(gvk schema.GroupVersionKind, name string, uid types.UID) metav1.OwnerReference {
	return metav1.OwnerReference{APIVersion: gvk.GroupVersion().String(), Kind: gvk.Kind, Name: name, UID: uid, Controller: k8sptr.To(true)}
}

func TestObservationProfileVerifiesDeclaredOwnerChain(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	depGVK := appsv1.SchemeGroupVersion.WithKind("Deployment")
	rsGVK := appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "root", Namespace: "runtime", UID: "root-uid"}}
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "rs", Namespace: "runtime", UID: "rs-uid", OwnerReferences: []metav1.OwnerReference{observationOwner(depGVK, dep.Name, dep.UID)}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "runtime", OwnerReferences: []metav1.OwnerReference{observationOwner(rsGVK, rs.Name, rs.UID)}}}
	profile, err := BuiltinObservationProfiles().Resolve(ObservationReplicaSetControllerV1, depGVK)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*corev1.Pod, *appsv1.ReplicaSet, *appsv1.Deployment)
		valid  bool
	}{
		{name: "current chain", valid: true},
		{name: "missing controller", mutate: func(p *corev1.Pod, _ *appsv1.ReplicaSet, _ *appsv1.Deployment) { p.OwnerReferences = nil }},
		{name: "intermediary API version", mutate: func(p *corev1.Pod, _ *appsv1.ReplicaSet, _ *appsv1.Deployment) {
			p.OwnerReferences[0].APIVersion = "extensions/v1beta1"
		}},
		{name: "intermediary UID", mutate: func(p *corev1.Pod, _ *appsv1.ReplicaSet, _ *appsv1.Deployment) { p.OwnerReferences[0].UID = "old" }},
		{name: "missing intermediary UID", mutate: func(p *corev1.Pod, _ *appsv1.ReplicaSet, _ *appsv1.Deployment) { p.OwnerReferences[0].UID = "" }},
		{name: "root API version", mutate: func(_ *corev1.Pod, r *appsv1.ReplicaSet, _ *appsv1.Deployment) {
			r.OwnerReferences[0].APIVersion = "extensions/v1beta1"
		}},
		{name: "root UID", mutate: func(_ *corev1.Pod, r *appsv1.ReplicaSet, _ *appsv1.Deployment) { r.OwnerReferences[0].UID = "old" }},
		{name: "other namespace", mutate: func(p *corev1.Pod, _ *appsv1.ReplicaSet, _ *appsv1.Deployment) { p.Namespace = "foreign" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, r, d := pod.DeepCopy(), rs.DeepCopy(), dep.DeepCopy()
			if tc.mutate != nil {
				tc.mutate(p, r, d)
			}
			reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(r, d).Build()
			root, err := profile.OwningRoot(context.Background(), reader, reader, p, &appsv1.Deployment{})
			if err != nil {
				t.Fatal(err)
			}
			if (root != nil) != tc.valid {
				t.Fatalf("root=%v valid=%v", root, tc.valid)
			}
		})
	}
}

func TestObservationDirectProfileSupportsTypedAndUnstructuredRoots(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	gvk := schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "TrainingJob"}
	crd := &unstructured.Unstructured{}
	crd.SetGroupVersionKind(gvk)
	crd.SetName("root")
	crd.SetNamespace("runtime")
	crd.SetUID("root-uid")
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "sts", Namespace: "runtime", UID: "sts-uid"}}
	for _, tc := range []struct {
		gvk  schema.GroupVersionKind
		root client.Object
	}{{gvk, crd}, {appsv1.SchemeGroupVersion.WithKind("StatefulSet"), sts}} {
		profile, err := BuiltinObservationProfiles().Resolve(ObservationDirectControllerV1, tc.gvk)
		if err != nil {
			t.Fatal(err)
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "runtime", OwnerReferences: []metav1.OwnerReference{observationOwner(tc.gvk, tc.root.GetName(), tc.root.GetUID())}}}
		reader := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tc.root).Build()
		root, err := profile.OwningRoot(context.Background(), reader, reader, pod, tc.root.DeepCopyObject().(client.Object))
		if err != nil || root == nil || root.GetUID() != tc.root.GetUID() {
			t.Fatalf("root=%v error=%v", root, err)
		}
		pod.OwnerReferences[0].UID = "old"
		if root, err := profile.OwningRoot(context.Background(), reader, reader, pod, tc.root.DeepCopyObject().(client.Object)); err != nil || root != nil {
			t.Fatalf("foreign root=%v error=%v", root, err)
		}
	}
}

type failingObservationReader struct {
	client.Reader
	err error
}

func (r failingObservationReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return r.err
}

func TestObservationOwnerReadFailureIsRetryable(t *testing.T) {
	profile, err := BuiltinObservationProfiles().Resolve(ObservationDirectControllerV1, appsv1.SchemeGroupVersion.WithKind("StatefulSet"))
	if err != nil {
		t.Fatal(err)
	}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "runtime", OwnerReferences: []metav1.OwnerReference{observationOwner(profile.Root, "root", "root-uid")}}}
	want := errors.New("unavailable")
	if _, err := profile.OwningRoot(context.Background(), failingObservationReader{err: want}, failingObservationReader{err: want}, pod, &appsv1.StatefulSet{}); !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
}

func TestObservationOwnerChainUsesDirectIntermediaryAndTypedCachedRoot(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	profile, err := BuiltinObservationProfiles().Resolve(ObservationReplicaSetControllerV1, appsv1.SchemeGroupVersion.WithKind("Deployment"))
	if err != nil {
		t.Fatal(err)
	}
	root := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "root", Namespace: "runtime", UID: "root-uid"}}
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "rs", Namespace: "runtime", UID: "rs-uid", OwnerReferences: []metav1.OwnerReference{observationOwner(profile.Root, root.Name, root.UID)}}}
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "runtime", OwnerReferences: []metav1.OwnerReference{observationOwner(profile.Intermediates[0], rs.Name, rs.UID)}}}
	cachedGets, directGets := 0, 0
	cached := fake.NewClientBuilder().WithScheme(scheme).WithObjects(root).WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, reader client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
		cachedGets++
		if _, ok := obj.(*appsv1.Deployment); !ok {
			t.Fatalf("cache read wrong prototype: %T", obj)
		}
		return reader.Get(ctx, key, obj, opts...)
	}}).Build()
	direct := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rs).WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, reader client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
		directGets++
		if obj.GetObjectKind().GroupVersionKind() != profile.Intermediates[0] {
			t.Fatalf("direct read wrong GVK: %v", obj.GetObjectKind().GroupVersionKind())
		}
		return reader.Get(ctx, key, obj, opts...)
	}}).Build()
	if got, err := profile.OwningRoot(context.Background(), cached, direct, pod, &appsv1.Deployment{}); err != nil || got == nil {
		t.Fatalf("root=%v error=%v", got, err)
	}
	if cachedGets != 1 || directGets != 1 {
		t.Fatalf("cached=%d direct=%d", cachedGets, directGets)
	}
	rs.OwnerReferences[0].UID = "new-root-uid"
	if err := direct.Update(context.Background(), rs); err != nil {
		t.Fatal(err)
	}
	if got, err := profile.OwningRoot(context.Background(), cached, direct, pod, &appsv1.Deployment{}); err != nil || got != nil {
		t.Fatalf("lagging cached UID accepted: root=%v error=%v", got, err)
	}
}

func TestObservationProfileRegistryRejectsInvalidContractsAndCopiesPaths(t *testing.T) {
	gvk := appsv1.SchemeGroupVersion.WithKind("ReplicaSet")
	for _, profiles := range [][]ObservationProfile{
		{{}}, {{ID: "a"}, {ID: "a"}}, {{ID: "a", Root: schema.GroupVersionKind{Kind: "Invalid"}}}, {{ID: "a", Intermediates: []schema.GroupVersionKind{gvk, gvk}}},
	} {
		if _, err := NewObservationProfiles(profiles...); err == nil {
			t.Fatal("invalid profile accepted")
		}
	}
	path := []schema.GroupVersionKind{gvk}
	registry, err := NewObservationProfiles(ObservationProfile{ID: "a", Intermediates: path})
	if err != nil {
		t.Fatal(err)
	}
	path[0].Kind = "mutated"
	first, err := registry.Resolve("a", appsv1.SchemeGroupVersion.WithKind("Deployment"))
	if err != nil {
		t.Fatal(err)
	}
	first.Intermediates[0].Kind = "also-mutated"
	fresh, err := registry.Resolve("a", appsv1.SchemeGroupVersion.WithKind("Deployment"))
	if err != nil || fresh.Intermediates[0] != gvk {
		t.Fatalf("profile=%v error=%v", fresh, err)
	}
	if _, err := registry.Resolve("unknown", gvk); err == nil {
		t.Fatal("unknown profile accepted")
	}
}
