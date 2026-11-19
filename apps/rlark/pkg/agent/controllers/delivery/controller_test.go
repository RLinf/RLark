package delivery

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/rlinf/rlark/apps/rlark/pkg/distribution"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeliveryPredicateEvents(t *testing.T) {
	p := secretPredicate()
	delivery := &corev1.Secret{Type: distribution.DeliverySecretType}
	replication := &corev1.Secret{Type: distribution.ReplicationSecretType}

	assert.True(t, p.Create(event.CreateEvent{Object: delivery}))
	assert.False(t, p.Create(event.CreateEvent{Object: replication}))
	assert.True(t, p.Update(event.UpdateEvent{ObjectOld: delivery, ObjectNew: replication}))
	assert.True(t, p.Delete(event.DeleteEvent{Object: delivery}))
	assert.True(t, p.Generic(event.GenericEvent{Object: delivery}))
}

func TestDeliveryReconcileIgnoresOtherManagementNamespace(t *testing.T) {
	r := &Reconciler{managementNamespace: "rlark-cluster-a"}
	result, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: clientKey("other", "delivery")})
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestDeliveryNamespaceResolver(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: "tenant-a", UID: "namespace-uid", Labels: map[string]string{"managed": "true"}},
		Status:     corev1.NamespaceStatus{Phase: corev1.NamespaceActive},
	}).Build()

	items, err := (namespaceResolver{client: c}).List(context.Background())
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "tenant-a", items[0].Name)
	assert.Equal(t, "namespace-uid", string(items[0].UID))
}

func TestNamespaceReconcileContinuesAfterDeliveryFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "broken", Namespace: "rlark-cluster-a"}, Type: distribution.DeliverySecretType},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "healthy", Namespace: "rlark-cluster-a"}, Type: distribution.DeliverySecretType},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ignored", Namespace: "rlark-cluster-a"}, Type: corev1.SecretTypeOpaque},
	).Build()
	var reconciled []string
	r := &Reconciler{
		managementClient:    c,
		managementNamespace: "rlark-cluster-a",
		reconcileDelivery: func(_ context.Context, request ctrl.Request) (ctrl.Result, error) {
			reconciled = append(reconciled, request.Name)
			if request.Name == "broken" {
				return ctrl.Result{}, errors.New("broken delivery")
			}
			return ctrl.Result{}, nil
		},
	}

	_, err := r.reconcileNamespace(context.Background(), ctrl.Request{})
	require.ErrorContains(t, err, "broken delivery")
	assert.ElementsMatch(t, []string{"broken", "healthy"}, reconciled)
}

func clientKey(namespace, name string) types.NamespacedName {
	return types.NamespacedName{Namespace: namespace, Name: name}
}
