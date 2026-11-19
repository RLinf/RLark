package controller

import (
	"context"
)

// IdentityOperationCoordinator serializes Station creation and rebinds so
// separate controllers cannot concurrently claim the same member Node UID.
type IdentityOperationCoordinator interface {
	Acquire(context.Context) (release func(), err error)
}

// SerializedIdentityOperations is shared by the Station and Rebind reconcilers
// inside the leader-elected controller manager.
type SerializedIdentityOperations struct {
	gate chan struct{}
}

func NewSerializedIdentityOperations() *SerializedIdentityOperations {
	return &SerializedIdentityOperations{gate: make(chan struct{}, 1)}
}

func (c *SerializedIdentityOperations) Acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case c.gate <- struct{}{}:
		// Cancellation and acquisition can become ready concurrently. Do not
		// hand an already-cancelled operation the lock in that case.
		if err := ctx.Err(); err != nil {
			<-c.gate
			return nil, err
		}
		return func() { <-c.gate }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
