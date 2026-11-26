package agent

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// supervise runs independent roles under one cancellation boundary and joins
// every component before returning. Reconciliation retries stay inside roles.
func supervise(ctx context.Context, components ...func(context.Context) error) error {
	group, runCtx := errgroup.WithContext(ctx)
	for _, component := range components {
		group.Go(func() error {
			err := component(runCtx)
			if err == nil && runCtx.Err() == nil {
				return fmt.Errorf("agent component exited unexpectedly")
			}
			return err
		})
	}
	return group.Wait()
}
