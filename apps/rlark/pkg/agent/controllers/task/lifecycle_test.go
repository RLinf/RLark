package task

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
)

type failingListClient struct {
	client.Client
	failEvents bool
}

func (c *failingListClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if _, ok := list.(*corev1.PodList); ok && !c.failEvents {
		return fmt.Errorf("pod list failed")
	}
	if _, ok := list.(*corev1.EventList); ok && c.failEvents {
		return fmt.Errorf("event list failed")
	}
	return c.Client.List(ctx, list, opts...)
}

func TestWorkloadIdentityPreservesUserLabels(t *testing.T) {
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "task-uid"}}
	template := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"user": "label"}}}
	applyWorkloadIdentity(&template, task)
	spec := &rlarkv1alpha1.KubernetesWorkloadSpec{Template: template}
	deploy := buildDeployment(task, spec)

	if deploy.Spec.Template.Labels["user"] != "label" {
		t.Fatal("user label was not preserved")
	}
	if got := deploy.Spec.Selector.MatchLabels; !reflect.DeepEqual(got, map[string]string{ManagementTaskUIDLabel: "task-uid"}) {
		t.Fatalf("selector = %v", got)
	}
	if deploy.Spec.Template.Labels[ManagementTaskUIDLabel] != "task-uid" || deploy.Spec.Template.Annotations[ManagementTaskUIDAnnotation] != "task-uid" {
		t.Fatal("template is missing Task UID identity")
	}
}

func TestCreateOrUpdateWorkloadWaitsForOldKind(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	old := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Finalizers: []string{"hold"}, Annotations: map[string]string{
		ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default", ManagementTaskUIDAnnotation: "uid",
	}}}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(old).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid"}}
	desired := buildStatefulSet(task, &rlarkv1alpha1.KubernetesWorkloadSpec{Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{ManagementTaskUIDLabel: "uid"}}}})

	result, err := r.createOrUpdateWorkload(context.Background(), task, "StatefulSet", &appsv1.StatefulSet{}, desired, func(client.Object, client.Object) {})
	if err != nil || result.RequeueAfter == 0 {
		t.Fatalf("result=%v err=%v", result, err)
	}
	var created appsv1.StatefulSet
	err = local.Get(context.Background(), client.ObjectKeyFromObject(desired), &created)
	if err == nil {
		t.Fatal("new kind was created before old kind disappeared")
	}
	if client.IgnoreNotFound(err) != nil {
		t.Fatal(err)
	}
}

func TestCreateOrUpdateWorkloadAdoptsLegacyKindsWithoutChangingSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Generation: 4}, Spec: rlarkv1alpha1.TaskSpec{Domain: "domain"}}
	one := int32(1)
	for _, tt := range []struct {
		name     string
		existing client.Object
		desired  client.Object
	}{
		{"Deployment", &appsv1.Deployment{Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task", "legacy": "true"}}}}}, buildDeployment(task, &rlarkv1alpha1.KubernetesWorkloadSpec{})},
		{"DaemonSet", &appsv1.DaemonSet{Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task", "legacy": "true"}}}}}, buildDaemonSet(task, &rlarkv1alpha1.KubernetesWorkloadSpec{})},
		{"StatefulSet", &appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Replicas: &one, Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}}, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task", "legacy": "true"}}}}}, buildStatefulSet(task, &rlarkv1alpha1.KubernetesWorkloadSpec{})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.existing.SetName("task")
			tt.existing.SetNamespace("rlark-system")
			tt.existing.SetAnnotations(map[string]string{ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default"})
			before := tt.existing.DeepCopyObject().(client.Object)
			local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.existing).Build()
			r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}
			applied := false
			result, err := r.createOrUpdateWorkload(context.Background(), task, tt.name, tt.existing.DeepCopyObject().(client.Object), tt.desired, func(client.Object, client.Object) { applied = true })
			if err != nil || result.RequeueAfter != 0 || applied {
				t.Fatalf("result=%v err=%v", result, err)
			}
			got := tt.existing.DeepCopyObject().(client.Object)
			if err := local.Get(context.Background(), client.ObjectKeyFromObject(tt.existing), got); err != nil || !got.GetDeletionTimestamp().IsZero() {
				t.Fatalf("legacy workload was deleted: %v", err)
			}
			if !reflect.DeepEqual(workloadLabelSelector(got), workloadLabelSelector(before)) || !reflect.DeepEqual(workloadTemplate(got), workloadTemplate(before)) || !reflect.DeepEqual(workloadReplicas(got), workloadReplicas(before)) {
				t.Fatal("legacy workload selector, template, or replicas changed")
			}
			if got.GetAnnotations()[ManagementTaskUIDAnnotation] != "uid" || got.GetAnnotations()[ManagementTaskAdoptedGenerationAnnotation] != "4" || got.GetAnnotations()[ManagementTaskGenerationAnnotation] != "" || got.GetAnnotations()[ManagementTaskDomainAnnotation] != "domain" {
				t.Fatalf("annotations = %v", got.GetAnnotations())
			}
		})
	}
}

func TestCreateOrUpdateWorkloadAdoptsUnmarkedLegacyWorkload(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system"}, Spec: appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}},
		Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}},
	}}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid"}}
	result, err := r.createOrUpdateWorkload(context.Background(), task, "Deployment", &appsv1.Deployment{}, buildDeployment(task, &rlarkv1alpha1.KubernetesWorkloadSpec{}), func(client.Object, client.Object) {})
	if err != nil || result.RequeueAfter != 0 {
		t.Fatalf("result=%v err=%v", result, err)
	}
	var got appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil || got.Annotations[ManagementTaskUIDAnnotation] != "uid" {
		t.Fatalf("unmarked legacy workload was not adopted: %v annotations=%v", err, got.Annotations)
	}
}

func TestCreateOrUpdateWorkloadKeepsMatchingUIDWithLegacySelector(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	one := int32(1)
	existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{
		ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default", ManagementTaskUIDAnnotation: "uid",
	}}, Spec: appsv1.DeploymentSpec{
		Replicas: &one,
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}},
		Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task"}}},
	}}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Generation: 2}}
	applied := false
	result, err := r.createOrUpdateWorkload(context.Background(), task, "Deployment", &appsv1.Deployment{}, buildDeployment(task, &rlarkv1alpha1.KubernetesWorkloadSpec{}), func(client.Object, client.Object) { applied = true })
	if err != nil || result.RequeueAfter != 0 || applied {
		t.Fatalf("result=%v applied=%v err=%v", result, applied, err)
	}
	var got appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil || !reflect.DeepEqual(got.Spec, existing.Spec) {
		t.Fatalf("partially migrated workload changed: err=%v spec=%v", err, got.Spec)
	}
	if got.Annotations[ManagementTaskAdoptedGenerationAnnotation] != "2" {
		t.Fatalf("adopted generation annotation = %q", got.Annotations[ManagementTaskAdoptedGenerationAnnotation])
	}
}

func TestLegacyWorkloadAppliesNextGenerationWithoutChangingSelector(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	for _, kind := range []string{"Deployment", "DaemonSet", "StatefulSet"} {
		t.Run(kind, func(t *testing.T) {
			one, three := int32(1), int32(3)
			selector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}, MatchExpressions: []metav1.LabelSelectorRequirement{{Key: "tier", Operator: metav1.LabelSelectorOpIn, Values: []string{"worker"}}}}
			template := corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "task", "tier": "worker"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "old", Command: []string{"old"}}}}}
			meta := metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default"}}
			var existing client.Object
			switch kind {
			case "Deployment":
				existing = &appsv1.Deployment{ObjectMeta: meta, Spec: appsv1.DeploymentSpec{Replicas: &one, Selector: selector, Template: template}}
			case "DaemonSet":
				existing = &appsv1.DaemonSet{ObjectMeta: meta, Spec: appsv1.DaemonSetSpec{Selector: selector, Template: template}}
			case "StatefulSet":
				existing = &appsv1.StatefulSet{ObjectMeta: meta, Spec: appsv1.StatefulSetSpec{Replicas: &one, Selector: selector, Template: template}}
			}
			local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
			r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}
			task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Generation: 4}}
			desiredSpec := &rlarkv1alpha1.KubernetesWorkloadSpec{Replicas: &three, Template: corev1.PodTemplateSpec{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"tier": "worker"}}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "main", Image: "new", Command: []string{"new"}}}}}}
			reconcileKind := func() error {
				var err error
				switch kind {
				case "Deployment":
					_, err = r.createOrUpdateDeployment(context.Background(), task, desiredSpec)
				case "DaemonSet":
					_, err = r.createOrUpdateDaemonSet(context.Background(), task, desiredSpec)
				case "StatefulSet":
					_, err = r.createOrUpdateStatefulSet(context.Background(), task, desiredSpec)
				}
				return err
			}
			if err := reconcileKind(); err != nil {
				t.Fatal(err)
			}
			task.Generation++
			if err := reconcileKind(); err != nil {
				t.Fatal(err)
			}
			got := existing.DeepCopyObject().(client.Object)
			if err := local.Get(context.Background(), client.ObjectKeyFromObject(existing), got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(workloadLabelSelector(got), selector) || workloadTemplate(got).Labels["app"] != "task" || workloadTemplate(got).Spec.Containers[0].Image != "new" || workloadTemplate(got).Spec.Containers[0].Command[0] != "new" {
				t.Fatalf("workload update was incompatible: selector=%v template=%v", workloadLabelSelector(got), workloadTemplate(got))
			}
			if replicas := workloadReplicas(got); kind != "DaemonSet" && (replicas == nil || *replicas != 3) {
				t.Fatalf("replicas = %v", replicas)
			}
		})
	}
}

func TestLegacySelectorIsPreservedWithoutDeletion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	existing := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default"}}, Spec: appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"other": "value"}}}}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid"}}
	desired := buildDeployment(task, &rlarkv1alpha1.KubernetesWorkloadSpec{})
	result, err := r.createOrUpdateWorkload(context.Background(), task, "Deployment", &appsv1.Deployment{}, desired, func(client.Object, client.Object) {})
	if err != nil || result.RequeueAfter != 0 {
		t.Fatalf("result=%v err=%v", result, err)
	}
	var got appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(existing), &got); err != nil || !got.DeletionTimestamp.IsZero() {
		t.Fatalf("mismatched workload was deleted: %v", err)
	}
}

func workloadReplicas(obj client.Object) *int32 {
	switch workload := obj.(type) {
	case *appsv1.Deployment:
		return workload.Spec.Replicas
	case *appsv1.StatefulSet:
		return workload.Spec.Replicas
	default:
		return nil
	}
}

func TestKindMigrationNeverDeletesConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	conflict := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{
		ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default", ManagementTaskUIDAnnotation: "other",
	}}}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(conflict).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{LocalKubeClient: local})}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid"}}
	if _, err := r.createOrUpdateWorkload(context.Background(), task, "StatefulSet", &appsv1.StatefulSet{}, buildStatefulSet(task, &rlarkv1alpha1.KubernetesWorkloadSpec{}), func(client.Object, client.Object) {}); err == nil {
		t.Fatal("expected conflicting workload error")
	}
	var got appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(conflict), &got); err != nil || !got.DeletionTimestamp.IsZero() {
		t.Fatalf("conflicting workload was deleted: %v", err)
	}
}

func TestPushOwnsTaskFencesOldKind(t *testing.T) {
	task := &rlarkv1alpha1.Task{
		ObjectMeta: metav1.ObjectMeta{UID: "uid"},
		Spec: rlarkv1alpha1.TaskSpec{AgentType: rlarkv1alpha1.AgentTypeKubernetes, Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{
			Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Kind: rlarkv1alpha1.KubernetesWorkloadStatefulSet},
		}},
	}
	workload := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{ManagementTaskUIDAnnotation: "uid"}}}
	if pushOwnsTask(task, string(rlarkv1alpha1.AgentTypeKubernetes), workload, rlarkv1alpha1.KubernetesWorkloadDeployment) {
		t.Fatal("old Deployment push controller retained status ownership")
	}
}

func TestPushOwnsTaskAcceptsMissingUIDAndRejectsConflict(t *testing.T) {
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid"}, Spec: rlarkv1alpha1.TaskSpec{
		AgentType:  rlarkv1alpha1.AgentTypeKubernetes,
		Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Kind: rlarkv1alpha1.KubernetesWorkloadDeployment}},
	}}
	legacy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default"}}}
	if !pushOwnsTask(task, string(rlarkv1alpha1.AgentTypeKubernetes), legacy, rlarkv1alpha1.KubernetesWorkloadDeployment) {
		t.Fatal("legacy workload without UID did not retain status ownership")
	}
	legacy.Annotations[ManagementTaskUIDAnnotation] = "other"
	if pushOwnsTask(task, string(rlarkv1alpha1.AgentTypeKubernetes), legacy, rlarkv1alpha1.KubernetesWorkloadDeployment) {
		t.Fatal("conflicting UID retained status ownership")
	}
}

func TestPushReconcilersObserveLegacyWorkloadsWithoutUID(t *testing.T) {
	for _, tt := range []struct {
		name      string
		kind      rlarkv1alpha1.KubernetesWorkloadKind
		workload  func(metav1.ObjectMeta, *metav1.LabelSelector) client.Object
		reconcile func(*Controller) base.KubernetesReconciler
		ownerKind string
	}{
		{"Deployment", rlarkv1alpha1.KubernetesWorkloadDeployment, func(meta metav1.ObjectMeta, selector *metav1.LabelSelector) client.Object {
			return &appsv1.Deployment{ObjectMeta: meta, Spec: appsv1.DeploymentSpec{Selector: selector}}
		}, func(c *Controller) base.KubernetesReconciler { return &pushDeploymentReconciler{c: c} }, "Deployment"},
		{"DaemonSet", rlarkv1alpha1.KubernetesWorkloadDaemonSet, func(meta metav1.ObjectMeta, selector *metav1.LabelSelector) client.Object {
			return &appsv1.DaemonSet{ObjectMeta: meta, Spec: appsv1.DaemonSetSpec{Selector: selector}}
		}, func(c *Controller) base.KubernetesReconciler { return &pushDaemonSetReconciler{c: c} }, "DaemonSet"},
		{"StatefulSet", rlarkv1alpha1.KubernetesWorkloadStatefulSet, func(meta metav1.ObjectMeta, selector *metav1.LabelSelector) client.Object {
			return &appsv1.StatefulSet{ObjectMeta: meta, Spec: appsv1.StatefulSetSpec{Selector: selector}}
		}, func(c *Controller) base.KubernetesReconciler { return &pushStatefulSetReconciler{c: c} }, "StatefulSet"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = rlarkv1alpha1.AddToScheme(scheme)
			task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "task-uid"}, Spec: rlarkv1alpha1.TaskSpec{
				AgentType:  rlarkv1alpha1.AgentTypeKubernetes,
				Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Kind: tt.kind}},
			}}
			selector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "task"}}
			workload := tt.workload(metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", UID: "workload-uid", Annotations: map[string]string{
				ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default",
			}}, selector)
			controller := true
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: "rlark-system", Labels: selector.MatchLabels, OwnerReferences: []metav1.OwnerReference{{
				Kind: tt.ownerKind, Name: "task", UID: "workload-uid", Controller: &controller,
			}}}, Spec: corev1.PodSpec{NodeName: "node-a"}}
			management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
			local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workload, pod).Build()
			controllerConfig := NewTaskController(base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementClient: management, LocalKubeClient: local})
			if _, err := tt.reconcile(controllerConfig).Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(workload)}); err != nil {
				t.Fatal(err)
			}
			var got rlarkv1alpha1.Task
			if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil || !reflect.DeepEqual(got.Status.ObservedNodes, []string{"node-a"}) {
				t.Fatalf("observedNodes=%v err=%v", got.Status.ObservedNodes, err)
			}
		})
	}
}

func TestListTaskPodsValidatesOwnerUID(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	controller := true
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "ns", UID: "current"}}
	pods := []client.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "current", Namespace: "ns", Labels: map[string]string{"x": "y"}, OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: "task", UID: "current", Controller: &controller}}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "stale", Namespace: "ns", Labels: map[string]string{"x": "y"}, OwnerReferences: []metav1.OwnerReference{{Kind: "StatefulSet", Name: "task", UID: "old", Controller: &controller}}}},
	}
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pods...).Build()
	got, err := listTaskPods(context.Background(), local, sts, map[string]string{"x": "y"})
	if err != nil || len(got) != 1 || got[0].Name != "current" {
		t.Fatalf("pods=%v err=%v", got, err)
	}
}

func TestRolloutReducersAndObservedNodes(t *testing.T) {
	one := int32(1)
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 2}, Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{ObservedGeneration: 1, ReadyReplicas: 1, Replicas: 1, UnavailableReplicas: 1}}
	if phase, _ := deploymentStatusPhase(deploy); phase != rlarkv1alpha1.TaskPhasePending {
		t.Fatalf("stale deployment phase = %s", phase)
	}
	deploy.Status = appsv1.DeploymentStatus{ObservedGeneration: 2, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1}
	if phase, _ := deploymentStatusPhase(deploy); phase != rlarkv1alpha1.TaskPhaseRunning {
		t.Fatalf("ready deployment phase = %s", phase)
	}
	if got := podNodeNames([]corev1.Pod{{Spec: corev1.PodSpec{NodeName: "b"}}, {Spec: corev1.PodSpec{NodeName: "a"}}, {Spec: corev1.PodSpec{NodeName: "b"}}}); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("nodes = %v", got)
	}
}

func TestExactRolloutPredicates(t *testing.T) {
	one := int32(1)
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Generation: 1}, Spec: appsv1.DeploymentSpec{Replicas: &one}, Status: appsv1.DeploymentStatus{
		ObservedGeneration: 1, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1,
	}}
	if phase, _ := deploymentStatusPhase(deploy); phase != rlarkv1alpha1.TaskPhasePending {
		t.Fatalf("unavailable Deployment phase = %s", phase)
	}
	deploy.Status.Conditions = []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse, Reason: "ProgressDeadlineExceeded", Message: "deadline"}}
	if phase, message := deploymentStatusPhase(deploy); phase != rlarkv1alpha1.TaskPhaseFailed || message != "deadline" {
		t.Fatalf("deadline Deployment phase=%s message=%q", phase, message)
	}

	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Generation: 1}, Spec: appsv1.StatefulSetSpec{Replicas: &one, Selector: &metav1.LabelSelector{}}, Status: appsv1.StatefulSetStatus{
		ObservedGeneration: 1, CurrentReplicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, CurrentRevision: "old", UpdateRevision: "new",
	}}
	if phase, _, _, err := statefulSetPhase(context.Background(), local, sts); err != nil || phase != rlarkv1alpha1.TaskPhasePending {
		t.Fatalf("revision-mismatched StatefulSet phase=%s err=%v", phase, err)
	}
	ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Generation: 1}, Spec: appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{}}, Status: appsv1.DaemonSetStatus{
		ObservedGeneration: 1, DesiredNumberScheduled: 1, UpdatedNumberScheduled: 1, NumberReady: 1,
	}}
	if phase, _, _, err := daemonSetPhase(context.Background(), local, ds); err != nil || phase != rlarkv1alpha1.TaskPhasePending {
		t.Fatalf("unavailable DaemonSet phase=%s err=%v", phase, err)
	}
}

func TestMergeWorkloadAnnotationsPreservesUnrelated(t *testing.T) {
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Generation: 2}}
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"example.com/user": "keep"}}}
	mergeWorkloadAnnotations(deploy, task)
	if deploy.Annotations["example.com/user"] != "keep" || deploy.Annotations[ManagementTaskUIDAnnotation] != "uid" {
		t.Fatalf("annotations = %v", deploy.Annotations)
	}
}

func TestPhasePropagatesPodAndEventListErrors(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	controller := true
	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "ns", UID: "uid", Generation: 1},
		Spec:       appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"x": "y"}}},
		Status:     appsv1.DeploymentStatus{ObservedGeneration: 1},
	}
	baseClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name: "pod", Namespace: "ns", UID: "pod-uid", Labels: map[string]string{"x": "y"},
		OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "task", UID: "uid", Controller: &controller}},
	}}).Build()
	if _, _, _, err := deploymentPhase(context.Background(), &failingListClient{Client: baseClient}, deploy); err == nil {
		t.Fatal("pod list error was swallowed")
	}
	if _, _, _, err := deploymentPhase(context.Background(), &failingListClient{Client: baseClient, failEvents: true}, deploy); err == nil {
		t.Fatal("event list error was swallowed")
	}
}

func TestFinalizerAddedOnlyAfterResponsibilityValidation(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rlarkv1alpha1.AddToScheme(scheme)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default"}, Spec: rlarkv1alpha1.TaskSpec{AgentType: rlarkv1alpha1.AgentTypeDocker}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementClient: management})}
	_, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: types.NamespacedName{Name: "task", Namespace: "default"}})
	if err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Task
	_ = management.Get(context.Background(), client.ObjectKeyFromObject(task), &got)
	if len(got.Finalizers) != 0 {
		t.Fatalf("unexpected finalizers: %v", got.Finalizers)
	}
}

func TestWrongAgentDoesNotCleanOrRemoveClaimedFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rlarkv1alpha1.AddToScheme(scheme)
	now := metav1.Now()
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{
		Name: "task", Namespace: "default", UID: "uid", DeletionTimestamp: &now, Finalizers: []string{ManagementTaskFinalizer},
		Annotations: map[string]string{ManagementTaskClaimantAnnotation: "other/Kubernetes", ManagementTaskClaimedDomainAnnotation: "domain"},
	}, Spec: rlarkv1alpha1.TaskSpec{AgentType: rlarkv1alpha1.AgentTypeKubernetes, Domain: "domain"}}
	workload := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Finalizers: []string{"hold"}, Annotations: map[string]string{
		ManagementTaskNameAnnotation: "task", ManagementTaskNamespaceAnnotation: "default", ManagementTaskUIDAnnotation: "uid",
	}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workload).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{
		AgentType: string(rlarkv1alpha1.AgentTypeKubernetes), ManagementNamespace: "shared", ManagementClient: management, LocalKubeClient: local,
	})}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	var gotTask rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &gotTask); err != nil || !containsString(gotTask.Finalizers, ManagementTaskFinalizer) {
		t.Fatalf("wrong agent removed finalizer: %v", err)
	}
	var gotWorkload appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(workload), &gotWorkload); err != nil || !gotWorkload.DeletionTimestamp.IsZero() {
		t.Fatalf("wrong agent deleted workload: %v", err)
	}
}

func TestFinalizerWithoutClaimantIsClaimedBeforeNormalReconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rlarkv1alpha1.AddToScheme(scheme)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Finalizers: []string{ManagementTaskFinalizer}}, Spec: rlarkv1alpha1.TaskSpec{
		AgentType: rlarkv1alpha1.AgentTypeKubernetes, Domain: "domain", Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Kind: rlarkv1alpha1.KubernetesWorkloadDeployment}},
	}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{AgentType: "Kubernetes", ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local})}

	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
		t.Fatal(err)
	}
	if got.Annotations[ManagementTaskClaimantAnnotation] != "cluster/Kubernetes" || got.Annotations[ManagementTaskClaimedDomainAnnotation] != "domain" {
		t.Fatalf("claim annotations = %#v", got.Annotations)
	}
	var workload appsv1.Deployment
	if err := local.Get(context.Background(), types.NamespacedName{Name: "task", Namespace: "rlark-system"}, &workload); err != nil {
		t.Fatalf("legacy Task was not reconciled: %v", err)
	}
}

func TestDeletingFinalizerWithoutClaimantIsClaimedAndCleaned(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rlarkv1alpha1.AddToScheme(scheme)
	now := metav1.Now()
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", DeletionTimestamp: &now, Finalizers: []string{ManagementTaskFinalizer}}, Spec: rlarkv1alpha1.TaskSpec{AgentType: rlarkv1alpha1.AgentTypeKubernetes, Domain: "domain"}}
	workload := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{ManagementTaskUIDAnnotation: "uid"}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workload).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{AgentType: "Kubernetes", ManagementNamespace: "cluster", ManagementClient: management, LocalKubeClient: local})}
	request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}

	for range 2 {
		if _, err := r.Reconcile(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	var got rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); client.IgnoreNotFound(err) != nil {
		t.Fatal(err)
	}
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(workload), &appsv1.Deployment{}); !errors.IsNotFound(err) {
		t.Fatalf("legacy workload was not cleaned: %v", err)
	}
}

func TestClaimantIdentityDoesNotDependOnLeaderIdentity(t *testing.T) {
	r := &pullReconciler{c: NewTaskController(base.Controller{AgentType: "Kubernetes", ManagementNamespace: "cluster"})}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{ManagementTaskClaimantAnnotation: "cluster/Kubernetes"}}}
	if r.claimantIdentity() != "cluster/Kubernetes" || !r.isTaskClaimant(task) {
		t.Fatal("stable namespace/type scope did not retain ownership")
	}
	for _, claimant := range []string{"other/Kubernetes", "cluster/Docker"} {
		task.Annotations[ManagementTaskClaimantAnnotation] = claimant
		if r.isTaskClaimant(task) {
			t.Fatalf("wrong scope %q was accepted", claimant)
		}
	}
}

func TestClaimantHandoffAfterDesiredSpecChanges(t *testing.T) {
	for _, tt := range []struct {
		name   string
		mutate func(*rlarkv1alpha1.Task)
	}{
		{"AgentType", func(task *rlarkv1alpha1.Task) { task.Spec.AgentType = rlarkv1alpha1.AgentTypeDocker }},
		{"domain", func(task *rlarkv1alpha1.Task) { task.Spec.Domain = "new-domain" }},
		{"unsupported workload", func(task *rlarkv1alpha1.Task) { task.Spec.Kubernetes.Workload.Kind = "Unsupported" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			_ = appsv1.AddToScheme(scheme)
			_ = corev1.AddToScheme(scheme)
			_ = rlarkv1alpha1.AddToScheme(scheme)
			task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", Finalizers: []string{ManagementTaskFinalizer}, Annotations: map[string]string{ManagementTaskClaimantAnnotation: "namespace/Kubernetes", ManagementTaskClaimedDomainAnnotation: "old-domain"}}, Spec: rlarkv1alpha1.TaskSpec{
				AgentType: rlarkv1alpha1.AgentTypeKubernetes, Domain: "old-domain", Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Kind: rlarkv1alpha1.KubernetesWorkloadDeployment}},
			}}
			tt.mutate(task)
			workload := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{ManagementTaskUIDAnnotation: "uid"}}}
			management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
			local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workload).Build()
			r := &pullReconciler{c: NewTaskController(base.Controller{AgentType: "Kubernetes", ManagementNamespace: "namespace", ManagementClient: management, LocalKubeClient: local})}
			request := reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}
			for range 2 {
				if _, err := r.Reconcile(context.Background(), request); err != nil {
					t.Fatal(err)
				}
			}
			var got rlarkv1alpha1.Task
			if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); err != nil {
				t.Fatal(err)
			}
			if containsString(got.Finalizers, ManagementTaskFinalizer) || got.Annotations[ManagementTaskClaimantAnnotation] != "" {
				t.Fatalf("claim was not released: finalizers=%v annotations=%v", got.Finalizers, got.Annotations)
			}
			var deleted appsv1.Deployment
			if err := local.Get(context.Background(), client.ObjectKeyFromObject(workload), &deleted); client.IgnoreNotFound(err) != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLegacyClaimDomainHandoffOnlyByExactScope(t *testing.T) {
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{ManagementTaskClaimantAnnotation: "shared/Kubernetes/old-domain"}}, Spec: rlarkv1alpha1.TaskSpec{AgentType: rlarkv1alpha1.AgentTypeKubernetes, Domain: "new-domain", Kubernetes: &rlarkv1alpha1.KubernetesTaskSpec{Workload: &rlarkv1alpha1.KubernetesWorkloadSpec{Kind: rlarkv1alpha1.KubernetesWorkloadDeployment}}}}
	owner := &pullReconciler{c: NewTaskController(base.Controller{AgentType: "Kubernetes", ManagementNamespace: "shared"})}
	other := &pullReconciler{c: NewTaskController(base.Controller{AgentType: "Kubernetes", ManagementNamespace: "other"})}
	if !owner.isTaskClaimant(task) || owner.claimedDomain(task) != "old-domain" || owner.canRealize(task) {
		t.Fatal("legacy claimant did not retain ownership for domain handoff")
	}
	if other.isTaskClaimant(task) {
		t.Fatal("wrong namespace matched legacy claim")
	}
}

func TestUnownedConflictDoesNotRetainFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = appsv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	_ = rlarkv1alpha1.AddToScheme(scheme)
	now := metav1.Now()
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default", UID: "uid", DeletionTimestamp: &now, Finalizers: []string{ManagementTaskFinalizer}, Annotations: map[string]string{ManagementTaskClaimantAnnotation: "namespace/Kubernetes", ManagementTaskClaimedDomainAnnotation: "domain"}}, Spec: rlarkv1alpha1.TaskSpec{AgentType: rlarkv1alpha1.AgentTypeKubernetes, Domain: "domain"}}
	conflict := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "rlark-system", Annotations: map[string]string{ManagementTaskUIDAnnotation: "other"}}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).Build()
	local := fake.NewClientBuilder().WithScheme(scheme).WithObjects(conflict).Build()
	r := &pullReconciler{c: NewTaskController(base.Controller{AgentType: "Kubernetes", ManagementNamespace: "namespace", ManagementClient: management, LocalKubeClient: local})}
	if _, err := r.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(task)}); err != nil {
		t.Fatal(err)
	}
	var got rlarkv1alpha1.Task
	if err := management.Get(context.Background(), client.ObjectKeyFromObject(task), &got); client.IgnoreNotFound(err) != nil {
		t.Fatal(err)
	}
	var remaining appsv1.Deployment
	if err := local.Get(context.Background(), client.ObjectKeyFromObject(conflict), &remaining); err != nil || !remaining.DeletionTimestamp.IsZero() {
		t.Fatalf("unowned conflict was modified: %v", err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestUpdateStatusClearsObservedNodesSnapshot(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = rlarkv1alpha1.AddToScheme(scheme)
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: "default"}, Status: rlarkv1alpha1.TaskStatus{
		Phase: rlarkv1alpha1.TaskPhaseRunning, ObservedNodes: []string{"old"},
	}}
	management := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(task).WithObjects(task).Build()
	var current rlarkv1alpha1.Task
	_ = management.Get(context.Background(), client.ObjectKeyFromObject(task), &current)
	if _, err := updateMgmtTaskStatus(context.Background(), logr.Discard(), management, &current, rlarkv1alpha1.TaskPhaseRunning, "", []string{}); err != nil {
		t.Fatal(err)
	}
	_ = management.Get(context.Background(), client.ObjectKeyFromObject(task), &current)
	if len(current.Status.ObservedNodes) != 0 {
		t.Fatalf("observedNodes was not cleared: %v", current.Status.ObservedNodes)
	}
}
