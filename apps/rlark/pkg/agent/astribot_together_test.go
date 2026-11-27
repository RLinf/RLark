package agent

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A manager that fails takes the agent down with it: the other is stopped, and the agent returns the failure
// instead of running on with what is left.
func TestStartTogetherStopsEveryManagerWhenOneFails(t *testing.T) {
	failed := errors.New("failed waiting for *v1alpha1.Node Informer to sync")
	stopped := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- startTogether(context.Background(),
			func(context.Context) error { return failed },
			func(ctx context.Context) error { <-ctx.Done(); close(stopped); return nil })
	}()
	select {
	case err := <-done:
		if !errors.Is(err, failed) {
			t.Fatalf("returned %v, want the manager's failure", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the agent ran on after one of its managers had failed")
	}
	<-stopped
}

// A manager that returns without an error while nobody stopped the agent is as gone as one that failed.
func TestStartTogetherReportsAManagerThatStoppedByItself(t *testing.T) {
	err := startTogether(context.Background(),
		func(context.Context) error { return nil },
		func(ctx context.Context) error { <-ctx.Done(); return nil })
	if err == nil {
		t.Fatal("a manager stopped by itself and the agent returned no error")
	}
}

// Stopped by its caller, the agent stops its managers and returns nothing.
func TestStartTogetherStopsCleanlyWithItsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := startTogether(ctx, func(ctx context.Context) error { <-ctx.Done(); return nil }); err != nil {
		t.Fatalf("returned %v", err)
	}
}
