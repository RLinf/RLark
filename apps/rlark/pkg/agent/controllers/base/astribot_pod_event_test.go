package base

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func podEvent(reason, eventType string, pod *corev1.Pod) *corev1.Event {
	return &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "e", Namespace: pod.Namespace}, Type: eventType, Reason: reason,
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Namespace: pod.Namespace, Name: pod.Name, UID: pod.UID}}
}

func TestPodStartFailureEventReobservesTheTasksWorkload(t *testing.T) {
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "job-c2", Namespace: "rlark-system", UID: "sts-uid"}}
	pod := makePod("job-c2-0", "rlark-system", controllerRef("StatefulSet", sts.Name, sts.UID))
	pod.UID = "pod-uid"
	c := fake.NewClientBuilder().WithObjects(sts, pod).Build()

	filter := isPodStartFailure()
	assert.True(t, filter.Create(event.CreateEvent{Object: podEvent("FailedMount", corev1.EventTypeWarning, pod)}))
	assert.False(t, filter.Create(event.CreateEvent{Object: podEvent("Pulled", corev1.EventTypeNormal, pod)}))
	assert.False(t, filter.Create(event.CreateEvent{Object: podEvent("BackOff", corev1.EventTypeWarning, pod)}))

	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer queue.ShutDown()
	enqueueEventPodWorkload(c, "StatefulSet").Create(context.Background(), event.CreateEvent{Object: podEvent("FailedMount", corev1.EventTypeWarning, pod)}, queue)
	assert.Equal(t, 1, queue.Len())
	request, _ := queue.Get()
	assert.Equal(t, reconcile.Request{NamespacedName: types.NamespacedName{Namespace: "rlark-system", Name: "job-c2"}}, request)

	// An Event of an earlier Pod of the same name is not this Pod's.
	stale := podEvent("FailedMount", corev1.EventTypeWarning, pod)
	stale.InvolvedObject.UID = "old-pod-uid"
	enqueueEventPodWorkload(c, "StatefulSet").Create(context.Background(), event.CreateEvent{Object: stale}, queue)
	assert.Equal(t, 0, queue.Len())
}
