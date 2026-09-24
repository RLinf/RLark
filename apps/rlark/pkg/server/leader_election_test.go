package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rlinf/rlark/apps/rlark/pkg/configs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func testLeaderElectionConfig() configs.LeaderElectionConfig {
	return configs.LeaderElectionConfig{
		Key:           "test-lock",
		Identity:      "test-instance",
		LeaseDuration: 500 * time.Millisecond,
		RenewDeadline: 300 * time.Millisecond,
		RetryPeriod:   100 * time.Millisecond,
	}
}

func TestRunLeaderInitialization(t *testing.T) {
	client := fake.NewSimpleClientset()
	called := false
	if err := runLeaderInitialization(context.Background(), client, "default", testLeaderElectionConfig(), func(context.Context) error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("runLeaderInitialization() error = %v", err)
	}
	if !called {
		t.Fatal("initializer was not called")
	}
	lease, err := client.CoordinationV1().Leases("default").Get(context.Background(), "test-lock", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get leader lease: %v", err)
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != "test-instance" {
		t.Fatalf("holder identity = %v, want test-instance", lease.Spec.HolderIdentity)
	}
}

func TestRunLeaderInitializationReturnsInitializerError(t *testing.T) {
	want := errors.New("initialize failed")
	err := runLeaderInitialization(context.Background(), fake.NewSimpleClientset(), "default", testLeaderElectionConfig(), func(context.Context) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("runLeaderInitialization() error = %v, want %v", err, want)
	}
}

func TestRunLeaderInitializationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := runLeaderInitialization(ctx, fake.NewSimpleClientset(), "default", testLeaderElectionConfig(), func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runLeaderInitialization() error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("initializer was called after context cancellation")
	}
}
