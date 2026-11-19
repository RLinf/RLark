package agent

import (
	"context"
	"errors"

	"golang.org/x/sync/errgroup"
)

// startTogether runs the agent's managers until the first of them stops, then
// stops the others and returns why. An agent is all of its managers: when one
// gives up (its first watches of the management plane did not sync in time,
// say), the agent that went on with the rest held its lease, reported its
// heartbeat and did nothing else, for as long as nobody restarted it. Now the
// process ends, and whatever keeps it (its Deployment) starts it anew.
func startTogether(ctx context.Context, starts ...func(context.Context) error) error {
	eg, ctx := errgroup.WithContext(ctx)
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	for _, start := range starts {
		eg.Go(func() error {
			defer stop()
			if err := start(ctx); err != nil {
				return err
			}
			if ctx.Err() == nil {
				return errors.New("a manager stopped while the agent was to run")
			}
			return nil
		})
	}
	return eg.Wait()
}
