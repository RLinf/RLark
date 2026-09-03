package task

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

func TestCleanupWorkloadWaitsForStatefulSetAndPVC(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name: "task", Namespace: "rlark-system", Finalizers: []string{"test/finalizer"},
	}}
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{
		Name: "data", Namespace: "rlark-system", Finalizers: []string{"test/finalizer"},
		Labels: map[string]string{PVCTaskLabel: "task"},
	}}
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(sts, pvc).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: localClient})}

	pending, err := r.cleanupWorkload(context.Background(), "task", "rlark-system")
	if err != nil {
		t.Fatal(err)
	}
	if !pending {
		t.Fatal("cleanup should remain pending while StatefulSet/PVC still exist")
	}

	var remainingSTS appsv1.StatefulSet
	if err := localClient.Get(context.Background(), types.NamespacedName{Name: "task", Namespace: "rlark-system"}, &remainingSTS); err != nil {
		t.Fatalf("StatefulSet should still exist until its finalizer is cleared: %v", err)
	}
	var remainingPVC corev1.PersistentVolumeClaim
	if err := localClient.Get(context.Background(), types.NamespacedName{Name: "data", Namespace: "rlark-system"}, &remainingPVC); err != nil {
		t.Fatalf("PVC should still exist until its finalizer is cleared: %v", err)
	}
}

func TestRestartCleanupRequiredAndAnnotationPropagation(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	oldSTS := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
		Name: "task", Namespace: "rlark-system",
		Annotations: map[string]string{RestartedAtAnnotation: "old"},
	}}
	localClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(oldSTS).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: localClient})}
	mgmtTask := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "task", Namespace: "default", ResourceVersion: "2",
		Annotations: map[string]string{RestartedAtAnnotation: "new"},
	}}

	required, err := r.restartCleanupRequired(context.Background(), mgmtTask, "rlark-system")
	if err != nil {
		t.Fatal(err)
	}
	if !required {
		t.Fatal("new restart annotation should require cleanup")
	}
	built := buildStatefulSet(mgmtTask, &rlarkv1alpha1.KubernetesWorkloadSpec{})
	if got := built.Annotations[RestartedAtAnnotation]; got != "new" {
		t.Fatalf("StatefulSet restart annotation = %q, want new", got)
	}

	oldSTS.Annotations[RestartedAtAnnotation] = "new"
	if err := localClient.Update(context.Background(), oldSTS); err != nil {
		t.Fatal(err)
	}
	required, err = r.restartCleanupRequired(context.Background(), mgmtTask, "rlark-system")
	if err != nil {
		t.Fatal(err)
	}
	if required {
		t.Fatal("matching restart annotation should make rebuild idempotent")
	}
}
