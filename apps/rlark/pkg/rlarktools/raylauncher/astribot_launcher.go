package raylauncher

import (
	"context"
	"os"
)

// runWithoutRay runs a Task without a Ray role (its Job has no head): it joins no cluster, so its image need not
// have Ray. Its run script follows prepare, and the launcher then stays up as a head's does.
func runWithoutRay(ctx context.Context, config Config) error {
	if script := os.Getenv("RLARK_RUN_SCRIPT"); script != "" {
		if err := commandRunner(ctx, config.BashBinary, "-c", script); err != nil {
			return preserveExit("run script", err)
		}
	}
	<-ctx.Done()
	return ctx.Err()
}
