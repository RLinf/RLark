package agent

import (
	"context"
	"errors"

	"golang.org/x/sync/errgroup"
)

// together runs the parts of an agent until the first of them stops, then
// stops the others. An agent is all of its parts: when one gives up (its first
// watches of the management plane did not sync in time, say), the agent that
// went on with the rest held its lease, reported its heartbeat and did nothing
// else, for as long as nobody restarted it. Now the process ends, and whatever
// keeps it (its Deployment or DaemonSet) starts it anew.
type together struct {
	eg   *errgroup.Group
	ctx  context.Context
	stop context.CancelFunc
}

// newTogether returns the group and the context its parts run in.
func newTogether(ctx context.Context) (*together, context.Context) {
	eg, ctx := errgroup.WithContext(ctx)
	ctx, stop := context.WithCancel(ctx)
	return &together{eg: eg, ctx: ctx, stop: stop}, ctx
}

func (t *together) Go(part func() error) {
	t.eg.Go(func() error {
		defer t.stop()
		if err := part(); err != nil {
			return err
		}
		if t.ctx.Err() == nil {
			return errors.New("a manager stopped while the agent was to run")
		}
		return nil
	})
}

// Wait returns why the first part stopped, once all have.
func (t *together) Wait() error {
	defer t.stop()
	return t.eg.Wait()
}

// startTogether runs starts together.
func startTogether(ctx context.Context, starts ...func(context.Context) error) error {
	parts, ctx := newTogether(ctx)
	for _, start := range starts {
		parts.Go(func() error { return start(ctx) })
	}
	return parts.Wait()
}
