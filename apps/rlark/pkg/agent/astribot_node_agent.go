package agent

import (
	"time"
)

// firstSyncTimeout is how long the node agent's first watches may take, as
// long as a controller's may in the cluster agent (controller-runtime's CacheSyncTimeout).
const firstSyncTimeout = 2 * time.Minute
