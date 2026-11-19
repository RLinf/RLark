package controller

import (
	"context"
	"errors"
	"testing"
)

func TestIdentityCoordinatorCancelledWaiterDoesNotAcquire(t *testing.T) {
	coordinator := NewSerializedIdentityOperations()
	release, err := coordinator.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := coordinator.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter: %v", err)
	}
	release()
	for i := 0; i < 100; i++ {
		if _, err := coordinator.Acquire(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled waiter acquired free coordinator: %v", err)
		}
	}
	release, err = coordinator.Acquire(context.Background())
	if err != nil {
		t.Fatalf("cancelled waiter retained lock: %v", err)
	}
	release()
}
