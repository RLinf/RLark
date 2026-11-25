package server

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rlinf/rlark/apps/rlark/pkg/apis"
)

const (
	heartbeatTestNamespace = "rlark-system-cluster-a"
	heartbeatTestName      = "node-heartbeat-node-a"
)

func heartbeatTestRenewer(client kubernetes.Interface, now time.Time) *heartbeatLeaseRenewer {
	return &heartbeatLeaseRenewer{
		client:        client,
		namespace:     heartbeatTestNamespace,
		name:          heartbeatTestName,
		identity:      "connection-2",
		leaseDuration: 45 * time.Second,
		retryPeriod:   time.Hour,
		now:           func() time.Time { return now },
	}
}

func getHeartbeatTestLease(t *testing.T, client kubernetes.Interface) *coordinationv1.Lease {
	t.Helper()
	lease, err := client.CoordinationV1().Leases(heartbeatTestNamespace).Get(context.Background(), heartbeatTestName, metav1.GetOptions{})
	require.NoError(t, err)
	return lease
}

func TestHeartbeatRenewerCreatesOwnedLease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	client := k8sfake.NewSimpleClientset()
	renewer := heartbeatTestRenewer(client, now)

	require.NoError(t, renewer.renew(context.Background()))

	lease := getHeartbeatTestLease(t, client)
	assert.Equal(t, "connection-2", ptr.Deref(lease.Spec.HolderIdentity, ""))
	assert.Equal(t, int32(45), ptr.Deref(lease.Spec.LeaseDurationSeconds, 0))
	require.NotNil(t, lease.Spec.RenewTime)
	assert.True(t, lease.Spec.RenewTime.Time.Equal(now), "renew time = %s", lease.Spec.RenewTime.Time)
	require.NotNil(t, lease.Spec.AcquireTime)
	assert.Equal(t, heartbeatLeaseManagedByValue, lease.Labels[heartbeatLeaseManagedByLabel])
}

func TestHeartbeatRenewerAdoptsLegacyLease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	previous := metav1.NewMicroTime(now.Add(-45 * time.Second))
	client := k8sfake.NewSimpleClientset(&coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: heartbeatTestName, Namespace: heartbeatTestNamespace},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To("connection-1"),
			LeaseDurationSeconds: ptr.To(int32(45)),
			RenewTime:            &previous,
		},
	})
	renewer := heartbeatTestRenewer(client, now)

	require.NoError(t, renewer.renew(context.Background()))

	lease := getHeartbeatTestLease(t, client)
	assert.Equal(t, "connection-2", ptr.Deref(lease.Spec.HolderIdentity, ""))
	assert.Equal(t, heartbeatLeaseManagedByValue, lease.Labels[heartbeatLeaseManagedByLabel])
	require.NotNil(t, lease.Spec.RenewTime)
	assert.True(t, lease.Spec.RenewTime.Time.Equal(now))
}

func TestHeartbeatRenewerRejectsForeignLease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	previous := metav1.NewMicroTime(now.Add(-time.Minute))
	client := k8sfake.NewSimpleClientset(&coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      heartbeatTestName,
			Namespace: heartbeatTestNamespace,
			Labels:    map[string]string{heartbeatLeaseManagedByLabel: "another-component"},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To("foreign"),
			LeaseDurationSeconds: ptr.To(int32(45)),
			RenewTime:            &previous,
		},
	})
	renewer := heartbeatTestRenewer(client, now)

	err := renewer.renew(context.Background())
	require.Error(t, err)
	assert.True(t, stderrors.Is(err, errHeartbeatLeaseOwnership), "err = %v", err)

	lease := getHeartbeatTestLease(t, client)
	assert.Equal(t, "foreign", ptr.Deref(lease.Spec.HolderIdentity, ""))
	require.NotNil(t, lease.Spec.RenewTime)
	assert.True(t, lease.Spec.RenewTime.Time.Equal(previous.Time))
}

func TestHeartbeatRenewerRejectsAmbiguousLease(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	client := k8sfake.NewSimpleClientset(&coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: heartbeatTestName, Namespace: heartbeatTestNamespace},
	})
	renewer := heartbeatTestRenewer(client, now)

	err := renewer.renew(context.Background())
	require.Error(t, err)
	assert.True(t, stderrors.Is(err, errHeartbeatLeaseOwnership), "err = %v", err)

	lease := getHeartbeatTestLease(t, client)
	assert.Nil(t, lease.Spec.RenewTime)
}

func TestHeartbeatRenewerRetriesAfterConflict(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	previous := metav1.NewMicroTime(now.Add(-45 * time.Second))
	client := k8sfake.NewSimpleClientset(&coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      heartbeatTestName,
			Namespace: heartbeatTestNamespace,
			Labels:    map[string]string{heartbeatLeaseManagedByLabel: heartbeatLeaseManagedByValue},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To("connection-1"),
			LeaseDurationSeconds: ptr.To(int32(45)),
			RenewTime:            &previous,
		},
	})
	updates := 0
	client.PrependReactor("update", "leases", func(ktesting.Action) (bool, kruntime.Object, error) {
		updates++
		if updates == 1 {
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Group: "coordination.k8s.io", Resource: "leases"},
				heartbeatTestName,
				stderrors.New("concurrent write"),
			)
		}
		return false, nil, nil
	})
	renewer := heartbeatTestRenewer(client, now)

	require.NoError(t, renewer.renew(context.Background()))

	assert.Equal(t, 2, updates)
	lease := getHeartbeatTestLease(t, client)
	require.NotNil(t, lease.Spec.RenewTime)
	assert.True(t, lease.Spec.RenewTime.Time.Equal(now))
}

func TestHeartbeatRenewerKeepsRenewTimeMonotonic(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	future := metav1.NewMicroTime(now.Add(time.Minute))
	client := k8sfake.NewSimpleClientset(&coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      heartbeatTestName,
			Namespace: heartbeatTestNamespace,
			Labels:    map[string]string{heartbeatLeaseManagedByLabel: heartbeatLeaseManagedByValue},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       ptr.To("connection-1"),
			LeaseDurationSeconds: ptr.To(int32(45)),
			RenewTime:            &future,
		},
	})
	renewer := heartbeatTestRenewer(client, now)

	require.NoError(t, renewer.renew(context.Background()))

	lease := getHeartbeatTestLease(t, client)
	require.NotNil(t, lease.Spec.RenewTime)
	assert.True(t, lease.Spec.RenewTime.After(future.Time), "renew time = %s", lease.Spec.RenewTime.Time)
}

func TestHeartbeatRenewerRunWritesImmediately(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	client := k8sfake.NewSimpleClientset()
	renewer := heartbeatTestRenewer(client, now)
	renewer.retryPeriod = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		renewer.Run(ctx)
	}()

	require.Eventually(t, func() bool {
		_, err := client.CoordinationV1().Leases(heartbeatTestNamespace).Get(context.Background(), heartbeatTestName, metav1.GetOptions{})
		return err == nil
	}, time.Second, 10*time.Millisecond)

	cancel()
	<-done
}

func TestStartAgentHeartbeatWritesNodeLease(t *testing.T) {
	t.Parallel()
	client := k8sfake.NewSimpleClientset()
	server := &Server{kubeClient: client, config: DefaultConfig()}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, server.startAgentHeartbeat(ctx, "cluster-a", "node-agent:node-a", "connection-1"))

	namespace := apis.RLarkAgentNamespacePrefix + "cluster-a"
	require.Eventually(t, func() bool {
		_, err := client.CoordinationV1().Leases(namespace).Get(context.Background(), "node-heartbeat-node-a", metav1.GetOptions{})
		return err == nil
	}, 5*time.Second, 10*time.Millisecond)
}
