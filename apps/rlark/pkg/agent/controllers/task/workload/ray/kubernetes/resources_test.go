package kubernetes

import (
	"context"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/rlinf/rlark/apps/rlark/pkg/common"
)

func TestEnsureImagePullSecretsIsStableAcrossReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "registry", Namespace: "ns", Labels: map[string]string{common.ImageRegistryCredentialLabel: "true"}, Annotations: map[string]string{common.ImageRegistryAnnotationRegistry: "registry.example.com"}}, Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("opaque")}}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
	template := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Image: "registry.example.com/project/image:v1"}}, ImagePullSecrets: []corev1.LocalObjectReference{{Name: "user"}}}}

	if err := EnsureImagePullSecrets(context.Background(), kubeClient, template, "ns"); err != nil {
		t.Fatal(err)
	}
	first := append([]corev1.LocalObjectReference(nil), template.Spec.ImagePullSecrets...)
	if err := EnsureImagePullSecrets(context.Background(), kubeClient, template, "ns"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(template.Spec.ImagePullSecrets, first) {
		t.Fatalf("second reconcile changed imagePullSecrets: first=%v second=%v", first, template.Spec.ImagePullSecrets)
	}
}

func TestEnsureImagePullSecretsMatchesAllContainersAndExcludesOtherRegistries(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	credential := func(name, registry string) *corev1.Secret {
		return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "ns", Labels: map[string]string{common.ImageRegistryCredentialLabel: "true"}, Annotations: map[string]string{common.ImageRegistryAnnotationRegistry: registry}}, Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte("opaque")}}
	}
	kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(credential("match-b", "registry.example.com"), credential("match-a", "registry.example.com"), credential("other", "other.example.com")).Build()
	template := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{
		Containers:       []corev1.Container{{Image: "registry.example.com/project/main:v1"}},
		InitContainers:   []corev1.Container{{Image: "registry.example.com/project/init:v1"}},
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "user"}},
	}}
	if err := EnsureImagePullSecrets(context.Background(), kubeClient, template, "ns"); err != nil {
		t.Fatal(err)
	}
	want := []corev1.LocalObjectReference{{Name: "user"}, {Name: "match-a"}, {Name: "match-b"}}
	if !reflect.DeepEqual(template.Spec.ImagePullSecrets, want) {
		t.Fatalf("imagePullSecrets=%v, want %v", template.Spec.ImagePullSecrets, want)
	}
}
