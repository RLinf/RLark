package framework

import (
	"context"
	"fmt"
	"time"
)

type Check func(context.Context) (done bool, observation string, err error)

func Poll(ctx context.Context, interval time.Duration, check Check) error {
	if interval <= 0 {
		return fmt.Errorf("poll interval must be positive")
	}
	var lastObservation string
	for {
		done, observation, err := check(ctx)
		if observation != "" {
			lastObservation = observation
		}
		if err != nil {
			return err
		}
		if done {
			return nil
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return fmt.Errorf("wait for condition: %w; last observation: %s", ctx.Err(), lastObservation)
		case <-timer.C:
		}
	}
}
