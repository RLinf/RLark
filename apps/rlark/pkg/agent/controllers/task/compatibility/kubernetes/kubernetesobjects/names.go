package kubernetesobjects

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	agentruntime "github.com/rlinf/rlark/apps/rlark/pkg/agent/execution/runtime"
)

// SupervisorConfigMapName returns the stable name of the Task-incarnation
// supervisor configuration object shared by planning and Runtime assembly.
func SupervisorConfigMapName(task agentruntime.TaskIdentity) string {
	sum := sha256.Sum256([]byte(task.ObjectUID))
	suffix := hex.EncodeToString(sum[:])[:10]
	base := strings.Trim(strings.ToLower(task.Logical.Name), "-.")
	if len(base) > 236 {
		base = base[:236]
	}
	return base + "-process-" + suffix
}
