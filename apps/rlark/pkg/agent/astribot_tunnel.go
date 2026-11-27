package agent

import (
	"context"
	"errors"
)

// The tunnel says whether the agent is ready (readiness.announce), and the
// Server keeps the heartbeat of a ready agent only. The agent connects before
// it is ready (the Server makes the agent's namespace and permissions when it
// connects, and the first watches need them), and connects anew whenever its
// readiness changes: a session that ends for that reason ends with errReadinessChanged.
var errReadinessChanged = errors.New("the agent's readiness changed")

// closeWhenChanged ends a session when the agent's readiness changes or the
// agent stops: a session waits for the Server's next stream and does not look
// at its context meanwhile, and closing it ends the wait.
func closeWhenChanged(session interface{ Close() }, changed <-chan struct{}, sessCtx context.Context) {
	select {
	case <-changed:
	case <-sessCtx.Done():
	}
	session.Close()
}

// changedOr is why a session ended: the agent's readiness changed, or err.
func changedOr(changed <-chan struct{}, err error) error {
	select {
	case <-changed:
		return errReadinessChanged
	default:
		return err
	}
}

// reconnectOnChange connects again at once, without the wait between
// attempts, when a session ended because the agent's readiness changed.
func reconnectOnChange(ctx context.Context, connect func() error) func() error {
	return func() error {
		for {
			err := connect()
			if !errors.Is(err, errReadinessChanged) {
				return err
			}
			if ctx.Err() != nil {
				return nil
			}
		}
	}
}
