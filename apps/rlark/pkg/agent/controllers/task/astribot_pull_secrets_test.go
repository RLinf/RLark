package task

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	"github.com/rlinf/rlark/apps/rlark/pkg/common"
	"github.com/rlinf/rlark/apps/rlark/pkg/tenant"
)

// An image registry credential that comes to match a Task's images (live 11-17: the office's credential moved to
// registry.rlark.internal:5443) replaces no running Pod of an unchanged Task: its workload keeps the credentials it
// was written with. A Task that changes gets the matching ones with the Pods the change replaces anyway.
func TestACredentialThatComesToMatchDoesNotReplaceRunningPods(t *testing.T) {
	ctx := context.Background()
	scheme := executionScheme(t)
	task := perNodeExecutionTask()
	task.Spec.Workload.Template.Spec.Containers[0].Image = "registry.rlark.internal:5443/fleet/station:v1"
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{ManagementTaskUIDLabel: string(task.UID)}}
	existing := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: task.Name, Namespace: tenant.DefaultNamespace, UID: "daemonset-uid", ResourceVersion: "1", Generation: 1, Annotations: map[string]string{
			ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace,
			ManagementTaskUIDAnnotation: string(task.UID), ManagementTaskGenerationAnnotation: fmt.Sprintf("%d", task.Generation),
		}},
		Spec: appsv1.DaemonSetSpec{Selector: selector, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: selector.MatchLabels}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "registry.rlark.internal:5443/fleet/station:v1"}}}}},
	}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(existing).WithObjects(existing).
		WithIndex(&corev1.Event{}, "involvedObject.uid", func(object client.Object) []string {
			return []string{string(object.(*corev1.Event).InvolvedObject.UID)}
		}).Build()
	key := client.ObjectKeyFromObject(task)
	pass := func() *appsv1.DaemonSet {
		t.Helper()
		execution, err := newTestKubernetesExecutionCoordinator(local, "rlark:1")
		if err != nil {
			t.Fatal(err)
		}
		controller := newHostedTestController(t, base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local}, execution)
		if _, err := (&pullReconciler{c: controller}).Reconcile(ctx, reconcile.Request{NamespacedName: key}); err != nil {
			t.Fatal(err)
		}
		daemonSet := &appsv1.DaemonSet{}
		if err := local.Get(ctx, client.ObjectKeyFromObject(existing), daemonSet); err != nil {
			t.Fatal(err)
		}
		return daemonSet
	}

	first := pass()
	if err := local.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ir-e9a22bbb4e156a1b", Namespace: tenant.DefaultNamespace,
			Labels: map[string]string{common.ImageRegistryCredentialLabel: "true"}, Annotations: map[string]string{common.ImageRegistryAnnotationRegistry: "registry.rlark.internal:5443"}},
		Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`)},
	}); err != nil {
		t.Fatal(err)
	}
	if second := pass(); !reflect.DeepEqual(second.Spec, first.Spec) {
		t.Fatalf("a credential that came to match changed a running workload: %v", second.Spec.Template.Spec.ImagePullSecrets)
	}

	var current rlarkv1alpha1.Task
	if err := management.Get(ctx, key, &current); err != nil {
		t.Fatal(err)
	}
	current.Generation++
	current.Spec.Workload.Template.Spec.Containers[0].Image = "registry.rlark.internal:5443/fleet/station:v2"
	if err := management.Update(ctx, &current); err != nil {
		t.Fatal(err)
	}
	third := pass()
	if !slices.Contains(third.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: "ir-e9a22bbb4e156a1b"}) {
		t.Fatalf("after the Task changed it pulls with %v", third.Spec.Template.Spec.ImagePullSecrets)
	}

	// A Pod that is not ready may be waiting for the credential: the unchanged Task takes it at once.
	third.Status.DesiredNumberScheduled = 1
	if err := local.Status().Update(ctx, third); err != nil {
		t.Fatal(err)
	}
	if err := local.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "ir-late", Namespace: tenant.DefaultNamespace,
			Labels: map[string]string{common.ImageRegistryCredentialLabel: "true"}, Annotations: map[string]string{common.ImageRegistryAnnotationRegistry: "registry.rlark.internal:5443"}},
		Type: corev1.SecretTypeDockerConfigJson, Data: map[string][]byte{corev1.DockerConfigJsonKey: []byte(`{"auths":{}}`)},
	}); err != nil {
		t.Fatal(err)
	}
	if fourth := pass(); !slices.Contains(fourth.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: "ir-late"}) {
		t.Fatalf("with a Pod not ready it pulls with %v", fourth.Spec.Template.Spec.ImagePullSecrets)
	}
}
