package task

import (
	"context"
	"errors"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	rlarkv1alpha1 "github.com/rlinf/rlark/api/rlark.io/v1alpha1"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/base"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/observation"
	kubernetesruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/controllers/task/runtime/kubernetes"
	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
	"github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/trigger"
)

var trainingGVK = schema.GroupVersionKind{Group: "example.io", Version: "v1", Kind: "TrainingJob"}

func observationFixture(t *testing.T, mutate func(*rlarkv1alpha1.Task, *unstructured.Unstructured)) (*Controller, *runtimeObservationReconciler, *rlarkv1alpha1.Task, *unstructured.Unstructured) {
	t.Helper()
	scope := KubernetesObservationScope{RuntimeID: kubernetesruntime.DefaultID, Namespace: "runtime", ManagementNamespace: "management"}
	task := &rlarkv1alpha1.Task{ObjectMeta: metav1.ObjectMeta{Name: "task", Namespace: scope.ManagementNamespace, UID: "task-uid", Generation: 2}, Spec: rlarkv1alpha1.TaskSpec{Runtime: rlarkv1alpha1.TaskRuntimeSpec{Kind: rlarkv1alpha1.RuntimeKindKubernetes}, Workload: rlarkv1alpha1.TaskWorkloadSpec{Carrier: rlarkv1alpha1.TaskWorkloadCarrier{Type: "example.io/training-job"}}}}
	root := &unstructured.Unstructured{}
	root.SetGroupVersionKind(trainingGVK)
	root.SetNamespace(scope.Namespace)
	root.SetName("root")
	root.SetUID("root-uid")
	root.SetAnnotations(map[string]string{ManagementTaskNameAnnotation: task.Name, ManagementTaskNamespaceAnnotation: task.Namespace, ManagementTaskUIDAnnotation: string(task.UID), kubernetesruntime.RuntimeIDAnnotation: string(scope.RuntimeID), kubernetesruntime.RuntimeKindAnnotation: string(kubernetesruntime.Kind)})
	if mutate != nil {
		mutate(task, root)
	}
	scheme := executionScheme(t)
	// Any observer write is a test failure, including Status subresource writes.
	forbid := interceptor.Funcs{
		Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
			t.Error("observer created an object")
			return errors.New("write forbidden")
		},
		Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
			t.Error("observer updated an object")
			return errors.New("write forbidden")
		},
		Patch: func(context.Context, client.WithWatch, client.Object, client.Patch, ...client.PatchOption) error {
			t.Error("observer patched an object")
			return errors.New("write forbidden")
		},
		SubResourceUpdate: func(context.Context, client.Client, string, client.Object, ...client.SubResourceUpdateOption) error {
			t.Error("observer wrote Status")
			return errors.New("write forbidden")
		},
		SubResourcePatch: func(context.Context, client.Client, string, client.Object, client.Patch, ...client.SubResourcePatchOption) error {
			t.Error("observer patched Status")
			return errors.New("write forbidden")
		},
	}
	c := NewTaskController(base.Controller{AgentType: "Kubernetes", ManagementNamespace: scope.ManagementNamespace,
		ManagementClient: fake.NewClientBuilder().WithScheme(scheme).WithObjects(task).WithInterceptorFuncs(forbid).Build(),
		LocalKubeClient:  fake.NewClientBuilder().WithScheme(scheme).WithObjects(root).WithInterceptorFuncs(forbid).Build(),
	})
	return observationFixtureWithController(t, c, scope, task, root)
}

func observationFixtureWithController(t *testing.T, c *Controller, scope KubernetesObservationScope, task *rlarkv1alpha1.Task, root *unstructured.Unstructured) (*Controller, *runtimeObservationReconciler, *rlarkv1alpha1.Task, *unstructured.Unstructured) {
	t.Helper()
	mapper := observationDependencyMapper()
	// The explicit CRD mapping is added by the test Runtime, not the Scheme.
	mapper.(*meta.DefaultRESTMapper).Add(trainingGVK, meta.RESTScopeNamespace)
	if err := c.WithKubernetesObservations(c.LocalKubeClient.Scheme(), mapper, []observation.Kubernetes{{GVK: trainingGVK, CarrierType: "example.io/training-job", InstanceProfile: kubernetesruntime.ObservationDirectControllerV1}}); err != nil {
		t.Fatal(err)
	}
	c.observationScope = &scope
	profile, err := kubernetesruntime.BuiltinObservationProfiles().Resolve(kubernetesruntime.ObservationDirectControllerV1, trainingGVK)
	if err != nil {
		t.Fatal(err)
	}
	push, err := newPushRuntimeReconciler(c, c.observations[0], scope, c.LocalKubeClient)
	if err != nil {
		t.Fatal(err)
	}
	return c, &runtimeObservationReconciler{push: *push, profile: &profile}, task, root
}

func TestScopedRootObservationRejectsForeignIdentity(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*rlarkv1alpha1.Task, *unstructured.Unstructured)
	}{
		{"foreign namespace", func(_ *rlarkv1alpha1.Task, r *unstructured.Unstructured) { r.SetNamespace("foreign") }},
		{"foreign management namespace", func(_ *rlarkv1alpha1.Task, r *unstructured.Unstructured) {
			a := r.GetAnnotations()
			a[ManagementTaskNamespaceAnnotation] = "foreign"
			r.SetAnnotations(a)
		}},
		{"malformed identity", func(_ *rlarkv1alpha1.Task, r *unstructured.Unstructured) {
			a := r.GetAnnotations()
			delete(a, ManagementTaskNameAnnotation)
			r.SetAnnotations(a)
		}},
		{"old Task UID", func(_ *rlarkv1alpha1.Task, r *unstructured.Unstructured) {
			a := r.GetAnnotations()
			a[ManagementTaskUIDAnnotation] = "old-uid"
			r.SetAnnotations(a)
		}},
		{"other Runtime ID", func(_ *rlarkv1alpha1.Task, r *unstructured.Unstructured) {
			a := r.GetAnnotations()
			a[kubernetesruntime.RuntimeIDAnnotation] = "other"
			r.SetAnnotations(a)
		}},
		{"other Runtime kind", func(_ *rlarkv1alpha1.Task, r *unstructured.Unstructured) {
			a := r.GetAnnotations()
			a[kubernetesruntime.RuntimeKindAnnotation] = "Raw"
			r.SetAnnotations(a)
		}},
		{"other Task Runtime ID", func(task *rlarkv1alpha1.Task, _ *unstructured.Unstructured) { task.Spec.Runtime.Name = "other" }},
		{"other Task Runtime kind", func(task *rlarkv1alpha1.Task, _ *unstructured.Unstructured) { task.Spec.Runtime.Kind = "Raw" }},
		{"carrier mismatch", func(task *rlarkv1alpha1.Task, _ *unstructured.Unstructured) {
			task.Spec.Workload.Carrier.Type = "other"
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, r, _, root := observationFixture(t, tc.mutate)
			if _, err := r.Reconcile(context.Background(), runtimeObservationRequest{Key: client.ObjectKeyFromObject(root), UID: root.GetUID()}); err != nil {
				t.Fatal(err)
			}
			if len(c.triggers.events) != 0 {
				t.Fatal("foreign root emitted a trigger")
			}
		})
	}
}

func TestPodEventsRequireManagementTaskMarkerBeforeEnqueue(t *testing.T) {
	for _, marked := range []bool{false, true} {
		for _, source := range []string{"create", "update", "delete", "tombstone", "generic"} {
			t.Run(fmt.Sprintf("%s/marked=%t", source, marked), func(t *testing.T) {
				_, r, task, root := observationFixture(t, nil)
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: root.GetNamespace(), UID: "pod-uid", OwnerReferences: []metav1.OwnerReference{{APIVersion: trainingGVK.GroupVersion().String(), Kind: trainingGVK.Kind, Name: root.GetName(), UID: root.GetUID(), Controller: ptr.To(true)}}}}
				if marked {
					pod.Annotations = map[string]string{ManagementTaskNameAnnotation: task.Name}
				}
				q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[runtimeObservationRequest]())
				defer q.ShutDown()
				h := runtimePodHandler(r.push.scope)
				ctx := context.Background()
				switch source {
				case "create":
					h.Create(ctx, event.TypedCreateEvent[client.Object]{Object: pod}, q)
				case "update":
					h.Update(ctx, event.TypedUpdateEvent[client.Object]{ObjectOld: pod.DeepCopy(), ObjectNew: pod}, q)
				case "delete", "tombstone":
					h.Delete(ctx, event.TypedDeleteEvent[client.Object]{Object: pod, DeleteStateUnknown: source == "tombstone"}, q)
				case "generic":
					h.Generic(ctx, event.TypedGenericEvent[client.Object]{Object: pod}, q)
				}
				want := 0
				if marked {
					want = 1
				}
				if q.Len() != want {
					t.Fatalf("queue length=%d want=%d", q.Len(), want)
				}
			})
		}
	}
}

func TestPodMarkerRemovalSkipsOwnerReadsAfterEnqueue(t *testing.T) {
	c, r, task, root := observationFixture(t, nil)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: root.GetNamespace(), UID: "pod-uid", Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name}}}
	pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: trainingGVK.GroupVersion().String(), Kind: trainingGVK.Kind, Name: root.GetName(), UID: root.GetUID(), Controller: ptr.To(true)}}
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[runtimeObservationRequest]())
	defer q.ShutDown()
	unmarked := pod.DeepCopy()
	unmarked.Annotations = nil
	// Both marker removal and addition must pass the event filter via either
	// the old or new object, while the live state decides whether to read owners.
	h := runtimePodHandler(r.push.scope)
	h.Update(context.Background(), event.TypedUpdateEvent[client.Object]{ObjectOld: pod, ObjectNew: unmarked}, q)
	h.Update(context.Background(), event.TypedUpdateEvent[client.Object]{ObjectOld: unmarked, ObjectNew: pod}, q)
	if q.Len() != 1 {
		t.Fatalf("marker transition queue length=%d", q.Len())
	}
	reads := 0
	c.LocalKubeClient = fake.NewClientBuilder().WithScheme(c.LocalKubeClient.Scheme()).WithObjects(unmarked).WithInterceptorFuncs(interceptor.Funcs{
		Get: func(ctx context.Context, reader client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			reads++
			if _, ok := obj.(*corev1.Pod); !ok {
				t.Fatal("unmarked Pod caused an owner/root read")
			}
			return reader.Get(ctx, key, obj, opts...)
		},
	}).Build()
	r.push.observedReader = c.LocalKubeClient
	req, _ := q.Get()
	q.Done(req)
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if reads != 1 || len(c.triggers.events) != 0 {
		t.Fatalf("reads=%d hints=%d", reads, len(c.triggers.events))
	}
}

func TestCRDRootAndPodTriggersCarryRegisteredGVKAndRuntime(t *testing.T) {
	for _, source := range []string{"root", "pod-create", "pod-delete"} {
		t.Run(source, func(t *testing.T) {
			deleted := source == "pod-delete"
			c, r, task, root := observationFixture(t, nil)
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: root.GetNamespace(), UID: "pod-uid", OwnerReferences: []metav1.OwnerReference{{APIVersion: trainingGVK.GroupVersion().String(), Kind: trainingGVK.Kind, Name: root.GetName(), UID: root.GetUID(), Controller: ptr.To(true)}}}}
			pod.Annotations = map[string]string{ManagementTaskNameAnnotation: task.Name}
			if !deleted {
				c.LocalKubeClient = fake.NewClientBuilder().WithScheme(c.LocalKubeClient.Scheme()).WithObjects(root, pod).Build()
			}
			q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[runtimeObservationRequest]())
			r.push.observedReader = c.LocalKubeClient
			t.Cleanup(q.ShutDown)
			if source == "root" {
				runtimeRootHandler(r.push.scope).Create(context.Background(), event.TypedCreateEvent[client.Object]{Object: root}, q)
			} else if deleted {
				runtimePodHandler(r.push.scope).Delete(context.Background(), event.TypedDeleteEvent[client.Object]{Object: pod, DeleteStateUnknown: true}, q)
			} else {
				runtimePodHandler(r.push.scope).Create(context.Background(), event.TypedCreateEvent[client.Object]{Object: pod}, q)
			}
			if q.Len() != 1 {
				t.Fatalf("pod queue length=%d", q.Len())
			}
			req, _ := q.Get()
			q.Done(req)
			if _, err := r.Reconcile(context.Background(), req); err != nil {
				t.Fatal(err)
			}
			set, ok := c.triggers.accumulator.Take(trigger.Key{Namespace: task.Namespace, Name: task.Name})
			if !ok || set.FullReconcileRequired || len(set.Roots) != 1 || set.Roots[0].ID != trainingGVK.String() || set.Roots[0].RuntimeID != r.push.scope.RuntimeID || set.Roots[0].UID != string(root.GetUID()) || set.ObjectUID != string(task.UID) {
				t.Fatalf("trigger=%+v", set)
			}
			ref := set.Roots[0]
			object, err := c.executionRootObject(context.Background(), ref)
			if err != nil || object.GetObjectKind().GroupVersionKind() != trainingGVK || object.GetUID() != root.GetUID() {
				t.Fatalf("dispatch object=%v error=%v", object, err)
			}
			ref.UID = "old-root"
			if _, err := c.executionRootObject(context.Background(), ref); err == nil {
				t.Fatal("dispatch accepted stale root UID")
			}
			ref.UID, ref.Namespace = string(root.GetUID()), "foreign"
			if _, err := c.executionRootObject(context.Background(), ref); err == nil {
				t.Fatal("dispatch accepted foreign namespace")
			}
		})
	}
}

func TestDeletedRootTombstoneForcesFullReconcileAndRetriesPressure(t *testing.T) {
	c, r, task, root := observationFixture(t, nil)
	q := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[runtimeObservationRequest]())
	defer q.ShutDown()
	runtimeRootHandler(r.push.scope).Delete(context.Background(), event.TypedDeleteEvent[client.Object]{Object: root, DeleteStateUnknown: true}, q)
	if q.Len() != 1 {
		t.Fatalf("delete queue length=%d", q.Len())
	}
	req, _ := q.Get()
	q.Done(req)
	// Remove root from the test API; no observer write is involved.
	c.LocalKubeClient = fake.NewClientBuilder().WithScheme(c.LocalKubeClient.Scheme()).Build()
	c.triggers.events = make(chan event.GenericEvent, 1)
	c.triggers.events <- event.GenericEvent{}
	if _, err := r.Reconcile(context.Background(), req); !errors.Is(err, ErrTriggerBridgeSaturated) {
		t.Fatalf("error=%v", err)
	}
	<-c.triggers.events
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(c.triggers.events) != 1 {
		t.Fatal("delete retry did not wake pull controller")
	}
	set, ok := c.triggers.accumulator.Take(trigger.Key{Namespace: task.Namespace, Name: task.Name})
	if !ok || !set.FullReconcileRequired || len(set.Roots) != 0 {
		t.Fatalf("deleted root used fast evidence: %+v", set)
	}
}

func TestDeletedRootDoesNotTriggerForReplacementOrOldTaskUID(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		c, r, _, root := observationFixture(t, nil)
		req := runtimeObservationRequest{Key: client.ObjectKeyFromObject(root), UID: root.GetUID(), Deleted: true, TaskName: "task", TaskUID: "old-task"}
		if !replacement {
			c.LocalKubeClient = fake.NewClientBuilder().WithScheme(c.LocalKubeClient.Scheme()).Build()
		} else {
			req.TaskUID = "task-uid"
		}
		if _, err := r.Reconcile(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		if len(c.triggers.events) != 0 {
			t.Fatal("stale deletion emitted a trigger")
		}
	}
}

type unavailableObservationClient struct {
	client.Client
	err error
}

func (c unavailableObservationClient) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return c.err
}

func TestObservationReadFailuresRecoverOnRetry(t *testing.T) {
	c, r, _, root := observationFixture(t, nil)
	local := c.LocalKubeClient
	want := errors.New("unavailable")
	c.LocalKubeClient = unavailableObservationClient{Client: local, err: want}
	r.push.observedReader = c.LocalKubeClient
	req := runtimeObservationRequest{Key: client.ObjectKeyFromObject(root), UID: root.GetUID()}
	if _, err := r.Reconcile(context.Background(), req); !errors.Is(err, want) {
		t.Fatalf("error=%v", err)
	}
	c.LocalKubeClient = local
	r.push.observedReader = local
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(c.triggers.events) != 1 {
		t.Fatal("retry did not emit trigger")
	}
}

func TestRuntimeScopeIsolation(t *testing.T) {
	for _, id := range []agentruntime.ID{"first", "second"} {
		c, r, _, root := observationFixture(t, func(task *rlarkv1alpha1.Task, root *unstructured.Unstructured) {
			task.Spec.Runtime.Name = string(id)
			a := root.GetAnnotations()
			a[kubernetesruntime.RuntimeIDAnnotation] = string(id)
			root.SetAnnotations(a)
		})
		scope := r.push.scope
		scope.RuntimeID = id
		r.push.scope = scope
		c.observationScope = &scope
		if _, err := r.Reconcile(context.Background(), runtimeObservationRequest{Key: client.ObjectKeyFromObject(root)}); err != nil {
			t.Fatal(err)
		}
		if len(c.triggers.events) != 1 {
			t.Fatal("own Runtime root did not trigger")
		}
		scope.RuntimeID = "foreign"
		r.push.scope = scope
		c.observationScope = &scope
		<-c.triggers.events
		if _, err := r.Reconcile(context.Background(), runtimeObservationRequest{Key: client.ObjectKeyFromObject(root)}); err != nil {
			t.Fatal(err)
		}
		if len(c.triggers.events) != 0 {
			t.Fatal("cross-Runtime trigger")
		}
	}
}

func TestPushReconcilerRejectsInvalidScopeAtConstruction(t *testing.T) {
	c, r, _, _ := observationFixture(t, nil)
	for _, scope := range []KubernetesObservationScope{
		{},
		{Namespace: "runtime", ManagementNamespace: "management"},
		{RuntimeID: "custom", ManagementNamespace: "management"},
		{RuntimeID: "custom", Namespace: "runtime"},
	} {
		if push, err := newPushRuntimeReconciler(c, r.push.observation, scope, c.LocalKubeClient); err == nil || push != nil {
			t.Fatalf("invalid scope accepted: scope=%+v push=%v error=%v", scope, push, err)
		}
	}
}

func TestPushReconcilerScopeIsIndependentOfControllerSetupState(t *testing.T) {
	c, r, task, root := observationFixture(t, func(task *rlarkv1alpha1.Task, root *unstructured.Unstructured) {
		task.Spec.Runtime.Name = "custom"
		a := root.GetAnnotations()
		a[kubernetesruntime.RuntimeIDAnnotation] = "custom"
		root.SetAnnotations(a)
	})
	scope := r.push.scope
	scope.RuntimeID = "custom"
	push, err := newPushRuntimeReconciler(c, r.push.observation, scope, c.LocalKubeClient)
	if err != nil {
		t.Fatal(err)
	}
	// Neither mutation of the caller's value nor missing controller setup state
	// may change the reconciler's validated identity or enable a default fallback.
	scope.RuntimeID = "foreign"
	c.observationScope = nil
	if _, err := push.Reconcile(context.Background(), reconcile.Request{NamespacedName: client.ObjectKeyFromObject(root)}); err != nil {
		t.Fatal(err)
	}
	set, ok := c.triggers.accumulator.Take(trigger.Key{Namespace: task.Namespace, Name: task.Name})
	if !ok || set.RuntimeID != "custom" || len(set.Roots) != 1 || set.Roots[0].RuntimeID != "custom" {
		t.Fatalf("trigger=%+v", set)
	}
}

func TestObservedReadsNeverFallBackToDirectClient(t *testing.T) {
	c, r, task, root := observationFixture(t, nil)
	directError := errors.New("direct root reads forbidden")
	c.LocalKubeClient = unavailableObservationClient{Client: c.LocalKubeClient, err: directError}
	req := runtimeObservationRequest{Key: client.ObjectKeyFromObject(root), UID: root.GetUID()}
	if _, err := r.Reconcile(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	key := trigger.Key{Namespace: task.Namespace, Name: task.Name}
	c.triggers.accumulator.Take(key)
	c.triggers.finish(key)
	<-c.triggers.events
	for _, readError := range []error{&cache.ErrResourceNotCached{GVK: trainingGVK}, errors.New("cache unavailable")} {
		r.push.observedReader = unavailableObservationClient{Client: c.LocalKubeClient, err: readError}
		if _, err := r.Reconcile(context.Background(), req); !errors.Is(err, readError) {
			t.Fatalf("cache error was swallowed: %v", err)
		}
	}
}

func TestPodHintUsesCachedPodAndRootWithoutDuplicateRead(t *testing.T) {
	c, r, task, root := observationFixture(t, nil)
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "pod", Namespace: root.GetNamespace(), UID: "pod-uid", Annotations: map[string]string{ManagementTaskNameAnnotation: task.Name}, OwnerReferences: []metav1.OwnerReference{{APIVersion: trainingGVK.GroupVersion().String(), Kind: trainingGVK.Kind, Name: root.GetName(), UID: root.GetUID(), Controller: ptr.To(true)}}}}
	reads := 0
	r.push.observedReader = fake.NewClientBuilder().WithScheme(c.LocalKubeClient.Scheme()).WithObjects(pod, root).WithInterceptorFuncs(interceptor.Funcs{Get: func(ctx context.Context, reader client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
		reads++
		return reader.Get(ctx, key, obj, opts...)
	}}).Build()
	c.LocalKubeClient = unavailableObservationClient{Client: c.LocalKubeClient, err: errors.New("unexpected direct Pod/root read")}
	if _, err := r.Reconcile(context.Background(), runtimeObservationRequest{Key: client.ObjectKeyFromObject(pod), UID: pod.UID, Pod: true}); err != nil {
		t.Fatal(err)
	}
	if reads != 2 || len(c.triggers.events) != 1 {
		t.Fatalf("cache reads=%d hints=%d", reads, len(c.triggers.events))
	}
}

func TestDeletedRootConfirmationIgnoresLaggingCache(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		c, r, _, root := observationFixture(t, nil)
		// Cache still holds the deleted incarnation, while direct state decides
		// whether a replacement exists. Never infer absence from cached state.
		r.push.observedReader = unavailableObservationClient{Client: c.LocalKubeClient, err: errors.New("cache must not be read for deletion confirmation")}
		if replacement {
			newRoot := root.DeepCopy()
			newRoot.SetUID("replacement-uid")
			c.LocalKubeClient = fake.NewClientBuilder().WithScheme(c.LocalKubeClient.Scheme()).WithObjects(newRoot).Build()
		} else {
			c.LocalKubeClient = fake.NewClientBuilder().WithScheme(c.LocalKubeClient.Scheme()).Build()
		}
		if _, err := r.Reconcile(context.Background(), runtimeObservationRequest{Key: client.ObjectKeyFromObject(root), UID: root.GetUID(), Deleted: true, TaskName: "task", TaskUID: "task-uid"}); err != nil {
			t.Fatal(err)
		}
		want := 1
		if replacement {
			want = 0
		}
		if len(c.triggers.events) != want {
			t.Fatalf("replacement=%t hints=%d", replacement, len(c.triggers.events))
		}
	}
}
